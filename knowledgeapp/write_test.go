package knowledgeapp

import (
	"context"
	"testing"

	"kc/kernel"
	"kc/knowledge/writer"
)

type commitRecorder struct {
	command string
	intent  writer.CommitIntent
}

func (r *commitRecorder) CommitIntent(command string, intent writer.CommitIntent) (writer.CommitReceipt, error) {
	r.command, r.intent = command, intent
	return writer.CommitReceipt{CommandID: command}, nil
}

func TestCommitExecutorPreservesTypedIntent(t *testing.T) {
	recorder := &commitRecorder{}
	request := CommitRequest{
		CommandID: "command-1",
		Intent: writer.CommitIntent{
			TargetRepository: kernel.RepositoryID("kr://app/write"),
			BaseCommit:       kernel.CommitID("fixed"),
		},
	}
	receipt, err := (CommitExecutor{Writer: recorder}).Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if recorder.command != request.CommandID ||
		recorder.intent.TargetRepository != request.Intent.TargetRepository ||
		recorder.intent.BaseCommit != request.Intent.BaseCommit ||
		receipt.CommandID != request.CommandID {
		t.Fatalf("typed commit intent was not preserved: %#v %#v", recorder, receipt)
	}
}
