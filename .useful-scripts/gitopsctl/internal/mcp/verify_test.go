package mcp

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	w.Close()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func fakeServer(t *testing.T, dropTool bool) *Server {
	t.Helper()
	script, err := filepath.Abs("testdata/fake-mcp.sh")
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"FAKE_MCP_STDERR": "doppler-style auth noise"}
	if dropTool {
		env["FAKE_MCP_DROP"] = "tool_a"
	}
	return &Server{
		Command:        "/bin/bash",
		Args:           []string{script},
		Env:            env,
		ConnectTimeout: 10,
		Tools:          &ToolsConfig{Include: []string{"tool_b", "tool_a"}},
	}
}

func TestExecutePreflightPass(t *testing.T) {
	manifest := writeManifest(t, map[string]*Server{"fake": fakeServer(t, false)})
	rc := 1
	out := captureStdout(t, func() { rc = Execute(manifest, "preflight", nil) })
	if rc != 0 {
		t.Fatalf("rc = %d, want 0\n%s", rc, out)
	}
	if !strings.Contains(out, "PASS fake 2 tools") {
		t.Fatalf("missing PASS line:\n%s", out)
	}
}

func TestExecutePreflightMissingToolFails(t *testing.T) {
	manifest := writeManifest(t, map[string]*Server{"fake": fakeServer(t, true)})
	rc := 1
	out := captureStdout(t, func() { rc = Execute(manifest, "preflight", nil) })
	if rc != 1 {
		t.Fatalf("rc = %d, want 1\n%s", rc, out)
	}
	if !strings.Contains(out, "FAIL fake") || !strings.Contains(out, "tool_a") {
		t.Fatalf("missing FAIL + missing-tool report:\n%s", out)
	}
	if !strings.Contains(out, "fake-mcp stderr noise") {
		t.Fatalf("server stderr should surface on failure:\n%s", out)
	}
}

func TestExecuteDriftQuietAndLoud(t *testing.T) {
	passManifest := writeManifest(t, map[string]*Server{"fake": fakeServer(t, false)})
	rc := 1
	out := captureStdout(t, func() { rc = Execute(passManifest, "drift", nil) })
	if rc != 0 {
		t.Fatalf("drift (clean) rc = %d, want 0\n%s", rc, out)
	}
	if strings.Contains(out, "PASS") {
		t.Fatalf("drift mode must stay quiet on match:\n%s", out)
	}

	driftManifest := writeManifest(t, map[string]*Server{"fake": fakeServer(t, true)})
	out = captureStdout(t, func() { rc = Execute(driftManifest, "drift", nil) })
	if rc != 1 {
		t.Fatalf("drift (mismatch) rc = %d, want 1\n%s", rc, out)
	}
	if !strings.Contains(out, "DRIFT fake") || !strings.Contains(out, "tool_a") {
		t.Fatalf("drift mismatch must print DRIFT + missing tools:\n%s", out)
	}
}

func TestExecuteOnlyAndParked(t *testing.T) {
	parked := fakeServer(t, false)
	parked.Enabled = boolPtr(false)
	manifest := writeManifest(t, map[string]*Server{
		"fake":   fakeServer(t, false),
		"parked": parked,
	})
	rc := 1
	out := captureStdout(t, func() { rc = Execute(manifest, "preflight", []string{"fake"}) })
	if rc != 0 {
		t.Fatalf("rc = %d, want 0\n%s", rc, out)
	}
	if strings.Contains(out, "parked") {
		t.Fatalf("parked server must not run:\n%s", out)
	}
}

func TestExecuteHangingServerBounded(t *testing.T) {
	hanger := &Server{
		Command:        "/bin/bash",
		Args:           []string{"-c", "sleep 30"},
		ConnectTimeout: 1,
	}
	manifest := writeManifest(t, map[string]*Server{"hanger": hanger})
	rc := 1
	out := captureStdout(t, func() { rc = Execute(manifest, "preflight", nil) })
	if rc != 1 {
		t.Fatalf("rc = %d, want 1\n%s", rc, out)
	}
	if !strings.Contains(out, "FAIL hanger") {
		t.Fatalf("hanger should FAIL:\n%s", out)
	}
}

// probedServer wraps the fake server with an auth probe on tool_b, optionally
// making every tools/call answer isError — the shape a dead credential takes.
func probedServer(t *testing.T, toolError string) *Server {
	t.Helper()
	server := fakeServer(t, false)
	if toolError != "" {
		server.Env["FAKE_MCP_TOOL_ERROR"] = toolError
	}
	server.AuthProbe = &AuthProbe{Tool: "tool_b"}
	return server
}

// TestExecuteDriftRunsAuthProbe is the regression test for the finding this
// probe exists for: a server whose handshake and tool list are perfect while
// every real call fails on auth (google-workspace's stored OAuth grant covered
// Drive only, so 33 of 35 declared tools answered "ACTION REQUIRED").
func TestExecuteDriftRunsAuthProbe(t *testing.T) {
	healthy := writeManifest(t, map[string]*Server{"fake": probedServer(t, "")})
	rc := 1
	out := captureStdout(t, func() { rc = Execute(healthy, "drift", nil) })
	if rc != 0 {
		t.Fatalf("drift (healthy probe) rc = %d, want 0\n%s", rc, out)
	}
	if out != "" {
		t.Fatalf("drift must stay quiet when everything matches:\n%s", out)
	}

	broken := writeManifest(t, map[string]*Server{"fake": probedServer(t, "ACTION REQUIRED: authorize Google Workspace")})
	out = captureStdout(t, func() { rc = Execute(broken, "drift", nil) })
	if rc != 1 {
		t.Fatalf("drift (dead credential) rc = %d, want 1\n%s", rc, out)
	}
	if !strings.Contains(out, "auth probe tool_b") || !strings.Contains(out, "ACTION REQUIRED") {
		t.Fatalf("probe failure must name the probe and surface the server's reason:\n%s", out)
	}
}

// TestExecutePreflightSkipsAuthProbe pins the blast radius: a dead third-party
// credential must alert on the drift cron, not fail a deploy's sync (a failing
// PostSync hook consumes the app's retry budget and strands it OutOfSync).
func TestExecutePreflightSkipsAuthProbe(t *testing.T) {
	manifest := writeManifest(t, map[string]*Server{"fake": probedServer(t, "ACTION REQUIRED")})
	rc := 1
	out := captureStdout(t, func() { rc = Execute(manifest, "preflight", nil) })
	if rc != 0 {
		t.Fatalf("preflight rc = %d, want 0 (probes are drift-only)\n%s", rc, out)
	}
	if strings.Contains(out, "auth probe") {
		t.Fatalf("preflight must not run probes:\n%s", out)
	}
}

// TestExecuteRetriesTransientFailure covers the other half of the noise: kiwi
// answered 503 once and two uv-linked servers lost their window to a shared
// install lock, each reporting a healthy server as drifted.
func TestExecuteRetriesTransientFailure(t *testing.T) {
	server := fakeServer(t, false)
	server.Env["FAKE_MCP_FAIL_ONCE_MARKER"] = filepath.Join(t.TempDir(), "failed-once")
	manifest := writeManifest(t, map[string]*Server{"fake": server})

	rc := 1
	out := captureStdout(t, func() { rc = Execute(manifest, "preflight", nil) })
	if rc != 0 {
		t.Fatalf("rc = %d, want 0 after retry\n%s", rc, out)
	}
	if !strings.Contains(out, "RETRY fake") || !strings.Contains(out, "PASS fake 2 tools") {
		t.Fatalf("expected a RETRY line then a PASS:\n%s", out)
	}
}

// TestExecuteUnresolvedPlaceholderFails pins the check that would have saved the
// doppler outage: a `${VAR}` that resolves nowhere reaches the server as the
// literal string, and the only symptom was the server's own "Cached token
// appears invalid" — i.e. it read as a dead credential, not a missing key.
func TestExecuteUnresolvedPlaceholderFails(t *testing.T) {
	server := fakeServer(t, false)
	server.Env["MCP_ABSENT_TOKEN"] = "${MCP_ABSENT_TOKEN}"
	manifest := writeManifest(t, map[string]*Server{"fake": server})

	rc := 1
	out := captureStdout(t, func() { rc = Execute(manifest, "preflight", nil) })
	if rc != 1 {
		t.Fatalf("rc = %d, want 1\n%s", rc, out)
	}
	if !strings.Contains(out, "FAIL fake: unresolved env placeholder(s): MCP_ABSENT_TOKEN") {
		t.Fatalf("expected a named unresolved placeholder:\n%s", out)
	}
	if strings.Contains(out, "PASS fake") {
		t.Fatalf("a server with an unresolved placeholder must not also report PASS:\n%s", out)
	}
}
