package observability

import (
	"testing"

	"kc/knowledge"
)

func TestRerankTrainingDoesNotPromoteSelfAcceptedOrAmbiguousAgentAnswers(t *testing.T) {
	refine := RefineEvent{Outcome: "COMPLETED"}
	answer := func(object knowledge.ObjectID) FeedbackEvent {
		return FeedbackEvent{
			Outcome: "answered", LabelSource: "agent",
			SelectedRefs: []knowledge.KnowledgeRef{{Repository: "kr://acme/runbooks", Object: object}},
		}
	}
	for _, tc := range []struct {
		name   string
		labels []FeedbackEvent
	}{
		{
			name: "agent self acceptance remains weak",
			labels: []FeedbackEvent{
				answer("runbook/one"), {Outcome: "accepted", LabelSource: "agent"},
			},
		},
		{
			name: "two answers make a user acceptance ambiguous",
			labels: []FeedbackEvent{
				answer("runbook/one"), answer("runbook/two"), {Outcome: "accepted", LabelSource: "user"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			strength, eligible := classifyTrainingLabels(refine, tc.labels)
			if eligible || strength != "agent-weak" {
				t.Fatalf("strength=%q eligible=%v", strength, eligible)
			}
		})
	}
}

func TestRerankTrainingOnlyPromotesHumanCorrections(t *testing.T) {
	refine := RefineEvent{Outcome: "COMPLETED"}
	ref := knowledge.KnowledgeRef{Repository: "kr://acme/runbooks", Object: "runbook/one"}
	for _, tc := range []struct {
		name     string
		label    FeedbackEvent
		strength string
		eligible bool
	}{
		{name: "human correction", label: FeedbackEvent{Outcome: "corrected", LabelSource: "user", SelectedRefs: []knowledge.KnowledgeRef{ref}}, strength: "corrected", eligible: true},
		{name: "agent correction", label: FeedbackEvent{Outcome: "corrected", LabelSource: "agent", SelectedRefs: []knowledge.KnowledgeRef{ref}}, strength: "none", eligible: false},
		{name: "human rejection", label: FeedbackEvent{Outcome: "rejected", LabelSource: "user"}, strength: "none", eligible: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			strength, eligible := classifyTrainingLabels(refine, []FeedbackEvent{tc.label})
			if strength != tc.strength || eligible != tc.eligible {
				t.Fatalf("strength=%q eligible=%v", strength, eligible)
			}
		})
	}
}
