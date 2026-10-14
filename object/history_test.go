package object_test

import (
	"testing"

	objectmodel "kc/object"
)

func TestObjectLogQueryPageSizeTreatsZeroAsDefault(t *testing.T) {
	empty := objectmodel.ObjectLogQuery{}
	if empty.PageSize() != objectmodel.DefaultObjectLogLimit {
		t.Fatalf("empty query page size = %d", empty.PageSize())
	}
	zero := objectmodel.ObjectLogQuery{Limit: 0}
	if zero.PageSize() != objectmodel.DefaultObjectLogLimit {
		t.Fatalf("limit 0 page size = %d", zero.PageSize())
	}
	negative := objectmodel.ObjectLogQuery{Limit: -1}
	if negative.PageSize() != objectmodel.DefaultObjectLogLimit {
		t.Fatalf("negative limit must not mean unbounded: %d", negative.PageSize())
	}
	explicit := objectmodel.ObjectLogQuery{Limit: 3}
	if explicit.PageSize() != 3 {
		t.Fatalf("explicit page size = %d", explicit.PageSize())
	}
}
