package catalog

import (
	"kc/kernel"
	"kc/reader"
)

// IndexLane is a retrieval face derived from AccessHints.
// summary / stored are payload, not lanes. Vector / graph are Capabilities, not hints.
type IndexLane string

const (
	LaneKey    IndexLane = "key"
	LaneFilter IndexLane = "filter"
	LaneText   IndexLane = "text"
	LaneSort   IndexLane = "sort"
)

var laneOrder = []IndexLane{LaneKey, LaneFilter, LaneText, LaneSort}

type PlannedField struct {
	Schema kernel.ObjectID     `json:"schema"`
	Entity string              `json:"entity,omitempty"`
	Aspect string              `json:"aspect,omitempty"`
	Path   string              `json:"path"`
	Type   string              `json:"type,omitempty"`
	Access []reader.AccessHint `json:"access"`
}

// IndexProjection is one repository's compiled plan: which hints at which pinned commit.
// It is not a built SQLite/OpenSearch index and not Canonical.
type IndexProjection struct {
	Repository   kernel.RepositoryID `json:"repository"`
	Commit       kernel.CommitID     `json:"commit"`
	SchemaDigest kernel.Digest       `json:"schemaDigest"`
	Schemas      []kernel.ObjectID   `json:"schemas"`
	Fields       []PlannedField      `json:"fields"`
	Lanes        []IndexLane         `json:"lanes"`
}

// IndexPlan is a View-scoped *recipe* at the current resolution, not an index.
// Each IndexProjection is one Repository (index sits above that repo).
// The View does not own a federated index blob; SEARCH fans out per repository.
type IndexPlan struct {
	ViewID             string            `json:"viewId"`
	DefinitionRevision int               `json:"definitionRevision"`
	Projections        []IndexProjection `json:"projections"`
}

func (c *Catalog) PlanIndex(viewID string) (plan IndexPlan, err error) {
	defer func() {
		err = c.note("index-plan", map[string]any{"view": viewID}, err)
	}()
	resolved, err := c.ResolveView(viewID)
	if err != nil {
		return IndexPlan{}, err
	}
	return c.PlanIndexResolved(resolved)
}

func (c *Catalog) PlanIndexResolved(resolved ResolvedView) (plan IndexPlan, err error) {
	plan = IndexPlan{
		ViewID:             resolved.ViewID,
		DefinitionRevision: resolved.Revision,
		Projections:        []IndexProjection{},
	}
	ids := make([]kernel.RepositoryID, 0, len(resolved.Repositories))
	for id := range resolved.Repositories {
		ids = append(ids, id)
	}
	sortRepoIDs(ids)
	for _, repositoryID := range ids {
		commit := resolved.Repositories[repositoryID]
		repo, err := c.store.Require(repositoryID, kernel.ErrTemporaryUnavailable)
		if err != nil {
			return IndexPlan{}, err
		}
		if !repo.HasCommit(commit) {
			return IndexPlan{}, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s is unresolved in %s", commit, repositoryID)
		}
		report, err := reader.DescribeRepoSchema(repo, commit, "")
		if err != nil {
			return IndexPlan{}, err
		}
		plan.Projections = append(plan.Projections, compileProjection(repositoryID, commit, report))
	}
	return plan, nil
}

func compileProjection(repositoryID kernel.RepositoryID, commit kernel.CommitID, report reader.SchemaReport) IndexProjection {
	spec := reader.SpecFromReport(report)
	proj := IndexProjection{
		Repository:   repositoryID,
		Commit:       commit,
		SchemaDigest: spec.Digest,
		Schemas:      spec.Schemas,
		Fields:       make([]PlannedField, 0, len(spec.Fields)),
		Lanes:        []IndexLane{},
	}
	laneSeen := map[IndexLane]struct{}{}
	for _, field := range spec.Fields {
		proj.Fields = append(proj.Fields, PlannedField{
			Schema: field.Schema,
			Entity: field.Entity,
			Aspect: field.Aspect,
			Path:   field.Path,
			Type:   field.Type,
			Access: field.Access,
		})
		for _, hint := range field.Access {
			if lane, ok := laneOf(hint); ok {
				laneSeen[lane] = struct{}{}
			}
		}
	}
	for _, lane := range laneOrder {
		if _, ok := laneSeen[lane]; ok {
			proj.Lanes = append(proj.Lanes, lane)
		}
	}
	return proj
}

func laneOf(hint reader.AccessHint) (IndexLane, bool) {
	switch hint {
	case reader.HintKey:
		return LaneKey, true
	case reader.HintFilter:
		return LaneFilter, true
	case reader.HintText:
		return LaneText, true
	case reader.HintSort:
		return LaneSort, true
	default:
		return "", false
	}
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
