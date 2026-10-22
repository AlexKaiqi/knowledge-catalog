package opensearchimpl

import (
	"context"
	"encoding/json"
	"errors"
	ports "kc/discovery/projection"
	"time"

	"kc/kernel"
)

func resourceProperties() map[string]any {
	return map[string]any{"type": "object", "dynamic": "strict", "properties": map[string]any{
		"version":      map[string]any{"type": "integer"},
		"queryLeases":  map[string]any{"type": "integer"},
		"supersededBy": map[string]any{"type": "keyword"}, "phase": map[string]any{"type": "keyword"},
		"leaseUntil": map[string]any{"type": "long"}, "pending": map[string]any{"type": "boolean"},
		"claims": map[string]any{"type": "object", "dynamic": "strict", "properties": map[string]any{
			"id": map[string]any{"type": "keyword"}, "phase": map[string]any{"type": "keyword"},
			"until": map[string]any{"type": "long"}, "evidence": map[string]any{"type": "binary"},
		}},
	}}
}

// Recovery enumerates positively owned candidates and interrupted deletions.
// Historical adopted resources additionally require complete query coverage, a
// READY replacement, and no effective claims. Unknown legacy ownership stays.
// Resource documents remain durable deletion fences, including after a 404.
func (e *openSearchEngine) reconcileCandidates() error {
	_, err := e.RecoverResources(context.Background())
	return err
}

func (e *openSearchEngine) RecoverResources(ctx context.Context) (ports.ResourceRecoveryReport, error) {
	if err := e.requireMutableProjection(); err != nil {
		return ports.ResourceRecoveryReport{}, err
	}
	return e.recoverCandidates(ctx, false)
}

func (e *openSearchEngine) recoverCandidates(ctx context.Context, instance bool) (report ports.ResourceRecoveryReport, recoveryErr error) {
	defer func() {
		if err := ctx.Err(); err != nil {
			recoveryErr = err
		}
	}()
	if err := ctx.Err(); err != nil {
		return report, err
	}
	phases := []any{
		map[string]any{"term": map[string]any{"resource.phase": resourcePublishing}},
		map[string]any{"term": map[string]any{"resource.phase": resourceDeleting}},
		map[string]any{"bool": map[string]any{"filter": []any{
			map[string]any{"term": map[string]any{"resource.phase": resourceBuilding}},
			map[string]any{"range": map[string]any{"resource.leaseUntil": map[string]any{"lte": time.Now().UnixMilli()}}},
		}}},
	}
	if e.namespace != "" {
		phases = append(phases, map[string]any{"term": map[string]any{"resource.phase": resourceAdopted}})
	}
	query := map[string]any{"bool": map[string]any{"filter": []any{
		map[string]any{"term": map[string]any{"state": "RESOURCE"}},
		map[string]any{"terms": map[string]any{"resource.version": []int{1, 2, 3, 4, resourceRecordVersion}}},
		map[string]any{"bool": map[string]any{"minimum_should_match": 1, "should": phases}},
	}}}
	if !instance {
		filters := query["bool"].(map[string]any)["filter"].([]any)
		query["bool"].(map[string]any)["filter"] = append(filters, map[string]any{"term": map[string]any{"repository": string(e.repository)}})
	}
	cursorID := e.controlID + "-resource-recovery"
	sort := []any{map[string]any{"generation": "asc"}}
	if instance {
		cursorID = "instance-resource-recovery"
		sort = []any{map[string]any{"repository": "asc"}, map[string]any{"generation": "asc"}}
	}
	cursor, cursorVersion, err := e.loadControlRecord(ctx, cursorID)
	if err != nil {
		return report, err
	}
	if cursorVersion != nil && (cursor.Repository != string(e.repository) || cursor.State != "RESOURCE_RECOVERY") {
		return report, kernel.Fail(kernel.ErrPreconditionFailed, "resource recovery cursor has different ownership")
	}
	var after []any
	if cursor.RecoveryAfter != "" {
		if instance {
			if err := decodeJSON([]byte(cursor.RecoveryAfter), &after); err != nil {
				return report, err
			}
		} else {
			after = []any{cursor.RecoveryAfter}
		}
	}
	if len(after) > 0 {
		if err := validateResourceCursor(after, len(sort)); err != nil {
			return report, err
		}
	}
	for page := 0; page < 16; page++ {
		hits, err := e.scanResourcePage(ctx, query, sort, after)
		if err != nil {
			return report, err
		}
		for _, hit := range hits {
			session, err := e.resourceSession(hit, instance, ctx)
			if err != nil {
				return report, err
			}
			var discardErr error
			if hit.Source.Resource != nil && hit.Source.Resource.Phase == resourcePublishing {
				discardErr = session.recoverResourcePublication()
			} else if hit.Source.Resource != nil && hit.Source.Resource.Phase == resourceAdopted {
				discardErr = session.discardHistoricalResource()
			} else {
				discardErr = session.discardResourceAt(true)
			}
			if errors.Is(discardErr, errResourceProtected) {
				report.Protected = append(report.Protected, session.physicalIndex)
				continue
			}
			if err := discardErr; err != nil {
				if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
					return report, err
				}
				report.Retained = append(report.Retained, session.physicalIndex)
			} else {
				report.Deleted = append(report.Deleted, session.physicalIndex)
			}
		}
		if len(hits) < 128 {
			if cursor.RecoveryAfter != "" {
				return report, e.saveCandidateCursor(ctx, cursorID, "", cursorVersion)
			}
			return report, nil
		}
		next, err := resourceNextCursor(hits, after, len(sort))
		if err != nil {
			return report, err
		}
		after = next
	}
	report.More = true
	next := after[0].(string)
	if instance {
		encoded, err := json.Marshal(after)
		if err != nil {
			return report, err
		}
		next = string(encoded)
	}
	return report, e.saveCandidateCursor(ctx, cursorID, next, cursorVersion)
}

// Checkpoint a bounded pass even when unresolved admissions fill early pages.
// A restarted coordinator resumes beyond them instead of starving later work.
func (e *openSearchEngine) saveCandidateCursor(ctx context.Context, id, after string, expected *controlVersion) error {
	_, err := e.putControlRecordContext(ctx, id, controlDoc{
		Repository: string(e.repository), State: "RESOURCE_RECOVERY", RecoveryAfter: after,
	}, expected)
	if kernel.CodeOf(err) == kernel.ErrNonFastForward {
		return nil
	} // Another pass checkpointed concurrently.
	return err
}
