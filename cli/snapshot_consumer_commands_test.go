package cli_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	snapshotclient "kc/snapshot/client"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"kc/kernel"
	"kc/snapshot"
)

func TestSnapshotCLIConsumerFollowsPublishedFilesAcrossPagesAndVersions(t *testing.T) {
	f := newDirectoryDelivery(t)
	published := publishFileDelivery(t, f, 2, "reference/policies", "reference/guides", "data/metrics")
	wantFiles := map[string]struct {
		repository kernel.RepositoryID
		content    string
	}{
		"reference/policies/README.md": {deliveryDocs, "policy v1\n"},
		"reference/policies/metric.md": {deliveryData, "metric v1\n"},
	}
	inventory := asMap(t, body(t, kcRemote(t, f.serverURL, "agent:consumer", "show")))
	listed := inventory["datasets"].([]any)
	if len(listed) != 1 {
		t.Fatalf("consumer could not discover the published Dataset: %#v", inventory)
	}
	id := asMap(t, listed[0])["id"].(string)
	current := asMap(t, body(t, kcRemote(t, f.serverURL, "agent:consumer", "dataset", "show", id)))
	expectCode(t, kcRemote(t, f.serverURL, "agent:consumer", "dataset", "show", id, "--revision", "99"), "VERSION_UNRESOLVED")
	if after := asMap(t, body(t, kcRemote(t, f.serverURL, "agent:consumer", "dataset", "show", id))); !reflect.DeepEqual(after, current) {
		t.Fatalf("unpublished-version lookup changed the accepted Dataset: before=%#v after=%#v", current, after)
	}
	directory := asMap(t, current["sources"].([]any)[0])["path"].(string)
	first := asMap(t, body(t, kcRemote(t, f.serverURL, "agent:consumer", "files", "list", "--dataset", id, "--path", directory, "--limit", "1")))
	if first["revision"] != float64(2) || first["exhausted"] != false {
		t.Fatalf("first page lost its fixed publication: %#v", first)
	}
	f.write(t, deliveryDocs, "policy-v3", []snapshot.TreeChange{{Path: "handbook/policies/README.md", Content: []byte("policy v3\n")}})
	if err := f.publish(3, fileDeliverySources("new/policies", "new/guides", "new/metrics")); err != nil {
		t.Fatal(err)
	}
	expectCode(t, kcRemote(t, f.serverURL, "agent:consumer", "files", "list", "--dataset", id, "--revision", "3", "--path", directory, "--limit", "1", "--continuation", first["continuation"].(string)), "USAGE_INVALID")
	seen := map[string]bool{}
	page := first
	for i := 0; ; i++ {
		if i > 10 {
			t.Fatal("listing failed to progress")
		}
		entries := page["entries"].([]any)
		if len(entries) > 1 {
			t.Fatalf("mixed delivery exceeded the requested page: %#v", page)
		}
		for _, entry := range entries {
			path := entry.(map[string]any)["path"].(string)
			if seen[path] {
				t.Fatalf("repeated delivered path %s", path)
			}
			seen[path] = true
			read := asMap(t, body(t, kcRemote(t, f.serverURL, "agent:consumer", "files", "read", "--dataset", id, "--revision", "2", "--path", path)))
			content, err := base64.StdEncoding.DecodeString(read["content"].(string))
			want, selected := wantFiles[path]
			if err != nil || !selected || string(content) != want.content || read["revision"] != float64(2) || read["path"] != path ||
				read["repository"] != string(want.repository) || read["commit"] != string(published.Repositories[want.repository]) {
				t.Fatalf("selected path did not read its published bytes: %#v %q %v", read, content, err)
			}
		}
		cursor, _ := page["continuation"].(string)
		if cursor == "" {
			if page["exhausted"] != true {
				t.Fatalf("missing continuation was reported as incomplete: %#v", page)
			}
			break
		}
		page = asMap(t, body(t, kcRemote(t, f.serverURL, "agent:consumer", "files", "list", "--dataset", id, "--revision", "2", "--path", directory, "--limit", "1", "--continuation", cursor)))
	}
	if !seen["reference/policies/README.md"] || !seen["reference/policies/metric.md"] || len(seen) != 2 {
		t.Fatalf("mixed directory lost a source: %#v", seen)
	}
	detail := asMap(t, body(t, kcRemote(t, f.serverURL, "agent:consumer", "dataset", "show", id, "--revision", "2", "--limit", "1")))
	if detail["revision"] != float64(2) || detail["servingRevision"] != float64(3) || len(detail["sources"].([]any)) != 5 || len(detail["versions"].([]any)) != 1 {
		t.Fatalf("historical detail lost publication/current distinction: %#v", detail)
	}
	if err := f.operator.AdminService().RemoveGrant(context.Background(), "delivery-reader", snapshotclient.RequestOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	expectCode(t, kcRemote(t, f.serverURL, "agent:consumer", "files", "read", "--dataset", id, "--revision", "2", "--path", directory+"/README.md"), "FORBIDDEN")
}

func TestSnapshotCLISingleFileDownloadPreservesBytesAndExistingTargets(t *testing.T) {
	f := newDirectoryDelivery(t)
	content := bytes.Repeat([]byte{0, 255, '\n', 128, 'a'}, 120000)
	f.write(t, deliveryData, "binary-file", []snapshot.TreeChange{{Path: "semantic/metrics/data.bin", Content: content}})
	if err := f.publish(2, directoryDeliverySources("reference/policies", "reference/guides", "data/metrics")); err != nil {
		t.Fatal(err)
	}
	published := f.mounts(t, nil).Pin
	target := filepath.Join(t.TempDir(), "copy.bin")
	receipt := asMap(t, body(t, kcRemote(t, f.serverURL, "agent:consumer", "files", "read", "--dataset", "delivery", "--revision", "2", "--path", "data/metrics/data.bin", "--out", target)))
	actual, err := os.ReadFile(target)
	wantReceipt := cloneJSONMap(t, map[string]any{"dataset": "delivery", "revision": 2, "pinId": published.PinID, "repositories": published.Repositories, "path": "data/metrics/data.bin", "out": target, "bytes": len(content)})
	if err != nil || !bytes.Equal(actual, content) || !reflect.DeepEqual(receipt, wantReceipt) {
		t.Fatalf("download changed original bytes: receipt=%#v bytes=%d err=%v", receipt, len(actual), err)
	}
	for _, path := range []string{target, filepath.Join(filepath.Dir(target), "link.bin")} {
		if path != target {
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
		}
		expectCode(t, kcRemote(t, f.serverURL, "agent:consumer", "files", "read", "--dataset", "delivery", "--path", "data/metrics/data.bin", "--out", path), "PRECONDITION_FAILED")
		if path != target {
			if destination, err := os.Readlink(path); err != nil || destination != target {
				t.Fatalf("rejected download replaced its symlink: target=%q err=%v", destination, err)
			}
		}
	}
	actual, _ = os.ReadFile(target)
	if !bytes.Equal(actual, content) {
		t.Fatal("rejected download changed the existing target")
	}
	for _, revision := range []int{0, -1, 99} {
		code := "USAGE_INVALID"
		if revision == 99 {
			code = "VERSION_UNRESOLVED"
		}
		expectCode(t, kcRemote(t, f.serverURL, "agent:consumer", "files", "read", "--dataset", "delivery", "--revision", strconv.Itoa(revision), "--path", "data/metrics/data.bin"), code)
	}
}

func TestSnapshotCLIGrantUpdateKeepsObservedRuleAndChangesConsumptionAtomically(t *testing.T) {
	f := newDirectoryDelivery(t)
	policy := func() map[string]any {
		return asMap(t, body(t, kcRemote(t, f.serverURL, "agent:publisher", "grant", "list")))
	}
	listed := policy()
	var old map[string]any
	for _, value := range listed["rules"].([]any) {
		rule := value.(map[string]any)
		if rule["id"] == "delivery-reader" {
			old = rule
		}
	}
	if old == nil {
		t.Fatal("observed grant is missing")
	}
	replacement := cloneJSONMap(t, old)
	replacement["actions"] = []string{"dataset.resolve"}
	dir := t.TempDir()
	observedPath, replacementPath := filepath.Join(dir, "observed.json"), filepath.Join(dir, "replacement.json")
	for path, value := range map[string]map[string]any{observedPath: old, replacementPath: replacement} {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	expectCode(t, kcRemote(t, f.serverURL, "agent:consumer", "grant", "update", "--id", "delivery-reader", "--file", replacementPath, "--expected-file", observedPath), "FORBIDDEN")
	if after := policy(); !reflect.DeepEqual(after, listed) {
		t.Fatalf("unprivileged editor changed the complete policy: before=%#v after=%#v", listed, after)
	}
	updated := asMap(t, body(t, kcRemote(t, f.serverURL, "agent:publisher", "grant", "update", "--id", "delivery-reader", "--file", replacementPath, "--expected-file", observedPath)))
	want := cloneJSONMap(t, replacement)
	if !reflect.DeepEqual(updated, want) {
		t.Fatalf("update lost rule identity or replacement: %#v", updated)
	}
	wantPolicy := cloneJSONMap(t, listed)
	for i, rule := range wantPolicy["rules"].([]any) {
		if asMap(t, rule)["id"] == old["id"] {
			wantPolicy["rules"].([]any)[i] = want
		}
	}
	if after := policy(); !reflect.DeepEqual(after, wantPolicy) {
		t.Fatalf("successful update changed another policy field: want=%#v actual=%#v", wantPolicy, after)
	}
	expectCode(t, kcRemote(t, f.serverURL, "agent:publisher", "grant", "update", "--id", "delivery-reader", "--file", replacementPath, "--expected-file", observedPath), "PRECONDITION_FAILED")
	if after := policy(); !reflect.DeepEqual(after, wantPolicy) {
		t.Fatalf("stale editor changed the complete policy: before=%#v after=%#v", wantPolicy, after)
	}
	expectCode(t, kcRemote(t, f.serverURL, "agent:consumer", "files", "read", "--dataset", "delivery", "--path", "reference/policies/README.md"), "FORBIDDEN")
}
