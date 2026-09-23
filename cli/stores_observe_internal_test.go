package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"kc/retrieval/opensearch"
)

func TestPublicHTTPEntryOmitsSecretsAndKeepsAuthorityOrigin(t *testing.T) {
	origin, name := publicHTTPEntry("http://127.0.0.1:18000/kc/acme-core")
	if origin != "http://127.0.0.1:18000/kc" || name != "acme-core" {
		t.Fatalf("lakefs-style dsn: origin=%q name=%q", origin, name)
	}
	origin, name = publicHTTPEntry("https://os.example:9200")
	if origin != "https://os.example:9200" || name != "" {
		t.Fatalf("opensearch url: origin=%q name=%q", origin, name)
	}
	if origin, name = publicHTTPEntry("http://user:secret@127.0.0.1:9200"); origin != "" || name != "" {
		t.Fatalf("secret dsn must be omitted, got origin=%q name=%q", origin, name)
	}
	if origin, name = publicHTTPEntry("s3://bucket/prefix"); origin != "" || name != "" {
		t.Fatalf("non-http dsn must be omitted, got origin=%q name=%q", origin, name)
	}
}

func TestObservePublicStoresOmitsLayoutAndSecretEnvNames(t *testing.T) {
	stores := StoresFile{
		Layout:     LayoutFile{Repos: "/tmp/secret-home/repos", Catalogs: "/tmp/secret-home/catalogs"},
		Profile:    "scale",
		Repository: "lakefs",
		Index:      "opensearch",
		OpenSearch: opensearch.Config{URL: "https://os.example:9200", User: "elastic", Password: "hunter2"},
	}
	view := observePublicStores(stores, HomeFile{
		Catalogs: []HomeCatalog{{ID: "kr://acme/catalog", Dir: "catalogs/acme"}},
		Repos:    []HomeRepo{{ID: "kr://acme/core", Dir: "repos/acme", Driver: "lakefs", DSN: "http://127.0.0.1:18000/acme-core"}},
	}, nil, []storeBinding{{ID: "kr://acme/extra", Driver: "lakefs", DSN: "http://user:secret@127.0.0.1:18000/extra"}})
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, leaked := range []string{"layout", "secret-home", "hunter2", "elastic", "KC_ELASTICSEARCH", "user:secret", "repos/acme"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("observation leaked %q: %s", leaked, text)
		}
	}
	snapshot := view["snapshot"].(map[string]any)
	retrieval := view["retrieval"].(map[string]any)
	if snapshot["driver"] != "lakefs" || snapshot["profile"] != "scale" || retrieval["driver"] != "opensearch" || retrieval["origin"] != "https://os.example:9200" {
		t.Fatalf("binding %#v", view)
	}
	authorities := snapshot["authorities"].([]map[string]any)
	if len(authorities) != 3 {
		t.Fatalf("authorities %#v", authorities)
	}
	var core, extra map[string]any
	for _, item := range authorities {
		switch item["id"] {
		case "kr://acme/core":
			core = item
		case "kr://acme/extra":
			extra = item
		}
	}
	if core["origin"] != "http://127.0.0.1:18000" || core["name"] != "acme-core" {
		t.Fatalf("snapshot entry %#v", core)
	}
	if extra == nil {
		t.Fatalf("missing extra authority: %#v", authorities)
	}
	if _, ok := extra["origin"]; ok {
		t.Fatalf("secret extra binding must not expose an origin: %#v", extra)
	}
}

func TestObservePublicStoresPublishesBrowserOrigins(t *testing.T) {
	t.Setenv("KC_LAKEFS_URL", "http://lakefs:8000")
	t.Setenv("KC_LAKEFS_PUBLIC_URL", "http://127.0.0.1:18001")
	t.Setenv("KC_OPENSEARCH_URL", "http://opensearch:9200")
	t.Setenv("KC_OPENSEARCH_PUBLIC_URL", "http://127.0.0.1:19201")
	view := observePublicStores(StoresFile{
		Profile: "local", Repository: "lakefs", Index: "opensearch",
		OpenSearch: opensearch.Config{URL: "http://opensearch:9200"},
	}, HomeFile{
		Catalogs: []HomeCatalog{{ID: "kr://acme/catalog"}},
		Repos:    []HomeRepo{{ID: "kr://acme/core", Driver: "lakefs", DSN: "http://lakefs:8000/table-meta"}},
	}, []storeBinding{{ID: "kr://acme/catalog", Driver: "lakefs", DSN: "http://lakefs:8000/kc-catalog"}}, nil)
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if text := string(raw); strings.Contains(text, "lakefs:8000") || strings.Contains(text, "opensearch:9200") {
		t.Fatalf("observation kept cluster-internal origins: %s", text)
	}
	snapshot := view["snapshot"].(map[string]any)
	retrieval := view["retrieval"].(map[string]any)
	if snapshot["driver"] != "lakefs" || retrieval["origin"] != "http://127.0.0.1:19201" {
		t.Fatalf("published observation %#v", view)
	}
	var catalog, repo map[string]any
	for _, item := range snapshot["authorities"].([]map[string]any) {
		switch item["id"] {
		case "kr://acme/catalog":
			catalog = item
		case "kr://acme/core":
			repo = item
		}
	}
	if catalog["origin"] != "http://127.0.0.1:18001" || catalog["name"] != "kc-catalog" {
		t.Fatalf("catalog origin %#v", catalog)
	}
	if repo["origin"] != "http://127.0.0.1:18001" || repo["name"] != "table-meta" {
		t.Fatalf("repository origin %#v", repo)
	}
}

func TestPublishedOriginLeavesReachableHostsAndDropsSecretPublicURL(t *testing.T) {
	if got := publishedOrigin("http://127.0.0.1:18000", "http://lakefs:8000", "http://example.test:18001", "lakefs"); got != "http://127.0.0.1:18000" {
		t.Fatalf("already-public origin rewritten: %q", got)
	}
	if got := publishedOrigin("http://lakefs:8000", "http://lakefs:8000", "http://user:secret@127.0.0.1:18001", "lakefs"); got != "http://lakefs:8000" {
		t.Fatalf("secret public URL must not be published: %q", got)
	}
}
