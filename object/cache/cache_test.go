package cache_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/cache"
)

type repository struct {
	objectmodel.Repository
	id        kernel.RepositoryID
	mu        sync.Mutex
	values    map[kernel.CommitID]map[objectmodel.ObjectID]objectmodel.KnowledgeValue
	batches   [][]objectmodel.ObjectID
	addresses []objectmodel.Address
	err       error
	pageCalls int
	pageLimit int
}

func (r *repository) ID() kernel.RepositoryID { return r.id }
func (r *repository) ReadMany(ids []objectmodel.ObjectID, commit kernel.CommitID) (map[objectmodel.ObjectID]objectmodel.KnowledgeValue, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.batches = append(r.batches, append([]objectmodel.ObjectID(nil), ids...))
	if r.err != nil {
		return nil, r.err
	}
	out := map[objectmodel.ObjectID]objectmodel.KnowledgeValue{}
	for _, id := range ids {
		if v, ok := r.values[commit][id]; ok {
			out[id] = v
		}
	}
	return out, nil
}
func (r *repository) ReadAddress(a objectmodel.Address, commit kernel.CommitID) (objectmodel.KnowledgeValue, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.addresses = append(r.addresses, a)
	if r.err != nil {
		return objectmodel.KnowledgeValue{}, r.err
	}
	v, ok := r.values[commit][a.ObjectID]
	if !ok {
		return objectmodel.KnowledgeValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "missing")
	}
	v.Address = a
	v.Value = string(a.Kind) + "/" + a.AspectName + "/" + a.MemberKey
	return v, nil
}
func (r *repository) ObjectIDsPage(commit kernel.CommitID, limit int, continuation string) (objectmodel.ObjectIDPage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pageCalls++
	r.pageLimit = limit
	ids := []objectmodel.ObjectID{}
	for id := range r.values[commit] {
		if len(ids) == limit {
			break
		}
		ids = append(ids, id)
	}
	return objectmodel.ObjectIDPage{ObjectIDs: ids, Continuation: "more", Exhausted: false}, nil
}
func value(repo kernel.RepositoryID, commit kernel.CommitID, id objectmodel.ObjectID, content any) objectmodel.KnowledgeValue {
	return objectmodel.KnowledgeValue{Repository: repo, Commit: commit, KnowledgeRef: objectmodel.KnowledgeRef{Repository: repo, Object: id}, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: id}, Value: content}
}
func repo(id kernel.RepositoryID) *repository {
	r := &repository{id: id, values: map[kernel.CommitID]map[objectmodel.ObjectID]objectmodel.KnowledgeValue{}}
	for _, commit := range []kernel.CommitID{"c1", "c2"} {
		r.values[commit] = map[objectmodel.ObjectID]objectmodel.KnowledgeValue{}
		for _, id := range []objectmodel.ObjectID{"a", "b", "c"} {
			r.values[commit][id] = value(r.id, commit, id, string(commit)+string(id))
		}
	}
	return r
}
func newCache(t *testing.T, cfg cache.Config) *cache.Cache {
	t.Helper()
	c, err := cache.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func read(t *testing.T, c *cache.Cache, r objectmodel.Repository, commit kernel.CommitID, ids ...objectmodel.ObjectID) map[objectmodel.ObjectID]objectmodel.KnowledgeValue {
	t.Helper()
	out, err := c.ReadMany(r, commit, ids)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCacheUsesRepositoryVersionAndBatchedMisses(t *testing.T) {
	c := newCache(t, cache.Config{})
	r := repo("kr://one")
	other := repo("kr://two")
	read(t, c, r, "c1", "a")
	got := read(t, c, r, "c1", "a", "b", "b", "missing")
	if len(got) != 2 || !reflect.DeepEqual(r.batches, [][]objectmodel.ObjectID{{"a"}, {"b", "missing"}}) {
		t.Fatalf("batch result=%v calls=%v", got, r.batches)
	}
	if got := read(t, c, r, "c2", "a")["a"].Value; got != "c2a" {
		t.Fatal(got)
	}
	if got := read(t, c, other, "c1", "a")["a"].Repository; got != other.id {
		t.Fatal(got)
	}
	read(t, c, r, "c1", "missing")
	if len(r.batches) != 4 {
		t.Fatalf("missing value was cached: %v", r.batches)
	}
	if _, err := c.ReadMany(r, "", []objectmodel.ObjectID{"a"}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("empty commit: %v", err)
	}
}

func TestCacheClonesAllMutableSnapshotData(t *testing.T) {
	c := newCache(t, cache.Config{})
	r := repo("kr://one")
	v := r.values["c1"]["a"]
	v.Value = map[string]any{"items": []any{map[string]any{"count": int64(7), "bytes": []byte{1, 2}}}}
	v.Units = []objectmodel.Address{{Kind: objectmodel.KindAspect, ObjectID: "a", AspectName: "x"}}
	v.Provenance = &objectmodel.ProvenanceEnvelope{SourceRefs: []string{"source"}, EvidenceRefs: []string{"evidence"}, Algorithm: &objectmodel.AlgorithmRef{CodeHash: "original"}}
	v.Declarations = []objectmodel.UnitDeclaration{{Address: v.Units[0], ValueSource: &objectmodel.ValueSource{Kind: objectmodel.ValueSourceBinding, Binding: &objectmodel.BindingDeclaration{Mode: objectmodel.BindingState, Operations: map[string]objectmodel.BindingOperation{"get": {Call: "original"}}}}}}
	r.values["c1"]["a"] = v
	first := read(t, c, r, "c1", "a")["a"]
	first.Value.(map[string]any)["items"].([]any)[0].(map[string]any)["bytes"].([]byte)[0] = 9
	first.Provenance.SourceRefs[0] = "changed"
	first.Provenance.Algorithm.CodeHash = "changed"
	first.Units[0].AspectName = "changed"
	first.Declarations[0].ValueSource.Binding.Operations["get"] = objectmodel.BindingOperation{Call: "changed"}
	second := read(t, c, r, "c1", "a")["a"]
	if !reflect.DeepEqual(second, v) {
		t.Fatalf("caller mutation leaked into cache: %#v", second)
	}
	v.Provenance.EvidenceRefs[0] = "source-mutated"
	third := read(t, c, r, "c1", "a")["a"]
	if third.Provenance.EvidenceRefs[0] != "evidence" {
		t.Fatal("source mutation leaked into cache")
	}
}

func TestCacheAddressKeysRetainEveryCoordinate(t *testing.T) {
	c := newCache(t, cache.Config{})
	r := repo("kr://one")
	addresses := []objectmodel.Address{
		{Kind: objectmodel.KindEntity, ObjectID: "a"},
		{Kind: objectmodel.KindRelation, ObjectID: "a"},
		{Kind: objectmodel.KindAspect, ObjectID: "a", AspectName: "x"},
		{Kind: objectmodel.KindMember, ObjectID: "a", AspectName: "x", MemberKey: "m1"},
		{Kind: objectmodel.KindMember, ObjectID: "a", AspectName: "x", MemberKey: "m2"},
	}
	read(t, c, r, "c1", "a")
	for _, a := range addresses {
		for i := 0; i < 2; i++ {
			got, err := c.ReadAddress(r, "c1", a)
			if err != nil || got.Address != a {
				t.Fatalf("%v: %v %v", a, got, err)
			}
		}
	}
	if len(r.addresses) != len(addresses) {
		t.Fatal(r.addresses)
	}
}

func TestCacheFailsOnSourceErrorsAndWrongBasis(t *testing.T) {
	for _, mutation := range []string{"repository", "ref-repository", "commit", "object", "address"} {
		t.Run(mutation, func(t *testing.T) {
			c := newCache(t, cache.Config{})
			r := repo("kr://one")
			v := r.values["c1"]["a"]
			switch mutation {
			case "repository":
				v.Repository = "other"
			case "ref-repository":
				v.KnowledgeRef.Repository = "other"
			case "commit":
				v.Commit = "c2"
			case "object":
				v.KnowledgeRef.Object = "b"
			case "address":
				v.Address.ObjectID = "b"
			}
			r.values["c1"]["a"] = v
			if _, err := c.ReadMany(r, "c1", []objectmodel.ObjectID{"a"}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				t.Fatalf("wrong basis accepted: %v", err)
			}
			if c.Stats().Entries != 0 {
				t.Fatal("invalid result cached")
			}
		})
	}
	c := newCache(t, cache.Config{})
	r := repo("kr://one")
	read(t, c, r, "c1", "a")
	want := errors.New("authority unavailable")
	r.err = want
	if _, err := c.ReadMany(r, "c1", []objectmodel.ObjectID{"a", "b"}); !errors.Is(err, want) {
		t.Fatal(err)
	}
	r.err = nil
	read(t, c, r, "c1", "a", "b")
	if len(r.batches) != 3 {
		t.Fatal("source error incorrectly retained", r.batches)
	}
}

func TestCacheLRUEvictionByteBudgetAndLifecycle(t *testing.T) {
	c := newCache(t, cache.Config{MaxEntries: 2})
	r := repo("kr://one")
	read(t, c, r, "c1", "a", "b")
	read(t, c, r, "c1", "a")
	read(t, c, r, "c1", "c")
	read(t, c, r, "c1", "a")
	if len(r.batches) != 2 {
		t.Fatal("recent object evicted", r.batches)
	}
	read(t, c, r, "c1", "b")
	if c.Stats().Entries != 2 || c.Stats().Evictions != 2 {
		t.Fatal(c.Stats())
	}
	c.Clear()
	if c.Stats().Entries != 0 || c.Stats().Bytes != 0 {
		t.Fatal(c.Stats())
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	read(t, c, r, "c1", "a")
	read(t, c, r, "c1", "a")
	if c.Stats().Entries != 0 {
		t.Fatal("closed cache retained data")
	}
	bounded := newCache(t, cache.Config{MaxBytes: 1024})
	r.values["c1"]["a"] = value(r.id, "c1", "a", strings.Repeat("x", 2048))
	read(t, bounded, r, "c1", "a")
	read(t, bounded, r, "c1", "b", "c")
	if s := bounded.Stats(); s.Bytes > 1024 || s.Bypasses == 0 {
		t.Fatal(s)
	}
}

func TestWarmRefreshesHotObjectsWithoutDiscardingOldPin(t *testing.T) {
	c := newCache(t, cache.Config{WarmLimit: 2, WarmBatchSize: 1})
	r := repo("kr://one")
	read(t, c, r, "c1", "a", "b")
	r.values["c2"]["a"] = value(r.id, "c2", "a", map[string]any{"nonIndexedField": "updated"})
	if err := c.Warm(context.Background(), r, "c2"); err != nil {
		t.Fatal(err)
	}
	if r.pageCalls != 0 || len(r.batches) != 3 || len(r.batches[1]) != 1 || len(r.batches[2]) != 1 {
		t.Fatalf("unbounded warm: %+v", r)
	}
	if got := read(t, c, r, "c1", "a")["a"].Value; got != "c1a" {
		t.Fatal("old pin changed", got)
	}
	if got := read(t, c, r, "c2", "a")["a"].Value; !reflect.DeepEqual(got, map[string]any{"nonIndexedField": "updated"}) {
		t.Fatal(got)
	}
	if len(r.batches) != 3 {
		t.Fatal("warm cache missed", r.batches)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Warm(ctx, r, "c2"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestColdWarmIsOptInAndOnlyOneBoundedPage(t *testing.T) {
	r := repo("kr://one")
	c := newCache(t, cache.Config{WarmLimit: 2, WarmBatchSize: 1})
	if err := c.Warm(context.Background(), r, "c1"); err != nil {
		t.Fatal(err)
	}
	if r.pageCalls != 0 || len(r.batches) != 0 {
		t.Fatal("cold scan was not opt in")
	}
	c = newCache(t, cache.Config{ColdStart: true, WarmLimit: 2, WarmBatchSize: 1})
	if err := c.Warm(context.Background(), r, "c1"); err != nil {
		t.Fatal(err)
	}
	if r.pageCalls != 1 || r.pageLimit != 2 || len(r.batches) != 2 {
		t.Fatalf("calls=%d limit=%d batches=%v", r.pageCalls, r.pageLimit, r.batches)
	}
}

func TestCacheConcurrentReadsAndWarmAreIsolated(t *testing.T) {
	c := newCache(t, cache.Config{MaxEntries: 4})
	r := repo("kr://one")
	var wg sync.WaitGroup
	for n := 0; n < 16; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				commit := kernel.CommitID(fmt.Sprintf("c%d", 1+n%2))
				if _, err := c.ReadMany(r, commit, []objectmodel.ObjectID{"a", "b"}); err != nil {
					t.Error(err)
				}
				if err := c.Warm(context.Background(), r, commit); err != nil {
					t.Error(err)
				}
			}
		}(n)
	}
	wg.Wait()
	if s := c.Stats(); s.Entries > 4 {
		t.Fatal(s)
	}
}

func TestColdWarmCompletionMarkersAreBoundedAndClearable(t *testing.T) {
	c := newCache(t, cache.Config{ColdStart: true, MaxEntries: 2})
	r := repo("kr://empty")
	r.values = nil
	for i := 0; i < 3; i++ {
		if err := c.Warm(context.Background(), r, "c1"); err != nil {
			t.Fatal(err)
		}
	}
	if r.pageCalls != 1 {
		t.Fatalf("repeated empty cold scan: %d", r.pageCalls)
	}
	for _, commit := range []kernel.CommitID{"c2", "c3", "c4"} {
		if err := c.Warm(context.Background(), r, commit); err != nil {
			t.Fatal(err)
		}
	}
	if s := c.Stats(); s.WarmMarkers > 2 || s.Entries != 0 || s.Bytes > 64<<20 {
		t.Fatal(s)
	}
	c.Clear()
	if err := c.Warm(context.Background(), r, "c4"); err != nil {
		t.Fatal(err)
	}
	if r.pageCalls != 5 {
		t.Fatalf("clear did not reset marker: %d", r.pageCalls)
	}
}

type blockedRepository struct {
	*repository
	started chan struct{}
	release chan struct{}
}

func (r *blockedRepository) ReadMany(ids []objectmodel.ObjectID, commit kernel.CommitID) (map[objectmodel.ObjectID]objectmodel.KnowledgeValue, error) {
	close(r.started)
	<-r.release
	return r.repository.ReadMany(ids, commit)
}

func TestClearAndCloseDoNotAllowInflightRefill(t *testing.T) {
	for _, closeCache := range []bool{false, true} {
		t.Run(fmt.Sprint(closeCache), func(t *testing.T) {
			c := newCache(t, cache.Config{})
			r := &blockedRepository{repository: repo("kr://one"), started: make(chan struct{}), release: make(chan struct{})}
			done := make(chan error, 1)
			go func() { _, err := c.ReadMany(r, "c1", []objectmodel.ObjectID{"a"}); done <- err }()
			<-r.started
			if closeCache {
				if err := c.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				c.Clear()
			}
			close(r.release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if s := c.Stats(); s.Entries != 0 || s.Bytes != 0 {
				t.Fatal(s)
			}
		})
	}
}

func TestWarmMissingObjectNeverFallsBackToPreviousCommit(t *testing.T) {
	c := newCache(t, cache.Config{})
	r := repo("kr://one")
	read(t, c, r, "c1", "a")
	delete(r.values["c2"], "a")
	if err := c.Warm(context.Background(), r, "c2"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, c, r, "c2", "a"); len(got) != 0 {
		t.Fatal("deleted value served from old commit", got)
	}
	if got := read(t, c, r, "c1", "a")["a"].Value; got != "c1a" {
		t.Fatal(got)
	}
	r.err = errors.New("temporary source failure")
	if err := c.Warm(context.Background(), r, "c2"); !errors.Is(err, r.err) {
		t.Fatal(err)
	}
	if c.Stats().WarmFailures != 1 {
		t.Fatal(c.Stats())
	}
}

type extraResultRepository struct{ *repository }

func (r *extraResultRepository) ReadMany(ids []objectmodel.ObjectID, commit kernel.CommitID) (map[objectmodel.ObjectID]objectmodel.KnowledgeValue, error) {
	out, err := r.repository.ReadMany(ids, commit)
	if err == nil {
		out["unexpected"] = value(r.id, commit, "unexpected", "unexpected")
	}
	return out, err
}
func TestBatchValidationIsAtomicAndRejectsUnexpectedIdentities(t *testing.T) {
	c := newCache(t, cache.Config{})
	r := &extraResultRepository{repo("kr://one")}
	if _, err := c.ReadMany(r, "c1", []objectmodel.ObjectID{"a"}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatal(err)
	}
	if c.Stats().Entries != 0 {
		t.Fatal("partial result cached before rejecting malformed batch")
	}
	r2 := repo("kr://two")
	wrong := r2.values["c1"]["b"]
	wrong.Commit = "c2"
	r2.values["c1"]["b"] = wrong
	if _, err := c.ReadMany(r2, "c1", []objectmodel.ObjectID{"a", "b"}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatal(err)
	}
	if c.Stats().Entries != 0 {
		t.Fatal("part of wrong-basis batch cached")
	}
}

func TestCacheAddressErrorsAndVersionIsolation(t *testing.T) {
	c := newCache(t, cache.Config{})
	r := repo("kr://one")
	a := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "a", AspectName: "private"}
	if _, err := c.ReadAddress(r, "c1", a); err != nil {
		t.Fatal(err)
	}
	if got, err := c.ReadAddress(r, "c2", a); err != nil || got.Commit != "c2" {
		t.Fatal(got, err)
	}
	r.err = errors.New("offline")
	if _, err := c.ReadAddress(r, "c3", a); !errors.Is(err, r.err) {
		t.Fatal(err)
	}
	r.err = nil
	if _, err := c.ReadAddress(r, "c3", a); kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved {
		t.Fatal(err)
	}
	wrong := r.values["c2"]["b"]
	wrong.Commit = "c1"
	r.values["c2"]["b"] = wrong
	a.ObjectID = "b"
	if _, err := c.ReadAddress(r, "c2", a); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatal(err)
	}
}

func TestWarmPreservesExactAddressReadShape(t *testing.T) {
	c := newCache(t, cache.Config{WarmLimit: 2, WarmBatchSize: 1})
	r := repo("kr://one")
	a := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "a", AspectName: "private", MemberKey: "m1"}
	if _, err := c.ReadAddress(r, "c1", a); err != nil {
		t.Fatal(err)
	}
	read(t, c, r, "c1", "b")
	if err := c.Warm(context.Background(), r, "c2"); err != nil {
		t.Fatal(err)
	}
	if len(r.addresses) != 2 || len(r.batches) != 2 {
		t.Fatalf("wrong warm shape: address=%v batches=%v", r.addresses, r.batches)
	}
	if _, err := c.ReadAddress(r, "c2", a); err != nil {
		t.Fatal(err)
	}
	read(t, c, r, "c2", "b")
	if len(r.addresses) != 2 || len(r.batches) != 2 {
		t.Fatal("new exact read missed warmed value")
	}
	if _, err := c.ReadAddress(r, "c1", a); err != nil {
		t.Fatal(err)
	}
	if len(r.addresses) != 2 {
		t.Fatal("old pinned address was discarded")
	}
}

func TestColdWarmCanRepopulateEvictedBodiesAtSameCommit(t *testing.T) {
	c := newCache(t, cache.Config{MaxEntries: 1, ColdStart: true})
	r := repo("kr://one")
	if err := c.Warm(context.Background(), r, "c1"); err != nil {
		t.Fatal(err)
	}
	read(t, c, repo("kr://two"), "c1", "a")
	if err := c.Warm(context.Background(), r, "c1"); err != nil {
		t.Fatal(err)
	}
	if r.pageCalls != 2 {
		t.Fatalf("eviction prevented cold rewarm: calls=%d", r.pageCalls)
	}
}

// Warm reads repository identity twice before choosing work. Blocking its next
// identity read pauses after the epoch check but before the public read used to
// capture its own epoch. This makes the Clear race deterministic without sleeps
// or production-only test hooks.
type identityBlockedRepository struct {
	*repository
	idCalls atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (r *identityBlockedRepository) ID() kernel.RepositoryID {
	if r.idCalls.Add(1) == 3 {
		close(r.started)
		<-r.release
	}
	return r.repository.ID()
}

func TestClearCancelsWarmBetweenEpochCheckAndReadStart(t *testing.T) {
	for _, addressRead := range []bool{false, true} {
		t.Run(fmt.Sprintf("address=%t", addressRead), func(t *testing.T) {
			c := newCache(t, cache.Config{})
			base := repo("kr://one")
			if addressRead {
				a := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "a", AspectName: "private"}
				if _, err := c.ReadAddress(base, "c1", a); err != nil {
					t.Fatal(err)
				}
			} else {
				read(t, c, base, "c1", "a")
			}
			r := &identityBlockedRepository{repository: base, started: make(chan struct{}), release: make(chan struct{})}
			done := make(chan error, 1)
			go func() { done <- c.Warm(context.Background(), r, "c2") }()
			select {
			case <-r.started:
			case <-time.After(3 * time.Second):
				close(r.release)
				t.Fatal("Warm did not reach read-start boundary")
			}
			c.Clear()
			close(r.release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if s := c.Stats(); s.Entries != 0 || s.Bytes != 0 || s.WarmMarkers != 0 {
				t.Fatalf("old Warm refilled after Clear: %+v", s)
			}
		})
	}
}

// Embed only the Repository interface so this fixture deliberately does not
// expose the base fixture's optional BatchReadStore capability.
type singleReadRepository struct {
	objectmodel.Repository
	base    *repository
	calls   []objectmodel.ObjectID
	failID  objectmodel.ObjectID
	failure error
}

func (r *singleReadRepository) Read(id objectmodel.ObjectID, commit kernel.CommitID) (objectmodel.KnowledgeValue, error) {
	r.calls = append(r.calls, id)
	if id == r.failID {
		return objectmodel.KnowledgeValue{}, r.failure
	}
	if v, ok := r.base.values[commit][id]; ok {
		return v, nil
	}
	return objectmodel.KnowledgeValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "missing")
}

func TestSourceReadsCountsEveryNonBatchAuthorityCallIncludingFailure(t *testing.T) {
	c := newCache(t, cache.Config{})
	base := repo("kr://one")
	r := &singleReadRepository{Repository: base, base: base, failID: "failure", failure: errors.New("source failed")}
	if _, ok := any(r).(objectmodel.BatchReadStore); ok {
		t.Fatal("fixture unexpectedly exposes batch reads")
	}
	read(t, c, r, "c1", "a", "a", "b", "missing")
	if got := c.Stats().SourceReads; got != 3 || got != uint64(len(r.calls)) {
		t.Fatalf("source count=%d, actual calls=%v", got, r.calls)
	}
	read(t, c, r, "c1", "a", "b")
	if got := c.Stats().SourceReads; got != 3 {
		t.Fatalf("hits changed source count: %d", got)
	}
	if _, err := c.ReadMany(r, "c1", []objectmodel.ObjectID{"a", "c", "failure", "missing"}); !errors.Is(err, r.failure) {
		t.Fatal(err)
	}
	if got := c.Stats().SourceReads; got != 5 || got != uint64(len(r.calls)) {
		t.Fatalf("failed call count=%d, actual calls=%v", got, r.calls)
	}
	read(t, c, r, "c1", "c")
	if got := c.Stats().SourceReads; got != 6 || got != uint64(len(r.calls)) {
		t.Fatalf("retry count=%d, actual calls=%v", got, r.calls)
	}
}

func TestCacheClonesPerAddressRecordedProvenance(t *testing.T) {
	c := newCache(t, cache.Config{})
	r := repo("kr://unit-sources")
	v := r.values["c1"]["a"]
	v.Declarations = []objectmodel.UnitDeclaration{
		{Address: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "a", AspectName: "columns", MemberKey: "id"}, Provenance: &objectmodel.ProvenanceEnvelope{OriginKind: objectmodel.OriginSource, ActorRef: "source-owner", SourceRefs: []string{"source/one"}, EvidenceRefs: []string{"evidence/one"}}},
		{Address: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "a", AspectName: "columns", MemberKey: "status"}, Provenance: &objectmodel.ProvenanceEnvelope{OriginKind: objectmodel.OriginDerivation, ActorRef: "deriver", SourceRefs: []string{"source/two"}, EvidenceRefs: []string{"evidence/two"}, InputWorkspaceVersionRef: "fixed-input", Algorithm: &objectmodel.AlgorithmRef{DerivationSpecRef: "algorithm/v1", ModelRef: "model/v1", CodeHash: "original"}}},
	}
	r.values["c1"]["a"] = v
	first := read(t, c, r, "c1", "a")["a"]
	for _, declaration := range first.Declarations {
		declaration.Provenance.SourceRefs[0] = "caller-changed"
		declaration.Provenance.EvidenceRefs[0] = "caller-changed"
		declaration.Provenance.ActorRef = "caller-changed"
	}
	first.Declarations[1].Provenance.Algorithm.CodeHash = "caller-changed"
	first.Declarations[1].Provenance.InputWorkspaceVersionRef = "caller-changed"
	second := read(t, c, r, "c1", "a")["a"]
	if !reflect.DeepEqual(second, v) {
		t.Fatalf("caller mutation leaked across per-Address sources: %#v", second.Declarations)
	}
	for _, declaration := range v.Declarations {
		declaration.Provenance.SourceRefs[0] = "authority-changed"
		declaration.Provenance.EvidenceRefs[0] = "authority-changed"
		declaration.Provenance.ActorRef = "authority-changed"
	}
	v.Declarations[1].Provenance.Algorithm.ModelRef = "authority-changed"
	third := read(t, c, r, "c1", "a")["a"]
	if !reflect.DeepEqual(third, second) || len(r.batches) != 1 {
		t.Fatalf("authority mutation leaked into cached fixed sources: third=%#v second=%#v batches=%v", third.Declarations, second.Declarations, r.batches)
	}
}
