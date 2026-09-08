package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	apphome "kc/home"
	"kc/internal/telemetry"
)

// httpFacade owns the request-scoped dependencies shared by all routes. Route
// registration is kept out of serve.go so process lifecycle and transport
// policy can evolve independently.
type httpFacade struct {
	home       string
	options    HTTPServerOptions
	runtime    *telemetry.Runtime
	ready      *readinessCache
	invoke     sync.RWMutex
	homeMu     sync.Mutex
	readHome   *Home
	openHome   func() (*Home, error)
	deployment *apphome.DeploymentConfig
}

// HTTPHandlerWithOptions adds a trusted authentication boundary to the typed
// service APIs. Without an Authenticator, requests must still assert an
// explicitly authorized local principal through X-Kc-As (--auth local).
func HTTPHandlerWithOptions(home string, options HTTPServerOptions) http.Handler {
	return newHTTPHandler(home, options, nil)
}

func newHTTPHandler(home string, options HTTPServerOptions, opened *Home) http.Handler {
	runtime, err := telemetry.New(telemetry.Config{ServiceName: "kc-server", EnableOTLP: true})
	if err != nil {
		panic(fmt.Sprintf("initialize telemetry: %v", err))
	}
	if runtime.StartupError() != nil {
		_, _ = fmt.Fprintln(os.Stderr, "kc telemetry: optional OTLP exporter disabled; inspect /metrics")
	}
	options.Authenticator = observeHTTPAuthenticator(bindHTTPAuthenticator(options.Authenticator, home, opened == nil), runtime)
	facade := &httpFacade{home: home, options: options, runtime: runtime, ready: newReadinessCache(home, 5*time.Second)}
	if opened != nil {
		facade.deployment = opened.Deployment
		facade.openHome = func() (*Home, error) { return apphome.OpenDeployment(*facade.deployment) }
		facade.readHome = opened
		if opened.Projection != nil {
			opened.Projection.SetStateLookup(options.StateLookup)
			opened.Projection.Start(context.Background())
		}
		facade.ready.probe = func(surface string) readinessResult {
			facade.invoke.RLock()
			defer facade.invoke.RUnlock()
			for _, registry := range opened.Registries {
				if err := registry.CheckAuthority(); err != nil {
					return readinessResult{Status: "not_ready", Surface: surface, ReasonCode: "CATALOG_STATE_UNAVAILABLE"}
				}
			}
			for _, id := range opened.Store.IDs() {
				source, _ := opened.Store.Get(id)
				if _, err := source.Head(defaultRef); err != nil {
					return readinessResult{Status: "not_ready", Surface: surface, ReasonCode: "SNAPSHOT_UNAVAILABLE"}
				}
			}
			return deploymentReadiness(*facade.deployment, surface)
		}
	}
	mux := http.NewServeMux()
	facade.registerStatusRoutes(mux)
	facade.registerServiceRoutes(mux)
	return &managedHTTPHandler{Handler: observedHTTPHandler(runtime, mux), runtime: runtime, closeHome: facade.closeReadHome}
}

func (f *httpFacade) registerStatusRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		body := map[string]any{"ok": true, "auth": f.options.authMode()}
		if f.options.localAssertion() {
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
	if f.deployment != nil {
		if err := apphome.ValidateDeploymentState(*f.deployment); err != nil {
			return nil, err
		}
		if _, err := ReadAllow(f.home); err != nil {
			return nil, err
		}
	}
	if f.readHome != nil {
		return f.readHome, nil
	}
	open := f.openHome
	if open == nil {
		open = func() (*Home, error) { return Open(f.home) }
	}
	ws, err := open()
	if err != nil {
		return nil, err
	}
	f.readHome = ws
	if ws.Projection != nil {
		// The worker belongs to this process-lifetime Home. Open() itself must
		// not Start: one-shot CLI search would otherwise CatchUp (P-01).
		ws.Projection.SetStateLookup(f.options.StateLookup)
		ws.Projection.Start(context.Background())
	}
	return ws, nil
}

func (f *httpFacade) closeReadHome() error {
	// Shutdown must not close adapters borrowed by an in-flight typed or file read.
	f.invoke.Lock()
	defer f.invoke.Unlock()
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
	if id.User != nil {
		flags["_identity-provider"] = id.User.Provider
		flags["_identity-issuer"] = id.User.Issuer
		flags["_identity-subject"] = id.User.Subject
	}
	if id.Principal != "" {
		flags["as"] = id.Principal
	}
	if id.OnBehalfOf != "" {
		flags["on-behalf-of"] = id.OnBehalfOf
	}
	if f.options.authenticated() {
		flags["auth-provider"] = id.Provider
		flags["auth-subject"] = id.Subject
		flags["auth-login"] = id.Login
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

// HTTPHandlerFromConfig restores declared authorities and durable control state
// before accepting requests. It never scans cache directories for inventory.
func HTTPHandlerFromConfig(path string, options HTTPServerOptions) (http.Handler, error) {
	cfg, err := apphome.ReadDeployment(path)
	if err != nil {
		return nil, err
	}
	if options.AuthMode == "" && options.Authenticator == nil {
		auth, err := httpServerOptionsFromFlags(map[string]FlagValue{"auth": cfg.Auth, "auth-url": cfg.AuthURL})
		if err != nil {
			return nil, err
		}
		options.AuthMode, options.Authenticator = auth.AuthMode, auth.Authenticator
	}
	ws, err := apphome.OpenDeployment(cfg)
	if err != nil {
		return nil, err
	}
	if _, err := ReadAllow(cfg.StateDir); err != nil {
		_ = ws.Close()
		return nil, err
	}
	return newHTTPHandler(cfg.StateDir, options, ws), nil
}
