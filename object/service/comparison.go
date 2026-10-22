package service

import (
	"context"

	"kc/kernel"
	objectmodel "kc/object"
)

type ComparisonRequest struct {
	Reference string                     `json:"reference"`
	Shape     string                     `json:"shape,omitempty"`
	From      objectmodel.ComparisonView `json:"from"`
	To        objectmodel.ComparisonView `json:"to"`
}

func (r ComparisonRequest) Validate() error {
	ref, err := objectmodel.ParseReference(r.Reference)
	if err != nil {
		return err
	}
	if r.Shape != "" {
		if ref.Locator != "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "comparison cannot combine a locator and shape")
		}
		if _, err := objectmodel.ParseShape(r.Shape); err != nil {
			return err
		}
	}
	if err := r.From.Validate(); err != nil {
		return err
	}
	return r.To.Validate()
}

// ComparisonExecutor opens both independent authorized views before exposing
// any comparison. Open must return complete local units, never observations.
type ComparisonExecutor struct {
	Open func(context.Context, objectmodel.ComparisonView, string, string) (objectmodel.ComparisonRead, error)
}

func (e ComparisonExecutor) Execute(ctx context.Context, r ComparisonRequest) (objectmodel.ObjectComparison, error) {
	if err := r.Validate(); err != nil {
		return objectmodel.ObjectComparison{}, err
	}
	if e.Open == nil {
		return objectmodel.ObjectComparison{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "object comparison service is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	sides := make([]objectmodel.ComparisonRead, 0, 2)
	for _, view := range []objectmodel.ComparisonView{r.From, r.To} {
		if err := ctx.Err(); err != nil {
			return objectmodel.ObjectComparison{}, err
		}
		side, err := e.Open(ctx, view, r.Reference, r.Shape)
		if err != nil {
			return objectmodel.ObjectComparison{}, err
		}
		// A resolver cannot silently change the requested object or selection. Use
		// the common scope canonicalization, without comparing side memberships.
		expected := side.Basis
		expected.Reference, expected.Shape = r.Reference, r.Shape
		want, err := expected.ScopeDigest()
		if err != nil {
			return objectmodel.ObjectComparison{}, err
		}
		got, err := side.Basis.ScopeDigest()
		if err != nil {
			return objectmodel.ObjectComparison{}, err
		}
		if want != got {
			return objectmodel.ObjectComparison{}, kernel.Fail(kernel.ErrPreconditionFailed, "comparison resolver changed the requested object selection")
		}
		if err := comparisonCoordinates(view, side.Basis); err != nil {
			return objectmodel.ObjectComparison{}, err
		}
		sides = append(sides, side)
	}
	if err := ctx.Err(); err != nil {
		return objectmodel.ObjectComparison{}, err
	}
	return objectmodel.CompareLocalReads(sides[0], sides[1])
}

func comparisonCoordinates(view objectmodel.ComparisonView, basis objectmodel.ReadBasis) error {
	if view.Repository != "" {
		if basis.Dataset != "" || basis.Temporary || len(basis.Candidates) != 1 || basis.Candidates[0].Repository != view.Repository || basis.Candidates[0].Commit != view.Commit {
			return kernel.Fail(kernel.ErrPreconditionFailed, "comparison resolver changed the fixed Repository view")
		}
	}
	if view.Dataset != "" && (basis.Dataset != view.Dataset || basis.Revision != view.Revision || basis.Temporary) {
		return kernel.Fail(kernel.ErrPreconditionFailed, "comparison resolver changed the fixed Dataset publication")
	}
	return nil
}
