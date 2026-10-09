package adapter_test

import (
	"context"
	"testing"

	"payment-core/internal/adapter"
)

func TestAdapterRegistry(t *testing.T) {
	reg := adapter.NewRegistry()

	mfs := adapter.NewMFSAdapter("http://localhost:5011")
	card := adapter.NewCardAdapter("http://localhost:5012")

	reg.Register("mfs", mfs)
	reg.Register("card", card)

	foundMFS, err := reg.Get("mfs")
	if err != nil {
		t.Fatalf("expected mfs adapter, got err: %v", err)
	}
	if foundMFS.Name() != "mock-mfs" {
		t.Errorf("expected name mock-mfs, got %s", foundMFS.Name())
	}

	foundCard, err := reg.Get("card")
	if err != nil {
		t.Fatalf("expected card adapter, got err: %v", err)
	}
	if foundCard.Name() != "mock-card" {
		t.Errorf("expected name mock-card, got %s", foundCard.Name())
	}

	_, err = reg.Get("unknown")
	if err == nil {
		t.Fatal("expected error for unregistered adapter, got nil")
	}
}

func TestMFSAdapter_CreateSessionAndCharge(t *testing.T) {
	mfs := adapter.NewMFSAdapter("http://mock-mfs:5011")

	ctx := context.Background()
	sess, err := mfs.CreateSession(ctx, adapter.CreateSessionRequest{
		PaymentID:  "01JABCDEF12345678901234567",
		CustomerID: "cust_123",
		Amount:     10000,
		Currency:   "BDT",
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	if sess.SessionID == "" || sess.RedirectURL == "" || sess.ProviderReference == "" {
		t.Fatalf("invalid session response: %+v", sess)
	}

	chg, err := mfs.Charge(ctx, adapter.ChargeRequest{
		PaymentID:      "01JABCDEF12345678901234567",
		CustomerID:     "cust_123",
		TokenReference: "agr_123",
		Amount:         10000,
		Currency:       "BDT",
	})
	if err != nil || !chg.Success {
		t.Fatalf("failed to charge: %v", err)
	}
}
