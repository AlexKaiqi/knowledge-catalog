package object

import (
	"bytes"
	"encoding/json"
	"strings"

	"kc/kernel"
)

type ValueSourceKind string

const (
	ValueSourceSnapshot ValueSourceKind = "snapshot"
	ValueSourceBinding  ValueSourceKind = "binding"
)

// BindingMode is declared by a ResourceDescriptor, never by a unit.
type BindingMode string

const (
	BindingState  BindingMode = "state"
	BindingStream BindingMode = "stream"

	ProtocolResourceAccessV1 = "resource-access/v1"
)

// BindingOperation is one call a ResourceDescriptor declares.
type BindingOperation struct {
	Call string `json:"call"`
}

// BindingDeclaration names the ResourceDescriptor that supplies a unit's
// value. How the external system is accessed lives only in that descriptor.
type BindingDeclaration struct {
	DescriptorRef ObjectID `json:"descriptorRef"`
}

// ValueSource is declared on an Aspect or Member unit. It is decoded strictly:
// an inline access description is a different (retired) shape and rejected.
type ValueSource struct {
	Kind    ValueSourceKind     `json:"kind"`
	Binding *BindingDeclaration `json:"binding,omitempty"`
}

func (s *ValueSource) UnmarshalJSON(raw []byte) error {
	type plain ValueSource
	var decoded plain
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return kernel.Fail(kernel.ErrUsageInvalid,
			"valueSource accepts only {kind, binding: {descriptorRef}}; access is described by a ResourceDescriptor: %v", err)
	}
	*s = ValueSource(decoded)
	return nil
}

func (s *ValueSource) Normalized() *ValueSource {
	if s == nil || s.Kind == "" || s.Kind == ValueSourceSnapshot {
		return nil
	}
	return s
}

// IsBinding reports whether the unit's value comes from a ResourceDescriptor.
func (s *ValueSource) IsBinding() bool {
	return s != nil && s.Kind == ValueSourceBinding && s.Binding != nil
}

func ValidateValueSource(source *ValueSource) error {
	if source == nil || source.Kind == "" || source.Kind == ValueSourceSnapshot {
		if source != nil && source.Binding != nil {
			return kernel.Fail(kernel.ErrUsageInvalid, "snapshot valueSource cannot contain binding")
		}
		return nil
	}
	if source.Kind != ValueSourceBinding || source.Binding == nil {
		return kernel.Fail(kernel.ErrUsageInvalid, "valueSource kind must be snapshot or binding")
	}
	ref := ObjectID(strings.TrimSpace(string(source.Binding.DescriptorRef)))
	if ref == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "binding requires descriptorRef naming a ResourceDescriptor")
	}
	if IsSchemaObject(ref) {
		return kernel.Fail(kernel.ErrUsageInvalid, "descriptorRef %s names a schema, not a ResourceDescriptor", ref)
	}
	return ValidateObjectID(ref)
}

type UnitDeclaration struct {
	Address           Address       `json:"address"`
	Digest            kernel.Digest `json:"digest"`
	DeclarationDigest kernel.Digest `json:"declarationDigest"`
	SchemaRef         string        `json:"schemaRef,omitempty"`
	ValueSource       *ValueSource  `json:"valueSource,omitempty"`
	// Provenance belongs to this Address at the returned fixed commit.
	Provenance *ProvenanceEnvelope `json:"provenance,omitempty"`
}

// DeclarationDigest versions the unit side of a declaration. LiveAccess
// combines it with the resolved descriptor's digest.
func DeclarationDigest(schemaRef string, source *ValueSource) kernel.Digest {
	return kernel.CanonicalDigest(map[string]any{
		"schemaRef":   strings.TrimSpace(schemaRef),
		"valueSource": source.Normalized(),
	})
}

// BindingDeclarationDigest is the declaration version of one binding unit. It
// changes when either the unit declaration or the adopted ResourceDescriptor
// version changes.
func BindingDeclarationDigest(unit, descriptor kernel.Digest) kernel.Digest {
	return kernel.CanonicalDigest(map[string]any{"unit": string(unit), "descriptor": string(descriptor)})
}
