package knowledge

import (
	"fmt"
	"net/url"
	"strings"

	"kc/kernel"
)

// ParseResourceAccessOrigin accepts the Domain Schema `origin` field: an
// http(s) service origin without /v1/access, credentials, query, or fragment.
// Empty input is valid and means the schema does not host Bound State.
func ParseResourceAccessOrigin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("origin must be an http(s) origin")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("origin must not contain credentials, query, or fragment")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(u.Path, "/v1/access") {
		return "", fmt.Errorf("origin is the service origin, without /v1/access")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

// ResourceAccessEndpoint appends the resource-access/v1 path to a Schema origin.
func ResourceAccessEndpoint(origin string) (string, error) {
	origin, err := ParseResourceAccessOrigin(origin)
	if err != nil {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "%s", err)
	}
	if origin == "" {
		return "", kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Bound State requires a schema origin")
	}
	return origin + "/v1/access", nil
}

// AttachSchemaOrigin copies a Canonical frontmatter origin onto a Schema body
// so ParseSchemaDefinition sees one assembled document. Frontmatter wins when
// both are present and they must name the same origin.
func AttachSchemaOrigin(value any, origin string) (any, error) {
	origin, err := ParseResourceAccessOrigin(origin)
	if err != nil {
		return nil, err
	}
	if origin == "" {
		return value, nil
	}
	body, ok := value.(map[string]any)
	if !ok {
		if value != nil {
			return nil, fmt.Errorf("schema origin requires an object body")
		}
		body = map[string]any{}
	}
	existing, _, err := optionalSchemaString(body, "origin")
	if err != nil {
		return nil, err
	}
	if existing != "" {
		existing, err = ParseResourceAccessOrigin(existing)
		if err != nil {
			return nil, err
		}
		if existing != origin {
			return nil, fmt.Errorf("frontmatter origin and body origin disagree")
		}
	}
	body["origin"] = origin
	return body, nil
}

// SplitSchemaOrigin pulls origin out of a Schema body so Canonical files can
// store the access path in frontmatter instead of the business document.
func SplitSchemaOrigin(value any) (string, any, error) {
	body, ok := value.(map[string]any)
	if !ok {
		return "", value, nil
	}
	origin, present, err := optionalSchemaString(body, "origin")
	if err != nil {
		return "", nil, err
	}
	if !present || origin == "" {
		return "", value, nil
	}
	origin, err = ParseResourceAccessOrigin(origin)
	if err != nil {
		return "", nil, err
	}
	out := make(map[string]any, len(body))
	for key, item := range body {
		if key == "origin" {
			continue
		}
		out[key] = item
	}
	return origin, out, nil
}
