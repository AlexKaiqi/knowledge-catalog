package cli

import (
	"context"
	"maps"
	"os"
	"strings"

	"kc/catalog"
	kchome "kc/home"
	"kc/internal/telemetry"
	"kc/kernel"
)

func runClientOperationWithTelemetry(ctx context.Context, runtime *telemetry.Runtime, path, command string, flags map[string]FlagValue) RunResult {
	started := telemetryStart{}
	if runtime != nil {
		ctx, started.span, started.at = runtime.StartOperation(ctx, telemetryFace(command), command)
	}
	result, err := runClientOperation(path, flags)
	// Deployment evidence belongs to its explicit durable state. Pure client
	// preprocessing has no server state and creates no implicit local directory.
	if strings.HasPrefix(path, "deployment ") {
		if config, configErr := kchome.ReadDeployment(FlagString(flags, "config")); configErr == nil && kchome.ValidateDeploymentState(config) == nil {
			stamp := maps.Clone(flags)
			stamp["as"] = config.BootstrapPrincipal
			stamp["request-id"] = telemetry.NewID("req")
			if runtime != nil {
				stamp["trace-id"] = started.span.SpanContext().TraceID().String()
				stamp["span-id"] = started.span.SpanContext().SpanID().String()
			}
			if auditErr := recordAudit(config.StateDir, command, stamp, result, err); auditErr != nil && err == nil {
				err = auditErr
			}
		}
	}
	if runtime != nil {
		outcome, errorType := telemetryResultFor(command, result, err)
		runtime.EndOperation(ctx, started.span, started.at, telemetryFace(command), command, outcome, errorType)
	}
	return shapeInvocationResult(result, err)
}

// Client operations run before task binding and never implicitly open a Home.
func runClientOperation(path string, flags map[string]FlagValue) (any, error) {
	if err := rejectUnknownFlags(flags); err != nil {
		return nil, err
	}
	if strings.HasPrefix(path, "deployment ") {
		if err := rejectFlagsOutside(flags, flagNames("config help"), "kc "+path); err != nil {
			return nil, err
		}
		config, err := kchome.ReadDeployment(FlagString(flags, "config"))
		if err != nil {
			return nil, err
		}
		switch path {
		case "deployment init":
			err := kchome.InitializeDeployment(config, func(dir, principal string) error {
				return WriteAllow(dir, AllowFile{Rules: []AllowRule{{ID: "bootstrap-deployment-admin", Principal: principal, Actions: []string{"*"}}}})
			})
			if err != nil {
				return nil, err
			}
			return map[string]any{"initialized": true, "catalogs": deploymentCatalogIDs(config)}, nil
		case "deployment system publish":
			return kchome.PublishDeploymentSystem(config)
		case "deployment status":
			opened, err := kchome.OpenDeployment(config)
			if err != nil {
				return nil, err
			}
			defer opened.Close()
			catalogs := make([]map[string]any, 0, len(config.Catalogs))
			for _, binding := range config.Catalogs {
				cat, registry, err := opened.UseCatalog(binding.ID)
				if err != nil {
					return nil, err
				}
				head, err := registry.Head()
				if err != nil {
					return nil, err
				}
				catalogs = append(catalogs, map[string]any{"id": binding.ID, "head": head, "archived": cat.Archived(), "repositories": cat.Repositories()})
			}
			return map[string]any{"status": "ready", "catalogs": catalogs}, nil
		}
	}
	if path == "workspace overlay" {
		return clientWorkspaceOverlay(flags)
	}
	if path == "pack" {
		if err := rejectFlagsOutside(flags, flagNames("repo dir ref base out help server as request-id trace-id span-id parent-span-id origin-kind source-ref evidence-ref actor-ref activity-ref input-workspace-version algorithm-spec algorithm-model algorithm-hash produced-at"), "kc pack"); err != nil {
			return nil, err
		}
		repository, err := RequireFlag(flags, "repo")
		if err != nil {
			return nil, err
		}
		dir, err := RequireFlag(flags, "dir")
		if err != nil {
			return nil, err
		}
		return buildIngestPreview(flags, dir, repository, snapshotRef(flags), kernel.CommitID(FlagString(flags, "base")))
	}
	return nil, kernel.Fail(kernel.ErrUsageInvalid, "unknown client operation")
}

func verbClientOperation(cx *invocation) (any, error) {
	return runClientOperation(strings.ReplaceAll(cx.Command, "-", " "), cx.Flags)
}

func deploymentCatalogIDs(config kchome.DeploymentConfig) []string {
	ids := make([]string, 0, len(config.Catalogs))
	for _, binding := range config.Catalogs {
		ids = append(ids, binding.ID)
	}
	return ids
}

func clientWorkspaceOverlay(flags map[string]FlagValue) (any, error) {
	if err := rejectFlagsOutside(flags, flagNames("file overlay out help"), "kc workspace overlay"); err != nil {
		return nil, err
	}
	file, err := RequireFlag(flags, "file")
	if err != nil {
		return nil, err
	}
	overlayFile, err := RequireFlag(flags, "overlay")
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	recipe, err := catalog.ParseWorkspaceRecipe(raw)
	if err != nil {
		return nil, err
	}
	raw, err = os.ReadFile(overlayFile)
	if err != nil {
		return nil, err
	}
	overlay, err := catalog.ParseWorkspaceOverlay(raw)
	if err != nil {
		return nil, err
	}
	definition, err := catalog.MergeOverlay(catalog.WorkspaceDefinition{WorkspaceID: recipe.Name, Revision: 1, Sources: recipe.Sources()}, overlay)
	if err != nil {
		return nil, err
	}
	if out := FlagString(flags, "out"); out != "" {
		merged, ok := catalog.RecipeFromWorkspace(definition)
		if !ok {
			return nil, kernel.Fail(kernel.ErrWorkspaceInvalid, "overlay does not produce a portable recipe")
		}
		body, err := catalog.FormatWorkspaceRecipe(merged)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(out, body, 0o644); err != nil {
			return nil, err
		}
		return map[string]any{"workspaceId": definition.WorkspaceID, "out": out}, nil
	}
	return definition, nil
}
