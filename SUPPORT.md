# Support

`truenas-mcp` is a community project for exposing TrueNAS SCALE capabilities through MCP.

## Where to Ask

- Use GitHub Issues for reproducible bugs and feature requests.
- Use pull requests for proposed fixes and improvements.
- For security-sensitive reports, see [`SECURITY.md`](SECURITY.md) instead of opening a detailed public issue.

## Before Opening an Issue

Please include:

- `truenas-mcp` commit or version
- TrueNAS SCALE version
- operating system and Go version, if building locally
- command or MCP client configuration, with API keys removed
- expected behavior and actual behavior
- relevant logs with secrets, hostnames, and NAS-private details redacted

## Scope

Good fits for this repository:

- MCP server bugs
- TrueNAS SCALE API compatibility issues
- read-only safety issues
- app, job, pool, alert, dataset, share, or snapshot tool behavior
- documentation improvements

Out of scope:

- general TrueNAS administration support
- recovery from destructive storage operations
- support for leaked API keys or compromised systems
- guaranteed response times

If you are connecting to a real TrueNAS system for the first time, start with [`docs/first-contact.md`](docs/first-contact.md).
