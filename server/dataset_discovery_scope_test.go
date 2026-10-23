package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// An empty facade has no runtime to open: invalid discovery coordinates must
// fail before authorization, source resolution or any engine operation.
func TestDatasetDiscoveryHTTPRejectsRetiredScopesBeforeRuntime(t *testing.T) {
	f := &httpFacade{}
	for name, handler := range map[string]func(http.ResponseWriter, *http.Request){"search": f.objectSearch, "relations": f.objectRelations, "traverse": f.objectTraverse, "index-ready": f.indexReady, "index-prepare": f.indexPrepare, "index-describe": f.indexDescribe, "access-spec": f.accessSpecDescribe} {
		for _, scope := range []string{`{}`, `{"repository":"kr://secret"}`, `{"dataset":"public","repository":"kr://secret"}`, `{"pin":{"setId":"forged","repositories":{"kr://secret":"c1"}}}`, `{"dataset":"public","definition":{"revision":1,"sources":[]}}`} {
			t.Run(name+"/"+scope, func(t *testing.T) {
				request := httptest.NewRequest("POST", "/", strings.NewReader(scope))
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				handler(response, request)
				var payload struct {
					Error struct {
						Code string `json:"code"`
					} `json:"error"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || response.Code < 400 || payload.Error.Code != "USAGE_INVALID" {
					t.Fatalf("retired/implicit scope reached runtime: status=%d body=%s err=%v", response.Code, response.Body.String(), err)
				}
			})
		}
	}
	for _, scope := range []string{`{"dataset":"public","inventoryDiscovery":true,"query":"secret"}`, `{"dataset":"public","commit":"c1","query":"secret"}`, `{"dataset":"public","ref":"main","query":"secret"}`} {
		response := httptest.NewRecorder()
		f.objectSearch(response, httptest.NewRequest("POST", "/", strings.NewReader(scope)))
		if response.Code < 400 || !strings.Contains(response.Body.String(), "USAGE_INVALID") {
			t.Fatalf("search scope bypass reached runtime: %s", response.Body.String())
		}
	}
}
