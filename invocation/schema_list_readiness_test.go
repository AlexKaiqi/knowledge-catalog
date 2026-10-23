package invocation

import (
	"context"
	"errors"
	retrieval "kc/discovery"
	index "kc/discovery/projection"
	"kc/home"
	"kc/internal/testkit"
	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
	"testing"
)

// countingIndexPlugin is a minimal IndexQueries whose readiness check always
// fails. DEBT-SCHEMA-LIST-COST-01: schema list rows never carry search
// readiness, so listing must not ask the index whether each member can be
// searched now; describe keeps the readiness contract.
type countingIndexPlugin struct {
	readinessChecks int
}

func (p *countingIndexPlugin) RequiresState(_ objectmodel.Repository, _ kernel.CommitID, _ retrieval.SearchRequest) (bool, error) {
	return false, nil
}

func (p *countingIndexPlugin) StateView(_ kernel.RepositoryID, _ kernel.CommitID) (string, bool) {
	return "", false
}

func (p *countingIndexPlugin) SearchAtContext(_ context.Context, _ objectmodel.Repository, _ kernel.CommitID, _ retrieval.SearchRequest) (retrieval.SearchResult, error) {
	return retrieval.SearchResult{}, errors.New("not implemented in this fixture")
}

func (p *countingIndexPlugin) SearchStateAtRevisionContext(_ context.Context, _ objectmodel.Repository, _ kernel.CommitID, _ string, _ retrieval.SearchRequest) (retrieval.SearchResult, error) {
	return retrieval.SearchResult{}, errors.New("not implemented in this fixture")
}

func (p *countingIndexPlugin) RelationsAtContext(_ context.Context, _ objectmodel.Repository, _ kernel.CommitID, _ retrieval.RelationPageRequest) (retrieval.RelationPage, error) {
	return retrieval.RelationPage{}, errors.New("not implemented in this fixture")
}

func (p *countingIndexPlugin) DescribeAt(_ objectmodel.Repository, _ kernel.CommitID) (index.IndexDescriptor, error) {
	return index.IndexDescriptor{}, errors.New("not implemented in this fixture")
}

func (p *countingIndexPlugin) EnsureAt(_ context.Context, _ objectmodel.Repository, _ kernel.CommitID) (index.IndexSync, error) {
	return index.IndexSync{}, errors.New("not implemented in this fixture")
}

func (p *countingIndexPlugin) Ensure(_ context.Context, _ objectmodel.Repository, _ kernel.CommitID) (index.IndexSync, error) {
	return index.IndexSync{}, errors.New("not implemented in this fixture")
}

func (p *countingIndexPlugin) RefreshState(_ context.Context, _ objectmodel.Repository, _ kernel.CommitID, _ index.ObservationSource) (index.StateSync, error) {
	return index.StateSync{}, errors.New("not implemented in this fixture")
}

func (p *countingIndexPlugin) AccessSpecAt(_ objectmodel.Repository, _ kernel.CommitID) (retrieval.AccessSpec, error) {
	return retrieval.AccessSpec{}, nil
}

func (p *countingIndexPlugin) CheckSearchProjectionAt(_ objectmodel.Repository, _ kernel.CommitID) (index.Meta, error) {
	p.readinessChecks++
	return index.Meta{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "projection is not ready in this fixture")
}

func (p *countingIndexPlugin) DatasetReadiness(_ context.Context, _ reader.MemberLookup, pin reader.DatasetPin) (index.IndexReadiness, error) {
	return index.IndexReadiness{Dataset: pin.SetID, Revision: pin.Revision, Ready: false}, nil
}

func (p *countingIndexPlugin) DatasetDiagnosis(_ context.Context, _ reader.MemberLookup, pin reader.DatasetPin) (index.IndexDiagnosis, error) {
	return index.IndexDiagnosis{Dataset: pin.SetID, Revision: pin.Revision, Ready: false}, nil
}

// TestSchemaListDoesNotAskIndexReadiness proves the debt's acceptance: on a
// Dataset whose projection readiness check fails, schema list still returns
// the complete summary rows without calling the readiness check, while schema
// describe keeps reporting readiness.
func TestSchemaListDoesNotAskIndexReadiness(t *testing.T) {
	s := testkit.NewSetup(t, "")
	commit, err := s.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/service.orders"}, Value: map[string]any{
				"entity": "Service", "aspect": "orders",
				"fields": map[string]any{"amount": map[string]any{"type": "string"}},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	plugin := &countingIndexPlugin{}
	pin := reader.DatasetPin{SetID: "agent", Revision: 1,
		Repositories: map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: commit},
		Items:        reader.WholeRepositoryItems(map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: commit})}
	serving := reader.Open(func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return s.Reader.Require(id, kernel.ErrObjectRefUnresolved)
	}, pin)
	cx := &Invocation{WS: &home.Home{Store: s.Store, Reader: s.Reader, Index: plugin},
		Flags: map[string]invoke.FlagValue{"dataset": "agent"}, Context: context.Background()}
	page, err := browseDatasetSchemas(cx, serving, 100)
	if err != nil {
		t.Fatal(err)
	}
	if plugin.readinessChecks != 0 {
		t.Fatalf("schema list must not call the projection readiness check: %d calls", plugin.readinessChecks)
	}
	if len(page.Schemas) == 0 {
		t.Fatalf("schema list must still return complete summary rows: %#v", page)
	}
	found := false
	for _, row := range page.Schemas {
		if row.ObjectID == "schema/service.orders" {
			found = true
			if row.Entity != "Service" || row.Aspect != "orders" || row.Fields == 0 {
				t.Fatalf("summary row lost contract-parsed fields: %#v", row)
			}
		}
	}
	if !found {
		t.Fatalf("schema list missed the seeded Schema: %#v", page.Schemas)
	}
	// Continuation pages must not start calling readiness either.
	if page.Continuation != "" {
		cx.Flags["continuation"] = page.Continuation
		if _, err := browseDatasetSchemas(cx, serving, 100); err != nil {
			t.Fatal(err)
		}
		if plugin.readinessChecks != 0 {
			t.Fatalf("schema list continuation must not call the projection readiness check: %d calls", plugin.readinessChecks)
		}
	}
	// Describe keeps the readiness contract: the failing check is reported.
	reports, err := describeDatasetSchemas(cx, serving, "")
	if err != nil {
		t.Fatal(err)
	}
	if plugin.readinessChecks == 0 {
		t.Fatal("schema describe must keep asking search readiness")
	}
	for _, report := range reports {
		if report.Search.State != SchemaSearchUnavailable {
			t.Fatalf("describe must surface the failing readiness: %#v", report.Search)
		}
	}
}
