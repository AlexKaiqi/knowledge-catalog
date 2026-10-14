package handler

import (
	"strings"

	"kc/kernel"
	objectmodel "kc/object"
)

// ObjectInterfacesAspect is the ordinary collection whose Members declare
// independently selectable implementations of object capabilities.
const ObjectInterfacesAspect = "interfaces"

// InterfaceOperation describes logical input/output contracts, not transport
// parameters. A write operation requires explicit execution authorization.
type InterfaceOperation struct {
	InputSchemaRef  string `json:"inputSchemaRef,omitempty"`
	ResultSchemaRef string `json:"resultSchemaRef,omitempty"`
	Effect          string `json:"effect"`
}

// InterfaceDefinition is versioned objectmodel. Executable handlers are installed
// independently and must declare support for this exact contract digest.
type InterfaceDefinition struct {
	ObjectID                      objectmodel.ObjectID          `json:"objectId"`
	Name                          string                        `json:"name"`
	Meaning                       string                        `json:"meaning"`
	StandardImplementationSchemas []string                      `json:"standardImplementationSchemas"`
	Operations                    map[string]InterfaceOperation `json:"operations"`
	Digest                        kernel.Digest                 `json:"digest"`
}

// InterfaceImplementation retains the domain-specific declaration unchanged.
// The selected implementation Schema, not a Go business type registry, checks it.
type InterfaceImplementation struct {
	InterfaceRef string         `json:"interfaceRef"`
	Value        map[string]any `json:"value"`
}

func ParseInterfaceImplementation(value any) (InterfaceImplementation, error) {
	body, ok := value.(map[string]any)
	if !ok {
		return InterfaceImplementation{}, kernel.Fail(kernel.ErrSchemaInstanceInvalid, "plugin implementation must be an object")
	}
	ref, ok := body["interfaceRef"].(string)
	if !ok || strings.TrimSpace(ref) == "" {
		return InterfaceImplementation{}, kernel.Fail(kernel.ErrSchemaInstanceInvalid, "plugin implementation requires interfaceRef")
	}
	return InterfaceImplementation{InterfaceRef: strings.TrimSpace(ref), Value: body}, nil
}

func ParseInterfaceDefinition(id objectmodel.ObjectID, value any) (InterfaceDefinition, error) {
	body, ok := value.(map[string]any)
	if !ok {
		return InterfaceDefinition{}, interfaceInvalid(id, "definition must be an object")
	}
	if definition, nested := body["definition"].(map[string]any); nested {
		body = definition
	}
	name, _ := body["name"].(string)
	meaning, _ := body["meaning"].(string)
	if strings.TrimSpace(name) == "" || strings.TrimSpace(meaning) == "" {
		return InterfaceDefinition{}, interfaceInvalid(id, "name and meaning are required")
	}
	rawOperations, ok := body["operations"].(map[string]any)
	if !ok || len(rawOperations) == 0 {
		return InterfaceDefinition{}, interfaceInvalid(id, "operations must be a nonempty object")
	}
	out := InterfaceDefinition{ObjectID: id, Name: name, Meaning: meaning, Operations: map[string]InterfaceOperation{}, Digest: kernel.CanonicalDigest(body)}
	if raw, exists := body["standardImplementationSchemas"]; exists {
		schemas, err := interfaceSchemaRefs(raw)
		if err != nil {
			return InterfaceDefinition{}, interfaceInvalid(id, "standardImplementationSchemas must contain schema references")
		}
		for _, ref := range schemas {
			if _, valid := objectmodel.ParseSchemaRef(ref); !valid {
				return InterfaceDefinition{}, interfaceInvalid(id, "standardImplementationSchemas contains invalid schema reference %q", ref)
			}
		}
		out.StandardImplementationSchemas = schemas
	}
	for name, raw := range rawOperations {
		operation, err := parseInterfaceOperation(id, name, raw)
		if err != nil {
			return InterfaceDefinition{}, err
		}
		out.Operations[name] = operation
	}
	return out, nil
}

func parseInterfaceOperation(id objectmodel.ObjectID, name string, raw any) (InterfaceOperation, error) {
	body, ok := raw.(map[string]any)
	if strings.TrimSpace(name) == "" || !ok {
		return InterfaceOperation{}, interfaceInvalid(id, "operation names must be nonempty and values must be objects")
	}
	out := InterfaceOperation{Effect: "read"}
	for key, target := range map[string]*string{"inputSchemaRef": &out.InputSchemaRef, "resultSchemaRef": &out.ResultSchemaRef, "effect": &out.Effect} {
		value, exists := body[key]
		if !exists {
			continue
		}
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return InterfaceOperation{}, interfaceInvalid(id, "operation %s %s must be a nonempty string", name, key)
		}
		*target = strings.TrimSpace(text)
	}
	if out.Effect != "read" && out.Effect != "write" {
		return InterfaceOperation{}, interfaceInvalid(id, "operation %s effect must be read or write", name)
	}
	return out, nil
}

func interfaceInvalid(id objectmodel.ObjectID, format string, args ...any) error {
	return kernel.Fail(kernel.ErrSchemaInstanceInvalid, "interface %s: "+format, append([]any{id}, args...)...)
}
