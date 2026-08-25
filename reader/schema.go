package reader

import (
	"kc/kernel"
	"kc/knowledge"
)

// AccessHint is a logical access declaration on a schema field, not a physical
// index declaration or query operator.
type AccessHint string

const (
	HintFilter AccessHint = "filter"
	HintText   AccessHint = "text"
	HintSort   AccessHint = "sort"
)

var hintOrder = []AccessHint{HintFilter, HintText, HintSort}

var knownHints = map[AccessHint]struct{}{
	HintFilter: {}, HintText: {}, HintSort: {},
}

type FieldAccess struct {
	Path   string       `json:"path"`
	Type   string       `json:"type,omitempty"`
	Access []AccessHint `json:"access,omitempty"`
}

type SchemaDescription struct {
	ObjectID   knowledge.ObjectID  `json:"objectId"`
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit"`
	Entity     string              `json:"entity,omitempty"`
	Aspect     string              `json:"aspect,omitempty"`
	Pattern    string              `json:"pattern,omitempty"`
	Fields     []FieldAccess       `json:"fields"`
	Digest     kernel.Digest       `json:"digest"`
}

// SchemaReport is DESCRIBE_SCHEMA: Entity/Aspect Schema, Pattern, AccessHints.
// It is introspection of schema/* (and schema_ref), not a GraphQL runtime.
type SchemaReport struct {
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit"`
	Schemas    []SchemaDescription `json:"schemas"`
}

func (r *Reader) DescribeSchema(repositoryID kernel.RepositoryID, commitID kernel.CommitID, objectID knowledge.ObjectID) (report SchemaReport, err error) {
	defer func() {
		refs := map[string]any{"repositoryId": string(repositoryID), "commit": string(commitID)}
		if objectID != "" {
			refs["object"] = string(objectID)
		}
		err = r.note("describe-schema", refs, err)
	}()
	repo, err := r.repoByID(repositoryID)
	if err != nil {
		return SchemaReport{}, err
	}
	return DescribeRepoSchema(repo, commitID, objectID)
}

// DescribeRepoSchema interprets schema/* (and optional schema_ref) at a pinned commit.
func DescribeRepoSchema(repo knowledge.Repository, commitID kernel.CommitID, objectID knowledge.ObjectID) (SchemaReport, error) {
	report := SchemaReport{Repository: repo.ID(), Commit: commitID, Schemas: []SchemaDescription{}}
	if objectID == "" {
		listed, err := repo.List(commitID)
		if err != nil {
			return SchemaReport{}, err
		}
		for _, value := range listed {
			if !knowledge.IsSchemaObject(value.Address.ObjectID) {
				continue
			}
			desc, err := describeValue(repo.ID(), commitID, value.Address.ObjectID, value.Value)
			if err != nil {
				return SchemaReport{}, err
			}
			report.Schemas = append(report.Schemas, desc)
		}
		sortSchemaDescriptions(report.Schemas)
		return report, nil
	}
	if knowledge.IsSchemaObject(objectID) {
		value, err := repo.Read(objectID, commitID)
		if err != nil {
			return SchemaReport{}, err
		}
		desc, err := describeValue(repo.ID(), commitID, objectID, value.Value)
		if err != nil {
			return SchemaReport{}, err
		}
		report.Schemas = []SchemaDescription{desc}
		return report, nil
	}
	refs, err := schemaRefsOf(repo, objectID, commitID)
	if err != nil {
		return SchemaReport{}, err
	}
	seen := map[knowledge.ObjectID]struct{}{}
	for _, ref := range refs {
		id, pin, ok := parseSchemaRef(ref)
		if !ok {
			return SchemaReport{}, kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "schema_ref %q is not a pinned schema object", ref)
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		at := commitID
		if pin != "" {
			if !repo.HasCommit(pin) {
				return SchemaReport{}, kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "schema_ref %q commit does not exist", ref)
			}
			at = pin
		}
		value, err := repo.Read(id, at)
		if err != nil {
			return SchemaReport{}, kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "schema_ref %q does not resolve to a schema object", ref)
		}
		desc, err := describeValue(repo.ID(), at, id, value.Value)
		if err != nil {
			return SchemaReport{}, err
		}
		report.Schemas = append(report.Schemas, desc)
	}
	sortSchemaDescriptions(report.Schemas)
	return report, nil
}

func schemaRefsOf(repo knowledge.Repository, objectID knowledge.ObjectID, commitID kernel.CommitID) ([]string, error) {
	value, err := repo.Read(objectID, commitID)
	if err != nil {
		return nil, err
	}
	var refs []string
	if len(value.Units) == 0 {
		res, err := repo.Resolve(objectID, commitID)
		if err != nil {
			return nil, err
		}
		if res.SchemaRef != "" {
			refs = append(refs, res.SchemaRef)
		}
		return refs, nil
	}
	for _, addr := range value.Units {
		res, err := repo.ResolveAddress(addr, commitID)
		if err != nil {
			return nil, err
		}
		if res.SchemaRef != "" {
			refs = append(refs, res.SchemaRef)
		}
	}
	return refs, nil
}

func describeValue(repositoryID kernel.RepositoryID, commitID kernel.CommitID, objectID knowledge.ObjectID, value any) (SchemaDescription, error) {
	if invalid := invalidAccessTokens(value); len(invalid) > 0 {
		return SchemaDescription{}, kernel.Fail(kernel.ErrUsageInvalid, "schema %s declares unsupported access %v; only text, filter and sort are allowed", objectID, invalid)
	}
	doc := parseSchemaDocument(value)
	fields := append([]FieldAccess{}, doc.Fields...)
	sortFieldAccess(fields)
	desc := SchemaDescription{
		ObjectID:   objectID,
		Repository: repositoryID,
		Commit:     commitID,
		Entity:     doc.Entity,
		Aspect:     doc.Aspect,
		Pattern:    doc.Pattern,
		Fields:     fields,
	}
	desc.Digest = kernel.CanonicalDigest(map[string]any{
		"entity":  desc.Entity,
		"aspect":  desc.Aspect,
		"pattern": desc.Pattern,
		"fields":  fields,
	})
	return desc, nil
}

func parseSchemaRef(ref string) (knowledge.ObjectID, kernel.CommitID, bool) {
	parsed, ok := knowledge.ParseSchemaRef(ref)
	return parsed.Object, parsed.Commit, ok
}

func sortSchemaDescriptions(items []SchemaDescription) {
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].ObjectID < items[i].ObjectID {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
}
