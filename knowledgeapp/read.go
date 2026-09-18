// Package knowledgeapp owns typed application use cases shared by CLI and
// HTTP transports. It composes protocol services but does not parse argv,
// register routes, open concrete providers, or encode transport responses.
package knowledgeapp

import (
	"context"

	"kc/kernel"
	"kc/knowledge"
)

type CanonicalReader interface {
	Read(knowledge.KnowledgeRef, kernel.CommitID, *knowledge.AspectSelector) (knowledge.KnowledgeValue, error)
	ReadAddress(kernel.RepositoryID, knowledge.Address, kernel.CommitID) (knowledge.KnowledgeValue, error)
}

type ReadRequest struct {
	Repository kernel.RepositoryID
	Commit     kernel.CommitID
	Object     knowledge.ObjectID
	Address    *knowledge.Address
	Selector   *knowledge.AspectSelector
}

type ReadExecutor struct {
	Reader CanonicalReader
}

func (e ReadExecutor) Execute(_ context.Context, request ReadRequest) (knowledge.KnowledgeValue, error) {
	if e.Reader == nil {
		return knowledge.KnowledgeValue{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"knowledge read service is unavailable")
	}
	if request.Repository == "" || request.Commit == "" {
		return knowledge.KnowledgeValue{}, kernel.Fail(kernel.ErrUsageInvalid,
			"knowledge read requires a repository and fixed commit")
	}
	if request.Address != nil {
		if request.Address.ObjectID == "" || (request.Object != "" && request.Object != request.Address.ObjectID) {
			return knowledge.KnowledgeValue{}, kernel.Fail(kernel.ErrUsageInvalid,
				"read address does not match its object")
		}
		return e.Reader.ReadAddress(request.Repository, *request.Address, request.Commit)
	}
	if request.Object == "" {
		return knowledge.KnowledgeValue{}, kernel.Fail(kernel.ErrUsageInvalid,
			"read requires an object")
	}
	return e.Reader.Read(knowledge.KnowledgeRef{
		Repository: request.Repository, Object: request.Object,
	}, request.Commit, request.Selector)
}
