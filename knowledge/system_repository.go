package knowledge

// System Repository: the immutable protocol publication bundled with the
// Server. Catalog only sees another Repository ID; the knowledge meaning is
// owned here at layer ② and wired by the application root.

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"kc/kernel"
	"kc/snapshot"
)

//go:embed system/schemas
var systemSchemaFS embed.FS

type systemSchemaFile struct {
	objectID ObjectID
	file     string
}

var systemSchemaFiles = []systemSchemaFile{
	{MetaSchemaV1, "system/schemas/schema-definition.v1.aspect.yaml"},
	{CoreResourceDescriptorSchemaV1, "system/schemas/resource-descriptor.v1.aspect.yaml"},
	{CoreRelationSchemaV1, "system/schemas/relation.v1.aspect.yaml"},
}

func (f systemSchemaFile) pathHint() string {
	return strings.TrimPrefix(f.file, "system/")
}

// SystemSchemaOperations returns fresh values so callers cannot mutate the
// process trust root. Paths are presentation/storage hints; object IDs remain
// the canonical identities. YAML under system/schemas/ is the tracked
// publication source and matches the published flat schemas/ tree; Canonical
// JSON digest is computed from the parsed value.
func SystemSchemaOperations() []Operation {
	out := make([]Operation, 0, len(systemSchemaFiles))
	for _, file := range systemSchemaFiles {
		value, err := loadSystemSchemaValue(file.file)
		if err != nil {
			panic(fmt.Sprintf("system schema %s: %v", file.file, err))
		}
		out = append(out, Operation{
			Op: OpPut, Address: Address{Kind: KindEntity, ObjectID: file.objectID},
			PathHint: file.pathHint(), Value: value,
		})
	}
	return out
}

func loadSystemSchemaValue(name string) (map[string]any, error) {
	raw, err := systemSchemaFS.ReadFile(name)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err := yaml.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	cloned, err := cloneSystemValue(value)
	if err != nil {
		return nil, err
	}
	body, ok := cloned.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("must unmarshal as an object")
	}
	return body, nil
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

func (r *SystemRepository) Log(objectID ObjectID, commit kernel.CommitID, query ObjectLogQuery) ([]ObjectRevision, error) {
	operation, err := r.operation(objectID, commit)
	if err != nil {
		return nil, err
	}
	if query.Limit < 0 {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "log limit must be non-negative")
	}
	if query.After != "" {
		return []ObjectRevision{}, nil
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
