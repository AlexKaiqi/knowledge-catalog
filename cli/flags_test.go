package cli

import (
	"kc/kernel"
	"strings"
	"testing"
)

func TestUnknownFlagIsRejectedBeforeOpeningHome(t *testing.T) {
	result := invokeInternal("object-read", map[string]FlagValue{"home": t.TempDir(), "wrokspace": "agent"})
	if result.Status == 0 || !strings.Contains(result.Stdout, "--wrokspace") {
		t.Fatalf("unknown flag was silently ignored: %#v", result)
	}
	if !strings.Contains(result.Stdout, string(kernel.ErrUsageInvalid)) {
		t.Fatalf("wrong error envelope: %s", result.Stdout)
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
