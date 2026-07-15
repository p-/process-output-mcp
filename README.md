# Process Output MCP Server

An MCP server that wraps a long-running process and exposes its timestamp-annotated stdout/stderr output via the **Streamable HTTP** transport.

## Build

```bash
cd process-output-mcp
go build -o process-output-mcp .
```

## Usage

```bash
./process-output-mcp [-p port] "<command>"
```

**Examples:**

```bash
# Wrap a Java server
./process-output-mcp "cd /app && java -jar server.jar"

# Wrap a build with custom port
./process-output-mcp -p 9090 "npm run dev"
```

By default the MCP server listens on port **8070** with endpoint `/mcp`.

Connect your MCP client to: `http://localhost:8070/mcp`

To change the port, use the `-p` flag (e.g. `-p 9090`).

## Transport

This server uses the **Streamable HTTP** transport (MCP spec 2025-03-26), which provides:
- Standard HTTP request/response communication
- Compatible with REST API infrastructure, load balancers, and API gateways
- No long-lived SSE connections required

## Tools

| Tool | Description | Parameters |
|------|-------------|------------|
| `get_output_lines_between` | Get output lines between two timestamps | `start_time` (RFC3339, required), `end_time` (RFC3339, required) |
| `get_output_lines_from` | Get all output lines from a given timestamp until now | `start_time` (RFC3339, required) |
| `get_latest_output` | Get the most recent lines of output | `lines` (number, default: 50) |
| `get_process_status` | Get process status (running/exited, exit code) | _(none)_ |
| `get_current_timestamp` | Get the current server timestamp in RFC3339 format | _(none)_ |

## Example Tool Responses

**get_latest_output:**
```json
[
  {"timestamp": "2026-07-14T10:00:01Z", "content": "Server starting...\n", "isStdErr": false},
  {"timestamp": "2026-07-14T10:00:02Z", "content": "Listening on :3000\n", "isStdErr": false}
]
```

**get_process_status:**
```json
{"running": true, "exitCode": 0}
```

## Tips & Tricks

### Capture file events with strace

You can use this tool to wrap `strace` and expose file-related syscalls of a running process via MCP:

```bash
# Trace all file-related syscalls of PID 74402
./process-output-mcp "strace -f -e %file -p 74402"
```

This lets an MCP client query which files a process is opening, creating, or modifying in real time.

### Follow a log file

Wrap `tail -f` to expose a log file's output via MCP:

```bash
# Stream a log file and make it queryable
./process-output-mcp "tail -f ./some-log.txt"
```

## Notes

- The server continues running after the wrapped process exits, so clients can still query captured output.
- Output is stored in memory (append-only). For very long-running processes with high output volume, consider periodic queries rather than accumulating indefinitely.
