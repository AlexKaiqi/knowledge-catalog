package invocation

import (
	"context"
	"maps"
	"strconv"

	kchome "kc/home"
	"kc/invoke"
	"kc/kernel"
	objectserving "kc/liveaccess"
	objectmodel "kc/object"
	"kc/object/reader"
	objectservice "kc/object/service"
)

// A comparison is two ordinary object-read authorizations, each with its own
// fixed scope and hooks. No merged scope may borrow the other side's grants.
func executeObjectComparison(ctx context.Context, action string, flags map[string]invoke.FlagValue, state objectserving.StateLookup, opened *kchome.Home, home string, observation *invoke.ObservationTelemetry) (any, error) {
	if action != "object.read" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "object comparison requires object read authorization")
	}
	from, okFrom := flags["from"].(objectmodel.ComparisonView)
	to, okTo := flags["to"].(objectmodel.ComparisonView)
	if !okFrom || !okTo {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "object comparison requires two typed fixed views")
	}
	request := objectservice.ComparisonRequest{Reference: invoke.FlagString(flags, "reference"), Shape: invoke.FlagString(flags, "shape"), From: from, To: to}
	return (objectservice.ComparisonExecutor{Open: func(ctx context.Context, view objectmodel.ComparisonView, reference, shape string) (objectmodel.ComparisonRead, error) {
		// Carry identity and evidence stamps, never the outer reference binding or
		// another side's version and authorization caches.
		side := maps.Clone(flags)
		for _, key := range []string{"from", "to", "reference", "shape", "object", referenceBindingFlag, referenceCommitFlag, referenceCandidatesFlag, referenceHoldingsFlag, referencePlannedFlag} {
			delete(side, key)
		}
		for _, key := range []string{"repo", "dataset", "revision", "commit", "ref", "basis", "pin", "dataset-file", "source", WorkspaceDefinitionFlag} {
			if _, exists := side[key]; exists {
				return objectmodel.ComparisonRead{}, kernel.Fail(kernel.ErrUsageInvalid, "comparison views cannot be combined with --%s", key)
			}
		}
		side["reference"], side["shape"] = reference, shape
		switch {
		case view.Basis != "":
			side["basis"] = view.Basis
		case view.Repository != "":
			side["repo"], side["commit"] = string(view.Repository), string(view.Commit)
		default:
			side["dataset"], side["revision"] = view.Dataset, strconv.Itoa(view.Revision)
		}
		value, err := ExecuteApplicationOperation(ctx, "object-comparison-side", "object.read", Command{Stage: invoke.StageGoverned, Run: readObjectComparisonSide}, side, state, opened, home, observation)
		if err != nil {
			return objectmodel.ComparisonRead{}, err
		}
		read, ok := value.(objectmodel.ComparisonRead)
		if !ok {
			return objectmodel.ComparisonRead{}, kernel.Fail(kernel.ErrPreconditionFailed, "comparison read returned an invalid local result")
		}
		return read, nil
	}}).Execute(ctx, request)
}

func readObjectComparisonSide(cx *Invocation) (any, error) {
	ref, err := objectmodel.ParseReference(cx.Flag("reference"))
	if err != nil {
		return nil, err
	}
	var basis objectmodel.ReadBasis
	if cx.readBasis != nil && cx.readBasis.fixed != nil {
		basis = *cx.readBasis.fixed
	}
	if ServingWorkspace(cx.Flags) {
		serving, _, err := OpenCompleteServing(cx, string(ref.Object))
		if err != nil {
			return nil, err
		}
		if basis.Version != 0 {
			if err := checkComparisonPublication(basis, serving.Pin()); err != nil {
				return nil, err
			}
		} else {
			basis, err = serving.LocalReadBasis(ref, cx.Flag("shape"))
			if err != nil {
				return nil, err
			}
		}
	} else if basis.Version == 0 {
		id, commit, err := PinCommit(cx.WS, cx.Flags)
		if err != nil {
			return nil, err
		}
		commits := map[kernel.RepositoryID]kernel.CommitID{id: commit}
		serving := reader.Open(func(id kernel.RepositoryID) (objectmodel.Repository, error) {
			return cx.WS.Reader.Require(id, kernel.ErrObjectRefUnresolved)
		}, reader.DatasetPin{Repositories: commits, Items: reader.WholeRepositoryItems(commits)})
		basis, err = serving.LocalReadBasis(ref, cx.Flag("shape"))
		if err != nil {
			return nil, err
		}
	}
	// Prove every candidate readable, including an empty contributor. Withheld
	// content cannot become absence or a complete comparison of a smaller set.
	selections := []objectmodel.Reference{ref}
	if basis.Shape != "" {
		shape, _ := objectmodel.ParseShape(basis.Shape)
		selections = nil
		for _, selection := range shape.Selections() {
			selected := ref
			selected.Locator = selection.Locator()
			selections = append(selections, selected)
		}
	}
	for _, source := range basis.Candidates {
		for _, selection := range selections {
			if !AllowedRepoAspectRead(cx.Home, cx.Flags, string(source.Repository), string(ref.Object), referenceAspect(selection)) {
				return nil, kernel.Fail(kernel.ErrForbidden, "comparison requires the complete authorized local selection")
			}
		}
	}
	return reader.ReadComparisonBasis(cx.Context, func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return cx.WS.Reader.Require(id, kernel.ErrObjectRefUnresolved)
	}, basis)
}

func checkComparisonPublication(basis objectmodel.ReadBasis, pin reader.DatasetPin) error {
	expected := basis
	expected.Dataset, expected.Revision, expected.Temporary = pin.SetID, pin.Revision, pin.Temporary
	candidates := map[kernel.RepositoryID]kernel.CommitID{}
	dependencies := map[kernel.RepositoryID]kernel.CommitID{}
	for _, source := range basis.Candidates {
		candidates[source.Repository] = source.Commit
	}
	for _, source := range basis.Dependencies {
		dependencies[source.Repository] = source.Commit
	}
	for _, failure := range basis.UnresolvedDependencies {
		if failure.Commit != "" {
			dependencies[failure.Repository] = failure.Commit
		}
	}
	expected.Paths = nil
	for _, item := range pin.Items {
		expected.Paths = append(expected.Paths, objectmodel.ReadPath{Repository: item.Repository, Kind: item.Kind, Prefix: item.Prefix, File: item.File})
	}
	before, err := basis.ScopeDigest()
	if err != nil {
		return err
	}
	after, err := expected.ScopeDigest()
	if err != nil {
		return err
	}
	if basis.Dataset != pin.SetID || basis.Revision != pin.Revision || basis.Temporary != pin.Temporary || before != after || !maps.Equal(candidates, pin.Repositories) || !maps.Equal(dependencies, pin.Dependencies) {
		return kernel.Fail(kernel.ErrPreconditionFailed, "comparison basis no longer matches its accepted Dataset publication")
	}
	return nil
}
