package invocation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	kchome "kc/home"
	"kc/internal/journal"
	"kc/invoke"
)

// PrincipalOf is who this command acts as. No --as means the machine owner,
// who is not subject to allow.json.
func PrincipalOf(flags map[string]invoke.FlagValue) string {
	as := strings.TrimSpace(invoke.FlagString(flags, "as"))
	if as == "" {
		return OwnerPrincipal
	}
	return as
}

func RequestIDFrom(flags map[string]invoke.FlagValue) (string, error) {
	raw := strings.TrimSpace(invoke.FlagString(flags, "request-id"))
	if raw == "" {
		return "", nil
	}
	if len(raw) > 128 {
		return "", fmt.Errorf("--request-id is too long")
	}
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' || r == ':' {
			continue
		}
		return "", fmt.Errorf("--request-id must be a correlation token")
	}
	return raw, nil
}

func MatchedRuleID(home, command string, flags map[string]invoke.FlagValue) string {
	if invoke.FlagString(flags, "as") == "" {
		return ""
	}
	file, err := ReadAllow(home)
	if err != nil {
		return ""
	}
	action := authorizationAction(command, flags)
	rule, ok := MatchAllow(withDatasetDefaults(home, invoke.FlagString(flags, "as"), file.Rules), AllowQuery{
		Principal: invoke.FlagString(flags, "as"),
		Action:    action,
		Repo:      invoke.FlagString(flags, "repo"),
		Ref:       invoke.FlagString(flags, "ref"),
		Object:    invoke.FlagString(flags, "object"),
		Aspect:    invoke.FlagString(flags, "aspect"),
		Dataset:   invoke.DatasetOf(flags),
	})
	if !ok {
		return ""
	}
	return rule.ID
}

func ObserveHome(ws *kchome.Home, command string, flags map[string]invoke.FlagValue) {
	if ws == nil {
		return
	}
	req, err := RequestIDFrom(flags)
	if err != nil {
		return
	}
	identity, err := IdentityContextFrom(flags)
	if err != nil {
		return
	}
	trace, err := TraceContextFrom(flags)
	if err != nil {
		return
	}
	as := identity.Principal
	rule := MatchedRuleID(ws.Dir, command, flags)
	ws.SetJournal(journal.WithContext(ws.Journal, journal.Stamp{
		Principal: as, OnBehalfOf: identity.OnBehalfOf, RequestID: req,
		TraceID: trace.TraceID, SpanID: trace.SpanID, ParentSpanID: trace.ParentSpanID,
		RuleID: rule,
	}))
	if ws.Writer != nil {
		ws.Writer.SetStamp(as, req, rule)
	}
	if ws.TreeWriter != nil {
		ws.TreeWriter.SetStamp(as, req, rule)
	}
	{
		cat := ws.Datasets
		if cat != nil {
			cat.SetStamp(as, req, rule)
		}
	}
}

// RequestHomeJournal starts from the durable journal sink, never a previous
// request's stamped wrapper. Empty optional identity fields cannot inherit
// an earlier request's delegation or trace context.
func RequestHomeJournal(home, command string, flags map[string]invoke.FlagValue) (journal.Journal, error) {
	req, err := RequestIDFrom(flags)
	if err != nil {
		return nil, err
	}
	identity, err := IdentityContextFrom(flags)
	if err != nil {
		return nil, err
	}
	trace, err := TraceContextFrom(flags)
	if err != nil {
		return nil, err
	}
	return journal.WithContext(journal.NewFile(systemPath(home)), journal.Stamp{
		Principal: identity.Principal, OnBehalfOf: identity.OnBehalfOf, RequestID: req,
		TraceID: trace.TraceID, SpanID: trace.SpanID, ParentSpanID: trace.ParentSpanID,
		RuleID: MatchedRuleID(home, command, flags),
	}), nil
}

// ActionOf identifies the resolved operation in access, retrieval and audit
// evidence. Consumption permission stays separate: searching a Dataset and
// reading one of its files both use file.read, but they are different actions.
func ActionOf(command string, flags map[string]invoke.FlagValue) string {
	switch command {
	case "object-search":
		return "object.search"
	case "object-relations":
		return "object.relations"
	case "object-traverse":
		return "object.traverse"
	case "operations-access-spec-describe":
		return "object.access.describe"
	case "operations-projection-describe":
		return "projection.read"
	case "operations-projection-sync":
		return "projection.manage"
	}
	return authorizationAction(command, flags)
}

func authorizationAction(command string, flags map[string]invoke.FlagValue) string {
	if action := strings.TrimSpace(invoke.FlagString(flags, "_action")); action != "" {
		return action
	}
	return "internal." + command
}

// OwnerPrincipal is the implicit principal of a local Home connection with no
// --as flag; it is not subject to allow.json.
const OwnerPrincipal = "owner"

// DefaultHome is the fallback workspace directory when neither --home nor
// KC_HOME is set.
const DefaultHome = ".kc"

// kchome.ResolveHome resolves the workspace directory: --home, then KC_HOME, then
// DefaultHome, made absolute.
func ResolveHome(flags map[string]invoke.FlagValue) (string, error) {
	home := invoke.FlagString(flags, "home")
	if home == "" {
		home = strings.TrimSpace(os.Getenv("KC_HOME"))
		if home == "" {
			home = DefaultHome
		}
	}
	return filepath.Abs(home)
}
