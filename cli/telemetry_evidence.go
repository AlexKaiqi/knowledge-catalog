package cli

import (
	"context"

	"kc/observability"
)

func recordFeedbackWithTelemetry(observation *operationTelemetry, store *observability.FileStore, event observability.FeedbackEvent) error {
	started := telemetryNow()
	err := store.RecordFeedback(context.Background(), event)
	if observe := noOperationTelemetry(observation).evidence; observe != nil {
		observe("feedback", telemetryOutcome(err), telemetrySince(started))
	}
	return err
}
