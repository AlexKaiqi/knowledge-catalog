package invocation

import (
	"strings"
	"time"

	index "kc/discovery/projection"
	kchome "kc/home"
	"kc/internal/telemetry"
	"kc/invoke"
)

func ObserveProjectionExecution(cx *Invocation) func() {
	started := TelemetryNow()
	return func() {
		cx.Observation.ProjectionElapsed = TelemetrySince(started)
		ObserveProjectionBacklog(cx)
	}
}

func ObserveProjectionBacklog(cx *Invocation) {
	observation := invoke.NoObservationTelemetry(cx.Observation)
	ObserveStandingProjection(observation.Runtime, cx.WS, cx.Flags)
	if observation.Runtime != nil || observation.Projection == nil {
		return
	}
	if cx.WS == nil || cx.WS.Plugins == nil {
		observation.Projection(0, time.Time{})
		return
	}
	lagging, oldest, ok := projectionBacklog(cx.WS)
	if !ok {
		return
	}
	observation.Projection(lagging, oldest)
}

func ObserveStandingProjection(runtime *telemetry.Runtime, ws *kchome.Home, flags map[string]invoke.FlagValue) {
	if runtime == nil || ws == nil {
		return
	}
	if ws.Plugins == nil {
		return
	}
	provider := projectionProviderOf(ws, flags)
	lagging, oldest, ok := projectionBacklog(ws)
	if !ok {
		return
	}
	runtime.SetProjectionBacklog(provider, lagging, oldest)
}

func projectionProviderOf(ws *kchome.Home, flags map[string]invoke.FlagValue) string {
	if ws != nil && strings.TrimSpace(ws.Stores.Index) != "" {
		return boundedTelemetryValue(ws.Stores.Index, "other", "none", "opensearch")
	}
	return TelemetryProvider(flags)
}

func projectionBacklog(ws *kchome.Home) (lagging int, oldest time.Time, ok bool) {
	if ws == nil || ws.Plugins == nil {
		return 0, time.Time{}, true
	}
	targets, err := ws.Plugins.Targets()
	if err != nil {
		return 0, time.Time{}, false
	}
	for _, target := range targets {
		if target.Status == index.TargetReady && target.AppliedCommit == target.DesiredCommit {
			continue
		}
		lagging++
		updated, err := time.Parse(time.RFC3339Nano, target.UpdatedAt)
		if err == nil && (oldest.IsZero() || updated.Before(oldest)) {
			oldest = updated
		}
	}
	return lagging, oldest, true
}
