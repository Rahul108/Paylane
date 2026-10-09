package saga

import (
	"errors"
	"fmt"
)

var (
	ErrUnknownJourneyType = errors.New("unknown journey type")
)

var JourneyDefinitions = map[string][]string{
	"ui_payment":           {"PAYMENT"},
	"uiless_payment":       {"PAYMENT"},
	"payment_recharge":     {"PAYMENT", "RECHARGE"},
	"payment_cashback":     {"PAYMENT", "CASHBACK"},
	"payment_subscription": {"PAYMENT", "SUBSCRIBE"},
}

func GetSteps(journeyType string) ([]string, error) {
	steps, ok := JourneyDefinitions[journeyType]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownJourneyType, journeyType)
	}
	return steps, nil
}
