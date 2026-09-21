// Package mcp implements the openagent MCP manifest model, the JSON-RPC
// handshake verifier (stdio + Streamable HTTP) and the boot-time cache
// prewarmer. Everything the gateway's MCP client does is mirrored here so a
// preflight PASS means a runtime PASS.
//
// The behavioural details (baseline env stripping, unresolved-placeholder
// diagnostics, process-group kills, stderr recovery) are deliberate ports of
// the Python implementations they replace (now the mcp-verify ConfigMap in
// templates/hooks.yaml) — keep them in sync when the client changes.
package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// probeRPCID is the JSON-RPC id the auth probe uses. Distinct from the
// handshake's ids so a stale frame from an earlier request can never be read
// as the probe's answer.
const probeRPCID = 4242

const ProtocolVersion = "2024-11-05"

// DefaultManifest is where the preflight/drift jobs mount the rendered
// manifest (see templates/hooks.yaml → openagent-mcp-manifest ConfigMap).
const DefaultManifest = "/mcp-verify/mcp-manifest"

// BootManifest is where the gateway pod mounts the same ConfigMap during boot
// (see values.yaml → hermes-agent.extraVolumeMounts /opt/data/mcp-verify).
const BootManifest = "/opt/data/mcp-verify/mcp-manifest"

const StderrLines = 40

// ToolsConfig carries the declared tool surface. Prompts/Resources are
// pointers so the static validator can distinguish "policy absent" from
// "policy explicitly false" (the Python lint checks key presence, not value).
type ToolsConfig struct {
	Include   []string `json:"include,omitempty"`
	Prompts   *bool    `json:"prompts,omitempty"`
	Resources *bool    `json:"resources,omitempty"`
}

// AuthProbe is an optional read-only tool call the drift verifier makes after
// a successful handshake. A handshake proves the child speaks JSON-RPC and
// lists its tools; it says nothing about the credential the server will use on
// the first real call, because most servers authenticate lazily per tool call.
// A revoked token or a partial OAuth grant therefore passes every handshake
// while every tool fails (hit live: google-workspace's stored grant covered
// Drive only, so tools/list reported 122 tools and 33 of the 35 declared ones
// answered "ACTION REQUIRED: Google Authentication Needed").
//
// Probes run in DRIFT mode only. A dead third-party credential must alert
// within one cron interval, not block an unrelated deploy: a failing preflight
// consumes the app's retry budget and strands the sync.
type AuthProbe struct {
	Tool string         `json:"tool"`
	Args map[string]any `json:"args,omitempty"`
}

type Server struct {
	Command            string            `json:"command,omitempty"`
	Args               []string          `json:"args,omitempty"`
	URL                string            `json:"url,omitempty"`
	Env                map[string]string `json:"env,omitempty"`
	Headers            map[string]string `json:"headers,omitempty"`
	Enabled            *bool             `json:"enabled,omitempty"`
	ConnectTimeout     int               `json:"connect_timeout,omitempty"`
	IdleTimeoutSeconds int               `json:"idle_timeout_seconds,omitempty"`
	MaxLifetimeSeconds int               `json:"max_lifetime_seconds,omitempty"`
	Lazy               bool              `json:"lazy,omitempty"`
	Tools              *ToolsConfig      `json:"tools,omitempty"`
	AuthProbe          *AuthProbe        `json:"auth_probe,omitempty"`
}

func (s Server) IsEnabled() bool { return s.Enabled == nil || *s.Enabled }

func (s Server) Stdio() bool { return s.Command != "" && s.URL == "" }

func (s Server) VerifyTimeout() int {
	base := s.ConnectTimeout
	if base <= 0 {
		base = 60
	}
	return base * 2
}

type NamedServer struct {
	Name   string
	Server Server
}

func Select(servers map[string]*Server, only []string) []NamedServer {
	out := make([]NamedServer, 0, len(servers))
	for name, sv := range servers {
		if sv == nil || !sv.IsEnabled() {
			continue
		}
		if len(only) > 0 && !slices.Contains(only, name) {
			continue
		}
		out = append(out, NamedServer{Name: name, Server: *sv})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func DeclaredTools(s Server) []string {
	if s.Tools == nil {
		return nil
	}
	return s.Tools.Include
}

var placeholders = regexp.MustCompile(`\$\{(\w+)\}|\$(\w+)`)

// Expand resolves ${VAR} / $VAR placeholders from env. Unresolved names are
// recorded in missing and the literal placeholder is kept — mirroring
// verify.py's expand() so a placeholder that resolves nowhere is handed to the
// server unexpanded and diagnosed (doppler failed exactly this way and the
// only trace was the server's own "Not authenticated" message).
func Expand(value any, env map[string]string, missing map[string]bool) string {
	return placeholders.ReplaceAllStringFunc(fmt.Sprint(value), func(match string) string {
		groups := placeholders.FindStringSubmatch(match)
		name := groups[1]
		if name == "" {
			name = groups[2]
		}
		if v, ok := env[name]; ok {
			return v
		}
		missing[name] = true
		return match
	})
}

// baselineKeys mirrors the native MCP client's safe child environment: it
// passes a baseline (plus XDG_*) and forwards everything else only when a
// server declares it under `env`. Passing the full os.environ made the old
// verifier report false greens — servers certified here that production could
// not start, because the client strips far more than this process does.
var baselineKeys = []string{
	"PATH", "HOME", "USER", "LANG", "LC_ALL", "TERM", "SHELL", "TMPDIR",
}

func BaselineEnv(environ []string) map[string]string {
	env := map[string]string{}
	for _, kv := range environ {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if slices.Contains(baselineKeys, k) || strings.HasPrefix(k, "XDG_") {
			env[k] = v
		}
	}
	if env["PATH"] == "" {
		env["PATH"] = "/usr/local/bin:/usr/bin:/bin"
	}
	if env["HOME"] == "" {
		env["HOME"] = "/"
	}
	return env
}

// ChildEnv computes the stdio child environment: the baseline plus the
// server's declared env, with placeholders resolved from the process
// environment (the secret bundle the Job gets via envFrom) falling back to
// values declared here — exactly what the HTTP transport and the client's own
// secret scope do.
func ChildEnv(server Server, environ []string) (map[string]string, map[string]bool) {
	missing := map[string]bool{}
	child := BaselineEnv(environ)
	lookup := map[string]string{}
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok {
			lookup[k] = v
		}
	}
	for k, v := range child {
		lookup[k] = v
	}
	for k, v := range server.Env {
		expanded := Expand(v, lookup, missing)
		child[k] = expanded
		lookup[k] = expanded
	}
	return child, missing
}

func environSlice(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

func LoadManifest(path string) (map[string]*Server, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var servers map[string]*Server
	if err := json.Unmarshal(data, &servers); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return servers, nil
}

func NPMCacheFor(server Server) string {
	if c := server.Env["npm_config_cache"]; c != "" {
		return c
	}
	if c := os.Getenv("NPM_CONFIG_CACHE"); c != "" {
		return c
	}
	return filepath.Join(os.Getenv("HOME"), ".npm")
}
