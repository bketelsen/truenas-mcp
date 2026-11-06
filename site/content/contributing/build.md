# Build from source

truenas-mcp is a Go module. This page names the toolchain, the build, and the
checks a pull request must pass.

## Toolchain

- Go, at the release in the `go` line of `go.mod` (1.26 or later).
- [mise](https://mise.jdx.dev/), for the pinned `golangci-lint` and Node.js.
  Run `mise install` in the clone.
- Node.js 22 or later, for the template checks.
- Python 3, only for this site.

## Build and check

```sh
make            # format, vet and build into build/truenas-mcp
make verify     # the gate without credentials: static checks, template checks and tests
make ci         # the CI gate: verify-static, race tests and cross-builds
make help       # list every target
```

`make build` stamps the binary with the git tag, the commit and the build time.
Run `make verify` before you call a change done. `make ci BASE=origin/main`
also checks that `.memory/corrections.jsonl` is append-only against the base.

The unit tests use an in-memory MCP client and a mock of the appliance. No test
talks to a real appliance.

## Build this site

The documentation site lives in `site/` and is built with ProperDocs and the
MaterialX theme:

```sh
make site        # strict build into site/dist
make site-serve  # preview at http://127.0.0.1:8000/truenas-mcp/
```

The site needs the Python lock in `site/requirements.txt` and writes its
virtual environment to `build/site-venv`. `make ci` does not build the site.
A strict build fails on a broken relative link or a missing anchor.

## Write for users

Pages live in `site/content/`. A page explains what a user does, not how the
code works: the maintainer contracts stay in
[`docs/`](https://github.com/bketelsen/truenas-mcp/tree/main/docs). A change
to user-visible behavior updates the affected page in the same pull request.

Commit messages follow Conventional Commits: the release version and the
changelog come from them. See
[CONTRIBUTING.md](https://github.com/bketelsen/truenas-mcp/blob/main/CONTRIBUTING.md).
