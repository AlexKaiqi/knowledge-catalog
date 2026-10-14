package access

// Repository operations concern native metadata, snapshot files and their version graph.
// They do not imply Object/Writer permissions or authority to grant access.
const (
	RepositoryMetadataRead   = "repository.metadata.read"
	RepositoryMetadataWrite  = "repository.metadata.write"
	RepositoryHistoryRead    = "repository.history.read"
	RepositoryFileRead       = "file.read"
	RepositoryFilesWrite     = "repository.files.write"
	RepositoryCommit         = "repository.commit"
	RepositoryRefsManage     = "repository.refs.manage"
	RepositoryMerge          = "repository.merge"
	RepositorySettingsRead   = "repository.settings.read"
	RepositorySettingsManage = "repository.settings.manage"
	RepositoryActionsRead    = "repository.actions.read"
	RepositoryPullsRead      = "repository.pulls.read"
	RepositoryPullsManage    = "repository.pulls.manage"
	RepositoryImport         = "repository.import"
	RepositoryCreate         = "repository.create"
	RepositoryDelete         = "repository.delete"
	// RepositoryDatasetPublish permits selecting source content for a Dataset
	// publication. It is independent of ordinary file reads and native writes.
	RepositoryDatasetPublish = "repository.dataset.publish"
)

// IsRepositoryAction is the closed set used by native API assembly. The
// general rule matcher remains open to application-owned semantic namespaces.
func IsRepositoryAction(action string) bool {
	switch action {
	case RepositoryMetadataRead, RepositoryMetadataWrite, RepositoryHistoryRead, RepositoryFileRead,
		RepositoryFilesWrite, RepositoryCommit, RepositoryRefsManage, RepositoryMerge,
		RepositorySettingsRead, RepositorySettingsManage, RepositoryActionsRead,
		RepositoryPullsRead, RepositoryPullsManage, RepositoryImport,
		RepositoryCreate, RepositoryDelete:
		return true
	default:
		return false
	}
}

// Creator/Sharing policies may explicitly choose these bounded Repo actions.
// Allocation, physical deletion and external import require separate grants.
func IsRepositoryCreatorAction(action string) bool {
	return action == RepositoryDatasetPublish || IsRepositoryAction(action) && action != RepositoryCreate && action != RepositoryDelete && action != RepositoryImport
}

func IsRepositoryReadAction(action string) bool {
	switch action {
	case RepositoryMetadataRead, RepositoryHistoryRead, RepositoryFileRead,
		RepositorySettingsRead, RepositoryActionsRead, RepositoryPullsRead:
		return true
	default:
		return false
	}
}

// MatchRepository uses the same allow matcher, excluding scopes that do not
// authorize raw Repo operations. In particular Object "*" is not a file ACL.
func MatchRepository(rules []Rule, query Query) (Rule, bool) {
	for _, rule := range rules {
		if rule.Dataset != "" || rule.Object != "" || rule.Aspect != "" {
			continue
		}
		if matched, ok := Match([]Rule{rule}, query); ok {
			return matched, true
		}
	}
	return Rule{}, false
}
