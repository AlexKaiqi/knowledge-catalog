package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot/access"
)

func (h *handler) nativeAPI(w http.ResponseWriter, r *http.Request) {
	if !safeNativePath(r.URL.Path) {
		writeNativeError(w, kernel.Fail(kernel.ErrForbidden, "invalid native path"))
		return
	}
	origin, err := identity.VerifierOrigin(h.config.NativeOrigin, "native API")
	if err != nil || h.config.NativeOrigin == "" {
		writeNativeError(w, kernel.Fail(kernel.ErrTemporaryUnavailable, "native API is not configured"))
		return
	}
	target, _ := url.Parse(origin)
	if h.nativeServiceIdentity() && h.nativeServicePublic(w, r, target) {
		return
	}
	if nativePublicRequest(r) {
		var proof http.Header
		if nativeHasProof(r) {
			proof, err = lakeFSCallerProof(r.Header)
			if err != nil {
				writeNativeError(w, err)
				return
			}
		}
		relayNative(w, r, target, proof, true, h.decorateNativeConfig)
		return
	}
	r = nativeNavigationRequest(r)
	caller, err := authenticateRequest(r, h.config.Identity, nativeUploadRequest(r))
	if err != nil {
		if kernel.CodeOf(err) == kernel.ErrPreconditionFailed {
			err = kernel.Fail(kernel.ErrTemporaryUnavailable, "native identity binding is unavailable")
		}
		writeNativeError(w, err)
		return
	}
	if !h.nativeServiceIdentity() && (caller.User == nil || caller.User.Provider != "lakefs" || caller.User.Issuer != origin) {
		writeNativeError(w, kernel.Fail(kernel.ErrForbidden, "native API requires proof from its configured authentication instance"))
		return
	}
	if nativeAccountRequest(r) {
		if h.nativeServiceIdentity() {
			if r.URL.Path == "/api/v1/user" && r.Method == http.MethodGet {
				writeJSON(w, 200, map[string]any{"user": map[string]string{"id": caller.Principal}})
			} else {
				writeNativeError(w, kernel.Fail(kernel.ErrForbidden, "accounts and credentials belong to the configured identity provider"))
			}
			return
		}
		proof, err := h.nativeStorageProof(r)
		if err != nil {
			writeNativeError(w, err)
			return
		}
		relayNative(w, r, target, proof, true, nil)
		return
	}
	proof, err := h.nativeStorageProof(r)
	if err != nil {
		writeNativeError(w, err)
		return
	}
	route, values, ok := matchNativeRoute(r)
	if !ok {
		writeNativeError(w, kernel.Fail(kernel.ErrForbidden, "native operation has no product authorization mapping"))
		return
	}
	operation := "native:" + route.action
	response := &nativeResponseWriter{ResponseWriter: w}
	w = response
	ctx, finish := h.observeRequest(r, caller, operation)
	var result any
	if finish != nil {
		defer func() {
			if err == nil && response.status >= 400 {
				err = kernel.Fail(kernel.ErrTemporaryUnavailable, "native operation returned HTTP %d", response.status)
			}
			finish(result, err)
		}()
	}
	if h.config.Acquire == nil {
		writeNativeError(w, kernel.Fail(kernel.ErrTemporaryUnavailable, "native authorization resources are not configured"))
		return
	}
	resources, release, err := h.config.Acquire(ctx, caller, operation)
	if release != nil {
		defer release()
	}
	if err == nil {
		result, err = h.serveNativeRepository(w, r.WithContext(ctx), caller, resources, target, proof, route, values)
	}
	if err != nil && response.status == 0 {
		writeNativeError(w, err)
	}
}

func safeNativePath(path string) bool {
	if strings.ContainsAny(path, "\\\x00") {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if part == "." || part == ".." {
			return false
		}
	}
	return true
}

func nativeHasProof(r *http.Request) bool {
	if r.Header.Get("Authorization") != "" {
		return true
	}
	for _, cookie := range r.Cookies() {
		switch cookie.Name {
		case "internal_auth_session", "oidc_auth_session", "saml_auth_session":
			return true
		}
	}
	return false
}

func (h *handler) serveNativeRepository(w http.ResponseWriter, r *http.Request, caller identity.HTTPIdentity, resources Resources, target *url.URL, proof http.Header, route nativeRoute, values map[string]string) (any, error) {
	if route.path == "/repositories" && r.Method == http.MethodGet {
		return nativeInventory(w, r, caller, resources, target, proof)
	}
	if route.action == access.RepositoryCreate {
		return createNativeRepository(w, r, caller, resources, target, proof)
	}
	repository := values["repository"]
	if len(route.refs) == 0 {
		if err := authorizeNative(r.Context(), caller, resources, repository, route.action, ""); err != nil {
			return nil, err
		}
	}
	for _, name := range route.refs {
		action := route.action
		if route.action == access.RepositoryMerge && name == "source" {
			action = access.RepositoryHistoryRead
		}
		if err := authorizeNative(r.Context(), caller, resources, repository, action, values[name]); err != nil {
			return nil, err
		}
	}
	if err := nativeSourceChecks(r, caller, resources, target, proof, route, values); err != nil {
		return nil, err
	}
	if route.path == "/repositories/{repository}/pulls/{pull}/merge" {
		if err := nativePullMergeChecks(r, caller, resources, target, proof, values); err != nil {
			return nil, err
		}
	}
	if route.action == access.RepositoryDelete {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "registered repository must be detached from deployment bindings and Dataset sources before physical deletion; no detach lifecycle is configured")
	}
	if route.path == "/repositories/{repository}/settings/gc_rules" && r.Method != http.MethodGet && r.Method != http.MethodHead {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "registered authority must retain committed history; no retention policy validator is configured")
	}
	// Download links remain under current product authorization. Native storage
	// presigned URLs would let later reads bypass this request boundary.
	query := r.URL.Query()
	query.Set("presign", "false")
	r.URL.RawQuery = query.Encode()
	relayNative(w, r, target, proof, false, h.nativeResponseDecoration(nil))
	return map[string]any{"repository": resources.NativeRepositories[repository], "action": route.action}, nil
}

func (h *handler) nativeResponseDecoration(next func(*http.Response) error) func(*http.Response) error {
	return func(response *http.Response) error {
		if h.nativeServiceIdentity() && (response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden) {
			return kernel.Fail(kernel.ErrTemporaryUnavailable, "private storage authorization is unavailable")
		}
		if next != nil {
			return next(response)
		}
		return nil
	}
}

func authorizeNative(ctx context.Context, caller identity.HTTPIdentity, resources Resources, name, action, ref string) error {
	id, ok := resources.NativeRepositories[name]
	if !ok || id == "" {
		return kernel.Fail(kernel.ErrForbidden, "repository is not registered for this native instance")
	}
	return resources.authorize(ctx, Authorization{Caller: caller, NativeRepository: true, Query: access.Query{Action: action, Repo: string(id), Ref: ref}})
}

func nativePublicRequest(r *http.Request) bool {
	switch r.URL.Path {
	case "/api/v1/auth/login":
		return r.Method == "POST"
	case "/api/v1/setup_lakefs":
		return r.Method == "GET" || r.Method == "POST"
	case "/api/v1/setup_comm_prefs":
		return r.Method == "POST"
	case "/api/v1/config", "/api/v1/auth/capabilities":
		return r.Method == "GET"
	default:
		return false
	}
}

func nativeAccountRequest(r *http.Request) bool {
	// Native account policy continues to be enforced by the identity provider.
	// This prefix has no repository operations and cannot grant product access.
	return r.URL.Path == "/api/v1/user" || strings.HasPrefix(r.URL.Path, "/api/v1/auth/") ||
		r.Method == "GET" && r.URL.Path == "/api/v1/statistics"
}

func relayNative(w http.ResponseWriter, r *http.Request, target *url.URL, proof http.Header, account bool, modify func(*http.Response) error) {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(p *httputil.ProxyRequest) {
			p.SetURL(target)
			p.Out.Header.Del("Authorization")
			p.Out.Header.Del("Cookie")
			for name := range p.Out.Header {
				if strings.HasPrefix(strings.ToLower(name), "x-kc-") || strings.HasPrefix(strings.ToLower(name), "x-forwarded-") || strings.HasPrefix(strings.ToLower(name), "x-tai-") {
					p.Out.Header.Del(name)
				}
			}
			for key, values := range proof {
				p.Out.Header[key] = append([]string(nil), values...)
			}
		},
		ModifyResponse: func(response *http.Response) error {
			if !account {
				response.Header.Del("Set-Cookie")
			}
			if modify != nil {
				if err := modify(response); err != nil {
					return err
				}
			}
			if !account && response.StatusCode >= 300 && response.StatusCode < 400 {
				return kernel.Fail(kernel.ErrForbidden, "native content redirects cannot bypass product authorization")
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeNativeError(w, kernel.Fail(kernel.ErrTemporaryUnavailable, "native backend request failed"))
		},
	}
	proxy.ServeHTTP(w, r)
}

func writeNativeError(w http.ResponseWriter, err error) {
	status := errorStatus(err)
	message := err.Error()
	if status == http.StatusUnauthorized {
		message = "error authenticating request"
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"message": message, "code": kernel.CodeOf(err)})
}
