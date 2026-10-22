package home

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	retrieval "kc/discovery"
	discoverydatasets "kc/discovery/datasets"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot/dataset"
)

func publicationMediumJSON(t *testing.T, method, address string, body any) (int, map[string]any) {
	t.Helper()
	var raw []byte
	var err error
	if body != nil {
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	request, err := http.NewRequest(method, address, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err = io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if len(raw) > 0 {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			t.Fatalf("read actual derived medium %s: %v", address, err)
		}
	}
	return response.StatusCode, value
}

func publicationBindingOnMedium(t *testing.T, ws *Home, setID string) (string, string, map[string]any) {
	t.Helper()
	cfg := ws.Stores.OpenSearch
	control := "kc-projection-control-v1-" + string(kernel.CanonicalDigest(cfg.Namespace)[:24])
	base := strings.TrimRight(cfg.URL, "/") + "/" + control
	status, response := publicationMediumJSON(t, http.MethodPost, base+"/_search", map[string]any{"size": 100, "query": map[string]any{"term": map[string]any{"state": "PUBLICATION_BINDING"}}})
	if status != http.StatusOK {
		t.Fatalf("binding enumeration: %d %#v", status, response)
	}
	for _, entry := range response["hits"].(map[string]any)["hits"].([]any) {
		hit := entry.(map[string]any)
		source := hit["_source"].(map[string]any)
		evidence, err := base64.StdEncoding.DecodeString(source["publication_evidence"].(string))
		if err != nil {
			t.Fatal(err)
		}
		var proof struct {
			Definition dataset.Dataset `json:"definition"`
		}
		if err := json.Unmarshal(evidence, &proof); err != nil {
			t.Fatal(err)
		}
		if proof.Definition.SetID == setID {
			return base, hit["_id"].(string), source
		}
	}
	t.Fatal("accepted publication did not have a durable physical binding")
	return "", "", nil
}

func replacePublicationPointerWithEmptyCorpus(t *testing.T, ws *Home, base string, binding map[string]any) string {
	t.Helper()
	endpoint := strings.TrimRight(ws.Stores.OpenSearch.URL, "/")
	original := binding["active_index"].(string)
	status, mapping := publicationMediumJSON(t, http.MethodGet, endpoint+"/"+original+"/_mapping", nil)
	if status != http.StatusOK {
		t.Fatalf("read original mapping: %d", status)
	}
	generation := string(kernel.CanonicalDigest(t.TempDir()))[:32]
	replacement := strings.TrimSuffix(original, binding["generation"].(string)) + generation
	status, created := publicationMediumJSON(t, http.MethodPut, endpoint+"/"+replacement, map[string]any{"settings": map[string]any{"index": map[string]any{"number_of_shards": 1, "number_of_replicas": 0}}, "mappings": mapping[original].(map[string]any)["mappings"]})
	if status >= 300 {
		t.Fatalf("create replacement test corpus: %d %#v", status, created)
	}
	digest := sha256.Sum256([]byte(binding["repository"].(string)))
	root := base + "/_doc/" + hex.EncodeToString(digest[:])
	status, current := publicationMediumJSON(t, http.MethodGet, root, nil)
	if status != http.StatusOK {
		t.Fatalf("read mutable test root: %d", status)
	}
	source := current["_source"].(map[string]any)
	source["active_index"], source["generation"], source["object_count"] = replacement, generation, 0
	address := root + "?if_seq_no=" + current["_seq_no"].(json.Number).String() + "&if_primary_term=" + current["_primary_term"].(json.Number).String() + "&refresh=true"
	status, updated := publicationMediumJSON(t, http.MethodPut, address, source)
	if status >= 300 {
		t.Fatalf("replace derived root: %d %#v", status, updated)
	}
	return root
}

func TestDatasetQueryUsesAcceptedGenerationAfterPointerReplacementAndColdReopen(t *testing.T) {
	ws, plain, _ := newDatasetPreparationHome(t, nil)
	def, err := datasetPreparationPublisher(ws).Execute(context.Background(), discoverydatasets.DatasetPublication{Dataset: "fixed-generation", Revision: 1, Sources: []dataset.DatasetSource{plain.mapping()}})
	if err != nil {
		t.Fatal(err)
	}

	assertQueryUsesPublishedSource(t, ws, def, plain)
	base, bindingID, binding := publicationBindingOnMedium(t, ws, def.SetID)
	root := replacePublicationPointerWithEmptyCorpus(t, ws, base, binding)
	assertQueryUsesPublishedSource(t, ws, def, plain)
	config := *ws.Deployment
	config.CacheDir = t.TempDir()
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenDeployment(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if err := reopened.VerifyIndexPlugins(); err != nil {
		t.Fatal(err)
	}
	assertQueryUsesPublishedSource(t, reopened, def, plain)
	status, deleted := publicationMediumJSON(t, http.MethodDelete, base+"/_doc/"+url.PathEscape(bindingID), nil)
	if status >= 300 {
		t.Fatalf("remove binding test medium: %d %#v", status, deleted)
	}
	resolved, err := reopened.Datasets.ResolveDataset(def.SetID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := reopened.Plugins.SearchDataset(context.Background(), func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return reopened.Reader.Require(id, kernel.ErrObjectRefUnresolved)
	}, DatasetPin(resolved), retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || len(result.Hits) != 0 {
		t.Fatalf("missing accepted binding fell back to mutable READY corpus: %#v %v", result, err)
	}
	if status, _ := publicationMediumJSON(t, http.MethodGet, base+"/_doc/"+url.PathEscape(bindingID), nil); status != http.StatusNotFound {
		t.Fatal("query rebuilt missing publication binding")
	}
	if status, current := publicationMediumJSON(t, http.MethodGet, root, nil); status != http.StatusOK || current["_source"].(map[string]any)["active_index"] == binding["active_index"] {
		t.Fatal("query repaired or changed the deliberately replaced root")
	}
	authority := reopened.Datasets.(dataset.PublicationCoordinator)
	key := dataset.PublicationLookup{SetID: def.SetID, Revision: def.Revision}
	before, err := authority.PublicationReceipt(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.Plugins.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertQueryUsesPublishedSource(t, reopened, def, plain)
	after, err := authority.PublicationReceipt(context.Background(), key)
	if err != nil || kernel.CanonicalDigest(before) != kernel.CanonicalDigest(after) {
		t.Fatalf("physical repair changed publication receipt: %#v %v", after, err)
	}
	actual, err := reopened.Datasets.DatasetVersion(def.SetID, def.Revision)
	if err != nil || kernel.CanonicalDigest(actual) != kernel.CanonicalDigest(def) {
		t.Fatalf("physical repair changed frozen Dataset: %#v %v", actual, err)
	}
	_, _, repaired := publicationBindingOnMedium(t, reopened, def.SetID)
	if repaired["generation"] == binding["generation"] {
		t.Fatal("maintenance reused old generation instead of rebuilding fixed sources")
	}
}

func assertQueryUsesPublishedSource(t *testing.T, host *Home, def dataset.Dataset, plain datasetPreparationSource) {
	t.Helper()
	resolved, err := host.Datasets.ResolveDataset(def.SetID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := host.Plugins.SearchDataset(context.Background(), func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return host.Reader.Require(id, kernel.ErrObjectRefUnresolved)
	}, DatasetPin(resolved), retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if err != nil || result.Completeness != retrieval.CompletenessComplete || len(result.Hits) != 1 || kernel.CanonicalDigest(result.Hits[0].Object) != kernel.CanonicalDigest(plain.canonical) {
		t.Fatalf("accepted query followed replacement corpus: %#v %v", result, err)
	}
}

func TestAcceptedDatasetQueryDoesNotWaitForLostResourceConfirmation(t *testing.T) {
	ws, plain, _ := newDatasetPreparationHome(t, nil)
	lost := errors.New("resource confirmation lost after Dataset accepted")
	publisher := datasetPreparationPublisher(ws)
	publisher.PrepareResources = func(ctx context.Context, def dataset.Dataset) (dataset.Dataset, discoverydatasets.PublicationProtection, error) {
		prepared, ticket, err := ws.DatasetPreparation.ExecuteProtected(ctx, def)
		if err != nil || ticket == nil {
			return prepared, ticket, err
		}
		return prepared, loseDatasetResourceHandoff{PublicationProtection: ticket, err: lost}, nil
	}
	_, err := publisher.Execute(context.Background(), discoverydatasets.DatasetPublication{Dataset: "unconfirmed-generation", Revision: 1, Sources: []dataset.DatasetSource{plain.mapping()}})
	if !errors.Is(err, lost) {
		t.Fatalf("handoff confirmation was not lost: %v", err)
	}
	def, err := ws.Datasets.DatasetVersion("unconfirmed-generation", 1)
	if err != nil {
		t.Fatal(err)
	}
	base, _, binding := publicationBindingOnMedium(t, ws, def.SetID)
	digest := sha256.Sum256([]byte(binding["repository"].(string)))
	path := base + "/_doc/" + hex.EncodeToString(digest[:]) + "-resource-" + binding["generation"].(string)
	_, before := readDatasetResourceClaims(t, path)
	if before != "ADMITTED" {
		t.Fatalf("test did not retain unknown confirmation: %s", before)
	}
	assertQueryUsesPublishedSource(t, ws, def, plain)
	config := *ws.Deployment
	config.CacheDir = t.TempDir()
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenDeployment(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if err := reopened.VerifyIndexPlugins(); err != nil {
		t.Fatal(err)
	}
	assertQueryUsesPublishedSource(t, reopened, def, plain)
	if _, after := readDatasetResourceClaims(t, path); after != "ADMITTED" {
		t.Fatal("query performed publication recovery or confirmation")
	}
}
