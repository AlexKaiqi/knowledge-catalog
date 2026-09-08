package knowledge

import "kc/kernel"

// Hydrator is the replaceable consumer-side exact Snapshot read seam. It reads
// stable declarations at the explicitly fixed commit, before dynamic Serving
// hydration and delivery authorization. Reader and Snapshot own no implementation
// state for this seam; upper retrieval/application assembly may inject a cache.
// Missing IDs are omitted exactly as in BatchReadStore. Every returned value
// must preserve the requested repository, object identity and immutable commit.
type Hydrator interface {
	ReadMany(repo Repository, commit kernel.CommitID, ids []ObjectID) (map[ObjectID]KnowledgeValue, error)
	ReadAddress(repo Repository, commit kernel.CommitID, address Address) (KnowledgeValue, error)
}
