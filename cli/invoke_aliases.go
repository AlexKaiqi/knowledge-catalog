package cli

// The transport-neutral execution vocabulary (flags, JSON envelope, RunResult,
// Stage, BindingScope, the observation accumulator) lives in kc/invoke since
// QUALITY-05 S3'; both the CLI front end and kc/cli/serve depend on that shared
// contract. Invocation/Command/Handler moved to kc/invocation in
// CAP-ORG-01 B5a: they carry the transport-opened *home.Home, so they are a
// workspace-carrier seam both transports import (neither may own the type —
// the transports must never import each other). The argv tokenizer
// (ParseArgs/setFlag) and the verb registry remain cli-owned; the aliases keep
// the 143 verb-body sites and the argv registry unchanged.

import (
	"context"

	"kc/internal/telemetry"
	inv "kc/invocation"
	"kc/invoke"
)

type (
	FlagValue          = invoke.FlagValue
	operationTelemetry = invoke.ObservationTelemetry
	invocation         = inv.Invocation
	command            = inv.Command
)

const (
	stageHome     = invoke.StageHome
	stageOpen     = invoke.StageOpen
	stageGoverned = invoke.StageGoverned
)

func FlagString(flags map[string]FlagValue, name string) string {
	return invoke.FlagString(flags, name)
}

func FlagStrings(flags map[string]FlagValue, name string) []string {
	return invoke.FlagStrings(flags, name)
}

func FlagBool(flags map[string]FlagValue, name string) bool {
	return invoke.FlagBool(flags, name)
}

func RequireFlag(flags map[string]FlagValue, name string) (string, error) {
	return invoke.RequireFlag(flags, name)
}

// newOperationTelemetry wires the observation accumulator; the projection
// provider is injected because storage-provider labeling resolves --home,
// which stays a front-end concern.
func newOperationTelemetry(ctx context.Context, runtime *telemetry.Runtime, operation string, projectionProvider func() string) *operationTelemetry {
	return invoke.NewObservationTelemetry(ctx, runtime, operation, projectionProvider)
}

// withHooks moves with the dispatcher (B5c); the seam owns the hook-run
// machinery since B5b.2 and the dispatcher is the only remaining caller.
