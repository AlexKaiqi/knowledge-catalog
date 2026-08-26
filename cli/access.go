package cli

import (
	"encoding/json"
	"strings"

	"kc/internal/journal"
	"kc/kernel"
	"kc/knowledge"
	"kc/observability"
	"kc/snapshot"
)

const (
	resolvedPinFlag    = "_resolved-pin-id"
	resolvedCommitFlag = "_resolved-commit"
)

type observedAccessResult struct {
	Output    any
	Knowledge []observability.KnowledgeAccess
}

func withKnowledgeEvidence(output, accessed any) observedAccessResult {
	return observedAccessResult{Output: output, Knowledge: knowledgeAccesses(accessed)}
}

func accessOutput(result any) any {
	if observed, ok := result.(observedAccessResult); ok {
		return observed.Output
	}
	return result
}

func identityContextFrom(flags map[string]FlagValue) (observability.IdentityContext, error) {
	assertion := observability.IdentityAssertion{
		Principal:  principalOf(flags),
		OnBehalfOf: strings.TrimSpace(FlagString(flags, "on-behalf-of")),
	}
	return (observability.PassThroughAuthenticator{}).Authenticate(assertion)
}

func traceContextFrom(flags map[string]FlagValue) (observability.TraceContext, error) {
	ctx := observability.TraceContext{
		TraceID:      strings.TrimSpace(FlagString(flags, "trace-id")),
		SpanID:       strings.TrimSpace(FlagString(flags, "span-id")),
		ParentSpanID: strings.TrimSpace(FlagString(flags, "parent-span-id")),
	}
	return ctx, ctx.Validate()
}

func knowledgeAccessCommand(command string, flags map[string]FlagValue) bool {
	if readingCatalog(command, flags) {
		return false
	}
	switch command {
	case "resolve", "resolve-binding", "read", "list", "relations", "search", "provenance", "describe-schema", "describe-access", "log", "diff",
		"checkout", "inspect", "vfs-read", "vfs-list":
		return true
	default:
		return false
	}
}

func recordKnowledgeAccess(home, command string, flags map[string]FlagValue, result any, callErr error) (string, error) {
	if !knowledgeAccessCommand(command, flags) {
		return "", nil
	}
	identity, err := identityContextFrom(flags)
	if err != nil {
		return "", err
	}
	trace, err := traceContextFrom(flags)
	if err != nil {
		return "", err
	}
	requestID, err := requestIDFrom(flags)
	if err != nil {
		return "", err
	}
	decision, outcome := "ALLOW", "RESOLVED"
	var fault map[string]any
	if callErr != nil {
		outcome = "ERROR"
		fault = journal.ErrorOf(callErr)
		if kernel.CodeOf(callErr) == kernel.ErrForbidden {
			decision = "DENY"
		}
	}
	event := observability.AccessEvent{
		Identity:  identity,
		Trace:     trace,
		Action:    command,
		RequestID: requestID,
		Workspace: workspaceIDOf(flags),
		PinID:     FlagString(flags, resolvedPinFlag),
		Decision:  decision,
		RuleID:    matchedRuleID(home, consumerAllowCmd(command, flags), flags),
		Result:    outcome,
		Knowledge: knowledgeAccesses(result),
		Files:     fileAccesses(result),
		Error:     fault,
	}
	if command == "checkout" {
		event.Snapshots = snapshotAccesses(accessOutput(result))
	}
	if len(event.Knowledge) == 0 {
		if target, ok := requestedKnowledge(flags); ok {
			event.Knowledge = append(event.Knowledge, target)
		}
	}
	return observability.NewFileStore(home).RecordAccessReceipt(event)
}

func requestedKnowledge(flags map[string]FlagValue) (observability.KnowledgeAccess, bool) {
	repo := FlagString(flags, "repo")
	commit := FlagString(flags, resolvedCommitFlag)
	object := FlagString(flags, "object")
	if repo == "" || commit == "" || object == "" {
		return observability.KnowledgeAccess{}, false
	}
	ref := knowledge.PinnedKnowledgeRef{
		KnowledgeRef: knowledge.KnowledgeRef{Repository: kernel.RepositoryID(repo), Object: knowledge.ObjectID(object)},
		Commit:       kernel.CommitID(commit),
	}
	var address *knowledge.Address
	if parsed, err := addressFrom(flags); err == nil {
		address = &parsed
	}
	return observability.KnowledgeAccess{KnowledgeRef: ref, Address: address}, true
}

func knowledgeAccesses(result any) []observability.KnowledgeAccess {
	if observed, ok := result.(observedAccessResult); ok {
		return observed.Knowledge
	}
	root := jsonValue(result)
	out := []observability.KnowledgeAccess{}
	seen := map[string]bool{}
	var walk func(any)
	walk = func(value any) {
		switch current := value.(type) {
		case []any:
			for _, child := range current {
				walk(child)
			}
		case map[string]any:
			repo := stringValue(current["repository"])
			commit := stringValue(current["commit"])
			object := stringValue(current["objectId"])
			if ref, ok := current["knowledgeRef"].(map[string]any); ok {
				if candidate := stringValue(ref["repository"]); candidate != "" {
					repo = candidate
				}
				if candidate := stringValue(ref["commit"]); candidate != "" {
					commit = candidate
				}
				object = stringValue(ref["object"])
			}
			_, addressOnly := current["kind"]
			if repo != "" && commit != "" && object != "" && !addressOnly {
				var address *knowledge.Address
				if raw, ok := current["address"].(map[string]any); ok {
					var parsed knowledge.Address
					if decodeMap(raw, &parsed) == nil && parsed.ObjectID != "" {
						address = &parsed
					}
				}
				key := repo + "\x00" + commit + "\x00" + object
				if address != nil {
					key += "\x00" + knowledge.AddressKey(*address)
				}
				if !seen[key] {
					seen[key] = true
					out = append(out, observability.KnowledgeAccess{
						KnowledgeRef: knowledge.PinnedKnowledgeRef{
							KnowledgeRef: knowledge.KnowledgeRef{Repository: kernel.RepositoryID(repo), Object: knowledge.ObjectID(object)},
							Commit:       kernel.CommitID(commit),
						},
						Address: address,
					})
				}
			}
			// Only response-envelope fields can contain additional knowledge
			// values. "value" and provenance are opaque knowledge payloads.
			for _, name := range []string{"schemas", "from", "to", "hits", "knowledge"} {
				if child, ok := current[name]; ok {
					walk(child)
				}
			}
		}
	}
	walk(root)
	return out
}

func fileAccesses(result any) []observability.FileAccess {
	root := jsonValue(result)
	out := []observability.FileAccess{}
	seen := map[string]bool{}
	var walk func(any)
	walk = func(value any) {
		switch current := value.(type) {
		case []any:
			for _, child := range current {
				walk(child)
			}
		case map[string]any:
			repo, commit, path := stringValue(current["repository"]), stringValue(current["commit"]), stringValue(current["path"])
			if repo != "" && commit != "" && path != "" && stringValue(current["objectId"]) == "" && stringValue(current["selector"]) == "" {
				key := repo + "\x00" + commit + "\x00" + path
				if !seen[key] {
					seen[key] = true
					out = append(out, observability.FileAccess{FileRef: snapshot.FileRef{
						Repository: kernel.RepositoryID(repo), Commit: kernel.CommitID(commit), Path: path,
					}})
				}
			}
			if child, ok := current["entries"]; ok {
				walk(child)
			}
		}
	}
	walk(root)
	return out
}

func snapshotAccesses(result any) []observability.SnapshotAccess {
	root := jsonValue(result)
	out := []observability.SnapshotAccess{}
	seen := map[string]bool{}
	add := func(repo, commit string) {
		key := repo + "\x00" + commit
		if repo == "" || commit == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, observability.SnapshotAccess{Repository: kernel.RepositoryID(repo), Commit: kernel.CommitID(commit)})
	}
	var walk func(any)
	walk = func(value any) {
		switch current := value.(type) {
		case []any:
			for _, child := range current {
				walk(child)
			}
		case map[string]any:
			add(stringValue(current["repository"]), stringValue(current["commit"]))
			if repositories, ok := current["repositories"].(map[string]any); ok {
				for repo, commit := range repositories {
					add(repo, stringValue(commit))
				}
			}
			for _, child := range current {
				walk(child)
			}
		}
	}
	walk(root)
	return out
}

func jsonValue(value any) any {
	if value == nil {
		return nil
	}
	body, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var out any
	if json.Unmarshal(body, &out) != nil {
		return nil
	}
	return out
}

func decodeMap(value map[string]any, dest any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dest)
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
