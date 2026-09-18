package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
)

func TestStoresObservationOmitsSecretsAndHomeLayout(t *testing.T) {
	t.Setenv("KC_TEST_OPENSEARCH_URL", "")
	home := testkit.TempDir(t)
	catalogID := "kr://acme/stores/catalog"
	repositoryID := "kr://acme/stores/repository"
	principal := "agent:stores-observe"
	body(t, kc(home, "local", "init", "--catalog", catalogID))
	body(t, kc(home, "local", "repository", "attach", "--repo", repositoryID))
	body(t, kc(home, "store-set", "--index", "opensearch", "--driver", "opensearch", "--url", "http://127.0.0.1:9200"))
	body(t, kc(home, "grant", "add", "--principal", principal, "--action", "catalog.read", "--catalog", catalogID))

	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	status, payload, _ := httpSurfaceRequest(t, server, http.MethodGet, "/operations/v1/stores", nil, principal)
	if status != http.StatusOK {
		t.Fatalf("GET /operations/v1/stores returned %d %#v", status, payload)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, leaked := range []string{"layout", "\"secrets\"", "KC_ELASTICSEARCH", home, "password", "apiKey"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("stores observation leaked %q: %s", leaked, text)
		}
	}
	view := asMap(t, payload)
	snapshot := asMap(t, view["snapshot"])
	retrieval := asMap(t, view["retrieval"])
	if snapshot["driver"] != "dolt" || retrieval["driver"] != "opensearch" || retrieval["origin"] != "http://127.0.0.1:9200" {
		t.Fatalf("store observation %#v", view)
	}
	authorities, _ := snapshot["authorities"].([]any)
	if len(authorities) < 2 {
		t.Fatalf("expected catalog and repository authorities: %#v", snapshot)
	}

	denied, forbidden, _ := httpSurfaceRequest(t, server, http.MethodGet, "/operations/v1/stores", nil, "agent:stranger")
	if denied != http.StatusForbidden {
		t.Fatalf("ungranted principal got %d %#v", denied, forbidden)
	}
}
