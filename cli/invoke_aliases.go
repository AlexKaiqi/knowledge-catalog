package cli

// The transport-neutral execution vocabulary (flags, JSON envelope, RunResult,
// Stage, BindingScope, the observation accumulator) lives in kc/invoke since
// QUALITY-05 S3'; both the CLI front end and kc/cli/serve depend on that shared
// contract. Invocation/Command/Handler came back to cli in CAP-ORG-01 R1: they
// carry the transport-opened *home.Home, so they are workspace carriers, not
// transport-neutral vocabulary. The argv tokenizer (ParseArgs/setFlag) and the
// verb registry remain cli-owned.

import (
	"context"

	"kc/internal/telemetry"
	"kc/invoke"
)

type (
	FlagValue             = invoke.FlagValue
	operationTelemetry    = invoke.ObservationTelemetry
	authorizationObserver = invoke.AuthorizationObserver
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

func setIDOf(flags map[string]FlagValue) string {
	return invoke.DatasetOf(flags)
}

func setIDFlag(flags map[string]FlagValue) (string, error) {
	return invoke.RequireDataset(flags)
}

// newOperationTelemetry wires the observation accumulator; the projection
// provider is injected because storage-provider labeling resolves --home,
// which stays a front-end concern.
func newOperationTelemetry(ctx context.Context, runtime *telemetry.Runtime, operation string, projectionProvider func() string) *operationTelemetry {
	return invoke.NewObservationTelemetry(ctx, runtime, operation, projectionProvider)
}

func noOperationTelemetry(observation *operationTelemetry) *operationTelemetry {
	return invoke.NoObservationTelemetry(observation)
}
