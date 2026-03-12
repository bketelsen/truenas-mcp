---
type: memory
category: project
id: PROJECT-001
tags:
  - product
  - vision
  - scope
created_at: 2026-03-12
updated_at: 2026-03-12
---

# Project Memory

> This file is loaded at the start of every session. Keep it concise and high-signal.

---

## Project Overview

**Name:** truenas-mcp

**Purpose:** Go CLI tool that operates as an MCP (Model Context Protocol) server, exposing TrueNAS SCALE NAS management capabilities to AI assistants like Claude. Enables AI agents to query and manage datasets, pools, snapshots, shares, system health, alerts, and apps on a personal TrueNAS server.

**Target Users:** Project owner (personal tooling for AI-assisted NAS management)

---

## Core Problems Being Solved

- AI assistants cannot interact with TrueNAS without a structured integration
- Manual NAS management tasks (snapshots, shares, alerts) could be delegated to AI agents
- No existing MCP server covers TrueNAS SCALE's WebSocket JSON-RPC API

---

## Success Metrics

- Working MCP server that Claude and other AI assistants can use to query and manage the NAS
- Covers core TrueNAS domains: datasets/pools, snapshots, sharing (SMB/NFS), system info/alerts, apps

---

## Tech Stack & Conventions

- **Language:** Go
- **CLI framework:** charmbracelet/fang v2 (Cobra wrapper — `charm.land/fang/v2`)
- **MCP library:** TBD — official `modelcontextprotocol/go-sdk` or `mark3labs/mcp-go`
- **TrueNAS client:** official `truenas/api_client_golang` (WebSocket JSON-RPC 2.0)
- **TrueNAS version:** SCALE Goldeye (25.10)
- **Transport:** stdio (standard for local AI tool use)

---

## Architectural Boundaries

- **CLI layer** — fang/cobra command tree, config loading (TrueNAS host, API key)
- **MCP server** — tool registration and request handling
- **TrueNAS client** — WebSocket JSON-RPC calls to TrueNAS API
- **Tool implementations** — one package/group per TrueNAS domain (pool, dataset, snapshot, sharing, system, app, alert)

---

## Known Constraints

- TrueNAS SCALE Goldeye uses WebSocket JSON-RPC 2.0 (REST is deprecated, will be removed in TrueNAS 26)
- API key authentication required (HTTPS only — keys auto-revoked over plain HTTP)
- Personal tooling — no multi-user, no publication, no backwards-compatibility concerns

---

## Out of Scope (Permanent)

- General-purpose CLI for human use (MCP server is the primary interface)
- Multi-NAS management (single server target)
- TrueNAS CORE support (SCALE only)
- REST API support (deprecated)
