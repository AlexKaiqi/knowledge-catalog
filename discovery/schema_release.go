package discovery

import (
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

// describeSchemaRelease interprets a reference already read from a selected
// declaration or verified adoption. A Dataset must first select the Schema's
// identity at its frozen source version; only then may that reference read an
// explicit older release of the same identity from the same authority. This
// does not open historical object reads on the Dataset member itself.
func describeSchemaRelease(repo objectmodel.Repository, commit kernel.CommitID, ref objectmodel.ParsedSchemaRef) (reader.SchemaReport, bool, error) {
	if scoped, ok := repo.(interface {
		DatasetScope() (reader.MemberLookup, reader.DatasetPin)
	}); ok {
		lookup, pin := scoped.DatasetScope()
		selected, err := selectedSchemaSource(lookup, pin, ref)
		if err != nil {
			return reader.SchemaReport{}, false, err
		}
		current := ref.Commit == "" || ref.Commit == selected.Commit
		member, err := reader.Open(lookup, pin).Member(selected.Repository)
		if err != nil {
			return reader.SchemaReport{}, false, err
		}
		at := selected.Commit
		if !current {
			member, err = lookup(selected.Repository)
			if err != nil {
				return reader.SchemaReport{}, false, err
			}
			at = ref.Commit
		}
		report, err := reader.DescribeRepoSchema(member, at, ref.Object)
		return report, current, err
	}
	if ref.Repository != "" && ref.Repository != repo.ID() {
		return reader.SchemaReport{}, false, kernel.Fail(kernel.ErrObjectRefUnresolved, "Schema %s is outside this repository", ref.Object)
	}
	current := ref.Commit == "" || ref.Commit == commit
	if !current {
		commit = ref.Commit
	}
	report, err := reader.DescribeRepoSchema(repo, commit, ref.Object)
	return report, current, err
}

func selectedSchemaSource(lookup reader.MemberLookup, pin reader.DatasetPin, ref objectmodel.ParsedSchemaRef) (objectmodel.FederatedValue, error) {
	scope := reader.Open(lookup, pin)
	if ref.Repository != "" {
		member, err := scope.Member(ref.Repository)
		if err != nil {
			return objectmodel.FederatedValue{}, err
		}
		_, err = member.Read(ref.Object, pin.Repositories[ref.Repository])
		return objectmodel.FederatedValue{Repository: ref.Repository, Commit: pin.Repositories[ref.Repository]}, err
	}
	values, err := scope.Read(ref.Object, nil)
	if err != nil {
		return objectmodel.FederatedValue{}, err
	}
	if len(values) == 0 {
		return objectmodel.FederatedValue{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "Schema %s is outside the Dataset declaration namespace", ref.Object)
	}
	if len(values) != 1 {
		return objectmodel.FederatedValue{}, kernel.Fail(kernel.ErrObjectIDConflict, "Schema %s has several Dataset holders", ref.Object)
	}
	return values[0], nil
}
