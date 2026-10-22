package service

import (
	"context"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
)

func TestComparisonValidatesBothViewsBeforeAnyRead(t *testing.T) {
	calls := 0
	executor := ComparisonExecutor{Open: func(context.Context, objectmodel.ComparisonView, string, string) (objectmodel.ComparisonRead, error) {
		calls++
		return objectmodel.ComparisonRead{}, nil
	}}
	valid := objectmodel.ComparisonView{Repository: "kr://source/a", Commit: "c1"}
	for _, invalid := range []objectmodel.ComparisonView{{}, {Repository: valid.Repository}, {Commit: "c1"}, {Dataset: "d"}, {Dataset: "d", Revision: -1}, {Basis: "stored", Repository: valid.Repository, Commit: "c1"}, {Dataset: "d", Revision: 1, Basis: "stored"}} {
		_, err := executor.Execute(context.Background(), ComparisonRequest{Reference: "urn:kc:object:table/orders", From: valid, To: invalid})
		if kernel.CodeOf(err) != kernel.ErrUsageInvalid || calls != 0 {
			t.Fatalf("invalid second side read the first: %#v %v calls=%d", invalid, err, calls)
		}
	}
}

func TestComparisonRejectsResolverCoordinateOrSelectionSubstitution(t *testing.T) {
	view := objectmodel.ComparisonView{Repository: "kr://source/a", Commit: "c1"}
	basis := objectmodel.ReadBasis{Version: 1, Reference: "urn:kc:object:table/orders", Candidates: []objectmodel.ReadSource{{Repository: view.Repository, Commit: view.Commit, Binding: "origin"}}, Paths: []objectmodel.ReadPath{{Repository: view.Repository, Kind: "prefix"}}}
	for _, mutate := range []func(*objectmodel.ReadBasis){func(b *objectmodel.ReadBasis) { b.Candidates[0].Commit = "c2" }, func(b *objectmodel.ReadBasis) { b.Reference = "urn:kc:object:table/secret" }, func(b *objectmodel.ReadBasis) { b.Shape = "{ secret }" }} {
		altered := basis
		altered.Candidates = append([]objectmodel.ReadSource(nil), basis.Candidates...)
		mutate(&altered)
		calls := 0
		executor := ComparisonExecutor{Open: func(context.Context, objectmodel.ComparisonView, string, string) (objectmodel.ComparisonRead, error) {
			calls++
			return objectmodel.ComparisonRead{Basis: altered, Complete: true}, nil
		}}
		_, err := executor.Execute(context.Background(), ComparisonRequest{Reference: basis.Reference, From: view, To: view})
		if kernel.CodeOf(err) != kernel.ErrPreconditionFailed || calls != 1 {
			t.Fatalf("substitution accepted or second read attempted: %v calls=%d", err, calls)
		}
	}
}

func TestComparisonSecondSideFailureExposesNoPartialResult(t *testing.T) {
	view := objectmodel.ComparisonView{Basis: "retained"}
	basis := objectmodel.ReadBasis{Version: 1, Reference: "urn:kc:object:table/orders", Candidates: []objectmodel.ReadSource{{Repository: "kr://source/a", Commit: "c1", Binding: "origin"}}, Paths: []objectmodel.ReadPath{{Repository: "kr://source/a", Kind: "prefix"}}}
	calls := 0
	executor := ComparisonExecutor{Open: func(context.Context, objectmodel.ComparisonView, string, string) (objectmodel.ComparisonRead, error) {
		calls++
		if calls == 2 {
			return objectmodel.ComparisonRead{}, kernel.Fail(kernel.ErrForbidden, "revoked")
		}
		return objectmodel.ComparisonRead{Basis: basis, Complete: true}, nil
	}}
	out, err := executor.Execute(context.Background(), ComparisonRequest{Reference: basis.Reference, From: view, To: view})
	if kernel.CodeOf(err) != kernel.ErrForbidden || out.Reference != "" || out.Equal != nil || calls != 2 {
		t.Fatalf("partial comparison leaked: %#v %v", out, err)
	}
}
