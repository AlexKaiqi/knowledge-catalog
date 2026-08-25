package observability

func (s *FileStore) Access(query AccessQuery) ([]AccessEvent, error) {
	all, err := readJSONL[AccessEvent](s.AccessPath)
	if err != nil {
		return nil, err
	}
	out := make([]AccessEvent, 0, len(all))
	for _, event := range all {
		if query.Principal != "" && event.Identity.Principal != query.Principal {
			continue
		}
		if query.OnBehalfOf != "" && event.Identity.OnBehalfOf != query.OnBehalfOf {
			continue
		}
		if query.Action != "" && event.Action != query.Action {
			continue
		}
		if query.TraceID != "" && event.Trace.TraceID != query.TraceID {
			continue
		}
		if (query.Repository != "" || query.Object != "") && !matchesKnowledge(event, query) {
			continue
		}
		out = append(out, event)
	}
	if query.Limit > 0 && len(out) > query.Limit {
		out = out[len(out)-query.Limit:]
	}
	return out, nil
}

func matchesKnowledge(event AccessEvent, query AccessQuery) bool {
	for _, target := range event.Knowledge {
		ref := target.KnowledgeRef
		if query.Repository != "" && ref.Repository != query.Repository {
			continue
		}
		if query.Object != "" && ref.Object != query.Object {
			continue
		}
		return true
	}
	return false
}
