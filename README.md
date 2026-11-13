<!-- readme-kind: tool -->
# truenas-mcp

[![CI](https://github.com/bketelsen/truenas-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/bketelsen/truenas-mcp/actions/workflows/ci.yml)

**Give an AI client careful access to your TrueNAS SCALE appliance.**

truenas-mcp is an MCP server for people who run TrueNAS SCALE and want their
AI assistant to work with the appliance: storage, sharing, system and app
management, as tools over stdio. It starts read-only. The 20 tools that change
the appliance exist only with `--enable-writes` or
`TRUENAS_ENABLE_WRITES=true`, so a plain `serve` cannot create, change or
delete anything.

**Status:** pre-release. The appliance must run TrueNAS SCALE Goldeye (25.10) or later.

**Documentation:** [bketelsen.github.io/truenas-mcp](https://bketelsen.github.io/truenas-mcp/)

## What it does

- **Read-only by default.** The default mode registers the 23 read-only tools
  only. A write tool that is not registered does not exist for the client, so
  it cannot be called by mistake. [Read-only by default](https://bketelsen.github.io/truenas-mcp/concepts/read-only/)
- **One view of the appliance.** Pools, datasets, snapshots, shares, alerts,
  apps and jobs, with the parameters and the safety class of every tool on 1
  page. [Tools](https://bketelsen.github.io/truenas-mcp/reference/tools/)
- **Reports that combine queries.** `truenas_health_report` answers with
  system, pool, disk and alert state; `truenas_protection_report` names each
  dataset that no snapshot task covers. [Tools](https://bketelsen.github.io/truenas-mcp/reference/tools/)
- **Writes ask for consent.** Write tools carry MCP annotations that mark the
  destructive ones, and several show a dry run until you pass `confirm` or the
  hash of the preview. [Enable write tools](https://bketelsen.github.io/truenas-mcp/guides/enable-writes/)
- **No replay of a write.** The server checks the connection before each call
  and reconnects, but it never sends a call again after a transport error,
  because the write may already have taken effect. [Configuration](https://bketelsen.github.io/truenas-mcp/reference/configuration/)

## Install

```sh
gh release download --repo bketelsen/truenas-mcp --pattern 'truenas-mcp_*_linux_amd64.tar.gz'
tar -xzf truenas-mcp_*_linux_amd64.tar.gz truenas-mcp
install -m 0755 truenas-mcp ~/.local/bin/
```

[Install](https://bketelsen.github.io/truenas-mcp/getting-started/install/) lists the
`.deb`, `.rpm` and `.apk` packages, the macOS and Windows assets, the build
from source, and how to verify a download with `gh attestation verify`.

## Use

```sh
export TRUENAS_HOST=truenas.example.org
read -rs TRUENAS_API_KEY
export TRUENAS_API_KEY
truenas-mcp serve
```

The server speaks MCP over stdio, so an MCP client normally starts it. Ask the
assistant for `truenas_health_report`, or for `truenas_protection_report` on
one pool, for a first result. Add `--enable-writes` only when you need writes.
The [configuration](https://bketelsen.github.io/truenas-mcp/reference/configuration/)
page has the client entry and every flag; [first connection](https://bketelsen.github.io/truenas-mcp/getting-started/first-connection/)
is the safe checklist for a real appliance.

## Requirements

A TrueNAS SCALE appliance on Goldeye (25.10) or later, and a TrueNAS API
key, created in the appliance's web interface, for a user with the roles you
plan to use. The server is 1 binary for Linux, macOS and Windows, on amd64 and
arm64, and an MCP client starts it over stdio.

## How it works

The server signs in to `wss://<host>/api/current` with the API key and calls
the appliance's WebSocket JSON-RPC methods; each tool is 1 or more of those
calls. The [API audit](docs/specs/api-audit.md) records every method, the
payload each tool sends, and the fallbacks that keep older releases working.
The [design](docs/design/overview.md) covers the connection, the tool
registration, the annotations and the tests.

## Develop

```sh
mise install   # the pinned golangci-lint and Node.js
make ci        # the gate CI runs
```

[Build from source](https://bketelsen.github.io/truenas-mcp/contributing/build/)
names the other targets and the test approach.

## Where to go next

- [AGENTS.md](AGENTS.md): the rules for agents and contributors.
- [docs/README.md](docs/README.md): the decisions, designs, specs and plans.
- [SECURITY.md](SECURITY.md): report a vulnerability privately.
- [LICENSE](LICENSE): MIT.
