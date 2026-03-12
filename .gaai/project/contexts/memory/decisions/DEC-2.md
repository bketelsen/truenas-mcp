---
id: DEC-2
domain: architecture
level: architectural
title: "Use TrueNAS official Go WebSocket JSON-RPC client"
status: active
created_by: discovery
created_at: 2026-03-12
last_updated_by: discovery
last_updated_at: 2026-03-12
supersedes: null
superseded_by: null
tags:
  - truenas
  - api
  - architecture
related_to: [DEC-1]
---

# DEC-2 — Use TrueNAS official Go WebSocket JSON-RPC client

## Context

TrueNAS SCALE Goldeye (25.10) deprecated REST API in favor of WebSocket JSON-RPC 2.0. REST will be fully removed in TrueNAS 26. Multiple Go clients exist but only `truenas/api_client_golang` targets the current WebSocket API.

## Decision

Use the official `truenas/api_client_golang` library for all TrueNAS API communication.

## Impact

- WebSocket JSON-RPC 2.0 via `wss://<host>:444/api/current`
- API key authentication via `auth.login_with_api_key`
- Future-proof — REST alternatives will stop working in TrueNAS 26
- LGPL-3.0 license (acceptable for personal tooling)
