package reader

import (
	"sort"

	"kc/kernel"
	objectmodel "kc/object"
)

// holdingConflictPageLimit bounds one identity page during the publication scan.
const holdingConflictPageLimit = 1000

// CheckHoldingConflicts proves that no object inside the published range takes
// the same Aspect from two member Repositories. Dataset publication runs it
// before acceptance; it enumerates identities only, then plans the objects
// that more than one member stores. A single-member pin cannot conflict.
func (s *Serving) CheckHoldingConflicts() error {
	ids := make([]kernel.RepositoryID, 0, len(s.pin.Repositories))
	for id := range s.pin.Repositories {
		ids = append(ids, id)
	}
	if len(ids) < 2 {
		return nil
	}
	sortRepoIDs(ids)
	repos := make(map[kernel.RepositoryID]objectmodel.Repository, len(ids))
	holders := map[objectmodel.ObjectID][]kernel.RepositoryID{}
	for _, id := range ids {
		repo, err := s.lookup(id)
		if err != nil {
			return err
		}
		repos[id] = repo
		if err := eachStoredObjectID(repo, s.pin.Repositories[id], func(object objectmodel.ObjectID) {
			holders[object] = append(holders[object], id)
		}); err != nil {
			return err
		}
	}
	shared := make([]objectmodel.ObjectID, 0)
	for object, members := range holders {
		if len(members) > 1 {
			shared = append(shared, object)
		}
	}
	sort.Slice(shared, func(i, j int) bool { return shared[i] < shared[j] })
	for _, object := range shared {
		holdings := make([]objectmodel.Holding, 0, len(holders[object]))
		for _, id := range holders[object] {
			holding, ok, err := s.publishedHolding(id, repos[id], object)
			if err != nil {
				return err
			}
			if ok {
				holdings = append(holdings, holding)
			}
		}
		if len(holdings) < 2 {
			continue
		}
		if _, err := objectmodel.SelectHoldings(object, "", holdings); kernel.CodeOf(err) == kernel.ErrObjectIDConflict {
			return err
		}
	}
	return nil
}

func (s *Serving) publishedHolding(id kernel.RepositoryID, repo objectmodel.Repository, object objectmodel.ObjectID) (objectmodel.Holding, bool, error) {
	commit := s.pin.Repositories[id]
	inRange, err := s.objectInDataset(id, commit, repo, object)
	if err != nil || !inRange {
		return objectmodel.Holding{}, false, err
	}
	value, err := repo.Read(object, commit)
	if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
		return objectmodel.Holding{}, false, nil
	}
	if err != nil {
		return objectmodel.Holding{}, false, err
	}
	return objectmodel.HoldingOf(id, commit, true, value.Units), true, nil
}

func eachStoredObjectID(repo objectmodel.Repository, commit kernel.CommitID, visit func(objectmodel.ObjectID)) error {
	pager, ok := repo.(objectmodel.SnapshotObjectPager)
	if !ok {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s cannot enumerate object identities for the publication conflict check", repo.ID())
	}
	continuation := ""
	for {
		page, err := pager.ObjectIDsPage(commit, holdingConflictPageLimit, continuation)
		if err != nil {
			return err
		}
		for _, object := range page.ObjectIDs {
			visit(object)
		}
		if page.Exhausted {
			return nil
		}
		if page.Continuation == "" || page.Continuation == continuation {
			return kernel.Fail(kernel.ErrTemporaryUnavailable, "repository %s returned a non-advancing identity page", repo.ID())
		}
		continuation = page.Continuation
	}
}
