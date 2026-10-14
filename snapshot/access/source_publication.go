package access

// MatchSourcePublication selects explicit source Repository authority, with an
// optional destination Dataset intersection. A Dataset-only rule cannot share
// private source content, and Object/Aspect qualifiers do not become file ACLs.
// Ref is the selected commit when explicitly fixed, otherwise the selector.
func MatchSourcePublication(rules []Rule, query Query) (Rule, bool) {
	if query.Action != RepositoryDatasetPublish || query.Repo == "" || query.Dataset == "" || query.Ref == "" {
		return Rule{}, false
	}
	for _, rule := range rules {
		if rule.Object != "" || rule.Aspect != "" || rule.Repo == "" && rule.Dataset != "" {
			continue
		}
		if matched, ok := Match([]Rule{rule}, query); ok {
			return matched, true
		}
	}
	return Rule{}, false
}
