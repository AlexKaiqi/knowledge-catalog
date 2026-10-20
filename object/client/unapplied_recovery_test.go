package client

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"kc/kernel"
)

func TestProvenUnappliedWriteNeverPollsReceipt(t *testing.T) {
	for _, operation := range []string{"commit", "put", "remove"} {
		t.Run(operation, func(t *testing.T) {
			posts, queries := 0, 0
			remote, _ := New(Config{BaseURL: "https://example.test", Transport: transportFunc(func(_ context.Context, _ string, r *http.Request) (*http.Response, error) {
				if r.Method == http.MethodPost {
					posts++
					response := response(`{"error":{"code":"TEMPORARY_UNAVAILABLE","message":"staging failed","writeOutcome":"NOT_APPLIED"}}`)
					response.StatusCode = http.StatusServiceUnavailable
					return response, nil
				}
				queries++
				// A stray receipt must never turn a proven rejection
				// into success; any receipt lookup is itself a regression here.
				return response(`{"receipt":{"commandId":"rejected","disposition":"APPLIED"}}`), nil
			})})
			var err error
			if operation == "commit" {
				_, err = CommitWithReceiptRecovery(context.Background(), remote, "kr://test/write", CommitRequest{CommandID: "rejected"}, RequestOptions{}, 10*time.Minute)
			} else {
				_, err = MaintainWithReceiptRecovery(context.Background(), remote, operation == "remove", MaintenanceRequest{CommandID: "rejected"}, RequestOptions{}, 10*time.Minute)
			}
			if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || posts != 1 || queries != 0 {
				t.Fatalf("%s: posts=%d receipt queries=%d error=%v", operation, posts, queries, err)
			}
		})
	}
}

func TestUnknownWriteOutcomeStillRecoversReceipt(t *testing.T) {
	cases := []struct {
		name, body string
		status     int
	}{
		{"legacy response", `{"error":{"code":"TEMPORARY_UNAVAILABLE","message":"not applied"}}`, http.StatusServiceUnavailable},
		{"unknown outcome", `{"error":{"code":"TEMPORARY_UNAVAILABLE","message":"not applied","writeOutcome":"UNKNOWN"}}`, http.StatusServiceUnavailable},
		{"future outcome", `{"error":{"code":"TEMPORARY_UNAVAILABLE","message":"not applied","writeOutcome":"FUTURE_VALUE"}}`, http.StatusServiceUnavailable},
		{"proxy failure", "not applied", http.StatusServiceUnavailable},
		{"malformed success response", "not applied", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			posts, queries := 0, 0
			remote, _ := New(Config{BaseURL: "https://example.test", Transport: transportFunc(func(_ context.Context, _ string, r *http.Request) (*http.Response, error) {
				if r.Method == http.MethodPost {
					posts++
					reply := response(tc.body)
					reply.StatusCode = tc.status
					return reply, nil
				}
				queries++
				if !strings.HasSuffix(r.URL.Path, "/receipts/original") {
					t.Fatalf("wrong receipt lookup: %s", r.URL.Path)
				}
				return response(`{"receipt":{"commandId":"original","disposition":"APPLIED"}}`), nil
			})})
			_, err := CommitWithReceiptRecovery(context.Background(), remote, "kr://test/write", CommitRequest{CommandID: "original"}, RequestOptions{}, time.Second)
			if err != nil || posts != 1 || queries != 1 {
				t.Fatalf("unknown outcome stopped recovery: posts=%d queries=%d err=%v", posts, queries, err)
			}
		})
	}
}
