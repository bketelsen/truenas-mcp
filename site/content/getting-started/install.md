# Install truenas-mcp

truenas-mcp is 1 binary. It runs on Linux, macOS and Windows, on amd64 and
arm64. This page names the requirements, then installs truenas-mcp from a
release or from source.

## Requirements

- A TrueNAS SCALE appliance that runs Goldeye (25.10) or later.
- A TrueNAS API key for the server to sign in with. TrueNAS checks every call
  against the roles of the user that owns the key, so give that user only the
  roles you need.
- An MCP client that starts servers over stdio, such as Claude Code.
- Go 1.26 or later, only to build from source.

## Install from a release

Every [GitHub release](https://github.com/bketelsen/truenas-mcp/releases)
has these assets:

| Asset | Platforms |
| --- | --- |
| `truenas-mcp_<version>_<os>_<arch>.tar.gz` | Linux and macOS, amd64 and arm64 |
| `truenas-mcp_<version>_windows_<arch>.zip` | Windows, amd64 and arm64 |
| `.deb`, `.rpm` and `.apk` packages | Linux, amd64 and arm64 |
| `checksums.txt` | The SHA-256 sum of every asset |

The packages also install shell completions for bash, zsh and fish, and the
`truenas-mcp(1)` man page. The `dev` pre-release tracks the latest commit on
`main`.

=== "Archive"

    ```sh
    tar -xzf truenas-mcp_*_linux_amd64.tar.gz truenas-mcp
    install -m 0755 truenas-mcp ~/.local/bin/
    ```

=== "Debian or Ubuntu"

    ```sh
    sudo apt install ./truenas-mcp_*_amd64.deb
    ```

=== "Fedora"

    ```sh
    sudo dnf install ./truenas-mcp-*.x86_64.rpm
    ```

### Verify a download

Every release asset is signed with GitHub build provenance. Verify an asset
with the GitHub CLI before you install it:

```sh
gh attestation verify truenas-mcp_*_linux_amd64.tar.gz --repo bketelsen/truenas-mcp
```

## Install from source

The module path is `truenas-mcp`, so `go install <url>@latest` does not work.
Clone the repository and use `make`:

```sh
git clone https://github.com/bketelsen/truenas-mcp.git
cd truenas-mcp
make install   # installs into $GOPATH/bin/truenas-mcp
```

`make build` puts the binary in `build/truenas-mcp` instead. See
[build from source](../contributing/build.md) for the checks.

!!! note "Check the version"

    Run `truenas-mcp --version` after the install. It prints the version and
    the commit the binary was built from, for example
    `truenas-mcp version 0.6.0 (3c6479d)`.

## Next step

Continue with the [first connection](first-connection.md). It connects the
server to your appliance in read-only mode.
