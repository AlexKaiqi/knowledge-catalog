package snapshot_test

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
)

type repositoryMetadataPort interface {
	ReadRepositoryMetadata() (map[string]string, error)
	WriteRepositoryMetadata(map[string]string) error
}

func openRepositoryMetadataPort(t *testing.T, fake *testkit.LakeFSFake, name string) (snapshot.Store, repositoryMetadataPort) {
	t.Helper()
	store, err := snapshot.OpenLakeFS("kr://conformance/repository-metadata", fake.DSN(name), fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	port, ok := store.(repositoryMetadataPort)
	if !ok {
		t.Fatal("lakeFS public factory omitted optional repository metadata capability")
	}
	return store, port
}

func TestLakeFSRepositoryMetadataRemainsNativeAcrossReopen(t *testing.T) {
	requests := 0
	fake := testkit.NewLakeFSFakeWithInterceptor(t, func(w http.ResponseWriter, r *http.Request) bool {
		if !strings.HasSuffix(r.URL.Path, "/metadata") {
			return false
		}
		requests++
		user, secret, ok := r.BasicAuth()
		if !ok || user != "access" || secret != "secret" {
			t.Error("metadata request did not use the configured repository credential")
			w.WriteHeader(http.StatusUnauthorized)
			return true
		}
		return false
	})
	name := fake.NewRepo()
	store, port := openRepositoryMetadataPort(t, fake, name)
	head := testkit.MustHead(t, store, snapshot.DefaultRef)
	empty, err := port.ReadRepositoryMetadata()
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("new repository metadata must be an empty native map: metadata=%v error=%v", empty, err)
	}
	if err := port.WriteRepositoryMetadata(map[string]string{"description": "original", "retained": "yes"}); err != nil {
		t.Fatal(err)
	}
	if err := port.WriteRepositoryMetadata(map[string]string{"description": "changed", "opaque": `{"version":1}`, "empty": ""}); err != nil {
		t.Fatal(err)
	}
	if err := port.WriteRepositoryMetadata(nil); err != nil {
		t.Fatal(err)
	}
	reopened, read := openRepositoryMetadataPort(t, fake, name)
	want := map[string]string{"description": "changed", "retained": "yes", "opaque": `{"version":1}`, "empty": ""}
	got, err := read.ReadRepositoryMetadata()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("metadata was cached, replaced or interpreted instead of reread: metadata=%v error=%v", got, err)
	}
	got["retained"] = "caller changed"
	again, err := port.ReadRepositoryMetadata()
	if err != nil || !reflect.DeepEqual(again, want) || requests != 6 {
		t.Fatalf("caller mutation reached native metadata or requests were added: metadata=%v requests=%d error=%v", again, requests, err)
	}
	if testkit.MustHead(t, reopened, snapshot.DefaultRef) != head {
		t.Fatal("repository metadata update created a Snapshot commit")
	}
}

func TestLakeFSRepositoryMetadataFailuresDoNotBecomeEmptyConfiguration(t *testing.T) {
	for _, row := range []struct {
		name string
		code int
		body string
		want kernel.ErrorCode
	}{
		{"unauthenticated", 401, `{"message":"credential rejected"}`, kernel.ErrUnauthenticated},
		{"forbidden", 403, `{"message":"metadata denied"}`, kernel.ErrForbidden},
		{"unavailable", 503, `{"message":"metadata unavailable"}`, kernel.ErrTemporaryUnavailable},
		{"redirect", 302, `{"setting":"redirect response"}`, kernel.ErrTemporaryUnavailable},
		{"malformed", 200, `{"setting":false}`, kernel.ErrTemporaryUnavailable},
		{"null-map", 200, `null`, kernel.ErrTemporaryUnavailable},
		{"null-value", 200, `{"setting":null}`, kernel.ErrTemporaryUnavailable},
	} {
		t.Run(row.name, func(t *testing.T) {
			fail := false
			calls := 0
			fake := testkit.NewLakeFSFakeWithInterceptor(t, func(w http.ResponseWriter, r *http.Request) bool {
				if !fail || !strings.HasSuffix(r.URL.Path, "/metadata") {
					return false
				}
				calls++
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(row.code)
				_, _ = w.Write([]byte(row.body))
				return true
			})
			store, port := openRepositoryMetadataPort(t, fake, fake.NewRepo())
			head := testkit.MustHead(t, store, snapshot.DefaultRef)
			if err := port.WriteRepositoryMetadata(map[string]string{"setting": "original"}); err != nil {
				t.Fatal(err)
			}
			fail = true
			metadata, err := port.ReadRepositoryMetadata()
			if kernel.CodeOf(err) != row.want || metadata != nil || calls != 1 {
				t.Fatalf("failed metadata read masqueraded as absence: metadata=%v error=%v calls=%d", metadata, err, calls)
			}
			if row.code >= 300 {
				if err := port.WriteRepositoryMetadata(map[string]string{"setting": "rejected"}); kernel.CodeOf(err) != row.want || calls != 2 {
					t.Fatalf("metadata write lost backend failure or retried: error=%v calls=%d", err, calls)
				}
			}
			fail = false
			metadata, err = port.ReadRepositoryMetadata()
			if err != nil || !reflect.DeepEqual(metadata, map[string]string{"setting": "original"}) || testkit.MustHead(t, store, snapshot.DefaultRef) != head {
				t.Fatalf("metadata failure changed accepted settings or HEAD: metadata=%v error=%v", metadata, err)
			}
		})
	}
}
