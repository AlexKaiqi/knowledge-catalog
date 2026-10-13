package cli

import (
	"context"
	inv "kc/invocation"
	"maps"
	"strings"

	kchome "kc/home"
	"kc/internal/telemetry"
	"kc/kernel"
)

func runClientOperationWithTelemetry(ctx context.Context, runtime *telemetry.Runtime, path, command string, flags map[string]FlagValue) RunResult {
	started := telemetryStart{}
	if runtime != nil {
		ctx, started.Span, started.At = runtime.StartOperation(ctx, telemetryFace(command), command)
	}
	result, err := runClientOperation(path, flags)
	// Deployment evidence belongs to its explicit durable state. Pure client
	// preprocessing has no server state and creates no implicit local directory.
	if strings.HasPrefix(path, "deployment ") {
		if config, configErr := kchome.ReadDeployment(flagString(flags, "config")); configErr == nil && kchome.ValidateDeploymentState(config) == nil {
			stamp := maps.Clone(flags)
			stamp["as"] = config.BootstrapPrincipal
			stamp["request-id"] = telemetry.NewID("req")
			if runtime != nil {
				stamp["trace-id"] = started.Span.SpanContext().TraceID().String()
				stamp["span-id"] = started.Span.SpanContext().SpanID().String()
			}
			if _, auditErr := recordAudit(config.StateDir, command, stamp, result, err); auditErr != nil && err == nil {
				err = auditErr
			}
		}
	}
	if runtime != nil {
		outcome, errorType := telemetryResultFor(command, result, err)
		runtime.EndOperation(ctx, started.Span, started.At, telemetryFace(command), command, outcome, errorType)
	}
	return shapeInvocationResult(result, err)
}

// Client operations run before task binding and never implicitly open a Home.
func runClientOperation(path string, flags map[string]FlagValue) (any, error) {
	if err := rejectUnknownFlags(flags); err != nil {
		return nil, err
	}
	if strings.HasPrefix(path, "deployment ") {
		allowed := "config help"
		if path == "deployment identity migrate" {
			allowed += " file"
		}
		if err := rejectFlagsOutside(flags, inv.FlagNames(allowed), "kc "+path); err != nil {
			return nil, err
		}
		config, err := kchome.ReadDeployment(flagString(flags, "config"))
		if err != nil {
			return nil, err
		}
		switch path {
		case "deployment identity migrate":
			if err := kchome.ValidateDeploymentState(config); err != nil {
				return nil, err
			}
			file, err := requireFlag(flags, "file")
			if err != nil {
				return nil, err
			}
			request, err := readIdentityMigrationRequest(file)
			if err != nil {
				return nil, err
			}
			return migrateLegacyIdentity(config.StateDir, request)
		case "deployment init":
			err := kchome.InitializeDeployment(config, func(dir, principal string) error {
				return writeAllow(dir, inv.AllowFile{Rules: []inv.AllowRule{{ID: "bootstrap-deployment-admin", Principal: principal, Actions: []string{"*"}}}})
			})
			if err != nil {
				return nil, err
			}
			return map[string]any{"initialized": true}, nil
		case "deployment system publish":
			return kchome.PublishDeploymentSystem(config)
		case "deployment status":
			opened, err := kchome.OpenDeployment(config)
			if err != nil {
				return nil, err
			}
			defer opened.Close()
			head, err := opened.Registry.Head()
			if err != nil {
				return nil, err
			}
			return map[string]any{"status": "ready", "registry": map[string]any{"head": head}, "repositories": opened.Datasets.Repositories()}, nil
		}
	}
	if path == "dataset overlay" {
		return clientKnowledgeSetOverlay(flags)
	}
	return nil, kernel.Fail(kernel.ErrUsageInvalid, "unknown client operation")
}

func verbClientOperation(cx *invocation) (any, error) {
	return runClientOperation(strings.ReplaceAll(cx.Command, "-", " "), cx.Flags)
}
