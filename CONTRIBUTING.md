# Contributing to truenas-mcp

Thanks for helping improve `truenas-mcp`. This project exposes TrueNAS SCALE management through MCP, so safety matters: read-only behavior should stay the default, and write-capable changes need extra care.

## Before You Start

- Check existing issues and pull requests to avoid duplicate work.
- For bugs, include your TrueNAS SCALE version, `truenas-mcp` commit/version, OS, and a minimal reproduction.
- Do not include API keys, hostnames you consider private, full NAS inventory, or other secrets in public issues.

## Local Setup

Requirements:

- Go 1.26+ (the version in `go.mod`)
- [mise](https://mise.jdx.dev) to install the pinned `golangci-lint` from `mise.toml`
- [svu](https://github.com/caarlos0/svu) and [GoReleaser Pro](https://goreleaser.com/pro) only if you cut releases

Install the pinned tools:

```bash
mise install
```

Build (output goes to `build/truenas-mcp`):

```bash
make build
```

Run tests:

```bash
make test
```

Run the recommended local gates before a PR:

```bash
make verify   # tidy diff, vet, gofmt, pinned golangci-lint, tests
```

`make ci` runs the same gate plus the race detector and cross-builds. `make help` lists every target.

`make lint` refuses to run unless the installed `golangci-lint` matches the pin in `mise.toml`, so local results match CI. Bump the pin in `mise.toml` in its own commit and run `mise install` to refresh `mise.lock`.

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

## Commit Messages

Use [Conventional Commits](https://www.conventionalcommits.org) (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`; add a scope such as `feat(app):` when it helps). The release changelog is grouped by these prefixes, and `svu` derives the next version from them: `feat` bumps minor, `fix` bumps patch, and a `!` or `BREAKING CHANGE:` footer bumps major.

## Releasing

Releases are tagged from a clean `main` checkout with:

```bash
make bump
```

This builds, tests, formats, and lints, refuses to continue if the working tree is dirty, then tags the version `svu next` computes and pushes the tag. The push triggers the `goreleaser` workflow, which builds the binaries and packages, publishes the GitHub release with a generated changelog, and attaches build provenance attestations.

Every successful CI run on `main` also republishes the `dev` pre-release from that commit via the `snapshot` workflow.

To try the release build locally without publishing, run `make snapshot` (needs `goreleaser-pro` and `GORELEASER_KEY`); the output lands in `dist/`.

## Project Style

- Keep tool output structured and easy for MCP clients to summarize.
- Prefer small, focused PRs.
- Keep docs honest: do not promise support, compatibility, or security response times unless they are actually available.
