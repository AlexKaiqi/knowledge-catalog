package observability

import (
	"kc/knowledge"
	"sort"
	"strings"
)

func (s *FileStore) Hitmap(query AccessQuery) ([]HitmapEntry, error) {
	events, err := s.matchingAccess(query)
	if err != nil {
		return nil, err
	}
	byKey := map[string]*HitmapEntry{}
	for _, event := range events {
		if event.Decision != "ALLOW" || event.Result != "RESOLVED" {
			continue
		}
		seen := map[string]bool{}
		for _, target := range event.Knowledge {
			ref := target.KnowledgeRef
			key := strings.Join([]string{string(ref.Repository), string(ref.Commit), string(ref.Object), addressKey(target.Address)}, "\x00")
			if seen[key] {
				continue
			}
			seen[key] = true
			hit := byKey[key]
			if hit == nil {
				hit = &HitmapEntry{
					KnowledgeRef: ref, Address: target.Address,
					FirstAccessedAt: event.OccurredAt, LastAccessedAt: event.OccurredAt,
					Principals: map[string]int{}, OnBehalfOf: map[string]int{},
				}
				byKey[key] = hit
			}
			hit.Hits++
			if occurredBefore(event.OccurredAt, hit.FirstAccessedAt) {
				hit.FirstAccessedAt = event.OccurredAt
			}
			if occurredBefore(hit.LastAccessedAt, event.OccurredAt) {
				hit.LastAccessedAt = event.OccurredAt
			}
			hit.Principals[event.Identity.Principal]++
			if event.Identity.OnBehalfOf != "" {
				hit.OnBehalfOf[event.Identity.OnBehalfOf]++
			}
		}
	}
	out := make([]HitmapEntry, 0, len(byKey))
	for _, hit := range byKey {
		if len(hit.OnBehalfOf) == 0 {
			hit.OnBehalfOf = nil
		}
		out = append(out, *hit)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Hits != out[j].Hits {
			return out[i].Hits > out[j].Hits
		}
		a, b := out[i].KnowledgeRef, out[j].KnowledgeRef
		return string(a.Repository)+string(a.Object)+string(a.Commit) < string(b.Repository)+string(b.Object)+string(b.Commit)
	})
	return out, nil
}

func addressKey(address *knowledge.Address) string {
	if address == nil {
		return ""
	}
	return knowledge.AddressKey(*address)
}
