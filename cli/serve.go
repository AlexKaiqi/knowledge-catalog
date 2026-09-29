package cli

import (
	"context"
	"fmt"
	inv "kc/invocation"
	"kc/server"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	apphome "kc/home"
)

const defaultListen = "127.0.0.1:7380"

func runServe(flags map[string]FlagValue) RunResult {
	if err := rejectUnknownFlags(flags); err != nil {
		return inv.ErrorResult(err)
	}
	if err := rejectServeFlags(flags); err != nil {
		return inv.ErrorResult(err)
	}
	config, err := apphome.ReadDeployment(flagString(flags, "config"))
	if err != nil {
		return inv.ErrorResult(err)
	}
	if flagString(flags, "auth") == "" {
		flags["auth"] = config.Auth
	}
	if flagString(flags, "auth-url") == "" {
		flags["auth-url"] = config.AuthURL
	}
	options, err := server.HTTPServerOptionsFromFlags(flags)
	if err != nil {
		return inv.ErrorResult(err)
	}
	listen := flagString(flags, "listen")
	if listen == "" {
		listen = config.Listen
		if listen == "" {
			listen = defaultListen
		}
	}
	authMode := options.AuthModeLabel()
	identityLine := "client must send X-Kc-As only"
	if options.Authenticated() {
		identityLine = "client must send Authorization only; X-Kc-As disabled"
	}
	stateRuntime := "disabled"
	if options.StateLookup != nil {
		stateRuntime = "resource-access/v1"
	}
	rerankRuntime := "disabled"
	if options.Reranker != nil {
		rerankRuntime = strings.TrimSpace(flagString(flags, "rerank-model"))
		if rerankRuntime == "" {
			rerankRuntime = strings.TrimSpace(os.Getenv("KC_RERANK_MODEL"))
		}
	}
	embeddingRuntime := "disabled"
	if options.Embedder != nil {
		embeddingRuntime = strings.TrimSpace(os.Getenv("KC_EMBEDDING_MODEL"))
	}
	_, _ = fmt.Fprintf(os.Stdout, "kc service\n  config  %s\n  listen  http://%s\n  console http://%s/console\n  auth    %s\n  state   %s\n  rerank  %s\n  embed   %s\n  APIs    /catalog/v1 /knowledge/v1 /dataset-files/v1 /writer/v1 /governance/v1 /identity/v1 /admin/v1 /operations/v1\n  as      %s\n  corr    header X-Kc-Request-Id\n", flagString(flags, "config"), listen, listen, authMode, stateRuntime, rerankRuntime, embeddingRuntime, identityLine)
	handler, err := server.HTTPHandlerFromConfig(flagString(flags, "config"), options)
	if err != nil {
		return inv.ErrorResult(err)
	}
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
			return inv.ErrorResult(err)
		}
	case <-stop.Done():
		ctx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(ctx); err != nil {
			return inv.ErrorResult(err)
		}
		if err := <-errCh; err != nil && err != http.ErrServerClosed {
			return inv.ErrorResult(err)
		}
	}
	return RunResult{Status: 0}
}

// httpHandler serves the typed APIs for one server-owned Home.
func httpHandler(home string) http.Handler {
	return server.HTTPHandlerWithOptions(home, server.HTTPServerOptions{})
}
