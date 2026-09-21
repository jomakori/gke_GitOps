package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// VerifyError carries the server's own stderr — the detail the Hermes MCP
// client discards and the reason the original problem was half-invisible.
type VerifyError struct {
	Msg    string
	Stderr string
}

func (e *VerifyError) Error() string { return e.Msg }

// Stdio is a client-mirrored stdio MCP child with a hard per-message deadline
// and a process-group kill on close — nothing can outlive the handshake.
type Stdio struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	messages chan json.RawMessage

	diagMu sync.Mutex
	diag   []string

	stderrMu sync.Mutex
	stderr   []string

	closeOnce sync.Once
	proc      *os.Process
}

// NewStdio starts the server child. mirroring verify.py's Stdio: baseline env
// plus declared env with process-env placeholder resolution.
func NewStdio(server Server, environ []string) (*Stdio, error) {
	childEnv, _ := ChildEnv(server, environ)
	cmd := exec.Command(server.Command, server.Args...)
	cmd.Env = environSlice(childEnv)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c := &Stdio{
		cmd:      cmd,
		proc:     cmd.Process,
		stdin:    stdin,
		messages: make(chan json.RawMessage, 64),
	}
	go c.drainStderr(stderr)
	go c.scanStdout(stdout)
	return c, nil
}

func (c *Stdio) Diag() []string {
	c.diagMu.Lock()
	defer c.diagMu.Unlock()
	return append([]string(nil), c.diag...)
}

func (c *Stdio) drainStderr(r io.Reader) {
	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			line = strings.TrimRight(line, "\r\n")
			c.stderrMu.Lock()
			if len(c.stderr) < StderrLines {
				c.stderr = append(c.stderr, line)
			}
			c.stderrMu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// scanStdout classifies stdout lines: JSON-RPC messages are forwarded, banner
// / log lines (see the grafana server) are kept as diagnostics.
func (c *Stdio) scanStdout(r io.Reader) {
	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) == 0 && err != nil {
			return
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			if err != nil {
				return
			}
			continue
		}
		var raw json.RawMessage
		if json.Unmarshal(line, &raw) == nil && raw[0] == '{' {
			c.messages <- append(json.RawMessage(nil), raw...)
		} else {
			c.diagMu.Lock()
			text := string(line)
			if len(text) > 300 {
				text = text[:300]
			}
			c.diag = append(c.diag, text)
			c.diagMu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (c *Stdio) Send(payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

// Read waits (bounded by deadline) for the response with the given id.
// Messages with other ids are discarded; wantID may be nil to take any.
func (c *Stdio) Read(wantID any, deadline time.Time) (map[string]any, error) {
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, &VerifyError{Msg: "timed out waiting for response"}
		}
		select {
		case raw := <-c.messages:
			var msg map[string]any
			if err := json.Unmarshal(raw, &msg); err != nil {
				return nil, &VerifyError{Msg: "malformed response: " + err.Error()}
			}
			if wantID == nil || fmt.Sprint(msg["id"]) == fmt.Sprint(wantID) {
				return msg, nil
			}
		case <-time.After(remaining):
			return nil, &VerifyError{Msg: "timed out waiting for response"}
		}
	}
}

func (c *Stdio) Stderr() []string {
	c.stderrMu.Lock()
	defer c.stderrMu.Unlock()
	return append([]string(nil), c.stderr...)
}

// Close kills the child's process group (SIGTERM, then SIGKILL after 500ms)
// and reaps it — the Python close() contract.
func (c *Stdio) Close() {
	c.closeOnce.Do(func() {
		if c.proc != nil && c.proc.Signal(syscall.Signal(0)) == nil {
			pgid := c.proc.Pid
			_ = syscall.Kill(-pgid, syscall.SIGTERM)
			time.Sleep(500 * time.Millisecond)
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		}
		done := make(chan struct{})
		go func() { _ = c.cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	})
}

func listToolsStdio(c *Stdio, deadline time.Time) ([]string, error) {
	var tools []string
	var cursor string
	rpcID := 2
	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		if err := c.Send(map[string]any{
			"jsonrpc": "2.0", "id": rpcID, "method": "tools/list", "params": params,
		}); err != nil {
			return nil, &VerifyError{Msg: "tools/list write: " + err.Error()}
		}
		response, err := c.Read(rpcID, deadline)
		if err != nil {
			return nil, err
		}
		if errMsg, ok := response["error"]; ok && errMsg != nil {
			return nil, &VerifyError{Msg: fmt.Sprintf("tools/list error: %v", errMsg)}
		}
		result, _ := response["result"].(map[string]any)
		for _, item := range toolItems(result["tools"]) {
			tools = append(tools, item)
		}
		if next, ok := result["nextCursor"].(string); ok && next != "" {
			cursor = next
			rpcID++
			continue
		}
		return tools, nil
	}
}

func toolItems(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		tool, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, present := tool["name"]
		if !present {
			out = append(out, "?")
		} else {
			out = append(out, fmt.Sprint(name))
		}
	}
	return out
}

// openStdio starts the child and completes initialize + notifications/
// initialized, returning the live connection. Shared by the handshake and the
// auth probe so both speak the client's exact opening sequence.
func openStdio(server Server, environ []string, deadline time.Time) (*Stdio, string, error) {
	client, err := NewStdio(server, environ)
	if err != nil {
		return nil, "", &VerifyError{Msg: "start: " + err.Error()}
	}
	if err := client.Send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "openagent-mcp-verify", "version": "1.0"},
		},
	}); err != nil {
		return nil, joinLines(client.Stderr()), &VerifyError{Msg: "initialize write: " + err.Error(), Stderr: joinLines(client.Stderr())}
	}
	response, err := client.Read(1, deadline)
	if err != nil {
		err.(*VerifyError).Stderr = joinLines(client.Stderr())
		return nil, joinLines(client.Stderr()), err
	}
	if errMsg, ok := response["error"]; ok && errMsg != nil {
		return nil, joinLines(client.Stderr()), &VerifyError{Msg: fmt.Sprintf("initialize error: %v", errMsg), Stderr: joinLines(client.Stderr())}
	}
	if err := client.Send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}); err != nil {
		return nil, joinLines(client.Stderr()), &VerifyError{Msg: "notifications/initialized write: " + err.Error(), Stderr: joinLines(client.Stderr())}
	}
	return client, joinLines(client.Stderr()), nil
}

// ProbeStdio runs a server's declared auth probe: its own connection, one
// initialize, then a single tools/call. This is the only check that exercises
// the CREDENTIAL rather than the protocol — see AuthProbe for why that gap
// matters.
func ProbeStdio(server Server, timeoutSec int, environ []string, probe AuthProbe) error {
	client, stderr, err := openStdio(server, environ, time.Now().Add(time.Duration(timeoutSec)*time.Second))
	if client == nil {
		return err
	}
	defer client.Close()
	if err != nil {
		return err
	}
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	if err := callToolStdio(client, probe, deadline); err != nil {
		if ve, ok := err.(*VerifyError); ok && ve.Stderr == "" {
			ve.Stderr = stderr
		}
		return err
	}
	return nil
}

// callToolStdio sends tools/call and treats both a JSON-RPC error and an
// isError result as failure — servers report a dead credential the second way
// (google-workspace answers with isError + an "ACTION REQUIRED" text block).
func callToolStdio(c *Stdio, probe AuthProbe, deadline time.Time) error {
	args := probe.Args
	if args == nil {
		args = map[string]any{}
	}
	if err := c.Send(map[string]any{
		"jsonrpc": "2.0", "id": probeRPCID, "method": "tools/call",
		"params": map[string]any{"name": probe.Tool, "arguments": args},
	}); err != nil {
		return &VerifyError{Msg: "tools/call write: " + err.Error()}
	}
	response, err := c.Read(probeRPCID, deadline)
	if err != nil {
		return &VerifyError{Msg: fmt.Sprintf("tools/call %s: %v", probe.Tool, err)}
	}
	if errMsg, ok := response["error"]; ok && errMsg != nil {
		return &VerifyError{Msg: fmt.Sprintf("tools/call %s error: %v", probe.Tool, errMsg)}
	}
	result, _ := response["result"].(map[string]any)
	if isErr, _ := result["isError"].(bool); isErr {
		return &VerifyError{Msg: fmt.Sprintf("tools/call %s returned isError: %s", probe.Tool, snippet(toolText(result)))}
	}
	return nil
}

// toolText flattens an MCP content array into one bounded line, which is what
// carries the real reason on failure (an OAuth consent URL, a 401 body).
func toolText(result map[string]any) string {
	parts := make([]string, 0, 4)
	if list, ok := result["content"].([]any); ok {
		for _, item := range list {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := entry["text"].(string); ok && text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, " ")
}

// HandshakeStdio performs initialize + notifications/initialized + a
// cursor-walking tools/list over stdio, bounded by timeoutSec.
func HandshakeStdio(server Server, timeoutSec int, environ []string) ([]string, string, error) {
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	client, stderr, err := openStdio(server, environ, deadline)
	if client == nil {
		return nil, stderr, err
	}
	warn := ""
	defer func() {
		client.Close()
		if warn != "" {
			client.diagMu.Lock()
			client.diag = append(client.diag, warn)
			client.diagMu.Unlock()
		}
	}()
	if err != nil {
		return nil, stderr, err
	}
	tools, err := listToolsStdio(client, deadline)
	if err != nil {
		if ve, ok := err.(*VerifyError); ok && ve.Stderr == "" {
			ve.Stderr = joinLines(client.Stderr())
		}
		return nil, joinLines(client.Stderr()), err
	}
	return tools, joinLines(client.Stderr()), nil
}

func joinLines(lines []string) string { return strings.Join(lines, "\n") }
