package handlers

import (
	"encoding/json"
	"net/http"

	customhttp "wa-api/pkg/presentation/http"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	"wa-api/pkg/application/usecase/message"

	"github.com/rs/zerolog/hlog"
)

// SendTemplateHandler é o handler HTTP para POST /chat/send/template.
type SendTemplateHandler struct {
	usecase *message.SendTemplateUseCase
}

// NewSendTemplateHandler cria o handler com o usecase injetado.
func NewSendTemplateHandler(uc *message.SendTemplateUseCase) *SendTemplateHandler {
	return &SendTemplateHandler{usecase: uc}
}

// ServeHTTP implementa http.Handler para POST /chat/send/template.
func (h *SendTemplateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).
			Str("path", r.URL.Path).
			Msg("send template rejected at the auth boundary")
		customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}

	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().Err(errMissingSessionID).
			Str("path", r.URL.Path).
			Msg("send template rejected without session id")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
		return
	}

	var req domain.SendTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).
			Str("path", r.URL.Path).
			Str("user_id", txtID).
			Msg("send template payload could not be decoded")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
		return
	}

	result, err := h.usecase.Execute(r.Context(), txtID, req)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).
			Str("path", r.URL.Path).
			Str("user_id", txtID).
			Msg("send template use case failed")
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
		return
	}

	customhttp.RespondJSON(w, http.StatusOK, result, nil)
}
