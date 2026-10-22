package reader

import (
	"kc/kernel"
	objectmodel "kc/object"
)

// OpenHistoryBasis reopens a fixed whole-repository selection without reading
// interpretation dependencies. Lookup must enforce current source history
// permission and publication scope before returning each repository.
func OpenHistoryBasis(lookup MemberLookup, basis objectmodel.ReadBasis) (*Serving, error) {
	if err := basis.Validate(); err != nil {
		return nil, err
	}
	reference, _ := objectmodel.ParseReference(basis.Reference)
	if basis.Shape != "" || reference.Locator != "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "object history requires a whole object selection")
	}
	roots := map[kernel.RepositoryID]bool{}
	for _, path := range basis.Paths {
		if path.Kind == "prefix" && path.Prefix == "" {
			roots[path.Repository] = true
		}
	}
	for _, source := range basis.Candidates {
		if !roots[source.Repository] {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "history source is outside the published whole-repository scope")
		}
	}
	return openReadBasis(lookup, basis, false)
}
