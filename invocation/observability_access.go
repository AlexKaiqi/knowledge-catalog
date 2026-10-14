package invocation

import (
	"encoding/json"
	interfaces "kc/liveaccess/handler"
	"strings"

	"kc/internal/journal"
	"kc/invoke"
	"kc/kernel"
	readresults "kc/liveaccess/evidence"
	objectmodel "kc/object"
	"kc/observability"
	"kc/snapshot"
)

// Access evidence (.kc/access.jsonl). Authorization lives in allow.go.

const (
	ResolvedPinFlag    = "_resolved-pin-id"
	ResolvedCommitFlag = "_resolved-commit"
)

type ObservedAccessResult struct {
	Output          any
	Object          []observability.ObjectAccess
	RetrievalSource any
	Refine          *observability.RefineEvent
	LiveAccess      *readresults.Reference
}

func RecordRefineEvidence(home string, result any, accessEvidenceID, retrievalEvidenceID string) (string, int64, error) {
	observed, ok := result.(ObservedAccessResult)
	if !ok || observed.Refine == nil {
		return "", -1, nil
	}
	event := *observed.Refine
	event.AccessEvidenceID = accessEvidenceID
	event.RetrievalEvidenceID = retrievalEvidenceID
	store := observability.NewFileStore(home)
	id, err := store.RecordRefineReceipt(event)
	return id, store.LastAppendBytes(), err
}

func accessOutput(result any) any {
	if observed, ok := result.(ObservedAccessResult); ok {
		return observed.Output
	}
	return result
}

func IdentityContextFrom(flags map[string]invoke.FlagValue) (observability.IdentityContext, error) {
	assertion := observability.IdentityAssertion{
		Principal:  PrincipalOf(flags),
		OnBehalfOf: strings.TrimSpace(invoke.FlagString(flags, "on-behalf-of")),
	}
	return (observability.PassThroughAuthenticator{}).Authenticate(assertion)
}

func TraceContextFrom(flags map[string]invoke.FlagValue) (observability.TraceContext, error) {
	ctx := observability.TraceContext{
		TraceID:      strings.TrimSpace(invoke.FlagString(flags, "trace-id")),
		SpanID:       strings.TrimSpace(invoke.FlagString(flags, "span-id")),
		ParentSpanID: strings.TrimSpace(invoke.FlagString(flags, "parent-span-id")),
	}
	return ctx, ctx.Validate()
}

func objectAccessCommand(command string, flags map[string]invoke.FlagValue) bool {
	switch command {
	case "pin", "object-resolve", "object-binding-show", "object-read", "object-access",
		"object-relations", "object-traverse", "object-search", "rerank", "search-rerank", "object-provenance",
		"object-schema-describe", "operations-access-spec-describe", "object-log", "object-invoke":
		return true
	default:
		return false
	}
}

func RecordObjectAccess(home, command string, flags map[string]invoke.FlagValue, result any, callErr error) (string, int64, error) {
	if !objectAccessCommand(command, flags) {
		return "", -1, nil
	}
	identity, err := IdentityContextFrom(flags)
	if err != nil {
		return "", -1, err
	}
	trace, err := TraceContextFrom(flags)
	if err != nil {
		return "", -1, err
	}
	requestID, err := RequestIDFrom(flags)
	if err != nil {
		return "", -1, err
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
		Action:    ActionOf(command, flags),
		RequestID: requestID,
		Dataset:   invoke.DatasetOf(flags),
		PinID:     invoke.FlagString(flags, ResolvedPinFlag),
		Decision:  decision,
		RuleID:    MatchedRuleID(home, ActionOf(command, flags), flags),
		Result:    outcome,
		Object:    ObjectAccesses(result),
		Files:     FileAccesses(result),
		Error:     fault,
	}
	if observed, ok := result.(ObservedAccessResult); ok {
		event.LiveAccess = observed.LiveAccess
	}
	if command == "file-mounts" {
		event.Snapshots = SnapshotAccesses(accessOutput(result))
	}
	if len(event.Object) == 0 {
		if target, ok := requestedObject(flags); ok {
			event.Object = append(event.Object, target)
		}
	}
	store := observability.NewFileStore(home)
	id, err := store.RecordAccessReceipt(event)
	return id, store.LastAppendBytes(), err
}

func requestedObject(flags map[string]invoke.FlagValue) (observability.ObjectAccess, bool) {
	repo := invoke.FlagString(flags, "repo")
	commit := invoke.FlagString(flags, ResolvedCommitFlag)
	object := invoke.FlagString(flags, "object")
	if repo == "" || commit == "" || object == "" {
		return observability.ObjectAccess{}, false
	}
	ref := objectmodel.PinnedObjectRef{
		ObjectRef: objectmodel.ObjectRef{Repository: kernel.RepositoryID(repo), Object: objectmodel.ObjectID(object)},
		Commit:    kernel.CommitID(commit),
	}
	var address *objectmodel.Address
	if parsed, err := AddressFrom(flags); err == nil {
		address = &parsed
	}
	if implementation := invoke.FlagString(flags, "implementation"); implementation != "" {
		address = &objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: ref.Object, AspectName: interfaces.ObjectInterfacesAspect, MemberKey: implementation}
	}
	return observability.ObjectAccess{ObjectRef: ref, Address: address}, true
}

func ObjectAccesses(result any) []observability.ObjectAccess {
	if observed, ok := result.(ObservedAccessResult); ok {
		return observed.Object
	}
	root := JSONValue(result)
	out := []observability.ObjectAccess{}
	seen := map[string]bool{}
	var walk func(any)
	walk = func(value any) {
		switch current := value.(type) {
		case []any:
			for _, child := range current {
				walk(child)
			}
		case map[string]any:
			// A SEARCH hit keeps observation bases in ObjectVersion while
			// the accessed repository/object coordinates live in objectmodel.
			// Join those envelope fields before the ordinary value walker runs;
			// never inspect the opaque business payload itself.
			if rawObject, ok := current["object"].(map[string]any); ok {
				joined := make(map[string]any, len(rawObject)+1)
				for key, child := range rawObject {
					joined[key] = child
				}
				if version, ok := current["version"].(map[string]any); ok {
					if observations, ok := version["observations"]; ok {
						joined["observations"] = observations
					}
				}
				walk(joined)
				return
			}
			repo := StringValue(current["repository"])
			commit := StringValue(current["commit"])
			object := StringValue(current["objectId"])
			if ref, ok := current["objectRef"].(map[string]any); ok {
				if candidate := StringValue(ref["repository"]); candidate != "" {
					repo = candidate
				}
				if candidate := StringValue(ref["commit"]); candidate != "" {
					commit = candidate
				}
				object = StringValue(ref["object"])
			}
			_, addressOnly := current["kind"]
			if repo != "" && commit != "" && object != "" && !addressOnly {
				var address *objectmodel.Address
				if raw, ok := current["address"].(map[string]any); ok {
					var parsed objectmodel.Address
					if decodeMap(raw, &parsed) == nil && parsed.ObjectID != "" {
						address = &parsed
					}
				}
				key := repo + "\x00" + commit + "\x00" + object
				if address != nil {
					key += "\x00" + objectmodel.AddressKey(*address)
				}
				if !seen[key] {
					seen[key] = true
					observations := []objectmodel.UnitObservation{}
					if rawObservations, ok := current["observations"].([]any); ok {
						for _, rawObservation := range rawObservations {
							item, ok := rawObservation.(map[string]any)
							if !ok {
								continue
							}
							var observation objectmodel.UnitObservation
							if decodeMap(item, &observation) == nil {
								observations = append(observations, observation)
							}
						}
					}
					out = append(out, observability.ObjectAccess{
						Reference: StringValue(current["reference"]), Locator: StringValue(current["locator"]),
						ObjectRef: objectmodel.PinnedObjectRef{
							ObjectRef: objectmodel.ObjectRef{Repository: kernel.RepositoryID(repo), Object: objectmodel.ObjectID(object)},
							Commit:    kernel.CommitID(commit),
						},
						Address: address, Observations: observations,
					})
				}
			}
			// Only response-envelope fields can contain additional object
			// values. "value" and provenance are opaque object payloads.
			for _, name := range []string{"schemas", "from", "to", "hits", "object"} {
				if child, ok := current[name]; ok {
					walk(child)
				}
			}
		}
	}
	walk(root)
	return out
}

func FileAccesses(result any) []observability.FileAccess {
	root := JSONValue(result)
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
			repo, commit, path := StringValue(current["repository"]), StringValue(current["commit"]), StringValue(current["path"])
			if repo != "" && commit != "" && path != "" && StringValue(current["objectId"]) == "" && StringValue(current["selector"]) == "" {
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

func SnapshotAccesses(result any) []observability.SnapshotAccess {
	root := JSONValue(result)
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
			add(StringValue(current["repository"]), StringValue(current["commit"]))
			if repositories, ok := current["repositories"].(map[string]any); ok {
				for repo, commit := range repositories {
					add(repo, StringValue(commit))
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

func JSONValue(value any) any {
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

func StringValue(value any) string {
	text, _ := value.(string)
	return text
}
