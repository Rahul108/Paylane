package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"mock-card/internal/service"
	"paylane-jwe"
)

type CardHandler struct {
	svc *service.Service
}

func NewCardHandler(svc *service.Service) *CardHandler {
	return &CardHandler{svc: svc}
}

// CreateSession handles POST /sessions
func (h *CardHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	var bodyBytes []byte
	if payload := jwe.GetDecryptedBody(r.Context()); len(payload) > 0 {
		bodyBytes = payload
	} else {
		var reqMap map[string]any
		_ = json.NewDecoder(r.Body).Decode(&reqMap)
		bodyBytes, _ = json.Marshal(reqMap)
	}

	var req service.CreateSessionRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	resp, err := h.svc.CreateSession(r.Context(), req)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

// GetSession handles GET /sessions/{id}
func (h *CardHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) >= 2 {
			id = parts[1]
		}
	}

	sess, err := h.svc.GetSession(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sess)
}

// RenderCheckout handles GET /checkout?session_id=...
func (h *CardHandler) RenderCheckout(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "missing session_id parameter", http.StatusBadRequest)
		return
	}

	sess, err := h.svc.GetSession(r.Context(), sessionID)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	amountFormatted := fmt.Sprintf("%.2f %s", float64(sess.Amount)/100.0, sess.Currency)

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Paylane Mock Card Gateway</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; }
    body { background: #0f172a; color: #f8fafc; display: flex; align-items: center; justify-content: center; min-height: 100vh; padding: 1.5rem; }
    .card { background: #1e293b; border: 1px solid rgba(255, 255, 255, 0.1); border-radius: 1rem; width: 100%%; max-width: 480px; padding: 2rem; box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.5); }
    .badge { display: inline-block; background: rgba(168, 85, 247, 0.2); color: #c084fc; font-size: 0.75rem; font-weight: 700; padding: 0.25rem 0.6rem; border-radius: 9999px; margin-bottom: 0.75rem; text-transform: uppercase; }
    h1 { font-size: 1.25rem; font-weight: 700; margin-bottom: 0.25rem; }
    p.sub { font-size: 0.85rem; color: #94a3b8; margin-bottom: 1.25rem; }
    .amount-box { background: rgba(15, 23, 42, 0.6); border: 1px solid rgba(255, 255, 255, 0.05); border-radius: 0.5rem; padding: 1rem; margin-bottom: 1.25rem; text-align: center; }
    .amount-title { font-size: 0.75rem; color: #94a3b8; text-transform: uppercase; letter-spacing: 0.05em; }
    .amount-val { font-size: 1.75rem; font-weight: 800; color: #38bdf8; margin-top: 0.25rem; }
    .test-box { background: rgba(59, 130, 246, 0.1); border: 1px solid rgba(59, 130, 246, 0.2); border-radius: 0.5rem; padding: 0.75rem; font-size: 0.75rem; color: #93c5fd; margin-bottom: 1.25rem; line-height: 1.4; }
    .field { margin-bottom: 1rem; }
    .row { display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem; }
    label { display: block; font-size: 0.8rem; font-weight: 600; color: #cbd5e1; margin-bottom: 0.35rem; }
    input, select { width: 100%%; background: #0f172a; border: 1px solid rgba(255, 255, 255, 0.15); border-radius: 0.5rem; padding: 0.65rem 0.85rem; color: #fff; font-size: 0.95rem; outline: none; transition: border-color 0.2s; }
    input:focus, select:focus { border-color: #a855f7; }
    .btn { width: 100%%; background: #9333ea; color: #fff; border: none; border-radius: 0.5rem; padding: 0.85rem; font-size: 1rem; font-weight: 700; cursor: pointer; transition: background 0.2s; box-shadow: 0 4px 12px rgba(147, 51, 234, 0.3); margin-top: 0.5rem; }
    .btn:hover { background: #7e22ce; }
    .footer { font-size: 0.75rem; color: #64748b; text-align: center; margin-top: 1.25rem; }
  </style>
</head>
<body>
  <div class="card">
    <span class="badge">3DS Secure Card Gateway</span>
    <h1>Mock Card Checkout</h1>
    <p class="sub">Session ID: <code>%s</code></p>

    <div class="amount-box">
      <div class="amount-title">Total Payable Amount</div>
      <div class="amount-val">%s</div>
    </div>

    <div class="test-box">
      <strong>Test Card Numbers:</strong><br>
      • <code>4111 1111 1111 1111</code> (Visa - Success)<br>
      • <code>5555 5555 5555 4444</code> (Mastercard - Success)<br>
      • <code>4000 0000 0000 0002</code> (Decline - Insufficient Funds)<br>
      • <code>4000 0000 0000 0003</code> (Requires 3DS OTP: 123456)
    </div>

    <form method="POST" action="/checkout/confirm">
      <input type="hidden" name="session_id" value="%s">

      <div class="field">
        <label for="cardholder">Cardholder Name</label>
        <input type="text" id="cardholder" name="cardholder" value="Jane Doe" required>
      </div>

      <div class="field">
        <label for="pan">Card PAN (16 digits)</label>
        <input type="text" id="pan" name="pan" value="4111111111111111" maxlength="19" required>
      </div>

      <div class="row">
        <div class="field">
          <label for="expiry">Expiry (MM/YY)</label>
          <input type="text" id="expiry" name="expiry" value="12/28" maxlength="5" required>
        </div>
        <div class="field">
          <label for="cvv">CVV (3 digits)</label>
          <input type="password" id="cvv" name="cvv" value="123" maxlength="4" required>
        </div>
      </div>

      <div class="field">
        <label for="otp">3DS OTP Code (Default: 123456)</label>
        <input type="text" id="otp" name="otp" value="123456" maxlength="6">
      </div>

      <div class="field">
        <label for="scenario">Scenario Simulation</label>
        <select id="scenario" name="scenario">
          <option value="SUCCESS">SUCCESS (Approve & 3DS Capture)</option>
          <option value="INSUFFICIENT_FUNDS">INSUFFICIENT_FUNDS (Decline)</option>
          <option value="DECLINED">DECLINED (Bank Refusal)</option>
          <option value="USER_ABANDONS">USER_ABANDONS (No Webhook)</option>
        </select>
      </div>

      <button type="submit" class="btn">Pay Now</button>
    </form>

    <div class="footer">Paylane Local Mock Gateway • 3D-Secure 2.0</div>
  </div>
</body>
</html>`, sess.ID, amountFormatted, sess.ID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

// ConfirmCheckout handles POST /checkout/confirm
func (h *CardHandler) ConfirmCheckout(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	sessionID := r.FormValue("session_id")
	cardholder := r.FormValue("cardholder")
	pan := r.FormValue("pan")
	expiry := r.FormValue("expiry")
	cvv := r.FormValue("cvv")
	otp := r.FormValue("otp")
	scenario := r.FormValue("scenario")

	if sessionID == "" {
		http.Error(w, "missing session_id", http.StatusBadRequest)
		return
	}

	targetURL, err := h.svc.ConfirmPayment(r.Context(), sessionID, cardholder, pan, expiry, cvv, otp, scenario)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, targetURL, http.StatusSeeOther)
}

// GetSettlement handles GET /settlement
func (h *CardHandler) GetSettlement(w http.ResponseWriter, r *http.Request) {
	report, err := h.svc.GetSettlementReport(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"provider": "mock-card",
		"total":    len(report),
		"records":  report,
	})
}
