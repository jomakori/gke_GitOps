package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type httpResponse struct {
	status  int
	header  http.Header
	message map[string]any
	body    string
}

// postJSON performs the Streamable HTTP POST the client makes, with the same
// Content-Type/Accept/protocol-version header set, optional session id and
// extra headers (server.declared headers), bounded by timeout.
func postJSON(url string, payload any, session string, extra map[string]string, timeout time.Duration) (*httpResponse, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", ProtocolVersion)
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	if session != "" {
		req.Header.Set("Mcp-Session-Id", session)
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	text := string(body)
	return &httpResponse{
		status:  resp.StatusCode,
		header:  resp.Header,
		message: parseBody(text),
		body:    text,
	}, nil
}

// parseBody mirrors verify.py's _parse_body: SSE "data:" events (last one
// wins) or a plain JSON body, else nil.
func parseBody(body string) map[string]any {
	var events []map[string]any
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(line[5:])), &event) == nil {
			events = append(events, event)
		}
	}
	if len(events) > 0 {
		return events[len(events)-1]
	}
	var msg map[string]any
	if json.Unmarshal([]byte(body), &msg) == nil {
		return msg
	}
	return nil
}

// openHTTP resolves the server's URL and headers, then completes initialize +
// notifications/initialized — the request context the handshake and the auth
// probe both need.
func openHTTP(server Server, environ []string, timeout time.Duration) (string, map[string]string, string, error) {
	missing := map[string]bool{}
	processEnv := map[string]string{}
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok {
			processEnv[k] = v
		}
	}
	url := Expand(server.URL, processEnv, missing)
	headers := map[string]string{}
	for k, v := range server.Headers {
		headers[k] = Expand(v, processEnv, missing)
	}

	initPayload := map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "openagent-mcp-verify", "version": "1.0"},
		},
	}
	resp, err := postJSON(url, initPayload, "", headers, timeout)
	if err != nil {
		return "", nil, "", &VerifyError{Msg: "initialize HTTP: " + err.Error()}
	}
	if resp.status >= 400 || resp.message == nil {
		return "", nil, "", &VerifyError{Msg: fmt.Sprintf("initialize HTTP %d: %s", resp.status, snippet(resp.body))}
	}
	if errMsg, ok := resp.message["error"]; ok && errMsg != nil {
		return "", nil, "", &VerifyError{Msg: fmt.Sprintf("initialize error: %v", errMsg)}
	}
	session := resp.header.Get("Mcp-Session-Id")
	_, _ = postJSON(url, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}, session, headers, timeout)
	return url, headers, session, nil
}

// ProbeHTTP runs a server's declared auth probe: one initialize, then a single
// tools/call. See AuthProbe for why the handshake alone is not evidence.
func ProbeHTTP(server Server, timeoutSec int, environ []string, probe AuthProbe) error {
	timeout := time.Duration(timeoutSec) * time.Second
	url, headers, session, err := openHTTP(server, environ, timeout)
	if err != nil {
		return err
	}
	args := probe.Args
	if args == nil {
		args = map[string]any{}
	}
	payload := map[string]any{
		"jsonrpc": "2.0", "id": probeRPCID, "method": "tools/call",
		"params": map[string]any{"name": probe.Tool, "arguments": args},
	}
	resp, err := postJSON(url, payload, session, headers, timeout)
	if err != nil {
		return &VerifyError{Msg: fmt.Sprintf("tools/call %s HTTP: %v", probe.Tool, err)}
	}
	if resp.status >= 400 || resp.message == nil {
		return &VerifyError{Msg: fmt.Sprintf("tools/call %s HTTP %d: %s", probe.Tool, resp.status, snippet(resp.body))}
	}
	if errMsg, ok := resp.message["error"]; ok && errMsg != nil {
		return &VerifyError{Msg: fmt.Sprintf("tools/call %s error: %v", probe.Tool, errMsg)}
	}
	result, _ := resp.message["result"].(map[string]any)
	if isErr, _ := result["isError"].(bool); isErr {
		return &VerifyError{Msg: fmt.Sprintf("tools/call %s returned isError: %s", probe.Tool, snippet(toolText(result)))}
	}
	return nil
}

// HandshakeHTTP performs initialize + notifications/initialized + a
// cursor-walking tools/list over Streamable HTTP, bounded by timeoutSec.
func HandshakeHTTP(server Server, timeoutSec int, environ []string) ([]string, string, error) {
	timeout := time.Duration(timeoutSec) * time.Second
	url, headers, session, err := openHTTP(server, environ, timeout)
	if err != nil {
		return nil, "", err
	}

	var tools []string
	var cursor string
	rpcID := 2
	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		payload := map[string]any{"jsonrpc": "2.0", "id": rpcID, "method": "tools/list", "params": params}
		resp, err := postJSON(url, payload, session, headers, timeout)
		if err != nil {
			return nil, "", &VerifyError{Msg: "tools/list HTTP: " + err.Error()}
		}
		if resp.status >= 400 || resp.message == nil {
			return nil, "", &VerifyError{Msg: fmt.Sprintf("tools/list HTTP %d: %s", resp.status, snippet(resp.body))}
		}
		if errMsg, ok := resp.message["error"]; ok && errMsg != nil {
			return nil, "", &VerifyError{Msg: fmt.Sprintf("tools/list error: %v", errMsg)}
		}
		result, _ := resp.message["result"].(map[string]any)
		tools = append(tools, toolItems(result["tools"])...)
		if next, ok := result["nextCursor"].(string); ok && next != "" {
			cursor = next
			rpcID++
			continue
		}
		return tools, "", nil
	}
}

func snippet(body string) string {
	if len(body) > 400 {
		return body[:400]
	}
	return body
}
