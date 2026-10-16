package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	indeximplementation "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/liveaccess"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/object/writer"
	"kc/observability"
	"kc/snapshot"
)

type partialScopeStore struct {
	snapshot.Store
	snapshot.TreeStore
	snapshot.DirectoryReader
	blocked   bool
	reads     map[string][]kernel.CommitID
	afterRead func(string)
}

func (s *partialScopeStore) ReadFile(path string, at kernel.CommitID) ([]byte, error) {
	s.reads[path] = append(s.reads[path], at)
	if s.blocked && path == "private/sibling.yaml" {
		return nil, fmt.Errorf("consumer read an unselected sibling")
	}
	data, err := s.TreeStore.ReadFile(path, at)
	if s.afterRead != nil {
		s.afterRead(path)
	}
	return data, err
}

type partialScopeEngine struct {
	workspaceBudgetEngine
	beforeRetrieve func() error
}

func (e *partialScopeEngine) Retrieve(request ports.RetrieveRequest) (ports.CandidatePage, error) {
	if e.beforeRetrieve != nil {
		if err := e.beforeRetrieve(); err != nil {
			return ports.CandidatePage{}, err
		}
	}
	page, err := e.workspaceBudgetEngine.Retrieve(request)
	for i := range page.Candidates {
		page.Candidates[i].Repository = request.Spec.Repository
	}
	return page, err
}

type partialScopeRuntime struct {
	requests []liveaccess.StateLookupRequest
	failure  error
}

func (r *partialScopeRuntime) LookupState(_ context.Context, request liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
	r.requests = append(r.requests, request)
	if r.failure != nil {
		return liveaccess.StateObservation{}, r.failure
	}
	return liveaccess.StateObservation{Value: map[string]any{"status": "current"}, Basis: objectmodel.ObservationBasis{
		BindingGeneration: "query-runtime", Consistency: objectmodel.ObservationRepeatable,
		SourceRevision: "runtime-7", ObservedAt: "2026-10-16T00:00:00Z",
	}}, nil
}

type partialScopeCancelReader struct {
	*liveservice.Service
	cancel context.CancelFunc
	reads  int
	store  *partialScopeStore
}

func (r *partialScopeCancelReader) ReadReference(ctx context.Context, ref objectmodel.Reference) ([]objectmodel.ObservedValue, error) {
	r.store.afterRead = func(path string) {
		if path == "public/article.yaml" || path == "public/runtime.yaml" {
			r.reads++
			r.cancel()
		}
	}
	defer func() { r.store.afterRead = nil }()
	return r.Service.ReadReference(ctx, ref)
}

type partialScopeSource struct {
	store     *partialScopeStore
	writer    *writer.Writer
	commit    kernel.CommitID
	aspect    string
	field     string
	schema    objectmodel.ObjectID
	publicPut objectmodel.Operation
}

func newPartialScopeSource(t *testing.T, name, aspect, field, value string, dynamic bool) partialScopeSource {
	t.Helper()
	raw := testkit.MakeTreeStore(t, "kr://partial-scope/"+name)
	writes := snapshot.NewRegistry()
	if err := writes.Add(raw); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(writes, nil)
	if err != nil {
		t.Fatal(err)
	}
	schemaID := objectmodel.ObjectID("schema/" + name)
	public := objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "article/one", AspectName: aspect},
		SchemaRef: string(schemaID), PathHint: "public/article.yaml", Value: map[string]any{field: value}}
	declaration := testkit.IndexUnder("indexes", testkit.IndexField(schemaID, field, "keyword"))
	declaration.Address.ObjectID = objectmodel.ObjectID("index/" + name)
	operations := []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: schemaID}, PathHint: "_schemas/article.yaml",
			Value: map[string]any{"entity": "Article", "aspect": aspect, "fields": map[string]any{field: map[string]any{"type": "string"}}}},
		declaration, public,
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "article/one", AspectName: "private"},
			PathHint: "private/sibling.yaml", Value: map[string]any{"secret": "unselected"}},
	}
	if dynamic {
		operations = append(operations,
			objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/runtime"}, PathHint: "_schemas/runtime.yaml",
				Value: map[string]any{"entity": "Article", "aspect": "runtime", "fields": map[string]any{"status": map[string]any{"type": "string"}}}},
			bindingfixture.WithPath(bindingfixture.PutDescriptor("resource/runtime", bindingfixture.Descriptor("https://runtime.example")), "public/descriptor.yaml"),
			bindingfixture.WithPath(bindingfixture.PutAspect("article/one", "runtime", "schema/runtime", "resource/runtime"), "public/runtime.yaml"))
	}
	base := testkit.MustHead(t, raw, snapshot.DefaultRef)
	receipt, err := w.Commit("publish-"+name, objectmodel.ChangeSet{TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: base, ExpectedTargetCommit: base, Operations: operations})
	if err != nil {
		t.Fatal(err)
	}
	directory, ok := snapshot.DirectoryReaderOf(raw)
	if !ok {
		t.Fatal("fixture source requires bounded maintenance coordinates")
	}
	return partialScopeSource{store: &partialScopeStore{Store: raw, TreeStore: raw.(snapshot.TreeStore), DirectoryReader: directory, reads: map[string][]kernel.CommitID{}},
		writer: w, commit: receipt.Result.NewCommit, aspect: aspect, field: field, schema: schemaID, publicPut: public}
}

func TestDatasetSearchHydratesSelectedPartialSourcesAtTheFixedRevision(t *testing.T) {
	for _, test := range []struct {
		name    string
		cross   bool
		dynamic bool
		failure bool
		cancel  bool
	}{
		{"one partial source", false, false, false, false},
		{"composed partial sources", true, false, false, false},
		{"composed sources with a selected external unit", true, true, false, false},
		{"one partial source with an external failure", false, true, true, false},
		{"selected external failure stays local", true, true, true, false},
		{"cancellation stops the next selected unit", false, true, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			sources := []partialScopeSource{newPartialScopeSource(t, "title", "properties", "name", "allowed", test.dynamic)}
			if test.cross {
				sources = append(sources, newPartialScopeSource(t, "body", "definition", "body", "fixed body", false))
			}
			registry := snapshot.NewRegistry()
			pin := reader.DatasetPin{SetID: "articles", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{}}
			for _, source := range sources {
				if err := registry.Add(source.store); err != nil {
					t.Fatal(err)
				}
				pin.Repositories[source.store.ID()] = source.commit
				for _, prefix := range []string{"public", "_schemas", "indexes"} {
					pin.Items = append(pin.Items, reader.DatasetItem{Repository: source.store.ID(), Commit: source.commit, Kind: "prefix", Prefix: prefix, Target: source.aspect + "/" + prefix})
				}
			}
			repositories := reader.NewReader(registry)
			lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
				return repositories.Require(id, kernel.ErrObjectRefUnresolved)
			}
			engine := &partialScopeEngine{workspaceBudgetEngine: workspaceBudgetEngine{superset: true}}
			idx := indeximplementation.NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
			t.Cleanup(func() { _ = idx.Close() })
			if _, err := idx.PrepareDataset(context.Background(), lookup, pin); err != nil {
				t.Fatal(err)
			}
			for _, source := range sources {
				source.store.blocked = true
				source.store.reads = map[string][]kernel.CommitID{}
			}
			advanced := false
			engine.beforeRetrieve = func() error {
				if advanced {
					return nil
				}
				advanced = true
				for _, source := range sources {
					update := source.publicPut
					update.Value = map[string]any{source.field: "changed after query planning"}
					if _, err := source.writer.Commit("advance-"+string(source.store.ID()), objectmodel.ChangeSet{TargetRepository: source.store.ID(), TargetRef: snapshot.DefaultRef,
						BaseCommit: source.commit, ExpectedTargetCommit: source.commit, Operations: []objectmodel.Operation{update}}); err != nil {
						return err
					}
				}
				return nil
			}
			serving := reader.Open(lookup, pin)
			runtime := &partialScopeRuntime{}
			if test.failure {
				runtime.failure = kernel.Fail(kernel.ErrTemporaryUnavailable, "external source unavailable")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			logical := liveservice.Open(serving, runtime, observability.IdentityContext{Principal: "consumer"})
			cancelReader := &partialScopeCancelReader{Service: logical, cancel: cancel, store: sources[0].store}
			resolves := 0
			executor := DatasetSearchExecutor{Authorize: func(context.Context) error { return nil }, Repositories: repositories, Projection: idx,
				Resolve: func(context.Context) (*reader.Serving, LogicalHydrator, error) {
					resolves++
					if test.cancel {
						return serving, cancelReader, nil
					}
					return serving, logical, nil
				}, Deliver: func(_ context.Context, hit retrieval.ObjectHit) (retrieval.ObjectHit, error) { return hit, nil }}
			request := retrieval.SearchOf(retrieval.SearchClause{Op: retrieval.OpEQ, Field: &retrieval.FieldRef{Schema: sources[0].schema, Aspect: sources[0].aspect, Path: sources[0].field}, Value: "allowed"})
			result, err := executor.Execute(ctx, request)
			if test.cancel {
				if !errors.Is(err, context.Canceled) || cancelReader.reads != 1 || len(runtime.requests) != 0 || len(result.Hits) != 0 {
					t.Fatalf("cancelled partial read continued or succeeded: reads=%d result=%#v error=%v", cancelReader.reads, result, err)
				}
				return
			}
			if err != nil || !advanced || resolves != 1 || len(result.Hits) != 1 || result.Dataset == nil || result.Dataset.Revision != 1 || !reflect.DeepEqual(result.SearchView.Snapshots, pin.Repositories) {
				t.Fatalf("formal Dataset search lost the selected fixed scope: result=%#v advanced=%v resolves=%d error=%v", result, advanced, resolves, err)
			}
			assertPartialScopeHit(t, sources, result.Hits[0], test.dynamic, runtime)
		})
	}
}

func assertPartialScopeHit(t *testing.T, sources []partialScopeSource, hit retrieval.ObjectHit, dynamic bool, runtime *partialScopeRuntime) {
	t.Helper()
	body := hit.Object.Value.(map[string]any)
	if !reflect.DeepEqual(body["properties"], map[string]any{"name": "allowed"}) || body["private"] != nil {
		t.Fatalf("query used a changed or unselected source value: %#v", hit)
	}
	if len(sources) > 1 && (!reflect.DeepEqual(body["definition"], map[string]any{"body": "fixed body"}) || len(hit.Version.Sources) != 2) {
		t.Fatalf("composed hit dropped a selected source: %#v", hit)
	}
	for _, source := range sources {
		if len(source.store.reads["private/sibling.yaml"]) != 0 {
			t.Fatal("consumer hydration read an unselected sibling")
		}
		for _, at := range source.store.reads["public/article.yaml"] {
			if at != source.commit {
				t.Fatalf("consumer followed source HEAD: got=%s want=%s", at, source.commit)
			}
		}
	}
	if len(sources) == 1 && hit.Version.DeclarationCommit != sources[0].commit {
		t.Fatalf("single-source declaration commit changed: %#v", hit.Version)
	}
	for _, version := range hit.Version.Sources {
		found := false
		for _, source := range sources {
			found = found || version.Repository == source.store.ID() && version.Commit == source.commit
		}
		if !found {
			t.Fatalf("composed version escaped fixed source commits: %#v", hit.Version)
		}
	}
	if runtime.failure != nil {
		failure, ok := body["runtime"].(liveservice.ValueFailure)
		if !ok || failure.Error.Code != kernel.ErrTemporaryUnavailable || len(runtime.requests) != 1 || len(hit.Version.Observations) != 0 {
			t.Fatalf("partial-source assembly swallowed the external unit failure: %#v", hit)
		}
		if len(sources) == 1 && hit.Object.Digest != "" {
			t.Fatalf("external failure acquired a static content digest: %#v", hit)
		}
		return
	}
	if dynamic {
		if len(runtime.requests) != 1 || len(hit.Version.Observations) != 1 || !reflect.DeepEqual(body["runtime"], map[string]any{"status": "current"}) {
			t.Fatalf("selected dynamic unit lost its current value or observation: %#v calls=%d", hit, len(runtime.requests))
		}
		observation := hit.Version.Observations[0]
		if observation.DeclarationCommit != sources[0].commit || observation.DeclarationDigest == "" || observation.DescriptorDigest == "" || observation.Basis.SourceRevision != "runtime-7" || runtime.requests[0].Binding.DeclarationCommit != sources[0].commit {
			t.Fatalf("dynamic unit lost fixed declaration or runtime basis: %#v", hit)
		}
	} else if len(hit.Version.Observations) != 0 || len(runtime.requests) != 0 {
		t.Fatalf("static query unexpectedly observed an external source: %#v", hit)
	}
}
