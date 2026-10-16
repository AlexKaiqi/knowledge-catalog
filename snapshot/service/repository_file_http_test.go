package service_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"maps"
	"net/http"
	"testing"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot/access"
	"kc/snapshot/service"
)

const repositoryFileRoute = "/snapshot/v1/files/file:read"

func repositoryFileInput(repository kernel.RepositoryID) map[string]any {
	return map[string]any{"repository": repository, "commit": "c1", "path": "media/image.png"}
}

func TestRepositoryFileHTTPReadsFixedBytesWithoutDataset(t *testing.T) {
	f, source := newFileHTTPFixture(t)
	want := []byte{137, 80, 78, 71, 0, 255, 1}
	source.files["c1"]["media/image.png"] = want
	source.files["c2"]["media/image.png"] = []byte("replacement")
	source.head = "c2"
	f.resources.Datasets = nil
	f.rules.rules = []access.Rule{{Principal: "alice", Repo: string(source.ID()), Ref: "c1", Actions: []string{"file.read"}}}
	var evidence []service.FileReadEvidence
	f.resources.RecordRead = func(ctx context.Context, caller identity.HTTPIdentity, read service.FileReadEvidence) error {
		if caller.Principal != "alice" || service.ContextMetadata(ctx).Repository != string(source.ID()) || read.Pin.PinID != "" || read.Pin.SetID != "" {
			t.Fatal("raw file evidence lost caller/scope or invented a Dataset pin")
		}
		evidence = append(evidence, read)
		return nil
	}
	request := repositoryFileInput(source.ID())
	var content []byte
	for _, offset := range []int{0, 3, 6, 7, 99} {
		request["offset"], request["length"] = offset, 3
		result := datasetHTTPCall(t, f.handler(), http.MethodPost, repositoryFileRoute, request, http.StatusOK)
		chunk, err := base64.StdEncoding.DecodeString(result["content"].(string))
		if err != nil {
			t.Fatal(err)
		}
		start := min(offset, len(want))
		end := min(start+3, len(want))
		if len(result) != 7 || result["repository"] != string(source.ID()) || result["commit"] != "c1" || result["path"] != "media/image.png" || result["offset"] != float64(start) || result["totalBytes"] != float64(len(want)) || result["eof"] != (end == len(want)) || !bytes.Equal(chunk, want[start:end]) {
			t.Fatalf("fixed raw file result changed: %#v", result)
		}
		content = append(content, chunk...)
	}
	if !bytes.Equal(content, want) {
		t.Fatalf("binary chunks changed: %v", content)
	}
	request["path"], request["offset"] = "empty", 0
	source.files["c1"]["empty"] = nil
	result := datasetHTTPCall(t, f.handler(), http.MethodPost, repositoryFileRoute, request, http.StatusOK)
	if result["content"] != "" || result["eof"] != true || result["totalBytes"] != float64(0) {
		t.Fatalf("empty file is not empty base64: %#v", result)
	}
	if len(evidence) != 6 || evidence[0].Repository != source.ID() || evidence[0].Commit != "c1" || evidence[0].Path != "media/image.png" || evidence[0].Bytes != len(want) || evidence[0].Err != nil {
		t.Fatalf("raw read evidence lost its actual source: %#v", evidence)
	}
}

func TestRepositoryFileHTTPRechecksRawScopeBeforeSource(t *testing.T) {
	f, source := newFileHTTPFixture(t)
	source.files["c1"]["media/image.png"] = []byte("image")
	request := repositoryFileInput(source.ID())
	grant := access.Rule{Principal: "alice", Repo: string(source.ID()), Ref: "c1", Actions: []string{"file.read"}}
	f.rules.rules = []access.Rule{grant}
	datasetHTTPCall(t, f.handler(), http.MethodPost, repositoryFileRoute, request, http.StatusOK)
	for _, scope := range []string{"revoked", "dataset", "object", "aspect", "different-commit"} {
		t.Run(scope, func(t *testing.T) {
			rule := grant
			switch scope {
			case "dataset":
				rule.Dataset = "delivery"
			case "object":
				rule.Object = "*"
			case "aspect":
				rule.Aspect = "definition"
			case "different-commit":
				rule.Ref = "c2"
			case "revoked":
				rule.Principal = "other"
			}
			f.rules.rules = []access.Rule{rule}
			reads, inspections := source.reads, source.inspections
			datasetHTTPCall(t, f.handler(), http.MethodPost, repositoryFileRoute, request, http.StatusForbidden)
			if source.reads != reads || source.inspections != inspections {
				t.Fatal("denied raw read inspected the source")
			}
		})
	}
}

func TestRepositoryFileHTTPRejectsMixedAndInvalidCoordinates(t *testing.T) {
	f, source := newFileHTTPFixture(t)
	reads, inspections := source.reads, source.inspections
	invalid := []map[string]any{
		{"repository": ""}, {"commit": ""}, {"commit": " "}, {"path": ""},
		{"dataset": "delivery"}, {"pin": map[string]any{}}, {"pin": nil},
		{"view": "repository"}, {"mountPath": "media"}, {"file": "image.png"},
		{"path": "../private"}, {"path": "media/../private"}, {"path": "/media/image.png"},
		{"offset": -1}, {"length": -1}, {"length": 4<<20 + 1},
	}
	for _, change := range invalid {
		request := repositoryFileInput(source.ID())
		maps.Copy(request, change)
		result := datasetHTTPCall(t, f.handler(), http.MethodPost, repositoryFileRoute, request, http.StatusBadRequest)
		if result["error"].(map[string]any)["code"] != string(kernel.ErrUsageInvalid) {
			t.Fatalf("invalid coordinates returned wrong failure: %#v", result)
		}
	}
	if source.reads != reads || source.inspections != inspections {
		t.Fatal("invalid file coordinates reached source")
	}
	for _, route := range []string{"mounts:list", "tree:list"} {
		datasetHTTPCall(t, f.handler(), http.MethodPost, "/snapshot/v1/files/"+route, repositoryFileInput(source.ID()), http.StatusBadRequest)
	}
}

func TestRepositoryFileHTTPDistinguishesMissingVersionAndPath(t *testing.T) {
	f, source := newFileHTTPFixture(t)
	for _, row := range []struct {
		change map[string]any
		code   kernel.ErrorCode
	}{
		{map[string]any{"commit": "missing"}, kernel.ErrVersionUnresolved},
		{map[string]any{"commit": "main"}, kernel.ErrVersionUnresolved},
		{map[string]any{"commit": "refs/heads/main"}, kernel.ErrVersionUnresolved},
		{map[string]any{"commit": "HEAD"}, kernel.ErrVersionUnresolved},
		{map[string]any{"path": "missing"}, kernel.ErrObjectRefUnresolved},
	} {
		request := repositoryFileInput(source.ID())
		maps.Copy(request, row.change)
		result := datasetHTTPCall(t, f.handler(), http.MethodPost, repositoryFileRoute, request, http.StatusNotFound)
		if result["error"].(map[string]any)["code"] != string(row.code) {
			t.Fatalf("missing coordinate returned wrong failure: %#v", result)
		}
	}
	if source.reads != 1 {
		t.Fatalf("missing version reached file bytes: %d reads", source.reads)
	}
}
