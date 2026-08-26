package cli

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"kc/internal/telemetry"
)

type httpTelemetryContext struct {
	PropagationOutcome string
	UseLegacyTrace     bool
	TraceID            string
	SpanID             string
	ParentSpanID       string
}

type httpTelemetryContextKey struct{}

func observedHTTPHandler(runtime *telemetry.Runtime, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("X-Kc-Request-Id"))
		if requestID != "" {
			if _, err := requestIDFrom(map[string]FlagValue{"request-id": requestID}); err != nil {
				requestID = ""
			}
		}
		if requestID == "" {
			requestID = telemetry.NewID("req")
			r.Header.Set("X-Kc-Request-Id", requestID)
		}
		w.Header().Set("X-Kc-Request-Id", requestID)

		started := time.Now()
		runtime.AddHTTPActive(r.Context(), 1, r.Method)
		defer runtime.AddHTTPActive(r.Context(), -1, r.Method)

		traceparent := strings.TrimSpace(r.Header.Get("traceparent"))
		legacy := hasLegacyTraceHeaders(r)
		parentContext := runtime.Propagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		remote := trace.SpanContextFromContext(parentContext)
		propagationOutcome, useLegacy := "generated", false
		switch {
		case traceparent != "" && remote.IsValid():
			propagationOutcome = "accepted"
			if legacy {
				propagationOutcome = "conflict"
			} else if state := strings.TrimSpace(r.Header.Get("tracestate")); state != "" {
				if _, err := trace.ParseTraceState(state); err != nil {
					propagationOutcome = "invalid"
				}
			}
		case traceparent != "" && legacy:
			parentContext = r.Context()
			propagationOutcome, useLegacy = "invalid", true
		case traceparent != "":
			parentContext = r.Context()
			propagationOutcome = "invalid"
		case legacy:
			parentContext = r.Context()
			propagationOutcome, useLegacy = "legacy", true
		default:
			parentContext = r.Context()
		}

		route := httpRoute(r)
		method := boundedHTTPMethod(r.Method)
		ctx, span := runtime.StartServer(parentContext, method+" "+route,
			attribute.String("http.request.method", method),
			attribute.String("http.route", route),
			attribute.String("kc.propagation.outcome", propagationOutcome),
			attribute.String("kc.request_id", requestID),
		)
		current := span.SpanContext()
		state := httpTelemetryContext{
			PropagationOutcome: propagationOutcome,
			UseLegacyTrace:     useLegacy,
			TraceID:            current.TraceID().String(),
			SpanID:             current.SpanID().String(),
		}
		if remote.IsValid() && !useLegacy {
			state.ParentSpanID = remote.SpanID().String()
		}
		ctx = context.WithValue(ctx, httpTelemetryContextKey{}, state)
		r = r.WithContext(ctx)
		recorder := &telemetryResponseWriter{ResponseWriter: w}
		defer func() {
			panicValue := recover()
			status := recorder.status
			if panicValue != nil {
				status = http.StatusInternalServerError
			}
			if status == 0 {
				status = http.StatusOK
			}
			span.SetAttributes(attribute.Int("http.response.status_code", status))
			if status >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, http.StatusText(status))
			}
			runtime.RecordHTTP(ctx, started, r.Method, route, status, propagationOutcome)
			span.End()
			if panicValue != nil {
				panic(panicValue)
			}
		}()
		next.ServeHTTP(recorder, r)
	})
}

func boundedHTTPMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions:
		return method
	default:
		return "OTHER"
	}
}

func httpTraceContext(r *http.Request) httpTelemetryContext {
	state, _ := r.Context().Value(httpTelemetryContextKey{}).(httpTelemetryContext)
	return state
}

func hasLegacyTraceHeaders(r *http.Request) bool {
	return strings.TrimSpace(r.Header.Get("X-Kc-Trace-Id")) != "" ||
		strings.TrimSpace(r.Header.Get("X-Kc-Span-Id")) != "" ||
		strings.TrimSpace(r.Header.Get("X-Kc-Parent-Span-Id")) != ""
}

func httpRoute(r *http.Request) string {
	path := r.URL.Path
	switch {
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/v1/"):
		return "/v1/{verb}"
	case strings.HasPrefix(path, "/readyz/"):
		return "/readyz/{surface}"
	case path == "/", path == "/health", path == "/livez", path == "/readyz", path == "/metrics", path == "/v1/_state", path == "/v1/_blob":
		return path
	default:
		return "unmatched"
	}
}

type telemetryResponseWriter struct {
	http.ResponseWriter
	status int
}

var (
	_ http.Flusher  = (*telemetryResponseWriter)(nil)
	_ http.Hijacker = (*telemetryResponseWriter)(nil)
	_ http.Pusher   = (*telemetryResponseWriter)(nil)
	_ io.ReaderFrom = (*telemetryResponseWriter)(nil)
)

// Unwrap lets http.ResponseController reach optional capabilities on the
// original writer.
func (w *telemetryResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *telemetryResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *telemetryResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *telemetryResponseWriter) Flush() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *telemetryResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

func (w *telemetryResponseWriter) Push(target string, options *http.PushOptions) error {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, options)
	}
	return http.ErrNotSupported
}

func (w *telemetryResponseWriter) ReadFrom(reader io.Reader) (int64, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if readerFrom, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return readerFrom.ReadFrom(reader)
	}
	return io.Copy(w.ResponseWriter, reader)
}
