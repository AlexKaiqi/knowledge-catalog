package home

import (
	"net/url"
	"strings"

	"kc/kernel"
)

// The existing authentication gateway owns session termination. This optional
// URL is deployment input, never a request-controlled redirect or a KC session.
func validateAuthLogoutURL(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || strings.ContainsAny(raw, "\\\r\n") {
		return kernel.Fail(kernel.ErrUsageInvalid, "authLogoutURL must be a trusted HTTP URL or same-origin path")
	}
	if u.Scheme == "" && u.Host == "" && strings.HasPrefix(u.Path, "/") && !strings.HasPrefix(raw, "//") {
		return nil
	}
	if (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		return nil
	}
	return kernel.Fail(kernel.ErrUsageInvalid, "authLogoutURL must be a trusted HTTP URL or same-origin path")
}
