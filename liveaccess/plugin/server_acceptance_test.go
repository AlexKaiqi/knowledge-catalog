package plugin_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"testing"

	kcclient "kc/client"
	"kc/kernel"
	"kc/liveaccess"
	accessclient "kc/liveaccess/client"
	objectmodel "kc/object"
	"kc/server"
	"kc/snapshot"
	snapshotclient "kc/snapshot/client"
	"kc/snapshot/dataset"
)

func assertAcceptanceMixed(t *testing.T, row objectmodel.ObservedValue, value int64, failure kernel.ErrorCode) {
	t.Helper()
	body, ok := row.Value.(map[string]any)
	if !ok {
		t.Fatalf("public object READ did not return object body: %#v", row)
	}
	if !reflect.DeepEqual(body["definition"], map[string]any{"owner": "fixed"}) || fmt.Sprint(body["good"].(map[string]any)["value"]) != fmt.Sprint(value) || body["empty"] != nil || fmt.Sprint(body["readings"].(map[string]any)["good"].(map[string]any)["value"]) != fmt.Sprint(value) {
		t.Fatalf("fixed/actual/null/Member values lost: %#v", row)
	}
	if len(row.Failures) != 1 || row.Failures[0].Address.AspectName != "broken" || row.Failures[0].Code != failure || row.Failures[0].Message == "" || len(row.Observations) != 3 {
		t.Fatalf("local failure or successful observation lost: %#v", row)
	}
	for _, observation := range row.Observations {
		if observation.Address.AspectName == "broken" || observation.DeclarationCommit != row.Commit || observation.Basis.SourceRevision != fmt.Sprintf("source-%d", value) {
			t.Fatalf("success acquired another unit or declaration/source version: %#v", row)
		}
	}
}

func assertAcceptanceUnavailable(t *testing.T, row objectmodel.ObservedValue) {
	t.Helper()
	if len(row.Failures) != 4 || len(row.Observations) != 0 || !reflect.DeepEqual(row.Value.(map[string]any)["definition"], map[string]any{"owner": "fixed"}) {
		t.Fatalf("unavailable contribution erased fixed siblings or invented observations: %#v", row)
	}
	positions := map[string]bool{}
	for _, failure := range row.Failures {
		key := failure.Address.AspectName + "/" + failure.Address.MemberKey
		if failure.Code != kernel.ErrCapabilityUnsatisfied || failure.Message == "" || positions[key] {
			t.Fatalf("unavailable selected contribution did not explain each position: %#v", row)
		}
		positions[key] = true
	}
	if !reflect.DeepEqual(positions, map[string]bool{"good/": true, "empty/": true, "broken/": true, "readings/good": true}) {
		t.Fatalf("unavailable contribution changed affected positions: %#v", row)
	}
}

// U14/U15: genuine Repo-selected HTTP instances cross the public SDK/Server;
// every result is adopted, with independent source/schema/basis failure cases.
func TestServerLiveAccessRepoSelectionMixedValuesAndDataset(t *testing.T) {
	one, two, replacement, fallback := newAcceptanceRuntime(t, 31), newAcceptanceRuntime(t, 22), newAcceptanceRuntime(t, 33), newAcceptanceRuntime(t, 88)
	incompatible := newAcceptanceRuntime(t, 99)
	incompatible.protocol = "source-access/v9"
	fallbackURL, _ := url.Parse(fallback.server.URL)
	f := newAcceptanceFixture(t, server.HTTPServerOptions{LiveAccessPlugins: acceptanceEntries(t, map[string]*acceptanceRuntime{"one": one, "two": two, "replacement": replacement, "incompatible": incompatible}), StateLookup: liveaccess.NewHTTPStateLookup(&http.Client{Transport: acceptanceTransport{target: fallbackURL}})})
	for _, repo := range []kernel.RepositoryID{acceptanceR1, acceptanceR2} {
		f.publish(t, repo, "http://logical-source.invalid")
	}
	for _, test := range []struct {
		mode string
		code kernel.ErrorCode
	}{{"source", kernel.ErrTemporaryUnavailable}, {"basis", kernel.ErrCapabilityUnsatisfied}, {"schema", kernel.ErrSchemaInstanceInvalid}} {
		t.Run(test.mode, func(t *testing.T) {
			one.mode.Store(test.mode)
			row, err := f.read(t, "reader", "mixed-"+test.mode, kcclient.KnowledgeReadRequest{Reference: acceptanceReference(t, acceptanceR1, ""), Commit: string(f.commits[acceptanceR1])})
			if err != nil {
				t.Fatalf("one value failed the entire public object read: %#v %v", row, err)
			}
			assertAcceptanceMixed(t, *row, 31, test.code)
		})
	}
	one.mode.Store("source")
	var published any
	err := f.client(t, "operator").SnapshotService().DefineKnowledgeSet(context.Background(), snapshotclient.KnowledgeSetRequest{Dataset: "both", Revision: 1, Sources: []dataset.KnowledgeSetSource{{Repository: acceptanceR1, Selector: snapshot.DefaultRef}, {Repository: acceptanceR2, Selector: snapshot.DefaultRef}}}, snapshotclient.RequestOptions{}, &published)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		mode string
		code kernel.ErrorCode
	}{{"source", kernel.ErrTemporaryUnavailable}, {"basis", kernel.ErrCapabilityUnsatisfied}, {"schema", kernel.ErrSchemaInstanceInvalid}} {
		one.mode.Store(test.mode)
		for i, repo := range []kernel.RepositoryID{acceptanceR1, acceptanceR2} {
			request := kcclient.KnowledgeReadRequest{KnowledgeDatasetIdentity: kcclient.KnowledgeDatasetIdentity{Dataset: "both"}, Reference: acceptanceReference(t, repo, "")}
			row, err := f.read(t, "reader", fmt.Sprintf("mixed-dataset-%s-%d", test.mode, i+1), request)
			if err != nil || row.Repository != repo || row.Commit != f.commits[repo] {
				t.Fatalf("combination Dataset changed explicit source or fixed version: source=%s row=%#v %v", repo, row, err)
			}
			want, code := int64(31), test.code
			if row.Repository == acceptanceR2 {
				want, code = 22, kernel.ErrTemporaryUnavailable
			} else if row.Repository != acceptanceR1 {
				t.Fatalf("Dataset invented a source: %#v", row)
			}
			assertAcceptanceMixed(t, *row, want, code)
		}
	}
	one.mode.Store("source")
	request := accessclient.BindingRequest{KnowledgeScope: kcclient.KnowledgeScope{Repository: string(acceptanceR1), Commit: string(f.commits[acceptanceR1])}, Object: string(acceptanceObject), Aspect: "good"}
	readRequest := kcclient.KnowledgeReadRequest{Reference: acceptanceReference(t, acceptanceR1, ""), Commit: string(f.commits[acceptanceR1])}
	var before, after liveaccess.ResolvedBinding
	if err := f.client(t, "reader").LiveAccessService().ResolveBinding(context.Background(), request, accessclient.RequestOptions{}, &before); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("read permission silently included binding resolve: %v", err)
	}
	if err := f.client(t, "operator").LiveAccessService().ResolveBinding(context.Background(), request, accessclient.RequestOptions{}, &before); err != nil {
		t.Fatal(err)
	}
	oldCalls, oldAttempts := one.calls.Load(), one.attempts.Load()
	f.selectInstance(t, acceptanceR1, "liveaccess", "replacement")
	f.stop(t)
	f.start(t)
	if err := f.client(t, "operator").LiveAccessService().ResolveBinding(context.Background(), request, accessclient.RequestOptions{}, &after); err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("instance replacement changed fixed declaration: before=%#v after=%#v %v", before, after, err)
	}
	row, err := f.read(t, "reader", "replacement-instance", readRequest)
	if err != nil {
		t.Fatal(err)
	}
	assertAcceptanceMixed(t, *row, 33, kernel.ErrTemporaryUnavailable)
	if one.calls.Load() != oldCalls || one.attempts.Load() != oldAttempts || replacement.calls.Load() != 4 {
		t.Fatal("replacement did not exclusively select the new runtime for each value")
	}
	if testkitHead := f.commits[acceptanceR1]; row.Commit != testkitHead {
		t.Fatal("instance replacement changed declaration commit")
	}
	// Actual Entry.Open failures cover a missing instance and an installed
	// runtime whose protocol cannot provide this entry's required contribution.
	for _, selection := range []struct{ name, instance string }{{"liveaccess", "missing"}, {"unregistered-liveaccess", "missing"}, {"liveaccess", "incompatible"}} {
		f.selectInstance(t, acceptanceR1, selection.name, selection.instance)
		f.stop(t)
		f.start(t)
		row, err = f.read(t, "reader", "unavailable-"+selection.name+"-"+selection.instance, readRequest)
		if err != nil {
			t.Fatalf("missing plugin erased static siblings or invented observations: %#v %v", row, err)
		}
		assertAcceptanceUnavailable(t, *row)
		row, err = f.read(t, "reader", "independent-"+selection.name+"-"+selection.instance, kcclient.KnowledgeReadRequest{Reference: acceptanceReference(t, acceptanceR2, ""), Commit: string(f.commits[acceptanceR2])})
		if err != nil {
			t.Fatalf("missing R1 plugin blocked R2: %#v %v", row, err)
		}
		assertAcceptanceMixed(t, *row, 22, kernel.ErrTemporaryUnavailable)
	}
	if fallback.calls.Load() != 0 || incompatible.calls.Load() != 0 || incompatible.attempts.Load() != 0 {
		t.Fatal("unavailable contribution contacted its source or fell back to global adapter")
	}
}
