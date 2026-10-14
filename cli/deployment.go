package cli

import (
	"context"
	inv "kc/invocation"
	snapshotcli "kc/snapshot/cli"
	snapshotclient "kc/snapshot/client"
	"maps"
	"strings"

	kchome "kc/home"
	"kc/internal/telemetry"
	"kc/invoke"
	"kc/kernel"
)

func runClientOperationWithTelemetry(ctx context.Context, runtime *telemetry.Runtime, path, command string, flags map[string]FlagValue) RunResult {
	started := inv.TelemetryStart{}
	if runtime != nil {
		ctx, started.Span, started.At = runtime.StartOperation(ctx, inv.TelemetryFace(command), command)
	}
	result, err := runClientOperation(path, flags)
	// Deployment evidence belongs to its explicit durable state. Pure client
	// preprocessing has no server state and creates no implicit local directory.
	if strings.HasPrefix(path, "deployment ") {
		if config, configErr := kchome.ReadDeployment(invoke.FlagString(flags, "config")); configErr == nil && kchome.ValidateDeploymentState(config) == nil {
			stamp := maps.Clone(flags)
			stamp["as"] = config.BootstrapPrincipal
			stamp["request-id"] = telemetry.NewID("req")
			if runtime != nil {
				stamp["trace-id"] = started.Span.SpanContext().TraceID().String()
				stamp["span-id"] = started.Span.SpanContext().SpanID().String()
			}
			if _, auditErr := inv.RecordAudit(config.StateDir, command, stamp, result, err); auditErr != nil && err == nil {
				err = auditErr
			}
		}
	}
	if runtime != nil {
		outcome, errorType := inv.TelemetryResultFor(command, result, err)
		runtime.EndOperation(ctx, started.Span, started.At, inv.TelemetryFace(command), command, outcome, errorType)
	}
	return inv.ShapeInvocationResult(result, err)
}

// Client operations run before task binding and never implicitly open a Home.
func runClientOperation(path string, flags map[string]FlagValue) (any, error) {
	if err := inv.RejectUnknownFlags(flags); err != nil {
		return nil, err
	}
	if strings.HasPrefix(path, "deployment ") {
		allowed := "config help"
		if err := inv.RejectFlagsOutside(flags, inv.FlagNames(allowed), "kc "+path); err != nil {
			return nil, err
		}
		config, err := kchome.ReadDeployment(invoke.FlagString(flags, "config"))
		if err != nil {
			return nil, err
		}
		switch path {
		case "deployment init":
			err := kchome.InitializeDeployment(config, func(dir, principal string) error {
				return inv.WriteAllow(dir, inv.AllowFile{Rules: []inv.AllowRule{{ID: "bootstrap-deployment-admin", Principal: principal, Actions: []string{"*"}}}})
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
		return snapshotcli.New(nil, snapshotcli.Context{}).Execute(context.Background(), "dataset overlay", snapshotFlags(flags), snapshotclient.RequestOptions{})
	}
	return nil, kernel.Fail(kernel.ErrUsageInvalid, "unknown client operation")
}
