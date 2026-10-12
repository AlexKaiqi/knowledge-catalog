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
	case "operations projection notice":
		request := remoteProjectionNotifyRequest(flags)
		err := service.NotifyProjection(ctx, request, options, &output)
		return output, err
	case "operations projection describe":
		request := remoteProjectionRequest(flags)
		err := service.DescribeProjection(ctx, request, options, &output)
		return output, err
	case "operations access-spec describe":
		if err := prepareKnowledgePinContext(flags); err != nil {
			return nil, err
		}
		request := kcclient.AccessSpecDescribeRequest{
			Dataset:    flagString(flags, "dataset"),
			Repository: flagString(flags, "repo"), Pin: remotePin(flags),
		}
		err := service.DescribeAccessSpec(ctx, request, options, &output)
		return output, err
	case "operations hook add", "operations gate add":
		request := kcclient.PolicyBindingRequest{On: flagString(flags, "on"), Phase: flagString(flags, "phase"), Repository: flagString(flags, "repo"), Run: flagString(flags, "run"), URL: flagString(flags, "url"), Require: splitCmds(flagString(flags, "require"))}
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
			err := service.RemoveHook(ctx, flagString(flags, "id"), options, &output)
			return output, err
		}
		err := service.RemoveGate(ctx, flagString(flags, "id"), options, &output)
		return output, err
	case "operations audit access", "operations audit hitmap":
		limit, err := remoteLimit(flags)
		if err != nil {
			return nil, err
		}
		request := kcclient.AuditQueryRequest{
			Principal: flagString(flags, "filter-principal"), OnBehalfOf: flagString(flags, "filter-on-behalf-of"),
			Action: flagString(flags, "action"), TraceID: flagString(flags, "trace-id"),
			Repository: flagString(flags, "repo"), Object: flagString(flags, "object"),
			Since: flagString(flags, "since"), Until: flagString(flags, "until"),
			Continuation: flagString(flags, "continuation"), Limit: limit, EvidenceID: flagString(flags, "evidence-id"),
		}
		if path == "operations audit access" {
			err = service.AccessLog(ctx, request, options, &output)
			return output, err
		}
		err = service.Hitmap(ctx, request, options, &output)
		return output, err
	case "operations audit trace":
		err := service.Trace(ctx, flagString(flags, "trace-id"), options, &output)
		return output, err
	case "operations feedback record":
		request := kcclient.FeedbackRequest{Dataset: flagString(flags, "dataset"), TraceID: flagString(flags, "trace-id"), Outcome: flagString(flags, "outcome"), Message: flagString(flags, "message")}
		err := service.Feedback(ctx, request, options, &output)
		return output, err
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote typed client does not implement %s", path)
	}
}

func remoteProjectionRequest(flags map[string]FlagValue) kcclient.ProjectionSyncRequest {
	return kcclient.ProjectionSyncRequest{
		Repository: flagString(flags, "repo"),
		Commit:     flagString(flags, "commit"),
		Ref:        flagString(flags, "ref"),
	}
}

func remoteProjectionNotifyRequest(flags map[string]FlagValue) kcclient.ProjectionNotifyRequest {
	request := kcclient.ProjectionNotifyRequest{
		Repository:     flagString(flags, "repo"),
		Ref:            flagString(flags, "ref"),
		SourceRevision: flagString(flags, "source-revision"),
	}
	if object := flagString(flags, "object"); object != "" {
		kind := flagString(flags, "kind")
		if kind == "" && flagString(flags, "aspect") != "" {
			kind = "Aspect"
		}
		request.Address = &kcclient.ProjectionNoticeAddress{
			Kind: kind, ObjectID: object, AspectName: flagString(flags, "aspect"),
		}
	}
	return request
}
