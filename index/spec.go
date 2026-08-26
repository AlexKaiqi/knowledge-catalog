package index

import (
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/retrieval"
)

func specAtCommit(repo knowledge.Repository, commit kernel.CommitID) (retrieval.AccessSpec, error) {
	report, err := reader.DescribeRepoSchema(repo, commit, "")
	if err != nil {
		return retrieval.AccessSpec{}, err
	}
	return retrieval.AccessSpecFromReport(report), nil
}
