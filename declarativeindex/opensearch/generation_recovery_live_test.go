package opensearch_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"kc/declarativeindex/control"
	"kc/declarativeindex/internal/testfixture"
	"kc/declarativeindex/opensearch"
	query "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/internal/testkit"
	"kc/kernel"
	sourcebridge "kc/liveaccess/discovery"
	objectmodel "kc/object"
	"kc/snapshot"
)

func TestLiveOpenSearchControllerRecoversGenerationLossWithReadyControl(t *testing.T) {
	endpoint := strings.TrimRight(os.Getenv("KC_TEST_OPENSEARCH_URL"), "/")
	if endpoint == "" {
		t.Fatal("KC_TEST_OPENSEARCH_URL must name an existing OpenSearch environment")
	}
	t.Setenv(opensearch.EnvEmbeddingModel, "")
	for _, lane := range []string{"snapshot", "state"} {
		t.Run(lane, func(t *testing.T) {
			namespace := "generation-loss-" + string(kernel.CanonicalDigest(t.TempDir()))[:20]
			media := &generationMedia{endpoint: endpoint, control: "kc-projection-control-v1-" + string(kernel.CanonicalDigest(namespace))[:24], client: &http.Client{Timeout: 10 * time.Second}, indices: map[string]bool{}}
			t.Cleanup(func() { media.cleanup(t) })
			var repo objectmodel.Repository
			var commit kernel.CommitID
			if lane == "snapshot" {
				repo, _, commit = testfixture.CommittedSearchablePolicy(t)
			} else {
				repo, commit, _ = testfixture.StateProjection(t)
			}
			idx := query.NewIndexEngine(t.TempDir(), opensearch.Open(opensearch.Config{URL: endpoint, Namespace: namespace, PrimaryShards: 1}))
			controller, err := control.NewController(idx, control.NewTargetStore(filepath.Join(t.TempDir(), "controller.db")), func(id kernel.RepositoryID) (objectmodel.Repository, error) {
				if id != repo.ID() {
					return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "unknown test source")
				}
				return repo, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { controller.Close(); _ = idx.Close() })
			controller.SetInventory(func() ([]kernel.RepositoryID, error) { return []kernel.RepositoryID{repo.ID()}, nil })
			lookup := &testfixture.StateLookup{Value: map[string]any{"status": "running"}, Basis: objectmodel.ObservationBasis{
				BindingGeneration: "g1", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "r1", ObservedAt: "2026-08-27T00:00:00Z",
			}}
			source := sourcebridge.Source{Lookup: lookup}
			if lane == "state" {
				controller.SetObservationSource(source)
			}
			if err := controller.CatchUp(context.Background()); err != nil {
				t.Fatal(err)
			}
			assertReady := func() {
				targets, err := controller.Targets()
				if err != nil || len(targets) != 1 || targets[0].Status != ports.TargetReady || targets[0].AppliedCommit != commit {
					t.Fatalf("durable target was not READY at fixed source: targets=%#v error=%v", targets, err)
				}
			}
			assertReady()
			request := retrieval.SearchOf(retrieval.SearchMATCH("runbook"))
			if lane == "state" {
				request = retrieval.SearchOf(retrieval.SearchClause{Op: retrieval.OpEQ, Field: &retrieval.FieldRef{Schema: "schema/job.runtime", Aspect: "runtime", Path: "status"}, Value: "running"})
			}
			search := func() (retrieval.SearchResult, error) {
				if lane == "state" {
					return idx.SearchStateAt(repo, commit, request)
				}
				return idx.SearchAt(repo, commit, request)
			}
			before, err := search()
			if err != nil || before.Completeness != retrieval.CompletenessComplete || len(before.Hits) != 1 {
				t.Fatalf("initial actual query: result=%#v error=%v", before, err)
			}
			records := media.records(t)
			var lost generationControl
			for _, record := range records {
				if record.Source.Basis == string(commit) && strings.HasSuffix(record.Source.Repository, "#state") == (lane == "state") {
					lost = record
				}
			}
			if lost.Source.ActiveIndex == "" || lost.Source.State != ports.ProjectionStateReady {
				t.Fatalf("could not locate owned ready generation: %#v", records)
			}
			originalControl := media.request(t, http.MethodGet, media.control+"/_doc/"+lost.ID)
			media.request(t, http.MethodDelete, lost.Source.ActiveIndex)
			if current := media.request(t, http.MethodGet, media.control+"/_doc/"+lost.ID); !reflect.DeepEqual(current, originalControl) {
				t.Fatal("generation-only fault changed its control document")
			}
			assertReady()
			calls := lookup.Calls
			if result, err := search(); err == nil {
				t.Errorf("lost generation was served: %#v", result)
			}
			if lookup.Calls != calls {
				t.Fatal("query attempted external State repair")
			}
			// No Desire, Notify, source commit, or manual sync precedes automatic reconciliation.
			if err := controller.CatchUp(context.Background()); err != nil {
				t.Errorf("maintenance did not recover actual generation: %v", err)
			}
			media.records(t)
			recovered, err := search()
			if err != nil {
				t.Fatalf("actual query after automatic reconciliation: %v", err)
			}
			assertReady()
			before.Stats, recovered.Stats = retrieval.SearchExecutionStats{}, retrieval.SearchExecutionStats{}
			if !reflect.DeepEqual(recovered, before) {
				t.Fatalf("recovery changed knowledge, evidence, or fixed view: before=%#v after=%#v", before, recovered)
			}
			if lane == "state" {
				_, err = idx.RefreshState(context.Background(), repo, commit, source)
			} else {
				_, err = idx.Rebuild(repo, commit)
			}
			if err != nil {
				t.Fatal(err)
			}
			media.records(t)
			rebuilt, err := search()
			rebuilt.Stats = retrieval.SearchExecutionStats{}
			if err != nil || !reflect.DeepEqual(recovered, rebuilt) {
				t.Fatalf("automatic recovery differs from actual rebuild: recovered=%#v rebuilt=%#v error=%v", recovered, rebuilt, err)
			}
			if head := testkit.MustHead(t, repo, snapshot.DefaultRef); head != commit {
				t.Fatal("derived recovery changed source HEAD")
			}
		})
	}
}

type generationControl struct {
	ID     string `json:"_id"`
	Source struct {
		Repository  string `json:"repository"`
		ActiveIndex string `json:"active_index"`
		Basis       string `json:"basis"`
		State       string `json:"state"`
	} `json:"_source"`
}
type generationMedia struct {
	endpoint, control string
	client            *http.Client
	indices           map[string]bool
}

func (m *generationMedia) request(t *testing.T, method, path string) []byte {
	t.Helper()
	request, err := http.NewRequest(method, m.endpoint+"/"+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := m.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode >= 300 {
		t.Fatalf("owned physical resource %s %s: status=%d error=%v body=%s", method, path, response.StatusCode, err, body)
	}
	return body
}
func (m *generationMedia) records(t *testing.T) []generationControl {
	t.Helper()
	var response struct {
		Hits struct {
			Hits []generationControl `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(m.request(t, http.MethodGet, m.control+"/_search?size=100"), &response); err != nil {
		t.Fatal(err)
	}
	for _, record := range response.Hits.Hits {
		if record.Source.ActiveIndex != "" {
			m.indices[record.Source.ActiveIndex] = true
		}
	}
	return response.Hits.Hits
}
func (m *generationMedia) cleanup(t *testing.T) {
	t.Helper()
	m.indices[m.control] = true
	for path := range m.indices {
		request, err := http.NewRequest(http.MethodDelete, m.endpoint+"/"+path, nil)
		if err != nil {
			t.Error(err)
			continue
		}
		response, err := m.client.Do(request)
		if err != nil {
			t.Errorf("cleanup exact owned resource %s: %v", path, err)
			continue
		}
		response.Body.Close()
		if response.StatusCode >= 300 && response.StatusCode != http.StatusNotFound {
			t.Errorf("cleanup exact owned resource %s: HTTP %d", path, response.StatusCode)
		}
	}
}
