package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"truenas-mcp/truenas"
)

func registerAppReadTools(s *mcp.Server, client truenas.Caller) {
	s.AddTool(&mcp.Tool{
		Name:        "truenas_app_list",
		Description: "List all installed apps with name, version, status (running/stopped), and update availability.",
		InputSchema: noArgs(),
		Annotations: &mcp.ToolAnnotations{
			Title:         "List Apps",
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := client.Call("app.query")
		if err != nil {
			return nil, fmt.Errorf("app.query: %w", err)
		}
		return jsonResult(result)
	})

	s.AddTool(&mcp.Tool{
		Name:        "truenas_app_get",
		Description: "Get detailed information for a specific app by name.",
		InputSchema: schema(map[string]any{
			"name": stringProp("app name to inspect"),
		}, "name"),
		Annotations: &mcp.ToolAnnotations{
			Title:         "Get App",
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := requireString(req, "name")
		if err != nil {
			return nil, err
		}
		result, err := client.Call("app.query", [][]any{{"name", "=", name}})
		if err != nil {
			return nil, fmt.Errorf("app.query: %w", err)
		}
		return jsonResult(result)
	})

	s.AddTool(&mcp.Tool{
		Name: "truenas_app_config",
		Description: "Get the installed configuration values for a specific app by name: " +
			"environment, storage, network, resource, and user/group settings as entered at install or edit time. " +
			"This is the raw config object and may contain plaintext secrets such as database passwords or API keys; " +
			"request it only when the configuration itself is needed. truenas_app_get shows the running workload without it.",
		InputSchema: schema(map[string]any{
			"name": stringProp("app name whose configuration to inspect"),
		}, "name"),
		Annotations: &mcp.ToolAnnotations{
			Title:         "Get App Configuration",
			ReadOnlyHint:  true, // no side effects, but the result can hold plaintext secrets
			OpenWorldHint: new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := requireString(req, "name")
		if err != nil {
			return nil, err
		}
		result, err := client.Call("app.config", name)
		if err != nil {
			return nil, fmt.Errorf("app.config: %w", err)
		}
		var config any
		if err := json.Unmarshal(result, &config); err != nil {
			return nil, fmt.Errorf("parsing app.config: %w", err)
		}
		return jsonValueResult(map[string]any{
			"name":   name,
			"config": config,
		})
	})

	s.AddTool(&mcp.Tool{
		Name:        "truenas_apps_update_report",
		Description: "Report installed apps with TrueNAS app or container image updates available.",
		InputSchema: noArgs(),
		Annotations: &mcp.ToolAnnotations{
			Title:         "App Update Report",
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := client.Call("app.query")
		if err != nil {
			return nil, fmt.Errorf("app.query: %w", err)
		}

		var apps []map[string]any
		if err := json.Unmarshal(result, &apps); err != nil {
			return nil, fmt.Errorf("parsing app.query: %w", err)
		}

		candidates := []map[string]any{}
		for _, app := range apps {
			upgradeAvailable, _ := app["upgrade_available"].(bool)
			imageUpdatesAvailable, _ := app["image_updates_available"].(bool)
			if !upgradeAvailable && !imageUpdatesAvailable {
				continue
			}

			candidates = append(candidates, map[string]any{
				"name":                    app["name"],
				"id":                      app["id"],
				"state":                   app["state"],
				"version":                 app["version"],
				"human_version":           app["human_version"],
				"latest_version":          app["latest_version"],
				"upgrade_available":       upgradeAvailable,
				"image_updates_available": imageUpdatesAvailable,
			})
		}

		report := map[string]any{
			"summary": map[string]any{
				"apps_total":        len(apps),
				"updates_available": len(candidates),
			},
			"apps": candidates,
		}
		return jsonValueResult(report)
	})

}

func registerAppWriteTools(s *mcp.Server, client truenas.Caller) {
	s.AddTool(&mcp.Tool{
		Name:        "truenas_app_start",
		Description: "Start a stopped app by name.",
		InputSchema: schema(map[string]any{
			"name": stringProp("app name to start"),
		}, "name"),
		Annotations: &mcp.ToolAnnotations{
			Title:           "Start App",
			ReadOnlyHint:    false,
			DestructiveHint: new(false),
			IdempotentHint:  false, // app.start always force-recreates containers, so starting a running app restarts it
			OpenWorldHint:   new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := requireString(req, "name")
		if err != nil {
			return nil, err
		}
		result, err := client.Call("app.start", name)
		if err != nil {
			return nil, fmt.Errorf("app.start: %w", err)
		}
		return jsonResult(result)
	})

	s.AddTool(&mcp.Tool{
		Name:        "truenas_app_stop",
		Description: "Stop a running app by name.",
		InputSchema: schema(map[string]any{
			"name": stringProp("app name to stop"),
		}, "name"),
		Annotations: &mcp.ToolAnnotations{
			Title:           "Stop App",
			ReadOnlyHint:    false,
			DestructiveHint: new(false), // compose down keeps volumes and images; truenas_app_start reverses it
			IdempotentHint:  true,
			OpenWorldHint:   new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := requireString(req, "name")
		if err != nil {
			return nil, err
		}
		result, err := client.Call("app.stop", name)
		if err != nil {
			return nil, fmt.Errorf("app.stop: %w", err)
		}
		return jsonResult(result)
	})

	s.AddTool(&mcp.Tool{
		Name: "truenas_app_restart",
		Description: "Restart an app by name by redeploying its containers, and return the TrueNAS job ID. " +
			"The app is briefly unavailable while the job runs; poll truenas_jobs_list for completion.",
		InputSchema: schema(map[string]any{
			"name": stringProp("app name to restart"),
		}, "name"),
		Annotations: &mcp.ToolAnnotations{
			Title:           "Restart App",
			ReadOnlyHint:    false,
			DestructiveHint: new(false), // redeploys containers with the stored config; no data or config changes
			IdempotentHint:  false,      // every call redeploys again
			OpenWorldHint:   new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := requireString(req, "name")
		if err != nil {
			return nil, err
		}
		// TrueNAS SCALE has no app.restart method (verified against 25.10.4's
		// core.get_methods); app.redeploy is the restart primitive and runs as a job.
		result, err := client.Call("app.redeploy", name)
		if err != nil {
			return nil, fmt.Errorf("app.redeploy: %w", err)
		}
		var jobID any
		if err := json.Unmarshal(result, &jobID); err != nil {
			return nil, fmt.Errorf("parsing app.redeploy: %w", err)
		}
		return jsonValueResult(map[string]any{
			"name":   name,
			"job_id": jobID,
		})
	})

	s.AddTool(&mcp.Tool{
		Name: "truenas_app_update",
		Description: "Upgrade a named app to its latest available version and return the TrueNAS job ID. " +
			"Before upgrading, TrueNAS stops the app and snapshots its ix-volumes and each ZFS dataset holding one of its host paths " +
			"(as <dataset>@ix-app-upgrade-<app>-<previous version>). An app rollback restores only the ix-volumes; " +
			"host-path snapshots must be rolled back by hand, and custom apps get no snapshots.",
		InputSchema: schema(map[string]any{
			"name": stringProp("app name to upgrade"),
		}, "name"),
		Annotations: &mcp.ToolAnnotations{
			Title:           "Update App",
			ReadOnlyHint:    false,
			DestructiveHint: new(true), // host paths are snapshotted, but app.rollback can't restore them (see upgradeApp)
			IdempotentHint:  true,      // app.upgrade runs under a per-app lock and refuses once no upgrade is available
			OpenWorldHint:   new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := requireString(req, "name")
		if err != nil {
			return nil, err
		}

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
		upgradeAvailable, _ := apps[0]["upgrade_available"].(bool)
		if !upgradeAvailable {
			return nil, fmt.Errorf("app %q has no update available", name)
		}

		jobID, err := upgradeApp(client, name)
		if err != nil {
			return nil, err
		}
		return jsonValueResult(map[string]any{
			"name":   name,
			"job_id": jobID,
		})
	})

	s.AddTool(&mcp.Tool{
		Name: "truenas_app_update_all",
		Description: "Upgrade all apps with updates available and return the TrueNAS job IDs. " +
			"Each upgrade snapshots that app's ix-volumes and host-path datasets first, as truenas_app_update does.",
		InputSchema: noArgs(),
		Annotations: &mcp.ToolAnnotations{
			Title:           "Update All Apps",
			ReadOnlyHint:    false,
			DestructiveHint: new(true), // truenas_app_update's manual host-path restore, for every app with an update at once
			IdempotentHint:  true,      // as truenas_app_update, per app
			OpenWorldHint:   new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := client.Call("app.query", [][]any{{"upgrade_available", "=", true}})
		if err != nil {
			return nil, fmt.Errorf("app.query: %w", err)
		}

		var apps []map[string]any
		if err := json.Unmarshal(result, &apps); err != nil {
			return nil, fmt.Errorf("parsing app.query: %w", err)
		}

		jobs := []map[string]any{}
		failures := []map[string]any{}
		for _, app := range apps {
			upgradeAvailable, _ := app["upgrade_available"].(bool)
			if !upgradeAvailable {
				continue
			}
			name, ok := app["name"].(string)
			if !ok || name == "" {
				failures = append(failures, map[string]any{
					"error": "app.query returned app without a name",
				})
				continue
			}
			jobID, err := upgradeApp(client, name)
			if err != nil {
				failures = append(failures, map[string]any{
					"name":  name,
					"error": err.Error(),
				})
				continue
			}
			jobs = append(jobs, map[string]any{
				"name":   name,
				"job_id": jobID,
			})
		}

		return jsonValueResult(map[string]any{
			"summary": map[string]any{
				"updates_available": len(apps),
				"jobs_started":      len(jobs),
				"failures":          len(failures),
			},
			"jobs":     jobs,
			"failures": failures,
		})
	})
}

func upgradeApp(client truenas.Caller, name string) (any, error) {
	// snapshot_hostpaths: true makes app.upgrade snapshot the app's host paths,
	// not only its ix-volumes, before the new version runs, so host-path data
	// (where most apps keep theirs) survives a bad upgrade or data migration.
	// TrueNAS 25.10 middleware (plugins/apps/upgrade.py,
	// take_snapshot_of_hostpath_and_stop_app):
	//   - each bind-mount source outside /mnt/.ix-apps is mapped to the ZFS
	//     dataset mounted there or containing it, and that whole dataset is
	//     snapshotted non-recursively as <dataset>@ix-app-upgrade-<app>-<old version>;
	//   - paths not on ZFS, on the boot pool, or missing are skipped, and the
	//     upgrade continues, so true never fails an upgrade over a path that
	//     isn't a dataset;
	//   - custom apps take an image-pull path that snapshots nothing.
	// app.rollback (plugins/apps/rollback.py) restores only the ix-volumes
	// snapshot. Host-path snapshots are restored by hand, and a ZFS rollback
	// reverts everything in that dataset, including other apps' data there.
	// That is why truenas_app_update and truenas_app_update_all stay destructive.
	result, err := client.Call("app.upgrade", name, map[string]any{
		"app_version":        "latest",
		"values":             map[string]any{},
		"snapshot_hostpaths": true,
	})
	if err != nil {
		return nil, fmt.Errorf("app.upgrade: %w", err)
	}

	var jobID any
	if err := json.Unmarshal(result, &jobID); err != nil {
		return nil, fmt.Errorf("parsing app.upgrade: %w", err)
	}
	return jobID, nil
}
