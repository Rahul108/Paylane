package handler

import (
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"paylane-jwe"
	"payment-core/internal/model"
	"payment-core/internal/service"
	"payment-core/internal/statemachine"
)

type PaymentHandler struct {
	serviceName string
	signKey     *rsa.PrivateKey
	registry    *jwe.KeyRegistry
	svc         *service.PaymentService
}

func NewPaymentHandler(serviceName string, signKey *rsa.PrivateKey, registry *jwe.KeyRegistry, svc *service.PaymentService) *PaymentHandler {
	return &PaymentHandler{
		serviceName: serviceName,
		signKey:     signKey,
		registry:    registry,
		svc:         svc,
	}
}

// respond responds with JWE if caller provided claims, or plain JSON otherwise.
func (h *PaymentHandler) respond(w http.ResponseWriter, r *http.Request, statusCode int, data any) {
	claims := jwe.GetClaims(r.Context())
	dataBytes, err := json.Marshal(data)
	if err != nil {
		http.Error(w, `{"error":"failed to marshal response"}`, http.StatusInternalServerError)
		return
	}

	if claims != nil && h.registry != nil && h.signKey != nil {
		callerEncKey, err := h.registry.GetEncPublicKey(claims.Issuer)
		if err == nil {
			sealed, err := jwe.Seal(dataBytes, h.signKey, callerEncKey, h.serviceName, claims.Issuer, 60*time.Second)
			if err == nil {
				w.Header().Set("Content-Type", "application/jose")
				w.WriteHeader(statusCode)
				_, _ = w.Write([]byte(sealed))
				return
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(dataBytes)
}

// InitiatePayment handles POST /payments
func (h *PaymentHandler) InitiatePayment(w http.ResponseWriter, r *http.Request) {
	var bodyBytes []byte
	var err error

	// If request came through JWE middleware, decrypted payload is in context or body
	if payload := jwe.GetDecryptedBody(r.Context()); len(payload) > 0 {
		bodyBytes = payload
	} else {
		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
			return
		}
		bodyBytes, _ = json.Marshal(reqBody)
	}

	var req service.InitiatePaymentRequest
	if err = json.Unmarshal(bodyBytes, &req); err != nil {
		http.Error(w, `{"error":"malformed request payload"}`, http.StatusBadRequest)
		return
	}

	resp, err := h.svc.InitiatePayment(r.Context(), req)
	if err != nil {
		if errors.Is(err, service.ErrInvalidRequest) {
			h.respond(w, r, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		h.respond(w, r, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	h.respond(w, r, http.StatusCreated, resp)
}

// GetPayment handles GET /payments/{id}
func (h *PaymentHandler) GetPayment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		id = strings.TrimPrefix(r.URL.Path, "/payments/")
	}

	p, err := h.svc.GetPayment(r.Context(), id)
	if err != nil {
		if errors.Is(err, statemachine.ErrPaymentNotFound) {
			h.respond(w, r, http.StatusNotFound, map[string]string{"error": "payment not found"})
			return
		}
		h.respond(w, r, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	h.respond(w, r, http.StatusOK, p)
}

// GetPaymentEvents handles GET /payments/{id}/events
func (h *PaymentHandler) GetPaymentEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) >= 2 {
			id = parts[1]
		}
	}

	events, err := h.svc.GetPaymentEvents(r.Context(), id)
	if err != nil {
		h.respond(w, r, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	h.respond(w, r, http.StatusOK, map[string]any{
		"payment_id": id,
		"events":     events,
	})
}

// GetPaymentLedger handles GET /payments/{id}/ledger
func (h *PaymentHandler) GetPaymentLedger(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) >= 2 {
			id = parts[1]
		}
	}

	entries, err := h.svc.GetLedgerEntries(r.Context(), id)
	if err != nil {
		h.respond(w, r, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	h.respond(w, r, http.StatusOK, map[string]any{
		"payment_id": id,
		"entries":    entries,
	})
}

// CreateRefund handles POST /payments/{id}/refunds
func (h *PaymentHandler) CreateRefund(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) >= 2 {
			id = parts[1]
		}
	}

	var req struct {
		IdempotencyKey string `json:"idempotency_key"`
		Amount         int64  `json:"amount"`
		Reason         string `json:"reason"`
	}

	var bodyBytes []byte
	if payload := jwe.GetDecryptedBody(r.Context()); len(payload) > 0 {
		bodyBytes = payload
	} else {
		_ = json.NewDecoder(r.Body).Decode(&req)
		bodyBytes, _ = json.Marshal(req)
	}
	_ = json.Unmarshal(bodyBytes, &req)

	if req.IdempotencyKey == "" {
		req.IdempotencyKey = model.NewULID()
	}
	if req.Reason == "" {
		req.Reason = "CUSTOMER_REQUESTED"
	}

	ref, err := h.svc.RefundPayment(r.Context(), id, req.IdempotencyKey, req.Amount, req.Reason)
	if err != nil {
		h.respond(w, r, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	h.respond(w, r, http.StatusCreated, ref)
}

// HandleWebhook handles POST /webhooks/{provider}
func (h *PaymentHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if provider == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) >= 2 {
			provider = parts[1]
		}
	}

	var webhookData struct {
		EventID           string `json:"event_id"`
		EventType         string `json:"event_type"` // CHARGE_SUCCESS, CHARGE_FAILED, etc.
		PaymentID         string `json:"payment_id"`
		ProviderReference string `json:"provider_reference"`
		Reason            string `json:"reason"`
	}

	var rawPayload []byte
	if payload := jwe.GetDecryptedBody(r.Context()); len(payload) > 0 {
		rawPayload = payload
	} else {
		var bodyMap map[string]any
		_ = json.NewDecoder(r.Body).Decode(&bodyMap)
		rawPayload, _ = json.Marshal(bodyMap)
	}

	if err := json.Unmarshal(rawPayload, &webhookData); err != nil {
		http.Error(w, `{"error":"invalid webhook payload"}`, http.StatusBadRequest)
		return
	}

	if webhookData.EventID == "" {
		webhookData.EventID = model.NewULID()
	}

	err := h.svc.ProcessWebhook(r.Context(), provider, webhookData.EventID, rawPayload, webhookData.PaymentID, webhookData.EventType, webhookData.Reason, webhookData.ProviderReference)
	if err != nil {
		if errors.Is(err, service.ErrDuplicateWebhook) {
			h.respond(w, r, http.StatusOK, map[string]string{"status": "already_processed"})
			return
		}
		h.respond(w, r, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	h.respond(w, r, http.StatusOK, map[string]string{"status": "processed"})
}
