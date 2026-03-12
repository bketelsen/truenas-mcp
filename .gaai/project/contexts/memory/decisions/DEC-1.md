---
id: DEC-1
domain: architecture
level: architectural
title: "Use official modelcontextprotocol/go-sdk for MCP server"
status: active
created_by: discovery
created_at: 2026-03-12
last_updated_by: discovery
last_updated_at: 2026-03-12
supersedes: null
superseded_by: null
tags:
  - mcp
  - go
  - architecture
related_to: []
---

# DEC-1 — Use official modelcontextprotocol/go-sdk for MCP server

## Context

Two viable Go libraries exist for building MCP servers: the official `modelcontextprotocol/go-sdk` (maintained by MCP project + Google) and the community `mark3labs/mcp-go` (more mature, broader transport support).

## Decision

Use the official `modelcontextprotocol/go-sdk` as the MCP server library.

## Impact

- Struct-based tool registration with auto-generated JSON schemas from Go struct tags
- Stdio transport only (sufficient for this project's AI assistant use case)
- Forward-compatible with the MCP specification as the official SDK
- Cleaner Go-idiomatic API compared to builder-pattern alternatives
- Locks out SSE/Streamable HTTP transport (not needed for personal tooling)
