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
	command = consumerAllowCmd(command, flags)
	rule, ok := MatchAllow(file.Rules, AllowQuery{
		Principal: FlagString(flags, "as"),
		Cmd:       command,
		Repo:      FlagString(flags, "repo"),
		Catalog:   FlagString(flags, "catalog"),
		Ref:       FlagString(flags, "ref"),
		Object:    FlagString(flags, "object"),
		Aspect:    FlagString(flags, "aspect"),
		Stream:    FlagString(flags, "stream"),
		Workspace: FlagString(flags, "workspace"),
	})
	if !ok {
		return ""
	}
	return rule.ID
}

func (ws *Home) observe(command string, flags map[string]FlagValue) {
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
	ws.setJournal(journal.WithContext(ws.Journal, journal.Stamp{
		Principal: as, OnBehalfOf: identity.OnBehalfOf, RequestID: req,
		TraceID: trace.TraceID, SpanID: trace.SpanID, ParentSpanID: trace.ParentSpanID,
		SessionID: trace.SessionID, RuleID: rule,
	}))
	if ws.Writer != nil {
		ws.Writer.SetStamp(as, req, rule)
	}
	for _, cat := range ws.Catalogs {
		if cat != nil {
			cat.SetStamp(as, req, rule)
		}
	}
}

func (ws *Home) setJournal(j journal.Journal) {
	ws.Journal = j
	if ws.Writer != nil {
		ws.Writer.SetJournal(j)
	}
	if ws.Reader != nil {
		ws.Reader.SetJournal(j)
	}
	if ws.ControlPlane != nil {
		ws.ControlPlane.SetJournal(j)
	}
	for _, cat := range ws.Catalogs {
		if cat != nil {
			cat.SetJournal(j)
		}
	}
}
