package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestObjectEnvelopeRejectsInvalidDatasetMetadata(t *testing.T) {
	for _, metadata := range []string{`null`, `{"x":null}`, `{"x":1}`, `{"x":[]}`, `{"x":"a","x":"b"}`} {
		var request objectReadRequest
		r := httptest.NewRequest(http.MethodPost, "/object/v1/read", strings.NewReader(`{"reference":"urn:kc:object:kr%3A%2F%2Fsource%2Fa:object","definition":{"revision":1,"sources":[],"metadata":`+metadata+`}}`))
		response := httptest.NewRecorder()
		if decodeServiceRequest(response, r, &request) || response.Code != http.StatusBadRequest {
			t.Fatalf("invalid inline metadata was accepted: %s HTTP %d", metadata, response.Code)
		}
	}
}

func TestObjectEnvelopeKeepsResourceMetadataSeparateFromQueryValues(t *testing.T) {
	var request struct {
		objectScope
		Value any `json:"value"`
	}
	body := `{"definition":{"revision":1,"sources":[],"metadata":{"custom":"yes"}},"value":{"metadata":{"number":9007199254740993,"nullable":null}}}`
	r := httptest.NewRequest(http.MethodPost, "/object/v1/read", strings.NewReader(body))
	if !decodeServiceRequest(httptest.NewRecorder(), r, &request) || request.Definition.Metadata["custom"] != "yes" {
		t.Fatalf("valid resource metadata rejected: %#v", request)
	}
	value := request.Value.(map[string]any)["metadata"].(map[string]any)
	number, exact := value["number"].(json.Number)
	if !exact || number.String() != "9007199254740993" || value["nullable"] != nil {
		t.Fatalf("resource metadata validator changed opaque values or exact numbers: %#v", value)
	}
	var writer writerCommitRequest
	if requestHasDatasetDefinition(&writer) {
		t.Fatal("large Writer requests must retain the streaming path")
	}
}
