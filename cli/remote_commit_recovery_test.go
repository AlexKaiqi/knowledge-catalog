package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	kcclient "kc/client"
	"kc/kernel"
	"kc/knowledge/writer"
)

// TestRemoteCommitRecoversReceiptAfterPOSTTimeout pins the receipt-recovery
// contract: when the commit POST times out while the server keeps applying
// (the apply does not ride the request context), the client must recover the
// authoritative receipt from the durable ledger instead of reporting failure.
func TestRemoteCommitRecoversReceiptAfterPOSTTimeout(t *testing.T) {
	var mu sync.Mutex
	applied := false
	receipt := writer.CommitReceipt{
		ReceiptRef:  "receipt:commit:recovered-1",
		CommandID:   "recovered-1",
		Surface:     "COMMIT",
		Disposition: writer.DispositionApplied,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/commits"):
			time.Sleep(200 * time.Millisecond) // server outlives the 50ms client timeout
			mu.Lock()
			applied = true
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(receipt)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/receipts/recovered-1"):
			mu.Lock()
			done := applied
			mu.Unlock()
			entry := map[string]any{"commandId": "recovered-1", "status": "PENDING"}
			if done {
				entry["status"] = "APPLIED"
				entry["receipt"] = receipt
			}
			_ = json.NewEncoder(w).Encode(entry)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	kc, err := kcclient.New(kcclient.Config{
		BaseURL:    server.URL,
		HTTPClient: &http.Client{Timeout: 50 * time.Millisecond},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kc.Login(context.Background(), kcclient.LoginRequest{Identity: kcclient.Identity{Principal: "kaiqidong"}}); err != nil {
		t.Fatal(err)
	}
	output, err := remoteCommitWithReceiptRecovery(context.Background(), kc, "kr://kaiqidong/recovery",
		kcclient.CommitRequest{CommandID: "recovered-1"}, kcclient.RequestOptions{}, 5*time.Second)
	if err != nil {
		t.Fatalf("commit outcome was lost after POST timeout: %v", err)
	}
	got, ok := output.(writer.CommitReceipt)
	if !ok {
		encoded, _ := json.Marshal(output)
		t.Fatalf("recovered output is not a typed receipt: %s", encoded)
	}
	if got.CommandID != "recovered-1" || got.Disposition != writer.DispositionApplied {
		t.Fatalf("recovered receipt mismatch: %#v", got)
	}
}

// TestRemoteCommitDoesNotPollDeterministicRejection pins the other side: a
// server rejection with a deterministic kernel code must come back as-is and
// must not spend the budget polling a receipt that will never exist.
func TestRemoteCommitDoesNotPollDeterministicRejection(t *testing.T) {
	var queries int
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/commits"):
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(kernel.FaultJSON(kernel.Fail(kernel.ErrWriteTargetRequired, "rejected before apply")))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/receipts"):
			mu.Lock()
			queries++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"commandId": "rejected-1", "status": "PENDING"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	kc, err := kcclient.New(kcclient.Config{
		BaseURL:    server.URL,
		HTTPClient: &http.Client{Timeout: 2 * time.Second},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kc.Login(context.Background(), kcclient.LoginRequest{Identity: kcclient.Identity{Principal: "kaiqidong"}}); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = remoteCommitWithReceiptRecovery(context.Background(), kc, "kr://kaiqidong/rejection",
		kcclient.CommitRequest{CommandID: "rejected-1"}, kcclient.RequestOptions{}, 5*time.Second)
	if kernel.CodeOf(err) != kernel.ErrWriteTargetRequired {
		t.Fatalf("deterministic rejection changed shape: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("deterministic rejection waited %.1fs before returning", elapsed.Seconds())
	}
	mu.Lock()
	defer mu.Unlock()
	if queries != 0 {
		t.Fatalf("deterministic rejection polled the receipt ledger %d times", queries)
	}
}
