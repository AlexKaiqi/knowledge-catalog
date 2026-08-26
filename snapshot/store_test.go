package snapshot

import (
	"errors"
	"slices"
	"testing"

	"kc/kernel"
)

type registryStore struct {
	id       kernel.RepositoryID
	closed   bool
	closeErr error
}

func (s *registryStore) ID() kernel.RepositoryID { return s.id }
func (s *registryStore) Head(string) (kernel.CommitID, error) {
	return "root", nil
}
func (s *registryStore) GetRef(string) (kernel.CommitID, bool)   { return "root", true }
func (s *registryStore) HasCommit(kernel.CommitID) bool          { return true }
func (s *registryStore) CreateRef(string, kernel.CommitID) error { return nil }
func (s *registryStore) Merge(string, kernel.CommitID, kernel.CommitID) (kernel.CommitID, error) {
	return "merged", nil
}
func (s *registryStore) Archived() bool { return false }
func (s *registryStore) Archive() error { return nil }
func (s *registryStore) Close() error {
	s.closed = true
	return s.closeErr
}

func TestRegistryOwnsMembershipEventsAndClose(t *testing.T) {
	registry := NewRegistry()
	first := &registryStore{id: "kr://test/first", closeErr: errors.New("close first")}
	second := &registryStore{id: "kr://test/second"}
	if err := registry.Add(first); err != nil {
		t.Fatal(err)
	}
	if err := registry.Add(second); err != nil {
		t.Fatal(err)
	}
	if err := registry.Add(first); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("duplicate registration must fail: %v", err)
	}
	ids := registry.IDs()
	slices.Sort(ids)
	if !slices.Equal(ids, []kernel.RepositoryID{first.id, second.id}) {
		t.Fatalf("registry ids: %#v", ids)
	}
	var advanced Advanced
	registry.OnAdvanced(func(event Advanced) { advanced = event })
	registry.NotifyAdvanced(Advanced{Store: first, From: "a", To: "b"})
	if advanced.Store != first || advanced.From != "a" || advanced.To != "b" {
		t.Fatalf("advanced event was not delivered: %#v", advanced)
	}
	if err := registry.Close(); err == nil || err.Error() != "close first" {
		t.Fatalf("first close error must be returned: %v", err)
	}
	if !first.closed || !second.closed || len(registry.IDs()) != 0 {
		t.Fatalf("close must release every store: first=%v second=%v ids=%v", first.closed, second.closed, registry.IDs())
	}
}

func TestSnapshotHelpersRejectSecretsAndDefaultRefs(t *testing.T) {
	if RefOrDefault("") != DefaultRef || RefOrDefault("refs/heads/release") != "refs/heads/release" {
		t.Fatal("ref defaulting changed")
	}
	for _, raw := range []string{
		"https://user:secret@example.test/repo",
		"https://example.test/repo?password=secret",
		"host=x api_key=secret",
	} {
		if err := RejectConfiguredSecret("test", raw, "KC_TEST_SECRET"); err == nil {
			t.Fatalf("secret-bearing config was accepted: %s", raw)
		}
	}
	if err := RejectConfiguredSecret("test", "https://example.test/repo", "KC_TEST_SECRET"); err != nil {
		t.Fatalf("non-secret config rejected: %v", err)
	}
}
