package object

import (
	"sort"
	"strings"

	"kc/kernel"
)

// Holding is what one Repository in the request scope contributes to an
// object at a fixed commit: the names of the Aspects it maintains there, or
// WholeValue when it maintains the object as one complete value. Readable
// records whether the caller may read that Repository; unreadable holdings
// still take part in conflict detection but never contribute content.
type Holding struct {
	Repository kernel.RepositoryID
	Commit     kernel.CommitID
	Aspects    []string
	WholeValue bool
	Readable   bool
}

// HoldingSelection is the outcome of resolving one reference inside a scope.
// Selected holdings are readable and contribute content; Withheld names
// Repositories that hold relevant content the caller may not read.
type HoldingSelection struct {
	Selected []Holding
	Withheld []kernel.RepositoryID
}

// SourceBasis is one entry of a read's version vector: which Repository and
// commit supplied which Aspects of the object.
type SourceBasis struct {
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit"`
	Aspects    []string            `json:"aspects,omitempty"`
}

// HoldingOf derives one Repository's holding from its metadata-only plan of
// the root reference.
func HoldingOf(repository kernel.RepositoryID, commit kernel.CommitID, readable bool, units []Address) Holding {
	h := Holding{Repository: repository, Commit: commit, Readable: readable}
	seen := map[string]bool{}
	for _, unit := range units {
		if IsEntityBlob(unit) {
			h.WholeValue = true
			continue
		}
		if unit.AspectName != "" && !seen[unit.AspectName] {
			seen[unit.AspectName] = true
			h.Aspects = append(h.Aspects, unit.AspectName)
		}
	}
	sort.Strings(h.Aspects)
	return h
}

func (h Holding) holds(aspect string) bool {
	if h.WholeValue {
		return true
	}
	for _, name := range h.Aspects {
		if name == aspect {
			return true
		}
	}
	return false
}

// SelectHoldings applies the identity rule of the object model: one object ID
// in a scope is one object; each Repository holding it contributes Aspects;
// the same Aspect from two Repositories is a conflict that is reported, never
// merged or silently resolved.
func SelectHoldings(id ObjectID, locator string, holdings []Holding) (HoldingSelection, error) {
	tokens, err := PointerTokens(locator)
	if err != nil {
		return HoldingSelection{}, err
	}
	relevant := make([]Holding, 0, len(holdings))
	held := holdings[:0:0]
	for _, h := range holdings {
		if h.WholeValue || len(h.Aspects) > 0 {
			held = append(held, h)
		}
	}
	holdings = held
	for _, h := range holdings {
		if len(tokens) == 0 || h.holds(tokens[0]) {
			relevant = append(relevant, h)
		}
	}
	if len(relevant) == 0 {
		if len(holdings) == 0 {
			return HoldingSelection{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "object %s is not in the request scope", id)
		}
		return HoldingSelection{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "no Repository in the request scope holds %s#%s", id, locator)
	}
	if err := holdingConflict(id, relevant); err != nil {
		return HoldingSelection{}, err
	}
	out := HoldingSelection{}
	for _, h := range relevant {
		if h.Readable {
			out.Selected = append(out.Selected, h)
		} else {
			out.Withheld = append(out.Withheld, h.Repository)
		}
	}
	if len(out.Selected) == 0 {
		return HoldingSelection{}, kernel.Fail(kernel.ErrForbidden, "object %s is held only by Repositories the caller may not read", id)
	}
	return out, nil
}

func holdingConflict(id ObjectID, holdings []Holding) error {
	owner := map[string]Holding{}
	for _, h := range holdings {
		if h.WholeValue && len(holdings) > 1 {
			other := holdings[0]
			if other.Repository == h.Repository {
				other = holdings[1]
			}
			return conflictError(id, "(whole value)", h, other)
		}
		for _, aspect := range h.Aspects {
			if prior, exists := owner[aspect]; exists {
				return conflictError(id, aspect, prior, h)
			}
			owner[aspect] = h
		}
	}
	return nil
}

func conflictError(id ObjectID, aspect string, a, b Holding) error {
	names := []string{repositoryLabel(a), repositoryLabel(b)}
	sort.Strings(names)
	return kernel.Fail(kernel.ErrObjectIDConflict, "object %s aspect %s comes from two Repositories in the request scope: %s", id, aspect, strings.Join(names, ", "))
}

func repositoryLabel(h Holding) string {
	if !h.Readable {
		return "(a Repository the caller may not read)"
	}
	return string(h.Repository)
}

// ComposeHoldingValues joins the root values read from each selected holding
// into one object. Aspect ownership was already proven disjoint by
// SelectHoldings, so composition is a union, never a merge of one Aspect.
func ComposeHoldingValues(reference string, values []ObjectValue) (ObjectValue, error) {
	if len(values) == 0 {
		return ObjectValue{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "no holding was read")
	}
	if len(values) == 1 {
		return values[0], nil
	}
	composed := map[string]any{}
	out := ObjectValue{Reference: reference, ObjectRef: ObjectRef{Object: values[0].ObjectRef.Object}}
	out.Address = Address{Kind: KindEntity, ObjectID: values[0].ObjectRef.Object}
	for _, value := range values {
		body, ok := value.Value.(map[string]any)
		if !ok {
			return ObjectValue{}, kernel.Fail(kernel.ErrObjectIDConflict, "object %s is a whole value in %s and cannot be composed with other Repositories", out.ObjectRef.Object, value.Repository)
		}
		for key, item := range body {
			if _, exists := composed[key]; exists {
				return ObjectValue{}, kernel.Fail(kernel.ErrObjectIDConflict, "object %s aspect %s comes from two Repositories in the request scope", out.ObjectRef.Object, key)
			}
			composed[key] = item
		}
		out.Units = append(out.Units, value.Units...)
		out.Declarations = append(out.Declarations, value.Declarations...)
		out.Sources = append(out.Sources, SourceBasis{Repository: value.Repository, Commit: value.Commit, Aspects: sortedKeys(body)})
	}
	out.Value = composed
	out.Digest = kernel.CanonicalDigest(composed)
	return out, nil
}

func sortedKeys(body map[string]any) []string {
	keys := make([]string, 0, len(body))
	for key := range body {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
