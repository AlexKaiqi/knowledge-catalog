package kernel

import "strings"

// SchemaObjectPrefix is the object_id prefix of schema knowledge.
const SchemaObjectPrefix = "schema/"

// ParsedSchemaRef is a schema_ref after parsing.
type ParsedSchemaRef struct {
	Object     ObjectID
	Commit     CommitID
	Repository RepositoryID
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
func IsSchemaObject(id ObjectID) bool {
	return strings.HasPrefix(string(id), SchemaObjectPrefix)
}

// ParseSchemaRef parses a schema_ref into a schema object, optional pin, and optional repo.
//
// Accepted forms: schema/foo, schema/foo@commit, kc://org/scope/name/schema/foo,
// kc://org/scope/name@commit/schema/foo. A kc:// form that names a repo fills Repository.
//
// Args:
//
//	ref: frontmatter or PUT schema_ref string.
//
// Returns:
//
//	parsed parts and true when ref names a schema/* object.
func ParseSchemaRef(ref string) (ParsedSchemaRef, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ParsedSchemaRef{}, false
	}
	if strings.HasPrefix(ref, "kc://") {
		rest := strings.TrimPrefix(ref, "kc://")
		if at := strings.IndexByte(rest, '@'); at >= 0 {
			after := rest[at+1:]
			slash := strings.IndexByte(after, '/')
			if slash < 0 {
				return ParsedSchemaRef{}, false
			}
			commit := CommitID(after[:slash])
			object := ObjectID(after[slash+1:])
			if !IsSchemaObject(object) || commit == "" {
				return ParsedSchemaRef{}, false
			}
			return ParsedSchemaRef{
				Object:     object,
				Commit:     commit,
				Repository: RepositoryID("kr://" + rest[:at]),
			}, true
		}
		if i := strings.Index(rest, "/"+SchemaObjectPrefix); i >= 0 {
			object := ObjectID(rest[i+1:])
			if IsSchemaObject(object) {
				return ParsedSchemaRef{
					Object:     object,
					Repository: RepositoryID("kr://" + rest[:i]),
				}, true
			}
		}
		return ParsedSchemaRef{}, false
	}
	object, commit := splitObjectCommit(ref)
	if !IsSchemaObject(object) {
		return ParsedSchemaRef{}, false
	}
	return ParsedSchemaRef{Object: object, Commit: commit}, true
}

func splitObjectCommit(ref string) (ObjectID, CommitID) {
	if i := strings.LastIndexByte(ref, '@'); i > 0 {
		return ObjectID(ref[:i]), CommitID(ref[i+1:])
	}
	return ObjectID(ref), ""
}
