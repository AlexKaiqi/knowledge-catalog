package cli

import (
	"context"

	kcclient "kc/client"
	"kc/invoke"
	"kc/kernel"
	accesscli "kc/liveaccess/cli"
)

func runRemoteOperations(ctx context.Context, client *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	var output any
	service := client.OperationsService()
	switch path {
	case "operations hook add":
		request := kcclient.PolicyBindingRequest{On: invoke.FlagString(flags, "on"), Phase: invoke.FlagString(flags, "phase"), Repository: invoke.FlagString(flags, "repo"), Run: invoke.FlagString(flags, "run"), URL: invoke.FlagString(flags, "url")}
		err := service.AddHook(ctx, request, options, &output)
		return output, err
	case "operations hook list":
		err := service.Hooks(ctx, options, &output)
		return output, err
	case "operations hook remove":
		err := service.RemoveHook(ctx, invoke.FlagString(flags, "id"), options, &output)
		return output, err
	case "operations audit access", "operations audit hitmap":
		limit, err := remoteLimit(flags)
		if err != nil {
			return nil, err
		}
		request := kcclient.AuditQueryRequest{
			RequestID: accesscli.AuditRequestID(snapshotFlags(flags)),
			Principal: invoke.FlagString(flags, "filter-principal"), OnBehalfOf: invoke.FlagString(flags, "filter-on-behalf-of"),
			Action: invoke.FlagString(flags, "action"), TraceID: invoke.FlagString(flags, "trace-id"),
			Repository: invoke.FlagString(flags, "repo"), Object: invoke.FlagString(flags, "object"),
			Since: invoke.FlagString(flags, "since"), Until: invoke.FlagString(flags, "until"),
			Continuation: invoke.FlagString(flags, "continuation"), Limit: limit, EvidenceID: invoke.FlagString(flags, "evidence-id"),
		}
		if path == "operations audit access" {
			err = service.AccessLog(ctx, request, options, &output)
			return output, err
		}
		err = service.Hitmap(ctx, request, options, &output)
		return output, err
	case "operations audit trace":
		err := service.Trace(ctx, invoke.FlagString(flags, "trace-id"), options, &output)
		return output, err
	case "operations feedback record":
		request := kcclient.FeedbackRequest{Dataset: invoke.FlagString(flags, "dataset"), TraceID: invoke.FlagString(flags, "trace-id"), Outcome: invoke.FlagString(flags, "outcome"), Message: invoke.FlagString(flags, "message")}
		err := service.Feedback(ctx, request, options, &output)
		return output, err
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote typed client does not implement %s", path)
	}
}
