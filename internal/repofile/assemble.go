package repofile

import (
	"kc/kernel"
	"kc/knowledge"
)

func Assemble(units []Unit) (any, error) {
	if len(units) == 0 {
		return nil, nil
	}
	var blobs, parts []Unit
	for _, u := range units {
		if knowledge.IsEntityBlob(u.Address) {
			blobs = append(blobs, u)
		} else {
			parts = append(parts, u)
		}
	}
	if len(blobs) > 0 && len(parts) > 0 {
		return nil, kernel.Fail(kernel.ErrObjectIDConflict, "%s mixes entity blob and aspects", units[0].ObjectID)
	}
	if len(blobs) > 1 {
		return nil, kernel.Fail(kernel.ErrObjectIDConflict, "duplicate object_id %s", blobs[0].ObjectID)
	}
	if len(blobs) == 1 {
		return blobs[0].Value, nil
	}
	recordNames := map[string]struct{}{}
	memberNames := map[string]struct{}{}
	out := map[string]any{}
	members := map[string]map[string]any{}
	for _, unit := range parts {
		name := unit.Address.AspectName
		if name == "" {
			continue
		}
		if unit.Address.MemberKey != "" {
			memberNames[name] = struct{}{}
			bucket := members[name]
			if bucket == nil {
				bucket = map[string]any{}
				members[name] = bucket
			}
			bucket[unit.Address.MemberKey] = unit.Value
		} else {
			recordNames[name] = struct{}{}
			out[name] = unit.Value
		}
	}
	for name := range memberNames {
		if _, ok := recordNames[name]; ok {
			return nil, kernel.Fail(kernel.ErrObjectIDConflict, "aspect %s is both Record and Member", name)
		}
		out[name] = members[name]
	}
	return out, nil
}
