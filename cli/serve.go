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
	"kc/invoke"
)

const defaultListen = "127.0.0.1:7380"

func runServe(flags map[string]FlagValue) RunResult {
	if err := inv.RejectUnknownFlags(flags); err != nil {
		return inv.ErrorResult(err)
	}
	if err := inv.RejectServeFlags(flags); err != nil {
		return inv.ErrorResult(err)
	}
	config, err := apphome.ReadDeployment(invoke.FlagString(flags, "config"))
	if err != nil {
		return inv.ErrorResult(err)
	}
	if invoke.FlagString(flags, "auth") == "" {
		flags["auth"] = config.Auth
	}
	if invoke.FlagString(flags, "auth-url") == "" {
		flags["auth-url"] = config.AuthURL
	}
	options, err := server.HTTPServerOptionsFromFlags(flags)
	if err != nil {
		return inv.ErrorResult(err)
	}
	listen := invoke.FlagString(flags, "listen")
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
		rerankRuntime = strings.TrimSpace(invoke.FlagString(flags, "rerank-model"))
		if rerankRuntime == "" {
			rerankRuntime = strings.TrimSpace(os.Getenv("KC_RERANK_MODEL"))
		}
	}
	embeddingRuntime := "disabled"
	if options.Embedder != nil {
		embeddingRuntime = strings.TrimSpace(os.Getenv("KC_EMBEDDING_MODEL"))
	}
	_, _ = fmt.Fprintf(os.Stdout, "kc service\n  config  %s\n  listen  http://%s\n  console http://%s/console\n  auth    %s\n  state   %s\n  rerank  %s\n  embed   %s\n  APIs    /snapshot/v1 /object/v1 /writer/v1 /identity/v1 /admin/v1 /operations/v1\n  as      %s\n  corr    header X-Kc-Request-Id\n", invoke.FlagString(flags, "config"), listen, listen, authMode, stateRuntime, rerankRuntime, embeddingRuntime, identityLine)
	handler, err := server.HTTPHandlerFromConfig(invoke.FlagString(flags, "config"), options)
	if err != nil {
		return inv.ErrorResult(err)
	}
	if closer, ok := handler.(interface{ Close() error }); ok {
		defer func() { _ = closer.Close() }()
	}
	if starter, ok := handler.(interface{ Start() error }); ok {
		if err := starter.Start(); err != nil {
			return inv.ErrorResult(err)
		}
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
		if stopper, ok := handler.(interface{ BeginShutdown() }); ok {
			stopper.BeginShutdown()
		}
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
