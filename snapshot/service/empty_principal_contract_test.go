package service

import (
	"context"
	"testing"

	"kc/kernel"
	"kc/snapshot/access"
)

func TestDebtSnapshotResourceGuardRejectsEmptyCallerBeforeCallback(t *testing.T) {
	called := false
	resources := Resources{Authorize: func(context.Context, Authorization) error { called = true; return nil }}
	err := resources.authorize(context.Background(), Authorization{Query: access.Query{Principal: "admin", Action: "file.read", Repo: "private"}})
	if kernel.CodeOf(err) != kernel.ErrUnauthenticated || called {
		t.Fatalf("query principal replaced an absent caller: called=%v error=%v", called, err)
	}
}
