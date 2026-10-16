package projection

import (
	"context"

	retrieval "kc/discovery"
	ports "kc/discovery/projection"
)

type residualVerifierContext interface {
	VerifyResidualMatchContext(context.Context, string, string, retrieval.MatchMode) (bool, error)
}

type contextualMatchVerifier struct {
	context  context.Context
	verifier residualVerifierContext
}

func (v contextualMatchVerifier) VerifyResidualMatch(text, query string, mode retrieval.MatchMode) (bool, error) {
	return v.verifier.VerifyResidualMatchContext(v.context, text, query, mode)
}

func matchVerifierForContext(ctx context.Context, engine ports.Engine) retrieval.ResidualMatchVerifier {
	if verifier, ok := engine.(residualVerifierContext); ok {
		return contextualMatchVerifier{context: ctx, verifier: verifier}
	}
	verifier, _ := engine.(retrieval.ResidualMatchVerifier)
	return verifier
}
