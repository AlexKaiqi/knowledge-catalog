package cli

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"kc/internal/telemetry"
)

// httpFacade owns the request-scoped dependencies shared by all routes. Route
// registration is kept out of serve.go so process lifecycle and transport
// policy can evolve independently.
type httpFacade struct {
	home     string
	options  HTTPServerOptions
	runtime  *telemetry.Runtime
	ready    *readinessCache
	invoke   sync.RWMutex
	homeMu   sync.Mutex
	readHome *Home
}

// HTTPHandlerWithOptions adds a trusted authentication boundary to the typed
// service APIs. Without an Authenticator, requests must still assert an
// explicitly authorized local principal through X-Kc-As.
func HTTPHandlerWithOptions(home string, options HTTPServerOptions) http.Handler {
	runtime, err := telemetry.New(telemetry.Config{ServiceName: "kc-server", EnableOTLP: true})
	if err != nil {
		panic(fmt.Sprintf("initialize telemetry: %v", err))
	}
	if runtime.StartupError() != nil {
		_, _ = fmt.Fprintln(os.Stderr, "kc telemetry: optional OTLP exporter disabled; inspect /metrics")
	}
	options.Authenticator = observeHTTPAuthenticator(options.Authenticator, runtime)
	facade := &httpFacade{home: home, options: options, runtime: runtime, ready: newReadinessCache(home, 5*time.Second)}
	mux := http.NewServeMux()
	facade.registerStatusRoutes(mux)
	facade.registerServiceRoutes(mux)
	return &managedHTTPHandler{Handler: observedHTTPHandler(runtime, mux), runtime: runtime, closeHome: facade.closeReadHome}
}

func (f *httpFacade) registerStatusRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		body := map[string]any{"ok": true, "auth": "local-principal"}
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

func (f *httpFacade) readHomeForRequest() (*Home, error) {
	f.homeMu.Lock()
	defer f.homeMu.Unlock()
	if f.readHome != nil {
		return f.readHome, nil
	}
	ws, err := Open(f.home)
	if err != nil {
		return nil, err
	}
	f.readHome = ws
	return ws, nil
}

func (f *httpFacade) closeReadHome() error {
	f.homeMu.Lock()
	defer f.homeMu.Unlock()
	if f.readHome == nil {
		return nil
	}
	err := f.readHome.Close()
	f.readHome = nil
	return err
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
