package delivery

import "kc/knowledge"

// RepositoryRead is the selected first delivery stage: without knowledge.read
// the caller keeps the knowledge ID and loses Canonical body.
type RepositoryRead struct {
	Allowed func(principal string, ref knowledge.KnowledgeRef) bool
}

func (s RepositoryRead) Apply(ctx Context, env Envelope) (Envelope, error) {
	if s.Allowed != nil && s.Allowed(ctx.Principal, env.ID.KnowledgeRef) {
		return env, nil
	}
	env.Value = nil
	env.Provenance = nil
	env.Observations = nil
	env.Units = nil
	env.Declarations = nil
	return env, nil
}
