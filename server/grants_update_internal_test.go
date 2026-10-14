package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	inv "kc/invocation"
	"kc/kernel"
)

func grantLedgerWithMigrationForTest(t *testing.T, dir string) inv.AllowFile {
	t.Helper()
	file, err := inv.ReadAllow(dir)
	if err != nil {
		t.Fatal(err)
	}
	file.Admissions = map[string]inv.AdmissionReceipt{"private-admission": {Principal: "reader", Actions: []string{"file.read"}}}
	if err := inv.WriteAllow(dir, file); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestGrantHTTPUpdatePreservesTheApplicationLedger(t *testing.T) {
	dir, handler := grantTestFacade(t, []inv.AllowRule{
		{ID: "manager", Principal: "operator", Actions: []string{"admin.grants.manage"}},
		{ID: "target", Principal: "reader", Actions: []string{"file.read"}, SharedBy: "publisher"},
	})
	before := grantLedgerWithMigrationForTest(t, dir)
	body := `{"principal":"reader","actions":["file.read"],"dataset":"narrowed","expected":{"id":"target","principal":"reader","actions":["file.read"],"sharedBy":"publisher"}}`
	updated := grantRequestForTest(t, handler, "operator", http.MethodPut, "/admin/v1/grants/target", body, http.StatusOK)
	if updated["id"] != "target" || updated["sharedBy"] != "publisher" || updated["dataset"] != "narrowed" {
		t.Fatalf("unexpected replacement receipt: %#v", updated)
	}
	grantRequestForTest(t, handler, "operator", http.MethodPut, "/admin/v1/grants/target", body, http.StatusConflict)
	after, err := inv.ReadAllow(dir)
	if err != nil {
		t.Fatal(err)
	}
	before.Rules[1].Dataset = "narrowed"
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("rule update damaged existing application state: %#v", after)
	}
}

func TestConcurrentGrantHTTPUpdatesPreserveOneDurableWinner(t *testing.T) {
	old := inv.AllowRule{ID: "target", Principal: "reader", Actions: []string{"file.read"}, Dataset: "one", SharedBy: "publisher"}
	dir, handler := grantTestFacade(t, []inv.AllowRule{{ID: "manager", Principal: "operator", Actions: []string{"admin.grants.manage"}}, old})
	before := grantLedgerWithMigrationForTest(t, dir)
	server := httptest.NewServer(handler)
	defer server.Close()
	client := server.Client()
	client.Timeout = 10 * time.Second
	type result struct {
		requested inv.AllowRule
		status    int
		body      []byte
		err       error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for _, action := range []string{"dataset.resolve", "dataset.manage"} {
		requested := old
		requested.Actions = []string{action}
		body, err := json.Marshal(map[string]any{"principal": old.Principal, "actions": requested.Actions, "dataset": old.Dataset, "expected": map[string]any{"id": old.ID, "principal": old.Principal, "actions": old.Actions, "dataset": old.Dataset, "sharedBy": old.SharedBy}})
		if err != nil {
			t.Fatal(err)
		}
		request, err := http.NewRequest(http.MethodPut, server.URL+"/admin/v1/grants/"+old.ID, strings.NewReader(string(body)))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("X-Kc-As", "operator")
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			response, err := client.Do(request)
			out := result{requested: requested, err: err}
			if err == nil {
				out.status = response.StatusCode
				out.body, out.err = io.ReadAll(response.Body)
				response.Body.Close()
			}
			results <- out
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	winners, conflicts := 0, 0
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		switch result.status {
		case http.StatusOK:
			var receipt inv.AllowRule
			if err := json.Unmarshal(result.body, &receipt); err != nil || !reflect.DeepEqual(receipt, result.requested) {
				t.Fatalf("winner receipt changed the requested rule: %s %v", result.body, err)
			}
			before.Rules[1] = result.requested
			winners++
		case http.StatusConflict:
			var response struct {
				Error struct{ Code kernel.ErrorCode } `json:"error"`
			}
			if err := json.Unmarshal(result.body, &response); err != nil || response.Error.Code != kernel.ErrPreconditionFailed {
				t.Fatalf("editor conflict changed: %s %v", result.body, err)
			}
			conflicts++
		default:
			t.Fatalf("unexpected editor status: %d %s", result.status, result.body)
		}
	}
	after, err := inv.ReadAllow(dir)
	if err != nil || winners != 1 || conflicts != 1 || !reflect.DeepEqual(after, before) {
		t.Fatalf("concurrent edits lost the winner or private ledger: winners=%d conflicts=%d want=%#v got=%#v err=%v", winners, conflicts, before, after, err)
	}
}
