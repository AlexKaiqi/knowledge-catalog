package reader

import (
	"kc/kernel"
	objectmodel "kc/object"
)

// AccessHint is a logical access declaration on a schema field, not a physical
// index declaration or query operator.
type AccessHint string

const (
	HintFilter AccessHint = "filter"
	HintText   AccessHint = "text"
	HintSort   AccessHint = "sort"
)

type FieldAccess struct {
	Path   string       `json:"path"`
	Type   string       `json:"type,omitempty"`
	Access []AccessHint `json:"access,omitempty"`
}

type SchemaDescription struct {
	ObjectID             objectmodel.ObjectID         `json:"objectId"`
	MetaSchema           objectmodel.ObjectID         `json:"metaSchema"`
	Entity               string                       `json:"entity,omitempty"`
	Aspect               string                       `json:"aspect,omitempty"`
	Pattern              string                       `json:"pattern,omitempty"`
	AdditionalProperties bool                         `json:"additionalProperties"`
	Origin               string                       `json:"origin,omitempty"`
	Fields               []FieldAccess                `json:"fields"`
	Definition           objectmodel.SchemaDefinition `json:"definition"`
	// Digest covers the complete normalized Definition, including constraints.
	Digest kernel.Digest `json:"digest"`
}

// SchemaReport is DESCRIBE_SCHEMA: Entity/Aspect Schema, Pattern, AccessHints.
// It is introspection of schema/* (and schema_ref), not a GraphQL runtime.
type SchemaReport struct {
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit"`
	Schemas    []SchemaDescription `json:"schemas"`
}

// InspectSchemaValue validates and normalizes an uncommitted schema/* draft.
// It is pure: callers such as ingest preview can report AccessHints before a
// Writer COMMIT without pretending the draft already exists at a commit.
func InspectSchemaValue(objectID objectmodel.ObjectID, value any) (SchemaDescription, error) {
	if !objectmodel.IsSchemaObject(objectID) {
		return SchemaDescription{}, kernel.Fail(kernel.ErrUsageInvalid, "schema draft %s must use a schema/* object_id", objectID)
	}
	return describeValue("", "", objectID, value)
}

func (r *Reader) DescribeSchema(repositoryID kernel.RepositoryID, commitID kernel.CommitID, objectID objectmodel.ObjectID) (report SchemaReport, err error) {
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
func DescribeRepoSchema(repo objectmodel.Repository, commitID kernel.CommitID, objectID objectmodel.ObjectID) (SchemaReport, error) {
	report := SchemaReport{Repository: repo.ID(), Commit: commitID, Schemas: []SchemaDescription{}}
	if objectID == "" {
		if native, ok := repo.(objectmodel.SchemaStore); ok {
			ids, err := native.SchemaObjectIDs(commitID)
			if err != nil {
				return SchemaReport{}, err
			}
			values := map[objectmodel.ObjectID]objectmodel.ObjectValue{}
			if batch, ok := repo.(objectmodel.BatchReadStore); ok {
				values, err = batch.ReadMany(ids, commitID)
				if err != nil {
					return SchemaReport{}, err
				}
			}
			for _, id := range ids {
				value, ok := values[id]
				if !ok {
					value, err = repo.Read(id, commitID)
					if err != nil {
						return SchemaReport{}, err
					}
				}
				desc, err := describeValue(repo.ID(), commitID, id, value.Value)
				if err != nil {
					return SchemaReport{}, err
				}
				report.Schemas = append(report.Schemas, desc)
			}
			sortSchemaDescriptions(report.Schemas)
			return report, nil
		}
		return SchemaReport{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"repository %s does not provide schema namespace location", repo.ID())
	}
	if objectmodel.IsSchemaObject(objectID) {
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
	seen := map[objectmodel.ObjectID]struct{}{}
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

func schemaRefsOf(repo objectmodel.Repository, objectID objectmodel.ObjectID, commitID kernel.CommitID) ([]string, error) {
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

func describeValue(_ kernel.RepositoryID, _ kernel.CommitID, objectID objectmodel.ObjectID, value any) (SchemaDescription, error) {
	definition, err := objectmodel.ParseSchemaDefinition(objectID, value)
	if err != nil {
		return SchemaDescription{}, err
	}
	fields := make([]FieldAccess, 0, len(definition.Fields))
	for _, field := range definition.Fields {
		access := make([]AccessHint, 0, len(field.Access))
		for _, hint := range field.Access {
			access = append(access, AccessHint(hint))
		}
		fields = append(fields, FieldAccess{Path: field.Path, Type: field.Type, Access: access})
	}
	desc := SchemaDescription{
		ObjectID:             objectID,
		MetaSchema:           definition.MetaSchema,
		Entity:               definition.Entity,
		Aspect:               definition.Aspect,
		Pattern:              definition.Pattern,
		AdditionalProperties: definition.AdditionalProperties,
		Origin:               definition.Origin,
		Fields:               fields,
		Definition:           definition,
	}
	desc.Digest = kernel.CanonicalDigest(definition)
	return desc, nil
}

func parseSchemaRef(ref string) (objectmodel.ObjectID, kernel.CommitID, bool) {
	parsed, ok := objectmodel.ParseSchemaRef(ref)
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
