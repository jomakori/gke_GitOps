package boot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const launchFixture = `agent:
  max_turns: 90
mcp_discovery_timeout: 20
mcp_servers:
  argocd:
    command: npx
    env:
      ARGOCD_TOKEN: ${MCP_ARGOCD_TOKEN}
  gistpad:
    enabled: false
    command: npx
  grafana:
    command: sh
    env:
      GRAFANA_URL: "${MCP_GRAFANA_URL}"
  kiwi:
    url: https://mcp.kiwi.com
    lazy: true
model:
  default: deepseek-v4-flash-direct
`

const staleProfileFixture = `agent:
  max_turns: 40
mcp_servers:
  argocd:
    command: npx
  gistpad:
    command: npx
  kubectl:
    url: http://kubernetes-mcp-server.openagent.svc.cluster.local:8000/mcp
model:
  default: glm-5.3
`

func TestTopLevelBlock(t *testing.T) {
	block, ok := topLevelBlock(launchFixture, "mcp_servers")
	if !ok {
		t.Fatal("mcp_servers block not found")
	}
	if !strings.HasPrefix(block, "mcp_servers:\n") {
		t.Fatalf("block does not start at the key:\n%s", block)
	}
	if !strings.Contains(block, "GRAFANA_URL") || strings.Contains(block, "model:") {
		t.Fatalf("block leaked past its top-level end:\n%s", block)
	}
}

func TestBlockServerNames(t *testing.T) {
	block, _ := topLevelBlock(launchFixture, "mcp_servers")
	names := blockServerNames(block)
	if len(names) != 4 || !names["argocd"] || !names["gistpad"] || !names["grafana"] || !names["kiwi"] {
		t.Fatalf("unexpected server names: %v", names)
	}
	if names["env"] || names["command"] {
		t.Fatalf("nested keys were read as servers: %v", names)
	}
}

func TestPlaceholders(t *testing.T) {
	block, _ := topLevelBlock(launchFixture, "mcp_servers")
	names := placeholders(block)
	want := []string{"MCP_ARGOCD_TOKEN", "MCP_GRAFANA_URL"}
	if len(names) != len(want) || names[0] != want[0] || names[1] != want[1] {
		t.Fatalf("placeholders = %v, want %v", names, want)
	}
}

func TestPinLaunchHome(t *testing.T) {
	block, _ := topLevelBlock(launchFixture, "mcp_servers")
	pinned := pinLaunchHome(block)

	if got := strings.Count(pinned, "HERMES_HOME: \""+dataRoot+"\""); got != 3 {
		t.Fatalf("pinned %d stdio entries, want 3 (kiwi is HTTP):\n%s", got, pinned)
	}
	// An entry with an env mapping gets the key inside it.
	if !strings.Contains(pinned, "    env:\n      HERMES_HOME: \"/opt/data\"\n      ARGOCD_TOKEN: ${MCP_ARGOCD_TOKEN}\n") {
		t.Fatalf("env entry was not extended:\n%s", pinned)
	}
	// An entry without one gets an env mapping.
	if !strings.Contains(pinned, "  gistpad:\n    enabled: false\n    command: npx\n    env:\n      HERMES_HOME: \"/opt/data\"\n") {
		t.Fatalf("env-less entry did not gain an env mapping:\n%s", pinned)
	}
	// HTTP entries keep their shape: env is irrelevant to their identity.
	if strings.Contains(pinned, "  kiwi:\n    url: https://mcp.kiwi.com\n    lazy: true\n    env:") {
		t.Fatalf("HTTP entry gained an env mapping:\n%s", pinned)
	}
	if again := pinLaunchHome(block); again != pinned {
		t.Fatal("pinning is not deterministic")
	}
}

func TestPinLaunchHomeLeavesDeclaredValue(t *testing.T) {
	block := "mcp_servers:\n  argocd:\n    command: npx\n    env:\n      HERMES_HOME: \"/custom\"\n"
	if got := pinLaunchHome(block); got != block {
		t.Fatalf("an entry that declares HERMES_HOME must be left alone:\n%s", got)
	}
}

func TestPinLaunchHomeInlineEnv(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"    env: {}\n", "    env: {HERMES_HOME: \"/opt/data\"}\n"},
		{"    env: {A: b}\n", "    env: {A: b, HERMES_HOME: \"/opt/data\"}\n"},
	} {
		block := "mcp_servers:\n  argocd:\n    command: npx\n" + tc.in
		if got := pinLaunchHome(block); !strings.Contains(got, tc.want) {
			t.Fatalf("inline env %q became:\n%s", tc.in, got)
		}
	}
}

func TestReplaceTopLevelBlockKeepsRest(t *testing.T) {
	block, _ := topLevelBlock(launchFixture, "mcp_servers")
	updated, replaced := replaceTopLevelBlock(staleProfileFixture, "mcp_servers", pinLaunchHome(block))
	if !replaced {
		t.Fatal("block was not replaced")
	}
	if strings.Contains(updated, "kubectl") {
		t.Fatalf("stale server survived:\n%s", updated)
	}
	if !strings.Contains(updated, "max_turns: 40") || !strings.Contains(updated, "default: glm-5.3") {
		t.Fatalf("profile-specific settings were lost:\n%s", updated)
	}
	if !strings.HasSuffix(updated, "\n") {
		t.Fatal("file lost its trailing newline")
	}
}

func TestReplaceTopLevelBlockAtEOF(t *testing.T) {
	text := "agent:\n  max_turns: 1\nmcp_servers:\n  old:\n    url: http://dead"
	updated, replaced := replaceTopLevelBlock(text, "mcp_servers", "mcp_servers:\n  new:\n    command: npx\n")
	if !replaced {
		t.Fatal("block was not replaced")
	}
	if !strings.HasSuffix(updated, "    command: npx\n") {
		t.Fatalf("block at EOF was not replaced cleanly:\n%s", updated)
	}
}

func TestReplaceTopLevelBlockWithoutKeyIsUntouched(t *testing.T) {
	text := "agent:\n  max_turns: 1\n"
	updated, replaced := replaceTopLevelBlock(text, "mcp_servers", "mcp_servers: {}\n")
	if replaced || updated != text {
		t.Fatal("a file without the key must not be rewritten")
	}
}

func TestSyncProfileMCPAlignsStaleProfiles(t *testing.T) {
	root := t.TempDir()
	launch := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(launch, []byte(launchFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	profiles := filepath.Join(root, "profiles")
	stale := filepath.Join(profiles, "atlas")
	clean := filepath.Join(profiles, "explore")
	for _, dir := range []string{stale, clean} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(stale, "config.yaml"), []byte(staleProfileFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	launchBlock, _ := topLevelBlock(launchFixture, "mcp_servers")
	// A profile already carrying the mirrored (pinned) block is left alone.
	if err := os.WriteFile(filepath.Join(clean, "config.yaml"), []byte(replaceFixture(t, launchFixture, launchBlock)), 0o600); err != nil {
		t.Fatal(err)
	}

	changed, err := syncProfileMCP(launch, profiles)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0] != filepath.Join(stale, "config.yaml") {
		t.Fatalf("changed = %v, want only the stale profile", changed)
	}

	seen, err := os.ReadFile(filepath.Join(stale, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(seen), "kubernetes-mcp-server") {
		t.Fatalf("stale kubectl entry survived:\n%s", seen)
	}
	want := pinLaunchHome(launchBlock)
	if !strings.Contains(string(seen), want) {
		t.Fatalf("profile block is not the pinned launch block:\n%s", seen)
	}
	if !strings.Contains(string(seen), "HERMES_HOME: \"/opt/data\"") {
		t.Fatalf("mirrored block carries no launched-home pin:\n%s", seen)
	}
	backup, err := os.ReadFile(filepath.Join(stale, "config.yaml.bak-mcp-sync"))
	if err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	if !strings.Contains(string(backup), "kubernetes-mcp-server") {
		t.Fatal("backup must hold the pre-sync file")
	}

	// Idempotent: a second run rewrites nothing.
	again, err := syncProfileMCP(launch, profiles)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("second run changed %v", again)
	}
}

// replaceFixture returns text with its mcp_servers block swapped for the
// already-pinned mirror, without going through file I/O.
func replaceFixture(t *testing.T, text, block string) string {
	t.Helper()
	updated, replaced := replaceTopLevelBlock(text, "mcp_servers", pinLaunchHome(block))
	if !replaced {
		t.Fatal("fixture has no mcp_servers block")
	}
	return updated
}

func TestSyncProfileSecretsWritesReferencedValues(t *testing.T) {
	root := t.TempDir()
	launch := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(launch, []byte(launchFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(root, "managed", ".env")
	environ := []string{
		"MCP_ARGOCD_TOKEN=argo-token",
		"MCP_GRAFANA_URL=http://grafana:3000",
		"UNRELATED=value",
	}

	written, mirrored, referenced, err := syncProfileSecrets(launch, envPath, environ)
	if err != nil {
		t.Fatal(err)
	}
	if !written || mirrored != 2 || referenced != 2 {
		t.Fatalf("written=%v mirrored=%d referenced=%d", written, mirrored, referenced)
	}
	body, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "MCP_ARGOCD_TOKEN=\"argo-token\"\nMCP_GRAFANA_URL=\"http://grafana:3000\"\n"
	if string(body) != want {
		t.Fatalf("managed env =\n%q\nwant\n%q", body, want)
	}
	if fi, err := os.Stat(envPath); err != nil || fi.Mode().Perm() != managedScopePerm {
		t.Fatalf("managed env mode = %v (%v), want %o", fi.Mode().Perm(), err, managedScopePerm)
	}

	// Idempotent, and an unchanged file keeps its mtime.
	before, _ := os.Stat(envPath)
	written, _, _, err = syncProfileSecrets(launch, envPath, environ)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(envPath)
	if written || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("an unchanged managed env must not be rewritten")
	}

	// A changed credential is picked up.
	environ[0] = "MCP_ARGOCD_TOKEN=rotated"
	written, _, _, err = syncProfileSecrets(launch, envPath, environ)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = os.ReadFile(envPath)
	if !written || !strings.Contains(string(body), "rotated") {
		t.Fatalf("rotation was not applied:\n%s", body)
	}
}

func TestSyncProfileSecretsSkipsMissingValues(t *testing.T) {
	root := t.TempDir()
	launch := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(launch, []byte(launchFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(root, ".env")
	written, mirrored, referenced, err := syncProfileSecrets(launch, envPath, []string{"MCP_ARGOCD_TOKEN="})
	if err != nil {
		t.Fatal(err)
	}
	if written || mirrored != 0 || referenced != 2 {
		t.Fatalf("written=%v mirrored=%d referenced=%d", written, mirrored, referenced)
	}
	if _, err := os.Stat(envPath); !os.IsNotExist(err) {
		t.Fatal("no file must be written when nothing resolves")
	}
}

func TestSyncProfileSecretsQuotesValues(t *testing.T) {
	root := t.TempDir()
	launch := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(launch, []byte(launchFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(root, ".env")
	environ := []string{
		`MCP_ARGOCD_TOKEN=has space # and hash`,
		`MCP_GRAFANA_URL=back\slash"quote`,
	}
	if _, _, _, err := syncProfileSecrets(launch, envPath, environ); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "MCP_ARGOCD_TOKEN=\"has space # and hash\"\n" + `MCP_GRAFANA_URL="back\\slash\"quote"` + "\n"
	if string(body) != want {
		t.Fatalf("managed env =\n%q\nwant\n%q", body, want)
	}
}
