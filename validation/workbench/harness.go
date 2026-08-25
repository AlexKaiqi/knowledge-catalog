package scenario

import (
	"testing"

	"kc/catalog"
	"kc/controlplane"
	"kc/gate"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/reader"
	"kc/repository"
	"kc/writer"
)

// workbench is one Catalog + three member Snapshot stores. Dynamic observations
// are represented by versioned Binding declarations, not a Repository stream.
type workbench struct {
	store    *repository.Store
	writer   *writer.Writer
	reader   *reader.Reader
	catalog  *catalog.Catalog
	plane    *controlplane.ControlPlane
	meta     repository.Repository
	sem      repository.Repository
	kai      repository.Repository
	commits  map[string]kernel.CommitID
	gens     map[string]string
	evidence map[string][]gate.Evidence
}

func newWorkbench(t *testing.T) *workbench {
	t.Helper()
	store := repository.NewStore()
	t.Cleanup(func() { _ = store.Close() })

	meta := testkit.MakeRepository(t, string(Metadata))
	sem := testkit.MakeRepository(t, string(Semantics))
	kai := testkit.MakeRepository(t, string(Personal))
	for _, repo := range []repository.Repository{meta, sem, kai} {
		if err := store.Add(repo); err != nil {
			t.Fatal(err)
		}
	}
	w, err := writer.NewWriter(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := catalog.NewRegistry(testkit.TempDir(t), CatalogID)
	if err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.NewCatalog(store, registry)
	if err != nil {
		t.Fatal(err)
	}
	wb := &workbench{
		store:    store,
		writer:   w,
		reader:   reader.NewReader(store),
		catalog:  cat,
		plane:    controlplane.New(store, w, cat),
		meta:     meta,
		sem:      sem,
		kai:      kai,
		commits:  map[string]kernel.CommitID{},
		gens:     map[string]string{},
		evidence: map[string][]gate.Evidence{},
	}
	wb.rememberCommit("Meta0", testkit.MustHead(t, meta, MainRef))
	wb.rememberCommit("Sem0", testkit.MustHead(t, sem, MainRef))
	wb.rememberCommit("Kai0", testkit.MustHead(t, kai, MainRef))
	wb.plane.SetMergeGate(func(repo kernel.RepositoryID) []string {
		if repo == Semantics {
			return []string{gate.RequireValidate, "suite:steward"}
		}
		return nil
	}, func(basis string) []gate.Evidence {
		return wb.evidence[basis]
	})
	return wb
}

func (wb *workbench) rememberCommit(alias string, id kernel.CommitID) {
	wb.commits[alias] = id
}

func (wb *workbench) repo(id kernel.RepositoryID) repository.Repository {
	switch id {
	case Metadata:
		return wb.meta
	case Semantics:
		return wb.sem
	case Personal:
		return wb.kai
	default:
		repo, _ := wb.store.Knowledge(id, kernel.ErrUsageInvalid)
		return repo
	}
}

func (wb *workbench) stamp(as, requestID, ruleID string) {
	wb.writer.SetStamp(as, requestID, ruleID)
	wb.catalog.SetStamp(as, requestID, ruleID)
}

func (wb *workbench) commit(t *testing.T, commandID string, repo kernel.RepositoryID, ops []repository.Operation, prov *kernel.ProvenanceEnvelope) writer.CommitReceipt {
	t.Helper()
	receipt, err := wb.writer.CommitIntent(commandID, writer.CommitIntent{
		TargetRepository: repo,
		TargetRef:        MainRef,
		Operations:       ops,
		Message:          commandID,
		Provenance:       prov,
	})
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func (wb *workbench) mustCommit(t *testing.T, alias, commandID string, repo kernel.RepositoryID, ops []repository.Operation, prov *kernel.ProvenanceEnvelope) {
	t.Helper()
	receipt := wb.commit(t, commandID, repo, ops, prov)
	if receipt.Disposition != writer.DispositionApplied {
		t.Fatalf("%s disposition %s", alias, receipt.Disposition)
	}
	wb.rememberCommit(alias, receipt.Result.CommitID)
}

func (wb *workbench) head(t *testing.T, repo kernel.RepositoryID) kernel.CommitID {
	t.Helper()
	return testkit.MustHead(t, wb.repo(repo), MainRef)
}

func (wb *workbench) snapshot() catalog.CatalogState {
	return catalog.NormalizeCatalogState(wb.catalog.DumpState())
}

func (wb *workbench) freeze(t *testing.T) catalog.CatalogState {
	t.Helper()
	return wb.snapshot()
}

func companyWorkspaceSources() []catalog.WorkspaceSource {
	out := make([]catalog.WorkspaceSource, 0, 2)
	for _, src := range companySources() {
		out = append(out, catalog.WorkspaceSource{Repository: src.Repository, Selector: src.Selector})
	}
	return out
}

func (wb *workbench) overlaySources() []catalog.WorkspaceSource {
	return []catalog.WorkspaceSource{
		{Repository: Metadata, Selector: MainRef},
		{Repository: Semantics, Selector: MainRef},
		{Repository: Personal, Selector: MainRef},
	}
}

func (wb *workbench) openView(workspaceID string) (*reader.Serving, error) {
	resolved, err := wb.catalog.ResolveWorkspace(workspaceID)
	if err != nil {
		return nil, err
	}
	return reader.Open(wb.catalog.RequireKnowledge, workspacePin(resolved)), nil
}

func (wb *workbench) federatedRead(workspaceID string, object kernel.ObjectID) ([]reader.FederatedValue, error) {
	serving, err := wb.openView(workspaceID)
	if err != nil {
		return nil, err
	}
	return serving.Read(object, nil)
}

func (wb *workbench) planAccess(workspaceID string) (reader.AccessPlan, error) {
	resolved, err := wb.catalog.ResolveWorkspace(workspaceID)
	if err != nil {
		return reader.AccessPlan{}, err
	}
	return reader.PlanAccess(wb.catalog.RequireKnowledge, workspacePin(resolved))
}

func workspacePin(resolved catalog.ResolvedWorkspace) reader.WorkspacePin {
	return reader.WorkspacePin{
		WorkspaceID:  resolved.WorkspaceID,
		Revision:     resolved.Revision,
		Repositories: resolved.Repositories,
	}
}
