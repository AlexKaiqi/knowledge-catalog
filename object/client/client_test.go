package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"kc/kernel"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transportFunc func(context.Context, string, *http.Request) (*http.Response, error)

func (f transportFunc) Do(ctx context.Context, audience string, r *http.Request) (*http.Response, error) {
	return f(ctx, audience, r)
}
func response(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestReferenceFragmentTravelsAsJSONAndNullIsExplicit(t *testing.T) {
	reference := "urn:kc:object:table/orders#/columns/order_id/constraints/nullable"
	var paths []string
	remote, err := New(Config{BaseURL: "https://example.test/api", Transport: transportFunc(func(_ context.Context, audience string, r *http.Request) (*http.Response, error) {
		if audience != "https://example.test/api" || r.Header.Get("X-Kc-Request-Id") != "request-1" || r.URL.Fragment != "" {
			t.Fatalf("transport context/URL: %s %#v", audience, r)
		}
		paths = append(paths, r.URL.Path)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["reference"] != reference {
			t.Fatalf("fragment lost before HTTP: %#v", body)
		}
		if strings.HasSuffix(r.URL.Path, ":read") && len(body) != 1 {
			t.Fatalf("read emitted alternate object coordinates: %#v", body)
		}
		if strings.HasSuffix(r.URL.Path, ":put") {
			if value, present := body["value"]; !present || value != nil {
				t.Fatalf("JSON null lost: %#v", body)
			}
		}
		return response("{\"value\":9007199254740993}"), nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	var output any
	options := RequestOptions{RequestID: "request-1"}
	if err := remote.Read(context.Background(), ReadRequest{Reference: reference}, options, &output); err != nil {
		t.Fatal(err)
	}
	if output.(map[string]any)["value"] != json.Number("9007199254740993") {
		t.Fatalf("number rounded: %#v", output)
	}
	if err := remote.Put(context.Background(), MaintenanceRequest{Reference: reference, CommandID: "put", Value: nil}, options, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Join(paths, ",") != "/api/object/v1/objects:read,/api/object/v1/objects:put" {
		t.Fatal(paths)
	}
}

func TestMaintenanceRecoveryPostsOnceAndReturnsOriginalReceipt(t *testing.T) {
	posts, queries := 0, 0
	remote, _ := New(Config{BaseURL: "https://example.test", Transport: transportFunc(func(_ context.Context, _ string, r *http.Request) (*http.Response, error) {
		if r.Method == "POST" {
			posts++
			return nil, errors.New("response lost")
		}
		queries++
		if r.URL.Path != "/writer/v1/receipts/original" {
			t.Fatalf("queried another command: %s", r.URL.Path)
		}
		return response("{\"receipt\":{\"commandId\":\"original\",\"disposition\":\"APPLIED\"}}"), nil
	})})
	_, err := MaintainWithReceiptRecovery(context.Background(), remote, false, MaintenanceRequest{CommandID: "original"}, RequestOptions{}, time.Second)
	if err != nil || posts != 1 || queries != 1 {
		t.Fatalf("uncertain result repeated mutation or lost recovery: posts=%d queries=%d err=%v", posts, queries, err)
	}
}

func TestMaintenanceDeterministicRejectionNeverPolls(t *testing.T) {
	posts, queries := 0, 0
	remote, _ := New(Config{BaseURL: "https://example.test", Transport: transportFunc(func(_ context.Context, _ string, r *http.Request) (*http.Response, error) {
		if r.Method == "POST" {
			posts++
			return nil, kernel.Fail(kernel.ErrForbidden, "denied")
		}
		queries++
		return response("{}"), nil
	})})
	_, err := MaintainWithReceiptRecovery(context.Background(), remote, true, MaintenanceRequest{CommandID: "denied"}, RequestOptions{}, time.Second)
	if kernel.CodeOf(err) != kernel.ErrForbidden || posts != 1 || queries != 0 {
		t.Fatalf("deterministic rejection changed: posts=%d queries=%d err=%v", posts, queries, err)
	}
}

func TestMaintenanceRecoveryRejectsAnotherCommandsReceipt(t *testing.T) {
	remote, _ := New(Config{BaseURL: "https://example.test", Transport: transportFunc(func(_ context.Context, _ string, r *http.Request) (*http.Response, error) {
		if r.Method == "POST" {
			return nil, errors.New("response lost")
		}
		return response(`{"receipt":{"commandId":"another","disposition":"APPLIED"}}`), nil
	})})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := MaintainWithReceiptRecovery(ctx, remote, false, MaintenanceRequest{CommandID: "original"}, RequestOptions{}, time.Second)
	if err == nil {
		t.Fatal("returned another command's result as the original mutation")
	}
}
