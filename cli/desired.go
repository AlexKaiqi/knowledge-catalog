package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"

	kcclient "kc/client"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/knowledge/writer"
	"kc/snapshot"
)

func ingestDesired(flags map[string]FlagValue, dir, repoID, targetRef string, base kernel.CommitID) (writer.IngestPreview, error) {
	preview, err := writer.Ingest(dir, kernel.RepositoryID(repoID), base)
	if err != nil {
		return writer.IngestPreview{}, err
	}
	if provenance := originFrom(flags); provenance != nil {
		preview.ChangeSet.Provenance = provenance
	}
	preview.ChangeSet.TargetRef = snapshot.RefOrDefault(targetRef)
	preview.ChangeSet.ExpectedTargetCommit = base
	return preview, nil
}

func classifyDesiredChanges(ops []knowledge.Operation, current map[string]string) []map[string]any {
	changes := make([]map[string]any, 0)
	for _, op := range ops {
		row := map[string]any{"objectId": string(op.Address.ObjectID)}
		if op.Address.AspectName != "" {
			row["aspectName"] = op.Address.AspectName
		}
		if op.Address.MemberKey != "" {
			row["memberKey"] = op.Address.MemberKey
		}
		if op.Op == knowledge.OpRemove {
			row["change"] = "remove"
			changes = append(changes, row)
			continue
		}
		digest := string(kernel.CanonicalDigest(op.Value))
		existing, ok := current[knowledge.AddressKey(op.Address)]
		switch {
		case !ok:
			row["change"] = "add"
			changes = append(changes, row)
		case existing != digest:
			row["change"] = "update"
			changes = append(changes, row)
		}
	}
	sort.Slice(changes, func(i, j int) bool {
		left, _ := changes[i]["objectId"].(string)
		right, _ := changes[j]["objectId"].(string)
		if left != right {
			return left < right
		}
		leftAspect, _ := changes[i]["aspectName"].(string)
		rightAspect, _ := changes[j]["aspectName"].(string)
		if leftAspect != rightAspect {
			return leftAspect < rightAspect
		}
		leftMember, _ := changes[i]["memberKey"].(string)
		rightMember, _ := changes[j]["memberKey"].(string)
		return leftMember < rightMember
	})
	return changes
}

func desiredDiffResult(repository string, commit kernel.CommitID, ops []knowledge.Operation, current map[string]string) map[string]any {
	return map[string]any{
		"repository": repository,
		"commit":     commit,
		"changes":    classifyDesiredChanges(ops, current),
	}
}

func currentDigests(r *reader.Reader, repositoryID kernel.RepositoryID, commit kernel.CommitID, ops []knowledge.Operation) (map[string]string, error) {
	out := map[string]string{}
	if r == nil || commit == "" {
		return out, nil
	}
	for _, op := range ops {
		resolution, err := r.ResolveAddress(repositoryID, op.Address, commit)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				continue
			}
			return nil, err
		}
		if resolution.Status == knowledge.StatusUnresolved || resolution.Digest == "" {
			continue
		}
		out[knowledge.AddressKey(op.Address)] = string(resolution.Digest)
	}
	return out, nil
}

func desiredBaseCommit(cx *invocation, repositoryID kernel.RepositoryID) (kernel.CommitID, error) {
	if base := cx.flag("base"); base != "" {
		return kernel.CommitID(base), nil
	}
	repo, err := requireRepo(cx.WS, string(repositoryID))
	if err != nil {
		return "", err
	}
	head, err := repo.Head(cx.targetRef("ref"))
	if err != nil {
		return "", err
	}
	return head, nil
}

// requireSnapshotRepository rejects catalog targets and requires the named
// repository to be a Snapshot Repository of the active workspace. It is the
// shared prelude of every local desired-state write surface.
func requireSnapshotRepository(cx *invocation, repositoryID string) error {
	if _, isCatalog := cx.WS.Catalogs[repositoryID]; isCatalog {
		return kernel.Fail(kernel.ErrTargetRepositoryDenied, "catalog %s is not a Snapshot Repository", repositoryID)
	}
	_, err := requireRepo(cx.WS, repositoryID)
	return err
}

// prepareDesiredIngest runs the shared local desired-state pipeline: deny
// catalog targets, require a Snapshot Repository, fix the base commit, ingest
// the desired directory, and collect current digests for the change set.
func prepareDesiredIngest(cx *invocation, repositoryID, dir string) (writer.IngestPreview, kernel.CommitID, map[string]string, error) {
	if err := requireSnapshotRepository(cx, repositoryID); err != nil {
		return writer.IngestPreview{}, "", nil, err
	}
	base, err := desiredBaseCommit(cx, kernel.RepositoryID(repositoryID))
	if err != nil {
		return writer.IngestPreview{}, "", nil, err
	}
	preview, err := ingestDesired(cx.Flags, dir, repositoryID, cx.targetRef("ref"), base)
	if err != nil {
		return writer.IngestPreview{}, "", nil, err
	}
	current, err := currentDigests(cx.WS.Reader, kernel.RepositoryID(repositoryID), base, preview.ChangeSet.Operations)
	if err != nil {
		return writer.IngestPreview{}, "", nil, err
	}
	return preview, base, current, nil
}

func replayedDesiredReceipt(cx *invocation, commandID string) (any, bool) {
	entry, ok := cx.WS.Writer.Lookup(commandID)
	if !ok || entry.Receipt.CommandID == "" {
		return nil, false
	}
	receipt := entry.Receipt
	receipt.Disposition = writer.DispositionReplayed
	return receipt, true
}

func parseResolution(raw any) (knowledge.Resolution, error) {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return knowledge.Resolution{}, err
	}
	var resolution knowledge.Resolution
	if err := json.Unmarshal(encoded, &resolution); err != nil {
		return knowledge.Resolution{}, err
	}
	return resolution, nil
}

// diffPreflightConcurrency bounds the concurrent resolve fan-out of the
// remote diff preflight. On a 53k object first import the serial loop spent
// one HTTP round trip per address before the commit POST even started.
const diffPreflightConcurrency = 16

// remoteCurrentDigests resolves the current digest of each operation's address
// at the fixed base commit. Resolution fan-out is bounded and concurrent; the
// result and error semantics match the former serial loop: unresolved
// addresses and empty digests contribute nothing, and any other error fails
// the whole preflight.
func remoteCurrentDigests(ctx context.Context, client *kcclient.Client, repository string, commit kernel.CommitID, ops []knowledge.Operation, options kcclient.RequestOptions) (map[string]string, error) {
	out := make(map[string]string, len(ops))
	if commit == "" || len(ops) == 0 {
		return out, nil
	}
	workers := diffPreflightConcurrency
	if workers > len(ops) {
		workers = len(ops)
	}
	var (
		next     int64
		mu       sync.Mutex
		firstErr error
		wg       sync.WaitGroup
	)
	wg.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer wg.Done()
			for {
				index := int(atomic.AddInt64(&next, 1)) - 1
				if index >= len(ops) {
					return
				}
				mu.Lock()
				abort := firstErr != nil
				mu.Unlock()
				if abort {
					return
				}
				op := ops[index]
				var raw any
				err := client.KnowledgeService().Resolve(ctx, kcclient.KnowledgeResolveRequest{
					Repository: repository,
					Commit:     string(commit),
					Object:     string(op.Address.ObjectID),
					Aspect:     op.Address.AspectName,
					Member:     op.Address.MemberKey,
				}, options, &raw)
				if err != nil {
					if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
						continue
					}
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					return
				}
				resolution, err := parseResolution(raw)
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					return
				}
				if resolution.Status == knowledge.StatusUnresolved || resolution.Digest == "" {
					continue
				}
				mu.Lock()
				out[knowledge.AddressKey(op.Address)] = string(resolution.Digest)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

// prepareRemoteDesiredIngest runs the shared remote desired-state pipeline:
// default the base commit to the remote head of the target ref, ingest the
// desired directory, and collect current digests through the typed client.
func prepareRemoteDesiredIngest(ctx context.Context, client *kcclient.Client, flags map[string]FlagValue, repository, dir string, options kcclient.RequestOptions) (writer.IngestPreview, kernel.CommitID, map[string]string, error) {
	ref := snapshotRef(flags)
	base := kernel.CommitID(FlagString(flags, "base"))
	if base == "" {
		var err error
		base, err = remoteHeadCommit(ctx, client, repository, ref, options)
		if err != nil {
			return writer.IngestPreview{}, "", nil, err
		}
	}
	preview, err := ingestDesired(flags, dir, repository, ref, base)
	if err != nil {
		return writer.IngestPreview{}, "", nil, err
	}
	current, err := remoteCurrentDigests(ctx, client, repository, base, preview.ChangeSet.Operations, options)
	if err != nil {
		return writer.IngestPreview{}, "", nil, err
	}
	return preview, base, current, nil
}

func remoteHeadCommit(ctx context.Context, client *kcclient.Client, repository, ref string, options kcclient.RequestOptions) (kernel.CommitID, error) {
	var raw any
	if err := client.WriterService().Head(ctx, repository, ref, options, &raw); err != nil {
		return "", err
	}
	row, _ := raw.(map[string]any)
	if row == nil {
		encoded, err := json.Marshal(raw)
		if err != nil {
			return "", err
		}
		if err := json.Unmarshal(encoded, &row); err != nil || row == nil {
			return "", fmt.Errorf("writer head did not return a commit")
		}
	}
	commit, _ := row["commit"].(string)
	if commit == "" {
		return "", fmt.Errorf("writer head did not return a commit")
	}
	return kernel.CommitID(commit), nil
}

func remoteReceipt(ctx context.Context, client *kcclient.Client, commandID string, options kcclient.RequestOptions) (writer.CommitReceipt, bool, error) {
	var raw any
	if err := client.WriterService().Receipt(ctx, commandID, options, &raw); err != nil {
		return writer.CommitReceipt{}, false, nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return writer.CommitReceipt{}, false, err
	}
	var entry struct {
		Receipt writer.CommitReceipt `json:"receipt"`
	}
	if err := json.Unmarshal(encoded, &entry); err != nil {
		return writer.CommitReceipt{}, false, err
	}
	if entry.Receipt.CommandID == "" {
		var receipt writer.CommitReceipt
		if err := json.Unmarshal(encoded, &receipt); err != nil || receipt.CommandID == "" {
			return writer.CommitReceipt{}, false, nil
		}
		return receipt, true, nil
	}
	return entry.Receipt, true, nil
}
