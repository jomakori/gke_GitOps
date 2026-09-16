package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func parseManifest(t *testing.T, raw string) map[string]*Server {
	t.Helper()
	var servers map[string]*Server
	if err := json.Unmarshal([]byte(raw), &servers); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return servers
}

func violationsString(vs []Violation) string {
	var b strings.Builder
	for _, v := range vs {
		b.WriteString("  ")
		b.WriteString(v.String())
		b.WriteString("\n")
	}
	return b.String()
}

// parkedPair is the two duplicate servers (github, gistpad) that must stay in
// the manifest as enabled: false. Fixtures that test other rules include them
// so the duplicate rule stays quiet.
const parkedPair = `,
				"github": {"command": "npx", "args": ["-y", "@modelcontextprotocol/server-github@2025.4.8"], "enabled": false, "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600},
				"gistpad": {"command": "npx", "args": ["-y", "gistpad-mcp@0.5.0"], "enabled": false, "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600}`

func TestValidate(t *testing.T) {
	cases := []struct {
		name     string
		manifest string
		want     []Violation
	}{
		{
			name: "clean manifest passes",
			manifest: `{
				"argocd": {"command": "npx", "args": ["-y", "argocd-mcp@0.9.0", "stdio"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600, "tools": {"include": ["list_applications"], "resources": false, "prompts": false}},
				"ferryhopper": {"url": "https://mcp.ferryhopper.com/mcp", "connect_timeout": 60, "tools": {"include": ["get_ports"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
		},
		{
			name: "@latest stdio package rejected",
			manifest: `{
				"argocd": {"command": "npx", "args": ["-y", "argocd-mcp@latest"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600, "tools": {"include": ["x"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
			want: []Violation{{Server: "argocd", Rule: "pin", Message: "unpinned package argument 'argocd-mcp@latest' (uses @latest)"}},
		},
		{
			name: "missing connect_timeout on enabled server rejected",
			manifest: `{
				"argocd": {"command": "npx", "args": ["-y", "argocd-mcp@0.9.0"], "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600, "tools": {"include": ["x"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
			want: []Violation{{Server: "argocd", Rule: "connect_timeout", Message: "missing or non-positive connect_timeout"}},
		},
		{
			name: "enabled stdio server missing lifecycle rejected",
			manifest: `{
				"argocd": {"command": "npx", "args": ["-y", "argocd-mcp@0.9.0"], "connect_timeout": 180, "tools": {"include": ["x"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
			want: []Violation{
				{Server: "argocd", Rule: "lifecycle", Message: "missing or non-positive idle_timeout_seconds"},
				{Server: "argocd", Rule: "lifecycle", Message: "missing or non-positive max_lifetime_seconds"},
			},
		},
		{
			name: "enabled server with empty tools.include rejected",
			manifest: `{
				"argocd": {"command": "npx", "args": ["-y", "argocd-mcp@0.9.0"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600, "tools": {"include": [], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
			want: []Violation{{Server: "argocd", Rule: "tools-include", Message: "enabled server has no non-empty tools.include"}},
		},
		{
			name: "enabled server missing tools.resources/prompts rejected",
			manifest: `{
				"argocd": {"command": "npx", "args": ["-y", "argocd-mcp@0.9.0"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600, "tools": {"include": ["x"]}}` + parkedPair + `
			}`,
			want: []Violation{
				{Server: "argocd", Rule: "tools-policy", Message: "enabled server has no tools.resources policy"},
				{Server: "argocd", Rule: "tools-policy", Message: "enabled server has no tools.prompts policy"},
			},
		},
		{
			name: "parked server skips tools checks",
			manifest: `{
				"gistpad": {"command": "npx", "args": ["-y", "gistpad-mcp@0.5.0"], "enabled": false, "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600},
				"github": {"command": "npx", "args": ["-y", "@modelcontextprotocol/server-github@2025.4.8"], "enabled": false, "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600}
			}`,
		},
		{
			name: "parked stdio server still needs lifecycle",
			manifest: `{
				"gistpad": {"command": "npx", "args": ["-y", "gistpad-mcp@0.5.0"], "enabled": false, "connect_timeout": 180},
				"github": {"command": "npx", "args": ["-y", "@modelcontextprotocol/server-github@2025.4.8"], "enabled": false, "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600}
			}`,
			want: []Violation{
				{Server: "gistpad", Rule: "lifecycle", Message: "missing or non-positive idle_timeout_seconds"},
				{Server: "gistpad", Rule: "lifecycle", Message: "missing or non-positive max_lifetime_seconds"},
			},
		},
		{
			name: "http server missing connect_timeout rejected",
			manifest: `{
				"ferryhopper": {"url": "https://mcp.ferryhopper.com/mcp", "tools": {"include": ["get_ports"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
			want: []Violation{{Server: "ferryhopper", Rule: "connect_timeout", Message: "missing or non-positive connect_timeout"}},
		},
		{
			name: "http server skips lifecycle rules",
			manifest: `{
				"ferryhopper": {"url": "https://mcp.ferryhopper.com/mcp", "connect_timeout": 60, "tools": {"include": ["get_ports"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
		},
		{
			name: "duplicate server missing from manifest",
			manifest: `{
				"argocd": {"command": "npx", "args": ["-y", "argocd-mcp@0.9.0"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600, "tools": {"include": ["x"], "resources": false, "prompts": false}}
			}`,
			want: []Violation{
				{Server: "github", Rule: "duplicate", Message: "duplicate server missing from manifest (must stay enabled: false)"},
				{Server: "gistpad", Rule: "duplicate", Message: "duplicate server missing from manifest (must stay enabled: false)"},
			},
		},
		{
			name: "duplicate server must stay parked",
			manifest: `{
				"github": {"command": "npx", "args": ["-y", "@modelcontextprotocol/server-github@2025.4.8"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600, "tools": {"include": ["x"], "resources": false, "prompts": false}},
				"gistpad": {"command": "npx", "args": ["-y", "gistpad-mcp@0.5.0"], "enabled": false, "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600}
			}`,
			want: []Violation{{Server: "github", Rule: "duplicate", Message: "duplicate server is not explicitly enabled: false"}},
		},
		{
			name: "npx command with no package argument",
			manifest: `{
				"argocd": {"command": "npx", "args": ["-y"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600, "tools": {"include": ["x"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
			want: []Violation{{Server: "argocd", Rule: "pin", Message: "no package argument found for npx/uvx command"}},
		},
		{
			name: "unpinned package without version",
			manifest: `{
				"argocd": {"command": "npx", "args": ["-y", "argocd-mcp"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600, "tools": {"include": ["x"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
			want: []Violation{{Server: "argocd", Rule: "pin", Message: "unpinned package argument 'argocd-mcp' (no exact version)"}},
		},
		{
			name: "uvx empty version after ==",
			manifest: `{
				"plane": {"command": "uvx", "args": ["plane-mcp-server=="], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600, "tools": {"include": ["x"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
			want: []Violation{{Server: "plane", Rule: "pin", Message: "unpinned package argument 'plane-mcp-server==' (empty version after '==')"}},
		},
		{
			name: "sh-wrapped pinned package passes",
			manifest: `{
				"grafana": {"command": "sh", "args": ["-c", "exec npx -y @leval/mcp-grafana@1.1.7 | grep --line-buffered jsonrpc"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600, "tools": {"include": ["search_dashboards"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
		},
		{
			name: "null server entry flagged as shape",
			manifest: `{
				"broken": null` + parkedPair + `
			}`,
			want: []Violation{{Server: "broken", Rule: "shape", Message: "server entry is not a mapping"}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Validate(parseManifest(t, c.manifest))
			if len(got) != len(c.want) {
				t.Fatalf("Validate() = %d violation(s), want %d:\n%s", len(got), len(c.want), violationsString(got))
			}
			for i := range c.want {
				if got[i] != c.want[i] {
					t.Errorf("violation[%d] = %+v, want %+v", i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestExecuteValidateCleanAndViolations(t *testing.T) {
	clean := writeManifest(t, parseManifest(t, `{
		"argocd": {"command": "npx", "args": ["-y", "argocd-mcp@0.9.0"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600, "tools": {"include": ["x"], "resources": false, "prompts": false}}`+parkedPair+`
	}`))
	rc := 1
	out := captureStdout(t, func() { rc = ExecuteValidate(clean) })
	if rc != 0 {
		t.Fatalf("clean rc = %d, want 0\n%s", rc, out)
	}
	if !strings.Contains(out, "OK — manifest is pinned, bounded and fully declared") {
		t.Fatalf("clean run must print OK line:\n%s", out)
	}

	bad := writeManifest(t, parseManifest(t, `{
		"argocd": {"command": "npx", "args": ["-y", "argocd-mcp@latest"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600, "tools": {"include": ["x"], "resources": false, "prompts": false}}`+parkedPair+`
	}`))
	out = captureStdout(t, func() { rc = ExecuteValidate(bad) })
	if rc != 1 {
		t.Fatalf("bad rc = %d, want 1\n%s", rc, out)
	}
	if !strings.Contains(out, "FAIL argocd") || !strings.Contains(out, "[pin]") || !strings.Contains(out, "uses @latest") {
		t.Fatalf("bad run must print FAIL line with pin rule:\n%s", out)
	}
}
