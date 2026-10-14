package service

import (
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
)

type namedSchemaRepo struct{ objectmodel.Repository }

func (namedSchemaRepo) ID() kernel.RepositoryID { return "kr://tables-only" }

func (namedSchemaRepo) Read(objectID objectmodel.ObjectID, commit kernel.CommitID) (objectmodel.KnowledgeValue, error) {
	return objectmodel.KnowledgeValue{
		KnowledgeRef: objectmodel.KnowledgeRef{Object: objectID},
		Value: map[string]any{
			"entity": "Note", "description": "A short note.", "pattern": "record",
			"fields": map[string]any{"body": map[string]any{"type": "string", "access": []any{"text"}}},
		},
	}, nil
}

func TestSchemaListNamesEntitiesWithoutContractDetails(t *testing.T) {
	listed, err := SchemaEntityDirectory(namedSchemaRepo{}, "commit-1", []objectmodel.ObjectID{"schema/note"})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ObjectID != "schema/note" || listed[0].Entity != "Note" || listed[0].Description != "A short note." {
		t.Fatalf("schema list must name the entity: %#v", listed)
	}
}
