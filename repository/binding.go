package repository

import (
	"strings"

	"kc/kernel"
)

// ValueSourceKind distinguishes a value stored in the Snapshot from a value
// observed through a stable Binding declaration. The default when omitted is
// Snapshot.
type ValueSourceKind string

const (
	ValueSourceSnapshot ValueSourceKind = "snapshot"
	ValueSourceBinding  ValueSourceKind = "binding"
)

// BindingMode is the observation shape. State returns at most one current
// value per Address; Stream returns records whose schema_ref describes one
// record, never an unbounded Aspect array.
type BindingMode string

const (
	BindingState  BindingMode = "state"
	BindingStream BindingMode = "stream"
)

// BindingOperation is a logical operation exported by a runtime protocol.
// Endpoint, credentials and physical topology are intentionally absent.
type BindingOperation struct {
	Call string `json:"call"`
}

// BindingDeclaration is either inline or a reference to a ResourceDescriptor
// in the same Repository commit. Inline fields and DescriptorRef are mutually
// exclusive.
type BindingDeclaration struct {
	Mode          BindingMode                 `json:"mode"`
	Runtime       string                      `json:"runtime,omitempty"`
	Protocol      string                      `json:"protocol,omitempty"`
	Operations    map[string]BindingOperation `json:"operations,omitempty"`
	DescriptorRef kernel.ObjectID             `json:"descriptorRef,omitempty"`
}

type ValueSource struct {
	Kind    ValueSourceKind     `json:"kind"`
	Binding *BindingDeclaration `json:"binding,omitempty"`
}

func (s *ValueSource) Normalized() *ValueSource {
	if s == nil || s.Kind == "" || s.Kind == ValueSourceSnapshot {
		return nil
	}
	return s
}

// ValidateValueSource validates declaration shape only. Resolving a
// DescriptorRef requires a pinned Repository and is performed by Reader.
func ValidateValueSource(source *ValueSource) error {
	if source == nil || source.Kind == "" || source.Kind == ValueSourceSnapshot {
		if source != nil && source.Binding != nil {
			return kernel.Fail(kernel.ErrUsageInvalid, "snapshot value_source cannot contain binding")
		}
		return nil
	}
	if source.Kind != ValueSourceBinding || source.Binding == nil {
		return kernel.Fail(kernel.ErrUsageInvalid, "value_source kind must be snapshot or binding")
	}
	b := source.Binding
	if b.Mode != BindingState && b.Mode != BindingStream {
		return kernel.Fail(kernel.ErrUsageInvalid, "binding mode must be state or stream")
	}
	ref := strings.TrimSpace(string(b.DescriptorRef))
	inline := strings.TrimSpace(b.Runtime) != "" || strings.TrimSpace(b.Protocol) != "" || len(b.Operations) > 0
	if ref != "" && inline {
		return kernel.Fail(kernel.ErrUsageInvalid, "binding must use either descriptorRef or inline runtime/protocol/operations")
	}
	if ref != "" {
		return nil
	}
	if strings.TrimSpace(b.Runtime) == "" || strings.TrimSpace(b.Protocol) == "" || len(b.Operations) == 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "inline binding requires runtime, protocol and operations")
	}
	for name, operation := range b.Operations {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(operation.Call) == "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "binding operations require non-empty names and calls")
		}
	}
	return nil
}

// UnitDeclaration is the versioned declaration envelope for one Address.
type UnitDeclaration struct {
	Address           kernel.Address `json:"address"`
	Digest            kernel.Digest  `json:"digest"`
	DeclarationDigest kernel.Digest  `json:"declarationDigest"`
	SchemaRef         string         `json:"schemaRef,omitempty"`
	ValueSource       *ValueSource   `json:"valueSource,omitempty"`
}

func DeclarationDigest(schemaRef string, source *ValueSource) kernel.Digest {
	return kernel.CanonicalDigest(map[string]any{
		"schemaRef":   strings.TrimSpace(schemaRef),
		"valueSource": source.Normalized(),
	})
}
