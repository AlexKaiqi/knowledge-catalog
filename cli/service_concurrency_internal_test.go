package cli

import (
	"testing"
	"time"

	apphome "kc/home"
)

// Deployment serving serves read-only actions on the immutable per-request
// view (ws.ReadView with a private request journal), so independent reads
// share the read lock.
func TestTypedReadInvocationsShareTheReadLock(t *testing.T) {
	facade := &httpFacade{deployment: &apphome.DeploymentConfig{}}
	firstUnlock := facade.lockTypedInvocation("knowledge.read")
	defer firstUnlock()

	acquired := make(chan func(), 1)
	go func() { acquired <- facade.lockTypedInvocation("knowledge.search") }()
	select {
	case unlock := <-acquired:
		unlock()
	case <-time.After(time.Second):
		t.Fatal("independent typed reads were serialized")
	}
}

// Component/fixture serving installs the request journal on the shared Home
// for every invocation, so reads must serialize with each other and with
// mutations (race detector: observeHome → SetJournal data race).
func TestTypedFixtureReadsSerializeWithEachOther(t *testing.T) {
	facade := &httpFacade{}
	firstUnlock := facade.lockTypedInvocation("knowledge.read")
	defer firstUnlock()

	acquired := make(chan func(), 1)
	go func() { acquired <- facade.lockTypedInvocation("knowledge.search") }()
	select {
	case unlock := <-acquired:
		unlock()
		t.Fatal("fixture-mode read crossed another read on the shared home")
	case <-time.After(25 * time.Millisecond):
	}
}

func TestTypedMutationExcludesReadsUntilItFinishes(t *testing.T) {
	facade := &httpFacade{}
	writeUnlock := facade.lockTypedInvocation("writer.commit")
	acquired := make(chan func(), 1)
	go func() { acquired <- facade.lockTypedInvocation("knowledge.read") }()

	select {
	case unlock := <-acquired:
		unlock()
		t.Fatal("read crossed an in-process mutation")
	case <-time.After(25 * time.Millisecond):
	}
	writeUnlock()
	select {
	case unlock := <-acquired:
		unlock()
	case <-time.After(time.Second):
		t.Fatal("read stayed blocked after mutation completed")
	}
}

func TestTypedInvocationReadOnlyClassification(t *testing.T) {
	for _, action := range []string{
		"catalog.read", "catalog.audit.read", "knowledge.read", "knowledge.search",
		"knowledge.relations", "knowledge.provenance", "knowledge.history.read",
		"knowledge.schema.read", "knowledge.binding.resolve", "dataset.resolve",
		"writer.receipt.read", "admin.grants.read", "projection.read",
		"knowledge.access.describe", "operations.hooks.read", "operations.gates.read", "audit.read",
	} {
		if !typedInvocationReadOnly(action) {
			t.Errorf("%s should be read-only", action)
		}
	}
	for _, action := range []string{"writer.commit", "catalog.repositories.create", "dataset.manage", "projection.manage", "feedback.write"} {
		if typedInvocationReadOnly(action) {
			t.Errorf("%s should exclude concurrent reads", action)
		}
	}
}
