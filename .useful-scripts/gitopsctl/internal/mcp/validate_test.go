package mcp

import (
	"encoding/json"
	"fmt"
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

// npxCache renders the private per-server npm cache every npx fixture must
// declare, so a case that tests a DIFFERENT rule does not also trip the
// npm-cache rule.
func npxCache(name string) string {
	return fmt.Sprintf(`, "env": {"npm_config_cache": "/opt/data/.npm-mcp/%s"}`, name)
}

// uvEnv is the toolchain layout a uvx fixture must declare (rule 8).
const uvEnv = `, "env": {"HOME": "/opt/data/home", "PATH": "/opt/data/bin", "UV_CACHE_DIR": "/opt/data/home/.cache/uv", "UV_TOOL_DIR": "/opt/data/home/.local/share/uv/tools", "UV_TOOL_BIN_DIR": "/opt/data/home/.local/bin"}`

// shEnv is the toolchain layout plus private npm cache a shell-launched
// fixture must declare (rules 7 and 11). npxCache alone is not enough: a
// `sh -c` entry also resolves binary names from PATH, and the MCP child's
// filtered env carries no MISE_*, so a mise shim on that PATH cannot resolve.
func shEnv(name string) string {
	return fmt.Sprintf(`, "env": {"npm_config_cache": "/opt/data/.npm-mcp/%s", "HOME": "/opt/data/home", "PATH": "/opt/data/bin:/opt/data/mise/shims", "MISE_CONFIG_FILE": "/mise/mise.toml", "MISE_DATA_DIR": "/opt/data/mise", "MISE_CACHE_DIR": "/opt/data/mise-cache"}`, name)
}

// parkedPair is the two duplicate servers (github, gistpad) that must stay in
// the manifest as enabled: false. Fixtures that test other rules include them
// so the duplicate rule stays quiet.
const parkedPair = `,
			"github": {"lazy": true, "command": "npx", "args": ["-y", "@modelcontextprotocol/server-github@2025.4.8"], "enabled": false, "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600, "env": {"npm_config_cache": "/opt/data/.npm-mcp/github"}},
			"gistpad": {"lazy": true, "command": "npx", "args": ["-y", "gistpad-mcp@0.5.0"], "enabled": false, "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600, "env": {"npm_config_cache": "/opt/data/.npm-mcp/gistpad"}}`

func TestValidate(t *testing.T) {
	cases := []struct {
		name     string
		manifest string
		want     []Violation
	}{
		{
			name: "clean manifest passes",
			manifest: `{
				"argocd": {"lazy": true, "command": "npx", "args": ["-y", "argocd-mcp@0.9.0", "stdio"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600` + npxCache("argocd") + `, "tools": {"include": ["list_applications"], "resources": false, "prompts": false}},
				"ferryhopper": {"lazy": true, "url": "https://mcp.ferryhopper.com/mcp", "connect_timeout": 60, "tools": {"include": ["get_ports"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
		},
		{
			name: "@latest stdio package rejected",
			manifest: `{
				"argocd": {"lazy": true, "command": "npx", "args": ["-y", "argocd-mcp@latest"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600` + npxCache("argocd") + `, "tools": {"include": ["x"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
			want: []Violation{{Server: "argocd", Rule: "pin", Message: "unpinned package argument 'argocd-mcp@latest' (uses @latest)"}},
		},
		{
			name: "eager server rejected",
			manifest: `{
				"argocd": {"command": "npx", "args": ["-y", "argocd-mcp@0.9.0"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600` + npxCache("argocd") + `, "tools": {"include": ["x"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
			want: []Violation{{Server: "argocd", Rule: "lazy", Message: "missing lazy: true (server connects eagerly at startup)"}},
		},
		{
			name: "missing connect_timeout on enabled server rejected",
			manifest: `{
				"argocd": {"lazy": true, "command": "npx", "args": ["-y", "argocd-mcp@0.9.0"], "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600` + npxCache("argocd") + `, "tools": {"include": ["x"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
			want: []Violation{{Server: "argocd", Rule: "connect_timeout", Message: "missing or non-positive connect_timeout"}},
		},
		{
			name: "enabled stdio server missing lifecycle rejected",
			manifest: `{
				"argocd": {"lazy": true, "command": "npx", "args": ["-y", "argocd-mcp@0.9.0"], "connect_timeout": 180` + npxCache("argocd") + `, "tools": {"include": ["x"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
			want: []Violation{
				{Server: "argocd", Rule: "lifecycle", Message: "missing or non-positive idle_timeout_seconds"},
				{Server: "argocd", Rule: "lifecycle", Message: "missing or non-positive max_lifetime_seconds"},
			},
		},
		{
			name: "enabled server with empty tools.include rejected",
			manifest: `{
				"argocd": {"lazy": true, "command": "npx", "args": ["-y", "argocd-mcp@0.9.0"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600` + npxCache("argocd") + `, "tools": {"include": [], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
			want: []Violation{{Server: "argocd", Rule: "tools-include", Message: "enabled server has no non-empty tools.include"}},
		},
		{
			name: "enabled server missing tools.resources/prompts rejected",
			manifest: `{
				"argocd": {"lazy": true, "command": "npx", "args": ["-y", "argocd-mcp@0.9.0"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600` + npxCache("argocd") + `, "tools": {"include": ["x"]}}` + parkedPair + `
			}`,
			want: []Violation{
				{Server: "argocd", Rule: "tools-policy", Message: "enabled server has no tools.resources policy"},
				{Server: "argocd", Rule: "tools-policy", Message: "enabled server has no tools.prompts policy"},
			},
		},
		{
			name: "parked server skips tools checks",
			manifest: `{
				"gistpad": {"lazy": true, "command": "npx", "args": ["-y", "gistpad-mcp@0.5.0"], "enabled": false, "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600` + npxCache("gistpad") + `},
				"github": {"lazy": true, "command": "npx", "args": ["-y", "@modelcontextprotocol/server-github@2025.4.8"], "enabled": false, "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600` + npxCache("github") + `}
			}`,
		},
		{
			name: "parked stdio server still needs lifecycle",
			manifest: `{
				"gistpad": {"lazy": true, "command": "npx", "args": ["-y", "gistpad-mcp@0.5.0"], "enabled": false, "connect_timeout": 180` + npxCache("gistpad") + `},
				"github": {"lazy": true, "command": "npx", "args": ["-y", "@modelcontextprotocol/server-github@2025.4.8"], "enabled": false, "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600` + npxCache("github") + `}
			}`,
			want: []Violation{
				{Server: "gistpad", Rule: "lifecycle", Message: "missing or non-positive idle_timeout_seconds"},
				{Server: "gistpad", Rule: "lifecycle", Message: "missing or non-positive max_lifetime_seconds"},
			},
		},
		{
			name: "http server missing connect_timeout rejected",
			manifest: `{
				"ferryhopper": {"lazy": true, "url": "https://mcp.ferryhopper.com/mcp", "tools": {"include": ["get_ports"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
			want: []Violation{{Server: "ferryhopper", Rule: "connect_timeout", Message: "missing or non-positive connect_timeout"}},
		},
		{
			name: "http server skips lifecycle rules",
			manifest: `{
				"ferryhopper": {"lazy": true, "url": "https://mcp.ferryhopper.com/mcp", "connect_timeout": 60, "tools": {"include": ["get_ports"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
		},
		{
			name: "duplicate server missing from manifest",
			manifest: `{
				"argocd": {"lazy": true, "command": "npx", "args": ["-y", "argocd-mcp@0.9.0"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600` + npxCache("argocd") + `, "tools": {"include": ["x"], "resources": false, "prompts": false}}
			}`,
			want: []Violation{
				{Server: "github", Rule: "duplicate", Message: "duplicate server missing from manifest (must stay enabled: false)"},
				{Server: "gistpad", Rule: "duplicate", Message: "duplicate server missing from manifest (must stay enabled: false)"},
			},
		},
		{
			name: "duplicate server must stay parked",
			manifest: `{
				"github": {"lazy": true, "command": "npx", "args": ["-y", "@modelcontextprotocol/server-github@2025.4.8"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600` + npxCache("github") + `, "tools": {"include": ["x"], "resources": false, "prompts": false}},
				"gistpad": {"lazy": true, "command": "npx", "args": ["-y", "gistpad-mcp@0.5.0"], "enabled": false, "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600` + npxCache("gistpad") + `}
			}`,
			want: []Violation{{Server: "github", Rule: "duplicate", Message: "duplicate server is not explicitly enabled: false"}},
		},
		{
			name: "npx command with no package argument",
			manifest: `{
				"argocd": {"lazy": true, "command": "npx", "args": ["-y"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600` + npxCache("argocd") + `, "tools": {"include": ["x"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
			want: []Violation{{Server: "argocd", Rule: "pin", Message: "no package argument found for npx/uvx command"}},
		},
		{
			name: "unpinned package without version",
			manifest: `{
				"argocd": {"lazy": true, "command": "npx", "args": ["-y", "argocd-mcp"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600` + npxCache("argocd") + `, "tools": {"include": ["x"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
			want: []Violation{{Server: "argocd", Rule: "pin", Message: "unpinned package argument 'argocd-mcp' (no exact version)"}},
		},
		{
			name: "uvx empty version after ==",
			manifest: `{
				"plane": {"lazy": true, "command": "uvx", "args": ["plane-mcp-server=="], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600` + uvEnv + `, "tools": {"include": ["x"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
			want: []Violation{{Server: "plane", Rule: "pin", Message: "unpinned package argument 'plane-mcp-server==' (empty version after '==')"}},
		},
		{
			name: "sh-wrapped pinned package passes",
			manifest: `{
				"grafana": {"lazy": true, "command": "sh", "args": ["-c", "exec npx -y @leval/mcp-grafana@1.1.7 | grep --line-buffered jsonrpc"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600` + shEnv("grafana") + `, "tools": {"include": ["search_dashboards"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
		},
		{
			name: "null server entry flagged as shape",
			manifest: `{
				"broken": null` + parkedPair + `
			}`,
			want: []Violation{{Server: "broken", Rule: "shape", Message: "server entry is not a mapping"}},
		},
		{
			// The live regression this rule exists for: bitwarden ran for weeks
			// without its own cache, so its npx install landed in the shared PVC
			// dir, was corrupted by a concurrent install, and the gateway could
			// never start it again — while every gate stayed green.
			name: "npx server without its own cache rejected",
			manifest: `{
				"bitwarden": {"lazy": true, "command": "sh", "args": ["-c", "exec npx -y @bitwarden/mcp-server@2026.7.0"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600, "env": {"BW_PASSWORD": "${BW_PASSWORD}", "HOME": "/opt/data/home", "PATH": "/opt/data/bin:/opt/data/mise/shims", "MISE_CONFIG_FILE": "/mise/mise.toml", "MISE_DATA_DIR": "/opt/data/mise", "MISE_CACHE_DIR": "/opt/data/mise-cache"}, "tools": {"include": ["status"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
			want: []Violation{{Server: "bitwarden", Rule: "npm-cache", Message: `npm_config_cache is "", want the private per-server cache "/opt/data/.npm-mcp/bitwarden"`}},
		},
		{
			name: "npx server sharing another server's cache rejected",
			manifest: `{
				"bitwarden": {"lazy": true, "command": "sh", "args": ["-c", "exec npx -y @bitwarden/mcp-server@2026.7.0"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600, "env": {"npm_config_cache": "/opt/data/.npm-mcp/grafana", "HOME": "/opt/data/home", "PATH": "/opt/data/bin:/opt/data/mise/shims", "MISE_CONFIG_FILE": "/mise/mise.toml", "MISE_DATA_DIR": "/opt/data/mise", "MISE_CACHE_DIR": "/opt/data/mise-cache"}, "tools": {"include": ["status"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
			want: []Violation{{Server: "bitwarden", Rule: "npm-cache", Message: `npm_config_cache is "/opt/data/.npm-mcp/grafana", want the private per-server cache "/opt/data/.npm-mcp/bitwarden"`}},
		},
		{
			// The #196/#197 class: uvx resolves its tool dir from HOME, and the
			// gateway's HOME (/opt/data) differs from the verifier Jobs'
			// (/opt/data/home), so an undeclared server is certified against a
			// directory the gateway never reads.
			name: "uvx server without a declared toolchain layout rejected",
			manifest: `{
				"plane": {"lazy": true, "command": "uvx", "args": ["plane-mcp-server==0.3.2", "stdio"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600, "env": {"PLANE_API_KEY": "${PLANE_API_KEY}"}, "tools": {"include": ["project"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
			want: []Violation{
				{Server: "plane", Rule: "uv-env", Message: "uvx server does not declare HOME"},
				{Server: "plane", Rule: "uv-env", Message: "uvx server does not declare PATH"},
				{Server: "plane", Rule: "uv-env", Message: "uvx server does not declare UV_CACHE_DIR"},
				{Server: "plane", Rule: "uv-env", Message: "uvx server does not declare UV_TOOL_DIR"},
				{Server: "plane", Rule: "uv-env", Message: "uvx server does not declare UV_TOOL_BIN_DIR"},
			},
		},
		{
			// The bitwarden class: `command: sh` resolves binary names from
			// PATH, the MCP child's env is filtered (no MISE_*), and the mise
			// shim on PATH then cannot resolve its tool. Every cheap check —
			// mise ls, the shim's existence, an interactive shell — looks fine.
			name: "shell-launched server without a declared toolchain layout rejected",
			manifest: `{
				"bitwarden": {"lazy": true, "command": "sh", "args": ["-c", "bw login --apikey; exec npx -y @bitwarden/mcp-server@2026.7.0"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600, "env": {"BW_CLIENTID": "${BW_CLIENTID}"}, "tools": {"include": ["status"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
			want: []Violation{
				{Server: "bitwarden", Rule: "npm-cache", Message: `npm_config_cache is "", want the private per-server cache "/opt/data/.npm-mcp/bitwarden"`},
				{Server: "bitwarden", Rule: "shell-env", Message: "shell-launched server does not declare HOME"},
				{Server: "bitwarden", Rule: "shell-env", Message: "shell-launched server does not declare PATH"},
				{Server: "bitwarden", Rule: "shell-env", Message: "shell-launched server does not declare MISE_CONFIG_FILE"},
				{Server: "bitwarden", Rule: "shell-env", Message: "shell-launched server does not declare MISE_DATA_DIR"},
				{Server: "bitwarden", Rule: "shell-env", Message: "shell-launched server does not declare MISE_CACHE_DIR"},
			},
		},
		{
			name: "shell-launched server with the toolchain layout passes",
			manifest: `{
				"bitwarden": {"lazy": true, "command": "sh", "args": ["-c", "bw login --apikey; exec npx -y @bitwarden/mcp-server@2026.7.0"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600, "auth_probe": {"tool": "status"}, "tools": {"include": ["status"], "resources": false, "prompts": false}` + shEnv("bitwarden") + `}` + parkedPair + `
			}`,
			want: nil,
		},
		{
			name: "auth probe naming an undeclared tool rejected",
			manifest: `{
				"argocd": {"lazy": true, "command": "npx", "args": ["-y", "argocd-mcp@0.9.0"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600` + npxCache("argocd") + `, "auth_probe": {"tool": "list_applications"}, "tools": {"include": ["get_application"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
			want: []Violation{{Server: "argocd", Rule: "auth-probe", Message: `auth_probe tool "list_applications" is not in tools.include`}},
		},
		{
			name: "auth probe on a parked server rejected",
			manifest: `{
				"github": {"lazy": true, "command": "npx", "args": ["-y", "@modelcontextprotocol/server-github@2025.4.8"], "enabled": false, "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600` + npxCache("github") + `, "auth_probe": {"tool": "search_code"}, "tools": {"include": ["search_code"], "resources": false, "prompts": false}},
				"gistpad": {"lazy": true, "command": "npx", "args": ["-y", "gistpad-mcp@0.5.0"], "enabled": false, "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600` + npxCache("gistpad") + `}
			}`,
			want: []Violation{{Server: "github", Rule: "auth-probe", Message: "auth_probe declared on a parked server (it can never run)"}},
		},
		{
			name: "auth probe with a matching declared tool passes",
			manifest: `{
				"argocd": {"lazy": true, "command": "npx", "args": ["-y", "argocd-mcp@0.9.0"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600` + npxCache("argocd") + `, "auth_probe": {"tool": "list_applications", "args": {"limit": 1}}, "tools": {"include": ["list_applications"], "resources": false, "prompts": false}}` + parkedPair + `
			}`,
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
		"argocd": {"lazy": true, "command": "npx", "args": ["-y", "argocd-mcp@0.9.0"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600`+npxCache("argocd")+`, "tools": {"include": ["x"], "resources": false, "prompts": false}}`+parkedPair+`
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
		"argocd": {"lazy": true, "command": "npx", "args": ["-y", "argocd-mcp@latest"], "connect_timeout": 180, "idle_timeout_seconds": 1800, "max_lifetime_seconds": 21600`+npxCache("argocd")+`, "tools": {"include": ["x"], "resources": false, "prompts": false}}`+parkedPair+`
	}`))
	out = captureStdout(t, func() { rc = ExecuteValidate(bad) })
	if rc != 1 {
		t.Fatalf("bad rc = %d, want 1\n%s", rc, out)
	}
	if !strings.Contains(out, "FAIL argocd") || !strings.Contains(out, "[pin]") || !strings.Contains(out, "uses @latest") {
		t.Fatalf("bad run must print FAIL line with pin rule:\n%s", out)
	}
}
