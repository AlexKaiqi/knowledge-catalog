package repofile

import (
	"regexp"

	"kc/internal/treepath"
	"kc/kernel"
	"kc/knowledge"
)

var knowledgeFile = regexp.MustCompile(`(?i)\.(json|md|ya?ml|txt)$`)

// KnowledgePath is true when a tree path may hold a knowledge unit.
func KnowledgePath(rel string) bool {
	return knowledgeFile.MatchString(rel)
}

func DefaultPath(address knowledge.Address) string {
	if address.MemberKey != "" && address.AspectName != "" {
		return "objects/" + string(address.ObjectID) + "/" + address.AspectName + "/" + address.MemberKey + ".json"
	}
	if address.AspectName != "" {
		return "objects/" + string(address.ObjectID) + "/" + address.AspectName + ".json"
	}
	return "objects/" + string(address.ObjectID) + ".json"
}

func SafeRelativePath(value string) (string, error) {
	return treepath.Clean(value)
}

func EntityPathHint(units []Unit, objectID knowledge.ObjectID) string {
	if len(units) == 1 {
		if units[0].PathHint != "" {
			return units[0].PathHint
		}
		return units[0].Path
	}
	if len(units) == 0 {
		return ""
	}
	return "objects/" + string(objectID)
}

func AssertLayout(units []Unit, incoming knowledge.Address) error {
	if len(units) == 0 {
		return nil
	}
	hasBlob, hasAspect := false, false
	for _, u := range units {
		if knowledge.IsEntityBlob(u.Address) {
			hasBlob = true
		} else {
			hasAspect = true
		}
	}
	if hasBlob && hasAspect {
		return kernel.Fail(kernel.ErrObjectIDConflict, "%s mixes entity blob and aspects", incoming.ObjectID)
	}
	if knowledge.IsEntityBlob(incoming) && hasAspect {
		return kernel.Fail(kernel.ErrObjectIDConflict, "cannot PUT an entity blob on %s; object already has aspects", incoming.ObjectID)
	}
	if !knowledge.IsEntityBlob(incoming) && hasBlob {
		return kernel.Fail(kernel.ErrObjectIDConflict, "cannot PUT an aspect on %s; object is an entity blob", incoming.ObjectID)
	}
	return nil
}
