package knowledge_test

import (
	"testing"

	"kc/knowledge"
)

func TestObjectLogQueryPageSizeTreatsZeroAsDefault(t *testing.T) {
	empty := knowledge.ObjectLogQuery{}
	if empty.PageSize() != knowledge.DefaultObjectLogLimit {
		t.Fatalf("empty query page size = %d", empty.PageSize())
	}
	zero := knowledge.ObjectLogQuery{Limit: 0}
	if zero.PageSize() != knowledge.DefaultObjectLogLimit {
		t.Fatalf("limit 0 page size = %d", zero.PageSize())
	}
	negative := knowledge.ObjectLogQuery{Limit: -1}
	if negative.PageSize() != knowledge.DefaultObjectLogLimit {
		t.Fatalf("negative limit must not mean unbounded: %d", negative.PageSize())
	}
	explicit := knowledge.ObjectLogQuery{Limit: 3}
	if explicit.PageSize() != 3 {
		t.Fatalf("explicit page size = %d", explicit.PageSize())
	}
}
