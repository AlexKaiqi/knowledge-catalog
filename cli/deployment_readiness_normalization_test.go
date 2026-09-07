package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"kc/cli"
)

func TestDeploymentReadinessUsesNormalizedIndexDriver(t *testing.T) {
	unavailable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "backend unavailable", http.StatusServiceUnavailable)
	}))
	defer unavailable.Close()
	for _, driver := range []string{"opensearch", "OpenSearch", " opensearch "} {
		t.Run(driver, func(t *testing.T) {
			cfg, path := declaredDeployment(t, false)
			cfg.Stores.Index = driver
			cfg.Stores.OpenSearch.URL = unavailable.URL
			writeDeployment(t, path, cfg)
			body(t, deploymentCommand(t, "deployment", "init", "--config", path))
			handler, err := cli.HTTPHandlerFromConfig(path, cli.HTTPServerOptions{})
			if err != nil {
				t.Fatal(err)
			}
			defer handler.(interface{ Close() error }).Close()
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz/search", nil))
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("accepted index driver %q skipped backend probe: %d %s", driver, response.Code, response.Body.String())
			}
			var result struct {
				ReasonCode string `json:"reasonCode"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.ReasonCode != "SEARCH_BACKEND_UNAVAILABLE" {
				t.Fatalf("unexpected readiness failure: %s", response.Body.String())
			}
		})
	}
}
