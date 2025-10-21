# kubectl-ai MCP Server

kubectl-ai can run as an MCP (Model Context Protocol) server, exposing kubectl-ai tools to other MCP clients. The server can run in two modes:

1. **Built-in tools only**: Exposes only kubectl-ai's native tools
2. **External tool discovery**: Additionally discovers and exposes tools from other MCP servers

## Quick Start

### Basic MCP Server (Built-in tools only)

Start the MCP server with only kubectl-ai's built-in tools:

```bash
kubectl-ai --mcp-server
```

### Enhanced MCP Server (With external tool discovery)

Start the MCP server with external MCP tool discovery enabled:

```bash
kubectl-ai --mcp-server --external-tools
```

### Expose an HTTP Endpoint for MCP Clients

Run the server with the streamable HTTP transport to serve compatible MCP clients (including kubectl-ai MCP client mode) over HTTP:

```bash
kubectl-ai --mcp-server --mcp-server-mode streamable-http --http-port 9080
```

This listens on `http://localhost:9080/mcp` by default. Use `--mcp-server-mode sse` for legacy HTTP+SSE clients.

## Configuration

When `--external-tools` is enabled, the enhanced MCP server will automatically discover and expose tools from configured MCP servers. You can configure MCP servers using the standard MCP client configuration file.

### Example MCP Configuration

Create `~/.config/kubectl-ai/mcp.yaml`:

```yaml
servers:
  filesystem:
    command: "npx"
    args:
      [
        "-y",
        "@modelcontextprotocol/server-filesystem",
        "/path/to/allowed/files",
      ]

  brave-search:
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-brave-search"]
    env:
      BRAVE_API_KEY: "your-api-key"
```

## Features

### Tool Aggregation

When external tool discovery is enabled with `--external-tools`, the kubectl-ai MCP server acts as a **tool aggregator**, providing:

- All kubectl-ai built-in tools (kubectl, cluster analysis, etc.)
- Tools from external MCP servers (filesystem, web search, etc.)
- Unified interface for all tools through a single MCP endpoint

### Graceful Degradation

The server handles external MCP connection failures gracefully:

- If external MCP servers are unavailable, the server continues with built-in tools only
- Individual tool failures don't affect the overall server operation
- Clear logging for troubleshooting connection issues

### Example Usage in Claude Desktop

Configure Claude Desktop to use kubectl-ai as an MCP server:

**Basic usage (built-in tools only):**

```json
{
  "mcpServers": {
    "kubectl-ai": {
      "command": "kubectl-ai",
      "args": ["--mcp-server"]
    }
  }
}
```

**Enhanced usage (with external tools):**

```json
{
  "mcpServers": {
    "kubectl-ai": {
      "command": "kubectl-ai",
      "args": ["--mcp-server", "--external-tools"]
    }
  }
}
```

## Available Tools

### Built-in Tools

kubectl-ai provides the following native tools:

- `bash`: Executes a bash command. Use this tool only when you need to execute a shell command.
- `kubectl`: Executes a kubectl command against the user's Kubernetes cluster. Use this tool only when you need to query or modify the state of the user's Kubernetes cluster.

### External Tools (when `--external-tools` is enabled)

Additional tools are available depending on the configured MCP servers:

- **Filesystem tools**: Read/write files, list directories
- **Web search tools**: Search the internet for information
- **Database tools**: Query databases
- **API tools**: Interact with external APIs
- **Custom tools**: Any MCP-compatible tools

## Command Line Options

| Flag                | Default          | Description                                                            |
| ------------------- | ---------------- | ---------------------------------------------------------------------- |
| `--mcp-server`      | `false`          | Run in MCP server mode                                                 |
| `--external-tools`  | `false`          | Discover and expose external MCP tools (requires --mcp-server)         |
| `--kubeconfig`      | `~/.kube/config` | Path to kubeconfig file                                                |
| `--mcp-server-mode` | `stdio`          | Transport for the MCP server (`stdio`, `sse`, or `streamable-http`)    |
| `--http-port`       | `9080`           | Port for the HTTP endpoint when using `sse` or `streamable-http` modes |

## Architecture

```txt
┌─────────────────┐    ┌───────────────────┐    ┌─────────────────┐
│   MCP Client    │───▶│ kubectl-ai Server │───▶│ External Tools  │
│  (Claude, etc.) │    │                   │    │ (filesystem,    │
│                 │    │ ┌───────────────┐ │    │  web search,    │
│                 │    │ │ Built-in      │ │    │  etc.)          │
│                 │    │ │ kubectl tools │ │    │                 │
│                 │    │ └───────────────┘ │    │                 │
└─────────────────┘    └───────────────────┘    └─────────────────┘
```

The kubectl-ai MCP server acts as both:

- An **MCP Server** (exposing tools to clients)
- An **MCP Client** (consuming tools from other servers, when `--external-tools` is enabled)

This creates a powerful tool aggregation pattern where kubectl-ai becomes a central hub for both Kubernetes operations and general-purpose tools.

## Troubleshooting

### External Tools Not Available

If external tools aren't appearing:

1. Ensure you're using both `--mcp-server` and `--external-tools` flags
2. Check MCP configuration file exists and is valid
3. Verify external MCP servers are working independently
4. Check kubectl-ai logs for connection errors
5. Try running with external tools disabled to isolate issues

### Performance Considerations

- Tool discovery adds startup time (usually 2-3 seconds) when `--external-tools` is enabled
- Each external tool call has network overhead
- Consider running without `--external-tools` for faster startup if external tools aren't needed

### Debugging

Enable verbose logging to troubleshoot:

```bash
kubectl-ai --mcp-server --external-tools -v=2
```

This will show:

- MCP server connection attempts
- Tool discovery results
- Tool call routing decisions
