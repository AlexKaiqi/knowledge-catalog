package invocation

import (
	"context"
	"encoding/json"
	"testing"

	retrieval "kc/discovery"
	"kc/home"
	"kc/internal/testkit"
	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

type namedConsumptionFixture struct {
	ws      *home.Home
	repos   []*testkit.ObjectRepository
	pin     dataset.ResolvedDataset
	sources []dataset.DatasetSource
	rules   []AllowRule
}

func newNamedConsumptionFixture(t *testing.T) namedConsumptionFixture {
	t.Helper()
	s := testkit.NewSetup(t, "kr://consumption/a")
	other := testkit.MakeRepository(t, "kr://consumption/b")
	if err := s.Store.Add(other); err != nil {
		t.Fatal(err)
	}
	repos := []*testkit.ObjectRepository{s.Repo, other}
	for i, repo := range repos {
		head := testkit.MustHead(t, repo, snapshot.DefaultRef)
		object := []objectmodel.ObjectID{"metric/gmv", "metric/count"}[i]
		if _, err := repo.ApplyObjectCommit(objectmodel.ChangeSet{
			TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: head, ExpectedTargetCommit: head,
			Operations: []objectmodel.Operation{{Op: objectmodel.OpPut,
				Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: object, AspectName: "definition"},
				Value:   map[string]any{"name": string(object)},
			}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	cat := testkit.OpenDatasets(t, s.Store)
	sources := []dataset.DatasetSource{{Repository: s.Repo.ID(), Selector: snapshot.DefaultRef}, {Repository: other.ID(), Selector: snapshot.DefaultRef}}
	if _, err := cat.DefineDataset("analysis", 1, sources); err != nil {
		t.Fatal(err)
	}
	pin, err := cat.ResolveDataset("analysis")
	if err != nil {
		t.Fatal(err)
	}
	rules := []AllowRule{{ID: "resolve", Principal: "reader", Dataset: "analysis", Actions: []string{"dataset.resolve"}}}
	for _, repo := range repos {
		rules = append(rules, AllowRule{ID: string(repo.ID()), Principal: "reader", Dataset: "analysis", Repo: string(repo.ID()), Ref: string(pin.Repositories[repo.ID()]), Actions: []string{"file.read"}})
	}
	f := namedConsumptionFixture{ws: &home.Home{Dir: t.TempDir(), Store: s.Store, Reader: reader.NewReader(s.Store), Datasets: cat}, repos: repos, pin: pin, sources: sources, rules: rules}
	f.allow(t, rules)
	return f
}

func (f namedConsumptionFixture) allow(t *testing.T, rules []AllowRule) {
	t.Helper()
	if err := WriteAllow(f.ws.Dir, AllowFile{Rules: rules}); err != nil {
		t.Fatal(err)
	}
}

func (f namedConsumptionFixture) flags() map[string]invoke.FlagValue {
	return map[string]invoke.FlagValue{"home": f.ws.Dir, "as": "reader", "dataset": "analysis", "revision": "1"}
}

func (f namedConsumptionFixture) invoke(action string, flags map[string]invoke.FlagValue, run Handler) invoke.RunResult {
	name := "object-read"
	if action == "file.read" {
		name = "object-search"
	}
	return InvokeApplicationWithTelemetryAtHome(context.Background(), nil, name, action,
		Command{Stage: invoke.StageGoverned, Run: run}, flags, nil, f.ws)
}

func namedConsumptionError(t *testing.T, result invoke.RunResult) kernel.ErrorCode {
	t.Helper()
	var body struct {
		Error *kernel.IngressError `json:"error"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &body); err != nil || body.Error == nil {
		t.Fatalf("expected typed failure: result=%#v decode=%v", result, err)
	}
	return body.Error.Code
}

// Synthetic typed search handlers still produce the same fixed-basis result
// envelope as the real executor, so retrieval evidence remains part of the test.
func namedConsumptionSearchOutput(pin reader.DatasetPin, hits ...retrieval.ObjectHit) retrieval.SearchResult {
	if hits == nil {
		hits = []retrieval.ObjectHit{}
	}
	return retrieval.SearchResult{
		SearchView:   retrieval.SearchView{Snapshots: pin.Repositories},
		Completeness: retrieval.CompletenessComplete,
		Hits:         hits,
		Dataset:      &retrieval.DatasetBasis{ID: pin.SetID, Revision: pin.Revision},
	}
}

func TestNamedMemberConsumptionTypedReadAndSearch(t *testing.T) {
	for _, action := range []string{"object.read", "file.read"} {
		t.Run(action, func(t *testing.T) {
			f := newNamedConsumptionFixture(t)
			called := false
			result := f.invoke(action, f.flags(), func(cx *Invocation) (any, error) {
				called = true
				serving, cat, err := OpenCompleteServing(cx, "metric/gmv")
				if err != nil {
					return nil, err
				}
				values, err := serving.Read("metric/gmv", nil)
				if err != nil || len(values) != 1 || values[0].Value == nil || values[0].Commit != f.pin.Repositories[f.repos[0].ID()] {
					t.Fatalf("fixed member read lost body/basis: values=%#v err=%v", values, err)
				}
				filtered := FilterObjectServingReads(cx.Home, cx.Flags, cat, []objectmodel.ObservedValue{{FederatedValue: values[0]}})
				if len(filtered) != 1 || !AllowedRepoAspectRead(cx.Home, cx.Flags, string(values[0].Repository), "metric/gmv", "definition") {
					t.Fatal("post-read source filter dropped an authorized fixed member")
				}
				raw, err := f.repos[0].Read("metric/gmv", values[0].Commit)
				if err != nil {
					return nil, err
				}
				hit, err := DeliverSearchHit(cx.Home, cx.Flags, retrieval.ObjectHit{Object: raw, Version: retrieval.VersionOf(raw)})
				if err != nil || hit.Object.Value == nil || cx.Flags[ResolvedPinFlag] != f.pin.PinID {
					t.Fatalf("search delivery lost member body/pin: hit=%#v flags=%#v err=%v", hit, cx.Flags, err)
				}
				if AllowedRepoRead(cx.Home, cx.Flags, "kr://consumption/outside", "metric/gmv") {
					t.Fatal("fixed-member authorization escaped its accepted membership")
				}
				if action == "file.read" {
					return namedConsumptionSearchOutput(serving.Pin(), hit), nil
				}
				return hit, nil
			})
			if result.Status != 0 || !called {
				t.Fatalf("typed fixed-member consumption rejected: called=%v result=%#v", called, result)
			}
		})
	}
}

func TestNamedMemberConsumptionRejectsIncompleteOrBorrowedScope(t *testing.T) {
	for _, scenario := range []string{"partial", "wrong-ref", "other-dataset", "temporary-label", "direct-repo", "missing-resolve", "forged-pin"} {
		for _, action := range []string{"object.read", "file.read"} {
			t.Run(scenario+"/"+action, func(t *testing.T) {
				f := newNamedConsumptionFixture(t)
				flags := f.flags()
				rules := append([]AllowRule(nil), f.rules...)
				switch scenario {
				case "partial":
					rules = rules[:2]
				case "wrong-ref":
					rules[2].Ref = "not-the-accepted-commit"
				case "other-dataset":
					if _, err := f.ws.Datasets.DefineDataset("other", 1, f.sources); err != nil {
						t.Fatal(err)
					}
					flags["dataset"] = "other"
					rules[0].Dataset = "other"
				case "temporary-label":
					flags[WorkspaceDefinitionFlag] = &dataset.Dataset{Revision: 1, Sources: f.sources}
				case "direct-repo":
					delete(flags, "dataset")
					delete(flags, "revision")
					flags["repo"], flags["ref"] = string(f.repos[0].ID()), string(f.pin.Repositories[f.repos[0].ID()])
				case "missing-resolve":
					rules = rules[1:]
				case "forged-pin":
					pin := f.pin
					pin.Repositories = map[kernel.RepositoryID]kernel.CommitID{f.repos[0].ID(): "unpublished", f.repos[1].ID(): f.pin.Repositories[f.repos[1].ID()]}
					raw, err := json.Marshal(pin)
					if err != nil {
						t.Fatal(err)
					}
					delete(flags, "revision")
					flags["pin"] = string(raw)
				}
				f.allow(t, rules)
				called := false
				result := f.invoke(action, flags, func(*Invocation) (any, error) { called = true; return nil, nil })
				if result.Status == 0 || called {
					t.Fatalf("invalid scope reached operation: called=%v result=%#v", called, result)
				}
				code := namedConsumptionError(t, result)
				if scenario != "temporary-label" && scenario != "forged-pin" && code != kernel.ErrForbidden {
					t.Fatalf("expected FORBIDDEN, got %s", code)
				}
			})
		}
	}
}

func TestNamedMemberConsumptionRechecksRevocationAtEveryLayer(t *testing.T) {
	for _, revoke := range []string{"member", "resolve"} {
		t.Run(revoke, func(t *testing.T) {
			f := newNamedConsumptionFixture(t)
			flags := f.flags()
			result := f.invoke("file.read", flags, func(cx *Invocation) (any, error) {
				serving, _, err := OpenCompleteServing(cx, "metric/gmv")
				if err != nil {
					return nil, err
				}
				// The carrier retains coordinates; it must never retain permission.
				rules := f.rules[:2]
				if revoke == "resolve" {
					rules = f.rules[1:]
				}
				f.allow(t, rules)
				if err := RequireCompleteWorkspaceRead(cx.Home, cx.Flags, serving.Pin(), "metric/gmv"); kernel.CodeOf(err) != kernel.ErrForbidden {
					t.Fatalf("integrity gate reused revoked authorization: %v", err)
				}
				if AllowedRepoAspectRead(cx.Home, cx.Flags, string(f.repos[0].ID()), "metric/gmv", "definition") {
					t.Fatal("source filter reused revoked authorization")
				}
				value := objectmodel.ObservedValue{FederatedValue: objectmodel.FederatedValue{Repository: f.repos[0].ID(), ObjectID: "metric/gmv", Value: "private"}}
				if values := FilterObjectServingReads(cx.Home, cx.Flags, f.ws.Datasets, []objectmodel.ObservedValue{value}); len(values) != 0 {
					t.Fatal("read output retained a revoked member")
				}
				raw, err := f.repos[0].Read("metric/gmv", f.pin.Repositories[f.repos[0].ID()])
				if err != nil {
					return nil, err
				}
				hit, err := DeliverSearchHit(cx.Home, cx.Flags, retrieval.ObjectHit{Object: raw})
				if err != nil || hit.Object.Value != nil {
					t.Fatalf("delivery retained revoked body: hit=%#v err=%v", hit, err)
				}
				return namedConsumptionSearchOutput(serving.Pin(), hit), nil
			})
			if result.Status != 0 {
				t.Fatalf("revocation probe failed: %#v", result)
			}
			called := false
			next := f.invoke("file.read", flags, func(*Invocation) (any, error) { called = true; return nil, nil })
			if next.Status == 0 || called || namedConsumptionError(t, next) != kernel.ErrForbidden {
				t.Fatalf("next request reused authorization: called=%v result=%#v", called, next)
			}
		})
	}
}

func TestNamedMemberConsumptionFixesOneAcceptedRevisionPerInvocation(t *testing.T) {
	f := newNamedConsumptionFixture(t)
	flags := f.flags()
	delete(flags, "revision")
	result := f.invoke("file.read", flags, func(cx *Invocation) (any, error) {
		// Authorization already chose v1; a new publication cannot change this
		// operation's basis or make its old grants authorize the new commits.
		repo := f.repos[0]
		head := f.pin.Repositories[repo.ID()]
		if _, err := repo.ApplyObjectCommit(objectmodel.ChangeSet{
			TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: head, ExpectedTargetCommit: head,
			Operations: []objectmodel.Operation{{Op: objectmodel.OpPut,
				Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "metric/gmv", AspectName: "definition"}, Value: map[string]any{"name": "new revision"},
			}},
		}); err != nil {
			return nil, err
		}
		if _, err := f.ws.Datasets.DefineDataset("analysis", 2, f.sources); err != nil {
			return nil, err
		}
		resolved, err := ResolveAtRevision(cx.WS, cx.Home, cx.WS.Datasets, "analysis", cx.Flags)
		if err != nil || resolved.Revision != 1 || resolved.PinID != f.pin.PinID {
			t.Fatalf("shared resolver changed the authorized revision: pin=%#v error=%v", resolved, err)
		}
		serving, _, err := OpenCompleteServing(cx, "metric/gmv")
		if err != nil {
			return nil, err
		}
		if serving.Pin().Revision != 1 || serving.Pin().Repositories[repo.ID()] != head || cx.Flags[ResolvedPinFlag] != f.pin.PinID {
			t.Fatalf("operation followed a publication after authorization: %#v", serving.Pin())
		}
		wrong := serving.Pin()
		wrong.Repositories[repo.ID()] = "different-operation-basis"
		if err := RequireCompleteWorkspaceRead(cx.Home, cx.Flags, wrong, "metric/gmv"); kernel.CodeOf(err) != kernel.ErrForbidden {
			t.Fatalf("integrity gate accepted a different operation pin: %v", err)
		}
		return namedConsumptionSearchOutput(serving.Pin()), nil
	})
	if result.Status != 0 {
		t.Fatalf("in-flight revision changed: %#v", result)
	}
	called := false
	next := f.invoke("file.read", flags, func(*Invocation) (any, error) { called = true; return nil, nil })
	if next.Status == 0 || called || namedConsumptionError(t, next) != kernel.ErrForbidden {
		t.Fatalf("new invocation reused the old accepted revision: called=%v result=%#v", called, next)
	}
	flags["revision"] = "1"
	old := f.invoke("file.read", flags, func(cx *Invocation) (any, error) {
		serving, _, err := OpenCompleteServing(cx, "metric/gmv")
		if err != nil {
			return nil, err
		}
		if serving.Pin().Revision != 1 {
			t.Fatalf("explicit old revision changed: %#v", serving.Pin())
		}
		return namedConsumptionSearchOutput(serving.Pin()), nil
	})
	if old.Status != 0 {
		t.Fatalf("still-authorized explicit old revision rejected: %#v", old)
	}
}
