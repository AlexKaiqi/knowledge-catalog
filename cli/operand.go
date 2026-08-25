package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"kc/kernel"
	"kc/repository"
)

// Flag decoding shared by more than one verb. A helper belongs here when two
// verbs must agree on what a flag means; a one-verb reader stays with its verb.

// requireRepo resolves a mounted repository. Every verb that names --repo goes
// through here so the error code and wording are the same everywhere.
// Verbs needing layer ② ask ws.Store.Knowledge instead.
func requireRepo(ws *Home, repositoryID string) (repository.SnapshotStore, error) {
	repo, ok := ws.Store.Get(kernel.RepositoryID(repositoryID))
	if !ok {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "unknown repository %s", repositoryID)
	}
	return repo, nil
}

// repoFlag is requireRepo for verbs that only need the id validated.
func (cx *invocation) repoFlag() (kernel.RepositoryID, error) {
	id, err := cx.require("repo")
	if err != nil {
		return "", err
	}
	if _, err := requireRepo(cx.WS, id); err != nil {
		return "", err
	}
	return kernel.RepositoryID(id), nil
}

// targetRef is the write target, defaulting to the protocol branch.
func (cx *invocation) targetRef(name string) string {
	return repository.RefOrDefault(cx.flag(name))
}

// limitFrom reads --limit. A missing flag falls back; a non-numeric or negative
// value is a usage error. Verbs used to disagree here — one silently treated
// garbage as zero — which made `kc log --limit abc` quietly return everything.
func limitFrom(flags map[string]FlagValue, fallback int) (int, error) {
	raw := FlagString(flags, "limit")
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("--limit must be a non-negative number")
	}
	return n, nil
}

// pinCommit freezes {repo, commit} for the duration of one command: an explicit
// --commit, else the tip of --ref at this instant. Nothing re-reads the ref later.
func pinCommit(ws *Home, flags map[string]FlagValue) (kernel.RepositoryID, kernel.CommitID, error) {
	repositoryID, err := RequireFlag(flags, "repo")
	if err != nil {
		return "", "", err
	}
	repo, err := requireRepo(ws, repositoryID)
	if err != nil {
		return "", "", err
	}
	if commit := FlagString(flags, "commit"); commit != "" {
		flags[resolvedCommitFlag] = commit
		return kernel.RepositoryID(repositoryID), kernel.CommitID(commit), nil
	}
	ref := repository.RefOrDefault(FlagString(flags, "ref"))
	commitID, ok := repo.GetRef(ref)
	if !ok {
		return "", "", fmt.Errorf("ref %s does not exist in %s", ref, repositoryID)
	}
	flags[resolvedCommitFlag] = string(commitID)
	return kernel.RepositoryID(repositoryID), commitID, nil
}

// addressFrom builds the write/read unit key: object_id plus optional aspect and
// member. --member without --aspect is not an address.
func addressFrom(flags map[string]FlagValue) (kernel.Address, error) {
	objectID, err := RequireFlag(flags, "object")
	if err != nil {
		return kernel.Address{}, err
	}
	aspect := FlagString(flags, "aspect")
	member := FlagString(flags, "member")
	if member != "" {
		if aspect == "" {
			return kernel.Address{}, fmt.Errorf("Member address requires --aspect and --member")
		}
		return kernel.Address{Kind: kernel.KindMember, ObjectID: kernel.ObjectID(objectID), AspectName: aspect, MemberKey: member}, nil
	}
	if aspect != "" {
		return kernel.Address{Kind: kernel.KindAspect, ObjectID: kernel.ObjectID(objectID), AspectName: aspect}, nil
	}
	kind := kernel.AddressKind(FlagString(flags, "kind"))
	if kind == "" {
		kind = kernel.KindEntity
	}
	return kernel.Address{Kind: kind, ObjectID: kernel.ObjectID(objectID)}, nil
}

func parseJSON(text, label string) (any, error) {
	var value any
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON", label)
	}
	return value, nil
}

// loadJSONFlag reads a value body from --file or --value/--payload, never both.
func loadJSONFlag(flags map[string]FlagValue, label string) (any, bool, error) {
	file := FlagString(flags, "file")
	raw := FlagString(flags, "value")
	if raw == "" {
		raw = FlagString(flags, "payload")
	}
	if file != "" && raw != "" {
		return nil, false, fmt.Errorf("use only one of --file or --value/--payload")
	}
	if file != "" {
		body, err := os.ReadFile(file)
		if err != nil {
			return nil, false, err
		}
		value, err := parseJSON(string(body), file)
		return value, true, err
	}
	if raw != "" {
		value, err := parseJSON(raw, label)
		return value, true, err
	}
	return nil, false, nil
}

func decodeChangeSet(body []byte, label string) (repository.CommitChangeSet, error) {
	var raw repository.CommitChangeSet
	if err := json.Unmarshal(body, &raw); err != nil {
		return repository.CommitChangeSet{}, fmt.Errorf("%s is not valid JSON", label)
	}
	if raw.TargetRepository != "" && raw.Operations != nil {
		return raw, nil
	}
	var wrapped struct {
		ChangeSet repository.CommitChangeSet `json:"changeSet"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil || wrapped.ChangeSet.TargetRepository == "" || wrapped.ChangeSet.Operations == nil {
		return repository.CommitChangeSet{}, fmt.Errorf("changeset must include targetRepository and operations")
	}
	return wrapped.ChangeSet, nil
}

// originFrom builds the provenance envelope, or nil when no origin flag was
// given. DERIVATION without a pinned input is rejected downstream by kernel.
func originFrom(flags map[string]FlagValue) *kernel.ProvenanceEnvelope {
	originKind := FlagString(flags, "origin-kind")
	sourceRefs := FlagStrings(flags, "source-ref")
	evidenceRefs := FlagStrings(flags, "evidence-ref")
	actor := FlagString(flags, "actor-ref")
	activity := FlagString(flags, "activity-ref")
	inputWorkspaceVersion := FlagString(flags, "input-workspace-version")
	spec := FlagString(flags, "algorithm-spec")
	model := FlagString(flags, "algorithm-model")
	hash := FlagString(flags, "algorithm-hash")
	produced := FlagString(flags, "produced-at")
	if originKind == "" && len(sourceRefs) == 0 && len(evidenceRefs) == 0 && actor == "" && activity == "" && inputWorkspaceVersion == "" && spec == "" && model == "" && hash == "" && produced == "" {
		return nil
	}
	kind := kernel.OriginKind(originKind)
	if kind == "" {
		kind = kernel.OriginSource
	}
	env := &kernel.ProvenanceEnvelope{
		OriginKind:               kind,
		ActorRef:                 actor,
		ActivityRef:              activity,
		SourceRefs:               sourceRefs,
		EvidenceRefs:             evidenceRefs,
		InputWorkspaceVersionRef: inputWorkspaceVersion,
		ProducedAt:               produced,
	}
	if spec != "" || model != "" || hash != "" {
		env.Algorithm = &kernel.AlgorithmRef{
			DerivationSpecRef: spec,
			ModelRef:          model,
			CodeHash:          hash,
		}
	}
	return env
}

func preconditionFrom(flags map[string]FlagValue) (*repository.Precondition, error) {
	ifAbsent := FlagBool(flags, "if-absent")
	digest := FlagString(flags, "if-digest")
	if ifAbsent && digest != "" {
		return nil, fmt.Errorf("use only one of --if-absent or --if-digest")
	}
	if ifAbsent {
		return &repository.Precondition{Type: repository.IfAbsent}, nil
	}
	if digest != "" {
		return &repository.Precondition{Type: repository.IfDigestEquals, Digest: kernel.Digest(digest)}, nil
	}
	return nil, nil
}

func writeOperation(flags map[string]FlagValue, op repository.OpKind, value any) (repository.Operation, error) {
	address, err := addressFrom(flags)
	if err != nil {
		return repository.Operation{}, err
	}
	pre, err := preconditionFrom(flags)
	if err != nil {
		return repository.Operation{}, err
	}
	var source *repository.ValueSource
	if raw := FlagString(flags, "value-source"); raw != "" {
		if op != repository.OpPut {
			return repository.Operation{}, fmt.Errorf("--value-source is only valid with PUT")
		}
		var parsed repository.ValueSource
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			return repository.Operation{}, fmt.Errorf("--value-source is not valid JSON")
		}
		if err := repository.ValidateValueSource(&parsed); err != nil {
			return repository.Operation{}, err
		}
		source = &parsed
	}
	return repository.Operation{
		Op:           op,
		Address:      address,
		Value:        value,
		PathHint:     FlagString(flags, "path-hint"),
		SchemaRef:    FlagString(flags, "schema-ref"),
		ValueSource:  source,
		Precondition: pre,
	}, nil
}

// dropByID removes one identified rule from a list, reporting whether it was
// there. Shared by allow / hook / gate, which all revoke by id.
func dropByID[T any](items []T, id string, idOf func(T) string) ([]T, bool) {
	kept := make([]T, 0, len(items))
	found := false
	for _, item := range items {
		if idOf(item) == id {
			found = true
			continue
		}
		kept = append(kept, item)
	}
	return kept, found
}
