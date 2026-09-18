package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

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

func remoteCurrentDigests(ctx context.Context, client *kcclient.Client, repository string, commit kernel.CommitID, ops []knowledge.Operation, options kcclient.RequestOptions) (map[string]string, error) {
	out := map[string]string{}
	if commit == "" {
		return out, nil
	}
	for _, op := range ops {
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
			return nil, err
		}
		resolution, err := parseResolution(raw)
		if err != nil {
			return nil, err
		}
		if resolution.Status == knowledge.StatusUnresolved || resolution.Digest == "" {
			continue
		}
		out[knowledge.AddressKey(op.Address)] = string(resolution.Digest)
	}
	return out, nil
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
