package home

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	discoverydatasets "kc/discovery/datasets"
	"kc/kernel"
	"kc/snapshot/dataset"
)

type loseDatasetResourceHandoff struct {
	discoverydatasets.PublicationProtection
	beforePublish bool
	err           error
}

func (r loseDatasetResourceHandoff) Admit(ctx context.Context, proof []byte) error {
	if err := r.PublicationProtection.Admit(ctx, proof); err != nil {
		return err
	}
	if r.beforePublish {
		return r.err
	}
	return nil
}
func (r loseDatasetResourceHandoff) Confirm(context.Context) error { return r.err }

type datasetResourceRecoveryHit struct {
	ID          string         `json:"_id"`
	SeqNo       int64          `json:"_seq_no"`
	PrimaryTerm int64          `json:"_primary_term"`
	Source      map[string]any `json:"_source"`
}

func expireDatasetResourceAdmission(t *testing.T, ws *Home) (string, string, string) {
	t.Helper()
	cfg := ws.Stores.OpenSearch
	control := "kc-projection-control-v1-" + string(kernel.CanonicalDigest(cfg.Namespace)[:24])
	base := strings.TrimRight(cfg.URL, "/") + "/" + control
	response, err := http.Post(base+"/_search", "application/json", strings.NewReader(`{"size":100,"seq_no_primary_term":true,"query":{"term":{"state":"RESOURCE"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Hits struct {
			Hits []datasetResourceRecoveryHit `json:"hits"`
		} `json:"hits"`
	}
	decoder := json.NewDecoder(response.Body)
	decoder.UseNumber()
	err = decoder.Decode(&result)
	_ = response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("enumerate actual resource authority: status=%d err=%v", response.StatusCode, err)
	}
	for _, hit := range result.Hits.Hits {
		resource, ok := hit.Source["resource"].(map[string]any)
		if !ok {
			continue
		}
		claims, ok := resource["claims"].([]any)
		if !ok {
			continue
		}
		for _, value := range claims {
			claim := value.(map[string]any)
			if claim["phase"] != "ADMITTED" {
				continue
			}
			claim["until"] = 1 // Test clock crosses the existing finite recovery deadline.
			raw, err := json.Marshal(hit.Source)
			if err != nil {
				t.Fatal(err)
			}
			path := base + "/_doc/" + url.PathEscape(hit.ID)
			request, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s?if_seq_no=%d&if_primary_term=%d&refresh=true", path, hit.SeqNo, hit.PrimaryTerm), bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			_ = response.Body.Close()
			if response.StatusCode >= 300 {
				t.Fatalf("test deadline injection rejected: HTTP %d", response.StatusCode)
			}
			return path, hit.Source["active_index"].(string), claim["evidence"].(string)
		}
	}
	t.Fatal("actual publication admission is missing")
	return "", "", ""
}

// claimAdmission decodes the admission a durable ticket was admitted under, so
// a retry can be checked against the ticket it actually left behind.
func claimAdmission(t *testing.T, evidence string) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(evidence)
	if err != nil {
		t.Fatal(err)
	}
	var proof struct {
		Admission string `json:"admission"`
	}
	if err := json.Unmarshal(raw, &proof); err != nil {
		t.Fatal(err)
	}
	return proof.Admission
}

func readDatasetResourceClaims(t *testing.T, path string) ([]any, string) {
	t.Helper()
	response, err := http.Get(path)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result struct {
		Source struct {
			Resource struct {
				Claims []any `json:"claims"`
			} `json:"resource"`
		} `json:"_source"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("read resource after recovery: %d %v", response.StatusCode, err)
	}
	if len(result.Source.Resource.Claims) == 0 {
		return nil, ""
	}
	claim := result.Source.Resource.Claims[0].(map[string]any)
	return result.Source.Resource.Claims, claim["phase"].(string)
}

func TestDatasetResourceRecoveryUsesActualAuthorityAcrossHostReopen(t *testing.T) {
	for _, phase := range []string{"confirmation-lost", "admission-lost"} {
		t.Run(phase, func(t *testing.T) {
			ws, plain, _ := newDatasetPreparationHome(t, nil)
			lost := errors.New("handoff response lost after actual durable admission")
			if _, err := datasetPreparationPublisher(ws).Execute(context.Background(), discoverydatasets.DatasetPublication{Dataset: "uncertain", Revision: 1, Sources: []dataset.DatasetSource{plain.mapping()}}); err != nil {
				t.Fatalf("publication must accept without waiting for a corpus: %v", err)
			}
			// Acceptance already happened; only the build's admission is lost, so
			// the release stays accepted and becomes ready on its own retry.
			err := buildDatasetRelease(ws, dataset.PublicationLookup{SetID: "uncertain", Revision: 1}, func(ticket discoverydatasets.PublicationProtection) discoverydatasets.PublicationProtection {
				return loseDatasetResourceHandoff{PublicationProtection: ticket, beforePublish: phase == "admission-lost", err: lost}
			})
			if !errors.Is(err, lost) {
				t.Fatalf("did not cross uncertain handoff: %v", err)
			}
			if _, err := ws.Datasets.DatasetVersion("uncertain", 1); err != nil {
				t.Fatalf("lost build admission reverted the accepted release: %v", err)
			}
			path, physical, proof := expireDatasetResourceAdmission(t, ws)
			config := *ws.Deployment
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
			// No Dataset query has reopened its corpus. A fresh instance maintenance
			// pass must find its durable ticket and consult the actual file authority.
			oldHead, err := reopened.Registry.Head()
			if err != nil {
				t.Fatal(err)
			}
			fenceLost := errors.New("actual fence accepted but evidence failed")
			reopened.Datasets.SetObserver(func(event string, _ map[string]any, prior error) error {
				if event == "dataset-publication-fence" {
					return fenceLost
				}
				return prior
			})
			if err := reopened.Plugins.CatchUp(context.Background()); !errors.Is(err, fenceLost) {
				t.Fatalf("unknown authority fence was treated as complete: %v", err)
			}
			claims, current := readDatasetResourceClaims(t, path)
			if len(claims) != 1 || current != "ADMITTED" || claims[0].(map[string]any)["evidence"] != proof {
				t.Fatalf("unknown authority fence released protection: %#v", claims)
			}
			reopened.Datasets.SetObserver(nil)
			if err := reopened.Plugins.CatchUp(context.Background()); err != nil {
				t.Fatal(err)
			}
			if next, err := reopened.Registry.Head(); err != nil || next == oldHead {
				t.Fatalf("recovery did not establish an authority fence: %s %v", next, err)
			}
			claims, current = readDatasetResourceClaims(t, path)
			if phase == "admission-lost" {
				// The acceptance already happened, so the retry adopts the release
				// under a new admission. The stale claim is released and the new one
				// settles as ACCEPTED once the corpus exists; what must not happen is
				// the old ticket surviving beside it.
				receipt, err := reopened.Datasets.(dataset.PublicationCoordinator).PublicationReceipt(context.Background(), dataset.PublicationLookup{SetID: "uncertain", Revision: 1})
				if err != nil || receipt.Admission == "" || receipt.Admission == proof {
					t.Fatalf("retry did not replace the lost admission: %#v %v", receipt, err)
				}
				if len(claims) != 1 || current != "ACCEPTED" {
					t.Fatalf("retry did not settle the release's own ticket: %#v", claims)
				}
				if claims[0].(map[string]any)["evidence"] == proof {
					t.Fatalf("stale unadopted ticket survived the retry: %#v", claims)
				}
				if admission := claimAdmission(t, claims[0].(map[string]any)["evidence"].(string)); admission != receipt.Admission {
					t.Fatalf("retained ticket belongs to another admission: %q != %q", admission, receipt.Admission)
				}
				return
			}
			if len(claims) != 1 || current != "ACCEPTED" || claims[0].(map[string]any)["evidence"] != proof {
				t.Fatalf("accepted resource lost its exact durable proof: %#v", claims)
			}
			if _, err := reopened.Datasets.DatasetVersion("uncertain", 1); err != nil {
				t.Fatal(err)
			}
			request, _ := http.NewRequest(http.MethodHead, strings.TrimRight(reopened.Stores.OpenSearch.URL, "/")+"/"+physical, nil)
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			_ = response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("reference settlement deleted physical corpus without lifecycle authority: %d", response.StatusCode)
			}
		})
	}
}

type gateDatasetResourceAdmission struct {
	discoverydatasets.PublicationProtection
	proof   chan<- []byte
	release <-chan struct{}
}

func (r gateDatasetResourceAdmission) Admit(ctx context.Context, proof []byte) error {
	if err := r.PublicationProtection.Admit(ctx, proof); err != nil {
		return err
	}
	r.proof <- append([]byte(nil), proof...)
	select {
	case <-r.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestEqualDatasetPreparationsRecoverOnlyTheActualAcceptedTicket(t *testing.T) {
	ws, plain, _ := newDatasetPreparationHome(t, nil)
	if _, err := datasetPreparationPublisher(ws).Execute(context.Background(), discoverydatasets.DatasetPublication{Dataset: "contended", Revision: 1, Sources: []dataset.DatasetSource{plain.mapping()}}); err != nil {
		t.Fatal(err)
	}
	var proofs [2]chan []byte
	var release [2]chan struct{}
	var done [2]chan error
	for i := range proofs {
		proofs[i], release[i], done[i] = make(chan []byte, 1), make(chan struct{}), make(chan error, 1)
		t.Cleanup(func() {
			select {
			case <-release[i]:
			default:
				close(release[i])
			}
		})
		go func() {
			done[i] <- buildDatasetRelease(ws, dataset.PublicationLookup{SetID: "contended", Revision: 1}, func(ticket discoverydatasets.PublicationProtection) discoverydatasets.PublicationProtection {
				return gateDatasetResourceAdmission{PublicationProtection: ticket, proof: proofs[i], release: release[i]}
			})
		}()
	}
	var raw [2][]byte
	var attempts [2]struct {
		Admission  string          `json:"admission"`
		Definition dataset.Dataset `json:"definition"`
	}
	for i := range raw {
		select {
		case raw[i] = <-proofs[i]:
		case err := <-done[i]:
			t.Fatalf("preparation stopped before admission: %v", err)
		case <-time.After(15 * time.Second):
			t.Fatal("preparation admission was not reached")
		}
		if err := json.Unmarshal(raw[i], &attempts[i]); err != nil {
			t.Fatal(err)
		}
	}
	if attempts[0].Admission == "" || attempts[0].Admission == attempts[1].Admission || !reflect.DeepEqual(attempts[0].Definition, attempts[1].Definition) {
		t.Fatalf("fixture did not prepare equal candidates with distinct tickets: %#v", attempts)
	}
	// Both attempts fence the authority before admitting, and each admission
	// carries the head its own fence produced. Whichever attempt commits last
	// therefore matches the current authority and is the one that settles; the
	// other one is excluded because its admission names a superseded head. That
	// makes the winner a property of the interleaving, not of this fixture's
	// slice order, so the winner is read off the results instead of assumed.
	close(release[0])
	close(release[1])
	var failures [2]error
	for i := range failures {
		failures[i] = <-done[i]
	}
	winner, loser := 0, 1
	if failures[0] != nil {
		winner, loser = 1, 0
	}
	if failures[winner] != nil || kernel.CodeOf(failures[loser]) != kernel.ErrNonFastForward {
		t.Fatalf("equal preparations were not settled to exactly one admission: %v / %v", failures[0], failures[1])
	}
	path, _, expired := expireDatasetResourceAdmission(t, ws)
	if expired != base64.StdEncoding.EncodeToString(raw[loser]) {
		t.Fatalf("expired ticket was not the excluded admission: got=%s want=%s", expired, base64.StdEncoding.EncodeToString(raw[loser]))
	}
	config := *ws.Deployment
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
	if err := reopened.Plugins.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	claims, _ := readDatasetResourceClaims(t, path)
	accepted := 0
	for _, claim := range claims {
		if claim.(map[string]any)["phase"] != "ACCEPTED" {
			continue
		}
		accepted++
		if claim.(map[string]any)["evidence"] != base64.StdEncoding.EncodeToString(raw[winner]) {
			t.Fatalf("equivalent candidate retained the loser or released the winner: %#v", claims)
		}
	}
	if accepted != 1 {
		t.Fatalf("want exactly one accepted ticket, got %d: %#v", accepted, claims)
	}
	receipt, err := reopened.Datasets.(dataset.PublicationCoordinator).PublicationReceipt(context.Background(), dataset.PublicationLookup{SetID: "contended", Revision: 1})
	if err != nil || receipt.Admission != attempts[winner].Admission {
		t.Fatalf("actual receipt does not identify the retained ticket: %#v %v", receipt, err)
	}
}
