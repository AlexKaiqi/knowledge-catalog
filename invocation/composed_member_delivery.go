package invocation

import (
	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"
)

func objectValueDeliveryAllowed(home string, flags map[string]invoke.FlagValue, value objectmodel.ObjectValue, single objectmodel.ObjectRef) bool {
	if value.Repository == "" && len(value.Sources) > 0 && (value.ObjectRef.Repository != "" || value.Commit != "") {
		return false
	}
	return AllowedRepoRead(home, flags, string(single.Repository), string(single.Object)) || composedMemberDeliveryAllowed(home, flags, value)
}

// A composed hit has a source vector instead of one Repository. Only an
// already accepted named scope can authorize it through complete member grants;
// the coordinate carrier never replaces the current authorization decision.
func composedMemberDeliveryAllowed(home string, flags map[string]invoke.FlagValue, value objectmodel.ObjectValue) bool {
	if value.Repository != "" || value.Commit != "" || value.ObjectRef.Repository != "" ||
		value.ObjectRef.Object == "" || value.ObjectRef.Object != value.Address.ObjectID || len(value.Sources) < 2 {
		return false
	}
	scope := namedConsumptionScopeOf(flags, nil)
	if scope == nil || scope.pin == nil {
		return false
	}
	pin, allowed, err := namedDatasetMemberConsumption(home, flags, nil)
	if err != nil || !allowed {
		return false
	}
	body, ok := value.Value.(map[string]any)
	if !ok || len(body) == 0 {
		return false
	}
	repositories := map[kernel.RepositoryID]bool{}
	aspects := map[string]bool{}
	for _, source := range value.Sources {
		if source.Repository == "" || source.Commit == "" || repositories[source.Repository] ||
			pin.Repositories[source.Repository] != source.Commit || len(source.Aspects) == 0 {
			return false
		}
		repositories[source.Repository] = true
		for _, aspect := range source.Aspects {
			if _, present := body[aspect]; aspect == "" || !present || aspects[aspect] {
				return false
			}
			aspects[aspect] = true
		}
	}
	return len(aspects) == len(body)
}
