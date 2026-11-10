---
title: truenas-mcp
description: An MCP server that exposes TrueNAS SCALE management capabilities to AI assistants.
hide:
  - navigation
  - toc
---

<div class="fy-hero" markdown>
<div class="fy-hero__text" markdown>
<p class="fy-eyebrow">truenas-mcp</p>

# truenas-mcp

<p class="fy-hero__lede">An MCP server that exposes TrueNAS SCALE management capabilities to AI assistants. It connects to a TrueNAS SCALE appliance through the WebSocket JSON-RPC API and gives an AI client tools for storage, sharing, system and app management over stdio. It starts read-only: tools that change the appliance exist only when you enable them.</p>

[Install truenas-mcp →](getting-started/install.md){ .md-button .md-button--primary }
[Make the first connection](getting-started/first-connection.md){ .md-button }

</div>
<img class="fy-hero__icon" src="assets/logo.svg" alt="">
</div>

[Install truenas-mcp](getting-started/install.md) covers the requirements, the release packages and the build from source.

## What truenas-mcp gives you

<div class="grid cards" markdown>

-   **One view of the appliance**

    ---

    Tools list pools, datasets, snapshots, shares, alerts, apps and jobs. Reports such as `truenas_health_report` and `truenas_protection_report` combine several queries into 1 answer.

-   **Read-only by default**

    ---

    A plain `truenas-mcp serve` registers only the 23 read-only tools. The 20 write tools exist only with `--enable-writes`. See [read-only by default](concepts/read-only.md).

-   **Plain configuration**

    ---

    2 required settings, the appliance host and an API key, as flags or environment variables. See [configuration](reference/configuration.md).

-   **No replay of writes**

    ---

    The server checks the connection before each call and reconnects when the check fails. It never sends a call again after a transport error, because a write may already have taken effect.

</div>

## Start here

<div class="grid cards" markdown>

- <span class="fy-index">01</span> **[Install](getting-started/install.md)**

    Download a release or build truenas-mcp from source.

- <span class="fy-index">02</span> **[First connection](getting-started/first-connection.md)**

    Connect to a real appliance in read-only mode.

- <span class="fy-index">03</span> **[Tools](reference/tools.md)**

    Look up every tool, its parameters and its safety class.

- <span class="fy-index">04</span> **[Build from source](contributing/build.md)**

    Build the server and this site yourself.

</div>
