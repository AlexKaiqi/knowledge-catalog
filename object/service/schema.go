package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"kc/kernel"
	objectmodel "kc/object"
	"sort"
)

type SchemaPageCursor struct {
	Version    int                  `json:"version"`
	Repository kernel.RepositoryID  `json:"repository"`
	Commit     kernel.CommitID      `json:"commit"`
	After      objectmodel.ObjectID `json:"after"`
	Check      kernel.Digest        `json:"check"`
}

// SchemaEntity is one published type name. It is not the Schema document and
// not DESCRIBE_SCHEMA. Aspect tells several contracts of one Entity apart, so a
// caller can pick the Aspect it needs before describing any contract.
type SchemaEntity struct {
	Entity      string               `json:"entity"`
	Aspect      string               `json:"aspect,omitempty"`
	Description string               `json:"description,omitempty"`
	ObjectID    objectmodel.ObjectID `json:"objectId"`
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
		After      objectmodel.ObjectID
	}{cursor.Version, cursor.Repository, cursor.Commit, cursor.After})
}

func SchemaEntityDirectory(repo objectmodel.Repository, commit kernel.CommitID, ids []objectmodel.ObjectID) ([]SchemaEntity, error) {
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

func SchemaEntityName(repo objectmodel.Repository, commit kernel.CommitID, objectID objectmodel.ObjectID) (SchemaEntity, error) {
	value, err := repo.Read(objectID, commit)
	if err != nil {
		return SchemaEntity{}, err
	}
	def, err := objectmodel.ParseSchemaDefinition(objectID, value.Value)
	if err != nil {
		return SchemaEntity{}, err
	}
	return SchemaEntity{Entity: def.Entity, Aspect: def.Aspect, Description: def.Description, ObjectID: objectID}, nil
}

func DecodeSchemaPageCursor(raw string, repository kernel.RepositoryID, commit kernel.CommitID) (objectmodel.ObjectID, error) {
	if raw == "" {
		return "", nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "schema continuation is invalid")
	}
	var cursor SchemaPageCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.Version != 1 ||
		cursor.Repository != repository || cursor.Commit != commit || cursor.Check != SchemaCursorCheck(cursor) {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "schema continuation does not match the fixed Repository basis")
	}
	return cursor.After, nil
}

func EncodeSchemaPageCursor(repository kernel.RepositoryID, commit kernel.CommitID, after objectmodel.ObjectID) string {
	cursor := SchemaPageCursor{Version: 1, Repository: repository, Commit: commit, After: after}
	cursor.Check = SchemaCursorCheck(cursor)
	raw, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(raw)
}

type SchemaBrowseRequest struct {
	Commit       kernel.CommitID
	Limit        int
	Continuation string
}
type SchemaBrowseExecutor struct{ Repository objectmodel.Repository }

func (e SchemaBrowseExecutor) Execute(_ context.Context, request SchemaBrowseRequest) (SchemaPageResponse, error) {
	if e.Repository == nil {
		return SchemaPageResponse{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "schema browse service is unavailable")
	}
	if request.Limit < 1 {
		return SchemaPageResponse{}, kernel.Fail(kernel.ErrUsageInvalid, "schema browse limit must be positive")
	}
	after, err := DecodeSchemaPageCursor(request.Continuation, e.Repository.ID(), request.Commit)
	if err != nil {
		return SchemaPageResponse{}, err
	}
	store, ok := e.Repository.(objectmodel.SchemaStore)
	if !ok {
		return SchemaPageResponse{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"repository %s does not provide bounded schema discovery", e.Repository.ID())
	}
	ids, err := store.SchemaObjectIDs(request.Commit)
	if err != nil {
		return SchemaPageResponse{}, err
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	start := 0
	if after != "" {
		start = sort.Search(len(ids), func(i int) bool { return ids[i] > after })
	}
	end := start + request.Limit
	if end > len(ids) {
		end = len(ids)
	}
	listed, err := SchemaEntityDirectory(e.Repository, request.Commit, ids[start:end])
	if err != nil {
		return SchemaPageResponse{}, err
	}
	exhausted := end == len(ids)
	continuation := ""
	if !exhausted && end > start {
		continuation = EncodeSchemaPageCursor(e.Repository.ID(), request.Commit, ids[end-1])
	}
	return SchemaPageResponse{
		Repository: e.Repository.ID(), Commit: request.Commit, Schemas: listed,
		Continuation: continuation,
	}, nil
}
