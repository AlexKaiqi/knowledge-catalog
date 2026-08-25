package index

import (
	"kc/kernel"
	"kc/knowledge"
	"kc/reader"
)

func specAtCommit(repo knowledge.Repository, commit kernel.CommitID) (reader.AccessSpec, error) {
	report, err := reader.DescribeRepoSchema(repo, commit, "")
	if err != nil {
		return reader.AccessSpec{}, err
	}
	return reader.AccessSpecFromReport(report), nil
}
