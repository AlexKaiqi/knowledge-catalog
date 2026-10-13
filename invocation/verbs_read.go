package invocation

import (
	"context"
	"fmt"
	"os"
	"strings"

	"encoding/json"
	"kc/invoke"
	"kc/kernel"
	knowledge "kc/knowledge"
	reader "kc/knowledge/reader"
	knowledgeserving "kc/knowledge/serving"
	knowledgeapp "kc/knowledgeapp"
	"kc/retrieval"
	"kc/snapshot/dataset"
	"strconv"
)

type (
	workspaceRead  = func(serving *reader.Serving, cat dataset.Service) (any, error)
	repositoryRead = func(repositoryID kernel.RepositoryID, commitID kernel.CommitID) (any, error)
)

// VerbResolveBinding returns the stable declaration required by an upper
// Materialization runtime. It never invokes the runtime or returns live data.
func VerbResolveBinding(cx *Invocation) (any, error) {
	address, err := AddressFrom(cx.Flags)
	if err != nil {
		return nil, err
	}
	if address.AspectName == "" {
		return nil, fmt.Errorf("resolve-binding requires --aspect")
	}
	return OnTarget(cx,
		func(serving *reader.Serving, _ dataset.Service) (any, error) {
			return serving.ResolveBinding(address)
		},
		func(repositoryID kernel.RepositoryID, commitID kernel.CommitID) (any, error) {
			return cx.WS.Reader.ResolveBinding(repositoryID, commitID, address)
		})
}

// OnTarget dispatches a read to the Workspace pin or to one pinned Repository commit.
func OnTarget(cx *Invocation, onWorkspace workspaceRead, onRepository repositoryRead) (any, error) {
	if ServingWorkspace(cx.Flags) {
		objectScope := invoke.FlagString(cx.Flags, "object")
		if cx.Command == "knowledge-relations" {
			// Relation results have identities distinct from the requested
			// endpoint, so object-scoped endpoint access cannot prove complete
			// coverage of the returned relation objects.
			objectScope = ""
		}
		serving, cat, err := OpenCompleteServing(cx, objectScope)
		if err != nil {
			return nil, err
		}
		return onWorkspace(serving, cat)
	}
	repositoryID, commitID, err := PinCommit(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	return onRepository(repositoryID, commitID)
}

// VerbResolve reports the Workspace pin itself. Object RESOLVE belongs to
// `kc resolve`; Registry must not interpret object_id.
func VerbResolve(cx *Invocation) (any, error) {
	if cx.Flag("object") != "" || cx.Flag("aspect") != "" || cx.Flag("member") != "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid,
			"pin returns only a fixed dataset pin; use kc resolve for an object")
	}
	if ServingWorkspace(cx.Flags) {
		cat, err := PickDatasets(cx.WS, cx.Flags)
		if err != nil {
			return nil, err
		}
		setID, err := cx.SetID()
		if err != nil {
			return nil, err
		}
		resolved, err := ResolveOrReplay(cx.WS, cx.Home, cat, setID, cx.Flags)
		if err != nil {
			return nil, err
		}
		return ShapePinOutput(cx.Flags, resolved)
	}
	return ResolveTemporaryWorkspace(cx)
}

// VerbResolveObject is protocol RESOLVE: the object's status at this command's
// frozen basis. Missing objects are unresolved, not an empty READ.
func VerbResolveObject(cx *Invocation) (any, error) {
	if cx.Flag("object") == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "resolve requires --object")
	}
	return OnTarget(cx,
		func(serving *reader.Serving, _ dataset.Service) (any, error) {
			if UsesAddress(cx.Flags) {
				address, err := AddressFrom(cx.Flags)
				if err != nil {
					return nil, err
				}
				return serving.ResolveAddress(address)
			}
			objectID, err := cx.Require("object")
			if err != nil {
				return nil, err
			}
			return serving.Resolve(knowledge.ObjectID(objectID))
		},
		func(repositoryID kernel.RepositoryID, commitID kernel.CommitID) (any, error) {
			if UsesAddress(cx.Flags) {
				address, err := AddressFrom(cx.Flags)
				if err != nil {
					return nil, err
				}
				return cx.WS.Reader.ResolveAddress(repositoryID, address, commitID)
			}
			ref, err := cx.KnowledgeRef(repositoryID)
			if err != nil {
				return nil, err
			}
			return cx.WS.Reader.Resolve(ref, commitID)
		})
}

func ResolveTemporaryWorkspace(cx *Invocation) (any, error) {
	if cx.Flag("source") == "" && cx.Flag("file") == "" && cx.Flag("from-repo") == "" && cx.Flag("payload") == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "kc pin requires --dataset <id> or --source <repository>[=selector]...")
	}
	sources, recipe, _, err := WorkspaceSources(cx)
	if err != nil {
		return nil, err
	}
	metadata, err := DatasetMetadata(cx.Flags, recipe.Metadata)
	if err != nil {
		return nil, err
	}
	cat, err := PickDatasets(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	revision := 1
	if raw := cx.Flag("revision"); raw != "" {
		revision, err = strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("--revision must be a number")
		}
	}
	resolved, err := cat.ResolveDefinition(dataset.KnowledgeSet{Revision: revision, Sources: sources, Metadata: metadata})
	if err != nil {
		return nil, err
	}
	return ShapePinOutput(cx.Flags, resolved)
}

func ShapePinOutput(flags map[string]invoke.FlagValue, result any) (any, error) {
	out := strings.TrimSpace(invoke.FlagString(flags, "out"))
	if out == "" {
		return result, nil
	}
	body, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(out, append(body, '\n'), 0o644); err != nil {
		return nil, err
	}
	summary := map[string]any{"out": out}
	row, _ := JSONValue(result).(map[string]any)
	if row == nil {
		if encoded, err := json.Marshal(result); err == nil {
			_ = json.Unmarshal(encoded, &row)
		}
	}
	if row != nil {
		if v := row["setId"]; v != nil && fmt.Sprint(v) != "" {
			summary["setId"] = v
		}
		if v := row["pinId"]; v != nil && fmt.Sprint(v) != "" {
			summary["pinId"] = v
		}
	}
	return summary, nil
}

func VerbRead(cx *Invocation) (any, error) {
	if ServingWorkspace(cx.Flags) {
		object, err := cx.Require("object")
		if err != nil {
			return nil, err
		}
		request := knowledgeapp.DatasetReadRequest{Object: knowledge.ObjectID(object), Selector: AspectSelectorFrom(cx.Flags)}
		if UsesAddress(cx.Flags) {
			address, err := AddressFrom(cx.Flags)
			if err != nil {
				return nil, err
			}
			request.Address = &address
		}
		return (knowledgeapp.DatasetReadExecutor{
			Authorize: func(context.Context) error { return Authorize(cx.Home, "knowledge.read", cx.Flags, nil, cx.WS) },
			Resolve: func(context.Context) (*knowledgeserving.Service, error) {
				serving, _, err := OpenCompleteServing(cx, object)
				if err != nil {
					return nil, err
				}
				return LogicalWorkspaceServing(cx, serving)
			},
			Deliver: func(_ context.Context, values []knowledgeserving.ReadResult) ([]knowledgeserving.ReadResult, error) {
				return FilterKnowledgeServingReads(cx.Home, cx.Flags, nil, values), nil
			},
		}).Execute(cx.Context, request)
	}
	return OnTarget(cx, nil,
		func(repositoryID kernel.RepositoryID, commitID kernel.CommitID) (any, error) {
			objectID, err := cx.Require("object")
			if err != nil {
				return nil, err
			}
			request := knowledgeapp.ReadRequest{
				Repository: repositoryID, Commit: commitID,
				Object: knowledge.ObjectID(objectID),
			}
			if UsesAddress(cx.Flags) {
				address, err := AddressFrom(cx.Flags)
				if err != nil {
					return nil, err
				}
				request.Address = &address
			}
			request.Selector = AspectSelectorFrom(cx.Flags)
			return (knowledgeapp.ReadExecutor{Reader: cx.WS.Reader}).Execute(cx.Context, request)
		})
}

// VerbProvenance returns the origin envelopes stamped on each unit. It does not
// crawl sourceRefs and it is not git log.
func VerbProvenance(cx *Invocation) (any, error) {
	if UsesAddress(cx.Flags) {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "provenance is object-level; do not pass --aspect or --member")
	}
	return OnTarget(cx,
		func(serving *reader.Serving, _ dataset.Service) (any, error) {
			objectID, err := cx.Require("object")
			if err != nil {
				return nil, err
			}
			traces, err := serving.GetProvenance(knowledge.ObjectID(objectID))
			if err != nil {
				return nil, err
			}
			out := []knowledge.ProvenanceTrace{}
			for _, trace := range traces {
				if AllowedRepoRead(cx.Home, cx.Flags, string(trace.Repository), string(trace.ObjectID)) {
					out = append(out, trace)
				}
			}
			return out, nil
		},
		func(repositoryID kernel.RepositoryID, commitID kernel.CommitID) (any, error) {
			ref, err := cx.KnowledgeRef(repositoryID)
			if err != nil {
				return nil, err
			}
			return cx.WS.Reader.GetProvenance(ref, commitID)
		})
}

// VerbRelations returns one-hop Canonical Relations touching --object. It does
// not recursively traverse a graph; every hit is read at this command's pin.
func VerbRelations(cx *Invocation) (any, error) {
	object, err := cx.Require("object")
	if err != nil {
		return nil, err
	}
	limit, err := PageLimit(cx.Flags, 100, 1000)
	if err != nil {
		return nil, err
	}
	direction := knowledge.RelationDirection(strings.ToUpper(cx.Flag("direction")))
	if direction != "" && direction != knowledge.RelationDirected && direction != knowledge.RelationUndirected {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "--direction must be DIRECTED or UNDIRECTED")
	}
	if ServingWorkspace(cx.Flags) {
		serving, _, err := OpenCompleteServing(cx, "")
		if err != nil {
			return nil, err
		}
		endpoint, err := WorkspaceRelationEndpoint(object, serving.Pin())
		if err != nil {
			return nil, err
		}
		if !AllowedRepoRead(cx.Home, cx.Flags, string(endpoint.Repository), string(endpoint.Object)) {
			return nil, kernel.Fail(kernel.ErrForbidden, "relation endpoint repository is not authorized")
		}
		return (knowledgeapp.DatasetRelationsExecutor{Serving: serving, Repositories: cx.WS.Reader, Projection: cx.WS.Index}).Execute(cx.Context, RelationPageRequest(endpoint, cx, limit, direction))
	}
	if strings.HasPrefix(object, "kc://") {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "repository relations requires a bare --object ObjectID")
	}
	repositoryID, commitID, err := PinCommit(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	repo, err := cx.WS.Reader.Require(repositoryID, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		return nil, err
	}
	endpoint := knowledge.KnowledgeRef{Repository: repositoryID, Object: knowledge.ObjectID(object)}
	return cx.WS.Index.RelationsAtContext(cx.Context, repo, commitID, RelationPageRequest(endpoint, cx, limit, direction))
}

func RelationPageRequest(endpoint knowledge.KnowledgeRef, cx *Invocation, limit int, direction knowledge.RelationDirection) retrieval.RelationPageRequest {
	return retrieval.RelationPageRequest{
		Query: retrieval.RelationQuery{
			Endpoint: endpoint, RelationType: cx.Flag("relation-type"), Role: cx.Flag("role"), Direction: direction,
		},
		Limit: limit, Continuation: cx.Flag("continuation"),
	}
}

// VerbTraverse runs the bounded neighborhood closure contract: the scope is
// fixed before execution (dataset manifest pin or single repository commit),
// every hop reuses the one-hop relation lane, out-of-scope frontiers stop
// with explicit boundaries, and pages are deltas of the closure. It never
// enumerates paths, never promises shortest paths, and never widens scope.
func VerbTraverse(cx *Invocation) (any, error) {
	object, err := cx.Require("object")
	if err != nil {
		return nil, err
	}
	maxHops, err := TraverseHops(cx.Flags)
	if err != nil {
		return nil, err
	}
	minHops := 0
	if raw := invoke.FlagString(cx.Flags, "min-hops"); raw != "" {
		minHops, err = strconv.Atoi(raw)
		if err != nil || minHops < 0 {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "--min-hops must be a non-negative number")
		}
	}
	limit, err := PageLimit(cx.Flags, retrieval.DefaultTraversePageLimit, retrieval.MaxTraversePageLimit)
	if err != nil {
		return nil, err
	}
	direction := knowledge.RelationDirection(strings.ToUpper(cx.Flag("direction")))
	if direction != "" && direction != knowledge.RelationDirected && direction != knowledge.RelationUndirected {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "--direction must be DIRECTED or UNDIRECTED")
	}
	step := retrieval.TraverseStep{RelationType: cx.Flag("relation-type"), Role: cx.Flag("role"), Direction: direction}
	request := retrieval.TraverseRequest{
		Query: retrieval.TraverseQuery{Step: step, MinHops: minHops, MaxHops: maxHops},
		Limit: limit, Continuation: cx.Flag("continuation"),
	}
	if ServingWorkspace(cx.Flags) {
		serving, _, err := OpenCompleteServing(cx, "")
		if err != nil {
			return nil, err
		}
		endpoint, err := WorkspaceRelationEndpoint(object, serving.Pin())
		if err != nil {
			return nil, err
		}
		if !AllowedRepoRead(cx.Home, cx.Flags, string(endpoint.Repository), string(endpoint.Object)) {
			return nil, kernel.Fail(kernel.ErrForbidden, "traverse start repository is not authorized")
		}
		request.Query.Start = endpoint
		return (knowledgeapp.DatasetTraverseExecutor{Serving: serving, Repositories: cx.WS.Reader, Projection: cx.WS.Index}).Execute(cx.Context, request)
	}
	if strings.HasPrefix(object, "kc://") {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "repository traverse requires a bare --object ObjectID")
	}
	repositoryID, commitID, err := PinCommit(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	request.Query.Start = knowledge.KnowledgeRef{Repository: repositoryID, Object: knowledge.ObjectID(object)}
	return (knowledgeapp.RepoTraverseExecutor{Repositories: cx.WS.Reader, Projection: cx.WS.Index}).Execute(cx.Context, request, repositoryID, commitID)
}

func WorkspaceRelationEndpoint(raw string, pin reader.KnowledgeSetPin) (knowledge.KnowledgeRef, error) {
	if !strings.HasPrefix(raw, "kc://") {
		return knowledge.KnowledgeRef{}, kernel.Fail(kernel.ErrUsageInvalid,
			"workspace relations requires an unpinned kc:// repository/object reference")
	}
	var match knowledge.KnowledgeRef
	matchedPrefix := 0
	for repository := range pin.Repositories {
		prefix := knowledge.FormatKnowledgeRef(repository, "")
		if len(prefix) > matchedPrefix && strings.HasPrefix(raw, prefix) && len(raw) > len(prefix) {
			match = knowledge.KnowledgeRef{Repository: repository, Object: knowledge.ObjectID(raw[len(prefix):])}
			matchedPrefix = len(prefix)
		}
	}
	if match.Repository == "" {
		return knowledge.KnowledgeRef{}, kernel.Fail(kernel.ErrUsageInvalid,
			"relation endpoint repository is not a member of the resolved Workspace")
	}
	return match, nil
}

// VerbDescribeSchema reports the AccessHints a schema declares, which is what
// decides the logical retrieval surface compiled into AccessSpec.
func VerbDescribeSchema(cx *Invocation) (any, error) {
	objectID := knowledge.ObjectID(cx.Flag("object"))
	return OnTarget(cx,
		func(serving *reader.Serving, _ dataset.Service) (any, error) {
			reports, err := serving.DescribeSchema(objectID)
			if err != nil {
				return nil, err
			}
			out := []reader.SchemaReport{}
			for _, report := range reports {
				if AllowedRepoRead(cx.Home, cx.Flags, string(report.Repository), string(objectID)) {
					out = append(out, report)
				}
			}
			return out, nil
		},
		func(repositoryID kernel.RepositoryID, commitID kernel.CommitID) (any, error) {
			return cx.WS.Reader.DescribeSchema(repositoryID, commitID, objectID)
		})
}

// VerbLog lists the commits that introduced each digest of one object. Registry
// history is `kc registry audit`; current combination space is `kc show`.
func VerbLog(cx *Invocation) (any, error) {
	if cx.Flag("object") == "" && cx.Flag("repo") == "" {
	}
	if cx.Flag("aspect") != "" || cx.Flag("member") != "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "log is object history; do not pass --aspect or --member")
	}
	limit, err := PageLimit(cx.Flags, DefaultHistoryLimit, MaxHistoryPageSize)
	if err != nil {
		return nil, err
	}
	return OnTarget(cx,
		func(serving *reader.Serving, _ dataset.Service) (any, error) {
			objectID, err := cx.Require("object")
			if err != nil {
				return nil, err
			}
			id := knowledge.ObjectID(objectID)
			return CollectObjectLogPage(cx.Home, cx.Flags, id, serving.Pin().Repositories, limit, cx.Flag("continuation"),
				func(repositoryID kernel.RepositoryID, commitID kernel.CommitID, query knowledge.ObjectLogQuery) ([]knowledge.ObjectRevision, error) {
					allowed, err := serving.Contains(repositoryID, id)
					if err != nil || !allowed {
						return nil, err
					}
					repo, err := serving.Member(repositoryID)
					if err != nil {
						return nil, err
					}
					return repo.Log(id, commitID, query)
				})
		},
		func(repositoryID kernel.RepositoryID, commitID kernel.CommitID) (any, error) {
			objectID, err := cx.Require("object")
			if err != nil {
				return nil, err
			}
			id := knowledge.ObjectID(objectID)
			pin := map[kernel.RepositoryID]kernel.CommitID{repositoryID: commitID}
			return CollectObjectLogPage(cx.Home, cx.Flags, id, pin, limit, cx.Flag("continuation"),
				func(member kernel.RepositoryID, commit kernel.CommitID, query knowledge.ObjectLogQuery) ([]knowledge.ObjectRevision, error) {
					return cx.WS.Reader.Log(member, id, commit, query)
				})
		})
}

// loadJSONFlag reads a value body from --file or --value/--payload, never both.
func LoadJSONFlag(flags map[string]invoke.FlagValue, label string) (any, bool, error) {
	file := invoke.FlagString(flags, "file")
	raw := invoke.FlagString(flags, "value")
	if raw == "" {
		raw = invoke.FlagString(flags, "payload")
	}
	if file != "" && raw != "" {
		return nil, false, fmt.Errorf("use only one of --file or --value/--payload")
	}
	if file != "" {
		body, err := os.ReadFile(file)
		if err != nil {
			return nil, false, err
		}
		value, err := ParseJSON(string(body), file)
		return value, true, err
	}
	if raw != "" {
		value, err := ParseJSON(raw, label)
		return value, true, err
	}
	return nil, false, nil
}

func SetTelemetryChangeCounts(observation *invoke.ObservationTelemetry, operations []knowledge.Operation) {
	observation = invoke.NoObservationTelemetry(observation)
	puts, removes := 0, 0
	for _, operation := range operations {
		switch operation.Op {
		case knowledge.OpPut:
			puts++
		case knowledge.OpRemove:
			removes++
		}
	}
	observation.PutCount = puts
	observation.RemoveCount = removes
	if payload, err := json.Marshal(operations); err == nil {
		observation.WriterPayloadBytes = len(payload)
	} else {
		observation.WriterPayloadBytes = -1
	}
	observation.WriterCountsSet = true
}

// openCompleteServing is for consumer operations whose public response has no
// coverage envelope (READ/RESOLVE/RELATIONS/LOG/PROVENANCE and similar exact reads). Those
// operations must not silently turn an authorization gap into an empty or
// apparently complete result. SEARCH keeps every pin member in the candidate
// set and applies the delivery chain after hydrate.
func OpenCompleteServing(cx *Invocation, object string) (*reader.Serving, dataset.Service, error) {
	serving, cat, err := OpenServingAt(cx, cx.WS, cx.Flags)
	if err != nil {
		return nil, nil, err
	}
	if err := RequireCompleteWorkspaceRead(cx.WS.Dir, cx.Flags, serving.Pin(), object); err != nil {
		return nil, nil, err
	}
	return serving, cat, nil
}

func AspectSelectorFrom(flags map[string]invoke.FlagValue) *knowledge.AspectSelector {
	include := invoke.FlagStrings(flags, "include")
	exclude := invoke.FlagStrings(flags, "exclude")
	if len(include) == 0 && len(exclude) == 0 {
		return nil
	}
	return &knowledge.AspectSelector{Include: include, Exclude: exclude}
}

func FilterKnowledgeServingReads(home string, flags map[string]invoke.FlagValue, _ dataset.Service, values []knowledgeserving.ReadResult) []knowledgeserving.ReadResult {
	out := []knowledgeserving.ReadResult{}
	for _, item := range values {
		if AllowedRepoRead(home, flags, string(item.Repository), string(item.ObjectID)) {
			out = append(out, item)
		}
	}
	return out
}

func CollectObjectLogPage(
	home string,
	flags map[string]invoke.FlagValue,
	objectID knowledge.ObjectID,
	pin map[kernel.RepositoryID]kernel.CommitID,
	limit int,
	continuation string,
	readLog func(kernel.RepositoryID, kernel.CommitID, knowledge.ObjectLogQuery) ([]knowledge.ObjectRevision, error),
) (objectLogPage, error) {
	basis := objectLogBasis(pin)
	after, err := decodeObjectLogCursor(continuation, objectID, basis)
	if err != nil {
		return objectLogPage{}, err
	}
	logs := []reader.ObjectLog{}
	nextAfter := map[string]string{}
	for _, repositoryID := range pinnedRepositoryIDs(pin) {
		if !AllowedRepoRead(home, flags, string(repositoryID), string(objectID)) {
			continue
		}
		repoKey := string(repositoryID)
		if after != nil {
			if _, open := after[repoKey]; !open {
				continue
			}
		}
		query := knowledge.ObjectLogQuery{Limit: limit + 1}
		if after != nil {
			query.After = kernel.CommitID(after[repoKey])
		}
		revisions, logErr := readLog(repositoryID, pin[repositoryID], query)
		if logErr != nil {
			if kernel.CodeOf(logErr) == kernel.ErrKnowledgeRefUnresolved {
				continue
			}
			return objectLogPage{}, logErr
		}
		if len(revisions) == 0 {
			continue
		}
		more := len(revisions) > limit
		if more {
			revisions = revisions[:limit]
			nextAfter[repoKey] = string(revisions[len(revisions)-1].Commit)
		}
		logs = append(logs, reader.ObjectLog{
			Repository: repositoryID,
			ObjectID:   objectID,
			Commit:     pin[repositoryID],
			Revisions:  revisions,
		})
	}
	page := objectLogPage{Logs: logs}
	if len(nextAfter) > 0 {
		page.Continuation = encodeObjectLogCursor(objectID, basis, nextAfter)
	}
	return page, nil
}

func ParseJSON(text, label string) (any, error) {
	var value any
	if err := kernel.UnmarshalJSON([]byte(text), &value); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON", label)
	}
	return value, nil
}
