package service_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kc/kernel"
	"kc/liveaccess/evidence"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	"kc/observability"
)

func auditFixture() (evidence.Record, observability.AccessEvent) {
	record := evidence.Record{
		Identity:  kernel.IdentityContext{Principal: "caller", OnBehalfOf: "consumer"},
		Trace:     kernel.TraceContext{TraceID: "trace", SpanID: "span", ParentSpanID: "parent"},
		RequestID: "request",
	}
	target := observability.ObjectAccess{ObjectRef: objectmodel.PinnedObjectRef{
		ObjectRef: objectmodel.ObjectRef{Repository: "kr://audit/source", Object: "fixture/sensor"}, Commit: "fixed"}}
	for _, name := range []string{"good", "null", "failed"} {
		address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "fixture/sensor", AspectName: name}
		sample := evidence.Sample{Repository: target.ObjectRef.Repository, Commit: target.ObjectRef.Commit,
			Address: address, Declaration: objectmodel.UnitDeclaration{Address: address, DeclarationDigest: kernel.Digest("declaration-" + name)}}
		if name == "failed" {
			sample.Outcome = evidence.FailureOutcome
			sample.Failure = &evidence.Failure{Code: kernel.ErrTemporaryUnavailable, Message: "Authorization denied by source"}
		} else {
			sample.Outcome = evidence.ValueOutcome
			if name == "good" {
				sample.Value = map[string]any{"reading": 31}
			}
			sample.Observation = &objectmodel.UnitObservation{Address: address, DeclarationCommit: sample.Commit,
				DeclarationDigest: sample.Declaration.DeclarationDigest, DescriptorDigest: "descriptor",
				Basis: objectmodel.ObservationBasis{BindingGeneration: "generation", Consistency: objectmodel.ObservationLatestOnly,
					ObservedAt: "2026-10-14T00:00:00Z", SourceRevision: "revision", Watermark: "watermark"}}
			target.Observations = append(target.Observations, *sample.Observation)
		}
		record.Units = append(record.Units, sample)
	}
	return record, observability.AccessEvent{Identity: record.Identity, Trace: record.Trace, RequestID: record.RequestID,
		Action: "object.read", Decision: "ALLOW", Result: "RESOLVED", Object: []observability.ObjectAccess{target}}
}

func retainAuditFixture(t *testing.T, home string, record evidence.Record, event observability.AccessEvent) (*evidence.FileStore, observability.AccessEvent) {
	t.Helper()
	store, err := liveservice.OpenResultStore(home)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := store.Record(context.Background(), record)
	if err != nil {
		t.Fatal(err)
	}
	event.LiveAccess = &reference
	process := observability.NewFileStore(home)
	receipt, err := process.RecordAccess(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}
	persisted, found, err := observability.NewFileStore(home).GetAccess(context.Background(), receipt.EvidenceID)
	if err != nil || !found {
		t.Fatalf("read durable process evidence: found=%v error=%v", found, err)
	}
	reopened, err := liveservice.OpenResultStore(home)
	if err != nil {
		t.Fatal(err)
	}
	return reopened, persisted
}

func TestAuditEntriesRestoreOriginalMixedResultsAfterRestart(t *testing.T) {
	record, event := auditFixture()
	event.Object[0].Address = &objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "fixture/sensor"}
	store, persisted := retainAuditFixture(t, t.TempDir(), record, event)
	rows := liveservice.AuditEntries(context.Background(), store, []observability.AccessEvent{persisted})
	if len(rows) != 1 || rows[0].ResultFailure != nil || rows[0].Results == nil ||
		kernel.CanonicalDigest(*rows[0].Results) != kernel.CanonicalDigest(record) {
		t.Fatalf("audit did not restore the original value, null and failure: %#v", rows)
	}
	if rows[0].Object[0].Locator != persisted.Object[0].Locator || rows[0].RequestID != record.RequestID {
		t.Fatalf("audit lost the process association: %#v", rows[0])
	}
}

func TestAuditEntriesMatchAdoptedUnitsInsideSelectedObjectPosition(t *testing.T) {
	for _, kind := range []string{"object", "aspect-collection", "member-leaf"} {
		t.Run(kind, func(t *testing.T) {
			record, event := auditMemberFixture()
			target := &event.Object[0]
			switch kind {
			case "object":
				target.Address = &objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "fixture/sensor"}
			case "aspect-collection":
				target.Address = &objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "fixture/sensor", AspectName: "readings"}
				target.Locator = "/readings"
			case "member-leaf":
				record.Units = record.Units[:1]
				target.Observations = target.Observations[:1]
				target.Address = &record.Units[0].Address
				target.Locator = "/readings/good/reading"
			}
			store, persisted := retainAuditFixture(t, t.TempDir(), record, event)
			rows := liveservice.AuditEntries(context.Background(), store, []observability.AccessEvent{persisted})
			if len(rows) != 1 || rows[0].ResultFailure != nil || rows[0].Results == nil ||
				kernel.CanonicalDigest(*rows[0].Results) != kernel.CanonicalDigest(record) || rows[0].Object[0].Locator != target.Locator {
				t.Fatalf("selected position rejected its original complete adopted units: %#v", rows)
			}
		})
	}
}

func auditMemberFixture() (evidence.Record, observability.AccessEvent) {
	record, event := auditFixture()
	for i := range record.Units {
		unit := &record.Units[i]
		unit.Address = objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "fixture/sensor", AspectName: "readings", MemberKey: unit.Address.AspectName}
		unit.Declaration.Address = unit.Address
		if unit.Observation != nil {
			unit.Observation.Address = unit.Address
			event.Object[0].Observations[i].Address = unit.Address
		}
	}
	return record, event
}

func TestAuditEntriesRejectUnrelatedSelectedPositions(t *testing.T) {
	for name, mutate := range map[string]func(*observability.ObjectAccess){
		"other-object": func(target *observability.ObjectAccess) {
			target.Address = &objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "fixture/other"}
		},
		"other-aspect": func(target *observability.ObjectAccess) {
			target.Address = &objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "fixture/sensor", AspectName: "other"}
		},
		"other-member": func(target *observability.ObjectAccess) {
			target.Address = &objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "fixture/sensor", AspectName: "readings", MemberKey: "other"}
		},
		"other-locator-aspect": func(target *observability.ObjectAccess) { target.Locator = "/other/good/reading" },
		"other-locator-member": func(target *observability.ObjectAccess) { target.Locator = "/readings/other/reading" },
		"invalid-locator":      func(target *observability.ObjectAccess) { target.Locator = "readings/good/reading" },
	} {
		t.Run(name, func(t *testing.T) {
			record, event := auditMemberFixture()
			record.Units = record.Units[:1]
			event.Object[0].Observations = event.Object[0].Observations[:1]
			mutate(&event.Object[0])
			store, persisted := retainAuditFixture(t, t.TempDir(), record, event)
			assertAuditUnavailable(t, liveservice.AuditEntries(context.Background(), store, []observability.AccessEvent{persisted}))
		})
	}
}

func TestAuditEntriesRejectMismatchedOriginalAccess(t *testing.T) {
	cases := map[string]func(*evidence.Record, *observability.AccessEvent){
		"request":     func(r *evidence.Record, _ *observability.AccessEvent) { r.RequestID = "other-request" },
		"principal":   func(r *evidence.Record, _ *observability.AccessEvent) { r.Identity.Principal = "other-caller" },
		"delegation":  func(r *evidence.Record, _ *observability.AccessEvent) { r.Identity.OnBehalfOf = "other-consumer" },
		"trace":       func(r *evidence.Record, _ *observability.AccessEvent) { r.Trace.TraceID = "other-trace" },
		"span":        func(r *evidence.Record, _ *observability.AccessEvent) { r.Trace.SpanID = "other-span" },
		"parent-span": func(r *evidence.Record, _ *observability.AccessEvent) { r.Trace.ParentSpanID = "other-parent" },
		"repository": func(_ *evidence.Record, e *observability.AccessEvent) {
			e.Object[0].ObjectRef.Repository = "kr://audit/other"
		},
		"fixed-commit": func(_ *evidence.Record, e *observability.AccessEvent) {
			e.Object[0].ObjectRef.Commit = "other-fixed"
			for i := range e.Object[0].Observations {
				e.Object[0].Observations[i].DeclarationCommit = "other-fixed"
			}
		},
		"object": func(_ *evidence.Record, e *observability.AccessEvent) {
			e.Object[0].ObjectRef.Object = "other-object"
			for i := range e.Object[0].Observations {
				e.Object[0].Observations[i].Address.ObjectID = "other-object"
			}
		},
		"explicit-position": func(_ *evidence.Record, e *observability.AccessEvent) {
			e.Object[0].Address = &objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "fixture/sensor", AspectName: "unread"}
		},
		"original-declaration": func(_ *evidence.Record, e *observability.AccessEvent) {
			e.Object[0].Observations[0].DeclarationDigest = "other-declaration"
		},
		"descriptor": func(_ *evidence.Record, e *observability.AccessEvent) {
			e.Object[0].Observations[0].DescriptorDigest = "other-descriptor"
		},
		"generation": func(_ *evidence.Record, e *observability.AccessEvent) {
			e.Object[0].Observations[0].Basis.BindingGeneration = "other-generation"
		},
		"source-revision": func(_ *evidence.Record, e *observability.AccessEvent) {
			e.Object[0].Observations[0].Basis.SourceRevision = "other-revision"
		},
		"watermark": func(_ *evidence.Record, e *observability.AccessEvent) {
			e.Object[0].Observations[0].Basis.Watermark = "other-watermark"
		},
		"observation-time": func(_ *evidence.Record, e *observability.AccessEvent) {
			e.Object[0].Observations[0].Basis.ObservedAt = "2026-10-14T00:00:01Z"
		},
		"consistency": func(_ *evidence.Record, e *observability.AccessEvent) {
			e.Object[0].Observations[0].Basis.Consistency = objectmodel.ObservationRepeatable
		},
		"missing-success-sample": func(r *evidence.Record, _ *observability.AccessEvent) { r.Units = r.Units[1:] },
		"missing-process-basis": func(_ *evidence.Record, e *observability.AccessEvent) {
			e.Object[0].Observations = e.Object[0].Observations[1:]
		},
		"success-as-failure": func(r *evidence.Record, _ *observability.AccessEvent) {
			r.Units[0].Outcome, r.Units[0].Value, r.Units[0].Observation = evidence.FailureOutcome, nil, nil
			r.Units[0].Failure = &evidence.Failure{Code: kernel.ErrTemporaryUnavailable, Message: "failed"}
		},
		"failure-as-null": func(r *evidence.Record, _ *observability.AccessEvent) {
			r.Units[2].Outcome, r.Units[2].Failure = evidence.ValueOutcome, nil
			observation := *r.Units[0].Observation
			observation.Address, observation.DeclarationDigest = r.Units[2].Address, r.Units[2].Declaration.DeclarationDigest
			r.Units[2].Observation = &observation
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			record, event := auditFixture()
			mutate(&record, &event)
			store, persisted := retainAuditFixture(t, t.TempDir(), record, event)
			assertAuditUnavailable(t, liveservice.AuditEntries(context.Background(), store, []observability.AccessEvent{persisted}))
		})
	}
}

func TestAuditEntriesRejectSwappedDurablePointers(t *testing.T) {
	home := t.TempDir()
	record, event := auditFixture()
	store, first := retainAuditFixture(t, home, record, event)
	record.RequestID, event.RequestID = "second-request", "second-request"
	record.Units[0].Value = "second-original-value"
	_, second := retainAuditFixture(t, home, record, event)
	first.LiveAccess = second.LiveAccess
	assertAuditUnavailable(t, liveservice.AuditEntries(context.Background(), store, []observability.AccessEvent{first}))
}

func TestAuditEntriesReportMissingFailurePayload(t *testing.T) {
	home := t.TempDir()
	record, event := auditFixture()
	store, persisted := retainAuditFixture(t, home, record, event)
	path := filepath.Join(home, "liveaccess", persisted.LiveAccess.ID+".json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	units := payload["record"].(map[string]any)["units"].([]any)
	delete(units[2].(map[string]any), "failure")
	body, err = json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	assertAuditUnavailable(t, liveservice.AuditEntries(context.Background(), store, []observability.AccessEvent{persisted}))
}

func assertAuditUnavailable(t *testing.T, rows []liveservice.AuditEntry) {
	t.Helper()
	if len(rows) != 1 || rows[0].Results != nil || rows[0].ResultFailure == nil || rows[0].ResultFailure.Code != kernel.ErrTemporaryUnavailable {
		t.Fatalf("unverifiable original results were reported as success: %#v", rows)
	}
}

func TestOpenResultStoreCannotIgnoreUnknownDiskOccupancy(t *testing.T) {
	store, err := liveservice.OpenResultStore(filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Fatal(err)
	}
	if used := store.FloodUsedFraction(); used < store.FloodStage {
		t.Fatalf("unknown disk occupancy permits retention: used=%v threshold=%v", used, store.FloodStage)
	}
}

func TestAuditEntriesPreserveExpiryAndStaticProcessEvidence(t *testing.T) {
	record, event := auditFixture()
	store, persisted := retainAuditFixture(t, t.TempDir(), record, event)
	deadline, err := time.Parse(time.RFC3339Nano, persisted.LiveAccess.RetainUntil)
	if err != nil {
		t.Fatal(err)
	}
	store.Now = func() time.Time { return deadline }
	static := persisted
	static.LiveAccess = nil
	rows := liveservice.AuditEntries(context.Background(), store, []observability.AccessEvent{persisted, static})
	if len(rows) != 2 || rows[0].Results != nil || rows[0].ResultFailure == nil || rows[0].ResultFailure.Code != kernel.ErrCapabilityUnsatisfied ||
		rows[1].Results != nil || rows[1].ResultFailure != nil || rows[1].RequestID != static.RequestID {
		t.Fatalf("expiry and static process evidence changed meaning: %#v", rows)
	}
}
