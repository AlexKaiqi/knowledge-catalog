package invocation

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"kc/invoke"
	"kc/kernel"
)

func TestDebtTypedServerRejectsMissingPrincipalBeforeEvaluation(t *testing.T) {
	for _, action := range []string{"inventory.read", "object.read", "file.read", "writer.commit"} {
		for _, principal := range []string{"omitted", "empty", "whitespace"} {
			t.Run(action+"/"+principal, func(t *testing.T) {
				dir := t.TempDir()
				t.Setenv("KC_HOME", dir)
				var flags map[string]invoke.FlagValue
				if principal != "omitted" {
					flags = map[string]invoke.FlagValue{"as": ""}
					if principal == "whitespace" {
						flags["as"] = " \t "
					}
				}
				called := false
				operation := Command{Stage: invoke.StageHome, Run: func(*Invocation) (any, error) {
					called = true
					return "must not execute", nil
				}}
				result := InvokeApplicationWithTelemetryAtHome(context.Background(), nil, "debt-server-principal", action, operation, flags, nil, nil)
				var body struct {
					Error *kernel.IngressError `json:"error"`
				}
				decodeErr := json.Unmarshal([]byte(result.Stdout), &body)
				if called || result.Status == 0 || decodeErr != nil || body.Error == nil || body.Error.Code != kernel.ErrUnauthenticated {
					t.Fatalf("Server omission reached owner evaluation: called=%v result=%#v error=%v", called, result, decodeErr)
				}
				entries, err := os.ReadDir(dir)
				if err != nil || len(entries) != 0 {
					t.Fatalf("rejected principal opened or observed Home: entries=%v error=%v", entries, err)
				}
			})
		}
	}
}

func TestDebtTypedServerRetainsAuthorizationAfterIdentity(t *testing.T) {
	dir := t.TempDir()
	called := false
	operation := Command{Stage: invoke.StageHome, Run: func(*Invocation) (any, error) { called = true; return "allowed", nil }}
	flags := map[string]invoke.FlagValue{"home": dir, "as": "reader"}
	denied := InvokeApplicationWithTelemetryAtHome(context.Background(), nil, "debt-server-principal", "inventory.read", operation, flags, nil, nil)
	var body struct {
		Error *kernel.IngressError `json:"error"`
	}
	if err := json.Unmarshal([]byte(denied.Stdout), &body); err != nil || body.Error == nil || body.Error.Code != kernel.ErrForbidden || called {
		t.Fatalf("named principal bypassed authorization: called=%v result=%#v decode=%v", called, denied, err)
	}
	if err := WriteAllow(dir, AllowFile{Rules: []AllowRule{{ID: "reader", Principal: "reader", Actions: []string{"inventory.read"}}}}); err != nil {
		t.Fatal(err)
	}
	allowed := InvokeApplicationWithTelemetryAtHome(context.Background(), nil, "debt-server-principal", "inventory.read", operation, flags, nil, nil)
	if allowed.Status != 0 || !called {
		t.Fatalf("authorized principal was rejected: %#v called=%v", allowed, called)
	}
}

func TestDebtLocalDirectExecutionRetainsOwnerSemantics(t *testing.T) {
	dir := t.TempDir()
	called := false
	value, err := ExecuteApplicationOperation(context.Background(), "local-owner-probe", "inventory.read",
		Command{Stage: invoke.StageHome, Run: func(*Invocation) (any, error) { called = true; return "owner", nil }},
		map[string]invoke.FlagValue{"home": dir}, nil, nil, dir, nil)
	if err != nil || !called || value != "owner" {
		t.Fatalf("local direct execution changed: value=%v called=%v error=%v", value, called, err)
	}
}
