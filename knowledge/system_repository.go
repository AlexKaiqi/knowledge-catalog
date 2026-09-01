package knowledge

// System Repository: the immutable protocol publication bundled with the
// Server. Catalog only sees another Repository ID; the knowledge meaning is
// owned here at layer ② and wired by the application root.

import (
	"encoding/json"
	"sort"

	"kc/kernel"
	"kc/snapshot"
)

// SystemSchemaOperations returns fresh values so callers cannot mutate the
// process trust root. Paths are presentation/storage hints; object IDs remain
// the canonical identities.
func SystemSchemaOperations() []Operation {
	return []Operation{
		{
			Op: OpPut, Address: Address{Kind: KindEntity, ObjectID: MetaSchemaV1},
			PathHint: "schemas/meta/schema-definition.v1.aspect.yaml",
			Value: map[string]any{
				"metaSchema": string(MetaSchemaV1), "entity": "SchemaDefinition", "pattern": "record",
				"additionalProperties": false,
				"fields": map[string]any{
					"metaSchema":           map[string]any{"type": "string"},
					"entity":               map[string]any{"type": "string", "required": true, "access": []any{"filter"}},
					"aspect":               map[string]any{"type": "string", "access": []any{"filter"}},
					"pattern":              map[string]any{"type": "string", "required": true, "access": []any{"filter"}},
					"additionalProperties": map[string]any{"type": "boolean"},
					"fields":               map[string]any{"type": "object", "required": true},
				},
			},
		},
		{
			Op: OpPut, Address: Address{Kind: KindEntity, ObjectID: CoreResourceDescriptorSchemaV1},
			PathHint: "schemas/core/resource-descriptor.v1.aspect.yaml",
			Value: map[string]any{
				"metaSchema": string(MetaSchemaV1), "entity": "ResourceDescriptor", "pattern": "record",
				"additionalProperties": true,
				"fields": map[string]any{
					"kind":     map[string]any{"type": "string", "required": true, "access": []any{"filter"}},
					"runtime":  map[string]any{"type": "string", "required": true},
					"protocol": map[string]any{"type": "string", "required": true},
					"access":   map[string]any{"type": "object", "required": true},
				},
			},
		},
		{
			Op: OpPut, Address: Address{Kind: KindEntity, ObjectID: CoreRelationSchemaV1},
			PathHint: "schemas/core/relation.v1.aspect.yaml",
			Value: map[string]any{
				"metaSchema": string(MetaSchemaV1), "entity": "Relation", "pattern": "record",
				"additionalProperties": true,
				"fields": map[string]any{
					"relationId":   map[string]any{"type": "string", "required": true},
					"relationType": map[string]any{"type": "string", "required": true, "access": []any{"filter"}},
					"direction":    map[string]any{"type": "string", "required": true, "access": []any{"filter"}},
					"endpoints":    map[string]any{"type": "relation_endpoint_list", "required": true},
				},
			},
		},
	}
}

func SystemMetaSchemaDigest() kernel.Digest {
	return kernel.CanonicalDigest(SystemSchemaOperations()[0].Value)
}

// SystemRepository is the immutable protocol Repository bundled with the
// Server. Its commit is content-addressed from all published system objects;
// a binary upgrade can publish a new commit without requiring a mutable
// deployment-specific authority or weakening the Meta Schema trust root.
type SystemRepository struct {
	commit  kernel.CommitID
	objects map[ObjectID]Operation
}

func NewSystemRepository() *SystemRepository {
	operations := SystemSchemaOperations()
	objects := make(map[ObjectID]Operation, len(operations))
	for _, operation := range operations {
		objects[operation.Address.ObjectID] = operation
	}
	return &SystemRepository{
		commit: kernel.CommitID(kernel.CanonicalDigest(operations)), objects: objects,
	}
}

func (r *SystemRepository) ID() kernel.RepositoryID { return SystemRepositoryID }
func (r *SystemRepository) Head(ref string) (kernel.CommitID, error) {
	if ref != "" && ref != "HEAD" && ref != snapshot.DefaultRef {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "system ref %s does not exist", ref)
	}
	return r.commit, nil
}
func (r *SystemRepository) GetRef(ref string) (kernel.CommitID, bool) {
	commit, err := r.Head(ref)
	return commit, err == nil
}
func (r *SystemRepository) HasCommit(commit kernel.CommitID) bool { return commit == r.commit }
func (r *SystemRepository) CreateRef(string, kernel.CommitID) error {
	return kernel.Fail(kernel.ErrForbidden, "System Repository is immutable")
}
func (r *SystemRepository) Merge(string, kernel.CommitID, kernel.CommitID) (kernel.CommitID, error) {
	return "", kernel.Fail(kernel.ErrForbidden, "System Repository is immutable")
}
func (r *SystemRepository) Archived() bool { return false }
func (r *SystemRepository) Archive() error {
	return kernel.Fail(kernel.ErrForbidden, "System Repository cannot be archived")
}
func (*SystemRepository) NativeKnowledgeRepository() {}

func (r *SystemRepository) operation(objectID ObjectID, commit kernel.CommitID) (Operation, error) {
	if !r.HasCommit(commit) {
		return Operation{}, kernel.Fail(kernel.ErrVersionUnresolved, "system commit %s does not exist", commit)
	}
	operation, ok := r.objects[objectID]
	if !ok {
		return Operation{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved,
			"system object %s is missing at commit %s", objectID, commit)
	}
	return operation, nil
}

func (r *SystemRepository) Resolve(objectID ObjectID, commit kernel.CommitID) (Resolution, error) {
	operation, err := r.operation(objectID, commit)
	if err != nil {
		if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
			return Resolution{Repository: r.ID(), Commit: commit, ObjectID: objectID,
				Address: Address{Kind: KindEntity, ObjectID: objectID}, Status: StatusUnresolved}, nil
		}
		return Resolution{}, err
	}
	return Resolution{Repository: r.ID(), Commit: commit, ObjectID: objectID,
		Address: operation.Address, PathHint: operation.PathHint,
		Digest: kernel.CanonicalDigest(operation.Value), Status: StatusResolved}, nil
}

func (r *SystemRepository) Read(objectID ObjectID, commit kernel.CommitID) (KnowledgeValue, error) {
	operation, err := r.operation(objectID, commit)
	if err != nil {
		return KnowledgeValue{}, err
	}
	cloned, err := cloneSystemValue(operation.Value)
	if err != nil {
		return KnowledgeValue{}, err
	}
	return KnowledgeValue{
		KnowledgeRef: KnowledgeRef{Repository: r.ID(), Object: objectID},
		Repository:   r.ID(), Commit: commit, Address: operation.Address, Value: cloned,
		Units: []Address{operation.Address}, Declarations: []UnitDeclaration{{
			Address: operation.Address, Digest: kernel.CanonicalDigest(operation.Value),
			DeclarationDigest: DeclarationDigest("", nil),
		}},
	}, nil
}

func cloneSystemValue(value any) (any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var cloned any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return nil, err
	}
	return cloned, nil
}

func (r *SystemRepository) ResolveAddress(address Address, commit kernel.CommitID) (Resolution, error) {
	if err := AssertWritable(address); err != nil {
		return Resolution{}, err
	}
	if address.Kind != KindEntity || address.AspectName != "" || address.MemberKey != "" {
		return Resolution{Repository: r.ID(), Commit: commit, ObjectID: address.ObjectID,
			Address: address, Status: StatusUnresolved}, nil
	}
	return r.Resolve(address.ObjectID, commit)
}

func (r *SystemRepository) ReadAddress(address Address, commit kernel.CommitID) (KnowledgeValue, error) {
	resolution, err := r.ResolveAddress(address, commit)
	if err != nil {
		return KnowledgeValue{}, err
	}
	if resolution.Status != StatusResolved {
		return KnowledgeValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved,
			"system address %s is missing at commit %s", AddressKey(address), commit)
	}
	return r.Read(address.ObjectID, commit)
}

func (r *SystemRepository) ReadMany(objectIDs []ObjectID, commit kernel.CommitID) (map[ObjectID]KnowledgeValue, error) {
	if !r.HasCommit(commit) {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "system commit %s does not exist", commit)
	}
	out := map[ObjectID]KnowledgeValue{}
	for _, objectID := range objectIDs {
		if _, exists := r.objects[objectID]; !exists {
			continue
		}
		value, err := r.Read(objectID, commit)
		if err != nil {
			return nil, err
		}
		out[objectID] = value
	}
	return out, nil
}

func (r *SystemRepository) SchemaObjectIDs(commit kernel.CommitID) ([]ObjectID, error) {
	if !r.HasCommit(commit) {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "system commit %s does not exist", commit)
	}
	ids := make([]ObjectID, 0, len(r.objects))
	for objectID := range r.objects {
		ids = append(ids, objectID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func (r *SystemRepository) ObjectIDsPage(commit kernel.CommitID, limit int, continuation string) (ObjectIDPage, error) {
	ids, err := r.SchemaObjectIDs(commit)
	if err != nil {
		return ObjectIDPage{}, err
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		return ObjectIDPage{}, kernel.Fail(kernel.ErrUsageInvalid, "object page limit cannot exceed 1000")
	}
	start := 0
	if continuation != "" {
		start = sort.Search(len(ids), func(i int) bool { return string(ids[i]) > continuation })
	}
	end := start + limit
	if end > len(ids) {
		end = len(ids)
	}
	next := ""
	if end < len(ids) && end > start {
		next = string(ids[end-1])
	}
	return ObjectIDPage{ObjectIDs: ids[start:end], Continuation: next, Exhausted: end == len(ids)}, nil
}

func (r *SystemRepository) GetProvenance(objectID ObjectID, commit kernel.CommitID) (ProvenanceTrace, error) {
	if _, err := r.operation(objectID, commit); err != nil {
		return ProvenanceTrace{}, err
	}
	return ProvenanceTrace{Repository: r.ID(), Commit: commit, ObjectID: objectID, Chain: []ProvenanceEnvelope{}}, nil
}

func (r *SystemRepository) Log(objectID ObjectID, commit kernel.CommitID, limit int) ([]ObjectRevision, error) {
	operation, err := r.operation(objectID, commit)
	if err != nil {
		return nil, err
	}
	if limit < 0 {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "log limit must be non-negative")
	}
	return []ObjectRevision{{Commit: r.commit, Status: StatusResolved, Digest: kernel.CanonicalDigest(operation.Value)}}, nil
}

func (r *SystemRepository) Diff(objectID ObjectID, from, to kernel.CommitID) (ObjectDiff, error) {
	fromValue, err := r.Read(objectID, from)
	if err != nil {
		return ObjectDiff{}, err
	}
	toValue, err := r.Read(objectID, to)
	if err != nil {
		return ObjectDiff{}, err
	}
	return ObjectDiff{ObjectID: objectID, FromCommit: from, ToCommit: to, From: &fromValue, To: &toValue}, nil
}
