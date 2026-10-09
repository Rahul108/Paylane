package saga_test

import (
	"testing"

	"orchestrator/internal/saga"
)

func TestJourneyDefinitions(t *testing.T) {
	cases := []struct {
		journeyType   string
		expectedSteps []string
	}{
		{"ui_payment", []string{"PAYMENT"}},
		{"uiless_payment", []string{"PAYMENT"}},
		{"payment_recharge", []string{"PAYMENT", "RECHARGE"}},
		{"payment_cashback", []string{"PAYMENT", "CASHBACK"}},
		{"payment_subscription", []string{"PAYMENT", "SUBSCRIBE"}},
	}

	for _, tc := range cases {
		steps, err := saga.GetSteps(tc.journeyType)
		if err != nil {
			t.Fatalf("expected valid steps for %s, got error: %v", tc.journeyType, err)
		}
		if len(steps) != len(tc.expectedSteps) {
			t.Fatalf("step count mismatch for %s: got %d, want %d", tc.journeyType, len(steps), len(tc.expectedSteps))
		}
		for i, s := range steps {
			if s != tc.expectedSteps[i] {
				t.Errorf("step order mismatch at %d: got %s, want %s", i, s, tc.expectedSteps[i])
			}
		}
	}

	_, err := saga.GetSteps("invalid_flow")
	if err == nil {
		t.Fatal("expected error for unknown journey type, got nil")
	}
}
