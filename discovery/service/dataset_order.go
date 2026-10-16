package service

import (
	retrieval "kc/discovery"
	"strings"
)

func bestWorkspaceHead(cursors []workspaceSearchCursor, req retrieval.SearchRequest) int {
	best := -1
	for i := range cursors {
		if cursors[i].head == nil {
			continue
		}
		if best < 0 || workspaceHitLess(*cursors[i].head, *cursors[best].head, req, cursors[i].sortType) {
			best = i
		}
	}
	return best
}

func workspaceHitLess(left, right retrieval.ObjectHit, req retrieval.SearchRequest, fieldTypes ...string) bool {
	fieldType := "string"
	if len(fieldTypes) > 0 {
		fieldType = fieldTypes[0]
	}
	order, sorted := workspaceSortOrder(req)
	if sorted {
		leftValue, leftOK := providerOrderValue(left)
		rightValue, rightOK := providerOrderValue(right)
		if leftOK != rightOK {
			return leftOK // missing values are last for both asc and desc
		}
		if leftOK {
			if cmp, err := retrieval.CompareScalarValues(fieldType, leftValue, rightValue); err == nil && cmp != 0 {
				if order == "desc" {
					return cmp > 0
				}
				return cmp < 0
			}
		}
	} else if workspaceHasMatch(req) {
		leftRank, rightRank := localRank(left), localRank(right)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
	}
	if left.Object.Repository != right.Object.Repository {
		return left.Object.Repository < right.Object.Repository
	}
	return left.Object.Address.ObjectID < right.Object.Address.ObjectID
}

func workspaceSortOrder(req retrieval.SearchRequest) (string, bool) {
	clause, ok := retrieval.SearchSortClause(req)
	if !ok {
		return "", false
	}
	order := strings.ToLower(strings.TrimSpace(clause.Order))
	if order == "" {
		order = "asc"
	}
	return order, true
}

func workspaceHasMatch(req retrieval.SearchRequest) bool {
	return retrieval.SearchHasOp(req, retrieval.OpMatch)
}

func providerOrderValue(hit retrieval.ObjectHit) (any, bool) {
	for _, evidence := range hit.Evidence {
		if len(evidence.ProviderOrder) > 0 && evidence.ProviderOrder[0] != nil {
			return evidence.ProviderOrder[0], true
		}
	}
	return nil, false
}

func localRank(hit retrieval.ObjectHit) int {
	for _, evidence := range hit.Evidence {
		if evidence.LocalRank > 0 {
			return evidence.LocalRank
		}
	}
	return int(^uint(0) >> 1)
}
