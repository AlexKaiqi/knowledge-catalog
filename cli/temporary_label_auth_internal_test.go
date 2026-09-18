package cli

import (
	"encoding/json"
	"testing"

	"kc/catalog"
	"kc/kernel"
	"kc/snapshot"
)

func TestE2EReadPayloadHoistsDefinitionBeforeAuthorize(t *testing.T) {
	home := t.TempDir()
	catalogID := "kr://acme/temporary-label/catalog"
	repositoryID := "kr://acme/temporary-label/source"
	const label = "trusted"
	const namedReader = "agent:named-reader"
	if err := WriteAllow(home, AllowFile{Rules: []AllowRule{
		{ID: "1", Principal: namedReader, Actions: []string{"dataset.resolve", "file.read"}, Catalog: catalogID, Dataset: label},
		{ID: "2", Principal: namedReader, Actions: []string{"knowledge.read"}, Repo: repositoryID},
	}}); err != nil {
		t.Fatal(err)
	}
	sources := []map[string]any{{"repository": repositoryID, "selector": snapshot.DefaultRef}}
	definition := map[string]any{"setId": label, "revision": 1, "sources": sources}
	pinWithDefinition := map[string]any{
		"setId": label, "revision": 1,
		"repositories": map[string]any{repositoryID: "fixed"},
		"catalog":      catalogID, "definition": definition,
	}
	body, err := json.Marshal(map[string]any{"pin": pinWithDefinition, "object": "Policy:label"})
	if err != nil {
		t.Fatal(err)
	}
	var request knowledgeReadRequest
	if err := catalog.DecodeJSON(body, &request); err != nil {
		t.Fatal(err)
	}
	flags := request.flags()
	flags["as"] = namedReader
	if err := hoistTaskPinDefinition(flags); err != nil {
		t.Fatal(err)
	}
	if err := prepareKnowledgePinContext(flags); err != nil {
		t.Fatal(err)
	}
	if err := authorize(home, "knowledge.read", flags, nil); err == nil {
		t.Fatalf("authorize accepted impersonation: %#v", flags)
	}
}

func TestHTTPKnowledgeReadFlagsExtractTemporaryDefinition(t *testing.T) {
	catalogID := "kr://acme/temporary-label/catalog"
	repositoryID := "kr://acme/temporary-label/source"
	const label = "trusted"
	pinWithDefinition := map[string]any{
		"setId": label, "revision": 1,
		"repositories": map[string]any{repositoryID: "fixed"},
		"catalog":      catalogID,
		"definition": map[string]any{
			"setId": label, "revision": 1,
			"sources": []map[string]any{{"repository": repositoryID, "selector": snapshot.DefaultRef}},
		},
	}
	body, err := json.Marshal(knowledgeReadRequest{Pin: mustRaw(t, pinWithDefinition), Object: "Policy:label"})
	if err != nil {
		t.Fatal(err)
	}
	var request knowledgeReadRequest
	if err := catalog.DecodeJSON(body, &request); err != nil {
		t.Fatal(err)
	}
	flags := request.flags()
	flags["as"] = "agent:named-reader"
	if err := prepareKnowledgePinContext(flags); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if suppliedKnowledgeSet(flags) == nil {
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
	catalogID := "kr://acme/temporary-label/catalog"
	repositoryID := "kr://acme/temporary-label/source"
	const label = "trusted"
	const namedReader = "agent:named-reader"
	rules := []AllowRule{
		{ID: "1", Principal: namedReader, Actions: []string{"dataset.resolve", "file.read"}, Catalog: catalogID, Dataset: label},
	}
	if err := WriteAllow(home, AllowFile{Rules: rules}); err != nil {
		t.Fatal(err)
	}
	flags := map[string]FlagValue{"as": namedReader, "object": "Policy:label", "dataset": label, "catalog": catalogID}
	if err := authorize(home, "knowledge.read", flags, nil); err != nil {
		t.Fatalf("consume must admit the composition gate: %v", err)
	}
	if !allowedRepoRead(home, flags, repositoryID, "Policy:label") {
		t.Fatal("dataset file.read must deliver listed-file bodies")
	}
	repoFlags := map[string]FlagValue{"as": namedReader, "object": "Policy:label", "repo": repositoryID}
	if allowedRepoRead(home, repoFlags, repositoryID, "Policy:label") {
		t.Fatal("dataset file.read must not admit --repo knowledge.read")
	}
}

func TestNamedReaderCannotUseTemporaryPinWithDefinition(t *testing.T) {
	home := t.TempDir()
	catalogID := "kr://acme/temporary-label/catalog"
	repositoryID := "kr://acme/temporary-label/source"
	const label = "trusted"
	const namedReader = "agent:named-reader"
	rules := []AllowRule{
		{ID: "1", Principal: namedReader, Actions: []string{"dataset.resolve", "file.read"}, Catalog: catalogID, Dataset: label},
		{ID: "2", Principal: namedReader, Actions: []string{"knowledge.read"}, Repo: repositoryID},
	}
	if err := WriteAllow(home, AllowFile{Rules: rules}); err != nil {
		t.Fatal(err)
	}
	definition := &catalog.KnowledgeSet{
		SetID: label,
		Revision:    1,
		Sources:     []catalog.KnowledgeSetSource{{Repository: kernel.RepositoryID(repositoryID), Selector: snapshot.DefaultRef}},
	}
	pin := taskKnowledgeSetPin{
		ResolvedKnowledgeSet: catalog.ResolvedKnowledgeSet{SetID: label, Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{kernel.RepositoryID(repositoryID): "fixed"}},
		Catalog:           catalogID,
		Definition:        definition,
	}
	rawPin, err := json.Marshal(pin)
	if err != nil {
		t.Fatal(err)
	}
	flags := map[string]FlagValue{"as": namedReader, "object": "Policy:label", "pin": string(rawPin), "catalog": catalogID}
	if err := prepareKnowledgePinContext(flags); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if suppliedKnowledgeSet(flags) == nil {
		t.Fatalf("definition not extracted: %#v", flags)
	}
	if FlagString(flags, "dataset") != "" {
		t.Fatalf("workspace not cleared: %q", FlagString(flags, "dataset"))
	}
	if err := authorize(home, "knowledge.read", flags, nil); err == nil {
		t.Fatal("expected forbidden")
	} else if kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("unexpected: %v", err)
	}
}
