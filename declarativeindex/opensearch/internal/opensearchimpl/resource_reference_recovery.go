package opensearchimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	ports "kc/discovery/projection"
	"kc/kernel"
)

const publicationRecoveryCursor = "instance-publication-recovery"

// RecoverPublications checks this engine scope. The instance coordinator uses
// the same recovery over its complete namespace, independently of query handles.
// Neither incomplete scans nor unknown authority outcomes permit release.
func (e *openSearchEngine) RecoverPublications(ctx context.Context, resolve func(context.Context, []byte) (bool, error)) (ports.PublicationRecoveryReport, error) {
	if err := e.requireMutableProjection(); err != nil {
		return ports.PublicationRecoveryReport{}, err
	}
	return e.recoverPublications(ctx, resolve, false, claimAdmitted)
}

func (e *openSearchEngine) recoverPublications(ctx context.Context, resolve func(context.Context, []byte) (bool, error), instance bool, phase string) (report ports.PublicationRecoveryReport, resultErr error) {
	if resolve == nil {
		return report, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "publication recovery requires an authority resolver")
	}
	cursorID := publicationRecoveryCursor
	owner := ""
	if !instance {
		cursorID = e.controlID + "-publication-recovery"
		owner = string(e.repository)
	}
	if phase == claimAccepted {
		cursorID += "-references"
	}
	cursor, version, err := e.loadControlRecord(ctx, cursorID)
	if err != nil {
		return report, err
	}
	if version != nil && (cursor.State != "PUBLICATION_RECOVERY" || cursor.Repository != owner) {
		return report, kernel.Fail(kernel.ErrPreconditionFailed, "publication recovery cursor has different ownership")
	}
	var after []any
	if cursor.RecoveryAfter != "" {
		if err := decodeJSON([]byte(cursor.RecoveryAfter), &after); err != nil {
			return report, err
		}
		if err := validateResourceCursor(after, 2); err != nil {
			return report, err
		}
	}
	filters := []any{map[string]any{"term": map[string]any{"state": "RESOURCE"}}}
	if phase == claimAccepted {
		filters = append(filters, map[string]any{"term": map[string]any{"resource.claims.phase": claimAccepted}})
	} else {
		filters = append(filters, map[string]any{"terms": map[string]any{"resource.claims.phase": []string{claimAdmitted, claimQuery, claimContinuation, claimRootQuery, claimRootContinuation}}}, map[string]any{"range": map[string]any{"resource.claims.until": map[string]any{"lte": time.Now().UnixMilli()}}})
	}
	query := map[string]any{"bool": map[string]any{"filter": filters}}
	if !instance {
		filters := query["bool"].(map[string]any)["filter"].([]any)
		query["bool"].(map[string]any)["filter"] = append(filters, map[string]any{"term": map[string]any{"repository": owner}})
	}
	sort := []any{map[string]any{"repository": "asc"}, map[string]any{"generation": "asc"}}
	for page := 0; page < 16; page++ {
		hits, err := e.scanResourcePage(ctx, query, sort, after)
		if err != nil {
			return report, errors.Join(resultErr, err)
		}
		for _, hit := range hits {
			session, err := e.resourceSession(hit, instance, ctx)
			if err != nil {
				return report, errors.Join(resultErr, err)
			}
			partial, err := session.recoverPublicationClaims(ctx, hit.Source.Resource, resolve, phase)
			report.Confirmed = append(report.Confirmed, partial.Confirmed...)
			report.Released = append(report.Released, partial.Released...)
			report.Retained = append(report.Retained, partial.Retained...)
			resultErr = errors.Join(resultErr, err)
		}
		if len(hits) < 128 {
			if cursor.RecoveryAfter != "" {
				resultErr = errors.Join(resultErr, e.savePublicationCursor(ctx, cursorID, owner, "", version))
			}
			return report, resultErr
		}
		after, err = resourceNextCursor(hits, after, 2)
		if err != nil {
			return report, errors.Join(resultErr, err)
		}
	}
	report.More = true
	raw, err := json.Marshal(after)
	if err != nil {
		return report, errors.Join(resultErr, err)
	}
	return report, errors.Join(resultErr, e.savePublicationCursor(ctx, cursorID, owner, string(raw), version))
}

func (s *rebuildSession) recoverPublicationClaims(ctx context.Context, snapshot *generationResource, resolve func(context.Context, []byte) (bool, error), phase string) (report ports.PublicationRecoveryReport, resultErr error) {
	if snapshot == nil || !supportedResourceVersion(snapshot.Version) || snapshot.Phase != resourceAdopted || snapshot.Pending || snapshot.LeaseUntil <= 0 {
		report.Retained = append(report.Retained, s.physicalIndex)
		return report, kernel.Fail(kernel.ErrPreconditionFailed, "publication resource has unknown ownership, phase, or pending writes: %s", s.physicalIndex)
	}
	if err := validateResourceProtection(snapshot); err != nil {
		report.Retained = append(report.Retained, s.physicalIndex)
		return report, err
	}
	if slices.ContainsFunc(snapshot.Claims, func(claim resourceClaim) bool {
		return (claim.Phase == claimQuery || claim.Phase == claimContinuation || claim.Phase == claimRootQuery || claim.Phase == claimRootContinuation) && claim.Until <= time.Now().UnixMilli()
	}) {
		expired, err := s.releaseExpiredQueries()
		if err != nil {
			report.Retained = append(report.Retained, s.physicalIndex)
			resultErr = errors.Join(resultErr, err)
		} else {
			report.Released = append(report.Released, expired...)
		}
	}
	for _, claim := range snapshot.Claims {
		if claim.Phase != phase || (phase == claimAdmitted && claim.Until > time.Now().UnixMilli()) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return report, errors.Join(resultErr, err)
		}
		id := s.physicalIndex + "/" + claim.ID
		accepted, err := resolve(ctx, bytes.Clone(claim.Evidence))
		if err == nil && phase == claimAccepted && accepted {
			var replaced bool
			replaced, err = s.publicationBindingReplaced(ctx, claim)
			if replaced {
				accepted = false
			}
		}
		if err != nil {
			report.Retained = append(report.Retained, id)
			resultErr = errors.Join(resultErr, err)
			continue
		}
		if phase == claimAccepted && accepted {
			continue
		}
		confirmed, err := s.settlePublicationClaim(claim, accepted)
		if err != nil {
			report.Retained = append(report.Retained, id)
			resultErr = errors.Join(resultErr, err)
		} else if confirmed {
			report.Confirmed = append(report.Confirmed, id)
		} else {
			report.Released = append(report.Released, id)
		}
	}
	return report, resultErr
}

// Recheck the exact claim after authority IO. A concurrent publisher or
// recoverer may have confirmed it already; that protection cannot be removed.
func (s *rebuildSession) settlePublicationClaim(expected resourceClaim, accepted bool) (confirmed bool, err error) {
	err = s.changeResource(func(resource *generationResource) error {
		confirmed = false
		if resource.Phase != resourceAdopted || resource.Pending {
			return kernel.Fail(kernel.ErrPreconditionFailed, "publication resource protection changed during recovery")
		}
		for i := range resource.Claims {
			claim := &resource.Claims[i]
			if claim.ID != expected.ID {
				continue
			}
			if !bytes.Equal(claim.Evidence, expected.Evidence) {
				return kernel.Fail(kernel.ErrPreconditionFailed, "publication claim evidence changed during recovery")
			}
			if expected.Phase == claimAccepted {
				if claim.Phase != claimAccepted || claim.Until != expected.Until {
					return kernel.Fail(kernel.ErrPreconditionFailed, "accepted reference changed during authority lookup")
				}
			} else if claim.Phase == claimAccepted {
				confirmed = true
				return nil
			}
			if expected.Phase != claimAccepted && (claim.Phase != claimAdmitted || claim.Until > time.Now().UnixMilli()) {
				return kernel.Fail(kernel.ErrPreconditionFailed, "publication claim admission changed during recovery")
			}
			if accepted {
				claim.Phase = claimAccepted
				confirmed = true
			} else {
				resource.Claims = slices.Delete(resource.Claims, i, i+1)
			}
			return nil
		}
		return nil // Another recovery already released this exact ticket.
	})
	return confirmed, err
}

func (e *openSearchEngine) savePublicationCursor(ctx context.Context, id, owner, after string, expected *controlVersion) error {
	_, err := e.putControlRecordContext(ctx, id, controlDoc{State: "PUBLICATION_RECOVERY", Repository: owner, RecoveryAfter: after}, expected)
	if kernel.CodeOf(err) == kernel.ErrNonFastForward {
		return nil
	}
	return err
}

func (e *openSearchEngine) ReconcilePublicationReferences(ctx context.Context, referenced func(context.Context, []byte) (bool, error)) (ports.PublicationRecoveryReport, error) {
	if err := e.requireMutableProjection(); err != nil {
		return ports.PublicationRecoveryReport{}, err
	}
	return e.recoverPublications(ctx, referenced, false, claimAccepted)
}
