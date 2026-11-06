# Configuration

truenas-mcp has 1 command, `serve`, and takes its settings from flags or from
environment variables. A flag that is set wins over its variable.

## Commands

| Command | What it does |
| --- | --- |
| `truenas-mcp serve` | Connect to the appliance, sign in with the API key, and serve MCP over stdio. |
| `truenas-mcp completion <shell>` | Print the completion script for `bash`, `zsh`, `fish` or `powershell`. |
| `truenas-mcp --version` | Print the version and the commit. |
| `truenas-mcp --help` | Print the usage. |

## `serve` settings

| Flag | Environment variable | Default | Meaning |
| --- | --- | --- | --- |
| `--host` | `TRUENAS_HOST` | none, required | The appliance host name or IP address, for example `truenas.local`. Not a URL. |
| `--api-key` | `TRUENAS_API_KEY` | none, required | The TrueNAS API key the server signs in with. |
| `--enable-writes` | `TRUENAS_ENABLE_WRITES` | off | Register the write tools. See [read-only by default](../concepts/read-only.md). |
| `--tls-insecure` | `TRUENAS_TLS_INSECURE` | off | Skip the TLS certificate check. Use it only for a self-signed certificate whose risk you accept. |

A boolean variable is on for `1`, `t`, `true`, `y`, `yes` or `on`, and off
for `0`, `f`, `false`, `n`, `no` or `off`, in any of the case forms in
[read-only by default](../concepts/read-only.md#which-values-turn-writes-on).
Any other value keeps the default.

When `--host` or `--api-key` has no value, `serve` exits with an error that
names the flag and its variable.

!!! note "Keep the API key off the command line"

    A value in `--api-key` is visible to other users of the machine in the
    process list, and stays in your shell history. Prefer
    `TRUENAS_API_KEY`, read with `read -rs` or set by your MCP client.

## Connect an MCP client

An MCP client starts the server as a child process and speaks MCP over its
stdin and stdout. For Claude Code, and for clients that use the same
`mcpServers` format, the entry is:

```json
{
  "mcpServers": {
    "truenas": {
      "command": "/absolute/path/to/truenas-mcp",
      "args": ["serve"],
      "env": {
        "TRUENAS_HOST": "truenas.local",
        "TRUENAS_API_KEY": "<your API key>"
      }
    }
  }
}
```

The client's configuration file then holds the API key: keep its permissions
tight. Add `"--enable-writes"` or `"--tls-insecure"` to `args` only when you
need them.

## Connection behavior

- The server connects to `wss://<host>/api/current` and signs in with the API
  key.
- TLS certificate verification is on by default.
- Before each call, the server checks the connection with `core.ping`. When
  the check fails, it connects and signs in again.
- Calls are serialized: 1 call to the appliance at a time.
- The server never sends a call again after a transport error, because a
  write may already have taken effect. It reports the error, and the next
  call opens a fresh connection.

## Supported TrueNAS releases

truenas-mcp supports TrueNAS SCALE Goldeye (25.10) and later. The
[API audit](https://github.com/bketelsen/truenas-mcp/blob/main/docs/specs/api-audit.md)
checked the middleware methods of the tools against a live TrueNAS
27.0.0-RC.1 on 2026-10-22. Methods that later tools added carry their own
check and its date in the audit, live or against the middleware source. The
audit also lists the fallbacks for older releases.
