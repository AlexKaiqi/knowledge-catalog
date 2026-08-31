package cli

import "kc/observability"

func recordFeedbackWithTelemetry(observation *operationTelemetry, store *observability.FileStore, event observability.FeedbackEvent) error {
	started := telemetryNow()
	err := store.RecordFeedback(event)
	if observe := noOperationTelemetry(observation).evidence; observe != nil {
		observe("feedback", telemetryOutcome(err), telemetrySince(started))
	}
	return err
}
