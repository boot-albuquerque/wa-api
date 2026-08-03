package http

import (
	"context"
	"encoding/json"
	"net/http"

	appport "wa-api/pkg/application/contracts"

	"github.com/rs/zerolog/hlog"
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
	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		hlog.FromRequest(r).Warn().
			Str("path", r.URL.Path).
			Msg("profile request without user info in context")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().
			Str("path", r.URL.Path).
			Msg("profile request with empty session id")
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}

	response, err := h.usecase.Execute(r.Context(), txtID)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).
			Str("user_id", txtID).
			Msg("get profile use case failed")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// RespondJSON envelopa em {code,data,success} (ADR-002), igual às demais
	// rotas de /session/*. Antes este handler escrevia `response` cru no
	// ResponseWriter — o cliente wa-worker (que desembrulha body.data como
	// todo o resto da API) sempre lia data=undefined e devolvia pushname/
	// avatar vazios, mesmo com o whatsmeow retornando os campos certos.
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	RespondJSON(w, http.StatusOK, json.RawMessage(response), nil)
}
