package cli

import (
	"kc/kernel"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestUnknownFlagIsRejectedBeforeOpeningServerTransport(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "unknown flags must be rejected before transport", http.StatusInternalServerError)
	}))
	defer server.Close()
	result := Run([]string{"read", "urn:kc:object:policy/A", "--server", server.URL, "--as", "alice", "--wrokspace", "agent"})
	if result.Status == 0 || !strings.Contains(result.Stdout, "--wrokspace") {
		t.Fatalf("unknown flag was silently ignored: %#v", result)
	}
	if !strings.Contains(result.Stdout, string(kernel.ErrUsageInvalid)) {
		t.Fatalf("wrong error envelope: %s", result.Stdout)
	}
	if requests.Load() != 0 {
		t.Fatalf("unknown flag opened Server transport: %d requests", requests.Load())
	}
}

func TestTransportSpecificFlagsCannotLeakAcrossSurfaces(t *testing.T) {
	result := invokeInternal("object-read", map[string]FlagValue{"home": t.TempDir(), "listen": "127.0.0.1:0"})
	if result.Status == 0 || !strings.Contains(result.Stdout, "only valid for kc serve") {
		t.Fatalf("command accepted server-only flag: %#v", result)
	}

	result = Run([]string{"serve", "--config", "deployment.yaml", "--object", "policy/A"})
	if result.Status == 0 || !strings.Contains(result.Stdout, "not valid for kc serve") {
		t.Fatalf("serve accepted command-only flag: %#v", result)
	}

	result = Run([]string{"serve", "--help"})
	if result.Status != 0 || !strings.Contains(result.Stdout, "kc serve") {
		t.Fatalf("serve help should not start a listener: %#v", result)
	}
}
