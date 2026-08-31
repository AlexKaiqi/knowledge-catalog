package telemetry

import (
	"context"
	"net/http"
	"strings"
	"time"

	otellog "go.opentelemetry.io/otel/log"
)

// Diagnostic log names and attribute keys are kept separate from the runtime
// wiring so their schema can evolve without mixing backend configuration into
// the event contract.
const (
	HTTPCompletionEvent = "kc.http.request.completed"

	logAttrRequestID          = "kc.request.id"
	logAttrOutcome            = "kc.outcome"
	logAttrDurationMS         = "kc.duration_ms"
	logAttrPropagationOutcome = "kc.propagation.outcome"
	logAttrHTTPMethod         = "http.request.method"
	logAttrHTTPRoute          = "http.route"
	logAttrHTTPStatus         = "http.response.status_code"
)

// RecordHTTPCompletion emits one payload-safe log record for one product HTTP
// request. The active span context is attached by the OTel SDK through ctx.
func (r *Runtime) RecordHTTPCompletion(ctx context.Context, requestID, method, route string, status int, propagationOutcome string, elapsed time.Duration) {
	if r == nil {
		return
	}
	outcome := httpOutcome(status)
	severity, severityText := otellog.SeverityInfo, "INFO"
	if status >= http.StatusInternalServerError {
		severity, severityText = otellog.SeverityError, "ERROR"
	} else if status >= http.StatusBadRequest {
		severity, severityText = otellog.SeverityWarn, "WARN"
	}
	record := otellog.Record{}
	record.SetTimestamp(time.Now())
	record.SetEventName(HTTPCompletionEvent)
	record.SetBody(otellog.StringValue(HTTPCompletionEvent))
	record.SetSeverity(severity)
	record.SetSeverityText(severityText)
	record.AddAttributes(
		otellog.String(logAttrRequestID, boundedLogID(requestID)),
		otellog.String(logAttrOutcome, outcome),
		otellog.Float64(logAttrDurationMS, float64(elapsed)/float64(time.Millisecond)),
		otellog.String(logAttrPropagationOutcome, enumValue(propagationOutcome, "invalid", "accepted", "generated", "legacy", "invalid", "conflict")),
		otellog.String(logAttrHTTPMethod, enumValue(method, "OTHER", "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "OTHER")),
		otellog.String(logAttrHTTPRoute, enumValue(route, "unmatched", "/", "/health", "/livez", "/readyz", "/readyz/{surface}", "/metrics",
			"/catalog/v1/{operation}", "/knowledge/v1/{operation}", "/workspace-files/v1/{operation}", "/writer/v1/{operation}",
			"/governance/v1/{operation}", "/identity/v1/{operation}", "/admin/v1/{operation}", "/operations/v1/{operation}", "unmatched")),
		otellog.Int(logAttrHTTPStatus, status),
	)
	r.logger.Emit(ctx, record)
}

func boundedLogID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return "other"
	}
	return value
}

func httpOutcome(status int) string {
	switch {
	case status >= 200 && status < 400:
		return "ok"
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return "denied"
	case status == http.StatusConflict:
		return "conflict"
	case status >= 400 && status < 500:
		return "invalid"
	default:
		return "error"
	}
}
