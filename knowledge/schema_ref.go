package knowledge

import (
	"strings"

	"kc/kernel"
)

// SchemaObjectPrefix is the object_id prefix of schema knowledge.
const SchemaObjectPrefix = "schema/"

type ParsedSchemaRef struct {
	Object     ObjectID
	Commit     kernel.CommitID
	Repository kernel.RepositoryID
}

func IsSchemaObject(id ObjectID) bool {
	return strings.HasPrefix(string(id), SchemaObjectPrefix)
}

// ParseSchemaRef accepts schema/foo, schema/foo@commit and pinned or unpinned
// kc:// references. The reference must name a schema/* knowledge object.
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
			commit := kernel.CommitID(after[:slash])
			object := ObjectID(after[slash+1:])
			if !IsSchemaObject(object) || commit == "" {
				return ParsedSchemaRef{}, false
			}
			return ParsedSchemaRef{Object: object, Commit: commit, Repository: kernel.RepositoryID("kr://" + rest[:at])}, true
		}
		if i := strings.Index(rest, "/"+SchemaObjectPrefix); i >= 0 {
			object := ObjectID(rest[i+1:])
			if IsSchemaObject(object) {
				return ParsedSchemaRef{Object: object, Repository: kernel.RepositoryID("kr://" + rest[:i])}, true
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

func splitObjectCommit(ref string) (ObjectID, kernel.CommitID) {
	if i := strings.LastIndexByte(ref, '@'); i > 0 {
		return ObjectID(ref[:i]), kernel.CommitID(ref[i+1:])
	}
	return ObjectID(ref), ""
}
