package gitea

import (
	"fmt"
	"net/url"
	"strings"

	"kc/snapshot"
)

const EnvToken = "KC_GITEA_TOKEN"

// Endpoint is a Gitea Repository address without credentials.
type Endpoint struct {
	Origin string
	API    string
	Owner  string
	Name   string
}

// ParseDSN reads http(s)://{origin}/{owner}/{name}. Extra path prefix is part of origin
// (subpath install). Token must not appear in the URL.
func ParseDSN(raw string) (Endpoint, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Endpoint{}, fmt.Errorf("gitea dsn is required (http(s)://host/owner/name)")
	}
	if err := snapshot.RejectConfiguredSecret("gitea", raw, EnvToken); err != nil {
		return Endpoint{}, err
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return Endpoint{}, fmt.Errorf("gitea dsn must be http(s)://host/owner/name")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return Endpoint{}, fmt.Errorf("gitea dsn must be http(s)://host/owner/name")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[len(parts)-1] == "" {
		return Endpoint{}, fmt.Errorf("gitea dsn must include /owner/name")
	}
	owner := parts[len(parts)-2]
	name := parts[len(parts)-1]
	prefix := parts[:len(parts)-2]
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	if len(prefix) == 0 {
		u.Path = ""
	} else {
		u.Path = "/" + strings.Join(prefix, "/")
	}
	origin := strings.TrimRight(u.String(), "/")
	return Endpoint{
		Origin: origin,
		API:    origin + "/api/v1",
		Owner:  owner,
		Name:   name,
	}, nil
}

func (e Endpoint) repoPath(suffix string) string {
	base := "/repos/" + url.PathEscape(e.Owner) + "/" + url.PathEscape(e.Name)
	if suffix == "" {
		return base
	}
	if strings.HasPrefix(suffix, "?") {
		return base + suffix
	}
	return base + "/" + strings.TrimPrefix(suffix, "/")
}
