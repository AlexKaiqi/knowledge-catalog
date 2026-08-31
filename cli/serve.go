package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"kc/internal/telemetry"
)

const defaultListen = "127.0.0.1:7380"

func runServe(flags map[string]FlagValue) RunResult {
	if err := rejectUnknownFlags(flags); err != nil {
		return errorResult(err)
	}
	if err := rejectServeFlags(flags); err != nil {
		return errorResult(err)
	}
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
	authMode := "local-principal"
	identityLine := "trusted local X-Kc-As assertion"
	if options.authenticated() {
		authMode = options.Authenticator.Name()
		identityLine = "Authorization → verified principal; X-Kc-As disabled"
	}
	stateRuntime := "disabled"
	if options.StateLookup != nil {
		stateRuntime = "resource-access/v1"
	}
	_, _ = fmt.Fprintf(os.Stdout, "kc service (API only)\n  home    %s\n  listen  http://%s\n  auth    %s\n  state   %s\n  APIs    /catalog/v1 /knowledge/v1 /workspace-files/v1 /writer/v1 /governance/v1 /identity/v1 /admin/v1 /operations/v1\n  as      %s\n  corr    header X-Kc-Request-Id\n", home, listen, authMode, stateRuntime, identityLine)
	handler := HTTPHandlerWithOptions(home, options)
	if closer, ok := handler.(interface{ Close() error }); ok {
		defer func() { _ = closer.Close() }()
	}
	server := &http.Server{
		Addr:              listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()
	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return errorResult(err)
		}
	case <-stop.Done():
		ctx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(ctx); err != nil {
			return errorResult(err)
		}
		if err := <-errCh; err != nil && err != http.ErrServerClosed {
			return errorResult(err)
		}
	}
	return RunResult{Status: 0}
}

// HTTPHandler serves the typed APIs for one server-owned Home.
func HTTPHandler(home string) http.Handler {
	return HTTPHandlerWithOptions(home, HTTPServerOptions{})
}

type managedHTTPHandler struct {
	http.Handler
	runtime   *telemetry.Runtime
	closeHome func() error
	once      sync.Once
	err       error
}

// Close flushes and shuts down the process telemetry owned by this handler.
// Embedders and tests can type-assert interface{ Close() error }.
func (h *managedHTTPHandler) Close() error {
	if h == nil {
		return nil
	}
	h.once.Do(func() {
		if h.closeHome != nil {
			h.err = h.closeHome()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := h.runtime.Shutdown(ctx); err != nil && h.err == nil {
			h.err = err
		}
	})
	return h.err
}

func httpServerOptionsFromFlags(flags map[string]FlagValue) (HTTPServerOptions, error) {
	options := HTTPServerOptions{}
	resourceAccessURL := strings.TrimSpace(FlagString(flags, "resource-access-url"))
	if resourceAccessURL == "" {
		resourceAccessURL = strings.TrimSpace(os.Getenv("KC_RESOURCE_ACCESS_URL"))
	}
	if resourceAccessURL != "" {
		stateLookup, err := NewHTTPStateLookup(resourceAccessURL, nil)
		if err != nil {
			return HTTPServerOptions{}, err
		}
		options.StateLookup = stateLookup
	}
	mode := strings.TrimSpace(FlagString(flags, "auth"))
	url := strings.TrimSpace(FlagString(flags, "auth-url"))
	admins := FlagStrings(flags, "auth-admin")
	switch mode {
	case "":
		if url != "" || len(admins) > 0 {
			return HTTPServerOptions{}, fmt.Errorf("--auth-url/--auth-admin require --auth gitea")
		}
		return options, nil
	case "gitea":
		if url == "" {
			return HTTPServerOptions{}, fmt.Errorf("--auth gitea requires --auth-url")
		}
		authenticator, err := NewGiteaAuthenticator(url, nil)
		if err != nil {
			return HTTPServerOptions{}, err
		}
		options.Authenticator = authenticator
		options.AdminPrincipals = admins
		return options, nil
	default:
		return HTTPServerOptions{}, fmt.Errorf("--auth must be gitea")
	}
}
