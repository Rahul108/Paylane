package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"payment-core/internal/service"
	"paylane-jwe"
)

// InitiateBinding handles POST /bindings/initiate
func (h *PaymentHandler) InitiateBinding(w http.ResponseWriter, r *http.Request) {
	var bodyBytes []byte
	if payload := jwe.GetDecryptedBody(r.Context()); len(payload) > 0 {
		bodyBytes = payload
	} else {
		var reqMap map[string]any
		_ = json.NewDecoder(r.Body).Decode(&reqMap)
		bodyBytes, _ = json.Marshal(reqMap)
	}

	var req service.InitiateBindingRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		h.respond(w, r, http.StatusBadRequest, map[string]string{"error": "malformed request payload"})
		return
	}

	resp, err := h.svc.InitiateBinding(r.Context(), req)
	if err != nil {
		h.respond(w, r, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	h.respond(w, r, http.StatusCreated, resp)
}

// ConfirmBinding handles POST /bindings/confirm
func (h *PaymentHandler) ConfirmBinding(w http.ResponseWriter, r *http.Request) {
	var bodyBytes []byte
	if payload := jwe.GetDecryptedBody(r.Context()); len(payload) > 0 {
		bodyBytes = payload
	} else {
		var reqMap map[string]any
		_ = json.NewDecoder(r.Body).Decode(&reqMap)
		bodyBytes, _ = json.Marshal(reqMap)
	}

	var req service.ConfirmBindingRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		h.respond(w, r, http.StatusBadRequest, map[string]string{"error": "malformed request payload"})
		return
	}

	binding, err := h.svc.ConfirmBinding(r.Context(), req)
	if err != nil {
		h.respond(w, r, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	h.respond(w, r, http.StatusCreated, binding)
}

// GetCustomerBindings handles GET /customers/{customer_id}/bindings
func (h *PaymentHandler) GetCustomerBindings(w http.ResponseWriter, r *http.Request) {
	customerID := r.PathValue("customer_id")
	if customerID == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) >= 2 {
			customerID = parts[1]
		}
	}

	bindings, err := h.svc.GetCustomerBindings(r.Context(), customerID)
	if err != nil {
		h.respond(w, r, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	h.respond(w, r, http.StatusOK, map[string]any{
		"customer_id": customerID,
		"bindings":    bindings,
	})
}

// Unbind handles POST /bindings/{id}/unbind
func (h *PaymentHandler) Unbind(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) >= 2 {
			id = parts[1]
		}
	}

	err := h.svc.Unbind(r.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrBindingNotFound) {
			h.respond(w, r, http.StatusNotFound, map[string]string{"error": "binding not found"})
			return
		}
		h.respond(w, r, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	h.respond(w, r, http.StatusOK, map[string]string{"status": "UNBOUND"})
}
