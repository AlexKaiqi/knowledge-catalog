package cli

import (
	"strings"
	"time"

	"kc/index"
	"kc/internal/telemetry"
)

func observeProjectionExecution(cx *invocation) func() {
	started := telemetryNow()
	return func() {
		cx.Observation.projectionElapsed = telemetrySince(started)
		observeProjectionBacklog(cx)
	}
}

func observeProjectionBacklog(cx *invocation) {
	observation := noOperationTelemetry(cx.Observation)
	observeStandingProjection(observation.runtime, cx.WS, cx.Flags)
	if observation.runtime != nil || observation.projection == nil {
		return
	}
	if cx.WS == nil || cx.WS.Projection == nil {
		observation.projection(0, time.Time{})
		return
	}
	lagging, oldest, ok := projectionBacklog(cx.WS)
	if !ok {
		return
	}
	observation.projection(lagging, oldest)
}

func observeStandingProjection(runtime *telemetry.Runtime, ws *Home, flags map[string]FlagValue) {
	if runtime == nil || ws == nil {
		return
	}
	if ws.Projection == nil {
		return
	}
	provider := projectionProviderOf(ws, flags)
	lagging, oldest, ok := projectionBacklog(ws)
	if !ok {
		return
	}
	runtime.SetProjectionBacklog(provider, lagging, oldest)
}

func projectionProviderOf(ws *Home, flags map[string]FlagValue) string {
	if ws != nil && strings.TrimSpace(ws.Stores.Index) != "" {
		return boundedTelemetryValue(ws.Stores.Index, "other", "none", "opensearch")
	}
	return telemetryProvider(flags)
}

func projectionBacklog(ws *Home) (lagging int, oldest time.Time, ok bool) {
	if ws == nil || ws.Projection == nil {
		return 0, time.Time{}, true
	}
	targets, err := ws.Projection.Targets()
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
