package observability

// RerankTrainingSample is a rebuildable join, not another source of truth.
// Raw inference and feedback remain in refine.jsonl / feedback.jsonl.
type RerankTrainingSample struct {
	Refine           RefineEvent     `json:"refine"`
	Feedback         []FeedbackEvent `json:"feedback"`
	LabelStrength    string          `json:"labelStrength"`
	TrainingEligible bool            `json:"trainingEligible"`
}

func (s *FileStore) RerankTrainingSamples(query RefineQuery) ([]RerankTrainingSample, error) {
	refines, err := s.Refine(query)
	if err != nil {
		return nil, err
	}
	feedback, err := readJSONL[FeedbackEvent](s.FeedbackPath)
	if err != nil {
		return nil, err
	}
	byRefine := map[string][]FeedbackEvent{}
	for _, event := range feedback {
		if event.RefineEvidenceID != "" {
			byRefine[event.RefineEvidenceID] = append(byRefine[event.RefineEvidenceID], event)
		}
	}
	out := make([]RerankTrainingSample, 0, len(refines))
	for _, event := range refines {
		labels := byRefine[event.EvidenceID]
		strength, eligible := classifyTrainingLabels(event, labels)
		out = append(out, RerankTrainingSample{
			Refine: event, Feedback: labels, LabelStrength: strength, TrainingEligible: eligible,
		})
	}
	return out, nil
}

func classifyTrainingLabels(refine RefineEvent, labels []FeedbackEvent) (string, bool) {
	return classifyCompletedTrainingLabels(refine.Outcome == "COMPLETED", labels)
}

func classifyCompletedTrainingLabels(completed bool, labels []FeedbackEvent) (string, bool) {
	if !completed {
		return "none", false
	}
	answerSelections := 0
	hasHumanAcceptance := false
	for _, label := range labels {
		humanLabel := label.LabelSource == "user" || label.LabelSource == "human-review"
		if humanLabel && label.Outcome == "corrected" && (len(label.IdealGroups) > 0 || len(label.SelectedRefs) > 0) {
			return "corrected", true
		}
		if label.Outcome == "answered" && len(label.SelectedRefs) > 0 {
			answerSelections++
		}
		if humanLabel && (label.Outcome == "accepted" || label.Outcome == "helpful") {
			hasHumanAcceptance = true
		}
	}
	// Without a separate answer identifier, an acceptance can only upgrade a
	// refine window when exactly one answer selected evidence from it. Multiple
	// answers are deliberately ambiguous and stay out of training.
	if answerSelections == 1 && hasHumanAcceptance {
		return "accepted-answer", true
	}
	if answerSelections > 0 {
		return "agent-weak", false
	}
	return "none", false
}

type RetrievalTrainingSample struct {
	Retrieval        RetrievalEvent  `json:"retrieval"`
	Feedback         []FeedbackEvent `json:"feedback"`
	LabelStrength    string          `json:"labelStrength"`
	TrainingEligible bool            `json:"trainingEligible"`
}

func (s *FileStore) RetrievalTrainingSamples(query RetrievalQuery) ([]RetrievalTrainingSample, error) {
	retrievals, err := s.Retrieval(query)
	if err != nil {
		return nil, err
	}
	feedback, err := readJSONL[FeedbackEvent](s.FeedbackPath)
	if err != nil {
		return nil, err
	}
	byRetrieval := map[string][]FeedbackEvent{}
	for _, event := range feedback {
		if event.RetrievalEvidenceID != "" {
			byRetrieval[event.RetrievalEvidenceID] = append(byRetrieval[event.RetrievalEvidenceID], event)
		}
	}
	out := make([]RetrievalTrainingSample, 0, len(retrievals))
	for _, event := range retrievals {
		labels := byRetrieval[event.EvidenceID]
		strength, eligible := classifyCompletedTrainingLabels(event.Outcome == "COMPLETED", labels)
		out = append(out, RetrievalTrainingSample{
			Retrieval: event, Feedback: labels, LabelStrength: strength, TrainingEligible: eligible,
		})
	}
	return out, nil
}
