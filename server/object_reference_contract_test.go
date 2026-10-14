package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestObjectReadWriteHTTPRejectsSeparateContentCoordinates(t *testing.T) {
	const reference = "urn:kc:object:kr%3A%2F%2Fsource%2Fa:table%2Forders#/columns/id"
	facade := &httpFacade{}
	for _, route := range []struct {
		name string
		run  http.HandlerFunc
	}{
		{"read", facade.knowledgeRead},
		{"put", facade.knowledgePut},
		{"remove", facade.knowledgeRemove},
	} {
		for key, value := range map[string]any{
			"repository": "kr://source/a", "object": "table/orders",
			"aspect": "columns", "member": "id", "include": []string{"columns"}, "exclude": []string{"private"},
		} {
			t.Run(route.name+"/"+key, func(t *testing.T) {
				body, err := json.Marshal(map[string]any{"reference": reference, "commandId": "change", "value": nil, key: value})
				if route.name == "read" {
					body, err = json.Marshal(map[string]any{"reference": reference, key: value})
				}
				if err != nil {
					t.Fatal(err)
				}
				result := httptest.NewRecorder()
				route.run(result, httptest.NewRequest(http.MethodPost, "/knowledge/v1/objects:"+route.name, strings.NewReader(string(body))))
				if result.Code != http.StatusBadRequest || !strings.Contains(result.Body.String(), "USAGE_INVALID") {
					t.Fatalf("separate coordinate reached execution: HTTP %d %s", result.Code, result.Body.String())
				}
			})
		}
	}
}

func TestObjectReadWriteHTTPRequiresACompleteReference(t *testing.T) {
	facade := &httpFacade{}
	for _, route := range []struct {
		name string
		run  http.HandlerFunc
	}{
		{"read", facade.knowledgeRead},
		{"put", facade.knowledgePut},
		{"remove", facade.knowledgeRemove},
	} {
		for _, reference := range []string{"", "table/orders", "#/columns/id"} {
			t.Run(route.name+"/"+reference, func(t *testing.T) {
				input := map[string]any{"reference": reference}
				if route.name != "read" {
					input["commandId"], input["value"] = "change", nil
				}
				body, _ := json.Marshal(input)
				result := httptest.NewRecorder()
				route.run(result, httptest.NewRequest(http.MethodPost, "/knowledge/v1/objects:"+route.name, strings.NewReader(string(body))))
				if result.Code != http.StatusBadRequest || !strings.Contains(result.Body.String(), "USAGE_INVALID") {
					t.Fatalf("incomplete reference reached execution: HTTP %d %s", result.Code, result.Body.String())
				}
			})
		}
	}
}
