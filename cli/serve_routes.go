package cli

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"kc/internal/telemetry"
	"kc/kernel"
)

// httpFacade owns the request-scoped dependencies shared by all routes. Route
// registration is kept out of serve.go so process lifecycle and transport
// policy can evolve independently.
type httpFacade struct {
	home    string
	options HTTPServerOptions
	runtime *telemetry.Runtime
	ready   *readinessCache
	invoke  sync.RWMutex
}

// HTTPHandlerWithOptions adds a trusted authentication boundary to the same
// verb facade. Without an Authenticator it is the local-owner handler used by
// CLI tests and single-user development.
func HTTPHandlerWithOptions(home string, options HTTPServerOptions) http.Handler {
	runtime, err := telemetry.New(telemetry.Config{ServiceName: "kc-server", EnableOTLP: true})
	if err != nil {
		panic(fmt.Sprintf("initialize telemetry: %v", err))
	}
	if runtime.StartupError() != nil {
		_, _ = fmt.Fprintln(os.Stderr, "kc telemetry: optional OTLP trace exporter disabled; inspect /metrics")
	}
	facade := &httpFacade{home: home, options: options, runtime: runtime, ready: newReadinessCache(home, 5*time.Second)}
	mux := http.NewServeMux()
	facade.registerStatusRoutes(mux)
	facade.registerInspectionRoutes(mux)
	mux.HandleFunc("POST /v1/{verb}", facade.invokeVerb)
	return &managedHTTPHandler{Handler: observedHTTPHandler(runtime, mux), runtime: runtime}
}

func (f *httpFacade) registerStatusRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(consoleHTML)
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		body := map[string]any{"ok": true, "auth": "local-owner"}
		if f.options.authenticated() {
			body["auth"] = f.options.Authenticator.Name()
		} else {
			body["home"] = f.home
		}
		writeJSON(w, http.StatusOK, body)
	})
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "live"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		writeReadiness(w, f.ready.overall())
	})
	mux.HandleFunc("GET /readyz/{surface}", func(w http.ResponseWriter, r *http.Request) {
		writeReadiness(w, f.ready.surface(r.PathValue("surface")))
	})
	mux.HandleFunc("GET /metrics", f.metrics)
}

func (f *httpFacade) registerInspectionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/_state", f.workspaceState)
	mux.HandleFunc("GET /v1/_blob", f.blob)
}

func writeReadiness(w http.ResponseWriter, result readinessResult) {
	status := http.StatusOK
	if result.Status != "ready" {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, result)
}

func (f *httpFacade) metrics(w http.ResponseWriter, r *http.Request) {
	if f.options.authenticated() {
		id, ok := authenticateHTTPRequest(w, r, f.options)
		if !ok {
			return
		}
		if !f.options.isAdmin(id) {
			writeHTTPForbidden(w, "%s is not allowed to inspect server metrics", id.Principal)
			return
		}
	}
	f.runtime.MetricsHandler().ServeHTTP(w, r)
}

func (f *httpFacade) workspaceState(w http.ResponseWriter, r *http.Request) {
	id, ok := authenticateHTTPRequest(w, r, f.options)
	if !ok {
		return
	}
	if f.options.authenticated() {
		if strings.TrimSpace(r.Header.Get("X-Kc-As")) != "" {
			writeHTTPForbidden(w, "X-Kc-As is disabled when authentication is enabled")
			return
		}
		if !f.options.isAdmin(id) {
			writeJSON(w, http.StatusOK, authenticatedWorkspaceState(f.home, id))
			return
		}
	}
	as := strings.TrimSpace(r.Header.Get("X-Kc-As"))
	if f.options.authenticated() {
		as = id.Principal
	}
	writeInvoke(w, workspaceState(f.home, as))
}

func (f *httpFacade) blob(w http.ResponseWriter, r *http.Request) {
	id, ok := authenticateHTTPRequest(w, r, f.options)
	if !ok {
		return
	}
	if f.options.authenticated() && !f.options.isAdmin(id) {
		writeHTTPForbidden(w, "%s is not allowed to inspect server files", id.Principal)
		return
	}
	code, body := blobStatus(f.home, r.URL.Query().Get("dir"), r.URL.Query().Get("ref"), r.URL.Query().Get("path"))
	writeJSON(w, code, body)
}

func (f *httpFacade) invokeVerb(w http.ResponseWriter, r *http.Request) {
	verb := r.PathValue("verb")
	// Same table the CLI dispatches on, so the two transports cannot drift on
	// which verbs exist. `serve` is intentionally not in it.
	if !Verb(verb) {
		writeJSON(w, http.StatusNotFound, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "unknown command %s", verb)))
		return
	}
	id, ok := authenticateHTTPRequest(w, r, f.options)
	if !ok || !f.validateIdentityHeaders(w, r, id) {
		return
	}
	raw, err := decodeJSONBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, kernel.FaultJSON(err))
		return
	}
	if f.options.authenticated() && rawRequestString(raw, "on-behalf-of") != "" {
		writeHTTPForbidden(w, "onBehalfOf must come from the trusted authenticator")
		return
	}
	if f.options.authenticated() && requiresHTTPAdmin(verb, raw, id) && !f.options.isAdmin(id) {
		writeHTTPForbidden(w, "%s is not allowed to administer kc", id.Principal)
		return
	}
	flags, cleanup, err := flagsFromRequest(f.home, raw)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, kernel.FaultJSON(err))
		return
	}
	f.addIdentityFlags(flags, r, id)
	addHTTPTraceFlags(flags, r)
	// Requests open Home independently. Readers may overlap, while mutations
	// serialize file-backed control state; Snapshot CAS still handles stale refs.
	if readOnlyHTTPVerb(verb) {
		f.invoke.RLock()
		defer f.invoke.RUnlock()
	} else {
		f.invoke.Lock()
		defer f.invoke.Unlock()
	}
	writeInvoke(w, invokeWithTelemetry(r.Context(), f.runtime, verb, flags))
}

func (f *httpFacade) validateIdentityHeaders(w http.ResponseWriter, r *http.Request, id HTTPIdentity) bool {
	if !f.options.authenticated() {
		return true
	}
	if strings.TrimSpace(r.Header.Get("X-Kc-As")) != "" {
		writeHTTPForbidden(w, "X-Kc-As is disabled when authentication is enabled")
		return false
	}
	if strings.TrimSpace(r.Header.Get("X-Kc-On-Behalf-Of")) != "" {
		writeHTTPForbidden(w, "onBehalfOf must come from the trusted authenticator")
		return false
	}
	return true
}

func (f *httpFacade) addIdentityFlags(flags map[string]FlagValue, r *http.Request, id HTTPIdentity) {
	if f.options.authenticated() {
		flags["as"] = id.Principal
		flags["auth-provider"] = id.Provider
		flags["auth-subject"] = id.Subject
		flags["auth-login"] = id.Login
		if id.OnBehalfOf != "" {
			flags["on-behalf-of"] = id.OnBehalfOf
		}
	} else {
		if value := strings.TrimSpace(r.Header.Get("X-Kc-As")); value != "" {
			flags["as"] = value
		}
		if value := strings.TrimSpace(r.Header.Get("X-Kc-On-Behalf-Of")); value != "" {
			flags["on-behalf-of"] = value
		}
	}
	if value := strings.TrimSpace(r.Header.Get("X-Kc-Request-Id")); value != "" {
		flags["request-id"] = value
	}
}

func addHTTPTraceFlags(flags map[string]FlagValue, r *http.Request) {
	traceContext := httpTraceContext(r)
	if traceContext.UseLegacyTrace {
		for header, flag := range map[string]string{
			"X-Kc-Trace-Id":       "trace-id",
			"X-Kc-Span-Id":        "span-id",
			"X-Kc-Parent-Span-Id": "parent-span-id",
		} {
			if value := strings.TrimSpace(r.Header.Get(header)); value != "" {
				flags[flag] = value
			}
		}
		return
	}
	flags["trace-id"] = traceContext.TraceID
	flags["span-id"] = traceContext.SpanID
	if traceContext.ParentSpanID != "" {
		flags["parent-span-id"] = traceContext.ParentSpanID
	}
}
