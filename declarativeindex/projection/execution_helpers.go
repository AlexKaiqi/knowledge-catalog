package projection

import (
	"context"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/kernel"
)

func markSearchBudget(result *retrieval.SearchResult, reason string) {
	result.Completeness = retrieval.CompletenessPartial
	result.Stats.MarkPartial("budget")
	result.Claims = append(result.Claims, "search execution budget exhausted: "+reason)
}

func searchPreparationError(ctx context.Context, err error) error {
	if err != nil && ports.SearchBudgetExhausted(ctx) {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "search execution time budget exhausted before a fixed retrieval plan was established")
	}
	return err
}
