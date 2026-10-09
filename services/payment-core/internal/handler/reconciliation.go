package handler

import (
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"payment-core/internal/service"
	"paylane-jwe"
)

type ReconciliationHandler struct {
	serviceName     string
	signKey         *rsa.PrivateKey
	registry        *jwe.KeyRegistry
	reconciliation  *service.ReconciliationService
	timeout         *service.TimeoutService
}

func NewReconciliationHandler(
	serviceName string,
	signKey *rsa.PrivateKey,
	registry *jwe.KeyRegistry,
	reconciliation *service.ReconciliationService,
	timeout *service.TimeoutService,
) *ReconciliationHandler {
	return &ReconciliationHandler{
		serviceName:    serviceName,
		signKey:        signKey,
		registry:       registry,
		reconciliation: reconciliation,
		timeout:        timeout,
	}
}

func (h *ReconciliationHandler) respond(w http.ResponseWriter, r *http.Request, statusCode int, data any) {
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

// RunReconciliation handles POST /reconciliation/run
func (h *ReconciliationHandler) RunReconciliation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider,omitempty"`
	}

	if payload := jwe.GetDecryptedBody(r.Context()); len(payload) > 0 {
		_ = json.Unmarshal(payload, &req)
	} else {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	report, err := h.reconciliation.RunReconciliation(r.Context(), req.Provider)
	if err != nil {
		h.respond(w, r, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	h.respond(w, r, http.StatusOK, report)
}

// GetReconciliationItems handles GET /reconciliation/items
func (h *ReconciliationHandler) GetReconciliationItems(w http.ResponseWriter, r *http.Request) {
	unresolvedOnly := r.URL.Query().Get("unresolved") == "true"
	items, err := h.reconciliation.GetReconciliationItems(r.Context(), unresolvedOnly)
	if err != nil {
		h.respond(w, r, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	h.respond(w, r, http.StatusOK, map[string]any{
		"items": items,
		"count": len(items),
	})
}

// ResolveItem handles POST /reconciliation/items/{id}/resolve
func (h *ReconciliationHandler) ResolveItem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) >= 3 {
			id = parts[2]
		}
	}

	err := h.reconciliation.ResolveReconciliationItem(r.Context(), id)
	if err != nil {
		h.respond(w, r, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	h.respond(w, r, http.StatusOK, map[string]string{"status": "resolved", "id": id})
}

// SweepTimeouts handles POST /timeouts/sweep
func (h *ReconciliationHandler) SweepTimeouts(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MaxAgeSeconds *int `json:"max_age_seconds,omitempty"`
	}

	if payload := jwe.GetDecryptedBody(r.Context()); len(payload) > 0 {
		_ = json.Unmarshal(payload, &req)
	} else {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	maxAge := 60 * time.Second
	if req.MaxAgeSeconds != nil {
		maxAge = time.Duration(*req.MaxAgeSeconds) * time.Second
	}

	res, err := h.timeout.Sweep(r.Context(), maxAge)
	if err != nil {
		h.respond(w, r, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	h.respond(w, r, http.StatusOK, res)
}
