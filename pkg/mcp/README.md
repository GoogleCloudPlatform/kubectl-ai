# MCP (Model Context Protocol) Client

This package provides functionality to interact with MCP (Model Context Protocol) servers from `kubectl-ai`.

## Overview

The MCP client allows `kubectl-ai` to connect to MCP servers, discover available tools, and execute them. This enables integration with various services and systems that expose their functionality through the MCP protocol.

## Features

- Connect to multiple MCP servers
- Automatic discovery of available tools from connected servers
- Execute tools on remote MCP servers
- Configuration-based server management
- Support for sequential-thinking server out of the box

## Configuration

MCP server configurations are stored in `~/.config/kubectl-ai/mcp.json`. If this file doesn't exist, a default configuration will be created automatically.

### Default Configuration

By default, the MCP client is configured to use a sequential-thinking server with the following settings:

```json
{
  "mcpServers": {
    "sequential-thinking": {
      "command": "npx",
      "args": [
        "-y",
        "@modelcontextprotocol/server-sequential-thinking"
      ]
    }
  }
}
```

### Configuration Format

The configuration file uses the following JSON structure:

```json
{
  "mcpServers": {
    "server-name": {
      "command": "path-to-server-binary",
      "args": ["--flag1", "value1"],
      "env": {
        "ENV_VAR": "value"
      }
    }
  }
}
```

## Usage

MCP servers are automatically discovered and used by the AI when needed. No manual interaction is required.

### Custom Server Example

To add a custom MCP server, edit the configuration file at `~/.config/kubectl-ai/mcp.json` and add a new server entry:

```json
{
  "mcpServers": {
    "custom-server": {
      "command": "/path/to/your/mcp-server",
      "args": ["--port", "8080"],
      "env": {
        "CUSTOM_VAR": "value"
      }
    }
  }
}
```

### Environment Variables

You can configure the following environment variables to customize MCP client behavior:

- `KUBECTL_AI_MCP_CONFIG`: Override the default configuration file path
- `MCP_<SERVER_NAME>_<ENV_VAR>`: Set environment variables for specific servers

## Implementation Details

### Client

The `Client` struct represents a connection to an MCP server. It provides methods to:
- Connect to the server
- List available tools
- Execute tools
- Close the connection

### Manager

The `Manager` struct manages multiple MCP client connections. It provides:
- Connection management for multiple servers
- Tool discovery across all connected servers
- Thread-safe operations

### Configuration

The `Config` struct handles loading and saving MCP server configurations from disk. The configuration is automatically loaded from `~/.config/kubectl-ai/mcp.json` when needed.

## Integration with kubectl-ai

The MCP client is integrated with `kubectl-ai` to automatically discover and use tools from configured MCP servers. The system will:

1. Load the MCP configuration on startup
2. Connect to all configured MCP servers
3. Make their tools available to the AI as needed
4. Automatically handle tool execution and result processing

## Security Considerations

- MCP servers can execute arbitrary commands with the same permissions as the `kubectl-ai` process
- Only connect to trusted MCP servers
- The configuration file has strict permissions (0600) by default
- Be cautious when adding environment variables with sensitive information

## Troubleshooting

- Use `-v=4` for debug logging to see MCP connection details
- Check the MCP server logs for connection issues
- Verify the MCP server is running and accessible
- Ensure the command path and arguments in the configuration are correct
