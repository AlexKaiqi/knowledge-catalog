package identity

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// VerifierOrigin is a deployment-selected endpoint, never a request parameter.
func VerifierOrigin(origin, provider string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(origin))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("--auth-url must be a %s http(s) origin", provider)
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", fmt.Errorf("--auth-url must not contain credentials, query, or fragment")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(u.Path, "/api/v1") {
		return "", fmt.Errorf("--auth-url is the %s origin, without /api/v1", provider)
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func DecodeVerifierUser(body io.Reader, target any, exact bool) error {
	reader := io.LimitReader(body, 1<<20)
	if exact {
		data, err := io.ReadAll(io.LimitReader(body, (1<<20)+1))
		if err != nil {
			return err
		}
		if len(data) > 1<<20 {
			return fmt.Errorf("verification response exceeds its limit")
		}
		reader = io.LimitReader(strings.NewReader(string(data)), 1<<20)
	}
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if exact {
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return fmt.Errorf("verification response must contain one JSON value")
		}
	}
	return nil
}
