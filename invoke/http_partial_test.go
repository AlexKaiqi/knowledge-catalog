package invoke

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPPartialOutcomeUsesMultiStatusAndKeepsReceipts(t *testing.T) {
	recorder := httptest.NewRecorder()
	WriteInvoke(recorder, RunResult{Status: 2, Stdout: `{"outcome":"partial","commits":[{"error":"NON_FAST_FORWARD"}]}`})
	if recorder.Code != http.StatusMultiStatus {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "commits") {
		t.Fatalf("partial response lost per-repository results: %s", recorder.Body.String())
	}
}
