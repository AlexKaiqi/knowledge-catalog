package gitdir

// BranchRef is the full ref path of a branch name.
func BranchRef(name string) string { return "refs/heads/" + name }
