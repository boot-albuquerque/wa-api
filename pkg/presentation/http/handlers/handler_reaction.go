package handlers

import (
	"net/http"

	customhttp "wa-api/pkg/presentation/http"
	dtomessage "wa-api/pkg/presentation/http/dto/message"

	"wa-api/pkg/application/usecase/message"

	"github.com/rs/zerolog/hlog"
)

type ReactHandler struct{ uc *message.ReactUseCase }

func NewReactHandler(uc *message.ReactUseCase) *ReactHandler {
	return &ReactHandler{uc: uc}
}
func (h *ReactHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const route = "/chats/react"

	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req dtomessage.ReactRequest
	if err := decodeRequest(w, r, &req); err != nil {
		if requestAnswered(err) {
			return
		}
		hlog.FromRequest(r).Warn().Err(errDecodePayload).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.uc.Execute(r.Context(), id, req.ToDomain())
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", route).Msg("request failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtomessage.PresentSendReaction(rsp), nil)
}

// ReactionHandlers agrupa os handlers de reacao a mensagem (/chat/react).
type ReactionHandlers struct {
	React *ReactHandler
}
