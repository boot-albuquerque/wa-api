package handlers

import (
	"net/http"

	customhttp "wa-api/pkg/presentation/http"
	dtomessage "wa-api/pkg/presentation/http/dto/message"

	appport "wa-api/pkg/application/contracts"

	"wa-api/pkg/application/usecase/message"

	"github.com/rs/zerolog/hlog"
)

// SendButtonsHandler é o handler HTTP para POST /chat/send/buttons.
//
// Saiu de handler_interactive.go no CAP-21, junto com a migração para
// port.InteractiveMessenger: aquele arquivo agrupa os handlers que ainda
// dependem de port.MessageComposer e respondem "validated" sem enviar nada,
// e este deixou de ser um deles. Mesma estrutura de
// handler_message_template.go.
type SendButtonsHandler struct {
	usecase *message.SendButtonsUseCase
}

// NewSendButtonsHandler cria o handler com o usecase injetado.
func NewSendButtonsHandler(uc *message.SendButtonsUseCase) *SendButtonsHandler {
	return &SendButtonsHandler{usecase: uc}
}

// ServeHTTP implementa http.Handler para POST /chat/send/buttons.
func (h *SendButtonsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const route = "/chat/send/buttons"

	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).
			Str("route", route).
			Msg("send buttons rejected at the auth boundary")
		customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}

	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().Err(errMissingSessionID).
			Str("route", route).
			Msg("send buttons rejected without session id")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
		return
	}

	var req dtomessage.SendButtonsRequest
	if err := decodeRequest(w, r, &req); err != nil {
		if requestAnswered(err) {
			return
		}
		hlog.FromRequest(r).Warn().Err(err).
			Str("route", route).
			Str("user_id", txtID).
			Msg("send buttons payload could not be decoded")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
		return
	}

	result, err := h.usecase.Execute(r.Context(), txtID, req.ToDomain())
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).
			Str("route", route).
			Str("user_id", txtID).
			Msg("send buttons use case failed")
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
		return
	}

	customhttp.RespondJSON(w, http.StatusOK, dtomessage.PresentSendButtons(result), nil)
}
