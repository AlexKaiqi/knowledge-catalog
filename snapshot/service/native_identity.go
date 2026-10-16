package service

import (
	"net/http"
	"net/url"
	"strings"

	"kc/kernel"
)

// nativeStorageProof is the Server's private storage credential. lakeFS is
// storage, not an identity provider: caller proof is never forwarded to it.
func (h *handler) nativeStorageProof(r *http.Request) (http.Header, error) {
	if h.config.NativeCredential == nil {
		return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "native storage credential is required for external authentication")
	}
	credential, err := h.config.NativeCredential(r.Context())
	if err != nil {
		return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "native storage credential is unavailable")
	}
	key, secret, ok := strings.Cut(credential, ":")
	if !ok || key == "" || secret == "" || strings.ContainsAny(key+secret, "\r\n") {
		return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "native storage credential is not configured")
	}
	proof := &http.Request{Header: http.Header{}}
	proof.SetBasicAuth(key, secret)
	return proof.Header, nil
}

// Service-backed consoles expose no storage-account login, setup or credential
// API. Only setup discovery is public; runtime config requires actual user proof.
func (h *handler) nativeServicePublic(w http.ResponseWriter, r *http.Request, target *url.URL) bool {
	if !nativePublicRequest(r) {
		return false
	}
	if r.Method != http.MethodGet {
		writeNativeError(w, kernel.Fail(kernel.ErrForbidden, "authenticate with the configured identity provider; storage setup is operator-only"))
		return true
	}
	switch r.URL.Path {
	case "/api/v1/setup_lakefs":
		var state struct {
			State string `json:"state"`
		}
		if err := fetchNativeJSON(r.Context(), target, "/setup_lakefs", nil, nil, &state); err != nil || state.State != "initialized" {
			writeNativeError(w, kernel.Fail(kernel.ErrTemporaryUnavailable, "native backend must be initialized by its operator"))
			return true
		}
		logout := h.config.Identity.LogoutURL
		if logout == "" {
			logout = "/ui/auth/login"
		}
		writeJSON(w, 200, map[string]any{"state": state.State, "comm_prefs_missing": false, "login_config": map[string]any{
			"RBAC": "external", "snapshot_identity": true, "provider_url": h.config.Identity.ProviderURL,
			"login_url": "", "login_cookie_names": []string{}, "logout_url": logout,
		}})
	case "/api/v1/auth/capabilities":
		writeJSON(w, 200, map[string]any{"invite": false, "external_principals_enabled": false})
	case "/api/v1/config":
		if _, err := authenticateRequest(r, h.config.Identity, false); err != nil {
			writeNativeError(w, err)
			return true
		}
		proof, err := h.nativeStorageProof(r)
		if err != nil {
			writeNativeError(w, err)
			return true
		}
		relayNative(w, r, target, proof, false, h.nativeResponseDecoration(h.decorateNativeConfig))
	}
	return true
}
