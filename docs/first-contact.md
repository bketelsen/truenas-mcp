# First Contact with TrueNAS

Use this checklist the first time you connect `truenas-mcp` to a real TrueNAS SCALE system.

The default server mode is read-only. Keep it that way until you have inspected the tools and verified responses from your NAS.

## Safety Rules

- Do **not** pass `--enable-writes` during first contact.
- Unset `TRUENAS_ENABLE_WRITES` so writes stay disabled by default.
- Use a dedicated API key. If your TrueNAS version supports scoped or read-only API credentials, use the narrowest scope available.
- Prefer valid TLS certificates. Use `--tls-insecure` only when you knowingly accept self-signed certificate verification risk.
- First prompts should only list tools or call read-only report/list tools.
- Remember that read-only tools can still expose NAS metadata to the connected AI client and its logs.

## Build Locally

```bash
make build
```

Or use the guided first-contact script:

```bash
export TRUENAS_HOST=truenas.local
read -rs TRUENAS_API_KEY
export TRUENAS_API_KEY
scripts/first-contact.sh
```

The script requires `make`, `go`, and `python3`. It builds `build/truenas-mcp`
and prints that binary's absolute clone path in both the command shape and
the generated MCP client JSON. It does not contact TrueNAS unless you pass
`--run-server`, which executes the same binary. The generated JSON uses the
placeholder `YOUR_API_KEY`, never the supplied key. Do not run it with shell
tracing (`bash -x`) because tracing can expose secrets.

The offline guard `test/first-contact.test.mjs`, included in
`make verify-template`, executes the planning path with a stubbed `make`,
compares the generated command with the Makefile build output and checks
that a dummy key is not printed. It does not build or start the Server.
See the [design and test map](design/overview.md#ci).

## Environment

```bash
unset TRUENAS_ENABLE_WRITES
export TRUENAS_HOST=truenas.local
read -rs TRUENAS_API_KEY
export TRUENAS_API_KEY
```

If your appliance uses a self-signed certificate and you accept that risk for first contact:

```bash
export TRUENAS_TLS_INSECURE=true
```

## Run Read-Only

```bash
./build/truenas-mcp serve --host "$TRUENAS_HOST" --api-key "$TRUENAS_API_KEY"
```

Do not add `--enable-writes`.

To let the guided script start the read-only server after checks:

```bash
scripts/first-contact.sh --run-server
```

## Claude MCP Configuration

For a local clone, replace `/absolute/path/to/clone` below with the clone's
absolute directory. The executable is inside `build/`, not at the clone root.
For an installed binary, use its installed absolute path instead. The
[user first-connection guide](../site/content/getting-started/first-connection.md)
has the same launch steps.

```json
{
  "mcpServers": {
    "truenas": {
      "command": "/absolute/path/to/clone/build/truenas-mcp",
      "args": ["serve", "--host", "truenas.local", "--api-key", "YOUR_API_KEY"]
    }
  }
}
```

For self-signed certificates, add `--tls-insecure` only if needed:

```json
{
  "mcpServers": {
    "truenas": {
      "command": "/absolute/path/to/clone/build/truenas-mcp",
      "args": ["serve", "--host", "truenas.local", "--api-key", "YOUR_API_KEY", "--tls-insecure"]
    }
  }
}
```

## First Prompts

Start with tool discovery and read-only checks:

- “List the available TrueNAS tools.”
- “Run `truenas_health_report` and summarize the result.”
- “Run `truenas_apps_update_report`; do not update anything.”
- “List recent TrueNAS jobs with `truenas_jobs_list`.”

Avoid any prompt asking the assistant to create, delete, start, stop, restart, upgrade, or modify resources during first contact.

## Expected Read-Only Tools

Representative tools available by default include:

- `truenas_health_report`
- `truenas_system_info`
- `truenas_pool_list`
- `truenas_app_list`
- `truenas_apps_update_report`
- `truenas_jobs_list`
- `truenas_filesystem_stat`

Tools with names like `create`, `delete`, `start`, `stop`, `restart`, `configure`, or `dismiss` should not appear unless you intentionally started the server with `--enable-writes` or `TRUENAS_ENABLE_WRITES=true`.
