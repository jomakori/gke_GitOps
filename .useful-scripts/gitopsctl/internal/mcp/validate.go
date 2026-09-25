package mcp

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Violation is one static-policy finding against a manifest server. The
// Server/Rule/Message shape and the FAIL line format mirror the Python lint
// (scripts/validate-mcp-manifest.py) so CI output stays greppable.
type Violation struct {
	Server  string
	Rule    string
	Message string
}

func (v Violation) String() string {
	return fmt.Sprintf("FAIL %-16s [%s] %s", v.Server, v.Rule, v.Message)
}

// duplicateServers must stay in the manifest as enabled: false — deleting one
// silently loses its pinned package, env mapping and tool surface.
var duplicateServers = []string{"github", "gistpad"}

var (
	scopedPinRe = regexp.MustCompile(`^@[^/]+/[^@]+@(.+)$`)
	plainPinRe  = regexp.MustCompile(`^[^@]+@(.+)$`)
)

// pinViolation returns a reason string when spec is not pinned to an exact
// version, mirroring the Python lint's pin_violation().
func pinViolation(spec string) string {
	if strings.Contains(spec, "@latest") {
		return fmt.Sprintf("unpinned package argument '%s' (uses @latest)", spec)
	}
	if strings.Contains(spec, "==") {
		// uv/pip PEP 440 pin: plane-mcp-server==0.3.2
		name, version, _ := strings.Cut(spec, "==")
		if name != "" && version != "" && !strings.HasPrefix(version, "*") {
			return ""
		}
		return fmt.Sprintf("unpinned package argument '%s' (empty version after '==')", spec)
	}
	var match []string
	if strings.HasPrefix(spec, "@") {
		match = scopedPinRe.FindStringSubmatch(spec)
	} else {
		match = plainPinRe.FindStringSubmatch(spec)
	}
	version := ""
	if len(match) > 1 {
		version = match[1]
	}
	if version != "" && version != "latest" {
		return ""
	}
	return fmt.Sprintf("unpinned package argument '%s' (no exact version)", spec)
}

// positiveInt mirrors the Python lint's _positive_int() for the typed int
// fields of Server (the Go manifest model already constrains these to ints).
func positiveInt(v int) bool { return v > 0 }

// Validate applies the static manifest policy ported 1:1 from
// scripts/validate-mcp-manifest.py. It performs no network or JSON-RPC work.
func Validate(servers map[string]*Server) []Violation {
	var violations []Violation

	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		server := servers[name]
		if server == nil {
			violations = append(violations, Violation{Server: name, Rule: "shape", Message: "server entry is not a mapping"})
			continue
		}

		enabled := server.IsEnabled()
		stdio := server.Stdio()

		// Rule 1 — stdio package arguments must be pinned to an exact version.
		if stdio {
			kind, spec := PackageSpec(*server)
			if (server.Command == "npx" || server.Command == "uvx") && kind == "" {
				violations = append(violations, Violation{Server: name, Rule: "pin", Message: "no package argument found for npx/uvx command"})
			}
			if kind != "" {
				if reason := pinViolation(spec); reason != "" {
					violations = append(violations, Violation{Server: name, Rule: "pin", Message: reason})
				}
			}
		}

		// Rule 2 — every server has a connect timeout.
		if !positiveInt(server.ConnectTimeout) {
			violations = append(violations, Violation{Server: name, Rule: "connect_timeout", Message: "missing or non-positive connect_timeout"})
		}

		// Rule 3 — every stdio server has a lifecycle.
		if stdio {
			for _, key := range []string{"idle_timeout_seconds", "max_lifetime_seconds"} {
				val := server.IdleTimeoutSeconds
				if key == "max_lifetime_seconds" {
					val = server.MaxLifetimeSeconds
				}
				if !positiveInt(val) {
					violations = append(violations, Violation{Server: name, Rule: "lifecycle", Message: fmt.Sprintf("missing or non-positive %s", key)})
				}
			}
		}

		// Rule 4 — every server registers lazily: its tools come from the
		// on-disk schema cache and the process starts on first use, so no
		// handshake sits in the gateway's startup path.
		if !server.Lazy {
			violations = append(violations, Violation{Server: name, Rule: "lazy", Message: "missing lazy: true (server connects eagerly at startup)"})
		}

		// Rule 5 — every enabled server declares a resources/prompts policy.
		if enabled {
			for _, key := range []string{"resources", "prompts"} {
				declared := false
				if server.Tools != nil {
					if key == "resources" {
						declared = server.Tools.Resources != nil
					} else {
						declared = server.Tools.Prompts != nil
					}
				}
				if !declared {
					violations = append(violations, Violation{Server: name, Rule: "tools-policy", Message: fmt.Sprintf("enabled server has no tools.%s policy", key)})
				}
			}
		}

		// Rule 6 — every enabled server declares the golden tool surface.
		if enabled {
			if server.Tools == nil || len(server.Tools.Include) == 0 {
				violations = append(violations, Violation{Server: name, Rule: "tools-include", Message: "enabled server has no non-empty tools.include"})
			}
		}

		// Rule 7 — a package-launched server declares its OWN package cache.
		// The MCP client spawns children with a filtered env: PATH/HOME/... plus
		// only what the entry declares. npm's cache is therefore HOME-derived, and
		// the gateway's baseline HOME (/opt/data) is the PVC dir every process —
		// gateway, coding agents, root-running jobs — installs into, so concurrent
		// installs corrupt each other and one interrupted reify leaves a
		// node_modules every later start SKIPS ("Cannot read package.json ... ENOENT").
		// A per-server cache is the fix; the path is pinned literally so a
		// copy-pasted entry cannot silently share another server's cache.
		if stdio {
			kind, _ := PackageSpec(*server)
			switch kind {
			case "npm":
				want := "/opt/data/.npm-mcp/" + name
				if got := server.Env["npm_config_cache"]; got != want {
					violations = append(violations, Violation{Server: name, Rule: "npm-cache", Message: fmt.Sprintf("npm_config_cache is %q, want the private per-server cache %q", got, want)})
				}
			case "uv":
				// Rule 8 — a uvx server pins its whole toolchain layout. uv resolves
				// the tool dir from HOME when UV_TOOL_DIR is absent, so the verifier
				// Jobs (HOME=/opt/data/home) and the gateway (HOME=/opt/data) read
				// DIFFERENT directories: the Jobs certify a warm install the gateway
				// cannot see and re-resolve fails inside the connect window. Declaring
				// them makes both contexts identical by construction.
				for _, key := range []string{"HOME", "PATH", "UV_CACHE_DIR", "UV_TOOL_DIR", "UV_TOOL_BIN_DIR"} {
					if server.Env[key] == "" {
						violations = append(violations, Violation{Server: name, Rule: "uv-env", Message: fmt.Sprintf("uvx server does not declare %s", key)})
					}
				}
			}
		}

		// Rule 11 — a shell-launched server pins its toolchain layout.
		// `command: sh` (or bash) means the entry resolves binary names from
		// PATH at runtime, and the MCP child gets a FILTERED env: mcp_tool.py's
		// _SAFE_ENV_KEYS carries PATH/HOME/USER/LANG/... plus only what the entry
		// declares — never MISE_*. The first PATH entries on this host are mise
		// shims, and a shim resolves its tool through MISE_DATA_DIR; without it
		// the child dies with `mise ERROR <bin> is not a valid shim`, while
		// `mise ls`, the shim file itself and an interactive shell all look
		// perfectly healthy. bitwarden failed exactly this way. npx/uvx are NOT
		// affected (they resolve to system binaries in /usr/local/bin), which is
		// why this rule keys on the shell rather than on every entry — and why a
		// new shell entry must declare the layout rather than inherit it.
		if stdio && (server.Command == "sh" || server.Command == "bash") {
			for _, key := range []string{"HOME", "PATH", "MISE_CONFIG_FILE", "MISE_DATA_DIR", "MISE_CACHE_DIR"} {
				if server.Env[key] == "" {
					violations = append(violations, Violation{Server: name, Rule: "shell-env", Message: fmt.Sprintf("shell-launched server does not declare %s", key)})
				}
			}
		}

		// Rule 9 — an auth probe must name a tool the server actually declares.
		// A typo here fails silently in the worst way: tools/list still matches,
		// the probe never runs, and the gate reports green.
		if server.AuthProbe != nil {
			probe := server.AuthProbe
			switch {
			case !enabled:
				violations = append(violations, Violation{Server: name, Rule: "auth-probe", Message: "auth_probe declared on a parked server (it can never run)"})
			case probe.Tool == "":
				violations = append(violations, Violation{Server: name, Rule: "auth-probe", Message: "auth_probe has no tool"})
			case !slices.Contains(DeclaredTools(*server), probe.Tool):
				violations = append(violations, Violation{Server: name, Rule: "auth-probe", Message: fmt.Sprintf("auth_probe tool %q is not in tools.include", probe.Tool)})
			}
		}
	}

	// Rule 10 — duplicates stay parked: present, and explicitly enabled: false.
	for _, name := range duplicateServers {
		server := servers[name]
		if server == nil {
			violations = append(violations, Violation{Server: name, Rule: "duplicate", Message: "duplicate server missing from manifest (must stay enabled: false)"})
		} else if server.Enabled == nil || *server.Enabled {
			violations = append(violations, Violation{Server: name, Rule: "duplicate", Message: "duplicate server is not explicitly enabled: false"})
		}
	}

	return violations
}

// ExecuteValidate is the static-policy runner for `mcp verify --mode
// validate`: load the manifest, apply the ported policy, print one FAIL line
// per violation, exit 0 when clean and 1 when any violation. No network or
// JSON-RPC work happens here.
func ExecuteValidate(manifestPath string) int {
	servers, err := LoadManifest(manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot load manifest %s: %v\n", manifestPath, err)
		return 1
	}
	enabled, parked := 0, 0
	for _, server := range servers {
		if server == nil {
			continue
		}
		if server.IsEnabled() {
			enabled++
		}
		if server.Enabled != nil && !*server.Enabled {
			parked++
		}
	}
	fmt.Printf("== MCP manifest validate: %d server(s) (enabled: %d, parked: %d)\n", len(servers), enabled, parked)

	violations := Validate(servers)
	for _, violation := range violations {
		fmt.Printf("  %s\n", violation)
	}
	if len(violations) > 0 {
		fmt.Printf("\n%d violation(s) — MCP manifest is NOT production-safe\n", len(violations))
		return 1
	}
	fmt.Println("OK — manifest is pinned, bounded and fully declared")
	return 0
}
