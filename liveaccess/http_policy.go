package liveaccess

import (
	"context"
	"kc/kernel"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPStateLookupOptions binds trusted credential audiences to the runtime
// adapter. Object declarations select the target, never the proof policy.
type HTTPStateLookupOptions struct {
	Client            *http.Client
	CredentialOrigins []string
}

func NewHTTPStateLookupWithOptions(options HTTPStateLookupOptions) (*HTTPStateLookup, error) {
	client := options.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	allowed := make(map[string]bool, len(options.CredentialOrigins))
	for _, target := range options.CredentialOrigins {
		origin, err := credentialOrigin(target)
		if err != nil {
			return nil, err
		}
		allowed[origin] = true
	}
	return &HTTPStateLookup{client: client, credentialOrigins: allowed}, nil
}

func (h *HTTPStateLookup) WithCredentialOrigins(origins []string) (*HTTPStateLookup, error) {
	if h == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "LiveAccess HTTP adapter is unavailable")
	}
	return NewHTTPStateLookupWithOptions(HTTPStateLookupOptions{Client: h.client, CredentialOrigins: origins})
}

func credentialOrigin(target string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(target))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "LiveAccess credential target requires an HTTP origin")
	}
	origin, ok := canonicalOrigin(u)
	if !ok {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "LiveAccess credential target requires an HTTP origin")
	}
	return origin, nil
}

// canonicalOrigin is the exact comparison key: scheme, host and port, with the
// scheme's default port filled in. No wildcard, suffix or scheme upgrade.
func canonicalOrigin(u *url.URL) (string, bool) {
	if u == nil {
		return "", false
	}
	scheme, host, port := strings.ToLower(u.Scheme), strings.ToLower(u.Hostname()), u.Port()
	if host == "" {
		return "", false
	}
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		default:
			return "", false
		}
	}
	return scheme + "://" + net.JoinHostPort(host, port), true
}

// perform sends caller proof only to an exactly trusted origin. Any other
// origin is still called, without proof: a descriptor can name its source but
// cannot widen the deployment's proof audience.
func (h *HTTPStateLookup) perform(ctx context.Context, request *http.Request) (*http.Response, error) {
	creds := callerCredentialsFromContext(ctx)
	origin, ok := canonicalOrigin(request.URL)
	if !ok {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "LiveAccess target requires an HTTP origin")
	}
	if h.credentialOrigins[origin] {
		applyCallerCredentials(request, creds)
	}
	proofHeaders, authority := request.Header.Clone(), request.URL.Host
	client := *h.client
	previous := client.CheckRedirect
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		validTarget := func() bool {
			target, ok := canonicalOrigin(next.URL)
			return ok && next.URL.User == nil && (next.Host == "" || strings.EqualFold(next.Host, authority)) && target == origin
		}
		if !validTarget() {
			return kernel.Fail(kernel.ErrForbidden, "LiveAccess redirect changes the authentication target")
		}
		if previous != nil {
			if err := previous(next, via); err != nil {
				return err
			}
			if !validTarget() {
				return kernel.Fail(kernel.ErrForbidden, "LiveAccess redirect changes the authentication target")
			}
			restoreProofHeaders(next, proofHeaders)
			return nil
		}
		if len(via) >= 10 {
			return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "LiveAccess exceeded redirect limit")
		}
		restoreProofHeaders(next, proofHeaders)
		return nil
	}
	response, err := client.Do(request)
	// net/http wraps redirect failures with the source-supplied Location URL.
	// Preserve the underlying failure without retaining that untrusted URL.
	if failure, ok := err.(*url.Error); ok {
		err = failure.Err
	}
	return response, SafeFailure(ctx, err)
}

func restoreProofHeaders(next *http.Request, original http.Header) {
	for _, name := range []string{"Authorization", "X-Tai-Identity", "X-Resource-Principal", "X-Resource-On-Behalf-Of", "X-Resource-Request-Id"} {
		next.Header.Del(name)
		if value := original.Get(name); value != "" {
			next.Header.Set(name, value)
		}
	}
}
