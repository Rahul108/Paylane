package handler

import (
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"orchestrator/internal/service"
	"paylane-jwe"
)

type JourneyHandler struct {
	serviceName string
	signKey     *rsa.PrivateKey
	registry    *jwe.KeyRegistry
	svc         *service.Service
}

func NewJourneyHandler(serviceName string, signKey *rsa.PrivateKey, registry *jwe.KeyRegistry, svc *service.Service) *JourneyHandler {
	return &JourneyHandler{
		serviceName: serviceName,
		signKey:     signKey,
		registry:    registry,
		svc:         svc,
	}
}

func (h *JourneyHandler) respond(w http.ResponseWriter, r *http.Request, statusCode int, data any) {
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

func (h *JourneyHandler) StartJourney(w http.ResponseWriter, r *http.Request) {
	var bodyBytes []byte
	if payload := jwe.GetDecryptedBody(r.Context()); len(payload) > 0 {
		bodyBytes = payload
	} else {
		var reqMap map[string]any
		_ = json.NewDecoder(r.Body).Decode(&reqMap)
		bodyBytes, _ = json.Marshal(reqMap)
	}

	var req service.StartJourneyRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		h.respond(w, r, http.StatusBadRequest, map[string]string{"error": "malformed request"})
		return
	}

	resp, err := h.svc.StartJourney(r.Context(), req)
	if err != nil {
		h.respond(w, r, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	h.respond(w, r, http.StatusCreated, resp)
}

func (h *JourneyHandler) GetJourney(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) >= 2 {
			id = parts[1]
		}
	}

	j, err := h.svc.GetJourney(r.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrJourneyNotFound) {
			h.respond(w, r, http.StatusNotFound, map[string]string{"error": "journey not found"})
			return
		}
		h.respond(w, r, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	h.respond(w, r, http.StatusOK, j)
}

func (h *JourneyHandler) HandleEvent(w http.ResponseWriter, r *http.Request) {
	var bodyBytes []byte
	if payload := jwe.GetDecryptedBody(r.Context()); len(payload) > 0 {
		bodyBytes = payload
	} else {
		var reqMap map[string]any
		_ = json.NewDecoder(r.Body).Decode(&reqMap)
		bodyBytes, _ = json.Marshal(reqMap)
	}

	var eventData struct {
		EventType string `json:"event_type"`
		PaymentID string `json:"payment_id"`
		Status    string `json:"status"`
		Reason    string `json:"reason"`
	}
	_ = json.Unmarshal(bodyBytes, &eventData)

	var rawMap map[string]any
	_ = json.Unmarshal(bodyBytes, &rawMap)

	eventType := eventData.EventType
	if eventType == "" {
		eventType = "payment.captured"
		if strings.ToUpper(eventData.Status) == "FAILED" {
			eventType = "payment.failed"
		}
	}

	err := h.svc.HandleEvent(r.Context(), eventType, eventData.PaymentID, rawMap)
	if err != nil {
		h.respond(w, r, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	h.respond(w, r, http.StatusOK, map[string]string{"status": "processed"})
}

func (h *JourneyHandler) RetryStep(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	journeyID := ""
	stepName := ""
	if len(parts) >= 4 {
		journeyID = parts[1]
		stepName = parts[3]
	}

	err := h.svc.RetryStep(r.Context(), journeyID, stepName)
	if err != nil {
		h.respond(w, r, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	h.respond(w, r, http.StatusOK, map[string]string{"status": "retrying"})
}
