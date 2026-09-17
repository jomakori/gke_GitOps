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
