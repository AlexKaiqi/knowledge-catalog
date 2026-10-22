package writer

import (
	"encoding/json"
	"sort"
	"strings"

	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/snapshot"
)

// Branch and merge are Writer operations on one Repository. A branch is an
// ordinary write target (CommitIntent.TargetRef); merge fast-forwards a target
// Ref to a reviewed source commit only after the source changes pass the same
// object and Schema contract a COMMIT onto that target would. Approval is the
// caller's business: Writer checks content, not who agreed to it.

const mergeKind = "MERGE"

// BranchIntent names a new Ref started from a commit of the same Repository.
type BranchIntent struct {
	TargetRepository kernel.RepositoryID
	Ref              string
	// FromRef is used when FromCommit is empty; empty means DefaultRef.
	FromRef    string
	FromCommit kernel.CommitID
}

type BranchReceipt struct {
	RepositoryID kernel.RepositoryID `json:"repositoryId"`
	Ref          string              `json:"ref"`
	CommitID     kernel.CommitID     `json:"commitId"`
}

func (w *Writer) CreateBranch(intent BranchIntent) (receipt BranchReceipt, err error) {
	refs := map[string]any{"repositoryId": string(intent.TargetRepository), "ref": intent.Ref}
	defer func() { err = w.note("branch", refs, err) }()
	if intent.TargetRepository == "" || intent.Ref == "" {
		return BranchReceipt{}, kernel.Fail(kernel.ErrWriteTargetRequired, "branch requires a repository and a ref")
	}
	if err := w.refuseRuntimeSystemMutation(intent.TargetRepository); err != nil {
		return BranchReceipt{}, err
	}
	repo, err := w.writableRepository(intent.TargetRepository)
	if err != nil {
		return BranchReceipt{}, err
	}
	if _, exists := repo.GetRef(intent.Ref); exists {
		return BranchReceipt{}, kernel.Fail(kernel.ErrPreconditionFailed, "ref %s already exists", intent.Ref)
	}
	from := intent.FromCommit
	if from == "" {
		from, err = repo.Head(snapshot.RefOrDefault(intent.FromRef))
		if err != nil {
			return BranchReceipt{}, err
		}
	}
	if err := repo.CreateRef(intent.Ref, from); err != nil {
		return BranchReceipt{}, err
	}
	refs["commitId"] = string(from)
	return BranchReceipt{RepositoryID: intent.TargetRepository, Ref: intent.Ref, CommitID: from}, nil
}

// MergeIntent fast-forwards TargetRef to SourceRef. SourceCommit pins what the
// caller reviewed; ExpectedTargetCommit is the target CAS. Empty values are
// filled from the current Refs on first use and frozen in the command ledger.
type MergeIntent struct {
	TargetRepository     kernel.RepositoryID
	SourceRef            string
	SourceCommit         kernel.CommitID
	TargetRef            string
	ExpectedTargetCommit kernel.CommitID
}

type mergeDigest struct {
	Repository kernel.RepositoryID `json:"repository"`
	SourceRef  string              `json:"sourceRef"`
	Source     kernel.CommitID     `json:"source"`
	TargetRef  string              `json:"targetRef"`
	Expected   kernel.CommitID     `json:"expected"`
}

func (w *Writer) Merge(commandID string, intent MergeIntent) (receipt CommitReceipt, err error) {
	target := snapshot.RefOrDefault(intent.TargetRef)
	refs := map[string]any{"commandId": commandID, "repositoryId": string(intent.TargetRepository), "targetRef": target, "sourceRef": intent.SourceRef}
	defer func() { err = w.note("merge", refs, err) }()
	if commandID == "" {
		return CommitReceipt{}, kernel.Fail(kernel.ErrUsageInvalid, "merge requires a command id")
	}
	if intent.TargetRepository == "" || intent.SourceRef == "" {
		return CommitReceipt{}, kernel.Fail(kernel.ErrWriteTargetRequired, "merge requires a repository and a source ref")
	}
	if intent.SourceRef == target {
		return CommitReceipt{}, kernel.Fail(kernel.ErrUsageInvalid, "merge source and target are the same ref %s", target)
	}
	if err := w.refuseRuntimeSystemMutation(intent.TargetRepository); err != nil {
		return CommitReceipt{}, err
	}
	repo, err := w.writableRepository(intent.TargetRepository)
	if err != nil {
		return CommitReceipt{}, err
	}
	if prior, ok := w.commands.Lookup(commandID); ok && prior.Request.Kind == mergeKind {
		intent.SourceCommit = kernel.CommitID(prior.Request.BaseCommit)
		intent.ExpectedTargetCommit = kernel.CommitID(prior.Request.ExpectedTargetCommit)
	}
	if intent.SourceCommit == "" {
		current, ok := repo.GetRef(intent.SourceRef)
		if !ok {
			return CommitReceipt{}, kernel.Fail(kernel.ErrVersionUnresolved, "ref %s does not exist", intent.SourceRef)
		}
		intent.SourceCommit = current
	}
	if intent.ExpectedTargetCommit == "" {
		intent.ExpectedTargetCommit, err = repo.Head(target)
		if err != nil {
			return CommitReceipt{}, err
		}
	}
	source, expected := intent.SourceCommit, intent.ExpectedTargetCommit
	digest := string(kernel.CanonicalDigest(mergeDigest{
		Repository: intent.TargetRepository, SourceRef: intent.SourceRef, Source: source,
		TargetRef: target, Expected: expected,
	}))
	entry, replayed, err := w.commands.Execute(commandID, digest, snapshot.CommandRequest{
		Kind: mergeKind, RepositoryID: string(intent.TargetRepository), TargetRef: target,
		BaseCommit: string(source), ExpectedTargetCommit: string(expected),
	}, func() (any, error) {
		if current, ok := repo.GetRef(intent.SourceRef); !ok || current != source {
			return nil, snapshot.WriteNotApplied(kernel.Fail(kernel.ErrCandidateMoved,
				"source %s moved from reviewed commit %s", intent.SourceRef, source))
		}
		if err := validateMergeSource(w.store, repo, target, expected, source); err != nil {
			return nil, snapshot.WriteNotApplied(err)
		}
		commitID, err := repo.Merge(target, source, expected)
		if err != nil {
			if code := kernel.CodeOf(err); code == kernel.ErrNonFastForward || code == kernel.ErrVersionUnresolved {
				return nil, snapshot.WriteNotApplied(err)
			}
			return nil, err
		}
		changed := expected != commitID
		if target == snapshot.DefaultRef && changed {
			w.store.NotifyAdvanced(snapshot.Advanced{Store: repo, From: expected, To: commitID})
		}
		changeState := objectmodel.ChangeUnchanged
		if changed {
			changeState = objectmodel.ChangeChanged
		}
		return CommitReceipt{
			ReceiptRef:  commandReceiptReference(mergeKind, commandID, digest),
			CommandID:   commandID,
			Surface:     mergeKind,
			Disposition: DispositionApplied,
			Result: CommitResult{
				Changed:      changeState,
				RepositoryID: intent.TargetRepository,
				CommitID:     commitID,
				TargetRef:    target,
				OldCommit:    expected,
				NewCommit:    commitID,
			},
		}, nil
	})
	if err != nil {
		return CommitReceipt{}, err
	}
	if err := json.Unmarshal(entry.Receipt, &receipt); err != nil {
		return CommitReceipt{}, err
	}
	if replayed {
		receipt.Disposition = DispositionReplayed
	}
	refs["disposition"] = string(receipt.Disposition)
	refs["newCommit"] = string(receipt.Result.NewCommit)
	return receipt, nil
}

func (w *Writer) writableRepository(id kernel.RepositoryID) (snapshot.Store, error) {
	repo, err := w.store.Require(id, kernel.ErrTargetRepositoryDenied)
	if err != nil {
		return nil, err
	}
	if repo.Archived() {
		return nil, kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", id)
	}
	return repo, nil
}

// validateMergeSource rebuilds the source changes as one ChangeSet onto the
// target basis and holds it to the COMMIT contract. Because merge only
// fast-forwards, the published tree is exactly the validated source tree, so
// the object locators the source carries must also agree with its units.
func validateMergeSource(registry snapshot.Registry, repo snapshot.Store, targetRef string, expected, source kernel.CommitID) error {
	if expected == source {
		return nil
	}
	operations, err := mergeOperations(repo, expected, source)
	if err != nil {
		return err
	}
	if len(operations) == 0 {
		return nil
	}
	cs := objectmodel.ChangeSet{
		TargetRepository: repo.ID(), TargetRef: targetRef,
		BaseCommit: expected, ExpectedTargetCommit: expected, Operations: operations,
	}
	if err := validateChangeSet(cs); err != nil {
		return err
	}
	return validateSchemaRefs(registry, repo, cs)
}

type mergeUnit struct {
	path string
	op   objectmodel.Operation
}

func mergeOperations(repo snapshot.Store, from, to kernel.CommitID) ([]objectmodel.Operation, error) {
	changes, changesOK := snapshot.ChangeStoreOf(repo)
	tree, treeOK := snapshot.TreeStoreOf(repo)
	if !changesOK || !treeOK {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"repository %s cannot list and read the changes a merge publishes", repo.ID())
	}
	paths, err := changes.ChangedPaths(from, to)
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	puts := []mergeUnit{}
	removes := []mergeUnit{}
	for _, path := range paths {
		if strings.HasPrefix(path, ".kc/") {
			continue
		}
		if !repofile.ObjectPath(path) {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "merge source changes %s, which is not an object unit", path)
		}
		unit, present, err := mergeUnitAt(tree, path, to)
		if err != nil {
			return nil, err
		}
		if present {
			puts = append(puts, mergeUnit{path: path, op: unit})
			continue
		}
		previous, existed, err := mergeUnitAt(tree, path, from)
		if err != nil {
			return nil, err
		}
		if existed {
			previous.Op = objectmodel.OpRemove
			previous.Value, previous.SchemaRef, previous.ValueSource, previous.PathHint = nil, "", nil, ""
			removes = append(removes, mergeUnit{path: path, op: previous})
		}
	}
	if err := checkMergeLocators(tree, to, puts, removes); err != nil {
		return nil, err
	}
	put := map[string]struct{}{}
	operations := make([]objectmodel.Operation, 0, len(puts)+len(removes))
	for _, unit := range puts {
		put[objectmodel.AddressKey(unit.op.Address)] = struct{}{}
		operations = append(operations, unit.op)
	}
	for _, unit := range removes {
		// A unit moved to another path is a PUT at its new path, not a removal.
		if _, moved := put[objectmodel.AddressKey(unit.op.Address)]; !moved {
			operations = append(operations, unit.op)
		}
	}
	return operations, nil
}

func mergeUnitAt(tree snapshot.TreeReader, path string, commit kernel.CommitID) (objectmodel.Operation, bool, error) {
	content, err := tree.ReadFile(path, commit)
	if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
		return objectmodel.Operation{}, false, nil
	}
	if err != nil {
		return objectmodel.Operation{}, false, err
	}
	op, _, err := ingestFile(path, content)
	if err != nil {
		return objectmodel.Operation{}, false, err
	}
	return op, true, nil
}

func checkMergeLocators(tree snapshot.TreeReader, at kernel.CommitID, puts, removes []mergeUnit) error {
	located := map[objectmodel.ObjectID]map[string]struct{}{}
	paths := func(id objectmodel.ObjectID) (map[string]struct{}, error) {
		if cached, ok := located[id]; ok {
			return cached, nil
		}
		list, err := repofile.ReadObjectLocator(tree, id, at)
		if err != nil {
			return nil, err
		}
		set := make(map[string]struct{}, len(list))
		for _, path := range list {
			set[path] = struct{}{}
		}
		located[id] = set
		return set, nil
	}
	for _, unit := range puts {
		set, err := paths(unit.op.Address.ObjectID)
		if err != nil {
			return err
		}
		if _, ok := set[unit.path]; !ok {
			return kernel.Fail(kernel.ErrPreconditionFailed,
				"merge source unit %s is not located for object %s; write the branch through Writer", unit.path, unit.op.Address.ObjectID)
		}
	}
	for _, unit := range removes {
		set, err := paths(unit.op.Address.ObjectID)
		if err != nil {
			return err
		}
		if _, ok := set[unit.path]; ok {
			return kernel.Fail(kernel.ErrPreconditionFailed,
				"merge source removed %s but object %s still locates it; write the branch through Writer", unit.path, unit.op.Address.ObjectID)
		}
	}
	return nil
}
