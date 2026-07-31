package http

import (
	"context"
	"fmt"
	"net/http"

	"disparazap/internal/application/port"
)

// Context key "userinfo" é injetada pelo middleware authalice do upstream.
// O valor é um struct com método Get(key string) string.
// Ambos package main e package http referenciam a mesma string key.
type userInfo interface {
	Get(key string) string
}

// ProfileUseCase define o contrato de uso para obtenção de perfil.
type ProfileUseCase interface {
	Execute(ctx context.Context, txtID string) (string, error)
}

// ProfileHandler é o handler HTTP para GET /session/profile.
type ProfileHandler struct {
	usecase ProfileUseCase
}

// NewProfileHandler cria o handler com o usecase injetado.
func NewProfileHandler(uc ProfileUseCase) *ProfileHandler {
	return &ProfileHandler{usecase: uc}
}

// ServeHTTP implementa http.Handler para GET /session/profile.
// Extrai txtID do contexto (injetado pelo middleware authalice do upstream).
func (h *ProfileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, ok := r.Context().Value(port.UserInfoKey).(userInfo)
	if !ok || info == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	txtID := info.Get("Id")
	if txtID == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}

	response, err := h.usecase.Execute(r.Context(), txtID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, response)
}
