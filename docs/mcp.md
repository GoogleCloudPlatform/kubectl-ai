# `kubectl-ai` as a MCP Server

`kubectl-ai` can ack as a [Model Context Protocol (MCP)](https://github.com/modelcontextprotocol) server. This allows AI agents and IDEs to act as MCP clients and connect to the `kubectl-ai` MCP server, effectively interacting with your local Kubernetes environment via `kubectl`.

## Overview

The MCP server integrated into `kubectl-ai` exposes the power of `kubectl` commands as "tools" that MCP-compatible clients can invoke. When an AI agent needs to interact with your Kubernetes cluster, it can use these tools through the MCP server.

Currently, the server primarily supports exposing `kubectl` commands as tools. This means a client can request the server to run a `kubectl` command (like `get pods`, `describe deployment`, etc.), and the server will execute it and return the output.

## Using with MCP Clients

### Claude

Here is an example Claude configuration for MacOS:

```json
{
  "mcpServers": {
    "kubectl-ai": {
      // Find the right path by running `which kubectl-ai`
      "command": "/usr/local/bin/kubectl-ai",
      // The `--kubeconfig` argument can often be omitted if your `kubectl` is already configured to point to the desired cluster
      "args": ["--kubeconfig", "~/.kube/config", "--mcp-server"],
      "env": {
        "PATH": "/Users/<your-user-name>/work/google-cloud-sdk/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
        "HOME": "/Users/<your-user-name>"
      }
    }
  }
}
```

### Cursor

Cursor also supports MCP servers. You can [configure `kubectl-ai` MCP Server](https://docs.cursor.com/context/model-context-protocol#configuring-mcp-servers). The `mcp.json` file should look like this:

```json
{
  "mcpServers": {
    "kubectl-ai": {
      // Find the right path by running `which kubectl-ai`
      "command": "/usr/local/bin/kubectl-ai",
      // The `--kubeconfig` argument can often be omitted if your `kubectl` is already configured to point to the desired cluster
      "args": ["--mcp-server", "~/.kube/config", "--mcp-server"],
      "env": {
        // Define specific environment variables needed
      }
    }
  }
}
```

## Demo

*(Coming Soon)*

## Troubleshooting

*   `kubectl-ai` not found:
    *   Ensure the `command` path in your MCP client's configuration (`Claude`, `Cursor`, etc.) points to the correct location of the `kubectl-ai` executable.
    *   Verify that `kubectl-ai` is in your system's `PATH` if you are not using an absolute path.
*   Incorrect Kubernetes Context:
    *   `kubectl-ai` uses the same mechanisms as `kubectl` to determine the active cluster and namespace. This usually comes from your `~/.kube/config` file or `KUBECONFIG` environment variable.
    *   Ensure your `kubectl` context is set correctly *before* the MCP client tries to use the tool.
*   Permission Issues:
    *   The `kubectl-ai` tool runs with the permissions of the user who started the MCP client.
    *   Ensure this user has the necessary RBAC permissions in the Kubernetes cluster to perform the actions requested by the MCP client.
*   Client Specific Issues:
    *   Refer to the documentation of your specific MCP client (Claude, Cursor, etc.) for troubleshooting steps related to their MCP implementation.
    *   Check the client's logs for any error messages related to MCP communication.
*   Error Logs:
    *   `kubectl-ai` logs errors. If it's launched via an MCP client, refer to the documentation of your specific MCP client to see how to access the logs directly. You can also run `kubectl-ai --mcp-server -v=X` manually (where X is a log level) to see if it reports any issues when a client tries to connect or send a command.