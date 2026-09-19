package mcp

import (
	"fmt"
	"os"
	"regexp"
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

		// Rule 4 — every enabled server declares a resources/prompts policy.
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

		// Rule 5 — every enabled server declares the golden tool surface.
		if enabled {
			if server.Tools == nil || len(server.Tools.Include) == 0 {
				violations = append(violations, Violation{Server: name, Rule: "tools-include", Message: "enabled server has no non-empty tools.include"})
			}
		}
	}

	// Rule 6 — duplicates stay parked: present, and explicitly enabled: false.
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
