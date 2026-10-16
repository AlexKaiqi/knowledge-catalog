package projection

import (
	retrieval "kc/discovery"
	objectmodel "kc/object"
	"kc/object/reader"
)

func (s datasetSpace) memberEvidence(value objectmodel.ObjectValue, request retrieval.SearchRequest, observations []objectmodel.UnitObservation) ([]retrieval.DeclarationParticipation, error) {
	fields, err := retrieval.ExtractFields(s.member, value, observations, s.spec)
	if err != nil {
		return nil, err
	}
	contributed := map[string]bool{}
	for _, field := range fields.Values {
		if field.Field.Has(reader.HintText) {
			contributed[field.Member] = true
		}
	}
	if request.Recall == retrieval.RecallSemantic {
		clear(contributed)
		for _, field := range fields.Semantic {
			contributed[field.Member] = true
		}
	}
	roles := map[string][]string{}
	for _, clause := range retrieval.SearchClauses(request) {
		role := "filter"
		if clause.Op == retrieval.OpSort {
			role = "sort"
		}
		if clause.Op == retrieval.OpMatch {
			role = "text"
			if request.Recall == retrieval.RecallSemantic {
				role = "semantic"
			}
		}
		if clause.Member != "" {
			roles[clause.Member] = append(roles[clause.Member], role)
			continue
		}
		if clause.Op == retrieval.OpMatch {
			for member := range contributed {
				roles[member] = append(roles[member], role)
			}
		}
	}
	out := []retrieval.DeclarationParticipation{}
	for _, status := range s.spec.Statuses() {
		seen := map[string]bool{}
		for _, role := range roles[objectmodel.AddressKey(status.Member)] {
			if seen[role] {
				continue
			}
			seen[role] = true
			out = append(out, retrieval.DeclarationParticipation{Repository: status.Repository, Commit: status.Commit, Member: status.Member, Role: role})
		}
	}
	return out, nil
}
