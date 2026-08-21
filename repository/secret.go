package repository

import (
	"fmt"
	"net/url"
	"strings"
)

// RejectConfiguredSecret refuses passwords or API keys in connection text.
func RejectConfiguredSecret(kind, raw, env string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "password=") || strings.Contains(lower, "api_key=") || strings.Contains(lower, "apikey=") {
		return fmt.Errorf("%s connection config must not contain secrets; set %s", kind, env)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	if u.User != nil {
		if _, ok := u.User.Password(); ok {
			return fmt.Errorf("%s connection config must not contain secrets; set %s", kind, env)
		}
	}
	return nil
}
