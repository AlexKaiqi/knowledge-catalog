package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	kcclient "kc/client"
	"kc/knowledge"
)

// TestRemoteCurrentDigestsRunsConcurrently pins the diff-preflight contract:
// resolving the current digests of N operations fans out over a bounded worker
// pool instead of paying one serial HTTP round trip per address. On a 53k
// object first import the serial loop is the dominant client cost.
func TestRemoteCurrentDigestsRunsConcurrently(t *testing.T) {
	var mu sync.Mutex
	var maxInFlight, inFlight int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&inFlight, 1)
		for {
			observed := atomic.LoadInt32(&maxInFlight)
			if current <= observed || atomic.CompareAndSwapInt32(&maxInFlight, observed, current) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond) // widen the overlap window
		var request kcclient.KnowledgeResolveRequest
		_ = json.NewDecoder(r.Body).Decode(&request)
		atomic.AddInt32(&inFlight, -1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"repository": request.Repository,
			"commit":     request.Commit,
			"objectId":   request.Object,
			"status":     "RESOLVED",
			"digest":     "digest-" + request.Object,
		})
		mu.Lock()
		defer mu.Unlock()
	}))
	defer server.Close()
	kc, err := kcclient.New(kcclient.Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kc.Login(context.Background(), kcclient.LoginRequest{Identity: kcclient.Identity{Principal: "kaiqidong"}}); err != nil {
		t.Fatal(err)
	}
	ops := make([]knowledge.Operation, 0, 12)
	for i := range 12 {
		ops = append(ops, knowledge.Operation{
			Op:      knowledge.OpPut,
			Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: knowledge.ObjectID(fmt.Sprintf("obj/%02d", i))},
		})
	}
	got, err := remoteCurrentDigests(context.Background(), kc, "kr://kaiqidong/preflight", "c0", ops, kcclient.RequestOptions{})
	if err != nil {
		t.Fatalf("concurrent preflight failed: %v", err)
	}
	if len(got) != len(ops) {
		t.Fatalf("preflight resolved %d of %d digests", len(got), len(ops))
	}
	if got[knowledge.AddressKey(ops[7].Address)] != "digest-obj/07" {
		t.Fatalf("digest mapping mismatch: %q", got[knowledge.AddressKey(ops[7].Address)])
	}
	if observed := atomic.LoadInt32(&maxInFlight); observed < 2 {
		t.Fatalf("diff preflight stayed serial: max in-flight resolves = %d", observed)
	}
}

// TestRemoteCurrentDigestsSkipsUnresolved keeps the skip semantics: an
// unresolved address contributes no entry, and empty digests are ignored.
func TestRemoteCurrentDigestsSkipsUnresolved(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request kcclient.KnowledgeResolveRequest
		_ = json.NewDecoder(r.Body).Decode(&request)
		if strings.HasSuffix(request.Object, "missing") {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "UNRESOLVED"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "RESOLVED", "digest": ""})
	}))
	defer server.Close()
	kc, err := kcclient.New(kcclient.Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kc.Login(context.Background(), kcclient.LoginRequest{Identity: kcclient.Identity{Principal: "kaiqidong"}}); err != nil {
		t.Fatal(err)
	}
	ops := []knowledge.Operation{
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "obj/missing"}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "obj/empty"}},
	}
	got, err := remoteCurrentDigests(context.Background(), kc, "kr://kaiqidong/preflight", "c0", ops, kcclient.RequestOptions{})
	if err != nil {
		t.Fatalf("preflight failed: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("unresolved and empty digests must not enter the diff basis: %#v", got)
	}
}
