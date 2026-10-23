package home

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	discoverydatasets "kc/discovery/datasets"
	"kc/kernel"
	"kc/snapshot/dataset"
)

func TestDatasetPublicationRetainsExactAcceptedResourceReferenceAcrossHostReopen(t *testing.T) {
	ws, plain, indexed := newDatasetPreparationHome(t, nil)
	def, err := datasetPreparationPublisher(ws).Execute(context.Background(), discoverydatasets.DatasetPublication{
		Dataset: "protected", Revision: 1, Sources: []dataset.DatasetSource{plain.mapping(), indexed.mapping()}, Metadata: map[string]string{"purpose": "resource-handoff"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Acceptance builds nothing, so the static corpus reference belongs to the
	// release's own build, which adopts the already accepted version.
	buildDatasetCorpus(t, ws)
	config := *ws.Deployment
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenDeployment(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	accepted, err := reopened.Datasets.DatasetVersion("protected", 1)
	if err != nil || !reflect.DeepEqual(accepted, def) {
		t.Fatalf("accepted Dataset was not recovered: %#v %v", accepted, err)
	}
	cfg := reopened.Stores.OpenSearch
	control := "kc-projection-control-v1-" + string(kernel.CanonicalDigest(cfg.Namespace)[:24])
	response, err := http.Post(strings.TrimRight(cfg.URL, "/")+"/"+control+"/_search", "application/json", strings.NewReader(`{"size":100,"query":{"term":{"state":"RESOURCE"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result struct {
		Hits struct {
			Hits []struct {
				Source struct {
					Resource struct {
						Claims []struct {
							Phase    string `json:"phase"`
							Evidence []byte `json:"evidence"`
						} `json:"claims"`
					} `json:"resource"`
				} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("resource authority unreadable: status=%d err=%v", response.StatusCode, err)
	}
	found := 0
	for _, hit := range result.Hits.Hits {
		for _, claim := range hit.Source.Resource.Claims {
			var proof struct {
				Version    int                          `json:"version"`
				Admission  string                       `json:"admission"`
				Authority  dataset.PublicationAuthority `json:"authority"`
				Definition dataset.Dataset              `json:"definition"`
			}
			if err := json.Unmarshal(claim.Evidence, &proof); err != nil {
				t.Fatal(err)
			}
			if proof.Definition.SetID != "protected" {
				continue
			}
			found++
			if claim.Phase != "ACCEPTED" || proof.Version != 2 || proof.Admission == "" || !reflect.DeepEqual(proof.Definition, def) || proof.Authority.Head == "" || proof.Authority.Binding == "" {
				t.Fatalf("resource reference lost exact adopted candidate or authority: phase=%s proof=%#v expected=%#v", claim.Phase, proof, def)
			}
			receipt, err := ws.Datasets.(dataset.PublicationCoordinator).PublicationReceipt(context.Background(), dataset.PublicationLookup{SetID: "protected", Revision: 1})
			if err != nil || receipt.Admission != proof.Admission || receipt.Authority != proof.Authority || receipt.Definition != kernel.CanonicalDigest(def) {
				t.Fatalf("build admission is not the one recorded for the accepted release: receipt=%#v proof=%#v err=%v", receipt, proof, err)
			}
		}
	}
	if found != 1 {
		t.Fatalf("want one durable static corpus reference, got %d", found)
	}
}
