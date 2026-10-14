package testkit

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"kc/snapshot"
)

func fakeRepositoryMetadataCall(t *testing.T, fake *LakeFSFake, name, method, body string, status int) map[string]string {
	t.Helper()
	request, err := http.NewRequest(method, fake.Origin()+"/api/v1/repositories/"+name+"/metadata", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.SetBasicAuth("access", "secret")
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		t.Fatalf("native metadata %s returned HTTP %d, want %d", method, response.StatusCode, status)
	}
	var metadata map[string]string
	if method == http.MethodGet && status == http.StatusOK {
		if err := json.NewDecoder(response.Body).Decode(&metadata); err != nil {
			t.Fatal(err)
		}
	}
	return metadata
}

func TestLakeFSFakeRepositoryMetadataMergesAndDeletesNamedKeys(t *testing.T) {
	fake := NewLakeFSFake(t)
	name := fake.NewRepo()
	if empty := fakeRepositoryMetadataCall(t, fake, name, http.MethodGet, "", 200); empty == nil || len(empty) != 0 {
		t.Fatalf("new metadata is not an empty map: %v", empty)
	}
	fakeRepositoryMetadataCall(t, fake, name, http.MethodPost, `{"metadata":{"description":"original","retained":"yes"}}`, 204)
	fakeRepositoryMetadataCall(t, fake, name, http.MethodPost, `{"metadata":{"description":"changed","empty":""}}`, 204)
	fakeRepositoryMetadataCall(t, fake, name, http.MethodPost, `{"metadata":{"invalid":false}}`, 400)
	want := map[string]string{"description": "changed", "retained": "yes", "empty": ""}
	if got := fakeRepositoryMetadataCall(t, fake, name, http.MethodGet, "", 200); !reflect.DeepEqual(got, want) {
		t.Fatalf("metadata replacement or invalid update changed native properties: %v", got)
	}
	fakeRepositoryMetadataCall(t, fake, name, http.MethodDelete, `{"keys":["empty","absent"]}`, 204)
	delete(want, "empty")
	if got := fakeRepositoryMetadataCall(t, fake, name, http.MethodGet, "", 200); !reflect.DeepEqual(got, want) {
		t.Fatalf("delete did not retain unrelated metadata: %v", got)
	}
}

func TestLakeFSFakeForkPreservesIndependentRepositoryMetadata(t *testing.T) {
	fake := NewLakeFSFake(t)
	source := fake.NewRepo()
	fakeRepositoryMetadataCall(t, fake, source, http.MethodPost, `{"metadata":{"configuration":"original"}}`, 204)
	root := t.TempDir()
	pointer := filepath.Join(root, "source")
	if err := snapshot.WriteLakeFSStamp(pointer, "kr://fixture/metadata", fake.DSN(source)); err != nil {
		t.Fatal(err)
	}
	if err := fake.ForkStamps(root); err != nil {
		t.Fatal(err)
	}
	_, forkDSN, err := snapshot.ReadLakeFSStamp(pointer)
	if err != nil {
		t.Fatal(err)
	}
	fork := lakefsRepoFromDSN(forkDSN)
	if fork == source || !reflect.DeepEqual(fakeRepositoryMetadataCall(t, fake, fork, http.MethodGet, "", 200), map[string]string{"configuration": "original"}) {
		t.Fatal("fork omitted native metadata or reused source authority")
	}
	fakeRepositoryMetadataCall(t, fake, fork, http.MethodPost, `{"metadata":{"configuration":"fork"}}`, 204)
	if got := fakeRepositoryMetadataCall(t, fake, source, http.MethodGet, "", 200); !reflect.DeepEqual(got, map[string]string{"configuration": "original"}) {
		t.Fatalf("fork metadata update changed source authority: %v", got)
	}
}
