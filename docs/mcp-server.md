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

### Read-only MCP Server

Start a server that exposes only a restricted kubectl tool:

```bash
kubectl-ai --mcp-server --mcp-read-only
```

This mode does not register `bash`, custom tools, or external MCP tools. Before
each execution it independently classifies the actual kubectl command and
rejects mutations, local kubeconfig changes, compound commands, and ambiguous
operations. The client-provided `modifies_resource` argument is not trusted.

The boundary prevents writes; it does not decide which data is appropriate for
a caller to see. Use a dedicated Kubernetes service account and RBAC rules to
limit readable namespaces and resources. When using the HTTP transport, enable
OAuth authentication as described below.

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

This listens on `http://localhost:9080/mcp` by default.

> **Warning:** Without authentication, anyone who can reach this port can execute
> arbitrary `kubectl` and `bash` commands through the MCP protocol. When exposing
> the `streamable-http` endpoint beyond localhost, enable authentication (see
> below).

### Securing the HTTP Endpoint with OAuth 2.1 (AuthGate)

In `streamable-http` mode the MCP server can act as an OAuth 2.1 **Resource
Server**, requiring callers to present a valid `Authorization: Bearer <JWT>`
access token. Tokens are verified locally against the Authorization Server's
JWKS (signature + `iss` + `aud` + `exp`, and `type == access`). This integrates
with [go-authgate/authgate](https://github.com/go-authgate/authgate) as the
Authorization Server, but works with any OAuth 2.1 / OIDC server that publishes
a JWKS.

Authentication is **opt-in**: if `--mcp-auth-issuer` is not set, the endpoint
behaves exactly as before (no authentication, no metadata endpoint).

```bash
kubectl-ai --mcp-server --mcp-server-mode streamable-http --http-port 9080 \
  --mcp-auth-issuer https://authgate.corp \
  --mcp-auth-audience https://kubectl-ai.corp/mcp
```

What this enables:

- `POST /mcp` requires a valid Bearer access token. Missing or invalid tokens
  receive `401 Unauthorized` with a `WWW-Authenticate` header pointing at the
  Protected Resource Metadata document.
- `GET /.well-known/oauth-protected-resource` serves the Protected Resource
  Metadata (RFC 9728) — unauthenticated — so MCP clients can discover the
  Authorization Server automatically:

  ```json
  {
    "resource": "https://kubectl-ai.corp/mcp",
    "authorization_servers": ["https://authgate.corp"],
    "bearer_methods_supported": ["header"]
  }
  ```

Authentication flags:

| Flag                  | Description                                                                                                                               |
| --------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `--mcp-auth-issuer`   | Authorization Server base URL (= JWT `iss`). **Non-empty enables authentication.** Empty keeps the previous, unauthenticated behavior.    |
| `--mcp-auth-audience` | This server's resource identifier (expected JWT `aud`, e.g. `https://kubectl-ai.corp/mcp`). **Required** when `--mcp-auth-issuer` is set. |
| `--mcp-auth-jwks-url` | Optional override for the JWKS URL. Empty uses OIDC discovery from `<issuer>/.well-known/openid-configuration`.                           |

#### Why the audience (`aud`) matters

The `--mcp-auth-audience` value is this server's **resource identifier**. During
verification kubectl-ai requires the JWT's `aud` claim to match it exactly (in
addition to checking the signature, `iss`, and `exp`). This is what binds a token
to *this specific* MCP server, and it is why the flag is mandatory once
authentication is enabled — there is no safe default to guess.

The signature and `iss` checks only prove the token is *genuine* (really issued
by your Authorization Server). The `aud` check proves the token was *meant for
you*. Without it, **any** valid token signed by the same Authorization Server
would be accepted, which opens up:

- **Cross-service token reuse / confused deputy:** one Authorization Server
  typically issues tokens for many resource servers (other internal APIs,
  dashboards, microservices). A user holding a legitimate token for, say, a
  reporting API could replay it against the MCP server and gain the ability to
  run `kubectl` and `bash`. With `aud` enforced, that token's audience does not
  match this server's resource identifier, so it is rejected.
- **Token theft by a downstream service:** if a token is not bound to an
  audience, any service that receives it (or a compromised/man-in-the-middle
  hop) can turn around and impersonate the user against the MCP server. Binding
  the audience makes a token issued for someone else useless here.

The audience also closes the discovery loop: the value is published verbatim as
the `resource` field of the Protected Resource Metadata document (see below), so
compliant MCP clients request a token scoped to exactly this resource (per RFC
8707 Resource Indicators) and the server then enforces that same scope.

> **Pick a stable, unique value.** Use a URI that uniquely identifies this MCP
> server (commonly its public `/mcp` URL, e.g. `https://kubectl-ai.corp/mcp`).
> It must be the same string the Authorization Server stamps into the `aud`
> claim — a mismatch results in every request being rejected with `401`.

Behavior notes:

- **Fail-fast on configuration errors:** an invalid issuer URL or a missing
  audience aborts startup with a clear error.
- **Tolerant of a temporarily unreachable Authorization Server:** if the JWKS
  cannot be fetched at startup, kubectl-ai logs a warning and keeps retrying in
  the background instead of crashing.
- **Fail-closed while keys are unavailable:** until signing keys are loaded,
  `/mcp` returns `503 Service Unavailable` rather than letting any request
  through.
- **Reverse proxies:** the `resource_metadata` URL in the `WWW-Authenticate`
  header is built from `X-Forwarded-Proto` (falling back to TLS detection) and
  the request `Host`. Make sure your proxy forwards `X-Forwarded-Proto`.

> **Tip:** [go-authgate/authgate](https://github.com/go-authgate/authgate) can
> be used as the Authorization Server that issues and signs these access tokens.
> See its documentation for installation and client setup.

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

With `--mcp-read-only`, only the restricted `kubectl` tool is exposed.

### External Tools (when `--external-tools` is enabled)

Additional tools are available depending on the configured MCP servers:

- **Filesystem tools**: Read/write files, list directories
- **Web search tools**: Search the internet for information
- **Database tools**: Query databases
- **API tools**: Interact with external APIs
- **Custom tools**: Any MCP-compatible tools

## Command Line Options

| Flag                  | Default          | Description                                                                                                           |
| --------------------- | ---------------- | --------------------------------------------------------------------------------------------------------------------- |
| `--mcp-server`        | `false`          | Run in MCP server mode                                                                                                |
| `--mcp-read-only`     | `false`          | Expose only kubectl commands proven read-only (requires `--mcp-server`)                                               |
| `--external-tools`    | `false`          | Discover and expose external MCP tools (incompatible with `--mcp-read-only`)                                          |
| `--kubeconfig`        | `~/.kube/config` | Path to kubeconfig file                                                                                               |
| `--mcp-server-mode`   | `stdio`          | Transport for the MCP server (`stdio` or `streamable-http`)                                                           |
| `--http-port`         | `9080`           | Port for the HTTP endpoint when using `streamable-http` modes                                                         |
| `--mcp-auth-issuer`   | `""`             | OAuth 2.1 authorization server base URL (JWT issuer). Non-empty enables Bearer auth on the `streamable-http` endpoint |
| `--mcp-auth-audience` | `""`             | Expected JWT audience (this server's resource identifier). Required when `--mcp-auth-issuer` is set                   |
| `--mcp-auth-jwks-url` | `""`             | Optional override for the JWKS URL; empty uses OIDC discovery from the issuer                                         |

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
