package opensearchimpl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"kc/kernel"
)

func TestResourceAuthorityRejectsUnknownOrMalformedPublicationProtection(t *testing.T) {
	id := strings.Repeat("a", 32)
	until := time.Now().Add(time.Minute).UnixMilli()
	for _, test := range []struct {
		name    string
		version int
		claims  []resourceClaim
	}{
		{"unknown-version", resourceRecordVersion + 1, nil},
		{"legacy-with-claims", 1, []resourceClaim{{ID: id, Phase: claimPrepared, Until: until}}},
		{"invalid-id", 2, []resourceClaim{{ID: "not-a-ticket", Phase: claimPrepared, Until: until}}},
		{"duplicate-id", 2, []resourceClaim{{ID: id, Phase: claimPrepared, Until: until}, {ID: id, Phase: claimPrepared, Until: until}}},
		{"query-in-old-version", 2, []resourceClaim{{ID: id, Phase: claimQuery, Until: until, Evidence: []byte("publication")}}},
		{"continuation-in-old-version", 3, []resourceClaim{{ID: id, Phase: claimContinuation, Until: until, Evidence: []byte("lease")}}},
		{"unknown-phase", 2, []resourceClaim{{ID: id, Phase: "UNKNOWN", Until: until}}},
		{"missing-admission-evidence", 2, []resourceClaim{{ID: id, Phase: claimAdmitted, Until: until}}},
		{"unexpected-preparation-evidence", 2, []resourceClaim{{ID: id, Phase: claimPrepared, Until: until, Evidence: []byte("unresolved")}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var records resourceControlFixture
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !records.serve(w, r) {
					t.Errorf("unexpected physical IO: %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected physical IO", 500)
				}
			}))
			defer server.Close()
			e := &openSearchEngine{base: server.URL, http: server.Client(), repository: "repo", prefix: "kc-test", controlID: "test"}
			session := &rebuildSession{engine: e, physicalIndex: "kc-test-g-" + id, generation: id}
			record := controlDoc{State: "RESOURCE", Repository: "repo", ActiveIndex: session.physicalIndex, Generation: id,
				Resource: &generationResource{Version: test.version, Phase: resourceAdopted, LeaseUntil: until, Claims: test.claims}}
			if _, err := e.putControlRecord(session.resourceID(), record, nil); err != nil {
				t.Fatal(err)
			}
			called := false
			if err := session.changeResource(func(*generationResource) error { called = true; return nil }); kernel.CodeOf(err) != kernel.ErrPreconditionFailed || called {
				t.Fatalf("unknown protection became mutable: %v called=%t", err, called)
			}
			after, _, err := e.loadControlRecord(context.Background(), session.resourceID())
			if err != nil || !reflect.DeepEqual(after, record) {
				t.Fatalf("rejected authority was changed: %#v %v", after, err)
			}
		})
	}
}

func TestResourceAuthorityRejectsUnknownQueryCoverageAndHistoricalProof(t *testing.T) {
	id, other := strings.Repeat("a", 32), strings.Repeat("b", 32)
	for _, test := range []struct {
		name     string
		resource generationResource
	}{
		{"query-protocol-in-legacy", generationResource{Version: 4, Phase: resourceAdopted, QueryLeases: 1}},
		{"unknown-query-protocol", generationResource{Version: resourceRecordVersion, Phase: resourceAdopted, QueryLeases: 2}},
		{"invalid-deletion-proof", generationResource{Version: resourceRecordVersion, Phase: resourceDeleting, QueryLeases: 1, SupersededBy: "not-a-generation"}},
		{"premature-deletion-proof", generationResource{Version: resourceRecordVersion, Phase: resourceAdopted, QueryLeases: 1, SupersededBy: other}},
		{"self-deletion-proof", generationResource{Version: resourceRecordVersion, Phase: resourceDeleting, QueryLeases: 1, SupersededBy: id}},
		{"untracked-root-query", generationResource{Version: resourceRecordVersion, Phase: resourceAdopted, Claims: []resourceClaim{{ID: other, Phase: claimRootQuery, Until: time.Now().Add(time.Minute).UnixMilli(), Evidence: []byte("untracked")}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var records resourceControlFixture
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !records.serve(w, r) {
					t.Errorf("unexpected IO: %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected", 500)
				}
			}))
			defer server.Close()
			e := &openSearchEngine{base: server.URL, http: server.Client(), repository: "repo", prefix: "kc-test", controlID: "test"}
			session := &rebuildSession{engine: e, generation: id, physicalIndex: "kc-test-g-" + id}
			test.resource.LeaseUntil = time.Now().Add(time.Minute).UnixMilli()
			record := controlDoc{State: "RESOURCE", Repository: "repo", Generation: id, ActiveIndex: session.physicalIndex, Resource: &test.resource}
			if _, err := e.putControlRecord(session.resourceID(), record, nil); err != nil {
				t.Fatal(err)
			}
			called := false
			if err := session.changeResource(func(*generationResource) error { called = true; return nil }); kernel.CodeOf(err) != kernel.ErrPreconditionFailed || called {
				t.Fatalf("invalid protocol became mutable: called=%v %v", called, err)
			}
			after, _, err := e.loadControlRecord(context.Background(), session.resourceID())
			if err != nil || !reflect.DeepEqual(after, record) {
				t.Fatalf("invalid protocol was repaired: %#v %v", after, err)
			}
		})
	}
}
