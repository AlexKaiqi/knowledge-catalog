package cli_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"kc/internal/bindingfixture"
	"kc/kernel"
	"kc/object/reader"
)

func TestObjectReadBasisFreezesDescriptorInterpretationAndRechecksItsAuthority(t *testing.T) {
	home, url, repo := objectReferenceServer(t)
	const descriptors = "kr://basis/descriptors"
	seedRepo(t, home, descriptors)
	admin := func(args ...string) kcRunResult { return kcRemote(t, url, "operator", args...) }
	descriptor := testObjectReference(t, descriptors, "resource/orders", "")
	encoded := func(value any) string {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	body(t, admin("writer", "put", descriptor, "--repo", descriptors, "--command-id", "descriptor-basis-state", "--value", encoded(bindingfixture.Descriptor("https://state.example"))))
	before := asMap(t, body(t, admin("read", descriptor, "--repo", descriptors)))
	bound := ordersReference(t, repo, "/runtime")
	body(t, admin("writer", "put", bound, "--repo", repo, "--kind", "Aspect", "--command-id", "descriptor-basis-binding", "--value", "null", "--value-source", `{"kind":"binding","binding":{"descriptorRef":"resource/orders"}}`))
	body(t, admin("grant", "add", "--principal", "reader", "--repo", repo, "--action", "object.read"))
	grant := asMap(t, body(t, admin("grant", "add", "--principal", "reader", "--repo", descriptors, "--action", "object.read")))
	run := func(args ...string) kcRunResult { return kcRemote(t, url, "reader", args...) }
	first := asMap(t, body(t, run("read", bound, "--repo", repo)))
	basis := first["basis"].(string)
	store, err := reader.NewReadBasisStore(filepath.Join(home, "read-bases.db"))
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.Resolve(context.Background(), basis)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Dependencies) != 1 || string(stored.Dependencies[0].Repository) != descriptors || string(stored.Dependencies[0].Commit) != before["commit"] {
		t.Fatalf("public basis omitted the actual interpretation coordinate: %#v", stored)
	}
	body(t, admin("writer", "put", descriptor, "--repo", descriptors, "--command-id", "descriptor-basis-stream", "--value", encoded(bindingfixture.StreamDescriptor("https://stream.example"))))
	latest := asMap(t, body(t, run("read", bound, "--repo", repo)))
	failure := func(value map[string]any) map[string]any { return asMap(t, value["failures"].([]any)[0]) }
	if !strings.Contains(failure(latest)["message"].(string), "stream Binding") {
		t.Fatalf("new request did not adopt the new descriptor: %#v", latest)
	}
	old := asMap(t, body(t, run("read", bound, "--basis", basis)))
	if strings.Contains(failure(old)["message"].(string), "stream Binding") {
		t.Fatalf("fixed basis silently followed descriptor head: %#v", old)
	}
	if asMap(t, first["state"])["digest"] != asMap(t, latest["state"])["digest"] || asMap(t, old["state"])["complete"] != true {
		t.Fatalf("interpretation or external failure changed local state: first=%#v latest=%#v old=%#v", first, latest, old)
	}
	body(t, admin("grant", "remove", "--id", grant["id"].(string)))
	denied := asMap(t, body(t, run("read", bound, "--basis", basis)))
	if failure(denied)["code"] != string(kernel.ErrForbidden) {
		t.Fatalf("descriptor basis bypassed current source permission: %#v", denied)
	}
}
