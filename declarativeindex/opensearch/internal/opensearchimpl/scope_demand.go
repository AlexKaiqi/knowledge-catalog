package opensearchimpl

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"

	"kc/kernel"
)

const scopeActive = "ACTIVE"
const scopeRetired = "RETIRED"

// Scope demand is separate from ROOT readiness and publication/query claims.
// A retired scope can still have consumers of an already built generation.
// This record alone is not evidence that legacy writers have been excluded.
type scopeDemand struct {
	Version int    `json:"version"`
	Epoch   string `json:"epoch"`
	Phase   string `json:"phase"`
}

func (d scopeDemand) validate() error {
	if d.Version != 1 || !validGeneration(d.Epoch) || (d.Phase != scopeActive && d.Phase != scopeRetired) {
		return kernel.Fail(kernel.ErrPreconditionFailed, "projection scope demand has unknown version, epoch, or phase")
	}
	return nil
}

func (d *scopeDemand) UnmarshalJSON(raw []byte) error {
	type wire scopeDemand
	var value wire
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return kernel.Fail(kernel.ErrPreconditionFailed, "invalid projection scope demand: %v", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return kernel.Fail(kernel.ErrPreconditionFailed, "projection scope demand has trailing content")
	}
	if err := scopeDemand(value).validate(); err != nil {
		return err
	}
	*d = scopeDemand(value)
	return nil
}

func (e *openSearchEngine) loadScopeDemand(ctx context.Context) (scopeDemand, *controlVersion, error) {
	if e.namespace == "" {
		return scopeDemand{}, nil, kernel.Fail(kernel.ErrPreconditionFailed, "projection scope demand requires an owned namespace")
	}
	record, version, err := e.loadControlRecord(ctx, e.controlID+"-scope")
	if err != nil || version == nil {
		return scopeDemand{}, version, err
	}
	if record.State != "SCOPE" || record.Repository != string(e.repository) || record.ScopeDemand == nil || record.Resource != nil || record.ActiveIndex != "" || record.Generation != "" {
		return scopeDemand{}, nil, kernel.Fail(kernel.ErrPreconditionFailed, "projection scope demand has unknown ownership")
	}
	if err := record.ScopeDemand.validate(); err != nil {
		return scopeDemand{}, nil, err
	}
	return *record.ScopeDemand, version, nil
}

// activateScopeDemand is reserved for confirmed demand, never a queued hint or
// query handle. Reopening an inactive scope always changes the writer epoch.
func (e *openSearchEngine) activateScopeDemand(ctx context.Context) (scopeDemand, error) {
	if err := e.requireMutableProjection(); err != nil {
		return scopeDemand{}, err
	}
	for attempt := 0; attempt < 8; attempt++ {
		current, version, err := e.loadScopeDemand(ctx)
		if err != nil || current.Phase == scopeActive {
			return current, err
		}
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return scopeDemand{}, err
		}
		next := scopeDemand{Version: 1, Epoch: hex.EncodeToString(nonce[:]), Phase: scopeActive}
		record := controlDoc{Repository: string(e.repository), State: "SCOPE", ScopeDemand: &next}
		_, err = e.putControlRecordContext(ctx, e.controlID+"-scope", record, version)
		if kernel.CodeOf(err) != kernel.ErrNonFastForward {
			return next, err
		}
	}
	return scopeDemand{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "projection scope demand changed repeatedly")
}

// Retiring an observed epoch cannot retire a demand activated after that
// observation. Missing authority is unknown, not permission to stop writers.
func (e *openSearchEngine) retireScopeDemand(ctx context.Context, epoch string) error {
	if err := e.requireMutableProjection(); err != nil {
		return err
	}
	for attempt := 0; attempt < 8; attempt++ {
		current, version, err := e.loadScopeDemand(ctx)
		if err != nil {
			return err
		}
		if version == nil || !validGeneration(epoch) || current.Epoch != epoch {
			return kernel.Fail(kernel.ErrPreconditionFailed, "projection scope demand epoch is unavailable or changed")
		}
		if current.Phase == scopeRetired {
			return nil
		}
		current.Phase = scopeRetired
		record := controlDoc{Repository: string(e.repository), State: "SCOPE", ScopeDemand: &current}
		_, err = e.putControlRecordContext(ctx, e.controlID+"-scope", record, version)
		if kernel.CodeOf(err) != kernel.ErrNonFastForward {
			return err
		}
	}
	return kernel.Fail(kernel.ErrTemporaryUnavailable, "projection scope demand changed repeatedly")
}

func (e *openSearchEngine) checkScopeDemand(ctx context.Context, epoch string) error {
	current, version, err := e.loadScopeDemand(ctx)
	if err != nil {
		return err
	}
	if version == nil || !validGeneration(epoch) || current.Phase != scopeActive || current.Epoch != epoch {
		return kernel.Fail(kernel.ErrPreconditionFailed, "projection scope demand is inactive or belongs to another writer epoch")
	}
	return nil
}

// enterDemand admits new physical work only for a live scope. Proven retirement
// refuses, an active scope is reused, and an unmigrated legacy scope registers
// the consumer asking for it now. A missing record is not proof of a stop.
func (e *openSearchEngine) enterDemand(ctx context.Context) error {
	if e.namespace == "" {
		return nil
	}
	current, version, err := e.loadScopeDemand(ctx)
	if err != nil {
		return err
	}
	if version != nil {
		if current.Phase == scopeRetired {
			return kernel.Fail(kernel.ErrPreconditionFailed, "projection scope demand is retired")
		}
		return nil
	}
	_, err = e.activateScopeDemand(ctx)
	return err
}

// RetireDemand stops background regeneration after the last consumer leaves.
// It retires the epoch observed here, so a stop cannot close newer demand.
func (e *openSearchEngine) RetireDemand(ctx context.Context) error {
	if e.namespace == "" {
		return nil
	}
	current, version, err := e.loadScopeDemand(ctx)
	if err != nil {
		return err
	}
	if version == nil || current.Phase == scopeRetired {
		return nil
	}
	return e.retireScopeDemand(ctx, current.Epoch)
}
