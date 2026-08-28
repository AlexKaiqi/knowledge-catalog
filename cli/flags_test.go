package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kc/kernel"
)

func TestUnknownFlagIsRejectedBeforeOpeningHome(t *testing.T) {
	result := invokeInternal("read", map[string]FlagValue{"home": t.TempDir(), "wrokspace": "agent"})
	if result.Status == 0 || !strings.Contains(result.Stdout, "--wrokspace") {
		t.Fatalf("unknown flag was silently ignored: %#v", result)
	}
	if !strings.Contains(result.Stdout, string(kernel.ErrUsageInvalid)) {
		t.Fatalf("wrong error envelope: %s", result.Stdout)
	}
}

func TestTransportSpecificFlagsCannotLeakAcrossSurfaces(t *testing.T) {
	result := invokeInternal("read", map[string]FlagValue{"home": t.TempDir(), "listen": "127.0.0.1:0"})
	if result.Status == 0 || !strings.Contains(result.Stdout, "only valid for kc serve") {
		t.Fatalf("command accepted server-only flag: %#v", result)
	}

	result = Run([]string{"serve", "--home", t.TempDir(), "--object", "policy/A"})
	if result.Status == 0 || !strings.Contains(result.Stdout, "not valid for kc serve") {
		t.Fatalf("serve accepted command-only flag: %#v", result)
	}

	result = Run([]string{"serve", "--help"})
	if result.Status != 0 || !strings.Contains(result.Stdout, "kc serve") {
		t.Fatalf("serve help should not start a listener: %#v", result)
	}
}

func TestHTTPPartialOutcomeUsesMultiStatusAndKeepsReceipts(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeInvoke(recorder, RunResult{Status: 2, Stdout: `{"outcome":"partial","commits":[{"error":"NON_FAST_FORWARD"}]}`})
	if recorder.Code != http.StatusMultiStatus {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "commits") {
		t.Fatalf("partial response lost per-repository results: %s", recorder.Body.String())
	}
}
