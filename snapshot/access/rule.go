// Package access owns explicit principal/action/scope grants. It does not
// interpret file contents or application action namespaces.
package access

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

// Rule retains the established grant wire shape. Object and Aspect are opaque
// application qualifiers: neither is a file path or a knowledge identifier
// interpreted by this package. File selection belongs to Dataset definitions.
type Rule struct {
	ID        string   `json:"id"`
	Principal string   `json:"principal"`
	Actions   []string `json:"actions,omitempty"`
	Cmds      []string `json:"cmds,omitempty"` // application-owned legacy migration only
	Repo      string   `json:"repo,omitempty"`
	Catalog   string   `json:"catalog,omitempty"`
	Ref       string   `json:"ref,omitempty"`
	Object    string   `json:"object,omitempty"`
	Aspect    string   `json:"aspect,omitempty"`
	Dataset   string   `json:"dataset,omitempty"`
	ShareID   string   `json:"shareId,omitempty"`
	SharedBy  string   `json:"sharedBy,omitempty"`
}

type Query struct {
	Principal string
	Action    string
	Repo      string
	Catalog   string
	Ref       string
	Object    string
	Aspect    string
	Dataset   string
}

func (r *Rule) UnmarshalJSON(data []byte) error {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		return err
	}
	if _, ok := keys["kset"]; ok {
		return fmt.Errorf("retired field kset")
	}
	type wire Rule
	var parsed wire
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*r = Rule(parsed)
	for _, action := range r.Actions {
		if strings.HasPrefix(action, "kset.") {
			return fmt.Errorf("retired action %s", action)
		}
	}
	r.Actions = NormalizeActions(r.Actions)
	return nil
}

// NormalizeActions retains the existing on-read dataset.consume migration.
// Command-to-action mappings remain the caller application's responsibility.
func NormalizeActions(actions []string) []string {
	out := make([]string, 0, len(actions))
	seen := map[string]struct{}{}
	for _, action := range actions {
		if action == "dataset.consume" {
			action = "file.read"
		}
		if _, ok := seen[action]; ok {
			continue
		}
		seen[action] = struct{}{}
		out = append(out, action)
	}
	if len(out) == 0 {
		return actions
	}
	return out
}

func ValidateActions(actions []string) error {
	if len(actions) == 0 {
		return fmt.Errorf("grant add requires --action")
	}
	for _, action := range actions {
		if !strings.Contains(action, ".") || strings.ContainsAny(action, " /?#") {
			return fmt.Errorf("invalid semantic action %s", action)
		}
		if strings.HasPrefix(action, "kset.") || action == "dataset.consume" {
			return fmt.Errorf("invalid semantic action %s", action)
		}
	}
	return nil
}

func ActionMatches(granted, requested string) bool {
	return granted == "*" || granted == requested ||
		(strings.HasSuffix(granted, ".*") && strings.HasPrefix(requested, strings.TrimSuffix(granted, "*")))
}

func Match(rules []Rule, q Query) (Rule, bool) {
	for _, rule := range rules {
		if rule.Principal != q.Principal || !actionAllowed(rule.Actions, q.Action) {
			continue
		}
		if !scopeMatches(rule, q) {
			continue
		}
		if rule.Object != "" {
			ok, err := path.Match(rule.Object, q.Object)
			if err != nil || !ok {
				continue
			}
		}
		return rule, true
	}
	return Rule{}, false
}

func scopeMatches(rule Rule, q Query) bool {
	for _, coordinate := range [][2]string{{rule.Repo, q.Repo}, {rule.Catalog, q.Catalog}, {rule.Ref, q.Ref}, {rule.Aspect, q.Aspect}, {rule.Dataset, q.Dataset}} {
		if coordinate[0] != "" && coordinate[0] != coordinate[1] {
			return false
		}
	}
	return true
}

func actionAllowed(actions []string, requested string) bool {
	for _, action := range actions {
		if ActionMatches(action, requested) {
			return true
		}
	}
	return false
}
