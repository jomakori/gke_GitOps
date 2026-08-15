const { spawn } = require('child_process');

// Spawn the MCP server
const proc = spawn('npx', ['-y', 'google-workspace-mcp', 'serve'], {
  stdio: ['pipe', 'pipe', 'pipe'],
  env: { ...process.env }
});

// Forward stdin to server (JSON-RPC messages from MCP client)
process.stdin.pipe(proc.stdin);

// Forward server stderr directly
proc.stderr.pipe(process.stderr);

// Filter server stdout: pino log lines -> stderr, JSON-RPC -> stdout
const readline = require('readline');
const rl = readline.createInterface({ input: proc.stdout, crlfDelay: Infinity });
rl.on('line', (line) => {
  try {
    const parsed = JSON.parse(line);
    // JSON-RPC messages have a "jsonrpc" field
    if (parsed.jsonrpc) {
      process.stdout.write(line + '\n');
    } else {
      // Pino/bunyan log lines - send to stderr
      process.stderr.write(line + '\n');
    }
  } catch {
    // Non-JSON or parse error - send to stdout for MCP client to handle
    process.stdout.write(line + '\n');
  }
});

// Handle exit
proc.on('exit', (code) => process.exit(code));
process.on('SIGTERM', () => proc.kill('SIGTERM'));
process.on('SIGINT', () => proc.kill('SIGINT'));
