package knowledge

import (
	"strings"
	"time"

	"kc/kernel"
)

// ObservationConsistency states what a runtime can prove about a dynamic
// value. It is deliberately separate from a Repository commit: a Workspace
// pin freezes the Binding declaration, not the value observed through it.
type ObservationConsistency string

const (
	ObservationRepeatable ObservationConsistency = "repeatable"
	ObservationBounded    ObservationConsistency = "bounded"
	ObservationLatestOnly ObservationConsistency = "latest-only"
)

// ObservationBasis identifies the runtime generation and source position used
// for one State observation. SourceRevision and Watermark are optional because
// not every source exposes them; ObservedAt must never be used as a substitute
// for either one.
type ObservationBasis struct {
	BindingGeneration string                 `json:"bindingGeneration"`
	Consistency       ObservationConsistency `json:"consistency"`
	SourceRevision    string                 `json:"sourceRevision,omitempty"`
	Watermark         string                 `json:"watermark,omitempty"`
	ObservedAt        string                 `json:"observedAt"`
}

func ValidateObservationBasis(basis ObservationBasis) error {
	if strings.TrimSpace(basis.BindingGeneration) == "" {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "State observation requires bindingGeneration")
	}
	for _, field := range []struct{ name, value string }{
		{name: "bindingGeneration", value: basis.BindingGeneration},
		{name: "sourceRevision", value: basis.SourceRevision},
		{name: "watermark", value: basis.Watermark},
		{name: "observedAt", value: basis.ObservedAt},
	} {
		if strings.TrimSpace(field.value) != field.value {
			return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "State observation %s must be trimmed", field.name)
		}
	}
	switch basis.Consistency {
	case ObservationRepeatable, ObservationBounded, ObservationLatestOnly:
	default:
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "State observation consistency must be repeatable, bounded, or latest-only")
	}
	observedAt := strings.TrimSpace(basis.ObservedAt)
	if observedAt == "" {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "State observation requires observedAt")
	}
	if _, err := time.Parse(time.RFC3339Nano, observedAt); err != nil {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "State observation observedAt must be RFC3339")
	}
	return nil
}

// UnitObservation binds one hydrated value back to both its immutable
// declaration and the independent runtime basis used to observe it.
type UnitObservation struct {
	Address           Address          `json:"address"`
	DeclarationCommit kernel.CommitID  `json:"declarationCommit"`
	DeclarationDigest kernel.Digest    `json:"declarationDigest"`
	DescriptorDigest  kernel.Digest    `json:"descriptorDigest,omitempty"`
	Basis             ObservationBasis `json:"basis"`
}
