# Typed Parameter Accessors Implementation Plan

> **For Norma:** REQUIRED SKILL: Use norma-develop to implement this plan task-by-task.

**Goal:** Replace the shallow `args()` pass-through and 15+ manual type-assertion blocks with typed accessor functions in a dedicated `params.go` module.

**Spec:** Surfaced by `improve-codebase-architecture` session — architecture review candidate #3.

**Architecture:** Create `server/params.go` with `requireString`, `optionalString`, `requireFloat64`, `optionalFloat64`, `optionalBool`, and `optionalSlice` accessors. Each accessor parses the request JSON and extracts a typed value, surfacing errors on missing required fields. Delete `args()` from `tools_system.go`. Update all 8 `tools_*.go` files to use the new accessors.

**Tech Stack:** Go, `github.com/modelcontextprotocol/go-sdk/mcp`

**Worktree:** `/home/bjk/projects/truenas-mcp/.worktrees/refactor/typed-params`

**Branch:** `refactor/typed-params`

---

## Task 1: Create params.go and params_test.go

**Files:**

- Create: `server/params.go`
- Create: `server/params_test.go`

### Step 1: Write params_test.go

```go
package server

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func makeReq(t *testing.T, fields map[string]any) *mcp.CallToolRequest {
	t.Helper()
	b, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return &mcp.CallToolRequest{
		Params: mcp.CallToolRequestParams{
			Arguments: b,
		},
	}
}

func TestRequireString_present(t *testing.T) {
	req := makeReq(t, map[string]any{"name": "myapp"})
	got, err := requireString(req, "name")
	if err != nil || got != "myapp" {
		t.Fatalf("got %q, err %v", got, err)
	}
}

func TestRequireString_missing(t *testing.T) {
	req := makeReq(t, map[string]any{})
	_, err := requireString(req, "name")
	if err == nil {
		t.Fatal("expected error for missing required param")
	}
}

func TestRequireString_empty(t *testing.T) {
	req := makeReq(t, map[string]any{"name": ""})
	_, err := requireString(req, "name")
	if err == nil {
		t.Fatal("expected error for empty required param")
	}
}

func TestOptionalString_present(t *testing.T) {
	req := makeReq(t, map[string]any{"level": "WARNING"})
	got := optionalString(req, "level")
	if got != "WARNING" {
		t.Fatalf("got %q", got)
	}
}

func TestOptionalString_absent(t *testing.T) {
	req := makeReq(t, map[string]any{})
	got := optionalString(req, "level")
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestRequireFloat64_present(t *testing.T) {
	req := makeReq(t, map[string]any{"id": float64(42)})
	got, err := requireFloat64(req, "id")
	if err != nil || got != 42 {
		t.Fatalf("got %v, err %v", got, err)
	}
}

func TestRequireFloat64_missing(t *testing.T) {
	req := makeReq(t, map[string]any{})
	_, err := requireFloat64(req, "id")
	if err == nil {
		t.Fatal("expected error for missing required float64 param")
	}
}

func TestOptionalBool_true(t *testing.T) {
	req := makeReq(t, map[string]any{"guest_ok": true})
	if !optionalBool(req, "guest_ok") {
		t.Fatal("expected true")
	}
}

func TestOptionalBool_absent(t *testing.T) {
	req := makeReq(t, map[string]any{})
	if optionalBool(req, "guest_ok") {
		t.Fatal("expected false")
	}
}

func TestOptionalSlice_present(t *testing.T) {
	req := makeReq(t, map[string]any{"hosts": []any{"10.0.0.1"}})
	got := optionalSlice(req, "hosts")
	if len(got) != 1 {
		t.Fatalf("expected 1 element, got %v", got)
	}
}

func TestOptionalSlice_absent(t *testing.T) {
	req := makeReq(t, map[string]any{})
	got := optionalSlice(req, "hosts")
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %v", got)
	}
}

func TestOptionalFloat64_present(t *testing.T) {
	req := makeReq(t, map[string]any{"limit": float64(25)})
	got, ok := optionalFloat64(req, "limit")
	if !ok || got != 25 {
		t.Fatalf("got %v, ok %v", got, ok)
	}
}

func TestOptionalFloat64_absent(t *testing.T) {
	req := makeReq(t, map[string]any{})
	_, ok := optionalFloat64(req, "limit")
	if ok {
		t.Fatal("expected ok=false for absent field")
	}
}
```

### Step 2: Run test to verify it fails

```bash
cd /home/bjk/projects/truenas-mcp/.worktrees/refactor/typed-params
go test ./server/ -run "TestRequire|TestOptional"
```

Expected: compile error — `requireString`, `optionalString`, etc. are undefined.

### Step 3: Write params.go

```go
package server

import (
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func parseArgs(req *mcp.CallToolRequest) map[string]any {
	var m map[string]any
	if err := json.Unmarshal(req.Params.Arguments, &m); err != nil || m == nil {
		return map[string]any{}
	}
	return m
}

func requireString(req *mcp.CallToolRequest, field string) (string, error) {
	m := parseArgs(req)
	v, ok := m[field].(string)
	if !ok || v == "" {
		return "", fmt.Errorf("required parameter %q missing", field)
	}
	return v, nil
}

func optionalString(req *mcp.CallToolRequest, field string) string {
	m := parseArgs(req)
	v, _ := m[field].(string)
	return v
}

func requireFloat64(req *mcp.CallToolRequest, field string) (float64, error) {
	m := parseArgs(req)
	v, ok := m[field].(float64)
	if !ok {
		return 0, fmt.Errorf("required parameter %q missing", field)
	}
	return v, nil
}

func optionalFloat64(req *mcp.CallToolRequest, field string) (float64, bool) {
	m := parseArgs(req)
	v, ok := m[field].(float64)
	return v, ok && v > 0
}

func optionalBool(req *mcp.CallToolRequest, field string) bool {
	m := parseArgs(req)
	v, _ := m[field].(bool)
	return v
}

func optionalSlice(req *mcp.CallToolRequest, field string) []any {
	m := parseArgs(req)
	v, _ := m[field].([]any)
	return v
}
```

### Step 4: Run tests to verify they pass

```bash
cd /home/bjk/projects/truenas-mcp/.worktrees/refactor/typed-params
go test ./server/ -run "TestRequire|TestOptional"
```

Expected: all 13 tests PASS.

### Step 5: Commit

```bash
cd /home/bjk/projects/truenas-mcp/.worktrees/refactor/typed-params
git add server/params.go server/params_test.go
git commit -m "feat: add typed parameter accessor functions in params.go"
```

---

## Task 2: Migrate all tool handlers to typed accessors

**Files:**

- Modify: `server/tools_alert.go`
- Modify: `server/tools_app.go`
- Modify: `server/tools_dataset.go`
- Modify: `server/tools_pool.go`
- Modify: `server/tools_reports.go`
- Modify: `server/tools_share.go`
- Modify: `server/tools_snapshot.go`

Apply these exact transformations in each file:

### tools_alert.go

`truenas_alert_list` handler — replace:

```go
a := args(req)
params := []any{}
if level, ok := a["level"].(string); ok && level != "" {
    params = append(params, [][]any{{"level", "=", level}})
}
```

With:

```go
params := []any{}
if level := optionalString(req, "level"); level != "" {
    params = append(params, [][]any{{"level", "=", level}})
}
```

`truenas_alert_dismiss` handler — replace:

```go
a := args(req)
id, ok := a["id"].(string)
if !ok || id == "" {
    return nil, fmt.Errorf("required parameter 'id' missing")
}
```

With:

```go
id, err := requireString(req, "id")
if err != nil {
    return nil, err
}
```

### tools_app.go

Five handlers (`truenas_app_get`, `truenas_app_start`, `truenas_app_stop`, `truenas_app_restart`, `truenas_app_update`) each contain:

```go
a := args(req)
name, ok := a["name"].(string)
if !ok || name == "" {
    return nil, fmt.Errorf("required parameter 'name' missing")
}
```

Replace each with:

```go
name, err := requireString(req, "name")
if err != nil {
    return nil, err
}
```

Note: `truenas_app_update_all` does NOT call `args(req)` on the request — it uses the `apps` slice from the API response. Leave that handler unchanged.

Also remove `"encoding/json"` from the import block if it is no longer used (it IS still used in `truenas_apps_update_report` and `truenas_app_update`, so leave it).

### tools_dataset.go

`truenas_dataset_list` handler — replace:

```go
a := args(req)
params := []any{}
if pool, ok := a["pool"].(string); ok && pool != "" {
    params = append(params, [][]any{{"pool", "=", pool}})
}
```

With:

```go
params := []any{}
if pool := optionalString(req, "pool"); pool != "" {
    params = append(params, [][]any{{"pool", "=", pool}})
}
```

`truenas_dataset_get` handler — replace:

```go
a := args(req)
path, ok := a["path"].(string)
if !ok || path == "" {
    return nil, fmt.Errorf("required parameter 'path' missing")
}
```

With:

```go
path, err := requireString(req, "path")
if err != nil {
    return nil, err
}
```

`truenas_dataset_create` handler — replace:

```go
a := args(req)
name, ok := a["name"].(string)
if !ok || name == "" {
    return nil, fmt.Errorf("required parameter 'name' missing")
}
params := map[string]any{"name": name}
if comments, ok := a["comments"].(string); ok && comments != "" {
    params["comments"] = comments
}
if compression, ok := a["compression"].(string); ok && compression != "" {
    params["compression"] = compression
}
```

With:

```go
name, err := requireString(req, "name")
if err != nil {
    return nil, err
}
params := map[string]any{"name": name}
if comments := optionalString(req, "comments"); comments != "" {
    params["comments"] = comments
}
if compression := optionalString(req, "compression"); compression != "" {
    params["compression"] = compression
}
```

`truenas_dataset_delete` handler — replace:

```go
a := args(req)
path, ok := a["path"].(string)
if !ok || path == "" {
    return nil, fmt.Errorf("required parameter 'path' missing")
}
```

With:

```go
path, err := requireString(req, "path")
if err != nil {
    return nil, err
}
```

### tools_pool.go

`truenas_pool_get` handler — replace:

```go
a := args(req)
name, ok := a["name"].(string)
if !ok || name == "" {
    return nil, fmt.Errorf("required parameter 'name' missing")
}
```

With:

```go
name, err := requireString(req, "name")
if err != nil {
    return nil, err
}
```

### tools_reports.go

`truenas_jobs_list` handler — replace:

```go
a := args(req)
filters := [][]any{}
if state, ok := a["state"].(string); ok && state != "" {
    filters = append(filters, []any{"state", "=", strings.ToUpper(state)})
}
if method, ok := a["method"].(string); ok && method != "" {
    filters = append(filters, []any{"method", "=", method})
}
limit := 50
if rawLimit, ok := a["limit"].(float64); ok && rawLimit > 0 {
    limit = int(rawLimit)
}
```

With:

```go
filters := [][]any{}
if state := optionalString(req, "state"); state != "" {
    filters = append(filters, []any{"state", "=", strings.ToUpper(state)})
}
if method := optionalString(req, "method"); method != "" {
    filters = append(filters, []any{"method", "=", method})
}
limit := 50
if rawLimit, ok := optionalFloat64(req, "limit"); ok {
    limit = int(rawLimit)
}
```

### tools_share.go

`truenas_smb_create` handler — replace:

```go
a := args(req)
name, ok := a["name"].(string)
if !ok || name == "" {
    return nil, fmt.Errorf("required parameter 'name' missing")
}
path, ok := a["path"].(string)
if !ok || path == "" {
    return nil, fmt.Errorf("required parameter 'path' missing")
}
params := map[string]any{"name": name, "path": path}
if comment, ok := a["comment"].(string); ok && comment != "" {
    params["comment"] = comment
}
if guestOK, ok := a["guest_ok"].(bool); ok && guestOK {
    params["guestok"] = true
}
```

With:

```go
name, err := requireString(req, "name")
if err != nil {
    return nil, err
}
path, err := requireString(req, "path")
if err != nil {
    return nil, err
}
params := map[string]any{"name": name, "path": path}
if comment := optionalString(req, "comment"); comment != "" {
    params["comment"] = comment
}
if optionalBool(req, "guest_ok") {
    params["guestok"] = true
}
```

`truenas_smb_delete` handler — replace:

```go
a := args(req)
id, ok := a["id"].(float64)
if !ok {
    return nil, fmt.Errorf("required parameter 'id' missing")
}
result, err := client.Call("sharing.smb.delete", int(id))
```

With:

```go
id, err := requireFloat64(req, "id")
if err != nil {
    return nil, err
}
result, err := client.Call("sharing.smb.delete", int(id))
```

`truenas_nfs_create` handler — replace:

```go
a := args(req)
path, ok := a["path"].(string)
if !ok || path == "" {
    return nil, fmt.Errorf("required parameter 'path' missing")
}
params := map[string]any{"path": path}
if networks, ok := a["networks"].([]any); ok && len(networks) > 0 {
    params["networks"] = networks
}
if hosts, ok := a["hosts"].([]any); ok && len(hosts) > 0 {
    params["hosts"] = hosts
}
```

With:

```go
path, err := requireString(req, "path")
if err != nil {
    return nil, err
}
params := map[string]any{"path": path}
if networks := optionalSlice(req, "networks"); len(networks) > 0 {
    params["networks"] = networks
}
if hosts := optionalSlice(req, "hosts"); len(hosts) > 0 {
    params["hosts"] = hosts
}
```

`truenas_nfs_delete` handler — replace:

```go
a := args(req)
id, ok := a["id"].(float64)
if !ok {
    return nil, fmt.Errorf("required parameter 'id' missing")
}
result, err := client.Call("sharing.nfs.delete", int(id))
```

With:

```go
id, err := requireFloat64(req, "id")
if err != nil {
    return nil, err
}
result, err := client.Call("sharing.nfs.delete", int(id))
```

### tools_snapshot.go

`truenas_snapshot_list` handler — replace:

```go
a := args(req)
dataset, ok := a["dataset"].(string)
if !ok || dataset == "" {
    return nil, fmt.Errorf("required parameter 'dataset' missing")
}
```

With:

```go
dataset, err := requireString(req, "dataset")
if err != nil {
    return nil, err
}
```

`truenas_snapshot_get` handler — replace:

```go
a := args(req)
name, ok := a["name"].(string)
if !ok || name == "" {
    return nil, fmt.Errorf("required parameter 'name' missing")
}
```

With:

```go
name, err := requireString(req, "name")
if err != nil {
    return nil, err
}
```

`truenas_snapshot_create` handler — replace:

```go
a := args(req)
dataset, ok := a["dataset"].(string)
if !ok || dataset == "" {
    return nil, fmt.Errorf("required parameter 'dataset' missing")
}
snapName, _ := a["name"].(string)
```

With:

```go
dataset, err := requireString(req, "dataset")
if err != nil {
    return nil, err
}
snapName := optionalString(req, "name")
```

`truenas_snapshot_delete` handler — replace:

```go
a := args(req)
name, ok := a["name"].(string)
if !ok || name == "" {
    return nil, fmt.Errorf("required parameter 'name' missing")
}
```

With:

```go
name, err := requireString(req, "name")
if err != nil {
    return nil, err
}
```

### Step: Run tests to verify all handlers still pass

```bash
cd /home/bjk/projects/truenas-mcp/.worktrees/refactor/typed-params
go build ./...
go test ./...
```

Expected: all existing tests pass, no compilation errors.

### Step: Commit

```bash
cd /home/bjk/projects/truenas-mcp/.worktrees/refactor/typed-params
git add server/tools_alert.go server/tools_app.go server/tools_dataset.go \
        server/tools_pool.go server/tools_reports.go server/tools_share.go \
        server/tools_snapshot.go
git commit -m "refactor: migrate tool handlers to typed parameter accessors"
```

---

## Task 3: Remove args() from tools_system.go

**Files:**

- Modify: `server/tools_system.go`

### Step 1: Delete the args() function

Remove lines:

```go
func args(req *mcp.CallToolRequest) map[string]any {
	var m map[string]any
	if err := json.Unmarshal(req.Params.Arguments, &m); err != nil || m == nil {
		return map[string]any{}
	}
	return m
}
```

`tools_system.go` still uses `encoding/json` in `jsonResult()` — keep the import.

### Step 2: Verify build and tests

```bash
cd /home/bjk/projects/truenas-mcp/.worktrees/refactor/typed-params
go build ./...
go test ./...
```

Expected: clean build, all tests pass.

### Step 3: Commit

```bash
cd /home/bjk/projects/truenas-mcp/.worktrees/refactor/typed-params
git add server/tools_system.go
git commit -m "refactor: remove shallow args() now replaced by typed accessors"
```
