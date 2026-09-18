package knowledgeapp

import (
	"context"
	"strings"

	"kc/kernel"
	"kc/knowledge/writer"
)

type Committer interface {
	CommitIntent(string, writer.CommitIntent) (writer.CommitReceipt, error)
}

type CommitRequest struct {
	CommandID string
	Intent    writer.CommitIntent
}

type CommitExecutor struct {
	Writer Committer
}

func (e CommitExecutor) Execute(_ context.Context, request CommitRequest) (writer.CommitReceipt, error) {
	if e.Writer == nil {
		return writer.CommitReceipt{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"knowledge writer service is unavailable")
	}
	if strings.TrimSpace(request.CommandID) == "" {
		return writer.CommitReceipt{}, kernel.Fail(kernel.ErrUsageInvalid,
			"writer commit requires a command identity")
	}
	if request.Intent.TargetRepository == "" {
		return writer.CommitReceipt{}, kernel.Fail(kernel.ErrWriteTargetRequired,
			"writer commit requires a target repository")
	}
	return e.Writer.CommitIntent(request.CommandID, request.Intent)
}
