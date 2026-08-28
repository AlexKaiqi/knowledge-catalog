// Package maintenance defines provider SPI used only for Snapshot rebuild,
// migration, explicit export and conformance. It is not a consumer API.
package maintenance

import (
	"kc/kernel"
	"kc/knowledge"
)

type ScanRequest struct {
	Limit        int
	Continuation string
}

type ScanPage struct {
	Values       []knowledge.KnowledgeValue
	Continuation string
	Exhausted    bool
}

const (
	DefaultScanLimit = 100
	MaxScanLimit     = 1000
)

func NormalizeScanLimit(limit int) (int, error) {
	if limit < 0 {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "scan limit cannot be negative")
	}
	if limit == 0 {
		return DefaultScanLimit, nil
	}
	if limit > MaxScanLimit {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "scan limit cannot exceed %d", MaxScanLimit)
	}
	return limit, nil
}

type SnapshotScanner interface {
	ScanSnapshotPage(kernel.CommitID, ScanRequest) (ScanPage, error)
}

func RequireScanner(value any) (SnapshotScanner, error) {
	scanner, ok := value.(SnapshotScanner)
	if ok {
		return scanner, nil
	}
	repository, repositoryOK := value.(knowledge.Repository)
	pager, pagerOK := value.(knowledge.SnapshotObjectPager)
	batch, batchOK := value.(knowledge.BatchReadStore)
	if repositoryOK && pagerOK && batchOK {
		return &pagedRepositoryScanner{repository: repository, pager: pager, batch: batch}, nil
	}
	return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository does not provide maintenance Snapshot scan")
}

type pagedRepositoryScanner struct {
	repository knowledge.Repository
	pager      knowledge.SnapshotObjectPager
	batch      knowledge.BatchReadStore
}

func (s *pagedRepositoryScanner) ScanSnapshotPage(commit kernel.CommitID, request ScanRequest) (ScanPage, error) {
	limit, err := NormalizeScanLimit(request.Limit)
	if err != nil {
		return ScanPage{}, err
	}
	identities, err := s.pager.ObjectIDsPage(commit, limit, request.Continuation)
	if err != nil {
		return ScanPage{}, err
	}
	values, err := s.batch.ReadMany(identities.ObjectIDs, commit)
	if err != nil {
		return ScanPage{}, err
	}
	page := ScanPage{Values: make([]knowledge.KnowledgeValue, 0, len(identities.ObjectIDs)),
		Continuation: identities.Continuation, Exhausted: identities.Exhausted}
	for _, objectID := range identities.ObjectIDs {
		value, ok := values[objectID]
		if !ok {
			return ScanPage{}, kernel.Fail(kernel.ErrPreconditionFailed,
				"maintenance identity %s is missing from repository %s at commit %s", objectID, s.repository.ID(), commit)
		}
		page.Values = append(page.Values, value)
	}
	return page, nil
}

func WalkSnapshot(scanner SnapshotScanner, commit kernel.CommitID, visit func(knowledge.KnowledgeValue) error) error {
	request := ScanRequest{Limit: MaxScanLimit}
	for {
		page, err := scanner.ScanSnapshotPage(commit, request)
		if err != nil {
			return err
		}
		for _, value := range page.Values {
			if err := visit(value); err != nil {
				return err
			}
		}
		if page.Exhausted {
			return nil
		}
		if page.Continuation == "" || page.Continuation == request.Continuation {
			return kernel.Fail(kernel.ErrTemporaryUnavailable, "Snapshot scanner returned a non-advancing page")
		}
		request.Continuation = page.Continuation
	}
}

func WalkRepository(value any, commit kernel.CommitID, visit func(knowledge.KnowledgeValue) error) error {
	scanner, err := RequireScanner(value)
	if err != nil {
		return err
	}
	return WalkSnapshot(scanner, commit, visit)
}
