package datasets

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

type publicationFaultAuthority struct {
	snapshot.Store
	snapshot.TreeStore
	snapshot.HistoryStore
	err      error
	attempts int
}

func (a *publicationFaultAuthority) ApplyTreeCommit(change snapshot.TreeChangeSet) (kernel.CommitID, error) {
	a.attempts++
	if a.err != nil {
		return "", a.err
	}
	return a.TreeStore.ApplyTreeCommit(change)
}

type recordingPublicationRegistry struct {
	DatasetRegistry
	events *[]string
}

func (r recordingPublicationRegistry) PrepareDataset(id string, revision int, sources []dataset.DatasetSource) (dataset.Dataset, error) {
	*r.events = append(*r.events, "freeze")
	return r.DatasetRegistry.PrepareDataset(id, revision, sources)
}

func (r recordingPublicationRegistry) PublishDataset(def dataset.Dataset) (dataset.Dataset, error) {
	*r.events = append(*r.events, "accept")
	return r.DatasetRegistry.PublishDataset(def)
}

type publicationFailureFixture struct {
	setup         testkit.Setup
	authority     *publicationFaultAuthority
	registry      dataset.Registry
	service       dataset.Service
	sources       []dataset.DatasetSource
	oldRelease    dataset.Dataset
	nextCommit    kernel.CommitID
	acceptedState dataset.State
	authorityHead string
}

func newPublicationFailureFixture(t *testing.T) publicationFailureFixture {
	t.Helper()
	s := testkit.NewSetup(t, "kr://declarative-index/publication-source")
	receipt, err := s.Writer.Commit("publish-source-a", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"}, Value: map[string]any{
				"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string"}},
			}},
			testkit.IndexText("schema/article", "body"),
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/current"}, SchemaRef: "schema/article", Value: map[string]any{"body": "runbook a"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw := testkit.MakeTreeStore(t, "kr://declarative-index/publication-authority")
	authority := &publicationFaultAuthority{Store: raw, TreeStore: raw.(snapshot.TreeStore), HistoryStore: raw.(snapshot.HistoryStore)}
	registry, err := dataset.CreateSnapshotRegistry(authority, snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	service, err := dataset.NewService(s.Store, registry)
	if err != nil {
		t.Fatal(err)
	}
	testkit.RegisterMounted(t, service, s.Store)
	sources := []dataset.DatasetSource{{Repository: s.RepositoryID, Selector: snapshot.DefaultRef, Path: dataset.MountPath("articles")}}
	old, err := service.PrepareDataset("reading", 1, sources)
	if err != nil {
		t.Fatal(err)
	}
	old.Metadata = map[string]string{"description": "accepted a"}
	old, err = service.PublishDataset(old)
	if err != nil {
		t.Fatal(err)
	}
	next := commitPublicationSource(t, s, receipt.Result.CommitID, "b")
	head, err := registry.Head()
	if err != nil {
		t.Fatal(err)
	}
	authority.attempts = 0
	return publicationFailureFixture{
		setup: s, authority: authority, registry: registry, service: service, sources: sources,
		oldRelease: old, nextCommit: next, acceptedState: dataset.NormalizeState(service.DumpState()), authorityHead: head,
	}
}

func commitPublicationSource(t *testing.T, s testkit.Setup, base kernel.CommitID, revision string) kernel.CommitID {
	t.Helper()
	receipt, err := s.Writer.Commit("publish-source-"+revision, objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: base, ExpectedTargetCommit: base,
		Operations: []objectmodel.Operation{{Op: objectmodel.OpPut,
			Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/current"}, SchemaRef: "schema/article",
			Value: map[string]any{"body": "runbook " + revision},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return receipt.Result.CommitID
}

func (f publicationFailureFixture) assertUnchanged(t *testing.T) {
	t.Helper()
	if state := dataset.NormalizeState(f.service.DumpState()); !reflect.DeepEqual(state, f.acceptedState) {
		t.Fatalf("failed publication changed serving state or release history: before=%#v after=%#v", f.acceptedState, state)
	}
	state, err := f.registry.Load()
	if err != nil || !reflect.DeepEqual(dataset.NormalizeState(state), f.acceptedState) {
		t.Fatalf("failed publication changed persisted releases: state=%#v error=%v", state, err)
	}
	if head, err := f.registry.Head(); err != nil || head != f.authorityHead {
		t.Fatalf("failed publication advanced registry authority: head=%s error=%v", head, err)
	}
	if current, err := f.service.Set("reading"); err != nil || !reflect.DeepEqual(current, f.oldRelease) {
		t.Fatalf("old release changed: release=%#v error=%v", current, err)
	}
	if _, err := f.service.DatasetVersion("reading", 2); kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
		t.Fatalf("unaccepted release became visible: %v", err)
	}
	if head := testkit.MustHead(t, f.setup.Repo, snapshot.DefaultRef); head != f.nextCommit {
		t.Fatalf("publication failure rolled back Writer source: head=%s want=%s", head, f.nextCommit)
	}
	value, err := f.setup.Repo.Read("article/current", f.nextCommit)
	if err != nil || value.Value.(map[string]any)["body"] != "runbook b" {
		t.Fatalf("accepted source content was lost: value=%#v error=%v", value, err)
	}
}

func TestDatasetPublicationFailuresPreserveAcceptedReleaseAndSourceCommit(t *testing.T) {
	for _, stage := range []string{"freeze", "accept", "cancel-before-accept"} {
		t.Run(stage, func(t *testing.T) {
			f := newPublicationFailureFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var events []string
			var frozen dataset.Dataset
			problem := kernel.Fail(kernel.ErrTemporaryUnavailable, "registry acceptance unavailable")
			executor := DatasetPublisher{
				Registry: recordingPublicationRegistry{DatasetRegistry: f.service, events: &events},
				Authorize: func(context.Context, DatasetPublication) error {
					events = append(events, "authorize")
					return nil
				},
				Prepare: func(_ context.Context, def dataset.Dataset) (dataset.Dataset, error) {
					events = append(events, "prepare")
					frozen = def
					if len(def.Sources) != 1 || def.Sources[0].Commit != f.nextCommit || len(def.Items) != 1 || def.Items[0].Commit != f.nextCommit ||
						def.Metadata["description"] != "candidate b" || !reflect.DeepEqual(dataset.NormalizeState(f.service.DumpState()), f.acceptedState) {
						t.Fatalf("preparation did not receive frozen b while a remained serving: %#v", def)
					}
					if stage == "accept" {
						f.authority.err = problem
					} else {
						cancel()
					}
					return def, nil
				},
			}
			request := DatasetPublication{Dataset: "reading", Revision: 2, Sources: f.sources, Metadata: map[string]string{"description": "candidate b"}}
			if stage == "freeze" {
				request.Sources = []dataset.DatasetSource{{Repository: f.setup.RepositoryID, Selector: "refs/heads/missing"}}
			}
			result, err := executor.Execute(ctx, request)
			want := []string{"authorize", "freeze"}
			switch stage {
			case "freeze":
				if kernel.CodeOf(err) != kernel.ErrDatasetInvalid || !strings.Contains(err.Error(), "has no ref refs/heads/missing") {
					t.Fatalf("freeze failure lost its cause: %v", err)
				}
			case "accept":
				want = append(want, "prepare", "accept")
				if !errors.Is(err, problem) || kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || f.authority.attempts != 1 {
					t.Fatalf("actual registry rejection was lost or bypassed: error=%v attempts=%d", err, f.authority.attempts)
				}
			default:
				want = append(want, "prepare")
				if !errors.Is(err, context.Canceled) || f.authority.attempts != 0 {
					t.Fatalf("cancellation reached registry acceptance: error=%v attempts=%d", err, f.authority.attempts)
				}
			}
			if !reflect.DeepEqual(events, want) || !reflect.DeepEqual(result, dataset.Dataset{}) {
				t.Fatalf("failed publication reported acceptance or ran later stages: events=%v result=%#v", events, result)
			}
			f.assertUnchanged(t)
			if stage == "freeze" {
				return
			}
			// The repaired retry must use the already prepared b, even after the
			// source moves again. Restarting does not authorize reading HEAD c.
			latest := commitPublicationSource(t, f.setup, f.nextCommit, "c")
			f.authority.err = nil
			request.Sources = frozen.Sources
			executor.Prepare = func(_ context.Context, def dataset.Dataset) (dataset.Dataset, error) {
				if !reflect.DeepEqual(def, frozen) {
					t.Fatalf("retry changed its prepared basis: got=%#v want=%#v", def, frozen)
				}
				return def, nil
			}
			accepted, err := executor.Execute(context.Background(), request)
			if err != nil || !reflect.DeepEqual(accepted, frozen) {
				t.Fatalf("repaired retry failed to accept b: release=%#v error=%v", accepted, err)
			}
			old, err := f.service.DatasetVersion("reading", 1)
			if err != nil || !reflect.DeepEqual(old, f.oldRelease) || testkit.MustHead(t, f.setup.Repo, snapshot.DefaultRef) != latest {
				t.Fatalf("retry rewrote history or source HEAD: old=%#v error=%v", old, err)
			}
		})
	}
}
