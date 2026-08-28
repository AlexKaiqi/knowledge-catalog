package gitdir

import "strings"

// Default commit identity when a caller passes no author. Snapshot adapters do
// not carry a user database; the principal that matters is the kc `--as` stamp,
// which travels in the Request-Id / Rule-Id trailers.
const (
	DefaultAuthor  = "knowledge-catalog"
	DefaultEmail   = "kc@local"
	DefaultMessage = "commit"

	maxAuthorLen = 128

	trailerRequestID = "Request-Id"
	trailerRuleID    = "Rule-Id"
)

// Signature is the commit metadata kc puts on every write, on any backend.
// Format and ParseTrailers are inverses, so `kc audit` reads back what a
// COMMIT wrote whether the tree lived in Dolt or Gitea.
type Signature struct {
	Author    string
	Message   string
	RequestID string
	RuleID    string
}

// Format renders the git author name, email and message body with trailers.
func (s Signature) Format() (name, email, message string) {
	name = strings.TrimSpace(strings.ReplaceAll(s.Author, "\n", " "))
	if name == "" {
		name = DefaultAuthor
	}
	if len(name) > maxAuthorLen {
		name = name[:maxAuthorLen]
	}
	message = strings.TrimSpace(s.Message)
	if message == "" {
		message = DefaultMessage
	}
	var trailers []string
	if id := strings.TrimSpace(s.RequestID); id != "" {
		trailers = append(trailers, trailerRequestID+": "+id)
	}
	if id := strings.TrimSpace(s.RuleID); id != "" {
		trailers = append(trailers, trailerRuleID+": "+id)
	}
	if len(trailers) > 0 {
		message += "\n\n" + strings.Join(trailers, "\n")
	}
	return name, DefaultEmail, message
}

// ParseTrailers recovers the kc stamps from a commit message body.
func ParseTrailers(body string) (requestID, ruleID string) {
	for _, line := range strings.Split(body, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case strings.ToLower(trailerRequestID):
			requestID = strings.TrimSpace(value)
		case strings.ToLower(trailerRuleID):
			ruleID = strings.TrimSpace(value)
		}
	}
	return requestID, ruleID
}
