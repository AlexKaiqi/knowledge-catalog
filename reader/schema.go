package reader

import (
	"strings"

	"kc/kernel"
	"kc/repository"
)

// AccessHint is the index declaration on a schema field (retrieval face, not a query op).
// text / filter / key / sort / summary / stored. Query ops are implied (see AllowsOp).
// Schema must not bind physical engines (HNSW / GIN / analyzer / embedding).
type AccessHint string

const (
	HintKey     AccessHint = "key"
	HintFilter  AccessHint = "filter"
	HintText    AccessHint = "text"
	HintSort    AccessHint = "sort"
	HintSummary AccessHint = "summary"
	HintStored  AccessHint = "stored"
)

var hintOrder = []AccessHint{HintKey, HintFilter, HintText, HintSort, HintSummary, HintStored}

var knownHints = map[AccessHint]struct{}{
	HintKey: {}, HintFilter: {}, HintText: {}, HintSort: {}, HintSummary: {}, HintStored: {},
}

// Physical tokens that may appear in schema JSON; DESCRIBE_SCHEMA drops them.
var physicalBindings = map[string]struct{}{
	"hnsw": {}, "gin": {}, "gist": {}, "analyzer": {}, "embedding": {},
	"embedding_model": {}, "ivfflat": {}, "btree": {}, "fts5": {}, "opensearch": {},
}

type FieldAccess struct {
	Path   string       `json:"path"`
	Type   string       `json:"type,omitempty"`
	Access []AccessHint `json:"access,omitempty"`
}

type SchemaDescription struct {
	ObjectID   kernel.ObjectID     `json:"objectId"`
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

// IsSchemaObject reports whether object_id names a schema/* object.
//
// Args:
//
//	id: candidate object_id.
//
// Returns:
//
//	true when id starts with schema/.
func IsSchemaObject(id kernel.ObjectID) bool {
	return kernel.IsSchemaObject(id)
}

func (r *Reader) DescribeSchema(repositoryID kernel.RepositoryID, commitID kernel.CommitID, objectID kernel.ObjectID) (report SchemaReport, err error) {
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
func DescribeRepoSchema(repo repository.Repository, commitID kernel.CommitID, objectID kernel.ObjectID) (SchemaReport, error) {
	report := SchemaReport{Repository: repo.ID(), Commit: commitID, Schemas: []SchemaDescription{}}
	if objectID == "" {
		listed, err := repo.List(commitID)
		if err != nil {
			return SchemaReport{}, err
		}
		for _, value := range listed {
			if !IsSchemaObject(value.Address.ObjectID) {
				continue
			}
			report.Schemas = append(report.Schemas, describeValue(repo.ID(), commitID, value.Address.ObjectID, value.Value))
		}
		sortSchemaDescriptions(report.Schemas)
		return report, nil
	}
	if IsSchemaObject(objectID) {
		value, err := repo.Read(objectID, commitID)
		if err != nil {
			return SchemaReport{}, err
		}
		report.Schemas = []SchemaDescription{describeValue(repo.ID(), commitID, objectID, value.Value)}
		return report, nil
	}
	refs, err := schemaRefsOf(repo, objectID, commitID)
	if err != nil {
		return SchemaReport{}, err
	}
	seen := map[kernel.ObjectID]struct{}{}
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
		report.Schemas = append(report.Schemas, describeValue(repo.ID(), at, id, value.Value))
	}
	sortSchemaDescriptions(report.Schemas)
	return report, nil
}

func schemaRefsOf(repo repository.Repository, objectID kernel.ObjectID, commitID kernel.CommitID) ([]string, error) {
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

func describeValue(repositoryID kernel.RepositoryID, commitID kernel.CommitID, objectID kernel.ObjectID, value any) SchemaDescription {
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
	return desc
}

type schemaDocument struct {
	Entity  string
	Aspect  string
	Pattern string
	Fields  []FieldAccess
}

func parseSchemaDocument(value any) schemaDocument {
	obj, ok := asObject(value)
	if !ok {
		return schemaDocument{}
	}
	if looksLikeSchema(obj) {
		return parseSchemaObject(obj)
	}
	for _, nested := range obj {
		child, ok := asObject(nested)
		if ok && looksLikeSchema(child) {
			return parseSchemaObject(child)
		}
	}
	return schemaDocument{}
}

func looksLikeSchema(obj map[string]any) bool {
	if _, ok := obj["fields"]; ok {
		return true
	}
	if _, ok := obj["pattern"]; ok {
		return true
	}
	_, hasEntity := obj["entity"]
	_, hasAspect := obj["aspect"]
	return hasEntity && hasAspect
}

func parseSchemaObject(obj map[string]any) schemaDocument {
	doc := schemaDocument{
		Entity:  stringField(obj, "entity"),
		Aspect:  stringField(obj, "aspect"),
		Pattern: normalizePattern(stringField(obj, "pattern")),
	}
	switch fields := obj["fields"].(type) {
	case map[string]any:
		for name, raw := range fields {
			doc.Fields = append(doc.Fields, parseField(name, raw))
		}
	case []any:
		for _, raw := range fields {
			item, ok := asObject(raw)
			if !ok {
				continue
			}
			name := stringField(item, "path")
			if name == "" {
				name = stringField(item, "name")
			}
			doc.Fields = append(doc.Fields, parseField(name, raw))
		}
	}
	return doc
}

func parseField(path string, raw any) FieldAccess {
	field := FieldAccess{Path: path, Access: []AccessHint{}}
	obj, ok := asObject(raw)
	if !ok {
		return field
	}
	if p := stringField(obj, "path"); p != "" {
		field.Path = p
	} else if n := stringField(obj, "name"); n != "" && field.Path == "" {
		field.Path = n
	}
	field.Type = stringField(obj, "type")
	field.Access = normalizeHints(obj["access"])
	return field
}

func normalizeHints(raw any) []AccessHint {
	var tokens []string
	switch v := raw.(type) {
	case []any:
		for _, item := range v {
			tokens = append(tokens, strings.ToLower(strings.TrimSpace(asString(item))))
		}
	case []string:
		for _, item := range v {
			tokens = append(tokens, strings.ToLower(strings.TrimSpace(item)))
		}
	case string:
		for _, item := range strings.Fields(v) {
			tokens = append(tokens, strings.ToLower(strings.TrimSpace(item)))
		}
	}
	seen := map[AccessHint]struct{}{}
	for _, token := range tokens {
		if token == "" {
			continue
		}
		if _, phys := physicalBindings[token]; phys {
			continue
		}
		hint := AccessHint(token)
		if _, ok := knownHints[hint]; !ok {
			continue
		}
		seen[hint] = struct{}{}
	}
	out := []AccessHint{}
	for _, hint := range hintOrder {
		if _, ok := seen[hint]; ok {
			out = append(out, hint)
		}
	}
	return out
}

func normalizePattern(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, " ", "_")
	switch s {
	case "record", "keyed_collection", "ordered_artifact", "relation_set":
		return s
	default:
		return s
	}
}

func parseSchemaRef(ref string) (kernel.ObjectID, kernel.CommitID, bool) {
	parsed, ok := kernel.ParseSchemaRef(ref)
	return parsed.Object, parsed.Commit, ok
}

func asObject(value any) (map[string]any, bool) {
	obj, ok := value.(map[string]any)
	return obj, ok
}

func stringField(obj map[string]any, key string) string {
	return strings.TrimSpace(asString(obj[key]))
}

func asString(value any) string {
	s, _ := value.(string)
	return s
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

func sortFieldAccess(fields []FieldAccess) {
	for i := 0; i < len(fields); i++ {
		for j := i + 1; j < len(fields); j++ {
			if fields[j].Path < fields[i].Path {
				fields[i], fields[j] = fields[j], fields[i]
			}
		}
	}
}
