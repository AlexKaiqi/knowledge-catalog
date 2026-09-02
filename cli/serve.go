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
	"kc/retrieval/llmhttp"
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
	authMode := options.authMode()
	identityLine := "client must send X-Kc-As only"
	if options.authenticated() {
		identityLine = "client must send Authorization only; X-Kc-As disabled"
	}
	stateRuntime := "disabled"
	if options.StateLookup != nil {
		stateRuntime = "resource-access/v1"
	}
	rerankRuntime := "disabled"
	if options.Reranker != nil {
		rerankRuntime = strings.TrimSpace(FlagString(flags, "rerank-model"))
		if rerankRuntime == "" {
			rerankRuntime = strings.TrimSpace(os.Getenv("KC_RERANK_MODEL"))
		}
	}
	_, _ = fmt.Fprintf(os.Stdout, "kc service (API only)\n  home    %s\n  listen  http://%s\n  auth    %s\n  state   %s\n  rerank  %s\n  APIs    /catalog/v1 /knowledge/v1 /workspace-files/v1 /writer/v1 /governance/v1 /identity/v1 /admin/v1 /operations/v1\n  as      %s\n  corr    header X-Kc-Request-Id\n", home, listen, authMode, stateRuntime, rerankRuntime, identityLine)
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
	model := strings.TrimSpace(FlagString(flags, "rerank-model"))
	if model == "" {
		model = strings.TrimSpace(os.Getenv("KC_RERANK_MODEL"))
	}
	if model != "" {
		timeout := 45 * time.Second
		rawTimeout := strings.TrimSpace(FlagString(flags, "rerank-timeout"))
		if rawTimeout == "" {
			rawTimeout = strings.TrimSpace(os.Getenv("KC_RERANK_TIMEOUT"))
		}
		if rawTimeout != "" {
			parsed, err := time.ParseDuration(rawTimeout)
			if err != nil || parsed <= 0 {
				return HTTPServerOptions{}, fmt.Errorf("--rerank-timeout must be a positive duration")
			}
			timeout = parsed
		}
		provider, err := llmhttp.New(llmhttp.Config{
			BaseURL: os.Getenv("OPENAI_BASE_URL"), APIKey: os.Getenv("OPENAI_API_KEY"), Model: model, Timeout: timeout,
		})
		if err != nil {
			return HTTPServerOptions{}, err
		}
		options.Reranker = provider
	}
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
	mode := strings.ToLower(strings.TrimSpace(FlagString(flags, "auth")))
	url := strings.TrimSpace(FlagString(flags, "auth-url"))
	admins := FlagStrings(flags, "auth-admin")
	hmac := strings.TrimSpace(FlagString(flags, "auth-hmac-secret"))
	if mode == "" {
		return HTTPServerOptions{}, fmt.Errorf("kc serve requires --auth %s", strings.Join(serveAuthModes(), ", "))
	}
	if mode == "local" {
		if url != "" || len(admins) > 0 || hmac != "" {
			return HTTPServerOptions{}, fmt.Errorf("--auth local does not accept --auth-url, --auth-admin, or --auth-hmac-secret")
		}
		options.AuthMode = "local"
		return options, nil
	}
	authenticator, err := resolveAuthenticator(mode, flags)
	if err != nil {
		return HTTPServerOptions{}, err
	}
	options.AuthMode = mode
	options.Authenticator = authenticator
	options.AdminPrincipals = admins

	// Service identity (Taihu application registration)
	options.ServiceIdentity = ResolveServiceIdentity(flags)

	return options, nil
}
