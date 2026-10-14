package cli

import (
	"context"
	"encoding/json"
	"fmt"
	inv "kc/invocation"
	"sync"
	"sync/atomic"

	kcclient "kc/client"
	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/writer"
)

func parseResolution(raw any) (objectmodel.Resolution, error) {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return objectmodel.Resolution{}, err
	}
	var resolution objectmodel.Resolution
	if err := json.Unmarshal(encoded, &resolution); err != nil {
		return objectmodel.Resolution{}, err
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
func remoteCurrentDigests(ctx context.Context, client *kcclient.Client, repository string, commit kernel.CommitID, ops []objectmodel.Operation, options kcclient.RequestOptions) (map[string]string, error) {
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
					KnowledgeScope: kcclient.KnowledgeScope{Repository: repository, Commit: string(commit)},
					Object:         string(op.Address.ObjectID),
					Aspect:         op.Address.AspectName,
					Member:         op.Address.MemberKey,
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
				if resolution.Status == objectmodel.StatusUnresolved || resolution.Digest == "" {
					continue
				}
				mu.Lock()
				out[objectmodel.AddressKey(op.Address)] = string(resolution.Digest)
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
	base := kernel.CommitID(invoke.FlagString(flags, "base"))
	if base == "" {
		var err error
		base, err = remoteHeadCommit(ctx, client, repository, ref, options)
		if err != nil {
			return writer.IngestPreview{}, "", nil, err
		}
	}
	preview, err := inv.IngestDesired(flags, dir, repository, ref, base)
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
