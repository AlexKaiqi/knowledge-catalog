package invocation

import (
	"kc/internal/journal"
	"kc/kernel"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	objectservice "kc/object/service"
)

// The transparent value path keeps the Reader task's existing process event
// without repeating declaration or body reads through the Reader facade.
func readRepositoryObserved(cx *Invocation, request objectservice.ReadRequest) (value objectmodel.ObservedValue, err error) {
	refs := map[string]any{"repositoryId": string(request.Reference.Repository), "object": string(request.Reference.Object), "commit": string(request.Commit), "locator": request.Reference.Locator}
	defer func() {
		err = journal.Finish(cx.WS.Journal, journal.LayerSystem, "reader", "read-reference", refs, err)
	}()
	repo, err := cx.WS.Reader.Require(request.Reference.Repository, kernel.ErrKnowledgeRefUnresolved)
	if err != nil {
		return objectmodel.ObservedValue{}, err
	}
	lookup, err := ResourceLookup(cx)
	if err != nil {
		return objectmodel.ObservedValue{}, err
	}
	readContext, err := stateRequestContextFrom(cx)
	if err != nil {
		return objectmodel.ObservedValue{}, err
	}
	return liveservice.ReadRepository(cx.Context, repo, liveservice.ReadRequest{
		Commit: request.Commit, Reference: &request.Reference, Hydrator: cx.WS.Hydrator,
	}, lookup, readContext)
}
