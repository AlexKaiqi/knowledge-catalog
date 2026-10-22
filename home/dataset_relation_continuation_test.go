package home

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	retrieval "kc/discovery"
	discoverydatasets "kc/discovery/datasets"
	"kc/discovery/space"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

func TestDatasetRelationContinuationKeepsSignedOriginalCorpusAcrossRepairAndColdReopen(t *testing.T) {
	if input := os.Getenv("KC_TEST_RELATION_CURSOR_PROCESS_INPUT"); input != "" {
		verifyRelationCursorChild(t, input)
		return
	}

	ws, plain, _ := newDatasetPreparationHome(t, nil)
	var operations []objectmodel.Operation
	for _, name := range []string{"one", "two", "three"} {
		id := objectmodel.ObjectID("relation/" + name)
		operations = append(operations, objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindRelation, ObjectID: id}, PathHint: "object/relation-" + name + ".yaml", Value: map[string]any{
			"relationId": string(id), "relationType": "owned-by", "direction": "DIRECTED", "endpoints": []any{
				map[string]any{"role": "subject", "object": "article/plain"}, map[string]any{"role": "owner", "object": "team/finance"},
			},
		}})
	}
	written, err := ws.Writer.Commit("seed-relations-for-cold-pages", objectmodel.CommitChangeSet{TargetRepository: plain.id, TargetRef: snapshot.DefaultRef, BaseCommit: plain.at, ExpectedTargetCommit: plain.at, Operations: operations})
	if err != nil {
		t.Fatal(err)
	}
	plain.at = written.Result.NewCommit
	def, err := datasetPreparationPublisher(ws).Execute(context.Background(), discoverydatasets.DatasetPublication{Dataset: "fixed-relation-pages", Revision: 1, Sources: []dataset.DatasetSource{plain.mapping()}})
	if err != nil {
		t.Fatal(err)
	}
	query := retrieval.RelationQuery{Endpoint: "article/plain", RelationType: "owned-by", Role: "subject"}
	readPage := func(host *Home, request retrieval.RelationPageRequest) (retrieval.RelationPage, error) {
		return readDatasetRelationPage(host, def.SetID, plain.id, plain.at, request)
	}
	first, err := readPage(ws, retrieval.RelationPageRequest{Query: query, Limit: 1})
	if err != nil || len(first.Hits) != 1 || first.Continuation == "" {
		t.Fatalf("initial relation page: %#v %v", first, err)
	}
	base, id, binding := publicationBindingOnMedium(t, ws, def.SetID)
	if first.Generation != binding["generation"] {
		t.Fatal("relation page did not bind accepted Dataset corpus")
	}
	remove := func() {
		t.Helper()
		if status, response := publicationMediumJSON(t, http.MethodDelete, base+"/_doc/"+url.PathEscape(id), nil); status >= 300 {
			t.Fatalf("remove publication mapping: %d %#v", status, response)
		}
	}
	remove()
	if err := ws.Plugins.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, _, replacement := publicationBindingOnMedium(t, ws, def.SetID)
	if replacement["generation"] == binding["generation"] {
		t.Fatal("maintenance did not replace relation generation")
	}
	remove()
	cfg := *ws.Deployment
	cfg.CacheDir = t.TempDir()
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	verifyRelationCursorInAnotherProcess(t, relationCursorProcessInput{Config: cfg, SetID: def.SetID, Source: plain.id, At: plain.at, Query: query, Token: first.Continuation, Generation: first.Generation, First: first.Hits[0].ObjectID})
	reopened, err := OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if err := reopened.VerifyIndexPlugins(); err != nil {
		t.Fatal(err)
	}
	altered := query
	altered.RelationType = "different"
	if _, err := readPage(reopened, retrieval.RelationPageRequest{Query: altered, Limit: 1, Continuation: first.Continuation}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("cursor changed relation query: %v", err)
	}
	outer, err := retrieval.DecodeContinuation(first.Continuation)
	if err != nil {
		t.Fatal(err)
	}
	signed, err := base64.RawURLEncoding.DecodeString(outer.Position)
	if err != nil {
		t.Fatal(err)
	}
	signed[0] ^= 1
	outer.Position = base64.RawURLEncoding.EncodeToString(signed)
	if _, err := readPage(reopened, retrieval.RelationPageRequest{Query: query, Limit: 1, Continuation: retrieval.EncodeContinuation(outer)}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("recomputed public wrapper bypassed relation signature: %v", err)
	}
	seen := map[objectmodel.ObjectID]bool{first.Hits[0].ObjectID: true}
	token := first.Continuation
	for token != "" {
		page, err := readPage(reopened, retrieval.RelationPageRequest{Query: query, Limit: 1, Continuation: token})
		if err != nil || page.Generation != first.Generation || (len(page.Hits) == 0 && !page.Exhausted) {
			t.Fatalf("cold relation continuation changed corpus: %#v %v", page, err)
		}
		for _, hit := range page.Hits {
			if seen[hit.ObjectID] {
				t.Fatalf("relation repeated %s", hit.ObjectID)
			}
			seen[hit.ObjectID] = true
		}
		token = page.Continuation
	}
	if len(seen) != 3 {
		t.Fatalf("relation continuation escaped fixed corpus: %#v", seen)
	}
	for _, id := range []objectmodel.ObjectID{"relation/one", "relation/two", "relation/three"} {
		if !seen[id] {
			t.Fatalf("cold relation continuation lost fixed content: %#v", seen)
		}
	}
	if status, _ := publicationMediumJSON(t, http.MethodGet, base+"/_doc/"+url.PathEscape(id), nil); status != http.StatusNotFound {
		t.Fatal("relation continuation recreated mutable mapping")
	}
	digest := sha256.Sum256([]byte(binding["repository"].(string)))
	keyPath := base + "/_doc/" + hex.EncodeToString(digest[:]) + "-cursor-key"
	if status, _ := publicationMediumJSON(t, http.MethodDelete, keyPath, nil); status >= 300 {
		t.Fatalf("remove cursor signing fixture: %d", status)
	}
	if _, err := readPage(reopened, retrieval.RelationPageRequest{Query: query, Limit: 1, Continuation: first.Continuation}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("missing key invented a new verifier: %v", err)
	}
	if status, _ := publicationMediumJSON(t, http.MethodGet, keyPath, nil); status != http.StatusNotFound {
		t.Fatal("verification recreated missing signing key")
	}
}

func readDatasetRelationPage(host *Home, setID string, source kernel.RepositoryID, at kernel.CommitID, request retrieval.RelationPageRequest) (retrieval.RelationPage, error) {
	resolved, err := host.Datasets.ResolveDataset(setID)
	if err != nil {
		return retrieval.RelationPage{}, err
	}
	member, err := space.Member(func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return host.Reader.Require(id, kernel.ErrObjectRefUnresolved)
	}, DatasetPin(resolved), source)
	if err != nil {
		return retrieval.RelationPage{}, err
	}
	return host.Plugins.RelationsAtContext(context.Background(), member, at, request)
}

type relationCursorProcessInput struct {
	Config            DeploymentConfig
	SetID             string
	Source            kernel.RepositoryID
	At                kernel.CommitID
	Query             retrieval.RelationQuery
	Token, Generation string
	First             objectmodel.ObjectID
	Output            string
}

func verifyRelationCursorInAnotherProcess(t *testing.T, input relationCursorProcessInput) {
	t.Helper()
	directory := t.TempDir()
	input.Output = filepath.Join(directory, "result.json")
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "input.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run", "^TestDatasetRelationContinuationKeepsSignedOriginalCorpusAcrossRepairAndColdReopen$", "-test.count=1")
	command.Env = append(os.Environ(), "KC_TEST_RELATION_CURSOR_PROCESS_INPUT="+path)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("fresh-process continuation failed: %v\n%s", err, output)
	}
	raw, err = os.ReadFile(input.Output)
	if err != nil {
		t.Fatal(err)
	}
	var ids []objectmodel.ObjectID
	if err := json.Unmarshal(raw, &ids); err != nil {
		t.Fatal(err)
	}
	seen := map[objectmodel.ObjectID]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	if len(ids) != 3 || !seen["relation/one"] || !seen["relation/two"] || !seen["relation/three"] {
		t.Fatalf("child did not prove exact fixed corpus: %#v", ids)
	}
}

func verifyRelationCursorChild(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var input relationCursorProcessInput
	if err := json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	host, err := OpenDeployment(input.Config)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	if err := host.VerifyIndexPlugins(); err != nil {
		t.Fatal(err)
	}
	ids := []objectmodel.ObjectID{input.First}
	seen := map[objectmodel.ObjectID]bool{input.First: true}
	token := input.Token
	for token != "" {
		page, err := readDatasetRelationPage(host, input.SetID, input.Source, input.At, retrieval.RelationPageRequest{Query: input.Query, Limit: 1, Continuation: token})
		if err != nil || page.Generation != input.Generation || (len(page.Hits) == 0 && !page.Exhausted) {
			t.Fatalf("new process lost signed original basis: %#v %v", page, err)
		}
		for _, hit := range page.Hits {
			if seen[hit.ObjectID] {
				t.Fatalf("child repeated %s", hit.ObjectID)
			}
			seen[hit.ObjectID] = true
			ids = append(ids, hit.ObjectID)
		}
		token = page.Continuation
	}
	raw, err = json.Marshal(ids)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(input.Output, raw, 0600); err != nil {
		t.Fatal(err)
	}
}
