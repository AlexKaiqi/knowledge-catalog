package cli

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"kc/internal/telemetry"
	"kc/kernel"
)

//go:embed console.html
var consoleHTML []byte

const defaultListen = "127.0.0.1:7380"

func runServe(flags map[string]FlagValue) RunResult {
	home, err := resolveHome(flags)
	if err != nil {
		return errorResult(err)
	}
	options, err := httpServerOptionsFromFlags(flags)
	if err != nil {
		return errorResult(err)
	}
	listen := FlagString(flags, "listen")
	if listen == "" {
		listen = defaultListen
	}
	authMode := "local-owner"
	identityLine := "header X-Kc-As → --as"
	if options.authenticated() {
		authMode = options.Authenticator.Name()
		identityLine = "Authorization → verified principal; X-Kc-As disabled"
	}
	_, _ = fmt.Fprintf(os.Stdout, "kc HTTP facade\n  home    %s\n  listen  http://%s/\n  auth    %s\n  POST    /v1/<verb>  JSON flags (CLI names; --home pinned here)\n  as      %s\n  corr    header X-Kc-Request-Id → --request-id\n", home, listen, authMode, identityLine)
	handler := HTTPHandlerWithOptions(home, options)
	if closer, ok := handler.(interface{ Close() error }); ok {
		defer func() { _ = closer.Close() }()
	}
	if err := http.ListenAndServe(listen, handler); err != nil {
		return errorResult(err)
	}
	return RunResult{Status: 0}
}

// HTTPHandler is the same facade as `kc` verbs, pinned to one --home.
func HTTPHandler(home string) http.Handler {
	return HTTPHandlerWithOptions(home, HTTPServerOptions{})
}

// HTTPHandlerWithOptions adds a trusted authentication boundary to the same
// verb facade. Without an Authenticator it is exactly the legacy local-owner
// handler used by CLI tests and single-user development.
func HTTPHandlerWithOptions(home string, options HTTPServerOptions) http.Handler {
	runtime, err := telemetry.New(telemetry.Config{ServiceName: "kc-server", EnableOTLP: true})
	if err != nil {
		panic(fmt.Sprintf("initialize telemetry: %v", err))
	}
	if runtime.StartupError() != nil {
		_, _ = fmt.Fprintln(os.Stderr, "kc telemetry: optional OTLP trace exporter disabled; inspect /metrics")
	}
	mux := http.NewServeMux()
	// Invoke opens the persisted Home for each request. Concurrent readers may
	// use independent snapshots, but every mutation is serialized so two
	// requests cannot both load the same writer/allow/catalog file and then
	// overwrite one another. Snapshot CAS still decides stale repository writes;
	// this lock protects the service's own file-backed control state.
	var invokeMu sync.RWMutex
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(consoleHTML)
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		body := map[string]any{"ok": true, "auth": "local-owner"}
		if options.authenticated() {
			body["auth"] = options.Authenticator.Name()
		} else {
			body["home"] = home
		}
		writeJSON(w, http.StatusOK, body)
	})
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "live"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		result := overallReadiness(home)
		status := http.StatusOK
		if result.Status != "ready" {
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, result)
	})
	mux.HandleFunc("GET /readyz/{surface}", func(w http.ResponseWriter, r *http.Request) {
		result := readiness(home, r.PathValue("surface"))
		status := http.StatusOK
		if result.Status != "ready" {
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, result)
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		if options.authenticated() {
			id, ok := authenticateHTTPRequest(w, r, options)
			if !ok {
				return
			}
			if !options.isAdmin(id) {
				writeHTTPForbidden(w, "%s is not allowed to inspect server metrics", id.Principal)
				return
			}
		}
		runtime.MetricsHandler().ServeHTTP(w, r)
	})
	mux.HandleFunc("GET /v1/_state", func(w http.ResponseWriter, r *http.Request) {
		id, ok := authenticateHTTPRequest(w, r, options)
		if !ok {
			return
		}
		if options.authenticated() {
			if strings.TrimSpace(r.Header.Get("X-Kc-As")) != "" {
				writeHTTPForbidden(w, "X-Kc-As is disabled when authentication is enabled")
				return
			}
			if !options.isAdmin(id) {
				writeJSON(w, http.StatusOK, authenticatedWorkspaceState(home, id))
				return
			}
		}
		as := strings.TrimSpace(r.Header.Get("X-Kc-As"))
		if options.authenticated() {
			as = id.Principal
		}
		writeInvoke(w, workspaceState(home, as))
	})
	mux.HandleFunc("GET /v1/_blob", func(w http.ResponseWriter, r *http.Request) {
		id, ok := authenticateHTTPRequest(w, r, options)
		if !ok {
			return
		}
		if options.authenticated() && !options.isAdmin(id) {
			writeHTTPForbidden(w, "%s is not allowed to inspect server files", id.Principal)
			return
		}
		code, body := blobStatus(home, r.URL.Query().Get("dir"), r.URL.Query().Get("ref"), r.URL.Query().Get("path"))
		writeJSON(w, code, body)
	})
	mux.HandleFunc("POST /v1/{verb}", func(w http.ResponseWriter, r *http.Request) {
		verb := r.PathValue("verb")
		// Same table the CLI dispatches on, so the two transports cannot drift
		// on which verbs exist. `serve` is not in it.
		if !Verb(verb) {
			writeJSON(w, http.StatusNotFound, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "unknown command %s", verb)))
			return
		}
		id, ok := authenticateHTTPRequest(w, r, options)
		if !ok {
			return
		}
		if options.authenticated() && strings.TrimSpace(r.Header.Get("X-Kc-As")) != "" {
			writeHTTPForbidden(w, "X-Kc-As is disabled when authentication is enabled")
			return
		}
		raw, err := decodeJSONBody(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, kernel.FaultJSON(err))
			return
		}
		if options.authenticated() && (rawRequestString(raw, "on-behalf-of") != "" || strings.TrimSpace(r.Header.Get("X-Kc-On-Behalf-Of")) != "") {
			writeHTTPForbidden(w, "onBehalfOf must come from the trusted authenticator")
			return
		}
		if options.authenticated() && requiresHTTPAdmin(verb, raw, id) && !options.isAdmin(id) {
			writeHTTPForbidden(w, "%s is not allowed to administer kc", id.Principal)
			return
		}
		flags, cleanup, err := flagsFromRequest(home, raw)
		if cleanup != nil {
			defer cleanup()
		}
		if err != nil {
			writeJSON(w, http.StatusBadRequest, kernel.FaultJSON(err))
			return
		}
		if options.authenticated() {
			flags["as"] = id.Principal
			flags["auth-provider"] = id.Provider
			flags["auth-subject"] = id.Subject
			flags["auth-login"] = id.Login
			if id.OnBehalfOf != "" {
				flags["on-behalf-of"] = id.OnBehalfOf
			}
		} else if as := strings.TrimSpace(r.Header.Get("X-Kc-As")); as != "" {
			flags["as"] = as
		}
		if reqID := strings.TrimSpace(r.Header.Get("X-Kc-Request-Id")); reqID != "" {
			flags["request-id"] = reqID
		}
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
		} else {
			flags["trace-id"] = traceContext.TraceID
			flags["span-id"] = traceContext.SpanID
			if traceContext.ParentSpanID != "" {
				flags["parent-span-id"] = traceContext.ParentSpanID
			}
		}
		if !options.authenticated() {
			if value := strings.TrimSpace(r.Header.Get("X-Kc-On-Behalf-Of")); value != "" {
				flags["on-behalf-of"] = value
			}
		}
		if readOnlyHTTPVerb(verb) {
			invokeMu.RLock()
			defer invokeMu.RUnlock()
		} else {
			invokeMu.Lock()
			defer invokeMu.Unlock()
		}
		writeInvoke(w, invokeWithTelemetry(r.Context(), runtime, verb, flags))
	})
	return &managedHTTPHandler{Handler: observedHTTPHandler(runtime, mux), runtime: runtime}
}

type managedHTTPHandler struct {
	http.Handler
	runtime *telemetry.Runtime
	once    sync.Once
	err     error
}

// Close flushes and shuts down the process telemetry owned by this handler.
// Embedders and tests can type-assert interface{ Close() error }.
func (h *managedHTTPHandler) Close() error {
	if h == nil {
		return nil
	}
	h.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		h.err = h.runtime.Shutdown(ctx)
	})
	return h.err
}

func httpServerOptionsFromFlags(flags map[string]FlagValue) (HTTPServerOptions, error) {
	mode := strings.TrimSpace(FlagString(flags, "auth"))
	url := strings.TrimSpace(FlagString(flags, "auth-url"))
	admins := FlagStrings(flags, "auth-admin")
	switch mode {
	case "":
		if url != "" || len(admins) > 0 {
			return HTTPServerOptions{}, fmt.Errorf("--auth-url/--auth-admin require --auth gitea")
		}
		return HTTPServerOptions{}, nil
	case "gitea":
		if url == "" {
			return HTTPServerOptions{}, fmt.Errorf("--auth gitea requires --auth-url")
		}
		authenticator, err := NewGiteaAuthenticator(url, nil)
		if err != nil {
			return HTTPServerOptions{}, err
		}
		return HTTPServerOptions{Authenticator: authenticator, AdminPrincipals: admins}, nil
	default:
		return HTTPServerOptions{}, fmt.Errorf("--auth must be gitea")
	}
}

func readOnlyHTTPVerb(verb string) bool {
	switch verb {
	case "help", "status", "store-ls", "whoami", "allowed", "receipt",
		"read", "list", "search", "provenance", "log",
		"describe-schema", "resolve", "resolve-binding", "describe-access", "inspect", "diff", "describe-index",
		"audit", "access-log", "trace", "hitmap", "hook-ls", "gate-ls", "vfs-read", "vfs-list":
		return true
	default:
		return false
	}
}
