package plugin_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	kcclient "kc/client"
	inv "kc/invocation"
	"kc/kernel"
	"kc/liveaccess/evidence"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	"kc/observability"
	"kc/server"
)

func (f *acceptanceFixture) audit(t *testing.T, principal, requestID string) ([]liveservice.AuditEntry, error) {
	t.Helper()
	var page struct {
		Entries []liveservice.AuditEntry `json:"entries"`
	}
	err := f.client(t, principal).OperationsService().AccessLog(context.Background(), kcclient.AuditQueryRequest{RequestID: requestID, Limit: 50}, kcclient.RequestOptions{}, &page)
	return page.Entries, err
}

// U17: the ordinary read returns its values only; authorized audit restores
// the adopted value/null/failure after source and Snapshot changes and restart.
func TestServerLiveAccessAuditRestoresOriginalReadAndRechecksSeparatePermission(t *testing.T) {
	one, two := newAcceptanceRuntime(t, 31), newAcceptanceRuntime(t, 22)
	f := newAcceptanceFixture(t, server.HTTPServerOptions{LiveAccessPlugins: acceptanceEntries(t, map[string]*acceptanceRuntime{"one": one, "two": two})})
	f.publish(t, acceptanceR1, "http://logical-source.invalid")
	oldCommit := f.commits[acceptanceR1]
	request := kcclient.ObjectReadRequest{Reference: acceptanceReference(t, acceptanceR1, ""), Repository: string(acceptanceR1), Commit: string(oldCommit)}
	row, err := f.read(t, "reader", "audit-original", request)
	if err != nil {
		t.Fatal(err)
	}
	assertAcceptanceMixed(t, *row, 31, kernel.ErrTemporaryUnavailable)
	var delivered map[string]json.RawMessage
	if err := f.client(t, "reader").Objects().Read(context.Background(), request, kcclient.RequestOptions{RequestID: "audit-private-delivery"}, &delivered); err != nil {
		t.Fatal(err)
	}
	if delivered["results"] != nil || delivered["liveaccess"] != nil {
		t.Fatal("ordinary read exposed the independent audit record")
	}
	one.value.Store(37)
	f.commit(t, acceptanceR1, []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: acceptanceObject, AspectName: "definition"}, Value: map[string]any{"owner": "advanced"}}})
	newRow, err := f.read(t, "reader", "audit-new", kcclient.ObjectReadRequest{Reference: acceptanceReference(t, acceptanceR1, ""), Repository: string(acceptanceR1), Commit: string(f.commits[acceptanceR1])})
	if err != nil || fmt.Sprint(newRow.Value.(map[string]any)["good"].(map[string]any)["value"]) != "37" || newRow.Value.(map[string]any)["definition"].(map[string]any)["owner"] != "advanced" || newRow.Commit == oldCommit {
		t.Fatalf("source/Snapshot update did not advance actual read: %#v %v", newRow, err)
	}
	f.stop(t)
	one.server.Close()
	f.start(t)
	sourceCalls, sourceAttempts := one.calls.Load(), one.attempts.Load()
	var fixed objectmodel.ObservedValue
	err = f.client(t, "reader").Objects().Read(context.Background(), kcclient.ObjectReadRequest{Reference: acceptanceReference(t, acceptanceR1, "/definition"), Repository: string(acceptanceR1), Commit: string(oldCommit)}, kcclient.RequestOptions{RequestID: "audit-fixed-snapshot"}, &fixed)
	if err != nil || fixed.Commit != oldCommit || fixed.Value.(map[string]any)["owner"] != "fixed" || one.attempts.Load() != sourceAttempts {
		t.Fatalf("original Snapshot position changed or called offline source: %#v %v", fixed, err)
	}
	entries, err := f.audit(t, "auditor", "audit-original")
	if err != nil || len(entries) != 1 || entries[0].Results == nil || entries[0].ResultFailure != nil {
		raw, _ := json.Marshal(entries)
		t.Fatalf("independent audit did not restore retained record: %s %v", raw, err)
	}
	entry := entries[0]
	if entry.RequestID != "audit-original" || entry.Identity.Principal != "reader" || len(entry.Object) != 1 || entry.Object[0].ObjectRef.Commit != oldCommit || entry.Results.RequestID != entry.RequestID || len(entry.Results.Units) != 4 {
		t.Fatalf("audit changed original request/declaration scope: %#v", entry)
	}
	seen := map[string]bool{}
	for _, sample := range entry.Results.Units {
		name := sample.Address.AspectName
		if sample.Address.MemberKey != "" {
			name = "member"
		}
		seen[name] = true
		if sample.Repository != acceptanceR1 || sample.Commit != oldCommit || sample.Declaration.Address != sample.Address || sample.Declaration.DeclarationDigest == "" {
			t.Fatalf("audit lost fixed unit declaration: %#v", sample)
		}
		switch name {
		case "good", "member":
			if sample.Outcome != evidence.ValueOutcome || fmt.Sprint(sample.Value.(map[string]any)["value"]) != "31" || sample.Observation.Basis.SourceRevision != "source-31" {
				t.Fatalf("audit substituted current source value: %#v", sample)
			}
		case "empty":
			if sample.Outcome != evidence.ValueOutcome || sample.Value != nil || sample.Observation == nil {
				t.Fatalf("audit lost successful null: %#v", sample)
			}
		case "broken":
			if sample.Outcome != evidence.FailureOutcome || sample.Failure.Code != kernel.ErrTemporaryUnavailable || !strings.Contains(sample.Failure.Message, "selected source unavailable") || sample.Observation != nil {
				t.Fatalf("audit lost original failure: %#v", sample)
			}
		default:
			t.Fatalf("audit invented a unit: %#v", sample)
		}
	}
	if len(seen) != 4 || one.calls.Load() != sourceCalls || one.attempts.Load() != sourceAttempts {
		t.Fatal("audit called offline source or lost a retained unit")
	}
	if _, err := f.audit(t, "reader", "audit-original"); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("object read implied independent audit permission: %v", err)
	}
	if _, err := f.read(t, "auditor", "audit-cannot-read", request); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("audit permission implied object read: %v", err)
	}
	allow, err := inv.ReadAllow(f.config.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	kept := allow.Rules[:0]
	for _, rule := range allow.Rules {
		if rule.ID != "auditor" {
			kept = append(kept, rule)
		}
	}
	allow.Rules = kept
	if err := inv.WriteAllow(f.config.StateDir, allow); err != nil {
		t.Fatal(err)
	}
	if _, err := f.audit(t, "auditor", "audit-original"); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("revoked audit permission survived: %v", err)
	}
	logs, err := filepath.Glob(filepath.Join(f.config.StateDir, "access", "*.jsonl"))
	if err != nil || len(logs) == 0 {
		t.Fatal("public read did not save process evidence", err)
	}
	for _, path := range logs {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), `"value":`) || strings.Contains(string(raw), `"units":`) || strings.Contains(string(raw), `"results":`) {
			t.Fatal("process access ledger duplicated retained payload")
		}
	}
}

// U18: actual filesystem failures independently block payload persistence and
// pointer persistence; neither can acknowledge a successful public read.
func TestServerLiveAccessPersistenceFailuresPreventSuccessfulDelivery(t *testing.T) {
	for _, target := range []string{"liveaccess", "access"} {
		t.Run(target, func(t *testing.T) {
			one, two := newAcceptanceRuntime(t, 31), newAcceptanceRuntime(t, 22)
			f := newAcceptanceFixture(t, server.HTTPServerOptions{LiveAccessPlugins: acceptanceEntries(t, map[string]*acceptanceRuntime{"one": one, "two": two})})
			f.publish(t, acceptanceR1, "http://logical-source.invalid")
			blocked := filepath.Join(f.config.StateDir, target)
			if err := os.WriteFile(blocked, []byte("actual filesystem blocker"), 0600); err != nil {
				t.Fatal(err)
			}
			row, err := f.read(t, "reader", "unpersisted-"+target, kcclient.ObjectReadRequest{Reference: acceptanceReference(t, acceptanceR1, ""), Repository: string(acceptanceR1), Commit: string(f.commits[acceptanceR1])})
			if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || row != nil || one.calls.Load() == 0 {
				t.Fatalf("failed %s persistence delivered source result: %#v %v calls=%d", target, row, err, one.calls.Load())
			}
			if target == "access" {
				retained, err := filepath.Glob(filepath.Join(f.config.StateDir, "liveaccess", "*.json"))
				if err != nil || len(retained) != 1 {
					t.Fatalf("pointer failure was not independently after payload persistence: %v %v", retained, err)
				}
			}
		})
	}
}

func TestServerLiveAccessAuditDistinguishesCorruptionLossAndExpiry(t *testing.T) {
	one, two := newAcceptanceRuntime(t, 31), newAcceptanceRuntime(t, 22)
	f := newAcceptanceFixture(t, server.HTTPServerOptions{LiveAccessPlugins: acceptanceEntries(t, map[string]*acceptanceRuntime{"one": one, "two": two})})
	f.publish(t, acceptanceR1, "http://logical-source.invalid")
	if _, err := f.read(t, "reader", "lifecycle-original", kcclient.ObjectReadRequest{Reference: acceptanceReference(t, acceptanceR1, ""), Repository: string(acceptanceR1), Commit: string(f.commits[acceptanceR1])}); err != nil {
		t.Fatal(err)
	}
	entries, err := f.audit(t, "auditor", "lifecycle-original")
	if err != nil || len(entries) != 1 || entries[0].Results == nil {
		raw, _ := json.Marshal(entries)
		t.Fatalf("original retained result missing: %s %v", raw, err)
	}
	original := entries[0]
	path := filepath.Join(f.config.StateDir, "liveaccess", original.LiveAccess.ID+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	one.value.Store(59)
	beforeCalls, beforeAttempts := one.calls.Load(), one.attempts.Load()
	for _, component := range []string{"value", "failure", "basis", "corrupt"} {
		t.Run(component, func(t *testing.T) {
			damaged := append([]byte(nil), raw...)
			if component == "corrupt" {
				damaged = append(damaged, 'x')
			} else {
				var stored map[string]any
				if err := json.Unmarshal(damaged, &stored); err != nil {
					t.Fatal(err)
				}
				removed := false
				for _, rawUnit := range stored["record"].(map[string]any)["units"].([]any) {
					unit := rawUnit.(map[string]any)
					if component == "failure" && unit["outcome"] == "failure" {
						delete(unit, "failure")
						removed = true
					} else if component != "failure" && unit["outcome"] == "value" && unit["value"] != nil {
						if component == "value" {
							delete(unit, "value")
						} else {
							delete(unit["observation"].(map[string]any), "basis")
						}
						removed = true
					}
					if removed {
						break
					}
				}
				if !removed {
					t.Fatal("fixture had no original component to lose")
				}
				damaged, err = json.Marshal(stored)
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(path, damaged, 0600); err != nil {
				t.Fatal(err)
			}
			entries, err := f.audit(t, "auditor", "lifecycle-original")
			if err != nil || len(entries) != 1 || entries[0].Results != nil || entries[0].ResultFailure == nil || entries[0].ResultFailure.Code != kernel.ErrTemporaryUnavailable || !strings.Contains(entries[0].ResultFailure.Message, "original digest") {
				t.Fatalf("lost %s was not reported on exact audit record: %#v %v", component, entries, err)
			}
		})
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	entries, err = f.audit(t, "auditor", "lifecycle-original")
	if err != nil || len(entries) != 1 || entries[0].ResultFailure == nil || entries[0].ResultFailure.Code != kernel.ErrTemporaryUnavailable || !strings.Contains(entries[0].ResultFailure.Message, "within its retention") {
		t.Fatalf("lost retained value became expiry/success: %#v %v", entries, err)
	}
	if one.calls.Load() != beforeCalls || one.attempts.Load() != beforeAttempts {
		t.Fatal("lost original evidence was replaced by current source data")
	}
	// A legal historical record had a 24h payload promise. The current 30d
	// process ledger retains it; a longer current policy cannot change the old
	// recorder-generated deadline. No ID, digest or deadline is forged.
	old := time.Now().UTC().Add(-25 * time.Hour)
	store, err := evidence.NewFileStore(filepath.Join(f.config.StateDir, "liveaccess"), 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	store.MaxBytes = observability.DefaultMaxBytes
	store.Now = func() time.Time { return old }
	record := *original.Results
	record.RequestID = "legitimate-historical-read"
	reference, err := store.Record(context.Background(), record)
	if err != nil {
		t.Fatal(err)
	}
	process := observability.NewFileStore(f.config.StateDir)
	process.Now = func() time.Time { return old }
	event := original.AccessEvent
	event.EvidenceID, event.OccurredAt, event.RequestID, event.LiveAccess = "", old.Format(time.RFC3339Nano), record.RequestID, &reference
	receipt, err := process.RecordAccess(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}
	var page struct {
		Entries []liveservice.AuditEntry `json:"entries"`
	}
	err = f.client(t, "auditor").OperationsService().AccessLog(context.Background(), kcclient.AuditQueryRequest{EvidenceID: receipt.EvidenceID, RequestID: record.RequestID, Since: old.Format(time.RFC3339Nano)}, kcclient.RequestOptions{}, &page)
	if err != nil || len(page.Entries) != 1 || page.Entries[0].ResultFailure == nil || page.Entries[0].ResultFailure.Code != kernel.ErrCapabilityUnsatisfied || !strings.Contains(page.Entries[0].ResultFailure.Message, "retention deadline") || page.Entries[0].Results != nil {
		t.Fatalf("normal exact deadline became loss/success: %#v %v", page, err)
	}
}
