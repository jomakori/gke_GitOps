package mcp

import (
	"strings"
	"testing"
	"time"
)

// A child that dies before answering must fail the read immediately with its
// stderr, not block until the handshake deadline.
func TestReadFailsFastWhenChildExits(t *testing.T) {
	c, err := NewStdio(Server{Command: "sh", Args: []string{"-c", "echo boom >&2; exit 1"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	start := time.Now()
	_, err = c.Read(1, time.Now().Add(30*time.Second))
	if err == nil {
		t.Fatal("expected an error from a child that exited")
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("read took %v; should fail as soon as the child exits", elapsed)
	}
	ve, ok := err.(*VerifyError)
	if !ok || !strings.Contains(ve.Msg, "exited") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(strings.Join(c.Stderr(), "\n"), "boom") {
		t.Fatalf("child stderr not captured: %v", c.Stderr())
	}
}
