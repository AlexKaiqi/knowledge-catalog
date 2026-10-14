package server

import (
	commonplugin "kc/discovery/plugin"
	inv "kc/invocation"
	"kc/invoke"
	"kc/kernel"
	"kc/liveaccess"
	sourcebridge "kc/liveaccess/discovery"
	accessplugin "kc/liveaccess/plugin"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	"kc/snapshot"

	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	apphome "kc/home"
	"kc/internal/telemetry"
	"kc/server/internal/readiness"
)

// httpFacade owns the request-scoped dependencies shared by all routes. Route
// registration is kept out of serve.go so process lifecycle and transport
// policy can evolve independently.
type httpFacade struct {
	home           string
	options        HTTPServerOptions
	runtime        *telemetry.Runtime
	ready          *readiness.Cache
	invoke         sync.RWMutex
	homeMu         sync.Mutex
	readHome       *Home
	workersStarted bool
	closed         bool
	openHome       func() (*Home, error)
	deployment     *apphome.DeploymentConfig
}

// HTTPHandlerWithOptions adds a trusted authentication boundary to the typed
// service APIs. Without an Authenticator, requests must still assert an
// explicitly authorized local principal through X-Kc-As (--auth local).
func HTTPHandlerWithOptions(home string, options HTTPServerOptions) http.Handler {
	return newHTTPHandler(home, options, nil)
}

// HTTPHandlerWithTelemetry assembles the facade over a caller-owned telemetry
// runtime so embedders and tests can export and inspect the emitted signals.
func HTTPHandlerWithTelemetry(home string, options HTTPServerOptions, runtime *telemetry.Runtime) http.Handler {
	return newHTTPHandlerWithRuntime(home, options, nil, runtime)
}

func newHTTPHandler(home string, options HTTPServerOptions, opened *Home) http.Handler {
	return newHTTPHandlerWithRuntime(home, options, opened, nil)
}

func newHTTPHandlerWithRuntime(home string, options HTTPServerOptions, opened *Home, runtime *telemetry.Runtime) http.Handler {
	if runtime == nil {
		var err error
		runtime, err = telemetry.New(telemetry.Config{ServiceName: "kc-server", EnableOTLP: true})
		if err != nil {
			panic(fmt.Sprintf("initialize telemetry: %v", err))
		}
	}
	if runtime.StartupError() != nil {
		_, _ = fmt.Fprintln(os.Stderr, "kc telemetry: optional OTLP exporter disabled; inspect /metrics")
	}
	options.Authenticator = observeHTTPAuthenticator(bindHTTPAuthenticator(options.Authenticator, home, opened == nil), runtime)
	facade := &httpFacade{home: home, options: options, runtime: runtime, ready: readiness.NewCache(home, 5*time.Second, readiness.Providers{SearchBackend: apphome.NewSearchBackendProbe()})}
	if opened != nil {
		facade.deployment = opened.Deployment
		facade.openHome = func() (*Home, error) { return apphome.OpenDeployment(*facade.deployment) }
		facade.readHome = opened
		facade.ready.BindProbe(func(surface string) readiness.Result {
			facade.invoke.RLock()
			defer facade.invoke.RUnlock()
			{
				registry := opened.Registry
				if err := registry.CheckAuthority(); err != nil {
					return readiness.Result{Status: "not_ready", Surface: surface, ReasonCode: "REGISTRY_STATE_UNAVAILABLE"}
				}
			}
			for _, id := range opened.Store.IDs() {
				source, _ := opened.Store.Get(id)
				if _, err := source.Head(snapshot.DefaultRef); err != nil {
					return readiness.Result{Status: "not_ready", Surface: surface, ReasonCode: "SNAPSHOT_UNAVAILABLE"}
				}
			}
			return readiness.Deployment(*facade.deployment, surface, facade.ready.Providers())
		})
	}
	mux := http.NewServeMux()
	facade.registerStatusRoutes(mux)
	facade.registerServiceRoutes(mux)
	return &ManagedHTTPHandler{Handler: observedHTTPHandler(runtime, mux), runtime: runtime, closeHome: facade.closeReadHome, stopHome: facade.stopReadHome,
		activate: func() error { _, err := facade.readHomeForRequest(); return err },
	}
}

func (f *httpFacade) registerStatusRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		body := map[string]any{"ok": true, "auth": f.options.AuthModeLabel()}
		if f.options.localAssertion() {
			body["home"] = f.home
		}
		invoke.WriteJSON(w, http.StatusOK, body)
	})
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) {
		invoke.WriteJSON(w, http.StatusOK, map[string]any{"status": "live"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		writeReadiness(w, f.ready.Overall())
	})
	mux.HandleFunc("GET /readyz/{surface}", func(w http.ResponseWriter, r *http.Request) {
		writeReadiness(w, f.ready.Surface(r.PathValue("surface")))
	})
	mux.HandleFunc("GET /metrics", f.metrics)
}

func writeReadiness(w http.ResponseWriter, result readiness.Result) {
	status := http.StatusOK
	if result.Status != "ready" {
		status = http.StatusServiceUnavailable
	}
	invoke.WriteJSON(w, status, result)
}

func (f *httpFacade) metrics(w http.ResponseWriter, r *http.Request) {
	if f.options.Authenticated() {
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
	if f.closed {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "HTTP runtime has closed")
	}
	if f.deployment != nil {
		if err := apphome.ValidateDeploymentState(*f.deployment); err != nil {
			return nil, err
		}
		if _, err := inv.ReadAllow(f.home); err != nil {
			return nil, err
		}
	}
	if f.readHome != nil {
		if !f.workersStarted {
			if err := f.startProjectionWorker(f.readHome); err != nil {
				_ = f.readHome.Close()
				f.readHome = nil
				return nil, err
			}
			f.workersStarted = true
		}
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
	if err := f.startProjectionWorker(ws); err != nil {
		_ = ws.Close()
		return nil, err
	}
	f.readHome = ws
	f.workersStarted = true
	return ws, nil
}

func (f *httpFacade) startProjectionWorker(ws *Home) error {
	if ws.Plugins != nil {
		entries := map[string]liveservice.PluginEntry{"liveaccess": accessplugin.Entry(accessplugin.Options{
			CredentialOrigins: f.options.LiveAccessCredentialOrigins,
			OpenLookup:        f.liveAccessAdapter(),
		})}
		for name, entry := range f.options.LiveAccessPlugins {
			entries[name] = entry
		}
		if ws.Plugins.Unactivated() {
			if err := ws.SetLiveAccessEntries(entries); err != nil {
				return err
			}
		}
		if err := ws.VerifyIndexPlugins(); err != nil {
			return err
		}
	}
	// Read native Repository configuration before decorating task I/O. The
	// verified selection stays fixed while requests borrow observed views.
	inv.BindHomeTelemetry(f.runtime, ws, map[string]FlagValue{"home": f.home})
	// Request-time runtime availability does not install a worker identity.
	// Background observation needs its own explicitly configured service principal.
	if ws.Plugins != nil {
		ws.Plugins.SetObservationSource(nil)
	}
	if identity := f.options.ServiceIdentity; ws.LiveAccess != nil && identity != nil && strings.TrimSpace(identity.Principal) != "" {
		if ws.Plugins != nil {
			ws.Plugins.SetObservationSource(sourcebridge.Source{Lookup: ws.LiveAccess, Request: objectmodel.RequestContext{
				Identity: kernel.IdentityContext{Principal: identity.Principal},
			}})
		}
	}
	if ws.LiveAccess != nil {
		if err := ws.LiveAccess.Start(context.Background()); err != nil {
			return err
		}
	}
	// The worker belongs to this process-lifetime Home. Open() itself must not
	// Start: one-shot CLI search would otherwise CatchUp (P-01).
	if ws.Plugins != nil {
		if err := ws.Plugins.Start(context.Background()); err != nil {
			return err
		}
	}
	if ws.ObjectMaintenance != nil {
		ws.ObjectMaintenance.Start(context.Background())
	}
	return nil
}

func (f *httpFacade) liveAccessAdapter() func(commonplugin.PluginScope) (liveaccess.StateLookup, error) {
	if f.options.StateLookup == nil {
		return nil
	}
	return func(commonplugin.PluginScope) (liveaccess.StateLookup, error) {
		if adapter, ok := f.options.StateLookup.(*liveaccess.HTTPStateLookup); ok && f.options.LiveAccessCredentialOrigins != nil {
			return adapter.WithCredentialOrigins(f.options.LiveAccessCredentialOrigins)
		}
		return f.options.StateLookup, nil
	}
}

func (f *httpFacade) stopReadHome() {
	f.homeMu.Lock()
	f.closed = true
	opened := f.readHome
	f.homeMu.Unlock()
	// Cancel workers before draining typed requests that may be waiting for
	// a worker's lock. Query resources remain owned until requests finish.
	if opened != nil {
		opened.StopMaintenance()
	}
}

func (f *httpFacade) closeReadHome() error {
	f.stopReadHome()
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
	f.workersStarted = false
	return err
}

func (f *httpFacade) addIdentityFlags(flags map[string]FlagValue, r *http.Request, id inv.HTTPIdentity) {
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
	if f.options.Authenticated() {
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
	if options.LiveAccessCredentialOrigins == nil && cfg.LiveAccess.CredentialOrigins != nil {
		options.LiveAccessCredentialOrigins = append([]string{}, cfg.LiveAccess.CredentialOrigins...)
	}
	if options.AuthMode == "" && options.Authenticator == nil {
		auth, err := HTTPServerOptionsFromFlags(map[string]FlagValue{"auth": cfg.Auth, "auth-url": cfg.AuthURL})
		if err != nil {
			return nil, err
		}
		options.AuthMode, options.Authenticator = auth.AuthMode, auth.Authenticator
		options.NativeAPIOrigin = auth.NativeAPIOrigin
	}
	if options.NativeAPIOrigin == "" && options.AuthModeLabel() == "lakefs" {
		options.NativeAPIOrigin = cfg.AuthURL
	}
	if options.NativeAPIOrigin == "" && cfg.Registry.Driver == "lakefs" {
		endpoint, err := snapshot.ParseLakeFSDSN(cfg.Registry.DSN)
		if err != nil {
			return nil, err
		}
		options.NativeAPIOrigin = endpoint.Origin
	}
	ws, err := apphome.OpenDeployment(cfg)
	if err != nil {
		return nil, err
	}
	if _, err := inv.ReadAllow(cfg.StateDir); err != nil {
		_ = ws.Close()
		return nil, err
	}
	return newHTTPHandler(cfg.StateDir, options, ws), nil
}

// HTTPHandler serves the typed APIs for one server-owned Home. Embedders and
// tests can type-assert interface{ Close() error } on the result.
func HTTPHandler(home string) http.Handler {
	return HTTPHandlerWithOptions(home, HTTPServerOptions{})
}
