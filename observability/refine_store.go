package observability

func (s *FileStore) Refine(query RefineQuery) ([]RefineEvent, error) {
	all, err := readJSONL[RefineEvent](s.RefinePath)
	if err != nil {
		return nil, err
	}
	out := make([]RefineEvent, 0, len(all))
	for _, event := range all {
		if query.EvidenceID != "" && event.EvidenceID != query.EvidenceID {
			continue
		}
		if query.TraceID != "" && event.Trace.TraceID != query.TraceID {
			continue
		}
		if query.Outcome != "" && event.Outcome != query.Outcome {
			continue
		}
		if query.Provider != "" && (event.ModelOutput == nil || event.ModelOutput.Provider != query.Provider) {
			continue
		}
		if query.Model != "" && (event.ModelOutput == nil || event.ModelOutput.Model != query.Model) {
			continue
		}
		out = append(out, event)
	}
	// Audit limits select the newest matching evidence, consistent with access
	// log queries. Preserve chronological order within that tail window.
	if query.Limit > 0 && len(out) > query.Limit {
		out = out[len(out)-query.Limit:]
	}
	return out, nil
}
