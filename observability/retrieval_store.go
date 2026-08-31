package observability

func (s *FileStore) Retrieval(query RetrievalQuery) ([]RetrievalEvent, error) {
	all, err := readJSONL[RetrievalEvent](s.RetrievalPath)
	if err != nil {
		return nil, err
	}
	out := make([]RetrievalEvent, 0, len(all))
	for _, event := range all {
		if query.EvidenceID != "" && event.EvidenceID != query.EvidenceID {
			continue
		}
		if query.TraceID != "" && event.Trace.TraceID != query.TraceID {
			continue
		}
		if query.Operator != "" && event.Operator != query.Operator {
			continue
		}
		if query.Outcome != "" && event.Outcome != query.Outcome {
			continue
		}
		if query.Provider != "" && !retrievalUsedProvider(event, query.Provider) {
			continue
		}
		out = append(out, event)
	}
	if query.Limit > 0 && len(out) > query.Limit {
		out = out[len(out)-query.Limit:]
	}
	return out, nil
}

func retrievalUsedProvider(event RetrievalEvent, provider string) bool {
	for _, candidate := range event.Candidates {
		for _, lane := range candidate.Evidence {
			if lane.Provider == provider {
				return true
			}
		}
	}
	return false
}
