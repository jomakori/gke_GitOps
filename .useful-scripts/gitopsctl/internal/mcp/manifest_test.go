package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeManifest(t *testing.T, servers map[string]*Server) string {
	t.Helper()
	data, err := json.Marshal(servers)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "mcp-manifest")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSelectSortsAndFilters(t *testing.T) {
	servers := map[string]*Server{
		"zeta":  {Command: "npx", Args: []string{"-y", "zeta-pkg"}},
		"alpha": {Command: "npx", Args: []string{"-y", "alpha-pkg"}},
		"park":  {Enabled: boolPtr(false), Command: "npx", Args: []string{"-y", "park-pkg"}},
		"nil":   nil,
	}
	all := Select(servers, nil)
	if len(all) != 2 || all[0].Name != "alpha" || all[1].Name != "zeta" {
		t.Fatalf("Select() = %v, want sorted alpha,zeta", names(all))
	}
	only := Select(servers, []string{"zeta"})
	if len(only) != 1 || only[0].Name != "zeta" {
		t.Fatalf("Select(only) = %v, want [zeta]", names(only))
	}
}

func names(sel []NamedServer) []string {
	var out []string
	for _, s := range sel {
		out = append(out, s.Name)
	}
	return out
}

func boolPtr(b bool) *bool { return &b }

func TestIsEnabled(t *testing.T) {
	if (Server{}).IsEnabled() != true {
		t.Fatal("absent enabled should default true")
	}
	if (Server{Enabled: boolPtr(false)}).IsEnabled() {
		t.Fatal("explicit false should disable")
	}
}

func TestVerifyTimeout(t *testing.T) {
	if got := (Server{ConnectTimeout: 25}).VerifyTimeout(); got != 50 {
		t.Fatalf("VerifyTimeout = %d, want 50s", got)
	}
	if got := (Server{}).VerifyTimeout(); got != 120 {
		t.Fatalf("VerifyTimeout default = %d, want 120s", got)
	}
}

func TestPackageSpec(t *testing.T) {
	cases := []struct {
		name     string
		server   Server
		wantKind string
		wantSpec string
	}{
		{"plain npx", Server{Command: "npx", Args: []string{"-y", "@dopplerhq/mcp-server@1.0.5"}}, "npm", "@dopplerhq/mcp-server@1.0.5"},
		{"plain uvx", Server{Command: "uvx", Args: []string{"plane-mcp-server==0.3.2", "stdio"}}, "uv", "plane-mcp-server==0.3.2"},
		{"sh-wrapped exec npx", Server{Command: "sh", Args: []string{"-c", "exec npx -y @leval/mcp-grafana@1.1.7 | grep --line-buffered jsonrpc"}}, "npm", "@leval/mcp-grafana@1.1.7"},
		{"sh-wrapped grep", Server{Command: "sh", Args: []string{"-c", "exec npx -y foo@2.0.0"}}, "npm", "foo@2.0.0"},
		{"abs path binary", Server{Command: "/opt/data/bin/obscura", Args: []string{"mcp"}}, "", ""},
		{"no package", Server{Command: "sh", Args: []string{"-c", "cd /tmp && sleep 1"}}, "", ""},
	}
	for _, c := range cases {
		kind, spec := PackageSpec(c.server)
		if kind != c.wantKind || spec != c.wantSpec {
			t.Errorf("%s: PackageSpec = (%q,%q), want (%q,%q)", c.name, kind, spec, c.wantKind, c.wantSpec)
		}
	}
}

func TestExpand(t *testing.T) {
	lookup := map[string]string{"HOME": "/home/test", "PLANE_API_KEY": "secret", "DOPPLER_X": "plain"}
	missing := map[string]bool{}
	got := Expand("${HOME}/sub", lookup, missing)
	if got != "/home/test/sub" {
		t.Errorf("Expand(HOME) = %q, want /home/test/sub", got)
	}
	if got := Expand("$MISSING_VAR", lookup, missing); got != "$MISSING_VAR" {
		t.Errorf("Expand(missing) = %q, want literal kept", got)
	}
	if !missing["MISSING_VAR"] {
		t.Error("missing var should be recorded")
	}
	if got := Expand("http://x/${PLANE_API_KEY}", lookup, missing); got != "http://x/secret" {
		t.Errorf("Expand(nested) = %q", got)
	}
	if got := Expand("${DOPPLER_X}", lookup, missing); got != "plain" {
		t.Errorf("Expand(chained) = %q", got)
	}
}

func TestChildEnvBaseline(t *testing.T) {
	server := Server{Env: map[string]string{"MY_VAR": "1", "PATH": "/usr/custom"}}
	environ := []string{"MY_VAR=2", "PATH=/usr/bin", "HOME=/tmp", "LANG=en", "KNOWS_SECRETS=yes"}
	got, _ := ChildEnv(server, environ)
	joined := strings.Join(environSlice(got), "\n")
	if !strings.Contains(joined, "MY_VAR=1") {
		t.Error("server env should win over ambient")
	}
	if !strings.Contains(joined, "PATH=/usr/custom") {
		t.Error("PATH should come from server env when declared")
	}
	if !strings.Contains(joined, "HOME=/tmp") {
		t.Error("HOME should survive from ambient baseline")
	}
	if strings.Contains(joined, "KNOWS_SECRETS=") {
		t.Error("undeclared ambient secrets must not leak into child env")
	}
}

func TestChildEnvMissingDiagnosis(t *testing.T) {
	server := Server{Env: map[string]string{"DOPPLER_TOKEN": "${MCP_DOPPLER_TOKEN}"}}
	_, missing := ChildEnv(server, []string{"HOME=/tmp"})
	if !missing["MCP_DOPPLER_TOKEN"] {
		t.Error("unresolved placeholder should be diagnosed")
	}
}

func TestNPMCacheFor(t *testing.T) {
	if got := NPMCacheFor(Server{Env: map[string]string{"npm_config_cache": "/opt/data/.npm-mcp/argocd"}}); got != "/opt/data/.npm-mcp/argocd" {
		t.Errorf("NPMCacheFor = %q", got)
	}
	if got := NPMCacheFor(Server{}); got == "" {
		t.Error("NPMCacheFor default should fall back to NPM_CONFIG_CACHE/HOME")
	}
}

func TestDeclaredToolsNilSafe(t *testing.T) {
	if got := DeclaredTools(Server{}); got != nil {
		t.Errorf("DeclaredTools(nil tools) = %v, want nil", got)
	}
	server := Server{Tools: &ToolsConfig{Include: []string{"tool_a", "tool_b"}}}
	got := DeclaredTools(server)
	if len(got) != 2 || got[0] != "tool_a" {
		t.Errorf("DeclaredTools = %v", got)
	}
}
