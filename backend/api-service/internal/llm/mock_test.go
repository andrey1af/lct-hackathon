package llm

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

func TestMockControlDialogues(t *testing.T) {
	raw, err := os.ReadFile("../../../testdata/dialogues/control.json")
	if err != nil {
		t.Fatal(err)
	}

	var dialogues []struct {
		CaseKey string   `json:"case_key"`
		Passed  bool     `json:"passed"`
		Lines   []string `json:"lines"`
	}

	if err := json.Unmarshal(raw, &dialogues); err != nil {
		t.Fatal(err)
	}

	if len(dialogues) != 16 {
		t.Fatalf("control dialogues = %d, want 16", len(dialogues))
	}

	criteria := map[string][]string{
		"assessment-interests":   {"interest_question", "checks_understanding", "uses_interest"},
		"assessment-concessions": {"conditional_trade", "batna_comparison", "rational_decision"},
		"practice-procurement":   {"partial_delivery_question", "measurable_trade", "alternative_check"},
		"practice-sales":         {"launch_interest", "conditional_discount", "sales_batna"},
		"assessment-objections":  {"objection_question", "argument_fit", "price_discipline"},
		"assessment-tradeoffs":   {"cross_issue_trade", "package_comparison", "next_step"},
		"practice-logistics":     {"cost_breakdown_question", "payment_trade", "carrier_batna"},
		"practice-partnership":   {"brand_interest_question", "value_trade", "launch_next_step"},
	}
	mock := NewMock()

	for _, dialogue := range dialogues {
		t.Run(dialogue.CaseKey, func(t *testing.T) {
			answer, err := mock.Complete(context.Background(), Request{Kind: KindAssessment, CaseKey: dialogue.CaseKey, CriteriaIDs: criteria[dialogue.CaseKey], PlayerLines: dialogue.Lines})
			if err != nil {
				t.Fatal(err)
			}

			var verdict mockVerdict

			if err := json.Unmarshal([]byte(answer), &verdict); err != nil {
				t.Fatal(err)
			}

			if len(verdict.Criteria) != 3 {
				t.Fatalf("criteria = %d, want 3", len(verdict.Criteria))
			}

			for _, item := range verdict.Criteria {
				if item.Met != dialogue.Passed {
					t.Errorf("criterion %s met=%v, want %v", item.ID, item.Met, dialogue.Passed)
				}
			}
		})
	}
}
