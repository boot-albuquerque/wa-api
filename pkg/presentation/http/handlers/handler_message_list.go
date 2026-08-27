package handlers

import (
	"net/http"

	customhttp "wa-api/pkg/presentation/http"
	dtomessage "wa-api/pkg/presentation/http/dto/message"

	appport "wa-api/pkg/application/contracts"

	"wa-api/pkg/application/usecase/message"

	"github.com/rs/zerolog/hlog"
)

// SendListHandler é o handler HTTP para POST /chat/send/list.
//
// Saiu de handler_interactive.go no CAP-22, junto com a migração para
// port.SimpleMessenger: era o ÚLTIMO handler daquele arquivo que ainda
// respondia "validated" sem enviar nada. Mesma estrutura de
// handler_message_buttons.go/handler_message_template.go, inclusive no log
// do decode: erro CRU do decoder, e não o sentinela genérico
// errDecodePayload — fecha HOUSEKEEP F141 para esta rota (a última das duas
// que o achado apontava).
type SendListHandler struct {
	usecase *message.SendListUseCase
}

// NewSendListHandler cria o handler com o usecase injetado.
func NewSendListHandler(uc *message.SendListUseCase) *SendListHandler {
	return &SendListHandler{usecase: uc}
}

// ServeHTTP implementa http.Handler para POST /chat/send/list.
func (h *SendListHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const route = "/chat/send/list"

	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).
			Str("route", route).
			Msg("send list rejected at the auth boundary")
		customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}

	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().Err(errMissingSessionID).
			Str("route", route).
			Msg("send list rejected without session id")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
		return
	}

	var req dtomessage.SendListRequest
	if err := decodeRequest(w, r, &req); err != nil {
		if requestAnswered(err) {
			return
		}
		hlog.FromRequest(r).Warn().Err(err).
			Str("route", route).
			Str("user_id", txtID).
			Msg("send list payload could not be decoded")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
		return
	}

	result, err := h.usecase.Execute(r.Context(), txtID, req.ToDomain())
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).
			Str("route", route).
			Str("user_id", txtID).
			Msg("send list use case failed")
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
		return
	}

	customhttp.RespondJSON(w, http.StatusOK, dtomessage.PresentSendList(result), nil)
}
