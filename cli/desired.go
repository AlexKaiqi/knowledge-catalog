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
// one HTTP round trip per address before the commit POST even started. It
// matches the server's own basis fan-out so neither side is the narrow end.
const diffPreflightConcurrency = 32

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
				err := client.Objects().Resolve(ctx, kcclient.ObjectResolveRequest{
					ObjectScope: kcclient.ObjectScope{Repository: repository, Commit: string(commit)},
					Object:      string(op.Address.ObjectID),
					Aspect:      op.Address.AspectName,
					Member:      op.Address.MemberKey,
				}, options, &raw)
				if err != nil {
					if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
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
	empty := false
	if base == "" {
		var err error
		base, empty, err = remoteHeadBasis(ctx, client, repository, ref, options)
		if err != nil {
			return writer.IngestPreview{}, "", nil, err
		}
	}
	preview, err := inv.IngestDesired(flags, dir, repository, ref, base)
	if err != nil {
		return writer.IngestPreview{}, "", nil, err
	}
	if empty {
		return preview, base, map[string]string{}, nil
	}
	current, err := remoteCurrentDigests(ctx, client, repository, base, preview.ChangeSet.Operations, options)
	if err != nil {
		return writer.IngestPreview{}, "", nil, err
	}
	return preview, base, current, nil
}

func remoteHeadBasis(ctx context.Context, client *kcclient.Client, repository, ref string, options kcclient.RequestOptions) (kernel.CommitID, bool, error) {
	var raw any
	if err := client.WriterService().Head(ctx, repository, ref, options, &raw); err != nil {
		return "", false, err
	}
	row, _ := raw.(map[string]any)
	if row == nil {
		encoded, err := json.Marshal(raw)
		if err != nil {
			return "", false, err
		}
		if err := json.Unmarshal(encoded, &row); err != nil || row == nil {
			return "", false, fmt.Errorf("writer head did not return a commit")
		}
	}
	commit, _ := row["commit"].(string)
	if commit == "" {
		return "", false, fmt.Errorf("writer head did not return a commit")
	}
	empty := false
	if value, ok := row["empty"]; ok {
		var valid bool
		empty, valid = value.(bool)
		if !valid {
			return "", false, fmt.Errorf("writer head returned an invalid empty state")
		}
	}
	return kernel.CommitID(commit), empty, nil
}
