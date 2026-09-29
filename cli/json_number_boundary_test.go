package cli

import (
	inv "kc/invocation"
	"kc/invoke"

	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"kc/knowledge"
)

func TestWriterJSONIngressPreservesLargeInteger(t *testing.T) {
	const body = `{"value":9007199254740993}`
	assertValue := func(t *testing.T, value any) {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil || string(raw) != body {
			t.Fatalf("ingress rounded exact JSON number: %s, %v", raw, err)
		}
	}
	t.Run("value-flag", func(t *testing.T) {
		value, err := inv.ParseJSON(body, "--value")
		if err != nil {
			t.Fatal(err)
		}
		assertValue(t, value)
	})
	const operation = `{"op":"PUT","address":{"kind":"Entity","objectId":"sample/A"},"value":` + body + `}`
	t.Run("changeset", func(t *testing.T) {
		change, err := inv.DecodeChangeSet([]byte(`{"targetRepository":"kr://acme/public/core","operations":[`+operation+`]}`), "changeset")
		if err != nil {
			t.Fatal(err)
		}
		assertValue(t, change.Operations[0].Value)
	})
	t.Run("proposal", func(t *testing.T) {
		operations, err := inv.ProposeOperations(map[string]FlagValue{"payload": "[" + operation + "]"})
		if err != nil {
			t.Fatal(err)
		}
		assertValue(t, operations[0].Value)
	})
	t.Run("typed-service-request", func(t *testing.T) {
		var target struct {
			Operations []knowledge.Operation `json:"operations"`
		}
		request := httptest.NewRequest("POST", "/", strings.NewReader(`{"operations":[`+operation+`]}`))
		response := httptest.NewRecorder()
		if !inv.DecodeServiceRequestBody(response, request, &target, 1<<20) {
			t.Fatal(response.Body.String())
		}
		assertValue(t, target.Operations[0].Value)
	})
	t.Run("generic-service-request", func(t *testing.T) {
		request := httptest.NewRequest("POST", "/", strings.NewReader(body))
		value, err := invoke.DecodeJSONBody(request)
		if err != nil {
			t.Fatal(err)
		}
		assertValue(t, value)
	})
}
