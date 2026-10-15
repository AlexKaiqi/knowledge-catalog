package server

import (
	"encoding/json"
	inv "kc/invocation"
	"kc/invoke"
	objectmodel "kc/object"
	"testing"

	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

func TestE2EReadPayloadHoistsDefinitionBeforeAuthorize(t *testing.T) {
	home := t.TempDir()
	repositoryID := "kr://acme/temporary-label/source"
	const label = "trusted"
	const namedReader = "agent:named-reader"
	if err := inv.WriteAllow(home, inv.AllowFile{Rules: []inv.AllowRule{
		{ID: "1", Principal: namedReader, Actions: []string{"dataset.resolve", "file.read"}, Dataset: label},
		{ID: "2", Principal: namedReader, Actions: []string{"object.read"}, Repo: repositoryID},
	}}); err != nil {
		t.Fatal(err)
	}
	sources := []map[string]any{{"repository": repositoryID, "selector": snapshot.DefaultRef}}
	definition := map[string]any{"setId": label, "revision": 1, "sources": sources}
	pinWithDefinition := map[string]any{
		"setId": label, "revision": 1,
		"repositories": map[string]any{repositoryID: "fixed"},
		"definition":   definition,
	}
	reference, err := objectmodel.FormatReference(objectmodel.Reference{Repository: kernel.RepositoryID(repositoryID), Object: "policy/label"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{"pin": pinWithDefinition, "reference": reference})
	if err != nil {
		t.Fatal(err)
	}
	var request objectReadRequest
	if err := dataset.DecodeJSON(body, &request); err != nil {
		t.Fatal(err)
	}
	flags := request.Flags()
	flags["as"] = namedReader
	if err := inv.HoistTaskPinDefinition(flags); err != nil {
		t.Fatal(err)
	}
	if err := inv.PrepareObjectPinContext(flags); err != nil {
		t.Fatal(err)
	}
	if err := inv.Authorize(home, "object.read", flags, nil); err == nil {
		t.Fatalf("authorize accepted impersonation: %#v", flags)
	}
}

func TestHTTPObjectReadFlagsExtractTemporaryDefinition(t *testing.T) {
	repositoryID := "kr://acme/temporary-label/source"
	const label = "trusted"
	pinWithDefinition := map[string]any{
		"setId": label, "revision": 1,
		"repositories": map[string]any{repositoryID: "fixed"},

		"definition": map[string]any{
			"setId": label, "revision": 1,
			"sources": []map[string]any{{"repository": repositoryID, "selector": snapshot.DefaultRef}},
		},
	}
	reference, err := objectmodel.FormatReference(objectmodel.Reference{Repository: kernel.RepositoryID(repositoryID), Object: "policy/label"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(objectReadRequest{
		objectDatasetIdentity: objectDatasetIdentity{Pin: mustRaw(t, pinWithDefinition)}, Reference: reference,
	})
	if err != nil {
		t.Fatal(err)
	}
	var request objectReadRequest
	if err := dataset.DecodeJSON(body, &request); err != nil {
		t.Fatal(err)
	}
	flags := request.Flags()
	flags["as"] = "agent:named-reader"
	if err := inv.PrepareObjectPinContext(flags); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if inv.SuppliedDataset(flags) == nil {
		t.Fatalf("HTTP flags lost definition: %#v pin=%s", flags, string(request.Pin))
	}
}

func mustRaw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestNamedReaderPublishedWorkspaceConsumeOnlyAdmitsComposition(t *testing.T) {
	home := t.TempDir()
	repositoryID := "kr://acme/temporary-label/source"
	const label = "trusted"
	const namedReader = "agent:named-reader"
	rules := []inv.AllowRule{
		{ID: "1", Principal: namedReader, Actions: []string{"dataset.resolve", "file.read"}, Dataset: label},
	}
	if err := inv.WriteAllow(home, inv.AllowFile{Rules: rules}); err != nil {
		t.Fatal(err)
	}
	flags := map[string]FlagValue{"as": namedReader, "object": "policy/label", "dataset": label}
	if err := inv.Authorize(home, "object.read", flags, nil); err != nil {
		t.Fatalf("consume must admit the composition gate: %v", err)
	}
	if !inv.AllowedRepoRead(home, flags, repositoryID, "policy/label") {
		t.Fatal("dataset file.read must deliver listed-file bodies")
	}
	repoFlags := map[string]FlagValue{"as": namedReader, "object": "policy/label", "repo": repositoryID}
	if inv.AllowedRepoRead(home, repoFlags, repositoryID, "policy/label") {
		t.Fatal("dataset file.read must not admit --repo object.read")
	}
}

func TestNamedReaderCannotUseTemporaryPinWithDefinition(t *testing.T) {
	home := t.TempDir()
	repositoryID := "kr://acme/temporary-label/source"
	const label = "trusted"
	const namedReader = "agent:named-reader"
	rules := []inv.AllowRule{
		{ID: "1", Principal: namedReader, Actions: []string{"dataset.resolve", "file.read"}, Dataset: label},
		{ID: "2", Principal: namedReader, Actions: []string{"object.read"}, Repo: repositoryID},
	}
	if err := inv.WriteAllow(home, inv.AllowFile{Rules: rules}); err != nil {
		t.Fatal(err)
	}
	definition := &dataset.Dataset{
		SetID:    label,
		Revision: 1,
		Sources:  []dataset.DatasetSource{{Repository: kernel.RepositoryID(repositoryID), Selector: snapshot.DefaultRef}},
	}
	pin := inv.TaskDatasetPin{
		ResolvedDataset: dataset.ResolvedDataset{SetID: label, Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{kernel.RepositoryID(repositoryID): "fixed"}},

		Definition: definition,
	}
	rawPin, err := json.Marshal(pin)
	if err != nil {
		t.Fatal(err)
	}
	flags := map[string]FlagValue{"as": namedReader, "object": "policy/label", "pin": string(rawPin)}
	if err := inv.PrepareObjectPinContext(flags); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if inv.SuppliedDataset(flags) == nil {
		t.Fatalf("definition not extracted: %#v", flags)
	}
	if invoke.FlagString(flags, "dataset") != "" {
		t.Fatalf("workspace not cleared: %q", invoke.FlagString(flags, "dataset"))
	}
	if err := inv.Authorize(home, "object.read", flags, nil); err == nil {
		t.Fatal("expected forbidden")
	} else if kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("unexpected: %v", err)
	}
}
