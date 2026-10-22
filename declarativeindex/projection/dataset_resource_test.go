package projection

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"kc/declarativeindex/internal/testfixture"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

type guardedDatasetResourceEngine struct {
	testfixture.StateEngine
	ticket   ports.PreparedResource
	err      error
	calls    int
	received ports.Meta
}

func (*guardedDatasetResourceEngine) ProviderID() string            { return "guarded-provider" }
func (*guardedDatasetResourceEngine) PhysicalDigest() kernel.Digest { return "driver-v1" }
func (*guardedDatasetResourceEngine) ProviderRevision() string      { return "provider-v1" }
func (e *guardedDatasetResourceEngine) PrepareResource(_ context.Context, meta ports.Meta) (ports.PreparedResource, error) {
	e.calls++
	e.received = meta
	return e.ticket, e.err
}

type guardedDatasetResourceTicket struct{}

func (*guardedDatasetResourceTicket) Admit(context.Context, []byte) error { return nil }
func (*guardedDatasetResourceTicket) Confirm(context.Context) error       { return nil }
func (*guardedDatasetResourceTicket) Cancel(context.Context) error        { return nil }
func (*guardedDatasetResourceTicket) Close() error                        { return nil }

func TestDatasetResourceProtectionChecksCurrentProviderCompatibilityBeforeAcquisition(t *testing.T) {
	for _, kind := range []string{"compatible", "access-changed", "driver-changed", "provider-changed", "manifest-changed", "basis-changed", "not-ready", "missing-ticket", "ticket-and-error"} {
		t.Run(kind, func(t *testing.T) {
			wrapped, at, _ := testfixture.StateProjection(t)
			repo := wrapped.(*testkit.ObjectRepository).Repository
			pin := reader.DatasetPin{SetID: "guarded", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): at}}
			pin.Items = reader.WholeRepositoryItems(pin.Repositories)
			lookup := func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo, nil }
			ticket := &guardedDatasetResourceTicket{}
			engine := &guardedDatasetResourceEngine{ticket: ticket}
			idx := NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
			t.Cleanup(func() { _ = idx.Close() })
			if _, err := idx.PrepareDataset(context.Background(), lookup, pin); err != nil {
				t.Fatal(err)
			}
			var expected ports.Meta
			encoded, err := json.Marshal(engine.Meta)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(encoded, &expected); err != nil {
				t.Fatal(err)
			}

			problem := errors.New("preparation returned a ticket with an error")
			switch kind {
			case "access-changed":
				engine.Meta.AccessDigest = "another-interpretation"
			case "driver-changed":
				engine.Meta.PhysicalDigest = "other-driver"
			case "provider-changed":
				engine.Meta.ProviderRevision = "other-provider"
			case "manifest-changed":
				idx.manifestDigest = "changed-manifest"
			case "basis-changed":
				engine.Meta.Basis = "another-basis"
			case "not-ready":
				engine.Meta.State = ports.ProjectionStateBuilding
			case "missing-ticket":
				engine.ticket = nil
			case "ticket-and-error":
				engine.err = problem
			}
			result, err := idx.ProtectDatasetResource(context.Background(), lookup, pin)
			switch kind {
			case "compatible":
				if err != nil || result != ticket || engine.calls != 1 || !reflect.DeepEqual(engine.received, expected) {
					t.Fatalf("compatible preparation changed exact metadata: ticket=%v calls=%d meta=%#v %v", result, engine.calls, engine.received, err)
				}
			case "missing-ticket":
				if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || result != nil || engine.calls != 1 {
					t.Fatalf("missing managed protection was accepted: %v %v", result, err)
				}
			case "ticket-and-error":
				if !errors.Is(err, problem) || result != ticket || engine.calls != 1 {
					t.Fatalf("error lost allocated ticket ownership: %v %v", result, err)
				}
			default:
				if kernel.CodeOf(err) != kernel.ErrPreconditionFailed || result != nil || engine.calls != 0 {
					t.Fatalf("incompatible ready resource acquired protection: ticket=%v calls=%d %v", result, engine.calls, err)
				}
			}
		})
	}
}

func TestDatasetProtectedPreparationReuseKeepsOriginalAdoptionValidation(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		t.Run(map[bool]string{false: "compatible", true: "corrupt-witness"}[corrupt], func(t *testing.T) {
			wrapped, at, _ := testfixture.StateProjection(t)
			repo := wrapped.(*testkit.ObjectRepository).Repository
			pin := reader.DatasetPin{SetID: "reuse-protection", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): at}}
			pin.Items = reader.WholeRepositoryItems(pin.Repositories)
			lookup := func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo, nil }
			ticket := &guardedDatasetResourceTicket{}
			engine := &guardedDatasetResourceEngine{ticket: ticket}
			idx := NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
			t.Cleanup(func() { _ = idx.Close() })
			if _, err := idx.PrepareDataset(context.Background(), lookup, pin); err != nil {
				t.Fatal(err)
			}
			if corrupt {
				engine.Meta.SchemaAdoptions = append(engine.Meta.SchemaAdoptions, retrieval.SchemaAdoption{SchemaRef: "invalid-retained-release"})
			}
			actual, err := idx.PrepareDatasetResource(context.Background(), lookup, pin)
			if corrupt {
				if kernel.CodeOf(err) != kernel.ErrPreconditionFailed || actual != nil || engine.calls != 0 {
					t.Fatalf("reuse bypassed retained witness validation: %v calls=%d %v", actual, engine.calls, err)
				}
			} else if err != nil || actual != ticket || engine.calls != 1 {
				t.Fatalf("compatible reuse: %v calls=%d %v", actual, engine.calls, err)
			}
		})
	}
}
