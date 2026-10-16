package plugin

import (
	"context"

	retrieval "kc/discovery"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

func (p *IndexPlugins) RequiresState(repo objectmodel.Repository, at kernel.CommitID, request retrieval.SearchRequest) (bool, error) {
	instance, done, err := p.takeRepository(repo)
	if err != nil {
		return false, err
	}
	defer done()
	return instance.services.Queries.RequiresState(repo, at, request)
}

func (p *IndexPlugins) StateView(repo kernel.RepositoryID, at kernel.CommitID) (string, bool) {
	instance, done, err := p.take(repo)
	if err != nil {
		return "", false
	}
	defer done()
	return instance.services.Queries.StateView(repo, at)
}

func (p *IndexPlugins) AccessSpecAt(repo objectmodel.Repository, at kernel.CommitID) (retrieval.AccessSpec, error) {
	instance, done, err := p.takeRepository(repo)
	if err != nil {
		return retrieval.AccessSpec{}, err
	}
	defer done()
	if scoped, ok := repo.(interface {
		DatasetScope() (reader.MemberLookup, reader.DatasetPin)
	}); ok {
		if queries, ok := instance.services.Queries.(interface {
			DatasetAccessSpec(reader.MemberLookup, reader.DatasetPin) (retrieval.AccessSpec, error)
		}); ok {
			lookup, pin := scoped.DatasetScope()
			return queries.DatasetAccessSpec(lookup, pin)
		}
	}
	return instance.services.Queries.AccessSpecAt(repo, at)
}

// DeclaredAccessSpecAt compiles the declarations this commit selects with the
// serving instance's manifest check. It touches no engine, projection or
// readiness probe, so explanations stay free of physical work; whether the
// member can serve search now is a separate readiness question.
func (p *IndexPlugins) DeclaredAccessSpecAt(repo objectmodel.Repository, at kernel.CommitID) (retrieval.AccessSpec, error) {
	instance, done, err := p.takeRepository(repo)
	if err != nil {
		return retrieval.AccessSpec{}, err
	}
	defer done()
	var check retrieval.DeclarationCheck
	if !nilContribution(instance.services.Capabilities) {
		check = instance.services.Capabilities.Check
	}
	return retrieval.AccessSpecForRepositoryChecked(repo, at, check)
}

func (p *IndexPlugins) SearchAtContext(ctx context.Context, repo objectmodel.Repository, at kernel.CommitID, request retrieval.SearchRequest) (retrieval.SearchResult, error) {
	return p.search(ctx, repo, at, "", false, request)
}

func (p *IndexPlugins) SearchStateAtRevisionContext(ctx context.Context, repo objectmodel.Repository, at kernel.CommitID, revision string, request retrieval.SearchRequest) (retrieval.SearchResult, error) {
	return p.search(ctx, repo, at, revision, true, request)
}

func (p *IndexPlugins) search(ctx context.Context, repo objectmodel.Repository, at kernel.CommitID, revision string, state bool, request retrieval.SearchRequest) (retrieval.SearchResult, error) {
	instance, done, err := p.takeRepository(repo)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	defer done()
	request.Continuation, err = instance.unwrap(request.Continuation)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	var result retrieval.SearchResult
	if state {
		result, err = instance.services.Queries.SearchStateAtRevisionContext(ctx, repo, at, revision, request)
	} else {
		result, err = instance.services.Queries.SearchAtContext(ctx, repo, at, request)
	}
	if err == nil {
		result.Continuation = instance.wrap(result.Continuation)
	}
	return result, err
}

func (p *IndexPlugins) SemanticWindowAtContext(ctx context.Context, repo objectmodel.Repository, at kernel.CommitID, request retrieval.SearchRequest, vector []float32) (retrieval.SearchResult, error) {
	instance, done, err := p.takeRepository(repo)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	defer done()
	window, ok := instance.services.Queries.(SemanticWindowProjection)
	if !ok {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "configured plugin has no semantic window")
	}
	request.Continuation, err = instance.unwrap(request.Continuation)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	result, err := window.SemanticWindowAtContext(ctx, repo, at, request, vector)
	if err == nil {
		result.Continuation = instance.wrap(result.Continuation)
	}
	return result, err
}

func (p *IndexPlugins) RelationsAtContext(ctx context.Context, repo objectmodel.Repository, at kernel.CommitID, request retrieval.RelationPageRequest) (retrieval.RelationPage, error) {
	instance, done, err := p.takeRepository(repo)
	if err != nil {
		return retrieval.RelationPage{}, err
	}
	defer done()
	request.Continuation, err = instance.unwrap(request.Continuation)
	if err != nil {
		return retrieval.RelationPage{}, err
	}
	result, err := instance.services.Queries.RelationsAtContext(ctx, repo, at, request)
	if err == nil {
		result.Continuation = instance.wrap(result.Continuation)
	}
	return result, err
}

func (i *indexInstance) wrap(position string) string {
	if position == "" {
		return ""
	}
	return retrieval.EncodeContinuation(retrieval.ContinuationState{Scope: "index-plugin", Projection: kernel.CanonicalDigest(i.selection), Position: position})
}

func (i *indexInstance) unwrap(position string) (string, error) {
	if position == "" {
		return "", nil
	}
	state, err := retrieval.DecodeContinuation(position)
	if err == nil && state.Scope == "index-plugin" {
		if state.Projection != kernel.CanonicalDigest(i.selection) {
			return "", kernel.Fail(kernel.ErrPreconditionFailed, "continuation belongs to another plugin instance")
		}
		return state.Position, nil
	}
	// Only the stable instance that adopts existing resources accepts their
	// existing tokens. The selected provider still performs its full checks.
	if i.selection.Name == "declarative-index" && i.selection.Instance == "main" {
		return position, nil
	}
	return "", kernel.Fail(kernel.ErrPreconditionFailed, "continuation has no matching plugin instance")
}

// SelectionDigest binds outer Dataset/traversal pages even if an inner member
// is already exhausted and its provider will not be called on the next page.
func (p *IndexPlugins) SelectionDigest(repos map[kernel.RepositoryID]kernel.CommitID) kernel.Digest {
	selected := map[kernel.RepositoryID]PluginSelection{}
	legacy := true
	for repo := range repos {
		if instance, err := p.selected(repo); err == nil {
			selected[repo] = instance.selection
			if instance.selection.Name != "declarative-index" || instance.selection.Instance != "main" {
				legacy = false
			}
		} else {
			legacy = false
		}
	}
	if legacy {
		return ""
	}
	return kernel.CanonicalDigest(selected)
}
