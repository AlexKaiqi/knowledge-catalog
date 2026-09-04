package cli

import (
	"context"

	kcclient "kc/client"
	"kc/kernel"
)

func runRemoteOperations(ctx context.Context, client *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	var output any
	service := client.OperationsService()
	switch path {
	case "operations projection sync":
		request := remoteProjectionRequest(flags)
		err := service.SyncProjection(ctx, request, options, &output)
		return output, err
	case "operations projection notify":
		request := remoteProjectionNotifyRequest(flags)
		err := service.NotifyProjection(ctx, request, options, &output)
		return output, err
	case "operations projection describe":
		request := remoteProjectionRequest(flags)
		err := service.DescribeProjection(ctx, request, options, &output)
		return output, err
	case "operations access describe":
		request := kcclient.AccessSpecDescribeRequest{
			Catalog: FlagString(flags, "catalog"), Workspace: FlagString(flags, "workspace"), Pin: remotePin(flags),
		}
		err := service.DescribeAccessSpec(ctx, request, options, &output)
		return output, err
	case "operations hook add", "operations gate add":
		request := kcclient.PolicyBindingRequest{On: FlagString(flags, "on"), Phase: FlagString(flags, "phase"), Repository: FlagString(flags, "repo"), Catalog: FlagString(flags, "catalog"), Run: FlagString(flags, "run"), URL: FlagString(flags, "url"), Require: splitCmds(FlagString(flags, "require"))}
		if path == "operations hook add" {
			err := service.AddHook(ctx, request, options, &output)
			return output, err
		}
		err := service.AddGate(ctx, request, options, &output)
		return output, err
	case "operations hook list", "operations gate list":
		if path == "operations hook list" {
			err := service.Hooks(ctx, options, &output)
			return output, err
		}
		err := service.Gates(ctx, options, &output)
		return output, err
	case "operations hook remove", "operations gate remove":
		if path == "operations hook remove" {
			err := service.RemoveHook(ctx, FlagString(flags, "id"), options, &output)
			return output, err
		}
		err := service.RemoveGate(ctx, FlagString(flags, "id"), options, &output)
		return output, err
	case "operations audit access", "operations audit hitmap":
		limit, err := remoteLimit(flags)
		if err != nil {
			return nil, err
		}
		request := kcclient.AuditQueryRequest{
			Principal: FlagString(flags, "filter-principal"), OnBehalfOf: FlagString(flags, "filter-on-behalf-of"),
			Action: FlagString(flags, "action"), TraceID: FlagString(flags, "trace-id"),
			Repository: FlagString(flags, "repo"), Object: FlagString(flags, "object"),
			Since: FlagString(flags, "since"), Until: FlagString(flags, "until"),
			Continuation: FlagString(flags, "continuation"), Limit: limit, EvidenceID: FlagString(flags, "evidence-id"),
		}
		if path == "operations audit access" {
			err = service.AccessLog(ctx, request, options, &output)
			return output, err
		}
		err = service.Hitmap(ctx, request, options, &output)
		return output, err
	case "operations audit trace":
		err := service.Trace(ctx, FlagString(flags, "trace-id"), options, &output)
		return output, err
	case "operations feedback record":
		request := kcclient.FeedbackRequest{Workspace: FlagString(flags, "workspace"), TraceID: FlagString(flags, "trace-id"), Outcome: FlagString(flags, "outcome"), Message: FlagString(flags, "message")}
		err := service.Feedback(ctx, request, options, &output)
		return output, err
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote typed client does not implement %s", path)
	}
}

func remoteProjectionRequest(flags map[string]FlagValue) kcclient.ProjectionSyncRequest {
	return kcclient.ProjectionSyncRequest{
		Repository: FlagString(flags, "repo"),
		Commit:     FlagString(flags, "commit"),
		Ref:        FlagString(flags, "ref"),
	}
}

func remoteProjectionNotifyRequest(flags map[string]FlagValue) kcclient.ProjectionNotifyRequest {
	request := kcclient.ProjectionNotifyRequest{
		Repository:     FlagString(flags, "repo"),
		Ref:            FlagString(flags, "ref"),
		SourceRevision: FlagString(flags, "source-revision"),
	}
	if object := FlagString(flags, "object"); object != "" {
		kind := FlagString(flags, "kind")
		if kind == "" && FlagString(flags, "aspect") != "" {
			kind = "Aspect"
		}
		request.Address = &kcclient.ProjectionNoticeAddress{
			Kind: kind, ObjectID: object, AspectName: FlagString(flags, "aspect"),
		}
	}
	return request
}
