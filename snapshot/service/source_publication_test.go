package service_test

import (
	"context"
	"net/http"
	"testing"

	"kc/identity"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
	"kc/snapshot/service"
)

func sourcePublicationGrant(f *datasetHTTPFixture) access.Rule {
	return access.Rule{ID: "source-publication", Principal: "alice", Actions: []string{access.RepositoryDatasetPublish}, Repo: string(f.source.ID()), Dataset: "files", Ref: snapshot.DefaultRef}
}

func TestDatasetPublicationRequiresSourcePermissionBeforeInspection(t *testing.T) {
	for _, scope := range []string{"manage-and-file-read", "dataset-wildcard", "dataset-only-publish", "other-repository", "other-destination", "other-ref", "fixed-commit-with-main-label"} {
		t.Run(scope, func(t *testing.T) {
			f := newDatasetHTTPFixture(t)
			f.rules.rules = []access.Rule{
				{ID: "target-management", Principal: "alice", Dataset: "files", Actions: []string{"dataset.manage"}},
				{ID: "source-read", Principal: "alice", Repo: string(f.source.ID()), Actions: []string{"file.read"}},
			}
			grant := sourcePublicationGrant(f)
			body := datasetHTTPDefinition(1, f.source.ID())
			switch scope {
			case "dataset-wildcard":
				f.rules.rules[0].Actions = []string{"dataset.*"}
			case "dataset-only-publish":
				grant.Repo = ""
			case "other-repository":
				grant.Repo = "kr://files/other"
			case "other-destination":
				grant.Dataset = "other"
			case "other-ref":
				grant.Ref = "refs/heads/private"
			case "fixed-commit-with-main-label":
				sources := body["sources"].([]dataset.DatasetSource)
				sources[0].Commit = "c2"
			}
			if scope != "manage-and-file-read" && scope != "dataset-wildcard" {
				f.rules.rules = append(f.rules.rules, grant)
			}
			portable, ready := 0, 0
			f.resources.BeforePublish = func(_ context.Context, _ identity.HTTPIdentity, _ dataset.Service, definition dataset.Dataset) (dataset.Dataset, map[string]any, error) {
				portable++
				return definition, nil, nil
			}
			f.resources.PreparePublication = func(context.Context, identity.HTTPIdentity, dataset.Dataset) error { ready++; return nil }
			before := dataset.NormalizeState(f.catalog.DumpState())
			head := testkit.MustHead(t, f.authority, snapshot.DefaultRef)
			f.source.inspections = 0
			failure := datasetHTTPCall(t, f.handler(), http.MethodPost, "/snapshot/v1/datasets", body, http.StatusForbidden)
			if failure["error"].(map[string]any)["code"] != string(kernel.ErrForbidden) {
				t.Fatalf("source refusal lost its protocol error: %+v", failure)
			}
			assertDatasetHTTPState(t, f, before)
			if f.source.inspections != 0 || portable != 0 || ready != 0 || testkit.MustHead(t, f.authority, snapshot.DefaultRef) != head {
				t.Fatalf("unlicensed source reached inspection/preparation/publication: inspections=%d portable=%d ready=%d", f.source.inspections, portable, ready)
			}
		})
	}
}

func TestDatasetPublicationExplicitSourceGrantAcceptsSelectedRefOrCommit(t *testing.T) {
	for _, fixed := range []bool{false, true} {
		name := "selector"
		if fixed {
			name = "commit"
		}
		t.Run(name, func(t *testing.T) {
			f := newDatasetHTTPFixture(t)
			grant := sourcePublicationGrant(f)
			body := datasetHTTPDefinition(1, f.source.ID())
			wantCommit := "c1"
			if fixed {
				grant.Ref, wantCommit = "c2", "c2"
				body["sources"].([]dataset.DatasetSource)[0].Commit = "c2"
			}
			f.rules.rules = []access.Rule{{ID: "target-management", Principal: "alice", Dataset: "files", Actions: []string{"dataset.manage"}}, grant}
			response := datasetHTTPCall(t, f.handler(), http.MethodPost, "/snapshot/v1/datasets", body, http.StatusOK)
			accepted, err := f.catalog.DatasetVersion("files", 1)
			if err != nil || response["setId"] != "files" || len(accepted.Sources) != 1 || accepted.Sources[0].Repository != f.source.ID() || string(accepted.Sources[0].Commit) != wantCommit {
				t.Fatalf("source permit did not accept its selected coordinate: %+v, %v", accepted, err)
			}
			// Publication source permission does not become a consumption requirement.
			f.rules.rules = []access.Rule{{ID: "consumer", Principal: "alice", Dataset: "files", Actions: []string{"dataset.resolve"}}}
			pin := datasetHTTPPin(t, datasetHTTPCall(t, f.handler(), http.MethodPost, "/snapshot/v1/datasets/files/resolve", map[string]any{}, http.StatusOK))
			if string(pin.Repositories[f.source.ID()]) != wantCommit {
				t.Fatalf("Dataset consumer needed a source publication grant: %+v", pin)
			}
		})
	}
}

func TestDatasetPublicationRechecksSourcePermissionAfterPreparation(t *testing.T) {
	for _, phase := range []string{"pre-hook", "before-publish", "portable-commit-change", "final-preparation", "prepared-source-replacement"} {
		t.Run(phase, func(t *testing.T) {
			f := newDatasetHTTPFixture(t)
			f.rules.rules = []access.Rule{{ID: "target-management", Principal: "alice", Dataset: "files", Actions: []string{"dataset.manage"}}, sourcePublicationGrant(f)}
			if _, err := f.catalog.DefineDataset("files", 1, datasetHTTPDefinition(1, f.source.ID())["sources"].([]dataset.DatasetSource)); err != nil {
				t.Fatal(err)
			}
			before := dataset.NormalizeState(f.catalog.DumpState())
			head := testkit.MustHead(t, f.authority, snapshot.DefaultRef)
			inspectionsAtRevocation := -1
			handler := f.handler()
			if phase == "pre-hook" {
				handler = service.NewHandler(service.Config{Identity: service.IdentityOptions{Local: true},
					Acquire: func(context.Context, identity.HTTPIdentity, string) (service.Resources, func(), error) {
						return f.resources, func() {}, nil
					},
					Around: func(_ context.Context, _ identity.HTTPIdentity, _ string, run func() (any, error)) (any, error) {
						f.rules.rules = f.rules.rules[:1]
						inspectionsAtRevocation = f.source.inspections
						return run()
					},
				})
			} else if phase == "before-publish" || phase == "portable-commit-change" {
				f.resources.BeforePublish = func(_ context.Context, _ identity.HTTPIdentity, _ dataset.Service, definition dataset.Dataset) (dataset.Dataset, map[string]any, error) {
					if phase == "portable-commit-change" {
						definition.Sources[0].Commit = "c2"
					} else {
						f.rules.rules = f.rules.rules[:1]
					}
					inspectionsAtRevocation = f.source.inspections
					return definition, nil, nil
				}
			} else {
				f.resources.PreparePublication = func(_ context.Context, _ identity.HTTPIdentity, definition dataset.Dataset) error {
					if phase == "prepared-source-replacement" {
						definition.Sources[0].Commit = "c2"
					} else {
						f.rules.rules = f.rules.rules[:1]
					}
					inspectionsAtRevocation = f.source.inspections
					return nil
				}
			}
			status, code := http.StatusForbidden, kernel.ErrForbidden
			if phase == "prepared-source-replacement" {
				status, code = http.StatusConflict, kernel.ErrPreconditionFailed
			}
			failure := datasetHTTPCall(t, handler, http.MethodPost, "/snapshot/v1/datasets", datasetHTTPDefinition(2, f.source.ID()), status)
			if failure["error"].(map[string]any)["code"] != string(code) {
				t.Fatalf("prepared source failure changed: %+v", failure)
			}
			assertDatasetHTTPState(t, f, before)
			if inspectionsAtRevocation < 0 || f.source.inspections != inspectionsAtRevocation || testkit.MustHead(t, f.authority, snapshot.DefaultRef) != head {
				t.Fatal("revoked/replaced source continued inspecting or changed registry HEAD")
			}
			current, err := f.catalog.Set("files")
			if err != nil || current.Revision != 1 {
				t.Fatalf("failed source permission changed current: %+v, %v", current, err)
			}
			if _, err := f.catalog.DatasetVersion("files", 2); kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
				t.Fatalf("failed source permission entered history: %v", err)
			}
		})
	}
}

func TestDatasetPublicationRequiresEverySourceBeforeAnyInspection(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	second := &datasetHTTPSource{id: "kr://files/other", head: "c2", files: map[kernel.CommitID]map[string][]byte{"c2": {"other.txt": []byte("second source")}}}
	if err := f.resources.Sources.(snapshot.Registry).Add(second); err != nil {
		t.Fatal(err)
	}
	if err := f.catalog.RegisterRepository(second.ID()); err != nil {
		t.Fatal(err)
	}
	body := datasetHTTPDefinition(1, f.source.ID())
	body["sources"] = append(body["sources"].([]dataset.DatasetSource), dataset.DatasetSource{Repository: second.ID(), Selector: snapshot.DefaultRef, File: "other.txt", Target: "docs/other.txt"})
	f.rules.rules = []access.Rule{{ID: "target-management", Principal: "alice", Dataset: "files", Actions: []string{"dataset.manage"}}, sourcePublicationGrant(f)}
	before := dataset.NormalizeState(f.catalog.DumpState())
	head := testkit.MustHead(t, f.authority, snapshot.DefaultRef)
	f.source.inspections, second.inspections = 0, 0
	datasetHTTPCall(t, f.handler(), http.MethodPost, "/snapshot/v1/datasets", body, http.StatusForbidden)
	assertDatasetHTTPState(t, f, before)
	if f.source.inspections != 0 || second.inspections != 0 || testkit.MustHead(t, f.authority, snapshot.DefaultRef) != head {
		t.Fatal("one source grant exposed or accepted another source")
	}
	f.rules.rules = append(f.rules.rules, access.Rule{ID: "second-publication", Principal: "alice", Repo: string(second.ID()), Dataset: "files", Ref: snapshot.DefaultRef, Actions: []string{access.RepositoryDatasetPublish}})
	datasetHTTPCall(t, f.handler(), http.MethodPost, "/snapshot/v1/datasets", body, http.StatusOK)
	accepted, err := f.catalog.DatasetVersion("files", 1)
	if err != nil || len(accepted.Sources) != 2 || accepted.Sources[0].Commit != "c1" || accepted.Sources[1].Commit != "c2" || len(accepted.Items) != 2 {
		t.Fatalf("complete source authority did not preserve version vector: %+v, %v", accepted, err)
	}
}
