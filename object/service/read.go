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
	ReadReference(objectmodel.Reference, kernel.CommitID) (objectmodel.ObjectValue, error)
}

type ReadRequest struct {
	Reference objectmodel.Reference
	Commit    kernel.CommitID
}

type ReadExecutor struct {
	Reader CanonicalReader
}

func (e ReadExecutor) Execute(_ context.Context, request ReadRequest) (objectmodel.ObjectValue, error) {
	if e.Reader == nil {
		return objectmodel.ObjectValue{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"object read service is unavailable")
	}
	if request.Commit == "" {
		return objectmodel.ObjectValue{}, kernel.Fail(kernel.ErrUsageInvalid,
			"reference read requires a fixed commit")
	}
	if request.Reference.Repository == "" {
		return objectmodel.ObjectValue{}, kernel.Fail(kernel.ErrUsageInvalid,
			"reference read at a fixed commit needs the Repository that commit belongs to")
	}
	if _, err := objectmodel.FormatReference(request.Reference); err != nil {
		return objectmodel.ObjectValue{}, err
	}
	return e.Reader.ReadReference(request.Reference, request.Commit)
}
