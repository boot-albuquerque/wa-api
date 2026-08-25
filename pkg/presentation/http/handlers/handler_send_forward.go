package handlers

import (
	"net/http"

	customhttp "wa-api/pkg/presentation/http"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	"wa-api/pkg/application/usecase/message"

	"github.com/rs/zerolog/hlog"
)

// SendForwardHandler is the HTTP handler for POST /chat/send/forward.
type SendForwardHandler struct {
	usecase *message.SendForwardUseCase
}

// NewSendForwardHandler creates the handler with the injected use case.
func NewSendForwardHandler(uc *message.SendForwardUseCase) *SendForwardHandler {
	return &SendForwardHandler{usecase: uc}
}

// ServeHTTP implements http.Handler for POST /chat/send/forward.
func (h *SendForwardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const route = "/chat/send/forward"

	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}

	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().Err(errMissingSessionID).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
		return
	}

	var req domain.SendForwardRequest
	if err := domain.DecodeRequest(r.Body, &req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
		return
	}

	result, err := h.usecase.Execute(r.Context(), txtID, req)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", route).Msg("request failed")
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
		return
	}

	customhttp.RespondJSON(w, http.StatusOK, result, nil)
}
