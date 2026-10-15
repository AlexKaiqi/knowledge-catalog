package object

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"kc/kernel"
)

// ResourceDescriptorKind is the body `kind` that makes an object a
// ResourceDescriptor: how one external system is accessed.
const ResourceDescriptorKind = "ResourceDescriptor"

// ResourceDescriptor is the parsed access description a unit's valueSource
// references. It carries no credentials: which origins may receive caller
// credentials is decided only by deployment configuration.
type ResourceDescriptor struct {
	Origin   string                      `json:"origin"`
	Mode     BindingMode                 `json:"mode"`
	Runtime  string                      `json:"runtime"`
	Protocol string                      `json:"protocol"`
	Access   map[string]BindingOperation `json:"access"`
}

// resourceDescriptorFields is the closed descriptor shape. Anything else
// (headers, tokens, nested definitions) is rejected so a descriptor cannot
// smuggle credentials or a second access description.
var resourceDescriptorFields = map[string]struct{}{
	"kind": {}, "name": {}, "description": {}, "origin": {}, "mode": {},
	"runtime": {}, "protocol": {}, "access": {},
}

// IsResourceDescriptorValue reports whether an entity body declares itself a
// ResourceDescriptor.
func IsResourceDescriptorValue(value any) bool {
	body, ok := value.(map[string]any)
	return ok && body["kind"] == ResourceDescriptorKind
}

// ParseResourceDescriptor validates one ResourceDescriptor body. Writer uses
// it before publication and LiveAccess again when it adopts the fixed version.
func ParseResourceDescriptor(id ObjectID, value any) (ResourceDescriptor, error) {
	body, ok := value.(map[string]any)
	if !ok || body["kind"] != ResourceDescriptorKind {
		return ResourceDescriptor{}, kernel.Fail(kernel.ErrUsageInvalid, "%s is not a ResourceDescriptor", id)
	}
	unknown := []string{}
	for key := range body {
		if _, allowed := resourceDescriptorFields[key]; !allowed {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return ResourceDescriptor{}, kernel.Fail(kernel.ErrUsageInvalid,
			"ResourceDescriptor %s has undeclared fields %s; descriptors carry no credentials or extra access settings", id, strings.Join(unknown, ", "))
	}
	text := func(name string) (string, error) {
		raw, exists := body[name]
		if !exists || raw == nil {
			return "", nil
		}
		value, ok := raw.(string)
		if !ok {
			return "", kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor %s %s must be a string", id, name)
		}
		return strings.TrimSpace(value), nil
	}
	out := ResourceDescriptor{}
	var err error
	for name, target := range map[string]*string{"runtime": &out.Runtime, "protocol": &out.Protocol} {
		if *target, err = text(name); err != nil {
			return ResourceDescriptor{}, err
		}
		if *target == "" {
			return ResourceDescriptor{}, kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor %s requires %s", id, name)
		}
	}
	origin, err := text("origin")
	if err != nil {
		return ResourceDescriptor{}, err
	}
	if out.Origin, err = ParseResourceAccessOrigin(origin); err != nil {
		return ResourceDescriptor{}, kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor %s: %v", id, err)
	}
	if out.Origin == "" {
		return ResourceDescriptor{}, kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor %s requires origin", id)
	}
	mode, err := text("mode")
	if err != nil {
		return ResourceDescriptor{}, err
	}
	out.Mode = BindingMode(mode)
	if out.Mode != BindingState && out.Mode != BindingStream {
		return ResourceDescriptor{}, kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor %s mode must be state or stream", id)
	}
	access, ok := body["access"].(map[string]any)
	if !ok || len(access) == 0 {
		return ResourceDescriptor{}, kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor %s requires access operations", id)
	}
	out.Access = make(map[string]BindingOperation, len(access))
	for name, raw := range access {
		operation, ok := raw.(map[string]any)
		if !ok {
			return ResourceDescriptor{}, kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor %s operation %s must be {call, description?}", id, name)
		}
		for key := range operation {
			if key != "call" && key != "description" {
				return ResourceDescriptor{}, kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor %s operation %s has undeclared field %s", id, name, key)
			}
		}
		call, _ := operation["call"].(string)
		if strings.TrimSpace(name) == "" || strings.TrimSpace(call) == "" {
			return ResourceDescriptor{}, kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor %s operations require non-empty names and calls", id)
		}
		out.Access[name] = BindingOperation{Call: strings.TrimSpace(call)}
	}
	return out, nil
}

// ParseResourceAccessOrigin accepts a ResourceDescriptor `origin`: an http(s)
// service origin without /v1/access, credentials, query, or fragment.
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

// ResourceAccessEndpoint appends the resource-access/v1 path to a descriptor origin.
func ResourceAccessEndpoint(origin string) (string, error) {
	origin, err := ParseResourceAccessOrigin(origin)
	if err != nil {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "%s", err)
	}
	if origin == "" {
		return "", kernel.Fail(kernel.ErrCapabilityUnsatisfied, "resource access requires a ResourceDescriptor origin")
	}
	return origin + "/v1/access", nil
}
