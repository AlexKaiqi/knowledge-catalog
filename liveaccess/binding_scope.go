package liveaccess

import (
	"sort"
	"strings"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

// ResolveBinding interprets the permitted declarations at the accepted source
// versions. It performs no runtime access; unavailable member targets are omitted.
func ResolveBinding(scope *reader.Serving, address objectmodel.Address) ([]ResolvedBinding, error) {
	pin := scope.Pin()
	ids := make([]kernel.RepositoryID, 0, len(pin.Repositories))
	for id := range pin.Repositories {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := []ResolvedBinding{}
	for _, id := range ids {
		binding, err := ResolveBindingAt(scope, id, address)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrCapabilityUnsatisfied || kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
				continue
			}
			return nil, err
		}
		out = append(out, binding)
	}
	return out, nil
}

// ResolveBindingAt uses the generic Object member scope for all auxiliary
// Schema and Descriptor reads before interpreting LiveAccess declarations.
func ResolveBindingAt(scope *reader.Serving, repository kernel.RepositoryID, address objectmodel.Address) (ResolvedBinding, error) {
	commit, ok := scope.Pin().Repositories[repository]
	if !ok {
		return ResolvedBinding{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "repository %s is not in dataset", repository)
	}
	allowed, err := scope.Contains(repository, address.ObjectID)
	if err != nil {
		return ResolvedBinding{}, err
	}
	if !allowed {
		return ResolvedBinding{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "address %s is not in dataset", address.ObjectID)
	}
	repo, err := scope.Member(repository)
	if err != nil {
		return ResolvedBinding{}, err
	}
	return ResolveRepoBinding(repo, commit, address)
}

func schemaAccessOrigin(repo objectmodel.Repository, commit kernel.CommitID, schemaRef string) (string, error) {
	schemaRef = strings.TrimSpace(schemaRef)
	if schemaRef == "" {
		return "", kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Bound State requires a schema_ref that declares origin")
	}
	parsed, ok := objectmodel.ParseSchemaRef(schemaRef)
	if !ok {
		return "", kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "schema_ref %q is not a pinned schema object", schemaRef)
	}
	if parsed.Repository != "" && parsed.Repository != repo.ID() {
		return "", kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "schema_ref %q is not in this repository", schemaRef)
	}
	at := commit
	if parsed.Commit != "" {
		at = parsed.Commit
	}
	value, err := repo.Read(parsed.Object, at)
	if err != nil {
		return "", kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "schema_ref %q does not resolve to a schema object", schemaRef)
	}
	definition, err := objectmodel.ParseSchemaDefinition(parsed.Object, value.Value)
	if err != nil {
		return "", err
	}
	if definition.Origin == "" {
		return "", kernel.Fail(kernel.ErrCapabilityUnsatisfied, "schema %s does not declare origin", parsed.Object)
	}
	return definition.Origin, nil
}
