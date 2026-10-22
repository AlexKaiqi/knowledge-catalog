package invocation

import (
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
)

func TestObjectReadFreezesDependencyCandidatesAcrossSelections(t *testing.T) {
	absent := newDescriptorScopeStore("kr://descriptor/absent", nil)
	f := newDescriptorScopeFixture(t, absent)
	f.allow(t, f.primary.id, f.ops.id, absent.id)
	f.cx.readBasis = &objectReadBasis{}
	first, err := requestDescriptors(f.cx, f.primary.id, "v1")("ops/stats")
	if err != nil || first.Commit != "v1" {
		t.Fatalf("first selection: %#v %v", first, err)
	}
	f.ops.advance("v2", descriptorScopeValues("new interpretation"))
	absent.advance("v2", descriptorScopeValues("newly conflicting holder"))
	second, err := requestDescriptors(f.cx, f.primary.id, "v1")("ops/monitor")
	if err != nil || second.Commit != "v1" || second.Value != "one" {
		t.Fatalf("one object request mixed interpretation versions: %#v %v", second, err)
	}
	dependencies, failures := f.cx.readBasis.dependencies()
	if len(failures) != 0 || len(dependencies) != 2 || dependencies[f.ops.id] != "v1" || dependencies[absent.id] != "v1" {
		t.Fatalf("lost contributing or absent dependency candidate: %#v %#v", dependencies, failures)
	}
	if heads, _ := f.ops.counts("ops/stats"); heads != 1 {
		t.Fatalf("dependency head fetched %d times", heads)
	}
	// A subsequent actual invocation gets fresh coordinates and observes the
	// newly introduced ambiguity; the old request's cache is not a global one.
	f.cx.readBasis = &objectReadBasis{}
	if _, err := requestDescriptors(f.cx, f.primary.id, "v1")("ops/stats"); kernel.CodeOf(err) != kernel.ErrObjectIDConflict {
		t.Fatalf("new invocation failed to observe a new holder: %v", err)
	}
}

func TestFixedInterpretationUsesOnlyOriginalCoordinatesAndCurrentPermission(t *testing.T) {
	outside := newDescriptorScopeStore("kr://descriptor/new", descriptorScopeValues("outside"))
	f := newDescriptorScopeFixture(t, outside)
	f.allow(t, f.primary.id, f.ops.id, outside.id)
	f.ops.advance("v2", descriptorScopeValues("new interpretation"))
	f.cx.readBasis = &objectReadBasis{fixed: &objectmodel.ReadBasis{
		Dependencies: []objectmodel.ReadSource{{Repository: f.ops.id, Commit: "v1", Binding: "ops-authority"}},
	}}
	at, err := requestDescriptors(f.cx, f.primary.id, "v1")("ops/stats")
	if err != nil || at.Commit != "v1" || at.Value != "one" {
		t.Fatalf("fixed interpretation followed latest: %#v %v", at, err)
	}
	if heads, _ := f.ops.counts("ops/stats"); heads != 0 {
		t.Fatalf("fixed interpretation fetched head %d times", heads)
	}
	if heads, reads := outside.counts("ops/stats"); heads != 0 || reads != 0 {
		t.Fatalf("fixed interpretation discovered an outside source: %d %d", heads, reads)
	}
	f.allow(t, f.primary.id, outside.id)
	if _, err := requestDescriptors(f.cx, f.primary.id, "v1")("ops/monitor"); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("fixed interpretation bypassed current permission: %v", err)
	}
	if heads, reads := f.ops.counts("ops/monitor"); heads != 0 || reads != 0 {
		t.Fatalf("revoked fixed dependency contacted: %d %d", heads, reads)
	}
}

func TestObjectReadPreservesUnresolvedInterpretationCandidate(t *testing.T) {
	f := newDescriptorScopeFixture(t)
	f.cx.readBasis = &objectReadBasis{}
	f.ops.headErr = kernel.Fail(kernel.ErrTemporaryUnavailable, "dependency offline")
	if _, err := requestDescriptors(f.cx, f.primary.id, "v1")("ops/stats"); kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Fatalf("dependency failure changed: %v", err)
	}
	commits, failures := f.cx.readBasis.dependencies()
	if len(commits) != 0 || len(failures) != 1 || failures[0].Repository != f.ops.id || failures[0].Code != kernel.ErrTemporaryUnavailable {
		t.Fatalf("unresolved dependency disappeared from basis: %#v %#v", commits, failures)
	}
}
