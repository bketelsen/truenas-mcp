package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"truenas-mcp/truenas"
)

// mockCaller implements truenas.Caller for testing.
type mockCaller struct {
	CallFunc func(method string, params ...interface{}) (json.RawMessage, error)
}

var _ truenas.Caller = (*mockCaller)(nil)

func (m *mockCaller) Call(method string, params ...interface{}) (json.RawMessage, error) {
	return m.CallFunc(method, params...)
}

// callTool creates a server with the given caller, connects an in-memory client,
// and invokes the named tool with the provided arguments.
func callTool(t *testing.T, caller truenas.Caller, readOnly bool, toolName string, args map[string]any) (*mcp.CallToolResult, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	s := New(caller, readOnly)
	ct, st := mcp.NewInMemoryTransports()

	ss, err := s.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer func() { _ = ss.Close() }()

	c := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
	cs, err := c.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = cs.Close() }()

	return cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
}

// listTools creates a server with the given caller, connects an in-memory client,
// and returns the registered tool names.
func listTools(t *testing.T, caller truenas.Caller, readOnly bool) []string {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	s := New(caller, readOnly)
	ct, st := mcp.NewInMemoryTransports()

	ss, err := s.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer func() { _ = ss.Close() }()

	c := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
	cs, err := c.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = cs.Close() }()

	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	return names
}

// resultText extracts the text string from the first TextContent in a CallToolResult.
func resultText(t *testing.T, r *mcp.CallToolResult) string {
	t.Helper()
	if r == nil {
		t.Fatal("result is nil")
	}
	if len(r.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := r.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("first content is %T, want *mcp.TextContent", r.Content[0])
	}
	return tc.Text
}
