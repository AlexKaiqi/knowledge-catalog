package home

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	retrieval "kc/discovery"
	consumerplugin "kc/discovery/plugin"
	"kc/kernel"
)

func TestConnectedPluginRuntimeVerifiesBeforePreparingAndReading(t *testing.T) {
	f := newConnectedPluginFixture(t)
	ws := f.openCold(t)
	repo, err := ws.Reader.Require(f.id, kernel.ErrKnowledgeRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	before := f.metadataReads.Load()
	if _, err := ws.Index.CheckSearchProjectionAt(repo, f.basis); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("unverified connected source was treated as absent configuration or ready: %v", err)
	}
	old := ws.Plugins
	view := ws.ReadView(nil)
	if err := view.VerifyIndexPlugins(); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("borrowed request view assembled plugins: %v", err)
	}
	if err := view.Close(); err != nil {
		t.Fatal(err)
	}
	if f.metadataReads.Load() != before || ws.Plugins != old {
		t.Fatal("borrowed view changed or probed the root's plugin configuration")
	}
	if err := ws.VerifyIndexPlugins(); err != nil {
		t.Fatal(err)
	}
	if f.metadataReads.Load() != before+1 || ws.Plugins == old {
		t.Fatal("explicit runtime verification did not read native configuration and replace the pending view")
	}
	if _, err := old.CheckSearchProjectionAt(repo, f.basis); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("old pending view was retained after runtime assembly: %v", err)
	}
	f.prepareAndRead(t, ws)
	view = ws.ReadView(nil)
	if err := view.Close(); err != nil {
		t.Fatal(err)
	}
	f.assertCanonical(t, ws)
	verified := ws.Plugins
	if err := ws.Plugins.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	source, _ := ws.Store.Get(f.id)
	writeConnectedPluginConfig(t, source, []consumerplugin.PluginSelection{})
	before = f.metadataReads.Load()
	if err := ws.VerifyIndexPlugins(); err != nil && kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("repeated verification returned an unrelated failure: %v", err)
	}
	if ws.Plugins != verified || f.metadataReads.Load() != before {
		t.Fatal("started root reloaded changed Repository configuration")
	}
	f.assertCanonical(t, ws)
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

func TestConnectedPluginConfigurationFailurePreservesManagementRecovery(t *testing.T) {
	for _, row := range []struct {
		name   string
		status int64
		code   kernel.ErrorCode
	}{{"forbidden", http.StatusForbidden, kernel.ErrForbidden}, {"unavailable", http.StatusServiceUnavailable, kernel.ErrTemporaryUnavailable}} {
		t.Run(row.name, func(t *testing.T) {
			f := newConnectedPluginFixture(t)
			f.metadataFault.Store(row.status)
			ws := f.openCold(t)
			before := f.metadataReads.Load()
			if err := ws.VerifyIndexPlugins(); err != nil {
				t.Fatalf("one failed connected source prevented management assembly: %v", err)
			}
			if f.metadataReads.Load() != before+1 || !ws.Plugins.ConfiguredFor(f.id) {
				t.Fatal("failed native metadata became an unconfigured source")
			}
			repo, err := ws.Reader.Require(f.id, kernel.ErrKnowledgeRefUnresolved)
			if err != nil {
				t.Fatal(err)
			}
			_, failure := ws.Index.CheckSearchProjectionAt(repo, f.basis)
			if kernel.CodeOf(failure) != row.code {
				t.Fatalf("native configuration failure lost its original reason: %v", failure)
			}
			frozen := f.frozen(t, ws)
			if err := ws.DatasetPreparation.Execute(context.Background(), frozen); err == nil || err.Error() != failure.Error() {
				t.Fatalf("preparation skipped or changed the failed configuration: %v", err)
			}
			result, err := ws.Index.SearchAtContext(context.Background(), repo, f.basis, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
			if err == nil || err.Error() != failure.Error() || !reflect.DeepEqual(result, retrieval.SearchResult{}) {
				t.Fatalf("failed configuration returned success, hits, or a different reason: result=%#v error=%v", result, err)
			}
			beforeRequests := f.requests.Load()
			connection, err := ws.GetConnection("alice", string(f.id))
			if err != nil || connection.Status != "READY" || connection.Head != f.basis || f.requests.Load() != beforeRequests {
				t.Fatalf("configuration failure blocked durable connection management: result=%#v error=%v", connection, err)
			}
			f.credential.Store("access:replacement-secret")
			rotated, err := ws.RotateConnectionCredential("alice", string(f.id), f.credential.Load().(string))
			if err != nil || rotated.Revision <= connection.Revision || rotated.Head != f.basis {
				t.Fatalf("metadata failure blocked credential recovery: result=%#v error=%v", rotated, err)
			}
			f.metadataFault.Store(0)
			before = f.metadataReads.Load()
			if err := ws.VerifyIndexPlugins(); err != nil {
				t.Fatal(err)
			}
			if f.metadataReads.Load() != before {
				t.Fatal("credential repair caused request-time configuration reload")
			}
			if _, err := ws.Index.CheckSearchProjectionAt(repo, f.basis); err == nil || err.Error() != failure.Error() {
				t.Fatalf("fixed failed running selection changed before a new assembly: %v", err)
			}
			if err := ws.Close(); err != nil {
				t.Fatal(err)
			}
			recovered := f.openCold(t)
			if err := recovered.VerifyIndexPlugins(); err != nil {
				t.Fatal(err)
			}
			f.prepareAndRead(t, recovered)
		})
	}
}
