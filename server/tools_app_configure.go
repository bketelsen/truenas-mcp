package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"truenas-mcp/truenas"
)

// registerAppConfigureTools registers the write tool that changes an installed
// app's configuration values.
//
// TrueNAS `app.update` merges the caller's `values` into the stored config with a
// SHALLOW top-level dict update (middleware plugins/apps/crud.py update_internal:
// `config.update(data['values'])`), then re-populates schema defaults for any key
// that is now missing. A partial nested payload such as {"network": {"web_port":
// {...}}} therefore replaces the whole `network` section and silently resets every
// sibling key (and any generated secret) in it to its default. This tool avoids
// that footgun by fetching the current config, deep-merging the caller's partial
// values into it client-side, and sending the complete result. The middleware
// resets `ix_certificates`, `ix_certificate_authorities`, `ix_volumes`, and
// `ix_context` itself (RESERVED_NAMES), so those keys are stripped before sending.
func registerAppConfigureTools(s *mcp.Server, client truenas.Caller) {
	s.AddTool(&mcp.Tool{
		Name: "truenas_app_configure",
		Description: "Change configuration values of an installed catalog app and return the TrueNAS job ID. " +
			"`values` is a PARTIAL nested object in the same shape truenas_app_config returns; it is deep-merged " +
			"into the current configuration (objects merge recursively, lists and scalars are replaced whole, " +
			"top-level sections must already exist). Applying the change redeploys the app's containers, so it " +
			"interrupts the service while it runs. Custom (compose) apps are refused. Set dry_run to preview " +
			"the exact merged configuration and changed paths without applying anything.",
		InputSchema: schema(map[string]any{
			"name":    stringProp("app name to configure"),
			"values":  objectProp("partial configuration to merge, e.g. {\"network\": {\"web_port\": {\"port_number\": 30990}}}"),
			"dry_run": boolProp("when true, return the merged configuration and changed paths without calling app.update"),
		}, "name", "values"),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := requireString(req, "name")
		if err != nil {
			return nil, err
		}
		values, err := requireObject(req, "values")
		if err != nil {
			return nil, err
		}
		if len(values) == 0 {
			return nil, fmt.Errorf("parameter %q must contain at least one key", "values")
		}
		dryRun := optionalBool(req, "dry_run")

		result, err := client.Call("app.query", [][]any{{"name", "=", name}})
		if err != nil {
			return nil, fmt.Errorf("app.query: %w", err)
		}
		var apps []map[string]any
		if err := json.Unmarshal(result, &apps); err != nil {
			return nil, fmt.Errorf("parsing app.query: %w", err)
		}
		if len(apps) == 0 {
			return nil, fmt.Errorf("app %q not found", name)
		}
		if custom, _ := apps[0]["custom_app"].(bool); custom {
			return nil, fmt.Errorf("app %q is a custom compose app; its configuration is a compose file, not catalog values, and is not supported by this tool", name)
		}

		result, err = client.Call("app.config", name)
		if err != nil {
			return nil, fmt.Errorf("app.config: %w", err)
		}
		var current map[string]any
		if err := json.Unmarshal(result, &current); err != nil {
			return nil, fmt.Errorf("parsing app.config: %w", err)
		}

		for key := range values {
			if _, ok := current[key]; !ok {
				return nil, fmt.Errorf("unknown top-level configuration section %q for app %q; existing sections: %s",
					key, name, strings.Join(sortedKeys(current), ", "))
			}
			if strings.HasPrefix(key, "ix_") {
				return nil, fmt.Errorf("configuration section %q is managed by TrueNAS and cannot be set", key)
			}
		}

		merged := deepMerge(current, values)
		stripReserved(current)
		stripReserved(merged)
		changed := changedPaths("", current, merged)

		if dryRun {
			return jsonValueResult(map[string]any{
				"name":          name,
				"dry_run":       true,
				"changed_paths": changed,
				"values":        merged,
			})
		}
		if len(changed) == 0 {
			return nil, fmt.Errorf("values for app %q are identical to the current configuration; nothing to apply", name)
		}

		result, err = client.Call("app.update", name, map[string]any{"values": merged})
		if err != nil {
			return nil, fmt.Errorf("app.update: %w", err)
		}
		var jobID any
		if err := json.Unmarshal(result, &jobID); err != nil {
			return nil, fmt.Errorf("parsing app.update: %w", err)
		}
		return jsonValueResult(map[string]any{
			"name":          name,
			"job_id":        jobID,
			"changed_paths": changed,
		})
	})
}

// deepMerge returns a new map with overlay merged into base: nested objects merge
// recursively, everything else (lists, scalars, null) replaces the base value.
func deepMerge(base, overlay map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(overlay))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		ov, overlayIsMap := v.(map[string]any)
		bv, baseIsMap := out[k].(map[string]any)
		if overlayIsMap && baseIsMap {
			out[k] = deepMerge(bv, ov)
			continue
		}
		out[k] = v
	}
	return out
}

// changedPaths lists dotted paths whose value differs between before and after,
// descending into nested objects so the report names leaves rather than sections.
func changedPaths(prefix string, before, after map[string]any) []string {
	var paths []string
	seen := map[string]bool{}
	for k := range before {
		seen[k] = true
	}
	for k := range after {
		seen[k] = true
	}
	for k := range seen {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		bv, bok := before[k]
		av, aok := after[k]
		if !bok || !aok {
			paths = append(paths, path)
			continue
		}
		bm, bIsMap := bv.(map[string]any)
		am, aIsMap := av.(map[string]any)
		if bIsMap && aIsMap {
			paths = append(paths, changedPaths(path, bm, am)...)
			continue
		}
		bj, _ := json.Marshal(bv)
		aj, _ := json.Marshal(av)
		if string(bj) != string(aj) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

// stripReserved removes the sections the middleware regenerates on every update
// (RESERVED_NAMES in plugins/apps/schema_construction_utils.py) so they are
// neither sent nor reported as changes.
func stripReserved(m map[string]any) {
	for key := range m {
		if strings.HasPrefix(key, "ix_") {
			delete(m, key)
		}
	}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
