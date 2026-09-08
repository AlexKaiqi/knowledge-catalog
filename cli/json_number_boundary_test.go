package cli

import (
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
		value, err := parseJSON(body, "--value")
		if err != nil {
			t.Fatal(err)
		}
		assertValue(t, value)
	})
	const operation = `{"op":"PUT","address":{"kind":"Entity","objectId":"sample/A"},"value":` + body + `}`
	t.Run("changeset", func(t *testing.T) {
		change, err := decodeChangeSet([]byte(`{"targetRepository":"kr://acme/public/core","operations":[`+operation+`]}`), "changeset")
		if err != nil {
			t.Fatal(err)
		}
		assertValue(t, change.Operations[0].Value)
	})
	t.Run("proposal", func(t *testing.T) {
		operations, err := proposeOperations(map[string]FlagValue{"payload": "[" + operation + "]"})
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
		if !decodeServiceRequest(response, request, &target) {
			t.Fatal(response.Body.String())
		}
		assertValue(t, target.Operations[0].Value)
	})
	t.Run("generic-service-request", func(t *testing.T) {
		request := httptest.NewRequest("POST", "/", strings.NewReader(body))
		value, err := decodeJSONBody(request)
		if err != nil {
			t.Fatal(err)
		}
		assertValue(t, value)
	})
}
