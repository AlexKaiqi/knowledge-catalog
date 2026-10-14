// Package service owns fixed object application use cases shared by CLI and
// HTTP transports. It composes Reader and Writer but does not parse argv,
// register routes, open concrete providers, or encode transport responses.
package service

import (
	"context"

	"kc/kernel"
	objectmodel "kc/object"
)

type CanonicalReader interface {
	ReadReference(objectmodel.Reference, kernel.CommitID) (objectmodel.KnowledgeValue, error)
}

type ReadRequest struct {
	Reference objectmodel.Reference
	Commit    kernel.CommitID
}

type ReadExecutor struct {
	Reader CanonicalReader
}

func (e ReadExecutor) Execute(_ context.Context, request ReadRequest) (objectmodel.KnowledgeValue, error) {
	if e.Reader == nil {
		return objectmodel.KnowledgeValue{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"knowledge read service is unavailable")
	}
	if request.Commit == "" {
		return objectmodel.KnowledgeValue{}, kernel.Fail(kernel.ErrUsageInvalid,
			"reference read requires a fixed commit")
	}
	if _, err := objectmodel.FormatReference(request.Reference); err != nil {
		return objectmodel.KnowledgeValue{}, err
	}
	return e.Reader.ReadReference(request.Reference, request.Commit)
}
