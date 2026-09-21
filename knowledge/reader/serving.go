package reader

import (
	"maps"
	"slices"
	"strings"

	"kc/kernel"
	"kc/knowledge"
)

// MemberLookup mounts a Snapshot by id. Catalog.Require is the usual implementation.
type MemberLookup func(kernel.RepositoryID) (knowledge.Repository, error)

// KnowledgeSetPin is a consumer read basis: one ResolveKnowledgeSet result, frozen for the command.
// Catalog produces the coordinates; this package reads object_id at those commits.

type KnowledgeSetPin struct {
	SetID        string                                  `json:"setId"`
	Revision     int                                     `json:"revision"`
	Repositories map[kernel.RepositoryID]kernel.CommitID `json:"repositories"`
	Items        []DatasetItem                           `json:"items,omitempty"`
}

// DatasetItem is the consumer copy of a Catalog file pointer. Serving uses it
// to skip objects whose unit paths are outside the published list.
type DatasetItem struct {
	Target     string              `json:"target"`
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit"`
	Kind       string              `json:"kind"`
	Prefix     string              `json:"prefix,omitempty"`
	File       string              `json:"file,omitempty"`
}

// Serving is the consumer read face on one KnowledgeSetPin.
// Callers name a Workspace (CLI --dataset). They do not pass repository, ref, or commit.

type Serving struct {
	lookup   MemberLookup
	pin      KnowledgeSetPin
	hydrator knowledge.Hydrator
}

// FederatedValue is one member hit of a consumer read.
// Same field set as knowledge.KnowledgeValue so read --dataset and search --dataset
// share one envelope. objectId is kept for checkout / older callers.

type FederatedValue struct {
	KnowledgeRef knowledge.KnowledgeRef        `json:"knowledgeRef"`
	Repository   kernel.RepositoryID           `json:"repository"`
	Commit       kernel.CommitID               `json:"commit"`
	ObjectID     knowledge.ObjectID            `json:"objectId"`
	Address      knowledge.Address             `json:"address"`
	Value        any                           `json:"value"`
	Provenance   *knowledge.ProvenanceEnvelope `json:"provenance,omitempty"`
	Units        []knowledge.Address           `json:"units,omitempty"`
	Declarations []knowledge.UnitDeclaration   `json:"declarations,omitempty"`
}

type FederatedPage struct {
	Values       []FederatedValue `json:"values"`
	Continuation string           `json:"continuation,omitempty"`
	Exhausted    bool             `json:"exhausted"`
}

func federatedOf(repositoryID kernel.RepositoryID, commit kernel.CommitID, objectID knowledge.ObjectID, src knowledge.KnowledgeValue, assembled any) FederatedValue {
	addr := src.Address
	if addr.ObjectID == "" {
		addr = knowledge.Address{Kind: knowledge.KindEntity, ObjectID: objectID}
	}
	return FederatedValue{
		KnowledgeRef: knowledge.KnowledgeRef{Repository: repositoryID, Object: objectID},
		Repository:   repositoryID,
		Commit:       commit,
		ObjectID:     objectID,
		Address:      addr,
		Value:        assembled,
		Provenance:   src.Provenance,
		Units:        append([]knowledge.Address(nil), src.Units...),
		Declarations: append([]knowledge.UnitDeclaration(nil), src.Declarations...),
	}
}

// ObjectLog is object history on one repository at the serving pin. Not git log.

type ObjectLog struct {
	Repository kernel.RepositoryID        `json:"repository"`
	ObjectID   knowledge.ObjectID         `json:"objectId"`
	Commit     kernel.CommitID            `json:"commit"`
	Revisions  []knowledge.ObjectRevision `json:"revisions"`
}

func Open(lookup MemberLookup, pin KnowledgeSetPin) *Serving {
	pin.Repositories = maps.Clone(pin.Repositories)
	pin.Items = slices.Clone(pin.Items)
	pin.Items = slices.DeleteFunc(pin.Items, func(item DatasetItem) bool {
		return item.Commit == "" || pin.Repositories[item.Repository] != item.Commit || (item.Kind != "prefix" && item.Kind != "file")
	})
	return &Serving{lookup: lookup, pin: pin}
}

// WholeRepositoryItems publishes every path in the named repositories. Product
// Dataset pins come from Catalog freeze; empty Items is not a whole-repo alias.
func WholeRepositoryItems(repos map[kernel.RepositoryID]kernel.CommitID) []DatasetItem {
	ids := make([]kernel.RepositoryID, 0, len(repos))
	for id := range repos {
		ids = append(ids, id)
	}
	sortRepoIDs(ids)
	items := make([]DatasetItem, 0, len(ids))
	for _, id := range ids {
		items = append(items, DatasetItem{
			Target: string(id), Repository: id, Commit: repos[id], Kind: "prefix",
		})
	}
	return items
}

func FederatedRead(lookup MemberLookup, pin KnowledgeSetPin, objectID knowledge.ObjectID) ([]FederatedValue, error) {
	return Open(lookup, pin).Read(objectID, nil)
}

func (s *Serving) Pin() KnowledgeSetPin {
	pin := s.pin
	pin.Repositories = maps.Clone(pin.Repositories)
	pin.Items = slices.Clone(pin.Items)
	return pin
}

func (s *Serving) Contains(repositoryID kernel.RepositoryID, objectID knowledge.ObjectID) (bool, error) {
	commit, ok := s.pin.Repositories[repositoryID]
	if !ok {
		return false, nil
	}
	repo, err := s.lookup(repositoryID)
	if err != nil {
		return false, err
	}
	return s.objectInDataset(repositoryID, commit, repo, objectID)
}

func (s *Serving) Member(id kernel.RepositoryID) (knowledge.Repository, error) {
	commit, ok := s.pin.Repositories[id]
	if !ok {
		return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "repository is outside dataset")
	}
	repo, err := s.lookup(id)
	if err != nil {
		return nil, err
	}
	return &datasetRepository{Repository: repo, serving: s, commit: commit}, nil
}

func (s *Serving) Read(objectID knowledge.ObjectID, selector *knowledge.AspectSelector) ([]FederatedValue, error) {
	out := []FederatedValue{}
	err := s.eachObject(objectID, func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo knowledge.Repository) error {
		value, err := readHydrated(s.hydrator, repo, objectID, commit)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				return nil
			}
			return err
		}
		v := value.Value
		if selector != nil {
			v = knowledge.SelectAspects(value.Value, value.Units, selector)
		}
		out = append(out, federatedOf(repositoryID, commit, objectID, value, v))
		return nil
	})
	return out, err
}

func (s *Serving) ReadAddress(address knowledge.Address) ([]FederatedValue, error) {
	out := []FederatedValue{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo knowledge.Repository) error {
		whole, err := s.objectInDataset(repositoryID, commit, repo, address.ObjectID)
		if err != nil {
			return err
		}
		var value knowledge.KnowledgeValue
		if whole {
			value, err = readAddressHydrated(s.hydrator, repo, address, commit)
		} else {
			// Partial objects must prove the exact unit before any shared cache.
			value, err = repo.ReadAddress(address, commit)
		}
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				return nil
			}
			return err
		}
		out = append(out, federatedOf(repositoryID, commit, address.ObjectID, value, value.Value))
		return nil
	})
	return out, err
}

func (s *Serving) Resolve(objectID knowledge.ObjectID) ([]knowledge.Resolution, error) {
	out := []knowledge.Resolution{}
	err := s.eachObject(objectID, func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo knowledge.Repository) error {
		resolution, err := repo.Resolve(objectID, commit)
		if err != nil {
			return err
		}
		if resolution.Status == knowledge.StatusUnresolved {
			return nil
		}
		out = append(out, resolution)
		return nil
	})
	return out, err
}

// ResolveAddress resolves one exact Aspect/Member unit at every repository in
// the pinned Workspace. It is the Address-level counterpart of Resolve and
// keeps connector/maintenance callers from accidentally comparing an
// assembled Entity digest with a unit digest.
func (s *Serving) ResolveAddress(address knowledge.Address) ([]knowledge.Resolution, error) {
	out := []knowledge.Resolution{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo knowledge.Repository) error {
		resolution, err := repo.ResolveAddress(address, commit)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				return nil
			}
			return err
		}
		if resolution.Status == knowledge.StatusUnresolved {
			return nil
		}
		out = append(out, resolution)
		return nil
	})
	return out, err
}

// ResolveBinding resolves one exact Aspect Binding against every pinned
// Workspace member. Missing addresses are ignored; malformed declarations
// fail closed and are never treated as Snapshot values.
func (s *Serving) ResolveBinding(address knowledge.Address) ([]ResolvedBinding, error) {
	out := []ResolvedBinding{}
	err := s.eachObject(address.ObjectID, func(_ kernel.RepositoryID, commit kernel.CommitID, repo knowledge.Repository) error {
		if _, err := repo.Read(address.ObjectID, commit); err != nil {
			if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				return nil
			}
			return err
		}
		binding, err := ResolveRepoBinding(repo, commit, address)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrCapabilityUnsatisfied || kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				return nil
			}
			return err
		}
		out = append(out, binding)
		return nil
	})
	return out, err
}

// ResolveBindingAt resolves one Binding in one named member of this pinned
// Workspace. It is the declaration primitive used by an upper Knowledge
// Serving layer after a raw READ has identified a bound unit.
func (s *Serving) ResolveBindingAt(repositoryID kernel.RepositoryID, address knowledge.Address) (ResolvedBinding, error) {
	commit, ok := s.pin.Repositories[repositoryID]
	if !ok {
		return ResolvedBinding{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "repository %s is not in workspace %s", repositoryID, s.pin.SetID)
	}
	repo, err := s.Member(repositoryID)
	if err != nil {
		return ResolvedBinding{}, err
	}
	ok, err = s.objectInDataset(repositoryID, commit, repo, address.ObjectID)
	if err != nil {
		return ResolvedBinding{}, err
	}
	if !ok {
		return ResolvedBinding{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "address %s is not in dataset %s", address.ObjectID, s.pin.SetID)
	}
	return ResolveRepoBinding(repo, commit, address)
}

func (s *Serving) GetProvenance(objectID knowledge.ObjectID) ([]knowledge.ProvenanceTrace, error) {
	out := []knowledge.ProvenanceTrace{}
	err := s.eachObject(objectID, func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo knowledge.Repository) error {
		trace, err := repo.GetProvenance(objectID, commit)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				return nil
			}
			return err
		}
		out = append(out, trace)
		return nil
	})
	return out, err
}

func (s *Serving) Log(objectID knowledge.ObjectID, limit int) ([]ObjectLog, error) {
	out := []ObjectLog{}
	err := s.eachObject(objectID, func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo knowledge.Repository) error {
		revs, err := repo.Log(objectID, commit, knowledge.ObjectLogQuery{Limit: limit})
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				return nil
			}
			return err
		}
		if len(revs) == 0 {
			return nil
		}
		out = append(out, ObjectLog{
			Repository: repositoryID,
			ObjectID:   objectID,
			Commit:     commit,
			Revisions:  revs,
		})
		return nil
	})
	return out, err
}

func (s *Serving) DescribeSchema(objectID knowledge.ObjectID) ([]SchemaReport, error) {
	out := []SchemaReport{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo knowledge.Repository) error {
		if objectID != "" {
			ok, err := s.objectInDataset(repositoryID, commit, repo, objectID)
			if err != nil || !ok {
				return err
			}
			resolution, err := repo.Resolve(objectID, commit)
			if err != nil {
				return err
			}
			if resolution.Status == knowledge.StatusUnresolved {
				return nil
			}
		}
		report, err := DescribeRepoSchema(repo, commit, objectID)
		if err != nil {
			return err
		}
		out = append(out, report)
		return nil
	})
	return out, err
}

func (s *Serving) eachRepository(fn func(kernel.RepositoryID, kernel.CommitID, knowledge.Repository) error) error {
	ids := make([]kernel.RepositoryID, 0, len(s.pin.Repositories))
	for id := range s.pin.Repositories {
		ids = append(ids, id)
	}
	sortRepoIDs(ids)
	for _, repositoryID := range ids {
		commit := s.pin.Repositories[repositoryID]
		repo, err := s.Member(repositoryID)
		if err != nil {
			return err
		}
		if err := fn(repositoryID, commit, repo); err != nil {
			return err
		}
	}
	return nil
}

func (s *Serving) eachObject(objectID knowledge.ObjectID, fn func(kernel.RepositoryID, kernel.CommitID, knowledge.Repository) error) error {
	return s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo knowledge.Repository) error {
		ok, err := s.objectInDataset(repositoryID, commit, repo, objectID)
		if err != nil || !ok {
			return err
		}
		return fn(repositoryID, commit, repo)
	})
}

func (s *Serving) objectInDataset(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo knowledge.Repository, objectID knowledge.ObjectID) (bool, error) {
	if len(s.pin.Items) == 0 || !datasetHasRepository(s.pin.Items, repositoryID) {
		return false, nil
	}
	if !datasetRestrictsRepository(s.pin.Items, repositoryID) {
		return true, nil
	}
	locator, ok := repo.(knowledge.UnitLocator)
	if !ok {
		return false, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s cannot filter dataset paths", repositoryID)
	}
	paths, err := locator.ObjectUnitPaths(objectID, commit)
	if err != nil {
		if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
			return false, nil
		}
		return false, err
	}
	return datasetPathsAllowed(s.pin.Items, repositoryID, paths), nil
}

func datasetHasRepository(items []DatasetItem, repository kernel.RepositoryID) bool {
	for _, item := range items {
		if item.Repository == repository {
			return true
		}
	}
	return false
}

func datasetRestrictsRepository(items []DatasetItem, repository kernel.RepositoryID) bool {
	found := false
	for _, item := range items {
		if item.Repository != repository {
			continue
		}
		found = true
		if item.Kind == "prefix" && strings.Trim(item.Prefix, "/") == "" {
			return false
		}
	}
	return found
}

func datasetPathsAllowed(items []DatasetItem, repository kernel.RepositoryID, paths []string) bool {
	for _, path := range paths {
		if !datasetPathAllowed(items, repository, path) {
			return false
		}
	}
	return len(paths) > 0
}

func datasetPathAllowed(items []DatasetItem, repository kernel.RepositoryID, path string) bool {
	for _, item := range items {
		if item.Repository != repository {
			continue
		}
		if datasetItemCovers(item, path) {
			return true
		}
	}
	return false
}

func datasetItemCovers(item DatasetItem, path string) bool {
	path = strings.Trim(path, "/")
	switch item.Kind {
	case "file":
		return path == strings.Trim(item.File, "/")
	case "prefix":
		prefix := strings.Trim(item.Prefix, "/")
		if prefix == "" {
			return true
		}
		return path == prefix || strings.HasPrefix(path, prefix+"/")
	}
	return false
}

func sortRepoIDs(ids []kernel.RepositoryID) {
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			if ids[j] < ids[i] {
				ids[i], ids[j] = ids[j], ids[i]
			}
		}
	}
}
