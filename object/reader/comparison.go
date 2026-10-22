package reader

import (
	"context"

	"kc/kernel"
	objectmodel "kc/object"
)

// ReadComparisonBasis reads only the complete selected local units. Lookup
// must already enforce current authorization and the accepted publication's
// file scope. Dependencies are preserved as coordinates, never interpreted or
// opened here; unavailable interpretation does not make local state partial.
func ReadComparisonBasis(ctx context.Context, lookup MemberLookup, basis objectmodel.ReadBasis) (objectmodel.ComparisonRead, error) {
	if lookup == nil {
		return objectmodel.ComparisonRead{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "comparison requires a repository lookup")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return objectmodel.ComparisonRead{}, err
	}
	serving, err := openReadBasis(func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		repo, err := lookup(id)
		if err != nil {
			return nil, err
		}
		return objectmodel.BindContext(ctx, repo), nil
	}, basis, false)
	if err != nil {
		if ctx.Err() != nil {
			return objectmodel.ComparisonRead{}, ctx.Err()
		}
		return objectmodel.ComparisonRead{}, err
	}
	reference, _ := objectmodel.ParseReference(basis.Reference)
	refs := []objectmodel.Reference{reference}
	keyed := map[string]bool{}
	if basis.Shape != "" {
		shape, _ := objectmodel.ParseShape(basis.Shape)
		refs = nil
		for _, selection := range shape.Selections() {
			selected := reference
			selected.Locator = selection.Locator()
			keyed[selected.Locator] = selection.Keyed
			refs = append(refs, selected)
		}
	}
	sources := map[kernel.RepositoryID]objectmodel.ReadSource{}
	for _, source := range basis.Candidates {
		sources[source.Repository] = source
	}
	out := objectmodel.ComparisonRead{Basis: basis, Units: []objectmodel.ComparisonUnit{}}
	err = serving.eachRepository(func(id kernel.RepositoryID, commit kernel.CommitID, repo objectmodel.Repository) error {
		selected := map[string]objectmodel.Address{}
		for _, ref := range refs {
			if err := ctx.Err(); err != nil {
				return err
			}
			ref.Repository = id
			addresses, err := planRepositoryReference(repo, ref, commit)
			if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
				continue
			}
			if err != nil {
				return err
			}
			for _, address := range addresses {
				if keyed[ref.Locator] && address.Kind != objectmodel.KindMember {
					return kernel.Fail(kernel.ErrUsageInvalid, "comparison keys require member maintenance units")
				}
				key := objectmodel.AddressKey(address)
				if prior, read := selected[key]; read {
					if prior != address {
						return kernel.Fail(kernel.ErrPreconditionFailed, "comparison plans disagree about the same fixed unit")
					}
					continue
				}
				exact := objectmodel.Reference{Repository: id, Object: ref.Object, Locator: objectmodel.FormatPointer(unitTokens(address))}
				units, err := readRepositoryReferenceUnits(repo, exact, commit)
				if err != nil {
					return err
				}
				if len(units) != 1 || units[0].Address != address {
					return kernel.Fail(kernel.ErrPreconditionFailed, "comparison read changed its planned maintenance unit")
				}
				selected[key] = address
				declaration := objectmodel.DeclareUnits(units)[0]
				out.Units = append(out.Units, objectmodel.ComparisonUnit{UnitDeclaration: declaration, Source: sources[id], Value: units[0].Value})
			}
		}
		return nil
	})
	if err != nil {
		return objectmodel.ComparisonRead{}, err
	}
	out.Complete = true
	return out, nil
}
