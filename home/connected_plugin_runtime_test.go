package home

import (
	"context"
	"net/http"
	"testing"

	"kc/kernel"
)

// The deployment, not the Repository, installs plugins: assembling and serving
// a connected source never reads Repository metadata to choose an instance.
func TestConnectedPluginRuntimeAssemblesFromDeploymentWithoutRepositoryMetadata(t *testing.T) {
	f := newConnectedPluginFixture(t)
	ws := f.openCold(t)
	repo, err := ws.Reader.Require(f.id, kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	before := f.metadataReads.Load()
	old := ws.Plugins
	view := ws.ReadView(nil)
	if err := view.VerifyIndexPlugins(); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("borrowed request view assembled plugins: %v", err)
	}
	if err := view.Close(); err != nil {
		t.Fatal(err)
	}
	if ws.Plugins != old {
		t.Fatal("borrowed view changed the root's plugin configuration")
	}
	if err := ws.VerifyIndexPlugins(); err != nil {
		t.Fatal(err)
	}
	if ws.Plugins == old {
		t.Fatal("explicit runtime assembly did not replace the cold view")
	}
	if _, err := old.CheckSearchProjectionAt(repo, f.basis); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("old cold view was retained after runtime assembly: %v", err)
	}
	f.prepareAndRead(t, ws)
	verified := ws.Plugins
	if err := ws.Plugins.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := ws.VerifyIndexPlugins(); kernel.CodeOf(err) != kernel.ErrPreconditionFailed || ws.Plugins != verified {
		t.Fatalf("started root reassembled its plugins: %v", err)
	}
	f.assertCanonical(t, ws)
	if f.metadataReads.Load() != before {
		t.Fatalf("plugin assembly or serving read Repository metadata %d times", f.metadataReads.Load()-before)
	}
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := verified.CheckSearchProjectionAt(repo, f.basis); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("root shutdown left the real query contribution open: %v", err)
	}
}

func TestConnectedPluginRuntimeRejectsAssemblyAfterActivation(t *testing.T) {
	f := newConnectedPluginFixture(t)
	ws := f.openCold(t)
	before, old := f.metadataReads.Load(), ws.Plugins
	if err := ws.Plugins.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := ws.VerifyIndexPlugins(); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("active root accepted configuration assembly: %v", err)
	}
	if ws.Plugins != old || f.metadataReads.Load() != before {
		t.Fatal("rejected assembly contacted configuration authority or replaced the active view")
	}
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := f.openCold(t)
	if err := reopened.VerifyIndexPlugins(); err != nil {
		t.Fatal(err)
	}
	f.prepareAndRead(t, reopened)
}

// A failing Repository metadata authority is not a plugin configuration
// failure: the deployed instance still prepares and serves the source, and
// durable connection management stays available.
func TestConnectedPluginRuntimeIsIndependentOfRepositoryMetadataFaults(t *testing.T) {
	for _, status := range []int64{http.StatusForbidden, http.StatusServiceUnavailable} {
		f := newConnectedPluginFixture(t)
		f.metadataFault.Store(status)
		ws := f.openCold(t)
		if err := ws.VerifyIndexPlugins(); err != nil {
			t.Fatalf("metadata status %d blocked plugin assembly: %v", status, err)
		}
		f.prepareAndRead(t, ws)
		beforeRequests := f.requests.Load()
		connection, err := ws.GetConnection("alice", string(f.id))
		if err != nil || connection.Status != "READY" || connection.Head != f.basis || f.requests.Load() != beforeRequests {
			t.Fatalf("metadata status %d blocked durable connection management: result=%#v error=%v", status, connection, err)
		}
		if err := ws.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
