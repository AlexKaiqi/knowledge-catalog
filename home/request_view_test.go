package home

import (
	"os"
	"path/filepath"
	"testing"

	"kc/kernel"
)

// Concurrent requests share one served Home. Each request view owns its
// journal and stamps; the shared Home and sibling views keep theirs, and a
// borrowed view cannot end the shared lifecycle or reassemble plugins.
func TestRequestViewOwnsStampsAndBorrowsLifecycle(t *testing.T) {
	cfg := deploymentFixture(t)
	cfg.Stores.Index = "none"
	seed := func(dir, _ string) error {
		return os.WriteFile(filepath.Join(dir, "allow.json"), []byte(`{"version":2,"rules":[]}`), 0600)
	}
	if err := InitializeDeployment(cfg, seed); err != nil {
		t.Fatal(err)
	}
	ws, err := OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	sharedJournal := ws.Journal
	sharedAs, sharedRequest, sharedRule := ws.Writer.Stamp()

	alice := ws.RequestView(nil, "alice", "req-a", "rule-a")
	bob := ws.RequestView(nil, "bob", "req-b", "rule-b")
	if as, request, rule := alice.Writer.Stamp(); as != "alice" || request != "req-a" || rule != "rule-a" {
		t.Fatalf("alice view stamp = %s %s %s", as, request, rule)
	}
	if as, request, rule := bob.Writer.Stamp(); as != "bob" || request != "req-b" || rule != "rule-b" {
		t.Fatalf("bob view stamp = %s %s %s", as, request, rule)
	}
	if as, request, rule := ws.Writer.Stamp(); as != sharedAs || request != sharedRequest || rule != sharedRule || ws.Journal != sharedJournal {
		t.Fatalf("request views rewrote the shared Home: %s %s %s", as, request, rule)
	}
	if alice.Store != ws.Store || alice.Commands != ws.Commands || alice.Registry != ws.Registry {
		t.Fatal("request view did not share the write authority")
	}
	if alice.Writer == ws.Writer || alice.TreeWriter == ws.TreeWriter || alice.Reader == ws.Reader || alice.Datasets == ws.Datasets {
		t.Fatal("request view shares a stamped collaborator with the process Home")
	}
	if err := alice.VerifyIndexPlugins(); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("request view assembled plugins: %v", err)
	}
	if err := alice.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ws.Registry.CheckAuthority(); err != nil {
		t.Fatalf("closing a request view closed the shared Home: %v", err)
	}
}
