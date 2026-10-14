package invocation

import (
	"encoding/json"
	"kc/invoke"
	objectmodel "kc/object"
	"net/http/httptest"
	"strings"
	"testing"
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
		value, err := ParseJSON(body, "--value")
		if err != nil {
			t.Fatal(err)
		}
		assertValue(t, value)
	})
	const operation = `{"op":"PUT","address":{"kind":"Entity","objectId":"sample/A"},"value":` + body + `}`
	t.Run("changeset", func(t *testing.T) {
		change, err := DecodeChangeSet([]byte(`{"targetRepository":"kr://acme/public/core","operations":[`+operation+`]}`), "changeset")
		if err != nil {
			t.Fatal(err)
		}
		assertValue(t, change.Operations[0].Value)
	})
	t.Run("proposal", func(t *testing.T) {
		operations, err := ProposeOperations(map[string]invoke.FlagValue{"payload": "[" + operation + "]"})
		if err != nil {
			t.Fatal(err)
		}
		assertValue(t, operations[0].Value)
	})
	t.Run("typed-service-request", func(t *testing.T) {
		var target struct {
			Operations []objectmodel.Operation `json:"operations"`
		}
		request := httptest.NewRequest("POST", "/", strings.NewReader(`{"operations":[`+operation+`]}`))
		response := httptest.NewRecorder()
		if !DecodeServiceRequestBody(response, request, &target, 1<<20) {
			t.Fatal(response.Body.String())
		}
		assertValue(t, target.Operations[0].Value)
	})

}
