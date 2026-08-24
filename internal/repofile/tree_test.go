package repofile

import (
	"strings"
	"testing"

	"kc/kernel"
	"kc/repository"
)

func TestApplyRejectsPathHintThatSnapshotReaderCannotDiscover(t *testing.T) {
	idx := NewTree()
	op := repository.Operation{
		Op:       repository.OpPut,
		Address:  kernel.Address{Kind: kernel.KindAspect, ObjectID: "Service:payment-api", AspectName: "observed"},
		Value:    map[string]any{"status": "healthy"},
		PathHint: "service",
	}
	err := Apply(idx, op, nil, map[string]string{}, map[string]struct{}{})
	if err == nil || !strings.Contains(err.Error(), "readable knowledge file extension") {
		t.Fatalf("expected unreadable pathHint rejection, got %v", err)
	}

	op.PathHint = "service.json"
	if err := Apply(idx, op, nil, map[string]string{}, map[string]struct{}{}); err != nil {
		t.Fatalf("valid knowledge pathHint was rejected: %v", err)
	}
}
