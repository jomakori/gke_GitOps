package boot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitopsctl/internal/mcp"
)

// manifestFixture is the rendered shape of the codegraph entry in values.yaml.
const manifestFixture = `{
  "codegraph": {
    "args": ["-y", "@colbymchenry/codegraph@1.6.0", "serve", "--mcp"],
    "command": "npx",
    "connect_timeout": 180,
    "env": {
      "CODEGRAPH_TELEMETRY": "0",
      "npm_config_cache": "/opt/data/.npm-mcp/codegraph"
    }
  },
  "gistpad": {"enabled": false, "command": "npx", "args": ["-y", "gistpad-mcp@0.5.0"]},
  "plain": {"url": "https://example.test/mcp"}
}`

func load(t *testing.T, body string) map[string]*mcp.Server {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mcp-manifest")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	servers, err := mcp.LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	return servers
}

func TestCodegraphFromManifest(t *testing.T) {
	spec, cache := CodegraphFrom(load(t, manifestFixture))
	if spec != "@colbymchenry/codegraph@1.6.0" {
		t.Errorf("spec = %q, want the pinned package", spec)
	}
	if cache != "/opt/data/.npm-mcp/codegraph" {
		t.Errorf("cache = %q, want the declared private cache", cache)
	}
}

func TestCodegraphFromManifestAbsent(t *testing.T) {
	spec, cache := CodegraphFrom(load(t, `{"gistpad":{"command":"npx","args":["-y","x@1"]}}`))
	if spec != "" || cache != "" {
		t.Errorf("got (%q,%q), want empty when no codegraph entry exists", spec, cache)
	}
}

func TestCodegraphCommandForInitVsSync(t *testing.T) {
	spec := "@colbymchenry/codegraph@1.6.0"
	dir := t.TempDir()

	cmd := codegraphCommandFor(spec, dir)
	if cmd.verb != "init" {
		t.Errorf("verb = %q, want init for an unindexed path", cmd.verb)
	}
	if want := "-y " + spec + " init -y " + dir; strings.Join(cmd.args, " ") != want {
		t.Errorf("args = %v, want %q", cmd.args, want)
	}

	if err := os.Mkdir(filepath.Join(dir, ".codegraph"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd = codegraphCommandFor(spec, dir)
	if cmd.verb != "sync" {
		t.Errorf("verb = %q, want sync for an indexed path", cmd.verb)
	}
	if want := "-y " + spec + " sync " + dir; strings.Join(cmd.args, " ") != want {
		t.Errorf("args = %v, want %q", cmd.args, want)
	}
	if cmd.budget != 120 {
		t.Errorf("budget = %d, want 120 for sync", cmd.budget)
	}
}
