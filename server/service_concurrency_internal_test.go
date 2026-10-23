package server

import (
	inv "kc/invocation"
	"testing"
	"time"

	apphome "kc/home"
)

// Admission is a shutdown barrier, not a mutual exclusion: concurrent
// mutations are decided by authority CAS, and stamps live on request views,
// so in-flight requests of any kind never wait for each other.
func TestTypedInvocationsDoNotWaitForEachOther(t *testing.T) {
	for _, facade := range []*httpFacade{{}, {deployment: &apphome.DeploymentConfig{}}} {
		first, admitted := facade.admitRequest()
		if !admitted {
			t.Fatal("open facade refused a request")
		}
		entered := make(chan func(), 1)
		go func() { entered <- facade.enterRequest() }()
		select {
		case release := <-entered:
			release()
		case <-time.After(time.Second):
			t.Fatal("a second in-flight request waited for the first")
		}
		first()
	}
}

func TestCloseWaitsForInFlightRequests(t *testing.T) {
	facade := &httpFacade{}
	release, admitted := facade.admitRequest()
	if !admitted {
		t.Fatal("open facade refused a request")
	}
	closed := make(chan error, 1)
	go func() { closed <- facade.closeReadHome() }()
	select {
	case <-closed:
		t.Fatal("close finished while a request was still in flight")
	case <-time.After(25 * time.Millisecond):
	}
	if _, admitted := facade.admitRequest(); admitted {
		t.Fatal("a closing facade admitted a new request")
	}
	release()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("close stayed blocked after the in-flight request finished")
	}
}

func TestTypedInvocationReadOnlyClassification(t *testing.T) {
	for _, action := range []string{
		"inventory.read", "registry.audit.read", "object.read", "object.search",
		"object.relations", "object.provenance", "object.history.read",
		"object.schema.read", "object.binding.resolve", "dataset.resolve",
		"writer.receipt.read", "admin.grants.read", "index.read",
		"object.access.describe", "operations.hooks.read", "operations.gates.read", "audit.read",
	} {
		if !inv.TypedInvocationReadOnly(action) {
			t.Errorf("%s should be read-only", action)
		}
	}
	for _, action := range []string{"writer.commit", "repositories.create", "dataset.manage", "index.manage", "feedback.write"} {
		if inv.TypedInvocationReadOnly(action) {
			t.Errorf("%s should not run on the immutable read view", action)
		}
	}
}
