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

func TestIngestRejectsMalformedOrInvalidValueSource(t *testing.T) {
	contents := []string{
		"---\nobject_id: Service:orders\naspect_name: health\nkind: ASPECT\nvalue_source: {bad-json}\n---\nnull\n",
		"---\nobject_id: Service:orders\naspect_name: health\nkind: ASPECT\nvalue_source: {\"kind\":\"binding\",\"binding\":{\"mode\":\"state\"}}\n---\nnull\n",
	}
	for _, content := range contents {
		idx := NewTree()
		if code := kernel.CodeOf(Ingest(idx, Parse(content), "objects/orders/health.json")); code != kernel.ErrUsageInvalid {
			t.Fatalf("invalid declaration must fail closed, got %s", code)
		}
	}
}
