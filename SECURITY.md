# Security Policy

`truenas-mcp` connects AI clients to TrueNAS SCALE. Please treat security and privacy issues carefully, especially anything involving API keys, write-capable tools, TLS handling, or unintended exposure of NAS metadata.

## Supported Versions

The `main` branch is the actively maintained version unless a release policy is documented later.

## Reporting a Vulnerability

Please do not post API keys, private hostnames, logs with secrets, or detailed exploit information in a public issue.

Preferred reporting path:

1. Use GitHub's private vulnerability reporting / Security Advisory flow for this repository if it is available.
2. If private reporting is not available, open a public issue with a minimal description such as “security report contact needed” and omit sensitive details.

## What to Include

When reporting privately, include:

- affected commit or version
- TrueNAS SCALE version, if relevant
- impact and expected vs actual behavior
- reproduction steps or proof of concept, if safe to share privately
- whether API keys, write tools, TLS verification, or logs are involved

## Safety Notes

- Read-only mode is the default and should remain fail-closed.
- Write-capable tools must require explicit opt-in with `--enable-writes` or `TRUENAS_ENABLE_WRITES=true`.
- Read-only tools can still expose NAS metadata to the connected AI client and its logs.
- Do not share real API keys in issues, pull requests, screenshots, or CI logs.
