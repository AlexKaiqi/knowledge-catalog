package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"kc/cli"
)

// TestManagedRepositoryCreatePublicArgv drives the exact public `create` argv
// through the real Run entry. It gives the managed-repository-created state a
// named formal command evidence for the product main path (see
// TestSceneFeaturesCoverPublicCLI); the behavioral provider journey remains
// TestManagedRepositoryProviderCreatesPublishesAndResumes.
func TestManagedRepositoryCreatePublicArgv(t *testing.T) {
	isolateClientCredentials(t)
	const catalogID = "kr://managed/catalog"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/catalog/v1/catalogs":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"catalogs": []map[string]string{{"id": catalogID}}})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/catalog/v1/repositories":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"catalog":"kr://managed/catalog","repositoryId":"kr://managed/source","status":"APPLIED","head":"initial"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	result := cli.Run([]string{"create", "--name", "source", "--server", server.URL, "--as", "agent:creator"})
	if result.Status != 0 {
		t.Fatalf("kc create --name: %s", result.Stdout)
	}
	// Two independent state-changing boundaries: no source at all, and a mixed
	// --name/--url request. Both fail before any Server call.
	expectCode(t, kcRemote(t, server.URL, "agent:creator", "create"), "USAGE_INVALID")
	expectCode(t, kcRemote(t, server.URL, "agent:creator", "create",
		"--name", "source", "--url", "https://git.example/acme/source.git", "--credential-file", "credential.json"), "USAGE_INVALID")
}
