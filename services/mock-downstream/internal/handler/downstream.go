package handler

import (
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"time"

	"mock-downstream/internal/service"
	"paylane-jwe"
)

type DownstreamHandler struct {
	serviceName string
	signKey     *rsa.PrivateKey
	registry    *jwe.KeyRegistry
	svc         *service.Service
}

func NewDownstreamHandler(serviceName string, signKey *rsa.PrivateKey, registry *jwe.KeyRegistry, svc *service.Service) *DownstreamHandler {
	return &DownstreamHandler{
		serviceName: serviceName,
		signKey:     signKey,
		registry:    registry,
		svc:         svc,
	}
}

func (h *DownstreamHandler) respond(w http.ResponseWriter, r *http.Request, statusCode int, data any) {
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

func (h *DownstreamHandler) Recharge(w http.ResponseWriter, r *http.Request) {
	var bodyBytes []byte
	if payload := jwe.GetDecryptedBody(r.Context()); len(payload) > 0 {
		bodyBytes = payload
	} else {
		var reqMap map[string]any
		_ = json.NewDecoder(r.Body).Decode(&reqMap)
		bodyBytes, _ = json.Marshal(reqMap)
	}

	var req service.RechargeRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		h.respond(w, r, http.StatusBadRequest, map[string]string{"error": "malformed request"})
		return
	}

	resp, err := h.svc.Recharge(r.Context(), req)
	if err != nil {
		h.respond(w, r, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	h.respond(w, r, http.StatusOK, resp)
}

func (h *DownstreamHandler) Cashback(w http.ResponseWriter, r *http.Request) {
	var bodyBytes []byte
	if payload := jwe.GetDecryptedBody(r.Context()); len(payload) > 0 {
		bodyBytes = payload
	} else {
		var reqMap map[string]any
		_ = json.NewDecoder(r.Body).Decode(&reqMap)
		bodyBytes, _ = json.Marshal(reqMap)
	}

	var req service.CashbackRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		h.respond(w, r, http.StatusBadRequest, map[string]string{"error": "malformed request"})
		return
	}

	resp, err := h.svc.Cashback(r.Context(), req)
	if err != nil {
		h.respond(w, r, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	h.respond(w, r, http.StatusOK, resp)
}

func (h *DownstreamHandler) Subscribe(w http.ResponseWriter, r *http.Request) {
	var bodyBytes []byte
	if payload := jwe.GetDecryptedBody(r.Context()); len(payload) > 0 {
		bodyBytes = payload
	} else {
		var reqMap map[string]any
		_ = json.NewDecoder(r.Body).Decode(&reqMap)
		bodyBytes, _ = json.Marshal(reqMap)
	}

	var req service.SubscribeRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		h.respond(w, r, http.StatusBadRequest, map[string]string{"error": "malformed request"})
		return
	}

	resp, err := h.svc.Subscribe(r.Context(), req)
	if err != nil {
		h.respond(w, r, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	h.respond(w, r, http.StatusOK, resp)
}
