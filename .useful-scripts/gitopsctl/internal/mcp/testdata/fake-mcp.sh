#!/bin/bash
# Tiny JSON-RPC MCP server for handshake tests. Serves initialize (id 1),
# notifications/initialized (no reply), tools/list (last id) and tools/call.
# Optional env vars: FAKE_MCP_DROP="tool_a" removes a tool; FAKE_MCP_STDERR
# writes to stderr; FAKE_MCP_TOOL_ERROR="msg" makes every tools/call answer
# with an isError result (the shape servers use for a dead credential).
if [ -n "${FAKE_MCP_STDERR:-}" ]; then
  echo "fake-mcp stderr noise: ${FAKE_MCP_STDERR}" >&2
fi
# Fail the first spawn only, then serve normally — the transient-failure shape
# a verify retry is meant to absorb (an upstream 503, an install-lock wait).
if [ -n "${FAKE_MCP_FAIL_ONCE_MARKER:-}" ] && [ ! -f "$FAKE_MCP_FAIL_ONCE_MARKER" ]; then
  : > "$FAKE_MCP_FAIL_ONCE_MARKER"
  exit 1
fi
INIT='{"jsonrpc":"2.0","id":__ID__,"result":{"protocolVersion":"2024-11-05","capabilities":{"tools":{}},"serverInfo":{"name":"fake","version":"1.0.0"}}}'
LIST_FULL='{"jsonrpc":"2.0","id":__ID__,"result":{"tools":[{"name":"tool_a","description":"a"},{"name":"tool_b","description":"b"}]}}'
LIST_DROP='{"jsonrpc":"2.0","id":__ID__,"result":{"tools":[{"name":"tool_b","description":"b"}]}}'
CALL_OK='{"jsonrpc":"2.0","id":__ID__,"result":{"content":[{"type":"text","text":"ok"}],"isError":false}}'
while IFS= read -r line; do
  id=$(printf '%s' "$line" | sed -n 's/.*"id":[[:space:]]*\([0-9]*\).*/\1/p')
  [ -z "$id" ] && continue
  case "$line" in
    *'"method":"initialize"'*) echo "$INIT" | sed "s/__ID__/$id/";;
    *'"method":"tools/list"'*)
      if [ "${FAKE_MCP_DROP:-}" = "tool_a" ]; then
        echo "$LIST_DROP" | sed "s/__ID__/$id/"
      else
        echo "$LIST_FULL" | sed "s/__ID__/$id/"
      fi;;
    *'"method":"tools/call"'*)
      if [ -n "${FAKE_MCP_TOOL_ERROR:-}" ]; then
        printf '{"jsonrpc":"2.0","id":%s,"result":{"content":[{"type":"text","text":"%s"}],"isError":true}}\n' "$id" "${FAKE_MCP_TOOL_ERROR}"
      else
        echo "$CALL_OK" | sed "s/__ID__/$id/"
      fi;;
  esac
done
