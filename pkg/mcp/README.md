# MCP (Model Context Protocol) Client

This package provides functionality to interact with MCP (Model Context Protocol) servers from `kubectl-ai`.

## Overview

The MCP client allows `kubectl-ai` to connect to MCP servers, discover available tools, and execute them. This enables integration with various services and systems that expose their functionality through the MCP protocol.

## Features

- Connect to multiple MCP servers simultaneously
- Support for both local (stdio-based) and remote (HTTP-based) MCP servers
- Authentication support for HTTP-based servers (Basic, Bearer Token, API Key)
- Automatic discovery of available tools from connected servers
- Execute tools on MCP servers with parameter conversion
- Configuration-based server management
- Generic parameter name and type conversion (snake_case → camelCase, intelligent type inference)
- Synchronous initialization ensuring tools are available before conversation starts

## Configuration

MCP server configurations are stored in `~/.config/kubectl-ai/mcp.yaml`. If this file doesn't exist, a default configuration will be created automatically.

### Default Configuration

By default, the MCP client is configured with sequential thinking MCP server:

```yaml
servers:
  - name: sequential-thinking
    command: npx
    args:
      - -y
      - "@modelcontextprotocol/server-sequential-thinking"
```

### Configuration Format

The configuration file uses YAML format and supports both local (stdio-based) and remote (HTTP-based) MCP servers:

#### Local (stdio-based) Server Configuration

```yaml
servers:
  - name: server-name
    command: path-to-server-binary
    args:
      - --flag1
      - value1
    env:
      ENV_VAR: value
```

#### Remote (HTTP-based) Server Configuration

```yaml
servers:
  - name: remote-server
    url: "https://mcp-server.example.com/"
    timeout: 30  # Optional: Timeout in seconds
    use_streaming: true  # Optional: Use streaming HTTP client
    # Optional authentication
    auth:
      type: "bearer"  # Options: "basic", "bearer", "api-key"
      token: "${YOUR_ENV_VAR}"  # Will be read from YOUR_ENV_VAR environment variable
    # Optional custom headers
    headers:
      X-Custom-Header: "custom-value"
      X-API-Version: "v1"
```

### Authentication Options

Remote MCP servers support different authentication methods:

1. **Bearer Token**:

   ```yaml
   auth:
     type: "bearer"
     token: "your-bearer-token"
   ```

2. **Basic Authentication**:

   ```yaml
   auth:
     type: "basic"
     username: "username"
     password: "password"
   ```

3. **API Key**:

   ```yaml
   auth:
     type: "api-key"
     api_key: "your-api-key"
     header_name: "X-Api-Key"  # Optional: Defaults to X-Api-Key
   ```

### Custom Headers

Remote MCP servers support custom HTTP headers for additional configuration or authentication requirements:

```yaml
servers:
  - name: remote-server
    url: "https://mcp-server.example.com/"
    headers:
      X-Custom-Header: "custom-value"
      X-API-Version: "v1"
      User-Agent: "kubectl-ai/1.0"
      Accept-Language: "en-US"
```

**Key Points:**

- Custom headers are applied to all HTTP requests sent to the MCP server
- Headers can be combined with authentication methods
- Authentication headers (e.g., `Authorization`) may override custom headers if both are specified
- All header values are strings
- Headers are case-sensitive as per HTTP specification

**Common Use Cases:**

- API versioning headers (e.g., `X-API-Version`)
- Custom user agent strings
- Request tracking headers (e.g., `X-Request-ID`)
- Content negotiation headers (e.g., `Accept`, `Accept-Language`)

### Environment Variable Support

Sensitive information like tokens and passwords can be read from environment variables using the `${VAR_NAME}` syntax in the configuration file. You can also set environment variables with the prefix `MCP_SERVER_NAME_` to override configuration values.

## Usage

Enable MCP client functionality with the `--mcp-client` flag:

```bash
kubectl-ai --mcp-client
```

### Checking Server Status

When you run kubectl-ai with the MCP client enabled, you'll see information about connected servers:

```txt
MCP Server Status:

Successfully connected to 2 MCP server(s) (2 tools discovered)

• sequential-thinking (npx) - Connected, Tools: sequentialthinking
• fetch (remote) - Connected, Tools: fetch  
```

MCP servers are automatically discovered and their tools made available to the AI. The system handles:

- **Parameter conversion**: Automatically converts snake_case parameters to camelCase
- **Type inference**: Intelligently converts string parameters to numbers/booleans based on naming patterns
- **Error handling**: Graceful fallbacks for connection issues

### Custom Server Examples

To add custom MCP servers, edit the configuration file at `~/.config/kubectl-ai/mcp.yaml`:
You can combine both local and remote servers in your configuration:

```yaml
servers:
  - name: sequential-thinking
    command: npx
    args:
      - -y
      - '@modelcontextprotocol/server-sequential-thinking'
  - name: cloudflare-documentation
    url: https://docs.mcp.cloudflare.com/mcp
```

### Environment Variables

You can configure the following environment variables to customize MCP client behavior:

- `KUBECTL_AI_MCP_CONFIG`: Override the default configuration file path
- `MCP_<SERVER_NAME>_<ENV_VAR>`: Set environment variables for specific servers

## Parameter Conversion

The MCP client automatically handles parameter name and type conversion to ensure compatibility with different MCP servers:

### Name Conversion

- Converts snake_case parameter names to camelCase
- Example: `thought_number` → `thoughtNumber`

### Type Conversion

Parameters are intelligently converted based on naming patterns:

**Numbers:** Parameters containing `number`, `count`, `total`, `max`, `min`, `limit`
**Booleans:** Parameters starting with `is`, `has`, `needs`, `enable` or containing `required`, `enabled`

### Fallback Behavior

- If type conversion fails, the original value is preserved
- Unknown servers use generic conversion rules
- No configuration required - works automatically with any MCP server

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

The `Config` struct handles loading and saving MCP server configurations from disk. The configuration is automatically loaded from `~/.config/kubectl-ai/mcp.yaml` when needed.

## Integration with kubectl-ai

The MCP client is integrated with `kubectl-ai` to automatically discover and use tools from configured MCP servers. The system:

1. **Loads configuration** from `~/.config/kubectl-ai/mcp.yaml` on startup
2. **Connects synchronously** to all configured MCP servers (when `--mcp-client` flag is used)
3. **Registers tools** before the conversation starts, ensuring they're immediately available
4. **Converts parameters** automatically using generic snake_case → camelCase conversion
5. **Handles execution** with proper error handling and result formatting
6. **Displays status** showing connected servers and available tool counts

📖 **For practical multi-server orchestration examples and security automation workflows, see the [MCP Client Integration Guide](../../docs/mcp-client.md).**

## Security Considerations

- MCP servers can execute arbitrary commands with the same permissions as the `kubectl-ai` process
- Only connect to trusted MCP servers
- The configuration file has strict permissions (0600) by default
- Be cautious when adding environment variables with sensitive information

## Troubleshooting

### Common Issues

**MCP tools are not available:**

- Ensure you're using the `--mcp-client` flag
- Check that `~/.config/kubectl-ai/mcp.yaml` exists and is valid (created by default)
- Verify MCP servers are installed (e.g., `npx` commands work)

**Connection failures:**

- Check network connectivity
- Ensure server commands and paths are correct in configuration
- Verify environment variables are properly set

**Parameter conversion issues:**

- The system automatically converts snake_case → camelCase
- String parameters are converted to numbers/booleans based on naming patterns
- Fallback behavior preserves original values if conversion fails

### Debug Information

- Use `-v=1` for basic MCP operation logging
- Use `-v=2` for detailed connection and tool discovery info  
- Check server status in the startup message
- Tool counts are displayed for each connected server
