package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"truenas-mcp/truenas"
)

func registerSnapshotReadTools(s *mcp.Server, client truenas.Caller) {
	s.AddTool(&mcp.Tool{
		Name:        "truenas_snapshot_list",
		Description: "List snapshots for a specific dataset with name, creation time, and referenced size.",
		InputSchema: schema(map[string]any{
			"dataset": stringProp("dataset path to list snapshots for"),
		}, "dataset"),
		Annotations: &mcp.ToolAnnotations{
			Title:         "List Snapshots",
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		dataset, err := requireString(req, "dataset")
		if err != nil {
			return nil, err
		}
		result, err := callSnapshot(client, "query", [][]any{{"dataset", "=", dataset}}, snapshotQueryOptions())
		if err != nil {
			return nil, fmt.Errorf("snapshot.query: %w", err)
		}
		return jsonResult(sortSnapshotsNewestFirst(result))
	})

	s.AddTool(&mcp.Tool{
		Name:        "truenas_snapshot_get",
		Description: "Get full details for a specific snapshot by name.",
		InputSchema: schema(map[string]any{
			"name": stringProp("full snapshot name (e.g. tank/data@snap1)"),
		}, "name"),
		Annotations: &mcp.ToolAnnotations{
			Title:         "Get Snapshot",
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := requireString(req, "name")
		if err != nil {
			return nil, err
		}
		result, err := callSnapshot(client, "query", [][]any{{"id", "=", name}}, snapshotQueryOptions())
		if err != nil {
			return nil, fmt.Errorf("snapshot.query: %w", err)
		}
		return jsonResult(result)
	})

}

func registerSnapshotWriteTools(s *mcp.Server, client truenas.Caller) {
	s.AddTool(&mcp.Tool{
		Name:        "truenas_snapshot_create",
		Description: "Create a ZFS snapshot. Auto-generates a timestamp name if omitted.",
		InputSchema: schema(map[string]any{
			"dataset": stringProp("dataset path to snapshot (e.g. tank/data)"),
			"name":    stringProp("optional snapshot name (auto-generates if omitted)"),
		}, "dataset"),
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create Snapshot",
			ReadOnlyHint:    false,
			DestructiveHint: new(false),
			IdempotentHint:  false, // without a name, every call creates a new timestamped snapshot
			OpenWorldHint:   new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		dataset, err := requireString(req, "dataset")
		if err != nil {
			return nil, err
		}
		snapName := optionalString(req, "name")
		if snapName == "" {
			snapName = "auto-" + time.Now().UTC().Format("20060102-150405")
		}
		params := map[string]any{
			"dataset": dataset,
			"name":    snapName,
		}
		result, err := callSnapshot(client, "create", params)
		if err != nil {
			return nil, fmt.Errorf("snapshot.create: %w", err)
		}
		return jsonResult(result)
	})

	s.AddTool(&mcp.Tool{
		Name:        "truenas_snapshot_delete",
		Description: "Delete a ZFS snapshot by full name (e.g. tank/data@snap1). This is destructive.",
		InputSchema: schema(map[string]any{
			"name": stringProp("full snapshot name to delete (e.g. tank/data@snap1)"),
		}, "name"),
		Annotations: &mcp.ToolAnnotations{
			Title:           "Delete Snapshot",
			ReadOnlyHint:    false,
			DestructiveHint: new(true),
			IdempotentHint:  true,
			OpenWorldHint:   new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := requireString(req, "name")
		if err != nil {
			return nil, err
		}
		result, err := callSnapshot(client, "delete", name)
		if err != nil {
			return nil, fmt.Errorf("snapshot.delete: %w", err)
		}
		return jsonResult(result)
	})
}

// snapshotProperties are the ZFS properties requested on snapshot queries.
// TrueNAS 27.0 returns an empty properties object unless asked for them.
var snapshotProperties = []string{"creation", "used", "referenced"}

func snapshotQueryOptions() map[string]any {
	return map[string]any{"extra": map[string]any{"properties": snapshotProperties}}
}

// snapshotCreation extracts the creation time (unix seconds) from a snapshot entry.
func snapshotCreation(item map[string]any) (int64, bool) {
	props, _ := item["properties"].(map[string]any)
	c, _ := props["creation"].(map[string]any)
	switch v := c["parsed"].(type) {
	case float64:
		return int64(v), true
	case map[string]any: // {"$date": millis}
		if ms, ok := v["$date"].(float64); ok {
			return int64(ms) / 1000, true
		}
	}
	if str, ok := c["rawvalue"].(string); ok {
		if n, err := strconv.ParseInt(str, 10, 64); err == nil {
			return n, true
		}
	}
	if str, ok := c["value"].(string); ok {
		if n, err := strconv.ParseInt(str, 10, 64); err == nil {
			return n, true
		}
	}
	return 0, false
}

// sortSnapshotsNewestFirst orders a snapshot list by creation time, newest first.
// Entries without a creation time sort last. Non-list input is returned unchanged.
func sortSnapshotsNewestFirst(raw json.RawMessage) json.RawMessage {
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		return raw
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, aok := snapshotCreation(items[i])
		b, bok := snapshotCreation(items[j])
		if aok != bok {
			return aok
		}
		return a > b
	})
	out, err := json.Marshal(items)
	if err != nil {
		return raw
	}
	return out
}

// callSnapshot calls a snapshot method by operation (query, create, delete).
// TrueNAS 27.0 removed zfs.snapshot.*; pool.snapshot.* (present since 25.10)
// replaces it. Try the new name first and fall back to the legacy name only
// when the server says the method does not exist, so older releases still work.
// A failed lookup never ran the operation, so the fallback cannot duplicate a write.
func callSnapshot(client truenas.Caller, op string, params ...interface{}) (json.RawMessage, error) {
	result, err := client.Call("pool.snapshot."+op, params...)
	if err == nil || !isMethodMissing(err) {
		return result, err
	}
	return client.Call("zfs.snapshot."+op, params...)
}

func isMethodMissing(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "method does not exist")
}
