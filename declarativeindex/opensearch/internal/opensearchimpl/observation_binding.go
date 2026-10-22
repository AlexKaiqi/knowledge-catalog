package opensearchimpl

import (
	"context"
	"reflect"
	"time"

	ports "kc/discovery/projection"
	"kc/kernel"
)

type observationAdmission struct {
	record controlDoc
	until  int64
}

func expiredObservation() error {
	return kernel.Fail(kernel.ErrPreconditionFailed, "observation continuation expired; restart the search")
}

func observationEvidence(record controlDoc) []byte {
	return []byte(kernel.CanonicalDigest(struct {
		Domain     string
		Repository string
		Meta       ports.Meta
	}{"observation-query-v1", record.Repository, readyResourceMeta(record)}))
}

func observationDeadline(root, record controlDoc, admittedAt int64) (int64, error) {
	if root.State != ports.ProjectionStateReady {
		return 0, kernel.Fail(kernel.ErrPreconditionFailed, "observation authority is not ready")
	}
	if root.Generation == record.Generation {
		if root.Cause != "observation" || !reflect.DeepEqual(readyResourceMeta(root), readyResourceMeta(record)) {
			return 0, kernel.Fail(kernel.ErrPreconditionFailed, "observation authority changed its fixed metadata")
		}
		return 0, nil
	}
	until := root.ObservationDeadlines[record.Generation]
	if until <= admittedAt {
		return 0, expiredObservation()
	}
	return until, nil
}

// BindObservation shares resource CAS admission and independent query leases
// with immutable publications. Its eligibility root is the accepted observation
// pointer, rather than a Dataset preparation receipt.
func (e *openSearchEngine) BindObservation(ctx context.Context, token string) (ports.Engine, error) {
	if err := e.requireMutableProjection(); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var state pitContinuation
	if token != "" {
		decoded, err := decodePITContinuation(token)
		if err != nil || !validGeneration(decoded.Generation) || decoded.Namespace != namespaceDigest(e.namespace) {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "invalid observation continuation scope")
		}
		state = decoded
	}
	root, rootVersion, err := e.loadControlContext(ctx)
	if err != nil {
		return nil, err
	}
	if rootVersion == nil || root.Cause != "observation" || root.State != ports.ProjectionStateReady {
		code := kernel.ErrPreconditionFailed
		if token == "" {
			code = kernel.ErrCapabilityUnsatisfied
		}
		return nil, kernel.Fail(code, "published observation is unavailable")
	}
	record, version := root, rootVersion
	if token != "" {
		session := &rebuildSession{engine: e, generation: state.Generation, physicalIndex: e.prefix + "-g-" + state.Generation}
		record, version, err = e.loadControlRecord(ctx, session.resourceID())
		if err != nil {
			return nil, err
		}
		if version == nil || record.Basis != string(state.Basis) {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "observation continuation resource is unavailable")
		}
	}
	if !validGeneration(record.Generation) || record.ProjectionRevision == "" || record.ObservationDigest == "" || record.Cause != "observation" || record.Repository != string(e.repository) || record.ActiveIndex != e.prefix+"-g-"+record.Generation {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "observation resource identity is invalid")
	}
	until, err := observationDeadline(root, record, time.Now().UnixMilli())
	if err != nil {
		return nil, err
	}
	session := &rebuildSession{engine: e, generation: record.Generation, physicalIndex: record.ActiveIndex, resourceContext: ctx}
	resource, resourceVersion, err := e.loadControlRecord(ctx, session.resourceID())
	if err != nil {
		return nil, err
	}
	if resourceVersion == nil || resource.State != "RESOURCE" || resource.Repository != record.Repository || resource.Generation != record.Generation || resource.ActiveIndex != record.ActiveIndex || resource.Cause != "observation" || !reflect.DeepEqual(readyResourceMeta(resource), readyResourceMeta(record)) {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "observation resource ownership is unavailable")
	}
	evidence := observationEvidence(record)
	ticket := &preparedResource{session: session, observation: &observationAdmission{record: record, until: until}}
	if token != "" {
		anchor, err := continuationClaim(resource.Resource, evidence, state)
		if err != nil {
			return nil, err
		}
		ticket.queryAnchor = &anchor
	}
	bound, err := e.openPublicationView(ctx, record, *version, ticket, evidence)
	if err != nil {
		return nil, err
	}
	view := bound.(*openSearchEngine)
	// Publication and query admission are different authorities. Recheck the
	// accepted pointer after obtaining resource protection, then keep the bounded
	// in-flight lease even if the original continuation deadline passes.
	current, _, err := e.loadControlContext(ctx)
	if err == nil {
		_, err = observationDeadline(current, record, view.queryProtection.admittedAt)
		if err != nil && until > view.queryProtection.admittedAt && until <= time.Now().UnixMilli() && current.Cause == "observation" && current.Generation != record.Generation && current.State == ports.ProjectionStateReady && current.ObservationDeadlines[record.Generation] == 0 {
			err = nil
		}
	}
	if err != nil {
		_ = bound.Close()
		return nil, err
	}
	return bound, nil
}
