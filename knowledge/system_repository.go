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

//go:embed system/schemas system/readme.md
var systemSchemaFS embed.FS

type systemSchemaFile struct {
	objectID ObjectID
	file     string
}

var systemSchemaFiles = []systemSchemaFile{
	{MetaSchemaV1, "system/schemas/schema-definition.v1.aspect.yaml"},
	{CoreResourceDescriptorSchemaV1, "system/schemas/resource-descriptor.v1.aspect.yaml"},
	{CoreRelationSchemaV1, "system/schemas/relation.v1.aspect.yaml"},
	{CoreReadmeSchemaV1, "system/schemas/readme.v1.aspect.yaml"},
}

func (f systemSchemaFile) pathHint() string {
	name := f.file
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	return CanonicalSchemaDir + "/" + name
}

// SystemSchemaOperations returns fresh values so callers cannot mutate the
// process trust root. Paths are presentation/storage hints; object IDs remain
	// the canonical identities. YAML under system/schemas/ is the tracked
	// publication source and matches the published flat _schemas/ tree; Canonical
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

// SystemReadmeOperation is the System Repository description: a README Aspect
// whose identity lives in markdown frontmatter.
func SystemReadmeOperation() Operation {
	raw, err := systemSchemaFS.ReadFile("system/readme.md")
	if err != nil {
		panic(fmt.Sprintf("system readme: %v", err))
	}
	body, err := markdownBodyAfterFrontmatter(string(raw))
	if err != nil {
		panic(fmt.Sprintf("system readme: %v", err))
	}
	return Operation{
		Op: OpPut, Address: Address{Kind: KindAspect, ObjectID: SystemReadmeObjectID, AspectName: ReadmeAspect},
		PathHint: RepositoryReadmePath, SchemaRef: string(CoreReadmeSchemaV1),
		Value: map[string]any{"body": body},
	}
}

// SystemPublicationOperations is Schema objects plus the System README.
func SystemPublicationOperations() []Operation {
	operations := SystemSchemaOperations()
	return append(operations, SystemReadmeOperation())
}

func SystemMetaSchemaDigest() kernel.Digest {
	return kernel.CanonicalDigest(SystemSchemaOperations()[0].Value)
}

// SystemRepository is the immutable protocol Repository bundled with the
// Server. Its commit is content-addressed from all published system objects;
// a binary upgrade can publish a new commit without requiring a mutable
// deployment-specific authority or weakening the Meta Schema trust root.
type SystemRepository struct {
	commit    kernel.CommitID
	objects   map[ObjectID]Operation
	instances map[ObjectID][]Operation
}

func NewSystemRepository() *SystemRepository {
	operations := SystemSchemaOperations()
	objects := make(map[ObjectID]Operation, len(operations))
	for _, operation := range operations {
		objects[operation.Address.ObjectID] = operation
	}
	readme := SystemReadmeOperation()
	instances := map[ObjectID][]Operation{readme.Address.ObjectID: {readme}}
	published := append(append([]Operation{}, operations...), readme)
	return &SystemRepository{
		commit:    kernel.CommitID(kernel.CanonicalDigest(published)),
		objects:   objects,
		instances: instances,
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

var (
	_ KnowledgeFileReader = (*SystemRepository)(nil)
	_ BindingLocator      = (*SystemRepository)(nil)
)

func (r *SystemRepository) BindingSchemaObjectIDs(commit kernel.CommitID) ([]ObjectID, error) {
	if !r.HasCommit(commit) {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "system commit %s does not exist", commit)
	}
	return nil, nil
}

func (r *SystemRepository) ReadKnowledgeFile(rel string, commit kernel.CommitID) ([]byte, error) {
	if !r.HasCommit(commit) {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "system commit %s does not exist", commit)
	}
	if rel == RepositoryReadmePath {
		raw, err := systemSchemaFS.ReadFile("system/readme.md")
		if err != nil {
			return nil, err
		}
		return raw, nil
	}
	for _, file := range systemSchemaFiles {
		if file.pathHint() == rel {
			raw, err := systemSchemaFS.ReadFile(file.file)
			if err != nil {
				return nil, err
			}
			return raw, nil
		}
	}
	return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "system file %s is missing", rel)
}

func markdownBodyAfterFrontmatter(content string) (string, error) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", fmt.Errorf("missing frontmatter")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return "", fmt.Errorf("unclosed frontmatter")
	}
	body := strings.TrimSpace(strings.Join(lines[end+1:], "\n"))
	if body == "" {
		return "", fmt.Errorf("empty markdown body")
	}
	return body, nil
}

func assembleSystemUnits(objectID ObjectID, units []Operation) (Address, any, error) {
	if len(units) == 1 && IsEntityBlob(units[0].Address) {
		cloned, err := cloneSystemValue(units[0].Value)
		return units[0].Address, cloned, err
	}
	out := map[string]any{}
	root := Address{Kind: KindEntity, ObjectID: objectID}
	for _, unit := range units {
		if unit.Address.AspectName == "" {
			return Address{}, nil, fmt.Errorf("system object %s mixes blob and aspects", objectID)
		}
		cloned, err := cloneSystemValue(unit.Value)
		if err != nil {
			return Address{}, nil, err
		}
		out[unit.Address.AspectName] = cloned
	}
	return root, out, nil
}

func (r *SystemRepository) units(objectID ObjectID, commit kernel.CommitID) ([]Operation, error) {
	if !r.HasCommit(commit) {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "system commit %s does not exist", commit)
	}
	if ops, ok := r.instances[objectID]; ok {
		return ops, nil
	}
	operation, ok := r.objects[objectID]
	if !ok {
		return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved,
			"system object %s is missing at commit %s", objectID, commit)
	}
	return []Operation{operation}, nil
}

func (r *SystemRepository) operation(objectID ObjectID, commit kernel.CommitID) (Operation, error) {
	units, err := r.units(objectID, commit)
	if err != nil {
		return Operation{}, err
	}
	if len(units) != 1 {
		return Operation{}, kernel.Fail(kernel.ErrPreconditionFailed, "system object %s is not a single unit", objectID)
	}
	return units[0], nil
}

func (r *SystemRepository) Resolve(objectID ObjectID, commit kernel.CommitID) (Resolution, error) {
	units, err := r.units(objectID, commit)
	if err != nil {
		if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
			return Resolution{Repository: r.ID(), Commit: commit, ObjectID: objectID,
				Address: Address{Kind: KindEntity, ObjectID: objectID}, Status: StatusUnresolved}, nil
		}
		return Resolution{}, err
	}
	address, value, err := assembleSystemUnits(objectID, units)
	if err != nil {
		return Resolution{}, err
	}
	return Resolution{Repository: r.ID(), Commit: commit, ObjectID: objectID,
		Address: address, PathHint: units[0].PathHint,
		Digest: kernel.CanonicalDigest(value), Status: StatusResolved}, nil
}

func (r *SystemRepository) Read(objectID ObjectID, commit kernel.CommitID) (KnowledgeValue, error) {
	units, err := r.units(objectID, commit)
	if err != nil {
		return KnowledgeValue{}, err
	}
	address, value, err := assembleSystemUnits(objectID, units)
	if err != nil {
		return KnowledgeValue{}, err
	}
	declarations := make([]UnitDeclaration, 0, len(units))
	addrs := make([]Address, 0, len(units))
	for _, unit := range units {
		addrs = append(addrs, unit.Address)
		declarations = append(declarations, UnitDeclaration{
			Address: unit.Address, Digest: kernel.CanonicalDigest(unit.Value),
			DeclarationDigest: DeclarationDigest(unit.SchemaRef, nil),
			SchemaRef:         unit.SchemaRef,
		})
	}
	return KnowledgeValue{
		KnowledgeRef: KnowledgeRef{Repository: r.ID(), Object: objectID},
		Repository:   r.ID(), Commit: commit, Address: address, Value: value,
		Units: addrs, Declarations: declarations,
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
	units, err := r.units(address.ObjectID, commit)
	if err != nil {
		if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
			return Resolution{Repository: r.ID(), Commit: commit, ObjectID: address.ObjectID,
				Address: address, Status: StatusUnresolved}, nil
		}
		return Resolution{}, err
	}
	for _, unit := range units {
		if AddressKey(unit.Address) == AddressKey(address) {
			return Resolution{Repository: r.ID(), Commit: commit, ObjectID: address.ObjectID,
				Address: unit.Address, PathHint: unit.PathHint, SchemaRef: unit.SchemaRef,
				Digest: kernel.CanonicalDigest(unit.Value), Status: StatusResolved}, nil
		}
	}
	return Resolution{Repository: r.ID(), Commit: commit, ObjectID: address.ObjectID,
		Address: address, Status: StatusUnresolved}, nil
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
	units, err := r.units(address.ObjectID, commit)
	if err != nil {
		return KnowledgeValue{}, err
	}
	for _, unit := range units {
		if AddressKey(unit.Address) == AddressKey(address) {
			cloned, err := cloneSystemValue(unit.Value)
			if err != nil {
				return KnowledgeValue{}, err
			}
			return KnowledgeValue{
				KnowledgeRef: KnowledgeRef{Repository: r.ID(), Object: address.ObjectID},
				Repository:   r.ID(), Commit: commit, Address: unit.Address, Value: cloned,
				Declarations: []UnitDeclaration{{
					Address: unit.Address, Digest: kernel.CanonicalDigest(unit.Value),
					DeclarationDigest: DeclarationDigest(unit.SchemaRef, nil),
					SchemaRef:         unit.SchemaRef,
				}},
			}, nil
		}
	}
	return KnowledgeValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved,
		"system address %s is missing at commit %s", AddressKey(address), commit)
}

func (r *SystemRepository) ReadMany(objectIDs []ObjectID, commit kernel.CommitID) (map[ObjectID]KnowledgeValue, error) {
	if !r.HasCommit(commit) {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "system commit %s does not exist", commit)
	}
	out := map[ObjectID]KnowledgeValue{}
	for _, objectID := range objectIDs {
		if _, exists := r.objects[objectID]; !exists {
			if _, inst := r.instances[objectID]; !inst {
				continue
			}
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
	if !r.HasCommit(commit) {
		return ObjectIDPage{}, kernel.Fail(kernel.ErrVersionUnresolved, "system commit %s does not exist", commit)
	}
	ids := make([]ObjectID, 0, len(r.objects)+len(r.instances))
	for objectID := range r.objects {
		ids = append(ids, objectID)
	}
	for objectID := range r.instances {
		ids = append(ids, objectID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
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
	if _, err := r.units(objectID, commit); err != nil {
		return ProvenanceTrace{}, err
	}
	return ProvenanceTrace{Repository: r.ID(), Commit: commit, ObjectID: objectID, Chain: []ProvenanceEnvelope{}}, nil
}

func (r *SystemRepository) Log(objectID ObjectID, commit kernel.CommitID, query ObjectLogQuery) ([]ObjectRevision, error) {
	units, err := r.units(objectID, commit)
	if err != nil {
		return nil, err
	}
	if query.Limit < 0 {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "log limit must be non-negative")
	}
	if query.After != "" {
		return []ObjectRevision{}, nil
	}
	_, value, err := assembleSystemUnits(objectID, units)
	if err != nil {
		return nil, err
	}
	return []ObjectRevision{{Commit: r.commit, Status: StatusResolved, Digest: kernel.CanonicalDigest(value)}}, nil
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
