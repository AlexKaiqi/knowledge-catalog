package service_test

import (
	"context"
	"net/http"
	"testing"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
)

func TestDatasetHTTPPublicationReplayKeepsAcceptedCommit(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	h := f.handler()
	const endpoint = "/snapshot/v1/datasets"
	datasetHTTPCall(t, h, http.MethodPost, endpoint, datasetHTTPDefinition(1, f.source.ID()), http.StatusOK)
	before := dataset.NormalizeState(f.catalog.DumpState())
	head, err := f.authority.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	f.source.head = "c2"
	f.source.inspections = 0
	for _, commit := range []string{"c1", "c2"} {
		f.rules.rules = append(f.rules.rules, access.Rule{ID: "publish-" + commit, Principal: "alice", Repo: string(f.source.ID()), Ref: commit, Actions: []string{access.RepositoryDatasetPublish}})
	}
	preparations := 0
	f.resources.BeforePublish = func(_ context.Context, _ identity.HTTPIdentity, _ dataset.Service, def dataset.Dataset) (dataset.Dataset, map[string]any, error) {
		preparations++
		return def, nil, nil
	}
	f.resources.PreparePublication = func(_ context.Context, _ identity.HTTPIdentity, def dataset.Dataset) (dataset.Dataset, error) {
		preparations++
		return def, nil
	}
	for _, test := range []struct {
		name   string
		commit kernel.CommitID
		status int
	}{
		{"branch after HEAD advanced", "", http.StatusOK},
		{"accepted explicit commit", "c1", http.StatusOK},
		{"different explicit commit", "c2", http.StatusConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := datasetHTTPDefinition(1, f.source.ID())
			sources := request["sources"].([]dataset.DatasetSource)
			sources[0].Commit = test.commit
			out := datasetHTTPCall(t, h, http.MethodPost, endpoint, request, test.status)
			if test.status == http.StatusOK {
				if out["disposition"] != "REPLAYED" || out["revision"] != float64(1) || out["sources"].([]any)[0].(map[string]any)["commit"] != "c1" {
					t.Fatalf("retry replaced the accepted commit: %#v", out)
				}
			} else if out["error"].(map[string]any)["code"] != string(kernel.ErrNonFastForward) {
				t.Fatalf("changed definition lost the monotonic revision error: %#v", out)
			}
			assertDatasetHTTPState(t, f, before)
		})
	}
	if f.source.inspections != 0 || preparations != 0 {
		t.Fatalf("same-revision retries inspected or prepared a source: inspections=%d preparations=%d", f.source.inspections, preparations)
	}
	if after, err := f.authority.Head(snapshot.DefaultRef); err != nil || after != head {
		t.Fatalf("same-revision retries changed registry HEAD: before=%s after=%s error=%v", head, after, err)
	}
}
