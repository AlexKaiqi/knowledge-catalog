package server

import (
	"fmt"
	apphome "kc/home"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"kc/identity"
	_ "kc/identity/builtin"
	"kc/internal/telemetry"
	inv "kc/invocation"
	"kc/invoke"
	"kc/kernel"
)

type ManagedHTTPHandler struct {
	http.Handler
	runtime   *telemetry.Runtime
	closeHome func() error
	stopHome  func()
	activate  func() error
	lifeMu    sync.Mutex
	closed    bool
	started   bool
	beginOnce sync.Once
	once      sync.Once
	err       error
}

// Start activates the long-lived runtime after cold handler construction.
// kc serve calls it before listening; embedded tasks use the same facade root.
// A successful activation runs once; a failed activation remains retryable.
func (h *ManagedHTTPHandler) Start() error {
	h.lifeMu.Lock()
	defer h.lifeMu.Unlock()
	if h.closed {
		return kernel.Fail(kernel.ErrPreconditionFailed, "HTTP handler has closed")
	}
	if h.started {
		return nil
	}
	if h.activate != nil {
		if err := h.activate(); err != nil {
			return err
		}
	}
	h.started = true
	return nil
}

func (h *ManagedHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.lifeMu.Lock()
	closed := h.closed
	h.lifeMu.Unlock()
	if closed {
		invoke.WriteInvoke(w, inv.ErrorResult(kernel.Fail(kernel.ErrPreconditionFailed, "HTTP handler has closed")))
		return
	}
	h.Handler.ServeHTTP(w, r)
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
	if mode == "" {
		return HTTPServerOptions{}, fmt.Errorf("kc serve requires --auth %s", strings.Join(identity.Names(), ", "))
	}
	authenticator, err := identity.Resolve(mode, serveSettings(flags))
	if err != nil {
		return HTTPServerOptions{}, err
	}
	options.AuthMode = mode
	options.Authenticator = authenticator
	options.AdminPrincipals = invoke.FlagStrings(flags, "auth-admin")
	options.LoginBroker = identity.BrokerOf(authenticator)
	options.ServicePrincipal = identity.ServicePrincipalOf(authenticator)
	return options, nil
}

// serveSettings adapts `kc serve` flags to the authenticator settings shape.
type serveSettings map[string]FlagValue

func (s serveSettings) String(name string) string {
	return strings.TrimSpace(invoke.FlagString(map[string]FlagValue(s), name))
}

func (s serveSettings) Strings(name string) []string {
	return invoke.FlagStrings(map[string]FlagValue(s), name)
}
