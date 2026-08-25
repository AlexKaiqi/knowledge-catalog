package knowledge

import (
	"strings"

	"kc/kernel"
)

type ValueSourceKind string

const (
	ValueSourceSnapshot ValueSourceKind = "snapshot"
	ValueSourceBinding  ValueSourceKind = "binding"
)

type BindingMode string

const (
	BindingState  BindingMode = "state"
	BindingStream BindingMode = "stream"
)

type BindingOperation struct {
	Call string `json:"call"`
}

type BindingDeclaration struct {
	Mode          BindingMode                 `json:"mode"`
	Runtime       string                      `json:"runtime,omitempty"`
	Protocol      string                      `json:"protocol,omitempty"`
	Operations    map[string]BindingOperation `json:"operations,omitempty"`
	DescriptorRef ObjectID                    `json:"descriptorRef,omitempty"`
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
	binding := source.Binding
	if binding.Mode != BindingState && binding.Mode != BindingStream {
		return kernel.Fail(kernel.ErrUsageInvalid, "binding mode must be state or stream")
	}
	ref := strings.TrimSpace(string(binding.DescriptorRef))
	inline := strings.TrimSpace(binding.Runtime) != "" || strings.TrimSpace(binding.Protocol) != "" || len(binding.Operations) > 0
	if ref != "" && inline {
		return kernel.Fail(kernel.ErrUsageInvalid, "binding must use either descriptorRef or inline runtime/protocol/operations")
	}
	if ref != "" {
		return nil
	}
	if strings.TrimSpace(binding.Runtime) == "" || strings.TrimSpace(binding.Protocol) == "" || len(binding.Operations) == 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "inline binding requires runtime, protocol and operations")
	}
	for name, operation := range binding.Operations {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(operation.Call) == "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "binding operations require non-empty names and calls")
		}
	}
	return nil
}

type UnitDeclaration struct {
	Address           Address       `json:"address"`
	Digest            kernel.Digest `json:"digest"`
	DeclarationDigest kernel.Digest `json:"declarationDigest"`
	SchemaRef         string        `json:"schemaRef,omitempty"`
	ValueSource       *ValueSource  `json:"valueSource,omitempty"`
}

func DeclarationDigest(schemaRef string, source *ValueSource) kernel.Digest {
	return kernel.CanonicalDigest(map[string]any{
		"schemaRef":   strings.TrimSpace(schemaRef),
		"valueSource": source.Normalized(),
	})
}
