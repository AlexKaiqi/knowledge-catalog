package writer_test

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/writer"
	"kc/snapshot"
)

type noChangeFixture struct {
	repo     snapshot.Store
	registry snapshot.Registry
	ledger   snapshot.CommandLedger
	writer   *writer.Writer
	file     string
	fake     *testkit.LakeFSFake
}

func newNoChangeFixture(t *testing.T, intercept func(http.ResponseWriter, *http.Request) bool) noChangeFixture {
	t.Helper()
	fake := testkit.NewLakeFSFakeWithInterceptor(t, intercept)
	repo, err := snapshot.OpenLakeFS("kr://writer/no-change", fake.DSN(fake.NewRepo()), fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	registry := snapshot.NewRegistry()
	if err := registry.Add(repo); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "commands.db")
	ledger, err := snapshot.NewCommandLedger(snapshot.NewBoltCommandStore(file))
	if err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(registry, ledger)
	if err != nil {
		t.Fatal(err)
	}
	return noChangeFixture{repo: repo, registry: registry, ledger: ledger, writer: w, file: file, fake: fake}
}
func (f noChangeFixture) change(base kernel.CommitID, value any) objectmodel.ChangeSet {
	return objectmodel.ChangeSet{TargetRepository: f.repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/A"}, PathHint: "policies/A.yaml", Value: value}}}
}
func mustNoChangeCommit(t *testing.T, w *writer.Writer, id string, cs objectmodel.ChangeSet, changed bool) objectmodel.CommitReceipt {
	t.Helper()
	receipt, err := w.Commit(id, cs)
	if err != nil {
		t.Fatal(err)
	}
	expected := objectmodel.ChangeUnchanged
	if changed {
		expected = objectmodel.ChangeChanged
	}
	if receipt.Result.Changed != expected {
		t.Fatalf("incorrect change verdict: %#v", receipt)
	}
	return receipt
}

func TestNoChangeWriterPersistsDistinctReceiptsWithoutCommitOrNotification(t *testing.T) {
	var commits, resets, advanced atomic.Int32
	f := newNoChangeFixture(t, func(_ http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/commits") {
			commits.Add(1)
		}
		if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/hard_reset") {
			resets.Add(1)
		}
		return false
	})
	seed := mustNoChangeCommit(t, f.writer, "seed", f.change(testkit.MustHead(t, f.repo, snapshot.DefaultRef), map[string]any{"version": 1}), true)
	commits.Store(0)
	resets.Store(0)
	f.registry.OnAdvanced(func(snapshot.Advanced) { advanced.Add(1) })
	cs := f.change(seed.Result.NewCommit, map[string]any{"version": 1})
	first := mustNoChangeCommit(t, f.writer, "unchanged-one", cs, false)
	second := mustNoChangeCommit(t, f.writer, "unchanged-two", cs, false)
	if first.Result.OldCommit != seed.Result.NewCommit || first.Result.NewCommit != seed.Result.NewCommit || first.Result.CommitID != seed.Result.NewCommit || first.ReceiptRef == second.ReceiptRef {
		t.Fatalf("no-change receipt identity/coordinates: %#v %#v", first, second)
	}
	if commits.Load() != 0 || resets.Load() != 0 || advanced.Load() != 0 || testkit.MustHead(t, f.repo, snapshot.DefaultRef) != seed.Result.NewCommit {
		t.Fatalf("no-change advanced authority: commits=%d resets=%d notifications=%d", commits.Load(), resets.Load(), advanced.Load())
	}
	record, ok := f.ledger.Lookup("unchanged-one")
	if !ok || record.Status != snapshot.StatusApplied || record.Phase != snapshot.CommandPhaseNoChange || len(record.Request.ApplicationRequest) == 0 || record.Request.Authority == "" {
		t.Fatalf("no durable completion/proof: %#v", record)
	}
	store, err := snapshot.OpenBoltCommandStore(f.file)
	if err != nil {
		t.Fatal(err)
	}
	cold, err := snapshot.NewCommandLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := writer.NewWriter(f.registry, cold)
	if err != nil {
		t.Fatal(err)
	}
	replay := mustNoChangeCommit(t, reopened, "unchanged-one", cs, false)
	if replay.Disposition != writer.DispositionReplayed || replay.ReceiptRef != first.ReceiptRef || commits.Load() != 0 {
		t.Fatalf("cold replay reissued work: %#v", replay)
	}
	changed := cs
	changed.Message = "different original intent"
	if _, err := reopened.Commit("unchanged-one", changed); kernel.CodeOf(err) != kernel.ErrIdempotencyConflict {
		t.Fatalf("same command changed intent: %v", err)
	}
	// Missing changed in an older receipt remains unknown, never false.
	var legacy objectmodel.CommitReceipt
	if err := json.Unmarshal([]byte(`{"commandId":"legacy","result":{"newCommit":"old"}}`), &legacy); err != nil || legacy.Result.Changed != objectmodel.ChangeUnknown {
		t.Fatalf("legacy change verdict guessed: %#v %v", legacy, err)
	}
}

func TestNoChangeWriterStillChecksPreconditionsSchemaAndBackendWritePermission(t *testing.T) {
	var denied, archiveUnknown atomic.Bool
	f := newNoChangeFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
		if archiveUnknown.Load() && strings.HasSuffix(r.URL.Path, "/branches/kc-archived") {
			http.Error(w, "archive condition unavailable", http.StatusServiceUnavailable)
			return true
		}
		if denied.Load() && strings.Contains(r.URL.Path, "/staging/backing") {
			http.Error(w, "write denied", http.StatusForbidden)
			return true
		}
		return false
	})
	root := testkit.MustHead(t, f.repo, snapshot.DefaultRef)
	seed := mustNoChangeCommit(t, f.writer, "seed", f.change(root, 1), true)
	cs := f.change(seed.Result.NewCommit, 1)
	cs.Operations[0].Precondition = &objectmodel.Precondition{Type: objectmodel.IfAbsent}
	if _, err := f.writer.Commit("if-absent", cs); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("no-change bypassed unit precondition: %v", err)
	}
	cs = f.change(seed.Result.NewCommit, 1)
	cs.Operations[0].Precondition = &objectmodel.Precondition{Type: objectmodel.IfDigestEquals, Digest: kernel.CanonicalDigest(2)}
	if _, err := f.writer.Commit("wrong-unit-digest", cs); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("no-change bypassed digest precondition: %v", err)
	}
	cs = f.change(seed.Result.NewCommit, 1)
	cs.ExpectedTargetCommit = root
	if _, err := f.writer.Commit("wrong-base", cs); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("no-change bypassed base/expected equality: %v", err)
	}
	cs = f.change(seed.Result.NewCommit, 1)
	cs.Operations[0].SchemaRef = "schema/missing"
	if _, err := f.writer.Commit("missing-schema", cs); kernel.CodeOf(err) != kernel.ErrSchemaRevisionUnresolved {
		t.Fatalf("no-change bypassed Schema: %v", err)
	}
	archiveUnknown.Store(true)
	if _, err := f.writer.Commit("unknown-archive", f.change(seed.Result.NewCommit, 1)); err == nil || !kernel.IsWriteNotApplied(err) {
		t.Fatalf("unknown archive condition accepted no-change: %v", err)
	}
	archiveUnknown.Store(false)
	denied.Store(true)
	cs = f.change(seed.Result.NewCommit, 1)
	receipt, err := f.writer.Commit("read-only-backend", cs)
	if err == nil || receipt.Result.NewCommit != "" || !kernel.IsWriteNotApplied(err) {
		t.Fatalf("read-only credentials obtained no-change success: %#v %v", receipt, err)
	}
	if testkit.MustHead(t, f.repo, snapshot.DefaultRef) != seed.Result.NewCommit {
		t.Fatal("rejected no-change changed ref")
	}
	denied.Store(false)
	if err := f.repo.Archive(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.writer.Commit("archived", cs); kernel.CodeOf(err) != kernel.ErrRepositoryArchived {
		t.Fatalf("archived no-change accepted: %v", err)
	}
}

func TestNoChangeWriterIncludesPathProvenanceAndDeclarationEffects(t *testing.T) {
	f := newNoChangeFixture(t, nil)
	seed := mustNoChangeCommit(t, f.writer, "seed", f.change(testkit.MustHead(t, f.repo, snapshot.DefaultRef), map[string]any{"version": 1}), true)
	move := f.change(seed.Result.NewCommit, map[string]any{"version": 1})
	move.Operations[0].PathHint = "archive/A.yaml"
	moved := mustNoChangeCommit(t, f.writer, "move", move, true)
	provenance := f.change(moved.Result.NewCommit, map[string]any{"version": 1})
	provenance.Operations[0].PathHint = "archive/A.yaml"
	provenance.Provenance = &objectmodel.ProvenanceEnvelope{OriginKind: objectmodel.OriginSource, SourceRefs: []string{"source://new-provenance"}}
	updated := mustNoChangeCommit(t, f.writer, "provenance", provenance, true)
	schema := objectmodel.ChangeSet{TargetRepository: f.repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: updated.Result.NewCommit, ExpectedTargetCommit: updated.Result.NewCommit, Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy"}, Value: schemaDoc()}}}
	schemaReceipt := mustNoChangeCommit(t, f.writer, "schema", schema, true)
	adopted := f.change(schemaReceipt.Result.NewCommit, map[string]any{"version": 1})
	adopted.Operations[0].PathHint = "archive/A.yaml"
	adopted.Operations[0].SchemaRef = "schema/policy"
	declared := mustNoChangeCommit(t, f.writer, "adopt", adopted, true)
	inherited := f.change(declared.Result.NewCommit, map[string]any{"version": 1})
	inherited.Operations[0].PathHint = "archive/A.yaml"
	mustNoChangeCommit(t, f.writer, "inherit", inherited, false)
}

func TestNoChangeMergeStillReportsAdvancementWhenTreesAreEqual(t *testing.T) {
	f := newNoChangeFixture(t, nil)
	seed := mustNoChangeCommit(t, f.writer, "seed", f.change(testkit.MustHead(t, f.repo, snapshot.DefaultRef), 1), true)
	const branch = "refs/heads/review"
	if err := f.repo.CreateRef(branch, seed.Result.NewCommit); err != nil {
		t.Fatal(err)
	}
	changed := f.change(seed.Result.NewCommit, 2)
	changed.TargetRef = branch
	middle := mustNoChangeCommit(t, f.writer, "branch-change", changed, true)
	reverted := f.change(middle.Result.NewCommit, 1)
	reverted.TargetRef = branch
	candidate := mustNoChangeCommit(t, f.writer, "branch-revert", reverted, true)
	paths, err := f.repo.(snapshot.ChangeStore).ChangedPaths(seed.Result.NewCommit, candidate.Result.NewCommit)
	if err != nil || len(paths) != 0 {
		t.Fatalf("fixture trees differ: %v %v", paths, err)
	}
	var notifications atomic.Int32
	f.registry.OnAdvanced(func(snapshot.Advanced) { notifications.Add(1) })
	merged, err := f.writer.Merge("publish-equal-tree", writer.MergeIntent{TargetRepository: f.repo.ID(), SourceRef: branch, SourceCommit: candidate.Result.NewCommit, ExpectedTargetCommit: seed.Result.NewCommit})
	if err != nil {
		t.Fatal(err)
	}
	if merged.Result.Changed != objectmodel.ChangeChanged || merged.Result.NewCommit != candidate.Result.NewCommit || merged.Result.OldCommit != seed.Result.NewCommit || notifications.Load() != 1 {
		t.Fatalf("equal content suppressed reviewed publication: %#v notifications=%d", merged, notifications.Load())
	}
}

func TestNoChangeWriterBulkKeepsRequestedDataPlaneWithoutCommit(t *testing.T) {
	var commits, presigned, uploads atomic.Int32
	f := newNoChangeFixture(t, func(_ http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/commits") {
			commits.Add(1)
		}
		if strings.Contains(r.URL.Path, "/staging/backing") {
			presigned.Add(1)
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/objects") {
			uploads.Add(1)
		}
		return false
	})
	seed := mustNoChangeCommit(t, f.writer, "seed", f.change(testkit.MustHead(t, f.repo, snapshot.DefaultRef), 1), true)
	commits.Store(0)
	presigned.Store(0)
	uploads.Store(0)
	cs := f.change(seed.Result.NewCommit, 1)
	cs.BulkIngest = true
	mustNoChangeCommit(t, f.writer, "bulk-no-change", cs, false)
	if commits.Load() != 0 || presigned.Load() != 0 || uploads.Load() == 0 {
		t.Fatalf("bulk no-change committed or fell back to presigned staging: commits=%d presigned=%d uploads=%d", commits.Load(), presigned.Load(), uploads.Load())
	}
}
