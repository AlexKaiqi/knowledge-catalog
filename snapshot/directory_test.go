package snapshot_test

import (
	"testing"

	"kc/kernel"
	"kc/snapshot"
)

func TestDirectoryContinuationBindsBasisAndRejectsTampering(t *testing.T) {
	commit := kernel.CommitID("commit-a")
	token := snapshot.EncodeDirectoryCursor(snapshot.DirectoryCursor{Commit: commit, Directory: "docs", Position: "next"})
	cursor, err := snapshot.DecodeDirectoryCursor(token, commit, "docs")
	if err != nil || cursor.Position != "next" {
		t.Fatalf("decode: %#v %v", cursor, err)
	}
	if _, err := snapshot.DecodeDirectoryCursor(token, "commit-b", "docs"); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("basis mismatch: %v", err)
	}
	position := len(token) / 2
	replacement := "A"
	if token[position] == 'A' {
		replacement = "B"
	}
	tampered := token[:position] + replacement + token[position+1:]
	if _, err := snapshot.DecodeDirectoryCursor(tampered, commit, "docs"); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("tampered token: %v", err)
	}
}
