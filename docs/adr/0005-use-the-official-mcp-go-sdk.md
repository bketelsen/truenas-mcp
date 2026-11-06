# 0005 — Use the official MCP Go SDK

- **Status:** Accepted
- **Date:** 2026-03-12

## Context

Two Go libraries build MCP servers: the official
`github.com/modelcontextprotocol/go-sdk`, maintained by the MCP project, and the
community `mark3labs/mcp-go`, which has broader transport support. The server
speaks MCP to one local AI client over stdio and needs no remote transport.

## Decision

The MCP protocol layer is `github.com/modelcontextprotocol/go-sdk v1.4.0`
(`go.mod`). `server.New` in `server/server.go` builds an `*mcp.Server` with
`mcp.NewServer`, and each `register*Tools` function adds tools with
`s.AddTool(&mcp.Tool{...}, handler)`. `server.Run` starts it on
`&mcp.StdioTransport{}` and nothing else, so the only transport is stdio.

Each tool states its own `InputSchema` as a JSON-schema object, built by
`schema()`, `stringProp()`, `numberProp()`, `boolProp()` and `arrayProp()` in
`server/tools_system.go`. The SDK does not validate arguments against that
schema, so handlers read arguments with the typed accessors in
`server/params.go`.

## Consequences

- The tool surface follows the official specification, so a client update and a
  server update stay compatible without a conversion layer.
- Tool annotations (`Title`, `ReadOnlyHint`, `DestructiveHint`, `IdempotentHint`,
  `OpenWorldHint`) come from the SDK's own types, unchanged on the wire.
- Schema validation is the caller's job: `optionalBool()` and `optionalSlice()`
  drop a value of the wrong type, so a tool whose default changes the effect of
  the call uses `strictBool()` and `strictStringList()` instead.
- No SSE or Streamable HTTP transport. A remote or multi-client deployment needs
  a new design, not a flag.

## Alternatives considered

- **`mark3labs/mcp-go`:** rejected because it is not the official SDK and does
  not track the specification on the same schedule; its extra transports are not
  used here.
- **A hand-written JSON-RPC server:** rejected because it reimplements the
  protocol envelope and the tool schema without gaining anything.

## References

- Shapes: [design/overview.md](../design/overview.md) (Key Dependencies, Key
  Patterns), [specs/tools.md](../specs/tools.md), [`AGENTS.md`](../../AGENTS.md)
- Code: `go.mod`, `server/server.go`, `server/tools_system.go`, `server/params.go`
- Builds on: [ADR-0001](0001-record-architecture-decisions.md), and pairs with
  [ADR-0006](0006-use-the-official-truenas-websocket-client.md)
