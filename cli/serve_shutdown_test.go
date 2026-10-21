package cli_test

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kc/cli"
	indexcontrol "kc/declarativeindex/control"
	index "kc/discovery/projection"
	"kc/kernel"
)

// Exercise the real argv, OS signal and HTTP lifecycle together. The lakeFS
// server supplies a held object read; the adapter, Reader, maintenance worker,
// persistent work ledger and OpenSearch are the production implementations.
func TestServeSIGTERMCancelsMaintenanceAndRecoversOnRestart(t *testing.T) {
	if path := os.Getenv("KC_TEST_SERVE_SHUTDOWN_CONFIG"); path != "" {
		result := cli.Run([]string{"serve", "--config", path, "--listen", os.Getenv("KC_TEST_SERVE_SHUTDOWN_LISTEN")})
		if result.Status != 0 {
			t.Fatalf("serve returned %d: %s", result.Status, result.Stdout)
		}
		return
	}
	f := newServeShutdownFixture(t)
	address := serveShutdownAddress(t)
	f.armed.Store(true)
	first := startServeShutdownProcess(t, f.configPath, address)
	first.waitHTTP(t, address)
	select {
	case <-f.entered:
	case <-time.After(30 * time.Second):
		t.Fatalf("listening service never entered the held maintenance object read\n%s", first.output(t))
	}
	t.Logf("maintenance held in lakeFS GET %s with HTTP listener active", f.heldRequest)
	elapsed := first.stop(t)
	select {
	case <-f.canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("SIGTERM did not cancel the in-flight lakeFS HTTP request")
	}
	// No release is sent to the fixture on the successful path.
	f.armed.Store(false)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("exited service retained its listener: %v", err)
	}
	_ = listener.Close()
	ledger := indexcontrol.NewTargetStore(filepath.Join(f.config.CacheDir, "projections", "controller.db"))
	pending := serveShutdownTarget(t, ledger, f.repository.ID())
	if pending.Status != index.TargetPending || pending.LastError != "" || pending.DesiredCommit != f.commit || pending.AppliedCommit != "" {
		t.Fatalf("interrupted work was not retained without a false success/failure: %#v", pending)
	}
	desc := f.describe(t)
	if desc.BasisCommit != "" || desc.ObjectCount != 0 || desc.State == index.ProjectionStateReady {
		t.Fatalf("canceled cold rebuild published partial state: %#v", desc)
	}

	second := startServeShutdownProcess(t, f.configPath, address)
	second.waitHTTP(t, address)
	f.waitReady(t)
	read := asMap(t, body(t, kcRemote(t, "http://"+address, f.config.BootstrapPrincipal,
		"read", testObjectReference(t, string(f.repository.ID()), "runbook/payment-oncall", ""),
		"--repo", string(f.repository.ID()), "--commit", string(f.commit))))
	if read["commit"] != string(f.commit) || asMap(t, read["value"])["body"] != "切换支付流量前先检查冻结窗口" {
		t.Fatalf("restarted service lost the fixed object read: %#v", read)
	}
	second.stop(t)
	ready := serveShutdownTarget(t, ledger, f.repository.ID())
	if ready.Status != index.TargetReady || ready.LastError != "" || ready.DesiredCommit != f.commit || ready.AppliedCommit != f.commit {
		t.Fatalf("restart did not finish the original pending target: %#v", ready)
	}
	desc = f.describe(t)
	if desc.BasisCommit != f.commit || desc.State != index.ProjectionStateReady || desc.ObjectCount == 0 {
		t.Fatalf("restart did not publish a usable complete projection: %#v", desc)
	}
	evidence, _ := json.Marshal(map[string]any{
		"shutdownMilliseconds": elapsed.Milliseconds(), "requestCanceled": true,
		"pending": pending, "recovered": ready, "fixedCommit": f.commit,
		"readCommit": read["commit"], "manualRelease": false, "forcedKill": false,
	})
	t.Logf("SIGTERM local evidence: %s", evidence)
}

func serveShutdownTarget(t *testing.T, store *indexcontrol.TargetStore, id kernel.RepositoryID) index.ProjectionTarget {
	t.Helper()
	targets, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if target.Repository == id {
			return target
		}
	}
	t.Fatalf("no durable target for %s: %#v", id, targets)
	return index.ProjectionTarget{}
}
