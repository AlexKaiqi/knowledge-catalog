package cli

import (
	"fmt"
	"strings"
	"unicode"

	"kc/internal/journal"
)

// principalOf is who this command acts as. No --as means the machine owner,
// who is not subject to allow.json.
func principalOf(flags map[string]FlagValue) string {
	as := strings.TrimSpace(FlagString(flags, "as"))
	if as == "" {
		return ownerPrincipal
	}
	return as
}

func requestIDFrom(flags map[string]FlagValue) (string, error) {
	raw := strings.TrimSpace(FlagString(flags, "request-id"))
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

func matchedRuleID(home, command string, flags map[string]FlagValue) string {
	if FlagString(flags, "as") == "" {
		return ""
	}
	file, err := ReadAllow(home)
	if err != nil {
		return ""
	}
	// Mirror authorize: an explicit operand, then the Home's current Catalog,
	// then the single default Catalog. A rule scoped to the current Catalog must
	// still be named when the caller no longer repeats --catalog.
	catalogID := FlagString(flags, "catalog")
	if catalogID == "" {
		catalogID = FlagString(flags, "_default-catalog")
	}
	catalogID = defaultAllowCatalog(home, catalogID)
	action := actionOf(command, flags)
	rule, ok := MatchAllow(file.Rules, AllowQuery{
		Principal: FlagString(flags, "as"),
		Action:    action,
		Repo:      FlagString(flags, "repo"),
		Catalog:   catalogID,
		Ref:       FlagString(flags, "ref"),
		Object:    FlagString(flags, "object"),
		Aspect:    FlagString(flags, "aspect"),
		Workspace: workspaceIDOf(flags),
	})
	if !ok {
		return ""
	}
	return rule.ID
}

func observeHome(ws *Home, command string, flags map[string]FlagValue) {
	if ws == nil {
		return
	}
	req, err := requestIDFrom(flags)
	if err != nil {
		return
	}
	identity, err := identityContextFrom(flags)
	if err != nil {
		return
	}
	trace, err := traceContextFrom(flags)
	if err != nil {
		return
	}
	as := identity.Principal
	rule := matchedRuleID(ws.Dir, command, flags)
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
	for _, cat := range ws.Catalogs {
		if cat != nil {
			cat.SetStamp(as, req, rule)
		}
	}
}

// requestHomeJournal starts from the durable journal sink, never a previous
// request's stamped wrapper. Empty optional identity fields cannot inherit
// an earlier request's delegation or trace context.
func requestHomeJournal(home, command string, flags map[string]FlagValue) (journal.Journal, error) {
	req, err := requestIDFrom(flags)
	if err != nil {
		return nil, err
	}
	identity, err := identityContextFrom(flags)
	if err != nil {
		return nil, err
	}
	trace, err := traceContextFrom(flags)
	if err != nil {
		return nil, err
	}
	return journal.WithContext(journal.NewFile(systemPath(home)), journal.Stamp{
		Principal: identity.Principal, OnBehalfOf: identity.OnBehalfOf, RequestID: req,
		TraceID: trace.TraceID, SpanID: trace.SpanID, ParentSpanID: trace.ParentSpanID,
		RuleID: matchedRuleID(home, command, flags),
	}), nil
}
