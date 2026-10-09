package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"mock-mfs/internal/service"
	"paylane-jwe"
)

type MFSHandler struct {
	svc *service.Service
}

func NewMFSHandler(svc *service.Service) *MFSHandler {
	return &MFSHandler{svc: svc}
}

// CreateSession handles POST /sessions (JWE or plain JSON)
func (h *MFSHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
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
func (h *MFSHandler) GetSession(w http.ResponseWriter, r *http.Request) {
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
func (h *MFSHandler) RenderCheckout(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "missing session_id query parameter", http.StatusBadRequest)
		return
	}

	sess, err := h.svc.GetSession(r.Context(), sessionID)
	if err != nil {
		http.Error(w, "invalid or expired session", http.StatusNotFound)
		return
	}

	amountFormatted := fmt.Sprintf("%.2f %s", float64(sess.Amount)/100.0, sess.Currency)

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Paylane Mock MFS Hosted Checkout</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; }
    body { background: #0f172a; color: #f8fafc; display: flex; align-items: center; justify-content: center; min-height: 100vh; padding: 1.5rem; }
    .card { background: #1e293b; border: 1px solid rgba(255, 255, 255, 0.1); border-radius: 1rem; width: 100%%; max-width: 440px; padding: 2rem; box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.5); }
    .badge { display: inline-block; background: rgba(59, 130, 246, 0.2); color: #60a5fa; font-size: 0.75rem; font-weight: 700; padding: 0.25rem 0.6rem; border-radius: 9999px; margin-bottom: 0.75rem; text-transform: uppercase; }
    h1 { font-size: 1.25rem; font-weight: 700; margin-bottom: 0.25rem; }
    p.sub { font-size: 0.85rem; color: #94a3b8; margin-bottom: 1.5rem; }
    .amount-box { background: rgba(15, 23, 42, 0.6); border: 1px solid rgba(255, 255, 255, 0.05); border-radius: 0.5rem; padding: 1rem; margin-bottom: 1.5rem; text-align: center; }
    .amount-title { font-size: 0.75rem; color: #94a3b8; text-transform: uppercase; letter-spacing: 0.05em; }
    .amount-val { font-size: 1.75rem; font-weight: 800; color: #38bdf8; margin-top: 0.25rem; }
    .field { margin-bottom: 1.25rem; }
    label { display: block; font-size: 0.8rem; font-weight: 600; color: #cbd5e1; margin-bottom: 0.35rem; }
    input, select { width: 100%%; background: #0f172a; border: 1px solid rgba(255, 255, 255, 0.15); border-radius: 0.5rem; padding: 0.65rem 0.85rem; color: #fff; font-size: 0.95rem; outline: none; transition: border-color 0.2s; }
    input:focus, select:focus { border-color: #3b82f6; }
    .btn { width: 100%%; background: #2563eb; color: #fff; border: none; border-radius: 0.5rem; padding: 0.85rem; font-size: 1rem; font-weight: 700; cursor: pointer; transition: background 0.2s; box-shadow: 0 4px 12px rgba(37, 99, 235, 0.3); }
    .btn:hover { background: #1d4ed8; }
    .footer { font-size: 0.75rem; color: #64748b; text-align: center; margin-top: 1.25rem; }
  </style>
</head>
<body>
  <div class="card">
    <span class="badge">MFS Payment Gateway</span>
    <h1>Mock MFS Checkout</h1>
    <p class="sub">Session ID: <code>%s</code></p>

    <div class="amount-box">
      <div class="amount-title">Total Payable Amount</div>
      <div class="amount-val">%s</div>
    </div>

    <form method="POST" action="/checkout/confirm">
      <input type="hidden" name="session_id" value="%s">

      <div class="field">
        <label for="msisdn">Mobile Wallet Account Number</label>
        <input type="text" id="msisdn" name="msisdn" value="01700000000" required>
      </div>

      <div class="field">
        <label for="pin">Wallet PIN (Default: 1234)</label>
        <input type="password" id="pin" name="pin" value="1234" maxlength="4" required>
      </div>

      <div class="field">
        <label for="otp">One-Time Password / OTP (Default: 123456)</label>
        <input type="text" id="otp" name="otp" value="123456" maxlength="6" required>
      </div>

      <div class="field">
        <label for="scenario">Scenario Simulation</label>
        <select id="scenario" name="scenario">
          <option value="SUCCESS">SUCCESS (Approve & Capture)</option>
          <option value="INSUFFICIENT_FUNDS">INSUFFICIENT_FUNDS (Decline)</option>
          <option value="WRONG_PIN">WRONG_PIN (Decline)</option>
          <option value="WRONG_OTP">WRONG_OTP (Decline)</option>
          <option value="USER_ABANDONS">USER_ABANDONS (No Webhook)</option>
        </select>
      </div>

      <button type="submit" class="btn">Confirm Payment</button>
    </form>

    <div class="footer">Paylane Local Mock Gateway • JWE Mesh Active</div>
  </div>
</body>
</html>`, sess.ID, amountFormatted, sess.ID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

// ConfirmCheckout handles POST /checkout/confirm
func (h *MFSHandler) ConfirmCheckout(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	sessionID := r.FormValue("session_id")
	msisdn := r.FormValue("msisdn")
	pin := r.FormValue("pin")
	otp := r.FormValue("otp")
	scenario := r.FormValue("scenario")

	if sessionID == "" {
		http.Error(w, "missing session_id", http.StatusBadRequest)
		return
	}

	targetURL, err := h.svc.ConfirmPayment(r.Context(), sessionID, msisdn, pin, otp, scenario)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, targetURL, http.StatusSeeOther)
}

// GetSettlement handles GET /settlement
func (h *MFSHandler) GetSettlement(w http.ResponseWriter, r *http.Request) {
	report, err := h.svc.GetSettlementReport(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"provider": "mock-mfs",
		"total":    len(report),
		"records":  report,
	})
}
