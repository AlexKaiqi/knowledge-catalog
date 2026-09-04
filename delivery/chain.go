package delivery

import (
	"kc/kernel"
	"kc/knowledge"
)

// Context is who the envelope is being delivered to. Authorization keys off
// Principal; onBehalfOf is not a delivery input.
type Context struct {
	Principal string
}

// Envelope is one already-hydrated knowledge object. ID is the locate result;
// the remaining fields are Canonical content the caller may or may not receive.
type Envelope struct {
	ID           knowledge.PinnedKnowledgeRef
	Address      knowledge.Address
	Value        any
	Provenance   *knowledge.ProvenanceEnvelope
	Observations []knowledge.UnitObservation
	Units        []knowledge.Address
	Declarations []knowledge.UnitDeclaration
}

// Stage rewrites a hydrated envelope for the caller. Stages must not change
// ID or Address. A nil Stage is skipped.
type Stage interface {
	Apply(ctx Context, env Envelope) (Envelope, error)
}

// StageFunc is the extension point for later selected rules (privacy, …).
type StageFunc func(Context, Envelope) (Envelope, error)

func (f StageFunc) Apply(ctx Context, env Envelope) (Envelope, error) {
	if f == nil {
		return env, nil
	}
	return f(ctx, env)
}

// Chain applies stages in order after SEARCH hydrate and before transport.
// An empty chain returns the hydrated envelope unchanged.
type Chain []Stage

func (c Chain) Apply(ctx Context, env Envelope) (Envelope, error) {
	id := env.ID
	address := env.Address
	for _, stage := range c {
		if stage == nil {
			continue
		}
		next, err := stage.Apply(ctx, env)
		if err != nil {
			return Envelope{}, err
		}
		if next.ID != id || next.Address != address {
			return Envelope{}, kernel.Fail(kernel.ErrPreconditionFailed, "delivery stage changed knowledge identity")
		}
		env = next
	}
	return env, nil
}

// FromValue copies a hydrated Canonical unit into a delivery envelope.
// Envelope identity uses the object's repository when present so a stale
// KnowledgeRef cannot widen the AUTH-01 read boundary.
func FromValue(value knowledge.KnowledgeValue, observations []knowledge.UnitObservation) Envelope {
	ref := value.KnowledgeRef
	if value.Repository != "" {
		ref.Repository = value.Repository
	}
	if ref.Object == "" {
		ref.Object = value.Address.ObjectID
	}
	return Envelope{
		ID: knowledge.PinnedKnowledgeRef{
			KnowledgeRef: ref,
			Commit:       value.Commit,
		},
		Address:      value.Address,
		Value:        value.Value,
		Provenance:   value.Provenance,
		Observations: append([]knowledge.UnitObservation(nil), observations...),
		Units:        append([]knowledge.Address(nil), value.Units...),
		Declarations: append([]knowledge.UnitDeclaration(nil), value.Declarations...),
	}
}

// WriteBody copies visible content back onto a hydrated unit without touching
// identity coordinates.
func (env Envelope) WriteBody(value knowledge.KnowledgeValue) (knowledge.KnowledgeValue, []knowledge.UnitObservation) {
	value.Value = env.Value
	value.Provenance = env.Provenance
	value.Units = append([]knowledge.Address(nil), env.Units...)
	value.Declarations = append([]knowledge.UnitDeclaration(nil), env.Declarations...)
	return value, append([]knowledge.UnitObservation(nil), env.Observations...)
}
