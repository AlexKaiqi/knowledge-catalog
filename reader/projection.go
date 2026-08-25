package reader

import (
	"encoding/json"
	"strings"

	"kc/kernel"
	"kc/repository"
)

type IndexDescriptor struct {
	BasisRepository kernel.RepositoryID `json:"basisRepository"`
	BasisCommit     kernel.CommitID     `json:"basisCommit"`
	ObjectCount     int                 `json:"objectCount"`
	HeadCommit      kernel.CommitID     `json:"headCommit"`
	LagBehindHead   bool                `json:"lagBehindHead"`
	AccessDigest    kernel.Digest       `json:"accessDigest,omitempty"`
	Mode            string              `json:"mode,omitempty"`
	Cause           string              `json:"cause,omitempty"`
}

type indexRow struct {
	objectID  kernel.ObjectID
	valueText string
}

// Projection is an Access Projection (design 7.3 / K-19): rebuildable, not
// Canonical, not a KnowledgeRef source. This FileGit profile is in-memory
// AND-contains (lexical). Hits hydrate from the pinned Canonical commit.
// Not EvaluationProjection (Refine field whitelist) and not AspectSelector.
type Projection struct {
	rows            []indexRow
	basisRepository kernel.RepositoryID
	basisCommit     kernel.CommitID
	selector        *repository.AspectSelector
}

func NewProjection() *Projection {
	return &Projection{}
}

func (p *Projection) Build(repo repository.Repository, commit kernel.CommitID, selector *repository.AspectSelector) error {
	p.selector = selector
	listed, err := repo.List(commit)
	if err != nil {
		return err
	}
	p.rows = p.rows[:0]
	for _, value := range listed {
		text := repository.SelectAspects(value.Value, value.Units, selector)
		b, _ := json.Marshal(text)
		p.rows = append(p.rows, indexRow{objectID: value.Address.ObjectID, valueText: string(b)})
	}
	p.basisRepository = repo.ID()
	p.basisCommit = commit
	return nil
}

func (p *Projection) Search(repo repository.Repository, query string) ([]repository.KnowledgeValue, error) {
	words := tokenize(query)
	var hits []repository.KnowledgeValue
	for _, row := range p.rows {
		if !allWords(row.valueText, words) {
			continue
		}
		value, err := repo.Read(row.objectID, p.basisCommit)
		if err != nil {
			return nil, err
		}
		if p.selector != nil {
			value.Value = repository.SelectAspects(value.Value, value.Units, p.selector)
		}
		hits = append(hits, value)
	}
	return hits, nil
}

func (p *Projection) DescribeIndex(repo repository.Repository) (IndexDescriptor, error) {
	head, err := repo.Head("")
	if err != nil {
		return IndexDescriptor{}, err
	}
	return IndexDescriptor{
		BasisRepository: p.basisRepository,
		BasisCommit:     p.basisCommit,
		ObjectCount:     len(p.rows),
		HeadCommit:      head,
		LagBehindHead:   head != p.basisCommit,
	}, nil
}

func tokenize(query string) []string {
	var words []string
	for _, w := range strings.Fields(strings.TrimSpace(query)) {
		if w != "" {
			words = append(words, strings.ToLower(w))
		}
	}
	return words
}

func allWords(text string, words []string) bool {
	lower := strings.ToLower(text)
	for _, w := range words {
		if !strings.Contains(lower, w) {
			return false
		}
	}
	return len(words) > 0
}
