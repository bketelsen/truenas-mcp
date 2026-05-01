# Contributing to truenas-mcp

Thanks for helping improve `truenas-mcp`. This project exposes TrueNAS SCALE management through MCP, so safety matters: read-only behavior should stay the default, and write-capable changes need extra care.

## Before You Start

- Check existing issues and pull requests to avoid duplicate work.
- For bugs, include your TrueNAS SCALE version, `truenas-mcp` commit/version, OS, and a minimal reproduction.
- Do not include API keys, hostnames you consider private, full NAS inventory, or other secrets in public issues.

## Local Setup

Requirements:

- Go 1.26+
- `golangci-lint` for local linting

Build:

```bash
make build
```

Run tests:

```bash
make test
```

Run the recommended local gates before a PR:

```bash
make all   # formats, vets, and builds
make test
make lint
```

For first contact with a real TrueNAS system, use the read-only guide:

```bash
scripts/first-contact.sh
```

See [`docs/first-contact.md`](docs/first-contact.md) for details.

## Pull Request Expectations

A good PR includes:

- a clear summary of the change
- tests for new behavior or a short explanation when tests do not apply
- local verification commands run before pushing
- safety notes for anything that touches TrueNAS writes, authentication, TLS, or exposed metadata

Write-capable tools must remain opt-in behind `--enable-writes` / `TRUENAS_ENABLE_WRITES=true` and should fail closed by default.

## Project Style

- Keep tool output structured and easy for MCP clients to summarize.
- Prefer small, focused PRs.
- Keep docs honest: do not promise support, compatibility, or security response times unless they are actually available.
