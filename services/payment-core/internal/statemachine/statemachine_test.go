package statemachine_test

import (
	"testing"

	"payment-core/internal/model"
	"payment-core/internal/statemachine"
)

func TestStateTransitions_Legal(t *testing.T) {
	legalCases := []struct {
		from model.PaymentStatus
		to   model.PaymentStatus
	}{
		{model.StatusCreated, model.StatusPending},
		{model.StatusPending, model.StatusAuthorized},
		{model.StatusPending, model.StatusCaptured}, // 1-step capture
		{model.StatusPending, model.StatusFailed},
		{model.StatusAuthorized, model.StatusCaptured},
		{model.StatusAuthorized, model.StatusFailed},
		{model.StatusAuthorized, model.StatusExpired},
		{model.StatusCaptured, model.StatusRefunded},
	}

	for _, tc := range legalCases {
		if !statemachine.IsValidTransition(tc.from, tc.to) {
			t.Errorf("expected transition from %s to %s to be legal, but was rejected", tc.from, tc.to)
		}
	}
}

func TestStateTransitions_Illegal(t *testing.T) {
	illegalCases := []struct {
		from model.PaymentStatus
		to   model.PaymentStatus
	}{
		{model.StatusCreated, model.StatusCaptured},
		{model.StatusCreated, model.StatusRefunded},
		{model.StatusCreated, model.StatusExpired},
		{model.StatusCaptured, model.StatusPending},
		{model.StatusCaptured, model.StatusAuthorized},
		{model.StatusCaptured, model.StatusFailed},
		{model.StatusFailed, model.StatusPending},
		{model.StatusFailed, model.StatusCaptured},
		{model.StatusExpired, model.StatusCaptured},
		{model.StatusExpired, model.StatusAuthorized},
		{model.StatusRefunded, model.StatusCaptured},
		{model.StatusRefunded, model.StatusPending},
	}

	for _, tc := range illegalCases {
		if statemachine.IsValidTransition(tc.from, tc.to) {
			t.Errorf("expected transition from %s to %s to be ILLEGAL, but was accepted", tc.from, tc.to)
		}
	}
}

func TestTerminalStates(t *testing.T) {
	terminal := []model.PaymentStatus{
		model.StatusFailed,
		model.StatusExpired,
		model.StatusRefunded,
	}

	for _, st := range terminal {
		if !st.IsTerminal() {
			t.Errorf("expected status %s to be terminal", st)
		}
		// No transitions allowed out of terminal states
		for _, next := range []model.PaymentStatus{model.StatusPending, model.StatusCaptured, model.StatusAuthorized} {
			if statemachine.IsValidTransition(st, next) {
				t.Errorf("illegal transition out of terminal state %s to %s was allowed", st, next)
			}
		}
	}

	nonTerminal := []model.PaymentStatus{
		model.StatusCreated,
		model.StatusPending,
		model.StatusAuthorized,
		model.StatusCaptured,
	}

	for _, st := range nonTerminal {
		if st.IsTerminal() {
			t.Errorf("expected status %s NOT to be terminal", st)
		}
	}
}
