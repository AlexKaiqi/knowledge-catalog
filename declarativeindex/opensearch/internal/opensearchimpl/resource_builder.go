package opensearchimpl

import (
	ports "kc/discovery/projection"
	"kc/kernel"
)

// CommitResource registers preparation before publishing ROOT. Recovery may
// confirm adoption while the caller waits, but cannot lose the original ticket.
func (s *rebuildSession) CommitResource() (ports.PreparedResource, error) {
	if s.done || s.preparation != nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "protected rebuild session is closed or already prepared")
	}
	id, err := newResourceID()
	if err != nil {
		return nil, s.Abort(err)
	}
	ticket := &preparedResource{id: id, session: s}
	s.preparation = ticket
	if err := s.Commit(); err != nil {
		return nil, err
	}
	return ticket, nil
}
