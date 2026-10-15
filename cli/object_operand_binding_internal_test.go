package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"kc/invoke"
)

const objectOperandTestReference = "urn:kc:object:table/orders#/columns/id"

func TestObjectPositionalsUseModuleBindingBeforeValidation(t *testing.T) {
	for _, argv := range [][]string{
		{"read", objectOperandTestReference, "--commit", "fixed"},
		{"writer", "put", objectOperandTestReference, "--command-id", "change", "--value", "null"},
		{"writer", "remove", objectOperandTestReference, "--command-id", "change"},
	} {
		argv = append(argv, "--as", "caller", "--request-id", "correlation")
		request, stopped := prepareCLI(argv)
		if stopped != nil {
			t.Fatalf("positional did not reach owned binding: %#v", stopped)
		}
		if invoke.FlagString(request.flags, "reference") != objectOperandTestReference || invoke.FlagString(request.flags, "as") != "caller" || invoke.FlagString(request.flags, "request-id") != "correlation" {
			t.Fatalf("module binding changed the complete reference: %#v", request.flags)
		}
		if request.path != "read" && invoke.FlagString(request.flags, "command-id") != "change" {
			t.Fatalf("module binding changed the caller's command identity: %#v", request.flags)
		}
		if _, rawFlag := request.flags["reference"].(string); rawFlag {
			t.Fatal("host erased the module's positional proof")
		}
		if err := validateCLIRequest(request); err != nil {
			t.Fatalf("owned positional was rejected as a flag: %v", err)
		}
		if err := preflightObjectClient(request.path, request.flags); err != nil {
			t.Fatalf("owned operand was lost before execution: %v", err)
		}
	}
}

func TestObjectReferenceFlagAliasFailsBeforeAnyHTTP(t *testing.T) {
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)
	for _, command := range []struct {
		parts []string
		flags []string
	}{
		{[]string{"read"}, nil},
		{[]string{"writer", "put"}, []string{"--command-id", "must-not-write", "--value", "null"}},
		{[]string{"writer", "remove"}, []string{"--command-id", "must-not-write"}},
	} {
		for _, double := range []bool{false, true} {
			argv := append([]string(nil), command.parts...)
			if double {
				argv = append(argv, objectOperandTestReference)
			}
			argv = append(argv, command.flags...)
			argv = append(argv, "--reference", objectOperandTestReference, "--server", server.URL)
			result := Run(argv)
			if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") || !strings.Contains(result.Stdout, "--reference") || !strings.Contains(result.Stdout, "object URN") || requests.Load() != 0 {
				t.Fatalf("target alias reached HTTP or a later failure: argv=%v result=%#v requests=%d", argv, result, requests.Load())
			}
		}
	}
}
