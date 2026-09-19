package boot

import (
	"os"
	"path/filepath"
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
	spec, cache := codegraphFrom(load(t, manifestFixture))
	if spec != "@colbymchenry/codegraph@1.6.0" {
		t.Errorf("spec = %q, want the pinned package", spec)
	}
	if cache != "/opt/data/.npm-mcp/codegraph" {
		t.Errorf("cache = %q, want the declared private cache", cache)
	}
}

func TestCodegraphFromManifestAbsent(t *testing.T) {
	spec, cache := codegraphFrom(load(t, `{"gistpad":{"command":"npx","args":["-y","x@1"]}}`))
	if spec != "" || cache != "" {
		t.Errorf("got (%q,%q), want empty when no codegraph entry exists", spec, cache)
	}
}
