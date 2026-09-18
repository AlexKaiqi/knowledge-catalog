// Package lakefs implements a layer ⓪ Snapshot authority over the lakeFS API.
package lakefs

import (
	"fmt"
	"net/url"
	"strings"

	"kc/snapshot"
)

const EnvCredential = "KC_LAKEFS_CREDENTIAL"

// Endpoint identifies one lakeFS repository. Credentials are deliberately
// excluded and are supplied by the Server-private credential provider.
type Endpoint struct {
	Origin     string
	API        string
	Repository string
}

// ParseDSN accepts http(s)://host[/prefix]/repository. The final path segment
// is the lakeFS repository name; an optional preceding path is the deployment
// prefix.
func ParseDSN(raw string) (Endpoint, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Endpoint{}, fmt.Errorf("lakefs dsn is required (http(s)://host/repository)")
	}
	if err := snapshot.RejectConfiguredSecret("lakefs", raw, EnvCredential); err != nil {
		return Endpoint{}, err
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return Endpoint{}, fmt.Errorf("lakefs dsn must be http(s)://host/repository")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) == 0 || parts[len(parts)-1] == "" {
		return Endpoint{}, fmt.Errorf("lakefs dsn must name one repository")
	}
	repository := parts[len(parts)-1]
	prefix := parts[:len(parts)-1]
	u.Path = ""
	origin := strings.TrimRight(u.String(), "/")
	if len(prefix) != 0 {
		origin += "/" + strings.Join(prefix, "/")
	}
	return Endpoint{Origin: origin, API: origin + "/api/v1", Repository: repository}, nil
}
