package cli

import (
	"time"

	"kc/index"
)

func observeProjectionExecution(cx *invocation) func() {
	started := telemetryNow()
	return func() {
		cx.Observation.projectionElapsed = telemetrySince(started)
		observeProjectionBacklog(cx)
	}
}

func observeProjectionBacklog(cx *invocation) {
	observe := cx.Observation.projection
	if observe == nil {
		return
	}
	if cx.WS.Projection == nil {
		observe(0, time.Time{})
		return
	}
	targets, err := cx.WS.Projection.Targets()
	if err != nil {
		return
	}
	lagging := 0
	oldest := time.Time{}
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
	observe(lagging, oldest)
}
