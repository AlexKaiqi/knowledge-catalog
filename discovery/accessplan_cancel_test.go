package discovery_test

import (
	"context"
	"errors"
	"testing"

	retrieval "kc/discovery"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

// A caller's transport timeout must stop plan compilation. The number of
// pinned members is caller data, so an uncancellable plan lets one request
// outlive its client and hold the serving goroutine indefinitely.
func TestPlanAccessContextStopsWhenCanceled(t *testing.T) {
	pin := reader.DatasetPin{
		SetID:    "canceled",
		Revision: 1,
		Repositories: map[kernel.RepositoryID]kernel.CommitID{
			"kr://canceled/a": "c1",
			"kr://canceled/b": "c1",
		},
		Items: []reader.DatasetItem{
			{Repository: "kr://canceled/a", Prefix: "a/"},
			{Repository: "kr://canceled/b", Prefix: "b/"},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	lookup := func(kernel.RepositoryID) (objectmodel.Repository, error) {
		t.Fatal("canceled plan opened a member")
		return nil, nil
	}
	_, err := retrieval.PlanAccessContext(ctx, lookup, pin)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled plan did not stop: %v", err)
	}
}
