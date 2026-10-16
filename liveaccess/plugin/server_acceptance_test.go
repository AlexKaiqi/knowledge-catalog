package plugin_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
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
			row, err := f.read(t, "reader", "mixed-"+test.mode, kcclient.ObjectReadRequest{Reference: acceptanceReference(t, acceptanceR1, ""), Repository: string(acceptanceR1), Commit: string(f.commits[acceptanceR1])})
			if err != nil {
				t.Fatalf("one value failed the entire public object read: %#v %v", row, err)
			}
			assertAcceptanceMixed(t, *row, 31, test.code)
		})
	}
	one.mode.Store("source")
	var published any
	define := func(name string, repos ...kernel.RepositoryID) {
		t.Helper()
		sources := []dataset.DatasetSource{}
		for _, repo := range repos {
			sources = append(sources, dataset.DatasetSource{Repository: repo, Selector: snapshot.DefaultRef})
		}
		if err := f.client(t, "operator").SnapshotService().DefineDataset(context.Background(), snapshotclient.DatasetRequest{Dataset: name, Revision: 1, Sources: sources}, snapshotclient.RequestOptions{}, &published); err != nil {
			t.Fatal(err)
		}
	}
	define("first", acceptanceR1)
	define("second", acceptanceR2)
	// G3: the same Aspect of one object ID held by two Repositories is a
	// conflict reported at Dataset publication, never a silent pick. Fixture
	// Schemas use distinct IDs so this is the only conflicting object.
	sources := []dataset.DatasetSource{{Repository: acceptanceR1, Selector: snapshot.DefaultRef}, {Repository: acceptanceR2, Selector: snapshot.DefaultRef}}
	err := f.client(t, "operator").SnapshotService().DefineDataset(context.Background(), snapshotclient.DatasetRequest{Dataset: "both", Revision: 1, Sources: sources}, snapshotclient.RequestOptions{}, &published)
	if kernel.CodeOf(err) != kernel.ErrObjectIDConflict || !strings.Contains(err.Error(), string(acceptanceObject)) || !strings.Contains(err.Error(), string(acceptanceR1)) || !strings.Contains(err.Error(), string(acceptanceR2)) {
		t.Fatalf("publication with the same Aspect from two Repositories must conflict and name object and both sources: %v", err)
	}
	// Without a Dataset the reader's readable Repositories are the scope; the
	// same conflict is reported on read.
	if _, err := f.read(t, "reader", "mixed-unscoped-conflict", kcclient.ObjectReadRequest{Reference: acceptanceReference(t, acceptanceR1, "")}); kernel.CodeOf(err) != kernel.ErrObjectIDConflict || !strings.Contains(err.Error(), string(acceptanceR1)) || !strings.Contains(err.Error(), string(acceptanceR2)) {
		t.Fatalf("unscoped read with the same Aspect from two Repositories must conflict and name both: %v", err)
	}
	for _, test := range []struct {
		mode string
		code kernel.ErrorCode
	}{{"source", kernel.ErrTemporaryUnavailable}, {"basis", kernel.ErrCapabilityUnsatisfied}, {"schema", kernel.ErrSchemaInstanceInvalid}} {
		one.mode.Store(test.mode)
		for i, scope := range []struct {
			set  string
			repo kernel.RepositoryID
			want int64
			code kernel.ErrorCode
		}{{"first", acceptanceR1, 31, test.code}, {"second", acceptanceR2, 22, kernel.ErrTemporaryUnavailable}} {
			request := kcclient.ObjectReadRequest{ObjectDatasetIdentity: kcclient.ObjectDatasetIdentity{Dataset: scope.set}, Reference: acceptanceReference(t, scope.repo, "")}
			row, err := f.read(t, "reader", fmt.Sprintf("mixed-dataset-%s-%d", test.mode, i+1), request)
			if err != nil || row.Repository != scope.repo || row.Commit != f.commits[scope.repo] {
				t.Fatalf("Dataset changed its only holder or fixed version: source=%s row=%#v %v", scope.repo, row, err)
			}
			assertAcceptanceMixed(t, *row, scope.want, scope.code)
		}
	}
	one.mode.Store("source")
	request := accessclient.BindingRequest{ObjectScope: kcclient.ObjectScope{Repository: string(acceptanceR1), Commit: string(f.commits[acceptanceR1])}, Object: string(acceptanceObject), Aspect: "good"}
	readRequest := kcclient.ObjectReadRequest{Reference: acceptanceReference(t, acceptanceR1, ""), Repository: string(acceptanceR1), Commit: string(f.commits[acceptanceR1])}
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
	// runtime whose protocol cannot provide this entry's required contribution;
	// an unmapped runtime has no instance at all. Each stays local to R1.
	for _, selection := range []struct{ name, instance string }{{"liveaccess", "missing"}, {"liveaccess", ""}, {"liveaccess", "incompatible"}} {
		f.selectInstance(t, acceptanceR1, selection.name, selection.instance)
		f.stop(t)
		f.start(t)
		label := selection.instance
		if label == "" {
			label = "unmapped"
		}
		row, err = f.read(t, "reader", "unavailable-"+label, readRequest)
		if err != nil {
			t.Fatalf("missing plugin erased static siblings or invented observations: %#v %v", row, err)
		}
		assertAcceptanceUnavailable(t, *row)
		row, err = f.read(t, "reader", "independent-"+label, kcclient.ObjectReadRequest{Reference: acceptanceReference(t, acceptanceR2, ""), Repository: string(acceptanceR2), Commit: string(f.commits[acceptanceR2])})
		if err != nil {
			t.Fatalf("missing R1 plugin blocked R2: %#v %v", row, err)
		}
		assertAcceptanceMixed(t, *row, 22, kernel.ErrTemporaryUnavailable)
	}
	if fallback.calls.Load() != 0 || incompatible.calls.Load() != 0 || incompatible.attempts.Load() != 0 {
		t.Fatal("unavailable contribution contacted its source or fell back to global adapter")
	}
	// An instance of an unknown plugin is a deployment error, not a missing value.
	f.stop(t)
	f.selectInstance(t, acceptanceR1, "unregistered-liveaccess", "missing")
	if _, err := server.HTTPHandlerFromConfig(f.path, f.options); kernel.CodeOf(err) != kernel.ErrPreconditionFailed || !strings.Contains(err.Error(), "unregistered-liveaccess") {
		t.Fatalf("deployment started with an unknown plugin instance: %v", err)
	}
	f.selectInstance(t, acceptanceR1, "liveaccess", "replacement")
	f.start(t)
}
