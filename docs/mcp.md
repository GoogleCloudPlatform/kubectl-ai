# MCP server

`kubectl-ai` implements MCP server for accessing `kubectl` tool on local machine.

## Claude integration

Here is an example Claude configuration for MacOS:

```json
{
  "mcpServers": {
    "kubectl-ai": {
      "command": "/usr/local/bin/kubectl-ai",
      "args": ["--kubeconfig", "~/.kube/config", "--mcp-server"],
      "env": {
        "PATH": "/Users/<your-user-name>/work/google-cloud-sdk/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
        "HOME": "/Users/<your-user-name>"
      }
    }
  }
}
```