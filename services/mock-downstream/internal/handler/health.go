package handler

import (
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"time"

	"paylane-jwe"
)

type HealthHandler struct {
	serviceName string
	signKey     *rsa.PrivateKey
	registry    *jwe.KeyRegistry
}

func NewHealthHandler(serviceName string, signKey *rsa.PrivateKey, registry *jwe.KeyRegistry) *HealthHandler {
	return &HealthHandler{
		serviceName: serviceName,
		signKey:     signKey,
		registry:    registry,
	}
}

func (h *HealthHandler) Livez(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"service": h.serviceName,
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	claims := jwe.GetClaims(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"missing claims"}`, http.StatusUnauthorized)
		return
	}

	respPayload := map[string]any{
		"status":    "healthy",
		"service":   h.serviceName,
		"caller":    claims.Issuer,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	respBytes, err := json.Marshal(respPayload)
	if err != nil {
		http.Error(w, `{"error":"marshal error"}`, http.StatusInternalServerError)
		return
	}

	callerEncKey, err := h.registry.GetEncPublicKey(claims.Issuer)
	if err != nil {
		http.Error(w, `{"error":"unknown caller encryption key"}`, http.StatusBadRequest)
		return
	}

	sealed, err := jwe.Seal(respBytes, h.signKey, callerEncKey, h.serviceName, claims.Issuer, 60*time.Second)
	if err != nil {
		http.Error(w, `{"error":"failed to seal response"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/jose")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(sealed))
}
