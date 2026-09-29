package server

import (
	"fmt"
	apphome "kc/home"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"kc/internal/telemetry"
	"kc/invoke"
)

type ManagedHTTPHandler struct {
	http.Handler
	runtime   *telemetry.Runtime
	closeHome func() error
	once      sync.Once
	err       error
}

func HTTPServerOptionsFromFlags(flags map[string]FlagValue) (HTTPServerOptions, error) {
	options := HTTPServerOptions{}
	model := strings.TrimSpace(invoke.FlagString(flags, "rerank-model"))
	if model == "" {
		model = strings.TrimSpace(os.Getenv("KC_RERANK_MODEL"))
	}
	if model != "" {
		timeout := 45 * time.Second
		rawTimeout := strings.TrimSpace(invoke.FlagString(flags, "rerank-timeout"))
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
		provider, err := apphome.NewRerankerProvider(model, timeout)
		if err != nil {
			return HTTPServerOptions{}, err
		}
		options.Reranker = provider
	}
	embeddingModel := strings.TrimSpace(os.Getenv("KC_EMBEDDING_MODEL"))
	if embeddingModel != "" {
		embedder, err := apphome.NewEmbedderProvider(embeddingModel)
		if err != nil {
			return HTTPServerOptions{}, err
		}
		options.Embedder = embedder
	}
	if options.StateLookup == nil {
		options.StateLookup = NewHTTPStateLookup(nil)
	}
	mode := strings.ToLower(strings.TrimSpace(invoke.FlagString(flags, "auth")))
	url := strings.TrimSpace(invoke.FlagString(flags, "auth-url"))
	admins := invoke.FlagStrings(flags, "auth-admin")
	hmac := strings.TrimSpace(invoke.FlagString(flags, "auth-hmac-secret"))
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
