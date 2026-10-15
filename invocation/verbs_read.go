package invocation

import (
	"context"
	"fmt"
	discoveryservice "kc/discovery/service"
	objectservice "kc/object/service"
	"os"
	"strings"

	"encoding/json"
	"kc/invoke"
	"kc/kernel"
	"kc/liveaccess"
	objectmodel "kc/object"
	reader "kc/object/reader"

	retrieval "kc/discovery"
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
	address, err := bindingAddress(cx)
	if err != nil {
		return nil, err
	}
	return OnTarget(cx,
		func(serving *reader.Serving, _ dataset.Service) (any, error) {
			return liveaccess.ResolveBinding(serving, address)
		},
		func(repositoryID kernel.RepositoryID, commitID kernel.CommitID) (any, error) {
			repo, err := cx.WS.Reader.Require(repositoryID, kernel.ErrObjectRefUnresolved)
			if err != nil {
				return nil, err
			}
			return liveaccess.ResolveRepoBinding(repo, commitID, address, requestDescriptors(cx, repositoryID, commitID))
		})
}

func bindingAddress(cx *Invocation) (objectmodel.Address, error) {
	address, err := AddressFrom(cx.Flags)
	if err != nil {
		return objectmodel.Address{}, err
	}
	if address.AspectName == "" {
		return objectmodel.Address{}, fmt.Errorf("resolve-binding requires --aspect")
	}
	return address, nil
}

func repositoryServingAt(cx *Invocation, repository kernel.RepositoryID, commit kernel.CommitID, setID string) *reader.Serving {
	repositories := map[kernel.RepositoryID]kernel.CommitID{repository: commit}
	return reader.Open(func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return cx.WS.Reader.Require(id, kernel.ErrObjectRefUnresolved)
	}, reader.DatasetPin{SetID: setID, Repositories: repositories, Items: reader.WholeRepositoryItems(repositories)})
}

// OnTarget dispatches a read to the Workspace pin or to one pinned Repository commit.
func OnTarget(cx *Invocation, onWorkspace workspaceRead, onRepository repositoryRead) (any, error) {
	if ServingWorkspace(cx.Flags) {
		objectScope := invoke.FlagString(cx.Flags, "object")
		if cx.Command == "object-relations" {
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
			return serving.Resolve(objectmodel.ObjectID(objectID))
		},
		func(repositoryID kernel.RepositoryID, commitID kernel.CommitID) (any, error) {
			if UsesAddress(cx.Flags) {
				address, err := AddressFrom(cx.Flags)
				if err != nil {
					return nil, err
				}
				return cx.WS.Reader.ResolveAddress(repositoryID, address, commitID)
			}
			ref, err := cx.ObjectRef(repositoryID)
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
	resolved, err := cat.ResolveDefinition(dataset.Dataset{Revision: revision, Sources: sources, Metadata: metadata})
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
	reference, err := referenceFromFlags(cx.Flags)
	if err != nil {
		return nil, err
	}
	if reference == nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "read requires an object URN")
	}
	if err := NormalizeReferenceFlags(cx.Flags); err != nil {
		return nil, err
	}
	if ServingWorkspace(cx.Flags) {
		values, err := (objectservice.DatasetReadExecutor{
			Authorize: func(context.Context) error { return Authorize(cx.Home, "object.read", cx.Flags, nil, cx.WS) },
			ReferenceRead: func(_ context.Context, ref objectmodel.Reference) ([]objectmodel.ObservedValue, error) {
				return readDatasetReference(cx, ref)
			},
			Deliver: func(_ context.Context, values []objectmodel.ObservedValue) ([]objectmodel.ObservedValue, error) {
				return FilterObjectServingReads(cx.Home, cx.Flags, nil, values), nil
			},
		}).Execute(cx.Context, objectservice.DatasetReadRequest{Reference: *reference})
		if err != nil {
			return nil, err
		}
		if len(values) != 1 {
			return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "Reference did not select one published source object")
		}
		return values[0], nil
	}
	selection, scoped := cx.Flags[referenceHoldingsFlag].(objectmodel.HoldingSelection)
	if !scoped {
		_, commitID, err := PinCommit(cx.WS, cx.Flags)
		if err != nil {
			return nil, err
		}
		selection = objectmodel.HoldingSelection{Selected: []objectmodel.Holding{{Repository: reference.Repository, Commit: commitID}}}
	}
	reads := make([]objectmodel.ObservedValue, 0, len(selection.Selected))
	for _, holding := range selection.Selected {
		member := *reference
		member.Repository = holding.Repository
		observed, err := readRepositoryObserved(cx, objectservice.ReadRequest{Reference: member, Commit: holding.Commit})
		if err != nil {
			return nil, err
		}
		if scoped {
			observed.Sources = []objectmodel.SourceBasis{{Repository: holding.Repository, Commit: holding.Commit, Aspects: holding.Aspects}}
		}
		reads = append(reads, observed)
	}
	observed := reads[0]
	if len(reads) > 1 {
		composed, err := composeObserved(*reference, reads)
		if err != nil {
			return nil, err
		}
		observed = composed
	}
	observed.Withheld = selection.Withheld
	if len(observed.Observations) == 0 && len(observed.Failures) == 0 {
		return observedObject(observed), nil
	}
	return observed, nil
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
			traces, err := serving.GetProvenance(objectmodel.ObjectID(objectID))
			if err != nil {
				return nil, err
			}
			out := []objectmodel.ProvenanceTrace{}
			for _, trace := range traces {
				if AllowedRepoRead(cx.Home, cx.Flags, string(trace.Repository), string(trace.ObjectID)) {
					out = append(out, trace)
				}
			}
			return out, nil
		},
		func(repositoryID kernel.RepositoryID, commitID kernel.CommitID) (any, error) {
			ref, err := cx.ObjectRef(repositoryID)
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
	direction := objectmodel.RelationDirection(strings.ToUpper(cx.Flag("direction")))
	if direction != "" && direction != objectmodel.RelationDirected && direction != objectmodel.RelationUndirected {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "--direction must be DIRECTED or UNDIRECTED")
	}
	endpoint, err := RelationEndpointFlag(object)
	if err != nil {
		return nil, err
	}
	if ServingWorkspace(cx.Flags) {
		serving, _, err := OpenCompleteServing(cx, string(endpoint))
		if err != nil {
			return nil, err
		}
		return (discoveryservice.DatasetRelationsExecutor{Serving: serving, Repositories: cx.WS.Reader, Projection: cx.WS.Index}).Execute(cx.Context, RelationPageRequest(endpoint, cx, limit, direction))
	}
	repositoryID, commitID, err := PinCommit(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	repo, err := cx.WS.Reader.Require(repositoryID, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		return nil, err
	}
	return cx.WS.Index.RelationsAtContext(cx.Context, repo, commitID, RelationPageRequest(endpoint, cx, limit, direction))
}

func RelationPageRequest(endpoint objectmodel.ObjectID, cx *Invocation, limit int, direction objectmodel.RelationDirection) retrieval.RelationPageRequest {
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
	direction := objectmodel.RelationDirection(strings.ToUpper(cx.Flag("direction")))
	if direction != "" && direction != objectmodel.RelationDirected && direction != objectmodel.RelationUndirected {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "--direction must be DIRECTED or UNDIRECTED")
	}
	step := retrieval.TraverseStep{RelationType: cx.Flag("relation-type"), Role: cx.Flag("role"), Direction: direction}
	request := retrieval.TraverseRequest{
		Query: retrieval.TraverseQuery{Step: step, MinHops: minHops, MaxHops: maxHops},
		Limit: limit, Continuation: cx.Flag("continuation"),
	}
	start, err := RelationEndpointFlag(object)
	if err != nil {
		return nil, err
	}
	request.Query.Start = start
	if ServingWorkspace(cx.Flags) {
		serving, _, err := OpenCompleteServing(cx, string(start))
		if err != nil {
			return nil, err
		}
		return (discoveryservice.DatasetTraverseExecutor{Serving: serving, Repositories: cx.WS.Reader, Projection: cx.WS.Index}).Execute(cx.Context, request)
	}
	repositoryID, commitID, err := PinCommit(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	return (discoveryservice.RepoTraverseExecutor{Repositories: cx.WS.Reader, Projection: cx.WS.Index}).Execute(cx.Context, request, repositoryID, commitID)
}

// RelationEndpointFlag reads --object for relations and traverse: an object
// ID, the same identity a Relation endpoint stores. It never names a
// Repository; the request scope decides where relations are looked up.
func RelationEndpointFlag(raw string) (objectmodel.ObjectID, error) {
	id := objectmodel.ObjectID(raw)
	if err := objectmodel.ValidateObjectID(id); err != nil || strings.Contains(raw, "://") || strings.HasPrefix(raw, "urn:") {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "--object takes an object ID <namespace>/<name>, not a URN or repository-qualified reference")
	}
	return id, nil
}

// VerbDescribeSchema reports the AccessHints a schema declares, which is what
// decides the logical retrieval surface compiled into AccessSpec.
func VerbDescribeSchema(cx *Invocation) (any, error) {
	objectID := objectmodel.ObjectID(cx.Flag("object"))
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
			id := objectmodel.ObjectID(objectID)
			return (objectservice.ObjectLogExecutor{Allow: func(repo kernel.RepositoryID, id objectmodel.ObjectID) bool {
				return AllowedRepoRead(cx.Home, cx.Flags, string(repo), string(id))
			}, ReadLog: func(repositoryID kernel.RepositoryID, commitID kernel.CommitID, query objectmodel.ObjectLogQuery) ([]objectmodel.ObjectRevision, error) {
				allowed, err := serving.Contains(repositoryID, id)
				if err != nil || !allowed {
					return nil, err
				}
				repo, err := serving.Member(repositoryID)
				if err != nil {
					return nil, err
				}
				return repo.Log(id, commitID, query)
			}}).Execute(cx.Context, objectservice.ObjectLogRequest{Object: id, Pin: serving.Pin().Repositories, Limit: limit, Continuation: cx.Flag("continuation")})
		},
		func(repositoryID kernel.RepositoryID, commitID kernel.CommitID) (any, error) {
			objectID, err := cx.Require("object")
			if err != nil {
				return nil, err
			}
			id := objectmodel.ObjectID(objectID)
			pin := map[kernel.RepositoryID]kernel.CommitID{repositoryID: commitID}
			return (objectservice.ObjectLogExecutor{Allow: func(repo kernel.RepositoryID, id objectmodel.ObjectID) bool {
				return AllowedRepoRead(cx.Home, cx.Flags, string(repo), string(id))
			}, ReadLog: func(member kernel.RepositoryID, commit kernel.CommitID, query objectmodel.ObjectLogQuery) ([]objectmodel.ObjectRevision, error) {
				return cx.WS.Reader.Log(member, id, commit, query)
			}}).Execute(cx.Context, objectservice.ObjectLogRequest{Object: id, Pin: pin, Limit: limit, Continuation: cx.Flag("continuation")})
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

func SetTelemetryChangeCounts(observation *invoke.ObservationTelemetry, operations []objectmodel.Operation) {
	observation = invoke.NoObservationTelemetry(observation)
	puts, removes := 0, 0
	for _, operation := range operations {
		switch operation.Op {
		case objectmodel.OpPut:
			puts++
		case objectmodel.OpRemove:
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

func AspectSelectorFrom(flags map[string]invoke.FlagValue) *objectmodel.AspectSelector {
	include := invoke.FlagStrings(flags, "include")
	exclude := invoke.FlagStrings(flags, "exclude")
	if len(include) == 0 && len(exclude) == 0 {
		return nil
	}
	return &objectmodel.AspectSelector{Include: include, Exclude: exclude}
}

func FilterObjectServingReads(home string, flags map[string]invoke.FlagValue, _ dataset.Service, values []objectmodel.ObservedValue) []objectmodel.ObservedValue {
	out := []objectmodel.ObservedValue{}
	for _, item := range values {
		if AllowedRepoRead(home, flags, string(item.Repository), string(item.ObjectID)) {
			out = append(out, item)
		}
	}
	return out
}

func ParseJSON(text, label string) (any, error) {
	var value any
	if err := kernel.UnmarshalJSON([]byte(text), &value); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON", label)
	}
	return value, nil
}
