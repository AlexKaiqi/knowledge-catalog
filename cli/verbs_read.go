package cli

import (
	"fmt"

	"kc/catalog"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
)

// Consumer read verbs. Every one of them answers on two targets:
//
//	--workspace  federated Serving over this command's ResolveWorkspace pin
//	--repo  one maintainer-pinned {repository, commit}
//
// The branch is written once in onTarget so a new read verb cannot accidentally
// support only one of them, and so neither path can start following a live ref
// halfway through a command.

func readVerbs() map[string]command {
	return map[string]command{
		"resolve":         {stage: stageGoverned, run: verbResolve},
		"resolve-binding": {stage: stageGoverned, run: verbResolveBinding},
		"read":            {stage: stageGoverned, run: verbRead},
		"provenance":      {stage: stageGoverned, run: verbProvenance},
		"list":            {stage: stageGoverned, run: verbList},
		"relations":       {stage: stageGoverned, run: verbRelations},
		"describe-schema": {stage: stageGoverned, run: verbDescribeSchema},
		"log":             {stage: stageGoverned, run: verbLog},
		"diff":            {stage: stageGoverned, run: verbDiff},
		"checkout":        {stage: stageGoverned, run: verbCheckout},
		"sync":            {stage: stageGoverned, run: verbSync},
		"inspect":         {stage: stageGoverned, run: verbInspect},
	}
}

// verbResolveBinding returns the stable declaration required by an upper
// Materialization runtime. It never invokes the runtime or returns live data.
func verbResolveBinding(cx *invocation) (any, error) {
	address, err := addressFrom(cx.Flags)
	if err != nil {
		return nil, err
	}
	if address.AspectName == "" {
		return nil, fmt.Errorf("resolve-binding requires --aspect")
	}
	return onTarget(cx,
		func(serving *reader.Serving, _ *catalog.Catalog) (any, error) {
			return serving.ResolveBinding(address)
		},
		func(repositoryID kernel.RepositoryID, commitID kernel.CommitID) (any, error) {
			return cx.WS.Reader.ResolveBinding(repositoryID, commitID, address)
		})
}

type (
	workspaceRead  = func(serving *reader.Serving, cat *catalog.Catalog) (any, error)
	repositoryRead = func(repositoryID kernel.RepositoryID, commitID kernel.CommitID) (any, error)
)

// onTarget dispatches a read to the Workspace pin or to one pinned Repository commit.
func onTarget(cx *invocation, onWorkspace workspaceRead, onRepository repositoryRead) (any, error) {
	if servingWorkspace(cx.Flags) {
		objectScope := FlagString(cx.Flags, "object")
		if cx.Command == "relations" {
			// Relation results have identities distinct from the requested
			// endpoint, so object-scoped endpoint access cannot prove complete
			// coverage of the returned relation objects.
			objectScope = ""
		}
		serving, cat, err := openCompleteServing(cx.WS, cx.Flags, objectScope)
		if err != nil {
			return nil, err
		}
		return onWorkspace(serving, cat)
	}
	repositoryID, commitID, err := pinCommit(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	return onRepository(repositoryID, commitID)
}

func (cx *invocation) knowledgeRef(repositoryID kernel.RepositoryID) (knowledge.KnowledgeRef, error) {
	objectID, err := cx.require("object")
	if err != nil {
		return knowledge.KnowledgeRef{}, err
	}
	return knowledge.KnowledgeRef{Repository: repositoryID, Object: knowledge.ObjectID(objectID)}, nil
}

// verbResolve without --object reports the Workspace pin itself; with --object it
// reports where that object resolves.
func verbResolve(cx *invocation) (any, error) {
	if servingWorkspace(cx.Flags) && cx.flag("object") == "" {
		cat, err := pickCatalog(cx.WS, cx.Flags)
		if err != nil {
			return nil, err
		}
		workspaceID, err := cx.workspaceID()
		if err != nil {
			return nil, err
		}
		resolved, err := resolveOrReplay(cx.WS, cx.Home, cat, workspaceID, cx.Flags)
		if err != nil {
			return nil, err
		}
		if err := requireCompleteWorkspaceRead(cx.Home, cx.Flags, workspacePin(resolved), ""); err != nil {
			return nil, err
		}
		return resolved, nil
	}
	return onTarget(cx,
		func(serving *reader.Serving, _ *catalog.Catalog) (any, error) {
			if cx.flag("aspect") != "" {
				address, err := addressFrom(cx.Flags)
				if err != nil {
					return nil, err
				}
				return serving.ResolveAddress(address)
			}
			objectID, err := cx.require("object")
			if err != nil {
				return nil, err
			}
			return serving.Resolve(knowledge.ObjectID(objectID))
		},
		func(repositoryID kernel.RepositoryID, commitID kernel.CommitID) (any, error) {
			if cx.flag("aspect") != "" {
				address, err := addressFrom(cx.Flags)
				if err != nil {
					return nil, err
				}
				return cx.WS.Reader.ResolveAddress(repositoryID, address, commitID)
			}
			ref, err := cx.knowledgeRef(repositoryID)
			if err != nil {
				return nil, err
			}
			return cx.WS.Reader.Resolve(ref, commitID)
		})
}

func verbRead(cx *invocation) (any, error) {
	if readingCatalog(cx.Command, cx.Flags) {
		return readCatalogState(cx)
	}
	return onTarget(cx,
		func(serving *reader.Serving, cat *catalog.Catalog) (any, error) {
			logical, err := logicalWorkspaceServing(cx, serving)
			if err != nil {
				return nil, err
			}
			if cx.flag("aspect") != "" {
				address, err := addressFrom(cx.Flags)
				if err != nil {
					return nil, err
				}
				values, err := logical.ReadAddress(cx.Context, address)
				if err != nil {
					return nil, err
				}
				return filterKnowledgeServingReads(cx.Home, cx.Flags, cat, values), nil
			}
			objectID, err := cx.require("object")
			if err != nil {
				return nil, err
			}
			values, err := logical.Read(cx.Context, knowledge.ObjectID(objectID), aspectSelectorFrom(cx.Flags))
			if err != nil {
				return nil, err
			}
			return filterKnowledgeServingReads(cx.Home, cx.Flags, cat, values), nil
		},
		func(repositoryID kernel.RepositoryID, commitID kernel.CommitID) (any, error) {
			if cx.flag("aspect") != "" {
				address, err := addressFrom(cx.Flags)
				if err != nil {
					return nil, err
				}
				return cx.WS.Reader.ReadAddress(repositoryID, address, commitID)
			}
			ref, err := cx.knowledgeRef(repositoryID)
			if err != nil {
				return nil, err
			}
			return cx.WS.Reader.Read(ref, commitID, aspectSelectorFrom(cx.Flags))
		})
}

// verbProvenance returns the origin envelopes stamped on each unit. It does not
// crawl sourceRefs and it is not git log.
func verbProvenance(cx *invocation) (any, error) {
	return onTarget(cx,
		func(serving *reader.Serving, _ *catalog.Catalog) (any, error) {
			objectID, err := cx.require("object")
			if err != nil {
				return nil, err
			}
			traces, err := serving.GetProvenance(knowledge.ObjectID(objectID))
			if err != nil {
				return nil, err
			}
			out := []knowledge.ProvenanceTrace{}
			for _, trace := range traces {
				if allowedRepoRead(cx.Home, cx.Flags, string(trace.Repository), string(trace.ObjectID)) {
					out = append(out, trace)
				}
			}
			return out, nil
		},
		func(repositoryID kernel.RepositoryID, commitID kernel.CommitID) (any, error) {
			ref, err := cx.knowledgeRef(repositoryID)
			if err != nil {
				return nil, err
			}
			return cx.WS.Reader.GetProvenance(ref, commitID)
		})
}

func verbList(cx *invocation) (any, error) {
	limit, err := limitFrom(cx.Flags, knowledge.DefaultPageLimit)
	if err != nil {
		return nil, err
	}
	request := knowledge.PageRequest{Limit: limit, Continuation: cx.flag("continuation")}
	return onTarget(cx,
		func(serving *reader.Serving, cat *catalog.Catalog) (any, error) {
			logical, err := logicalWorkspaceServing(cx, serving)
			if err != nil {
				return nil, err
			}
			page, err := logical.ListPage(cx.Context, request)
			if err != nil {
				return nil, err
			}
			page.Values = filterKnowledgeServingReads(cx.Home, cx.Flags, cat, page.Values)
			return page, nil
		},
		func(repositoryID kernel.RepositoryID, commitID kernel.CommitID) (any, error) {
			return cx.WS.Reader.ListPage(repositoryID, commitID, request)
		})
}

// verbRelations returns one-hop Canonical Relations touching --object. It does
// not recursively traverse a graph; every hit is read at this command's pin.
func verbRelations(cx *invocation) (any, error) {
	objectID, err := cx.require("object")
	if err != nil {
		return nil, err
	}
	query := reader.RelationQuery{
		Endpoint: knowledge.ObjectID(objectID), RelationType: cx.flag("relation-type"), Role: cx.flag("role"),
	}
	return onTarget(cx,
		func(serving *reader.Serving, _ *catalog.Catalog) (any, error) {
			hits, err := serving.Relations(query)
			if err != nil {
				return nil, err
			}
			out := []reader.RelationHit{}
			for _, hit := range hits {
				if allowedRepoRead(cx.Home, cx.Flags, string(hit.Repository), string(hit.ObjectID)) {
					out = append(out, hit)
				}
			}
			return out, nil
		},
		func(repositoryID kernel.RepositoryID, commitID kernel.CommitID) (any, error) {
			return cx.WS.Reader.Relations(repositoryID, commitID, query)
		})
}

// verbDescribeSchema reports the AccessHints a schema declares, which is what
// decides the logical retrieval surface compiled into AccessSpec.
func verbDescribeSchema(cx *invocation) (any, error) {
	objectID := knowledge.ObjectID(cx.flag("object"))
	return onTarget(cx,
		func(serving *reader.Serving, _ *catalog.Catalog) (any, error) {
			reports, err := serving.DescribeSchema(objectID)
			if err != nil {
				return nil, err
			}
			out := []reader.SchemaReport{}
			for _, report := range reports {
				if allowedRepoRead(cx.Home, cx.Flags, string(report.Repository), string(objectID)) {
					out = append(out, report)
				}
			}
			return out, nil
		},
		func(repositoryID kernel.RepositoryID, commitID kernel.CommitID) (any, error) {
			return cx.WS.Reader.DescribeSchema(repositoryID, commitID, objectID)
		})
}

// verbLog lists the commits that introduced each digest of one object. Registry
// history is `kc audit`; current combination space is `kc read --catalog`.
func verbLog(cx *invocation) (any, error) {
	if cx.flag("object") == "" && cx.flag("repo") == "" {
		if _, ok := cx.Flags["catalog"]; ok {
			return nil, fmt.Errorf("catalog registry history is kc audit")
		}
	}
	limit, err := limitFrom(cx.Flags, 0)
	if err != nil {
		return nil, err
	}
	return onTarget(cx,
		func(serving *reader.Serving, _ *catalog.Catalog) (any, error) {
			objectID, err := cx.require("object")
			if err != nil {
				return nil, err
			}
			logs, err := serving.Log(knowledge.ObjectID(objectID), limit)
			if err != nil {
				return nil, err
			}
			return filterWorkspaceLogs(cx.Home, cx.Flags, logs), nil
		},
		func(repositoryID kernel.RepositoryID, commitID kernel.CommitID) (any, error) {
			objectID, err := cx.require("object")
			if err != nil {
				return nil, err
			}
			return cx.WS.Reader.Log(repositoryID, knowledge.ObjectID(objectID), commitID, limit)
		})
}

// verbDiff compares one object across two pinned commits. It is a maintainer
// verb: both ends are named explicitly, so there is no Workspace path.
func verbDiff(cx *invocation) (any, error) {
	repositoryID, err := cx.repoFlag()
	if err != nil {
		return nil, err
	}
	objectID, err := cx.require("object")
	if err != nil {
		return nil, err
	}
	from, err := cx.require("from")
	if err != nil {
		return nil, err
	}
	to, err := cx.require("to")
	if err != nil {
		return nil, err
	}
	return cx.WS.Reader.Diff(repositoryID, knowledge.ObjectID(objectID), kernel.CommitID(from), kernel.CommitID(to))
}

// verbCheckout materialises this command's Workspace pin as a read-only tree for
// grep. It only makes sense on a Workspace, so --repo is rejected rather than guessed.
func verbCheckout(cx *invocation) (any, error) {
	if !servingWorkspace(cx.Flags) {
		return nil, fmt.Errorf("checkout requires --workspace (do not pass --repo / --commit / --ref)")
	}
	return checkoutWorkspace(cx.WS, cx.Home, cx.Flags)
}

func verbInspect(cx *invocation) (any, error) {
	if !servingWorkspace(cx.Flags) {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "inspect requires --workspace")
	}
	return inspectWorkspace(cx.WS, cx.Flags)
}
