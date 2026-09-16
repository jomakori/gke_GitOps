package mcp

import (
	"fmt"
	"os"
	"strings"
)

func VerifyServer(server Server, environ []string) ([]string, string, error) {
	timeout := server.VerifyTimeout()
	if server.Stdio() {
		return HandshakeStdio(server, timeout, environ)
	}
	return HandshakeHTTP(server, timeout, environ)
}

// Execute is the preflight/drift runner: load the manifest, handshake every
// enabled server (optionally --only), certify the declared tools.include
// surface, and report in the same PASS/FAIL/DRIFT format the Python verifier
// produced so job alerts and dashboards keep their shape. Returns the process
// exit code.
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
		actual, stderr, err := VerifyServer(server, os.Environ())
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
		} else if preflight {
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

func printFailure(name string, err error) {
	fmt.Printf("FAIL %s: %v\n", name, err)
	if ve, ok := err.(*VerifyError); ok && ve.Stderr != "" {
		for _, line := range strings.Split(ve.Stderr, "\n") {
			fmt.Printf("    %s\n", line)
		}
	}
}
