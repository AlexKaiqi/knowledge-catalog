package gitdir

import "strings"

// BranchRef is the full ref path of a branch name.
func BranchRef(name string) string { return "refs/heads/" + name }

// BranchName is the branch name of a ref path.
func BranchName(ref string) string { return strings.TrimPrefix(ref, "refs/heads/") }
