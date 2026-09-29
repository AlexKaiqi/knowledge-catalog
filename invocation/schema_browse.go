package invocation

import (
	"kc/kernel"
	"kc/knowledge"
)

type SchemaPageCursor struct {
	Version    int                 `json:"version"`
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit"`
	After      knowledge.ObjectID  `json:"after"`
	Check      kernel.Digest       `json:"check"`
}

// SchemaEntity is one published type name. It is not the Schema document and
// not DESCRIBE_SCHEMA.
type SchemaEntity struct {
	Entity      string             `json:"entity"`
	Description string             `json:"description,omitempty"`
	ObjectID    knowledge.ObjectID `json:"objectId"`
}

type SchemaPageResponse struct {
	Repository   kernel.RepositoryID `json:"repository"`
	Commit       kernel.CommitID     `json:"commit"`
	Schemas      []SchemaEntity      `json:"schemas"`
	Continuation string              `json:"continuation,omitempty"`
}

func SchemaCursorCheck(cursor SchemaPageCursor) kernel.Digest {
	return kernel.CanonicalDigest(struct {
		Version    int
		Repository kernel.RepositoryID
		Commit     kernel.CommitID
		After      knowledge.ObjectID
	}{cursor.Version, cursor.Repository, cursor.Commit, cursor.After})
}

func SchemaEntityDirectory(repo knowledge.Repository, commit kernel.CommitID, ids []knowledge.ObjectID) ([]SchemaEntity, error) {
	listed := make([]SchemaEntity, 0, len(ids))
	for _, objectID := range ids {
		item, err := SchemaEntityName(repo, commit, objectID)
		if err != nil {
			return nil, err
		}
		listed = append(listed, item)
	}
	return listed, nil
}

func SchemaEntityName(repo knowledge.Repository, commit kernel.CommitID, objectID knowledge.ObjectID) (SchemaEntity, error) {
	value, err := repo.Read(objectID, commit)
	if err != nil {
		return SchemaEntity{}, err
	}
	def, err := knowledge.ParseSchemaDefinition(objectID, value.Value)
	if err != nil {
		return SchemaEntity{}, err
	}
	return SchemaEntity{Entity: def.Entity, Description: def.Description, ObjectID: objectID}, nil
}
