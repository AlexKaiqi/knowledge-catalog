package invoke

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGenericServiceJSONIngressPreservesLargeInteger(t *testing.T) {
	const body = `{"value":9007199254740993}`
	request := httptest.NewRequest("POST", "/", strings.NewReader(body))
	value, err := DecodeJSONBody(request)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(value)
	if err != nil || string(raw) != body {
		t.Fatalf("ingress rounded exact JSON number: %s, %v", raw, err)
	}
}
