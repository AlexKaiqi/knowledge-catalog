package invocation

import (
	"encoding/json"
	"kc/invoke"
	"kc/kernel"
	"kc/snapshot/dataset"
	"testing"
)

func TestTemporaryDefinitionDoesNotBypassConsumeGrant(t *testing.T) {
	q := AllowQuery{Principal: "reader", Action: "object.read"}
	read := AllowRule{ID: "read", Principal: "reader", Actions: []string{"object.read"}, Repo: "kr://acme/source"}
	if err := AuthorizeWorkspaceObject([]AllowRule{read}, q, true); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("temporary composition bypassed consume: %v", err)
	}
	named := AllowRule{ID: "named", Principal: "reader", Actions: []string{"file.read"}, Dataset: "named-only"}
	if err := AuthorizeWorkspaceObject([]AllowRule{named, read}, q, true); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("named-only grant authorized arbitrary temporary composition: %v", err)
	}
	named.Dataset = ""
	if err := AuthorizeWorkspaceObject([]AllowRule{named, read}, q, true); err != nil {
		t.Fatal(err)
	}
	q.Action = "object.search"
	if err := AuthorizeWorkspaceObject([]AllowRule{named, read}, q, true); err != nil {
		t.Fatalf("catalog-wide dataset file.read must admit pin object.search: %v", err)
	}
}

func TestTemporaryReplayRejectsChangedMembershipAndLayout(t *testing.T) {
	source := dataset.DatasetSource{Repository: "kr://acme/source", Selector: "refs/heads/main"}
	def := dataset.Dataset{Revision: 1, Sources: []dataset.DatasetSource{source}}
	pin := dataset.ResolvedDataset{Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{source.Repository: "fixed"}}
	pin.PinID = dataset.HashResolved("", def.Sources, pin.Repositories)
	raw, _ := json.Marshal(pin)
	if _, err := DecodeReplayPin(def, string(raw)); err != nil {
		t.Fatal(err)
	}
	def.Sources[0].Path = dataset.MountPath("changed-layout")
	if _, err := DecodeReplayPin(def, string(raw)); kernel.CodeOf(err) != kernel.ErrDatasetInvalid {
		t.Fatalf("changed layout reused pin identity: %v", err)
	}
	def.Sources[0] = source
	def.Sources = append(def.Sources, dataset.DatasetSource{Repository: "kr://acme/other", Selector: "refs/heads/main"})
	if _, err := DecodeReplayPin(def, string(raw)); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("changed membership reused pin: %v", err)
	}
}

func TestTemporaryCompositionUsesEveryMemberScopedResolveAndConsume(t *testing.T) {
	home := t.TempDir()
	rules := []AllowRule{
		{ID: "a", Principal: "reader", Repo: "kr://acme/a", Actions: []string{"dataset.resolve", "file.read", "object.read"}},
		{ID: "b", Principal: "reader", Repo: "kr://acme/b", Actions: []string{"dataset.resolve", "file.read", "object.read"}},
	}
	if err := WriteAllow(home, AllowFile{Rules: rules}); err != nil {
		t.Fatal(err)
	}
	definition := &dataset.Dataset{Revision: 1, Sources: []dataset.DatasetSource{{Repository: "kr://acme/a", Selector: "refs/heads/main"}, {Repository: "kr://acme/b", Selector: "refs/heads/main"}}}
	flags := map[string]invoke.FlagValue{"as": "reader", WorkspaceDefinitionFlag: definition}
	for _, action := range []string{"dataset.resolve", "object.read"} {
		if err := Authorize(home, action, flags, nil); err != nil {
			t.Fatalf("member-scoped %s blocked: %v", action, err)
		}
	}
	if err := Authorize(home, "inventory.read", flags, nil); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("member share widened catalog access: %v", err)
	}
	definition.Sources = append(definition.Sources, dataset.DatasetSource{Repository: "kr://acme/unshared", Selector: "refs/heads/main"})
	for _, action := range []string{"dataset.resolve", "object.read"} {
		if err := Authorize(home, action, flags, nil); kernel.CodeOf(err) != kernel.ErrForbidden {
			t.Fatalf("unshared member allowed %s: %v", action, err)
		}
	}
}
