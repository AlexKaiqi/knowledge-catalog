package invoke

// Stage is how much of the local workspace a verb needs before it runs. It is
// the only axis on which verbs differ before their own body starts, so it is
// declared per verb instead of re-derived by an if-chain in the dispatcher.
// (moved from cli/command.go, QUALITY-05 S3')
type Stage int

const (
	// StageHome resolves --home and nothing else. The verb opens what it needs,
	// which is how `audit` and `store-ls` still answer when Repository
	// attachment fails.
	StageHome Stage = iota

	// StageOpen opens this --home's object graph (stores, catalogs, writer)
	// and records the call. Used by the verbs that shape the home itself,
	// before any grant exists.
	StageOpen

	// StageGoverned mounts the workspace, checks allow.json, binds the control
	// plane and wraps the body in pre/post hooks. Every protocol verb is here.
	StageGoverned
)

// BindingScope is the selector triple shared by the policy verbs.
type BindingScope struct {
	On      string
	Repo    string
	Catalog string
}

// Matches reports whether the scope selects the given coordinate: an empty
// selector matches everything.
func (s BindingScope) Matches(on, repo, catalogID string) bool {
	if s.On != "" && s.On != on {
		return false
	}
	if s.Repo != "" && s.Repo != repo {
		return false
	}
	if s.Catalog != "" && s.Catalog != catalogID {
		return false
	}
	return true
}
