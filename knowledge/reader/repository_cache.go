package reader

import (
	"container/list"
	"sync"

	"kc/kernel"
	"kc/knowledge"
)

const defaultCanonicalCacheEntries = 2048

type canonicalKey struct {
	repository kernel.RepositoryID
	commit     kernel.CommitID
	objectID   knowledge.ObjectID
}

type canonicalEntry struct {
	key   canonicalKey
	value knowledge.KnowledgeValue
}

type canonicalCache struct {
	mu      sync.Mutex
	limit   int
	entries map[canonicalKey]*list.Element
	lru     *list.List
}

func newCanonicalCache(limit int) *canonicalCache {
	if limit < 1 {
		limit = 1
	}
	return &canonicalCache{limit: limit, entries: map[canonicalKey]*list.Element{}, lru: list.New()}
}

func (c *canonicalCache) get(key canonicalKey) (knowledge.KnowledgeValue, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.entries[key]
	if !ok {
		return knowledge.KnowledgeValue{}, false
	}
	c.lru.MoveToFront(element)
	return cloneKnowledgeValue(element.Value.(canonicalEntry).value), true
}

func (c *canonicalCache) put(key canonicalKey, value knowledge.KnowledgeValue) {
	value = cloneKnowledgeValue(value)
	c.mu.Lock()
	defer c.mu.Unlock()
	if element, ok := c.entries[key]; ok {
		element.Value = canonicalEntry{key: key, value: value}
		c.lru.MoveToFront(element)
		return
	}
	element := c.lru.PushFront(canonicalEntry{key: key, value: value})
	c.entries[key] = element
	for c.lru.Len() > c.limit {
		oldest := c.lru.Back()
		entry := oldest.Value.(canonicalEntry)
		delete(c.entries, entry.key)
		c.lru.Remove(oldest)
	}
}

// KnowledgeValue contains maps, slices and pointers. Snapshot values are
// immutable, so cache callers must not be able to mutate a later caller's
// result through shared references.
func cloneKnowledgeValue(value knowledge.KnowledgeValue) knowledge.KnowledgeValue {
	cloned := value
	cloned.Value = cloneJSONValue(value.Value)
	cloned.Units = append([]knowledge.Address(nil), value.Units...)
	cloned.Declarations = append([]knowledge.UnitDeclaration(nil), value.Declarations...)
	if value.Provenance != nil {
		provenance := *value.Provenance
		provenance.SourceRefs = append([]string(nil), value.Provenance.SourceRefs...)
		provenance.EvidenceRefs = append([]string(nil), value.Provenance.EvidenceRefs...)
		if value.Provenance.Algorithm != nil {
			algorithm := *value.Provenance.Algorithm
			provenance.Algorithm = &algorithm
		}
		cloned.Provenance = &provenance
	}
	for i := range cloned.Declarations {
		cloned.Declarations[i].ValueSource = cloneValueSource(value.Declarations[i].ValueSource)
	}
	return cloned
}

func cloneJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = cloneJSONValue(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = cloneJSONValue(item)
		}
		return out
	default:
		return value
	}
}

func cloneValueSource(source *knowledge.ValueSource) *knowledge.ValueSource {
	if source == nil {
		return nil
	}
	out := *source
	if source.Binding != nil {
		binding := *source.Binding
		if source.Binding.Operations != nil {
			binding.Operations = make(map[string]knowledge.BindingOperation, len(source.Binding.Operations))
			for name, operation := range source.Binding.Operations {
				binding.Operations[name] = operation
			}
		}
		out.Binding = &binding
	}
	return &out
}
