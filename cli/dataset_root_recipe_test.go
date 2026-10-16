package cli_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"testing"

	inv "kc/invocation"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
)

// dataset.md publication and service.md U1: the explicit root mapping must use
// the same authorized Writer and frozen publication through the product CLI.
func TestDatasetRootRecipeCLIWriteAndFrozenRead(t *testing.T) {
	f := newDirectoryDelivery(t)
	result := asMap(t, body(t, kcRemote(t, f.serverURL, "agent:publisher", "dataset", "define",
		"--dataset", "root-files", "--revision", "1", "--source", deliveryDocs+"=refs/heads/main@")))
	if result["recipeFile"] != dataset.DatasetFileName || result["recipeRepository"] != deliveryDocs ||
		result["recipeLocation"] != "repository" || result["recipeCommit"] == "" || result["recipeSkipped"] != nil {
		t.Fatalf("root publication did not return its Writer result: %#v", result)
	}
	read := func(path string) map[string]any {
		return asMap(t, body(t, kcRemote(t, f.serverURL, "agent:publisher", "files", "read",
			"--dataset", "root-files", "--path", path)))
	}
	recipeRead := read(dataset.DatasetFileName)
	raw, err := base64.StdEncoding.DecodeString(recipeRead["content"].(string))
	if err != nil {
		t.Fatal(err)
	}
	recipe, err := dataset.ParseDatasetRecipe(raw)
	if err != nil || recipe.Name != "root-files" || len(recipe.Sources()) != 1 ||
		recipe.Sources()[0].Path == nil || *recipe.Sources()[0].Path != "" || recipeRead["commit"] != result["recipeCommit"] {
		t.Fatalf("frozen root recipe differs from its receipt: %#v %#v %v", recipe, recipeRead, err)
	}
	before := read("handbook/policies/README.md")
	f.write(t, deliveryDocs, "root-new-source", []snapshot.TreeChange{{Path: "handbook/policies/README.md", Content: []byte("policy v2\n")}})
	after := read("handbook/policies/README.md")
	content, err := base64.StdEncoding.DecodeString(after["content"].(string))
	if err != nil || string(content) != "policy v1\n" || !reflect.DeepEqual(before, after) {
		t.Fatalf("source advancement changed the accepted root publication: before=%#v after=%#v %v", before, after, err)
	}
}

func TestDatasetRootRecipeHTTPKeepsPublicationAndWriteGrantsSeparate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		actions []string
		status  int
		skipped bool
	}{
		{"no source permit", []string{"dataset.manage", "writer.commit"}, http.StatusForbidden, false},
		{"no target management", []string{access.RepositoryDatasetPublish, "writer.commit"}, http.StatusForbidden, false},
		{"no source write", []string{"dataset.manage", access.RepositoryDatasetPublish}, http.StatusOK, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDirectoryDelivery(t)
			policy, err := inv.ReadAllow(f.cfg.StateDir)
			if err != nil {
				t.Fatal(err)
			}
			for _, action := range tc.actions {
				rule := access.Rule{ID: "root-" + action, Principal: "agent:root-publisher", Actions: []string{action}}
				if action == "dataset.manage" {
					rule.Dataset = "delivery"
				} else {
					rule.Repo = deliveryDocs
				}
				policy.Rules = append(policy.Rules, rule)
			}
			if err := inv.WriteAllow(f.cfg.StateDir, policy); err != nil {
				t.Fatal(err)
			}
			before := f.mounts(t, nil)
			status, payload := rootRecipeHTTP(t, f.serverURL, map[string]any{"dataset": "delivery", "revision": 2,
				"sources": []dataset.DatasetSource{{Repository: deliveryDocs, Selector: snapshot.DefaultRef, Path: rootRecipePath()}}}, "agent:root-publisher")
			if status != tc.status {
				t.Fatalf("publication status=%d want=%d payload=%#v", status, tc.status, payload)
			}
			if status == http.StatusForbidden {
				if asMap(t, asMap(t, payload)["error"])["code"] != string(kernel.ErrForbidden) || !reflect.DeepEqual(f.mounts(t, nil), before) {
					t.Fatalf("denied publication changed the accepted release: %#v", payload)
				}
				return
			}
			result := asMap(t, payload)
			if tc.skipped && (result["recipeSkipped"] != "no write grant for root mount" || result["recipeCommit"] != nil) {
				t.Fatalf("publication borrowed a source write grant: %#v", result)
			}
		})
	}
}

func rootRecipePath() *string {
	root := ""
	return &root
}

func rootRecipeHTTP(t *testing.T, serverURL string, payload any, principal string) (int, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, serverURL+"/snapshot/v1/datasets", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Kc-As", principal)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err = io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, result
}
