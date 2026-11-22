package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func VerifyServer(server Server, environ []string) ([]string, string, error) {
	timeout := server.VerifyTimeout()
	if server.Stdio() {
		return HandshakeStdio(server, timeout, environ)
	}
	return HandshakeHTTP(server, timeout, environ)
}

// ProbeServer runs the server's declared auth probe over whichever transport
// it uses.
func ProbeServer(server Server, environ []string, probe AuthProbe) error {
	timeout := server.VerifyTimeout()
	if server.Stdio() {
		return ProbeStdio(server, timeout, environ, probe)
	}
	return ProbeHTTP(server, timeout, environ, probe)
}

const (
	// verifyAttempts covers failures that are environmental rather than real:
	// an upstream 503 (kiwi answered that way once) or a uv/npm install lock
	// held by the gateway's own child on the shared tool dir, which makes the
	// handshake exceed its window while nothing is actually wrong with the
	// server. A single retry absorbs both; a genuine break fails twice.
	verifyAttempts = 2
	retryDelay     = 5 * time.Second
)

// Execute is the preflight/drift runner: load the manifest, handshake every
// enabled server (optionally --only), certify the declared tools.include
// surface, and report in the same PASS/FAIL/DRIFT format the Python verifier
// produced so job alerts and dashboards keep their shape. Returns the process
// exit code.
//
// Drift mode additionally runs each server's declared auth probe. A handshake
// certifies the protocol, never the credential — see AuthProbe.
func Execute(manifestPath, mode string, only []string) int {
	servers, err := LoadManifest(manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot load manifest %s: %v\n", manifestPath, err)
		return 1
	}
	selected := Select(servers, only)
	preflight := mode == "preflight"
	if preflight {
		fmt.Printf("== MCP preflight: %d enabled server(s)\n", len(selected))
	}

	failures := 0
	for _, named := range selected {
		name, server := named.Name, named.Server

		// OAuth servers hold no credential this runner can replay. The grant is
		// issued to the agent (DCR + PKCE), refreshed by it, and stored under the
		// agent's token dir — handshaking from here can only 401, which says
		// nothing about the server. Certify the durable half instead: the grant
		// file the agent reconnects from.
		if strings.EqualFold(server.Auth, "oauth") {
			if err := CertifyOAuthGrant(name); err != nil {
				failures++
				fmt.Printf("FAIL %s: %v\n", name, err)
				continue
			}
			fmt.Printf("PASS %s oauth grant on disk\n", name)
			continue
		}

		// Unresolved placeholders first, and cheaply: the child env is built
		// from a baseline plus what the entry declares, and a name that resolves
		// nowhere reaches the server as the LITERAL `${VAR}`. The failure then
		// surfaces as whatever the server says about the literal string being a
		// bad credential (doppler: "Cached token appears invalid. Not
		// authenticated"), which reads as a dead token rather than a missing
		// key. The Jobs get the whole secret bundle via envFrom, so a name that
		// does not resolve here is a real wiring gap.
		if _, missing := ChildEnv(server, os.Environ()); len(missing) > 0 {
			failures++
			fmt.Printf("FAIL %s: unresolved env placeholder(s): %s\n", name, strings.Join(sortedKeys(missing), ", "))
			continue
		}

		actual, stderr, err := handshakeWithRetry(server, preflight, name)
		if err != nil {
			failures++
			printFailure(name, err)
			continue
		}
		declared := DeclaredTools(server)
		var missing []string
		live := map[string]bool{}
		for _, tool := range actual {
			live[tool] = true
		}
		for _, tool := range declared {
			if !live[tool] {
				missing = append(missing, tool)
			}
		}
		if len(missing) > 0 {
			failures++
			if preflight {
				fmt.Printf("FAIL %s\n", name)
				fmt.Printf("    declared tools missing from live surface: %s\n", strings.Join(missing, ", "))
			} else {
				fmt.Printf("DRIFT %s: declared tools missing from live surface: %s\n", name, strings.Join(missing, ", "))
			}
			if stderr != "" {
				for _, line := range strings.Split(stderr, "\n") {
					fmt.Printf("    %s\n", line)
				}
			}
			continue
		}

		// Auth probe: drift only. A dead third-party credential should alert on
		// the next cron tick, not fail an unrelated deploy's sync.
		if !preflight && server.AuthProbe != nil {
			if probeErr := ProbeServer(server, os.Environ(), *server.AuthProbe); probeErr != nil {
				failures++
				fmt.Printf("FAIL %s: auth probe %s\n", name, server.AuthProbe.Tool)
				fmt.Printf("    %v\n", probeErr)
				if ve, ok := probeErr.(*VerifyError); ok && ve.Stderr != "" {
					for _, line := range strings.Split(ve.Stderr, "\n") {
						fmt.Printf("    %s\n", line)
					}
				}
				continue
			}
		}

		if preflight {
			fmt.Printf("PASS %s %d tools\n", name, len(actual))
		}
	}

	if failures > 0 {
		noun := "failed"
		if !preflight {
			noun = "drifted"
		}
		fmt.Printf("%d server(s) %s\n", failures, noun)
		return 1
	}
	if preflight {
		fmt.Println("all enabled MCP servers passed")
	}
	return 0
}

// handshakeWithRetry runs the handshake, retrying a failed one once. The retry
// is announced only in preflight mode, where every line is meant to be legible.
func handshakeWithRetry(server Server, preflight bool, name string) ([]string, string, error) {
	var (
		tools  []string
		stderr string
		err    error
	)
	for attempt := 1; attempt <= verifyAttempts; attempt++ {
		tools, stderr, err = VerifyServer(server, os.Environ())
		if err == nil {
			return tools, stderr, nil
		}
		if attempt < verifyAttempts {
			if preflight {
				fmt.Printf("RETRY %s: %v\n", name, err)
			}
			time.Sleep(retryDelay)
		}
	}
	return tools, stderr, err
}

func printFailure(name string, err error) {
	fmt.Printf("FAIL %s: %v\n", name, err)
	if ve, ok := err.(*VerifyError); ok && ve.Stderr != "" {
		for _, line := range strings.Split(ve.Stderr, "\n") {
			fmt.Printf("    %s\n", line)
		}
	}
}

// oauthTokenDir is where the agent keeps its per-server OAuth grants.
// MCP_TOKEN_DIR wins so tests (and any relocation) do not need a real home dir.
func oauthTokenDir() string {
	if dir := os.Getenv("MCP_TOKEN_DIR"); dir != "" {
		return dir
	}
	home := os.Getenv("HERMES_HOME")
	if home == "" {
		home = "/opt/data"
	}
	return filepath.Join(home, "mcp-tokens")
}

// CertifyOAuthGrant asserts a usable grant is on disk for an OAuth server: the
// file exists, parses, and carries at least one token. Whether that token is
// still accepted upstream is the agent's question — it owns the refresh flow.
func CertifyOAuthGrant(name string) error {
	path := filepath.Join(oauthTokenDir(), name+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("no oauth grant at %s (authenticate once with `hermes mcp login %s`): %w", path, name, err)
	}
	var grant struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(raw, &grant); err != nil {
		return fmt.Errorf("oauth grant %s is not valid JSON: %w", path, err)
	}
	if grant.AccessToken == "" && grant.RefreshToken == "" {
		return fmt.Errorf("oauth grant %s holds no token (re-run `hermes mcp login %s`)", path, name)
	}
	return nil
}

// sortedKeys renders a placeholder-name set deterministically, so job output
// and tests do not depend on map iteration order.
func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
