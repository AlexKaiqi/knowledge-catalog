package invocation

import objectmodel "kc/object"

// A composed value has no single repository/commit. Its source vector and
// Aspect scopes describe the content actually adopted; withheld holders
// and business payloads must never become successful access targets.
func composedAccessEnvelopes(current map[string]any) []map[string]any {
	if StringValue(current["repository"]) != "" || StringValue(current["commit"]) != "" {
		return nil
	}
	var composed struct {
		ObjectRef objectmodel.ObjectRef
		Sources   []objectmodel.SourceBasis
	}
	if decodeMap(map[string]any{"objectRef": current["objectRef"], "sources": current["sources"]}, &composed) != nil || composed.ObjectRef.Object == "" {
		return nil
	}
	out := []map[string]any{}
	for _, source := range composed.Sources {
		if source.Repository == "" || source.Commit == "" {
			continue
		}
		for _, aspect := range source.Aspects {
			if aspect == "" {
				continue
			}
			address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: composed.ObjectRef.Object, AspectName: aspect}
			out = append(out, map[string]any{
				"reference": current["reference"], "locator": current["locator"],
				"repository": string(source.Repository), "commit": string(source.Commit),
				"objectId": string(composed.ObjectRef.Object), "address": JSONValue(address),
			})
		}
	}
	return out
}
