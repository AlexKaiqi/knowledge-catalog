package service

import (
	"context"
	"path/filepath"

	"kc/kernel"
	"kc/liveaccess/evidence"
	objectmodel "kc/object"
	"kc/observability"
)

// OpenResultStore applies the existing deployment evidence policy to the
// module's separately retained result payloads. Process logs keep pointers.
func OpenResultStore(home string) (*evidence.FileStore, error) {
	policy := observability.NewFileStore(home)
	retention := policy.HotRetention
	if retention == 0 {
		retention = observability.DefaultHotRetention
	}
	store, err := evidence.NewFileStore(filepath.Join(home, "liveaccess"), retention)
	if err != nil {
		return nil, err
	}
	store.MaxBytes = policy.MaxBytes
	if store.MaxBytes == 0 {
		store.MaxBytes = observability.DefaultMaxBytes
	}
	store.FloodStage = observability.DefaultFloodStage
	store.FloodUsedFraction = func() float64 {
		used, ok := policy.DiskUsedFraction()
		if !ok {
			return 1
		}
		return used
	}
	return store, nil
}

type AuditEntry struct {
	observability.AccessEvent
	Results       *evidence.Record     `json:"results,omitempty"`
	ResultFailure *kernel.IngressError `json:"resultFailure,omitempty"`
}

// AuditEntries is called only after the audit task's separate authorization.
// It restores original results, and reports expiry/loss without source calls.
func AuditEntries(ctx context.Context, store evidence.Store, entries []observability.AccessEvent) []AuditEntry {
	out := make([]AuditEntry, 0, len(entries))
	for _, entry := range entries {
		row := AuditEntry{AccessEvent: entry}
		if entry.LiveAccess != nil {
			record, err := store.Read(ctx, *entry.LiveAccess)
			if err == nil {
				err = validateAuditRecord(entry, record)
			}
			if err != nil {
				row.ResultFailure = kernel.Normalize(err)
			} else {
				row.Results = &record
			}
		}
		out = append(out, row)
	}
	return out
}

func validateAuditRecord(entry observability.AccessEvent, record evidence.Record) error {
	if err := record.Validate(); err != nil {
		return auditAssociationFailure()
	}
	if record.Identity != entry.Identity || record.Trace != entry.Trace || record.RequestID != entry.RequestID {
		return auditAssociationFailure()
	}
	for _, sample := range record.Units {
		if !auditSampleMatches(entry.Object, sample) {
			return auditAssociationFailure()
		}
	}
	for _, target := range entry.Object {
		if !auditObservationsMatch(target, record.Units) {
			return auditAssociationFailure()
		}
	}
	return nil
}

func auditSampleMatches(targets []observability.ObjectAccess, sample evidence.Sample) bool {
	for _, target := range targets {
		if !auditTargetMatches(target, sample) {
			continue
		}
		if sample.Outcome == evidence.FailureOutcome {
			return true
		}
		for _, observation := range target.Observations {
			if sample.Observation != nil && observation == *sample.Observation {
				return true
			}
		}
	}
	return false
}

func auditTargetMatches(target observability.ObjectAccess, sample evidence.Sample) bool {
	ref := target.ObjectRef
	return ref.Repository == sample.Repository && ref.Commit == sample.Commit && ref.Object == sample.Address.ObjectID &&
		auditAddressContains(target.Address, sample.Address) && auditLocatorContains(target.Locator, sample.Address)
}

func auditAddressContains(selected *objectmodel.Address, unit objectmodel.Address) bool {
	if selected == nil {
		return true
	}
	if selected.ObjectID != unit.ObjectID {
		return false
	}
	if *selected == unit || (selected.Kind == objectmodel.KindEntity || selected.Kind == objectmodel.KindRelation) && objectmodel.IsEntityBlob(*selected) {
		return true
	}
	return selected.Kind == objectmodel.KindAspect && unit.Kind == objectmodel.KindMember && selected.AspectName == unit.AspectName
}

func auditLocatorContains(locator string, address objectmodel.Address) bool {
	if locator == "" {
		return true
	}
	selected, err := objectmodel.PointerTokens(locator)
	if err != nil {
		return false
	}
	unit := unitPointerTokens(address)
	return tokenPrefix(unit, selected) || tokenPrefix(selected, unit)
}

func auditObservationsMatch(target observability.ObjectAccess, samples []evidence.Sample) bool {
	for _, observation := range target.Observations {
		if !auditObservationMatches(target, observation, samples) {
			return false
		}
	}
	return true
}

func auditObservationMatches(target observability.ObjectAccess, observation objectmodel.UnitObservation, samples []evidence.Sample) bool {
	for _, sample := range samples {
		if auditTargetMatches(target, sample) && sample.Outcome == evidence.ValueOutcome &&
			sample.Observation != nil && *sample.Observation == observation {
			return true
		}
	}
	return false
}

func auditAssociationFailure() error {
	return kernel.Fail(kernel.ErrTemporaryUnavailable, "retained results do not match the original access record")
}
