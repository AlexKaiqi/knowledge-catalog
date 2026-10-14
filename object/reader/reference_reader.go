package reader

import (
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
)

// ReadReference plans the selected content before reading maintenance units.
// Its commit is already fixed by the calling task service.
func (r *Reader) ReadReference(ref objectmodel.Reference, commit kernel.CommitID) (value objectmodel.ObjectValue, err error) {
	defer func() {
		err = r.note("read-reference", map[string]any{"repositoryId": string(ref.Repository), "object": string(ref.Object), "commit": string(commit), "locator": ref.Locator}, err)
	}()
	if _, err := objectmodel.FormatReference(ref); err != nil {
		return objectmodel.ObjectValue{}, err
	}
	repo, err := r.repoByID(ref.Repository)
	if err != nil {
		return objectmodel.ObjectValue{}, err
	}
	return readHydratedReference(r.hydration(), repo, ref, commit)
}

// A root reference selects the whole object, so it may reuse the existing
// exact-basis hydrator after the complete Dataset scope check. A locator keeps
// the selected-unit path and never reads a whole object merely to trim it.
func readHydratedReference(h objectmodel.Hydrator, repo objectmodel.Repository, ref objectmodel.Reference, commit kernel.CommitID) (objectmodel.ObjectValue, error) {
	if h == nil || ref.Locator != "" {
		return readRepositoryReference(repo, ref, commit)
	}
	if _, err := objectmodel.FormatReference(ref); err != nil {
		return objectmodel.ObjectValue{}, err
	}
	if ref.Repository != repo.ID() {
		return objectmodel.ObjectValue{}, kernel.Fail(kernel.ErrUsageInvalid, "reference source does not match repository")
	}
	value, err := readHydrated(h, repo, ref.Object, commit)
	if err != nil {
		return objectmodel.ObjectValue{}, err
	}
	value.Digest = kernel.CanonicalDigest(value.Value)
	return describeReference(value, ref)
}

func readRepositoryReference(repo objectmodel.Repository, ref objectmodel.Reference, commit kernel.CommitID) (objectmodel.ObjectValue, error) {
	if _, err := objectmodel.FormatReference(ref); err != nil {
		return objectmodel.ObjectValue{}, err
	}
	if ref.Repository != repo.ID() {
		return objectmodel.ObjectValue{}, kernel.Fail(kernel.ErrUsageInvalid, "reference source does not match repository")
	}
	if port, ok := repo.(objectmodel.ReferenceReader); ok {
		value, err := port.ReadReference(ref, commit)
		if err != nil {
			return objectmodel.ObjectValue{}, err
		}
		if value.Repository != ref.Repository || value.ObjectRef.Repository != ref.Repository || value.ObjectRef.Object != ref.Object || value.Address.ObjectID != ref.Object || value.Commit != commit {
			return objectmodel.ObjectValue{}, kernel.Fail(kernel.ErrPreconditionFailed, "reference read changed source identity or fixed basis")
		}
		return value, nil
	}
	if ref.Locator != "" {
		return objectmodel.ObjectValue{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository cannot plan selected reference reads")
	}
	value, err := repo.Read(ref.Object, commit)
	if err != nil {
		return objectmodel.ObjectValue{}, err
	}
	value.Digest = kernel.CanonicalDigest(value.Value)
	return describeReference(value, ref)
}

func describeReference(value objectmodel.ObjectValue, ref objectmodel.Reference) (objectmodel.ObjectValue, error) {
	raw, err := objectmodel.FormatReference(ref)
	if err != nil {
		return objectmodel.ObjectValue{}, err
	}
	value.Reference, value.Locator = raw, ref.Locator
	return value, nil
}

func (r *treeRepository) ReadReference(ref objectmodel.Reference, commit kernel.CommitID) (objectmodel.ObjectValue, error) {
	if ref.Repository != r.ID() {
		return objectmodel.ObjectValue{}, kernel.Fail(kernel.ErrUsageInvalid, "reference source does not match repository")
	}
	if ref.Locator == "" {
		value, err := r.Read(ref.Object, commit)
		if err != nil {
			return objectmodel.ObjectValue{}, err
		}
		value.Digest = kernel.CanonicalDigest(value.Value)
		return describeReference(value, ref)
	}
	return r.ReadReferenceAtPaths(ref, commit, nil)
}

// ReadReferenceAtPaths limits selected unit bodies to an already accepted file
// scope. A nil path list is unrestricted; an empty non-nil list admits nothing.
func (r *treeRepository) ReadReferenceAtPaths(ref objectmodel.Reference, commit kernel.CommitID, paths []string) (objectmodel.ObjectValue, error) {
	units, err := r.selectedReferenceUnits(ref, commit, paths)
	if err != nil {
		return objectmodel.ObjectValue{}, err
	}
	value, err := assembleObjectValue(r.ID(), ref.Object, commit, units)
	if err != nil {
		return objectmodel.ObjectValue{}, err
	}
	value.Value, err = objectmodel.SelectPointer(value.Value, ref.Locator)
	if err != nil {
		return objectmodel.ObjectValue{}, err
	}
	tokens, _ := objectmodel.PointerTokens(ref.Locator)
	if len(units) == 1 && tokenPrefix(unitTokens(units[0].Address), tokens) {
		value.Address, value.Digest = units[0].Address, units[0].Digest
	}
	return describeReference(value, ref)
}

func (r *treeRepository) selectedReferenceUnits(ref objectmodel.Reference, commit kernel.CommitID, paths []string) ([]repofile.Unit, error) {
	if ref.Repository != r.ID() {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "reference source does not match repository")
	}
	tokens, err := objectmodel.PointerTokens(ref.Locator)
	if err != nil {
		return nil, err
	}
	locations, err := repofile.ReadObjectUnitLocations(r.tree, ref.Object, commit)
	if err != nil {
		return nil, err
	}
	selected, err := selectReferenceLocations(locations, tokens, paths)
	if err != nil {
		if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved && len(tokens) > 0 && tokens[0] == objectmodel.ReadmeAspect {
			for _, location := range locations {
				prefix := unitTokens(location.Address)
				if tokenPrefix(prefix, tokens) || tokenPrefix(tokens, prefix) {
					return nil, err
				}
			}
			address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: ref.Object, AspectName: objectmodel.ReadmeAspect}
			unit, found, readmeErr := r.readmeAddressUnit(address, commit, paths)
			if readmeErr != nil {
				return nil, readmeErr
			}
			if found {
				return []repofile.Unit{unit}, nil
			}
		}
		return nil, err
	}
	return r.readLocatedUnits(selected, ref.Object, commit)
}

func unitTokens(address objectmodel.Address) []string {
	if objectmodel.IsEntityBlob(address) {
		return nil
	}
	tokens := []string{address.AspectName}
	if address.MemberKey != "" {
		tokens = append(tokens, address.MemberKey)
	}
	return tokens
}

func tokenPrefix(prefix, tokens []string) bool {
	if len(prefix) > len(tokens) {
		return false
	}
	for i, token := range prefix {
		if tokens[i] != token {
			return false
		}
	}
	return true
}

func selectReferenceLocations(locations []repofile.ObjectUnitLocation, tokens, paths []string) ([]repofile.ObjectUnitLocation, error) {
	selected := make([]repofile.ObjectUnitLocation, 0)
	for _, location := range locations {
		prefix := unitTokens(location.Address)
		if !tokenPrefix(prefix, tokens) && !tokenPrefix(tokens, prefix) {
			continue
		}
		if paths != nil && !containsPath(paths, location.Path) {
			return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "selected content is outside dataset file scope")
		}
		selected = append(selected, location)
	}
	if len(selected) == 0 {
		return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "reference does not select published maintenance content")
	}
	return selected, nil
}

func (r *treeRepository) readLocatedUnits(locations []repofile.ObjectUnitLocation, objectID objectmodel.ObjectID, commit kernel.CommitID) ([]repofile.Unit, error) {
	paths := make([]string, 0, len(locations))
	for _, location := range locations {
		paths = append(paths, location.Path)
	}
	units, err := readObjectUnitsAtPaths(r.tree, r.ID(), paths, objectID, commit)
	if err != nil {
		return nil, err
	}
	if len(units) != len(locations) {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "reference unit metadata does not match stored object")
	}
	for i, unit := range units {
		if objectmodel.AddressKey(unit.Address) != objectmodel.AddressKey(locations[i].Address) {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "reference maintenance coordinates disagree with %s", locations[i].Path)
		}
	}
	return units, nil
}

type referencePathsReader interface {
	ReadReferenceAtPaths(objectmodel.Reference, kernel.CommitID, []string) (objectmodel.ObjectValue, error)
}

func (r *datasetRepository) ReadReference(ref objectmodel.Reference, commit kernel.CommitID) (objectmodel.ObjectValue, error) {
	if ref.Repository != r.ID() || commit != r.commit {
		return objectmodel.ObjectValue{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "reference is outside dataset source basis")
	}
	if ref.Locator == "" {
		if err := r.check(ref.Object, commit); err != nil {
			return objectmodel.ObjectValue{}, err
		}
		return readRepositoryReference(r.Repository, ref, commit)
	}
	if !datasetRestrictsRepository(r.serving.pin.Items, r.ID()) && datasetHasRepository(r.serving.pin.Items, r.ID()) {
		return readRepositoryReference(r.Repository, ref, commit)
	}
	port, ok := r.Repository.(referencePathsReader)
	if !ok {
		return objectmodel.ObjectValue{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository cannot plan selected reads within dataset file paths")
	}
	allowed, err := r.referenceAllowedPaths(ref.Object, commit)
	if err != nil {
		return objectmodel.ObjectValue{}, err
	}
	return port.ReadReferenceAtPaths(ref, commit, allowed)
}

func (r *datasetRepository) referenceAllowedPaths(id objectmodel.ObjectID, commit kernel.CommitID) ([]string, error) {
	paths, err := r.ObjectUnitPaths(id, commit)
	if err != nil {
		return nil, err
	}
	allowed := make([]string, 0, len(paths))
	for _, path := range paths {
		if datasetPathAllowed(r.serving.pin.Items, r.ID(), path) {
			allowed = append(allowed, path)
		}
	}
	return allowed, nil
}

// ReadReference retains the explicitly named source inside the accepted pin.
// It never chooses a repository from another member's matching object ID.
func (s *Serving) ReadReference(ref objectmodel.Reference) ([]objectmodel.FederatedValue, error) {
	if _, err := objectmodel.FormatReference(ref); err != nil {
		return nil, err
	}
	repo, err := s.Member(ref.Repository)
	if err != nil {
		return nil, err
	}
	value, err := readHydratedReference(s.hydrator, repo, ref, s.pin.Repositories[ref.Repository])
	if err != nil {
		return nil, err
	}
	return []objectmodel.FederatedValue{federatedOf(ref.Repository, value.Commit, ref.Object, value, value.Value)}, nil
}
