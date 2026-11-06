# First connection

Use this checklist the first time you connect truenas-mcp to a real TrueNAS
SCALE appliance. The server starts read-only. Keep it read-only until you have
checked the tool results against your appliance.

## Safety rules

- Do not pass `--enable-writes`, and unset `TRUENAS_ENABLE_WRITES`.
- Use a dedicated API key. When your TrueNAS release supports scoped or
  read-only credentials, use the narrowest scope it offers.
- Prefer a valid TLS certificate. Use `--tls-insecure` only when you accept
  the risk of a self-signed certificate.
- Start with prompts that list tools or call read-only list and report tools.
- Read-only tools still send appliance metadata to the AI client and its logs.
  `truenas_app_config` can return plaintext secrets from an app's
  configuration.

## Step 1: set the environment

Read the API key without echo, so that it stays out of your shell history:

```sh
unset TRUENAS_ENABLE_WRITES
export TRUENAS_HOST=truenas.local
read -rs TRUENAS_API_KEY
export TRUENAS_API_KEY
```

`TRUENAS_HOST` is a host name or an IP address, not a URL. The server
connects to `wss://<host>/api/current`.

If the appliance uses a self-signed certificate and you accept that risk:

```sh
export TRUENAS_TLS_INSECURE=true
```

## Step 2: run the server read-only

```sh
truenas-mcp serve
```

The server reads `TRUENAS_HOST` and `TRUENAS_API_KEY` from the environment.
It writes its status to stderr:

```text
Connecting to TrueNAS at truenas.local...
Connected and authenticated.
Starting MCP server on stdio (read-only mode)...
```

The server then waits for an MCP client on stdin. Stop it with ++ctrl+c++. An
MCP client starts the server itself; the [configuration](../reference/configuration.md#connect-an-mcp-client)
page shows the client entry.

=== "From a clone"

    The repository has a guided script. It builds the server, checks that
    writes are off and that `TRUENAS_HOST` is not a URL, and prints the
    read-only command and the client configuration. It never prints the API
    key, and it contacts the appliance only with `--run-server`.

    ```sh
    scripts/first-contact.sh              # check, build and print the runbook
    scripts/first-contact.sh --run-server # start the read-only server
    ```

    The script needs `make`, `go` and `python3`. Do not run it with shell
    tracing (`bash -x`): a trace can show the API key.

## Step 3: ask read-only questions

Start with tool discovery and read-only checks:

- "List the available TrueNAS tools."
- "Run `truenas_health_report` and summarize the result."
- "Run `truenas_apps_update_report`; do not update anything."
- "List recent TrueNAS jobs with `truenas_jobs_list`."

Do not ask the assistant to create, delete, start, stop, restart, upgrade or
change anything during the first connection.

## Step 4: check the tool list

The read-only server registers these tools, among others:

- `truenas_health_report`
- `truenas_system_info`
- `truenas_pool_list`
- `truenas_app_list`
- `truenas_apps_update_report`
- `truenas_jobs_list`

The read-only server registers exactly 21 tools. No write tool must appear:
no tool that creates, deletes, starts, stops, restarts, updates, configures,
runs or dismisses something. If one appears, the server runs with writes
enabled: stop it and check `TRUENAS_ENABLE_WRITES`. The
[tool reference](../reference/tools.md) marks each write tool.

## Undo it

The read-only server changes nothing on the appliance. To end the first
connection, stop the server and remove the API key in the TrueNAS web
interface.

When the read-only results look right, read
[enable write tools](../guides/enable-writes.md) before you turn writes on.
