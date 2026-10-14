package delivery

import (
	"kc/kernel"
	objectmodel "kc/object"
)

// Context is who the envelope is being delivered to. Authorization keys off
// Principal; onBehalfOf is not a delivery input.
type Context struct {
	Principal string
}

// Envelope is one already-hydrated object. ID is the locate result;
// the remaining fields are Canonical content the caller may or may not receive.
type Envelope struct {
	ID           objectmodel.PinnedObjectRef
	Address      objectmodel.Address
	Value        any
	Provenance   *objectmodel.ProvenanceEnvelope
	Observations []objectmodel.UnitObservation
	Units        []objectmodel.Address
	Declarations []objectmodel.UnitDeclaration
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
			return Envelope{}, kernel.Fail(kernel.ErrPreconditionFailed, "delivery stage changed object identity")
		}
		env = next
	}
	return env, nil
}

// FromValue copies a hydrated Canonical unit into a delivery envelope.
// Envelope identity uses the object's repository when present so a stale
// ObjectRef cannot widen the AUTH-01 read boundary.
func FromValue(value objectmodel.ObjectValue, observations []objectmodel.UnitObservation) Envelope {
	ref := value.ObjectRef
	if value.Repository != "" {
		ref.Repository = value.Repository
	}
	if ref.Object == "" {
		ref.Object = value.Address.ObjectID
	}
	return Envelope{
		ID: objectmodel.PinnedObjectRef{
			ObjectRef: ref,
			Commit:    value.Commit,
		},
		Address:      value.Address,
		Value:        value.Value,
		Provenance:   value.Provenance,
		Observations: append([]objectmodel.UnitObservation(nil), observations...),
		Units:        append([]objectmodel.Address(nil), value.Units...),
		Declarations: append([]objectmodel.UnitDeclaration(nil), value.Declarations...),
	}
}

// WriteBody copies visible content back onto a hydrated unit without touching
// identity coordinates.
func (env Envelope) WriteBody(value objectmodel.ObjectValue) (objectmodel.ObjectValue, []objectmodel.UnitObservation) {
	value.Value = env.Value
	value.Provenance = env.Provenance
	value.Units = append([]objectmodel.Address(nil), env.Units...)
	value.Declarations = append([]objectmodel.UnitDeclaration(nil), env.Declarations...)
	return value, append([]objectmodel.UnitObservation(nil), env.Observations...)
}
