package home

import (
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
)

type metadataWrapperReader interface {
	ReadRepositoryMetadata() (map[string]string, error)
}

type metadataWrapperWriter interface {
	WriteRepositoryMetadata(map[string]string) error
}

func TestConnectedSourcePreservesNativeMetadataAndRotatedCredential(t *testing.T) {
	var credential atomic.Value
	credential.Store("access:original-secret")
	var failure atomic.Int64
	fake := testkit.NewLakeFSFakeWithInterceptor(t, func(w http.ResponseWriter, r *http.Request) bool {
		user, secret, ok := r.BasicAuth()
		if !ok || user+":"+secret != credential.Load().(string) {
			w.WriteHeader(http.StatusUnauthorized)
			return true
		}
		if status := failure.Load(); status != 0 && strings.HasSuffix(r.URL.Path, "/metadata") {
			w.WriteHeader(int(status))
			return true
		}
		return false
	})
	name := fake.NewRepo()
	id := kernel.RepositoryID("kr://metadata/connected")
	store, err := snapshot.OpenLakeFS(id, fake.DSN(name), credential.Load().(string))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.(metadataWrapperWriter).WriteRepositoryMetadata(map[string]string{"setting": "original", "retained": "yes"}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := createConnectionLedger(filepath.Join(dir, connectionLedgerFile)); err != nil {
		t.Fatal(err)
	}
	record := connectionRecord{
		AllocationID: strings.Repeat("a", 32), Owner: "alice",
		Binding: RepositoryBinding{ID: string(id), Driver: "lakefs", DSN: fake.DSN(name)}, BackendID: name,
		InitialHead: testkit.MustHead(t, store, snapshot.DefaultRef), Phase: "READY",
		Actions: []string{"repository.connections.manage", "repository.metadata.read"},
	}
	if err := saveConnection(dir, &record, credential.Load().(string), 0); err != nil {
		t.Fatal(err)
	}
	var wrapped snapshot.Store = &connectedSource{dir: dir, id: id}
	read, readable := wrapped.(metadataWrapperReader)
	write, writable := wrapped.(metadataWrapperWriter)
	if !readable || !writable {
		t.Fatal("connected source omitted native metadata capabilities")
	}
	metadata, err := read.ReadRepositoryMetadata()
	if err != nil || !reflect.DeepEqual(metadata, map[string]string{"setting": "original", "retained": "yes"}) {
		t.Fatalf("wrapped read changed native metadata: metadata=%v error=%v", metadata, err)
	}
	credential.Store("access:replacement-secret")
	if err := saveConnection(dir, &record, credential.Load().(string), record.Revision); err != nil {
		t.Fatal(err)
	}
	if err := write.WriteRepositoryMetadata(map[string]string{"setting": "rotated"}); err != nil {
		t.Fatalf("existing wrapper did not use the rotated credential: %v", err)
	}
	metadata, err = read.ReadRepositoryMetadata()
	if err != nil || !reflect.DeepEqual(metadata, map[string]string{"setting": "rotated", "retained": "yes"}) {
		t.Fatalf("rotation or metadata update replaced unrelated properties: metadata=%v error=%v", metadata, err)
	}
	for _, row := range []struct {
		status int64
		code   kernel.ErrorCode
	}{{403, kernel.ErrForbidden}, {503, kernel.ErrTemporaryUnavailable}} {
		failure.Store(row.status)
		metadata, err = read.ReadRepositoryMetadata()
		if metadata != nil || kernel.CodeOf(err) != row.code {
			t.Fatalf("metadata failure became empty configuration: metadata=%v error=%v", metadata, err)
		}
		if err := write.WriteRepositoryMetadata(map[string]string{"setting": "rejected"}); kernel.CodeOf(err) != row.code {
			t.Fatalf("metadata write lost its failure: %v", err)
		}
	}
	failure.Store(0)
	metadata, err = read.ReadRepositoryMetadata()
	if err != nil || !reflect.DeepEqual(metadata, map[string]string{"setting": "rotated", "retained": "yes"}) || testkit.MustHead(t, wrapped, snapshot.DefaultRef) != record.InitialHead {
		t.Fatalf("rejected metadata update changed native properties or HEAD: metadata=%v error=%v", metadata, err)
	}
}

func TestSystemAuthorityRetainsMetadataReadsWithoutExposingWrites(t *testing.T) {
	var failure atomic.Bool
	fake := testkit.NewLakeFSFakeWithInterceptor(t, func(w http.ResponseWriter, r *http.Request) bool {
		if failure.Load() && strings.HasSuffix(r.URL.Path, "/metadata") {
			w.WriteHeader(http.StatusServiceUnavailable)
			return true
		}
		return false
	})
	source, err := snapshot.OpenLakeFS("kr://kc/system", fake.DSN(fake.NewRepo()), fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	if err := source.(metadataWrapperWriter).WriteRepositoryMetadata(map[string]string{"setting": "native"}); err != nil {
		t.Fatal(err)
	}
	wrapped, err := readOnlySystemAuthority(source)
	if err != nil {
		t.Fatal(err)
	}
	read, ok := wrapped.(metadataWrapperReader)
	if !ok {
		t.Fatal("read-only System wrapper omitted native metadata reads")
	}
	if _, ok := wrapped.(metadataWrapperWriter); ok {
		t.Fatal("read-only System wrapper exposed native metadata mutation")
	}
	metadata, err := read.ReadRepositoryMetadata()
	if err != nil || !reflect.DeepEqual(metadata, map[string]string{"setting": "native"}) {
		t.Fatalf("System metadata read did not use native authority: metadata=%v error=%v", metadata, err)
	}
	failure.Store(true)
	if metadata, err := read.ReadRepositoryMetadata(); metadata != nil || kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Fatalf("System metadata failure became empty configuration: metadata=%v error=%v", metadata, err)
	}
	// Hide the optional metadata capability on the same actual authority:
	// wrapping must not turn absence into a fictitious implemented port.
	withoutMetadata := struct {
		snapshot.Store
		snapshot.TreeReader
		snapshot.DirectoryReader
		snapshot.HistoryStore
		snapshot.ChangeStore
	}{source, source.(snapshot.TreeReader), source.(snapshot.DirectoryReader), source.(snapshot.HistoryStore), source.(snapshot.ChangeStore)}
	unsupported, err := readOnlySystemAuthority(withoutMetadata)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := unsupported.(metadataWrapperReader); ok {
		t.Fatal("System wrapper falsely advertised absent optional metadata capability")
	}
}
