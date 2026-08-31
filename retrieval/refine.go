package retrieval

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"kc/kernel"
	"kc/knowledge"
)

// Refine is optional Semantic Refinement (design 7.9). Ref-preserving:
// SEMANTIC_FILTER and SEMANTIC_RERANK. Not a base read. Not Derivation.

type FilterJudgment string

const (
	JudgmentMatch   FilterJudgment = "MATCH"
	JudgmentNoMatch FilterJudgment = "NO_MATCH"
	JudgmentUnknown FilterJudgment = "UNKNOWN"
)

type Candidate struct {
	Ref          knowledge.KnowledgeRef      `json:"ref"`
	Value        any                         `json:"value"`
	Observations []knowledge.UnitObservation `json:"observations,omitempty"`
	// OriginalRank and RetrievalEvidence are executor evidence. ExecuteRerank
	// commits them to the input digest but strips them from the model request so
	// the semantic judge is not biased by a physical lane's rank or score.
	OriginalRank      int            `json:"originalRank,omitempty"`
	RetrievalEvidence []LaneEvidence `json:"retrievalEvidence,omitempty"`
}

type SemanticFilterResult struct {
	Matched          []knowledge.KnowledgeRef `json:"matched"`
	Rejected         []knowledge.KnowledgeRef `json:"rejected"`
	Unknown          []knowledge.KnowledgeRef `json:"unknown"`
	Unjudged         []knowledge.KnowledgeRef `json:"unjudged"`
	Complete         bool                     `json:"complete"`
	TruncationReason string                   `json:"truncationReason,omitempty"`
}

type RankGroup struct {
	Rank int                      `json:"rank"`
	Refs []knowledge.KnowledgeRef `json:"refs"`
}

type SemanticRerankResult struct {
	SearchView       *SearchView              `json:"searchView,omitempty"`
	Groups           []RankGroup              `json:"groups"`
	NotSelected      []knowledge.KnowledgeRef `json:"notSelected,omitempty"`
	Unjudged         []knowledge.KnowledgeRef `json:"unjudged"`
	Complete         bool                     `json:"complete"`
	TruncationReason string                   `json:"truncationReason,omitempty"`
	Evidence         *SemanticRerankEvidence  `json:"evidence,omitempty"`
}

type FilterJudge func(candidate Candidate, criterion string) FilterJudgment
type RerankScorer func(candidate Candidate, criterion string) float64

const (
	MaxRerankCandidates     = 200
	MaxRerankInputBytes     = 128 << 10
	MaxRerankCandidateBytes = 32 << 10
)

// RerankRequest is the fixed-basis, authorized input sent to a semantic
// provider. Candidate values have already been Canonical-read and reduced by
// EvaluationProjection; providers never resolve refs or read repositories.
type RerankRequest struct {
	SearchView SearchView           `json:"searchView"`
	Spec       SemanticOperatorSpec `json:"spec"`
	Candidates []Candidate          `json:"candidates"`
}

// RerankProviderResult is deliberately rank-based. A provider may leave
// candidates unjudged, but every input ref must occur exactly once in Groups
// or Unjudged. Raw model scores are neither required nor exposed as globally
// comparable probabilities.
type RerankProviderResult struct {
	Groups         []RankGroup              `json:"groups"`
	Unjudged       []knowledge.KnowledgeRef `json:"unjudged,omitempty"`
	Provider       string                   `json:"provider"`
	Model          string                   `json:"model"`
	ModelRevision  string                   `json:"modelRevision,omitempty"`
	PromptRevision string                   `json:"promptRevision,omitempty"`
}

type SemanticRerankEvidence struct {
	RefineEvidenceID    string                    `json:"refineEvidenceId,omitempty"`
	Provider            string                    `json:"provider"`
	Model               string                    `json:"model"`
	ModelRevision       string                    `json:"modelRevision,omitempty"`
	PromptRevision      string                    `json:"promptRevision,omitempty"`
	SpecRef             string                    `json:"specRef"`
	SpecRevision        int                       `json:"specRevision"`
	CandidateDigest     kernel.Digest             `json:"candidateDigest"`
	CandidateCount      int                       `json:"candidateCount"`
	JudgedCount         int                       `json:"judgedCount"`
	ProjectedInputBytes int                       `json:"projectedInputBytes"`
	Candidates          []RerankCandidateEvidence `json:"candidates"`
}

// RerankExecutionRecord contains exactly the projected semantic input and the
// structured provider output needed by the application evidence recorder. It
// excludes credentials, transport payloads and chain-of-thought. A failed
// provider call still returns the prepared record for analysis.
type RerankExecutionRecord struct {
	SearchView          SearchView            `json:"searchView"`
	Spec                SemanticOperatorSpec  `json:"spec"`
	Candidates          []Candidate           `json:"candidates"`
	CandidateDigest     kernel.Digest         `json:"candidateDigest"`
	ProjectedInputBytes int                   `json:"projectedInputBytes"`
	ProviderDuration    time.Duration         `json:"providerDuration"`
	ProviderResult      *RerankProviderResult `json:"providerResult,omitempty"`
}

// RerankCandidateEvidence preserves where a candidate came from without
// exposing retrieval rank/score to the semantic provider.
type RerankCandidateEvidence struct {
	Ref               knowledge.KnowledgeRef `json:"ref"`
	OriginalRank      int                    `json:"originalRank"`
	RetrievalEvidence []LaneEvidence         `json:"retrievalEvidence,omitempty"`
}

// Reranker is the wall-out semantic provider port. Implementations own model
// credentials, batching, rate limits and retries. The executor owns fixed
// basis reads, field projection, output validation and top-K semantics.
type Reranker interface {
	Rerank(context.Context, RerankRequest) (RerankProviderResult, error)
}

type SemanticOperator string

const (
	OpSemanticFilter SemanticOperator = "SEMANTIC_FILTER"
	OpSemanticRerank SemanticOperator = "SEMANTIC_RERANK"
)

// EvaluationProjection is the Refine judge/scorer field whitelist (design 7.3).
// Not an Access Projection and not AspectSelector.
type EvaluationProjection struct {
	Fields []string `json:"fields,omitempty"`
}

type OutputContract struct {
	TopK          *int `json:"topK,omitempty"`
	AllowTies     bool `json:"allowTies"`
	AllowUnjudged bool `json:"allowUnjudged"`
}

type SemanticOperatorSpec struct {
	SpecRef              string                `json:"specRef"`
	Revision             int                   `json:"revision"`
	Operator             SemanticOperator      `json:"operator"`
	Criterion            string                `json:"criterion"`
	EvaluationProjection *EvaluationProjection `json:"evaluationProjection,omitempty"`
	ContextRefs          []string              `json:"contextRefs,omitempty"`
	OutputContract       OutputContract        `json:"outputContract"`
}

func ValidateSemanticOperatorSpec(spec SemanticOperatorSpec) error {
	if strings.TrimSpace(spec.SpecRef) == "" || spec.Revision <= 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "semantic operator requires specRef and positive revision")
	}
	if spec.Operator != OpSemanticFilter && spec.Operator != OpSemanticRerank {
		return kernel.Fail(kernel.ErrUsageInvalid, "unknown semantic operator %q", spec.Operator)
	}
	if strings.TrimSpace(spec.Criterion) == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "semantic operator requires criterion")
	}
	if spec.OutputContract.TopK != nil && *spec.OutputContract.TopK <= 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "semantic rerank topK must be positive")
	}
	if len(spec.ContextRefs) > 0 {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "semantic contextRefs are not executable yet")
	}
	seen := map[string]struct{}{}
	if spec.EvaluationProjection != nil {
		for _, field := range spec.EvaluationProjection.Fields {
			normalized := strings.TrimSpace(field)
			if normalized == "" {
				return kernel.Fail(kernel.ErrUsageInvalid, "evaluationProjection fields must be non-empty")
			}
			if normalized != field {
				return kernel.Fail(kernel.ErrUsageInvalid, "evaluationProjection field %q must not contain surrounding whitespace", field)
			}
			if _, duplicate := seen[normalized]; duplicate {
				return kernel.Fail(kernel.ErrUsageInvalid, "evaluationProjection field %q is duplicated", normalized)
			}
			seen[normalized] = struct{}{}
		}
	}
	return nil
}

func projectFields(value any, projection EvaluationProjection) any {
	obj, _ := value.(map[string]any)
	out := map[string]any{}
	for _, f := range projection.Fields {
		parts := strings.Split(f, ".")
		var cur any
		if obj != nil {
			cur = obj
		}
		for _, p := range parts {
			m, ok := cur.(map[string]any)
			if !ok {
				cur = nil
				break
			}
			cur = m[p]
		}
		out[f] = cur
	}
	return out
}

type Refine struct{}

func (Refine) Filter(candidates []Candidate, criterion string, judge FilterJudge, budget *int) SemanticFilterResult {
	matched := []knowledge.KnowledgeRef{}
	rejected := []knowledge.KnowledgeRef{}
	unknown := []knowledge.KnowledgeRef{}
	unjudged := []knowledge.KnowledgeRef{}
	judged := 0
	for _, c := range candidates {
		if budget != nil && judged >= *budget {
			unjudged = append(unjudged, c.Ref)
			continue
		}
		judged++
		switch judge(c, criterion) {
		case JudgmentMatch:
			matched = append(matched, c.Ref)
		case JudgmentNoMatch:
			rejected = append(rejected, c.Ref)
		default:
			unknown = append(unknown, c.Ref)
		}
	}
	reason := ""
	if len(unjudged) > 0 {
		reason = "CANDIDATE_BUDGET"
	}
	return SemanticFilterResult{
		Matched: matched, Rejected: rejected, Unknown: unknown, Unjudged: unjudged,
		Complete: len(unjudged) == 0, TruncationReason: reason,
	}
}

func (Refine) Rerank(candidates []Candidate, criterion string, scorer RerankScorer, topK *int) SemanticRerankResult {
	type scored struct {
		ref   knowledge.KnowledgeRef
		score float64
	}
	items := make([]scored, 0, len(candidates))
	for _, c := range candidates {
		items = append(items, scored{ref: c.Ref, score: scorer(c, criterion)})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].score > items[j].score })
	var groups []RankGroup
	var current []knowledge.KnowledgeRef
	var currentScore *float64
	for _, s := range items {
		if currentScore != nil && *currentScore != s.score {
			groups = append(groups, RankGroup{Rank: len(groups) + 1, Refs: current})
			current = nil
		}
		score := s.score
		currentScore = &score
		current = append(current, s.ref)
	}
	if len(current) > 0 {
		groups = append(groups, RankGroup{Rank: len(groups) + 1, Refs: current})
	}
	contract := OutputContract{TopK: topK}
	return applyRerankOutput(groups, nil, contract)
}

// ExecuteRerank is the fail-closed semantic execution boundary. It projects
// model-visible fields, validates the provider's ref-preserving partition and
// applies output policy centrally so a provider cannot reinterpret topK/ties.
func ExecuteRerank(ctx context.Context, reranker Reranker, request RerankRequest) (SemanticRerankResult, error) {
	result, _, err := ExecuteRerankRecorded(ctx, reranker, request)
	return result, err
}

// ExecuteRerankRecorded is ExecuteRerank plus application-owned evidence
// source facts. Callers durably record the execution before acknowledging a
// completed or attempted semantic operation.
func ExecuteRerankRecorded(ctx context.Context, reranker Reranker, request RerankRequest) (SemanticRerankResult, RerankExecutionRecord, error) {
	empty := RerankExecutionRecord{}
	if err := ValidateSemanticOperatorSpec(request.Spec); err != nil {
		return SemanticRerankResult{}, empty, err
	}
	if request.Spec.Operator != OpSemanticRerank {
		return SemanticRerankResult{}, empty, kernel.Fail(kernel.ErrUsageInvalid, "ExecuteRerank requires SEMANTIC_RERANK")
	}
	if len(request.Candidates) == 0 || len(request.Candidates) > MaxRerankCandidates {
		return SemanticRerankResult{}, empty, kernel.Fail(kernel.ErrUsageInvalid,
			"semantic rerank requires between 1 and %d candidates", MaxRerankCandidates)
	}
	if reranker == nil {
		return SemanticRerankResult{}, empty, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "semantic reranker is not configured")
	}
	seen := map[knowledge.KnowledgeRef]struct{}{}
	projected := make([]Candidate, len(request.Candidates))
	evidence := make([]RerankCandidateEvidence, len(request.Candidates))
	for i, candidate := range request.Candidates {
		if candidate.Ref.Repository == "" || candidate.Ref.Object == "" {
			return SemanticRerankResult{}, empty, kernel.Fail(kernel.ErrUsageInvalid, "semantic rerank candidate requires repository and object")
		}
		if _, duplicate := seen[candidate.Ref]; duplicate {
			return SemanticRerankResult{}, empty, kernel.Fail(kernel.ErrUsageInvalid, "semantic rerank candidate %s is duplicated", candidate.Ref.Object)
		}
		seen[candidate.Ref] = struct{}{}
		if candidate.OriginalRank == 0 {
			candidate.OriginalRank = i + 1
		}
		if candidate.OriginalRank != i+1 {
			return SemanticRerankResult{}, empty, kernel.Fail(kernel.ErrUsageInvalid,
				"semantic rerank candidate originalRank must match candidate window order")
		}
		projected[i] = candidate
		if request.Spec.EvaluationProjection != nil {
			projected[i].Value = projectFields(candidate.Value, *request.Spec.EvaluationProjection)
		}
		valueBytes, err := json.Marshal(projected[i].Value)
		if err != nil {
			return SemanticRerankResult{}, empty, kernel.Fail(kernel.ErrUsageInvalid, "semantic rerank candidate value is not JSON serializable")
		}
		if len(valueBytes) > MaxRerankCandidateBytes {
			return SemanticRerankResult{}, empty, kernel.Fail(kernel.ErrUsageInvalid,
				"semantic rerank candidate %s exceeds %d model-visible bytes", candidate.Ref.Object, MaxRerankCandidateBytes)
		}
		evidence[i] = RerankCandidateEvidence{
			Ref: candidate.Ref, OriginalRank: candidate.OriginalRank,
			RetrievalEvidence: append([]LaneEvidence(nil), candidate.RetrievalEvidence...),
		}
	}
	digestRequest := request
	digestRequest.Candidates = projected
	inputDigest := kernel.CanonicalDigest(digestRequest)
	providerRequest := request
	providerRequest.Candidates = make([]Candidate, len(projected))
	for i, candidate := range projected {
		providerRequest.Candidates[i] = Candidate{Ref: candidate.Ref, Value: candidate.Value}
	}
	providerBytes, err := json.Marshal(providerRequest)
	if err != nil {
		return SemanticRerankResult{}, empty, kernel.Fail(kernel.ErrUsageInvalid, "semantic rerank input is not JSON serializable")
	}
	execution := RerankExecutionRecord{
		SearchView: request.SearchView, Spec: request.Spec, Candidates: projected,
		CandidateDigest: inputDigest, ProjectedInputBytes: len(providerBytes),
	}
	if len(providerBytes) > MaxRerankInputBytes {
		return SemanticRerankResult{}, execution, kernel.Fail(kernel.ErrUsageInvalid,
			"semantic rerank input exceeds %d model-visible bytes", MaxRerankInputBytes)
	}
	started := time.Now()
	providerResult, err := reranker.Rerank(ctx, providerRequest)
	execution.ProviderDuration = time.Since(started)
	if err != nil {
		return SemanticRerankResult{}, execution, err
	}
	execution.ProviderResult = &providerResult
	if strings.TrimSpace(providerResult.Provider) == "" || strings.TrimSpace(providerResult.Model) == "" {
		return SemanticRerankResult{}, execution, kernel.Fail(kernel.ErrPreconditionFailed, "semantic reranker omitted provider or model identity")
	}
	judged, err := validateProviderRanking(seen, providerResult)
	if err != nil {
		return SemanticRerankResult{}, execution, err
	}
	if !request.Spec.OutputContract.AllowUnjudged && len(providerResult.Unjudged) > 0 {
		return SemanticRerankResult{}, execution, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "unjudged results not allowed by output contract")
	}
	result := applyRerankOutput(providerResult.Groups, providerResult.Unjudged, request.Spec.OutputContract)
	view := request.SearchView
	result.SearchView = &view
	result.Evidence = &SemanticRerankEvidence{
		Provider: providerResult.Provider, Model: providerResult.Model, ModelRevision: providerResult.ModelRevision, PromptRevision: providerResult.PromptRevision,
		SpecRef: request.Spec.SpecRef, SpecRevision: request.Spec.Revision, CandidateDigest: inputDigest,
		CandidateCount: len(request.Candidates), JudgedCount: judged, ProjectedInputBytes: len(providerBytes), Candidates: evidence,
	}
	return result, execution, nil
}

func validateProviderRanking(inputs map[knowledge.KnowledgeRef]struct{}, result RerankProviderResult) (int, error) {
	seen := map[knowledge.KnowledgeRef]struct{}{}
	judged := 0
	for i, group := range result.Groups {
		if group.Rank != i+1 || len(group.Refs) == 0 {
			return 0, kernel.Fail(kernel.ErrPreconditionFailed, "semantic reranker returned invalid rank groups")
		}
		for _, ref := range group.Refs {
			if _, exists := inputs[ref]; !exists {
				return 0, kernel.Fail(kernel.ErrPreconditionFailed, "semantic reranker returned an unknown ref")
			}
			if _, duplicate := seen[ref]; duplicate {
				return 0, kernel.Fail(kernel.ErrPreconditionFailed, "semantic reranker returned a duplicate ref")
			}
			seen[ref] = struct{}{}
			judged++
		}
	}
	for _, ref := range result.Unjudged {
		if _, exists := inputs[ref]; !exists {
			return 0, kernel.Fail(kernel.ErrPreconditionFailed, "semantic reranker returned an unknown unjudged ref")
		}
		if _, duplicate := seen[ref]; duplicate {
			return 0, kernel.Fail(kernel.ErrPreconditionFailed, "semantic reranker returned a duplicate ref")
		}
		seen[ref] = struct{}{}
	}
	if len(seen) != len(inputs) {
		return 0, kernel.Fail(kernel.ErrPreconditionFailed, "semantic reranker omitted one or more input refs")
	}
	return judged, nil
}

func applyRerankOutput(groups []RankGroup, unjudged []knowledge.KnowledgeRef, contract OutputContract) SemanticRerankResult {
	result := SemanticRerankResult{
		Groups: []RankGroup{}, NotSelected: []knowledge.KnowledgeRef{},
		Unjudged: append([]knowledge.KnowledgeRef(nil), unjudged...), Complete: len(unjudged) == 0,
	}
	if len(unjudged) > 0 {
		result.TruncationReason = "PROVIDER_BUDGET"
	}
	if contract.TopK == nil {
		result.Groups = cloneRankGroups(groups)
		return result
	}
	kept := 0
	for i, group := range groups {
		remaining := *contract.TopK - kept
		if remaining <= 0 {
			appendNotSelected(&result, groups[i:])
			break
		}
		if len(group.Refs) <= remaining || contract.AllowTies {
			result.Groups = append(result.Groups, RankGroup{Rank: group.Rank, Refs: append([]knowledge.KnowledgeRef(nil), group.Refs...)})
			kept += len(group.Refs)
			if kept >= *contract.TopK {
				appendNotSelected(&result, groups[i+1:])
				break
			}
			continue
		}
		result.Groups = append(result.Groups, RankGroup{Rank: group.Rank, Refs: append([]knowledge.KnowledgeRef(nil), group.Refs[:remaining]...)})
		result.NotSelected = append(result.NotSelected, group.Refs[remaining:]...)
		appendNotSelected(&result, groups[i+1:])
		break
	}
	return result
}

func appendNotSelected(result *SemanticRerankResult, groups []RankGroup) {
	for _, group := range groups {
		result.NotSelected = append(result.NotSelected, group.Refs...)
	}
}

func cloneRankGroups(groups []RankGroup) []RankGroup {
	out := make([]RankGroup, len(groups))
	for i, group := range groups {
		out[i] = RankGroup{Rank: group.Rank, Refs: append([]knowledge.KnowledgeRef(nil), group.Refs...)}
	}
	return out
}

func (s Refine) Run(spec SemanticOperatorSpec, candidates []Candidate, judge FilterJudge, scorer RerankScorer) (any, error) {
	if err := ValidateSemanticOperatorSpec(spec); err != nil {
		return nil, err
	}
	projected := candidates
	if spec.EvaluationProjection != nil {
		projected = make([]Candidate, len(candidates))
		for i, c := range candidates {
			projected[i] = Candidate{Ref: c.Ref, Value: projectFields(c.Value, *spec.EvaluationProjection)}
		}
	}
	if spec.Operator == OpSemanticFilter {
		if judge == nil {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "SEMANTIC_FILTER requires a judge")
		}
		result := s.Filter(projected, spec.Criterion, judge, nil)
		if !spec.OutputContract.AllowUnjudged && len(result.Unjudged) > 0 {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "unjudged results not allowed by output contract")
		}
		return result, nil
	}
	if scorer == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "SEMANTIC_RERANK requires a scorer")
	}
	result := s.Rerank(projected, spec.Criterion, scorer, nil)
	result = applyRerankOutput(result.Groups, nil, spec.OutputContract)
	if !spec.OutputContract.AllowUnjudged && len(result.Unjudged) > 0 {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "unjudged results not allowed by output contract")
	}
	return result, nil
}

func KeywordJudge(criterion string) FilterJudge {
	keywords := keywordList(criterion)
	return func(candidate Candidate, _ string) FilterJudgment {
		text := strings.ToLower(mustJSON(candidate.Value))
		for _, k := range keywords {
			if strings.Contains(text, k) {
				return JudgmentMatch
			}
		}
		return JudgmentNoMatch
	}
}

func KeywordScorer(criterion string) RerankScorer {
	keywords := keywordList(criterion)
	return func(candidate Candidate, _ string) float64 {
		text := strings.ToLower(mustJSON(candidate.Value))
		n := 0.0
		for _, k := range keywords {
			if strings.Contains(text, k) {
				n++
			}
		}
		return n
	}
}

func keywordList(criterion string) []string {
	var out []string
	for _, w := range strings.Fields(strings.ToLower(criterion)) {
		if len(w) > 1 {
			out = append(out, w)
		}
	}
	return out
}

func mustJSON(value any) string {
	b, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(b)
}
