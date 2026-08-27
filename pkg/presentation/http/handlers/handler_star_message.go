package handlers

import (
	"net/http"

	"github.com/rs/zerolog/hlog"

	"wa-api/pkg/application/usecase/chat"
	"wa-api/pkg/domain"
	customhttp "wa-api/pkg/presentation/http"
	dtomessage "wa-api/pkg/presentation/http/dto/message"
)

// StarMessageHandler handles POST /message/star.
type StarMessageHandler struct{ usecase *chat.StarMessageUseCase }

func NewStarMessageHandler(uc *chat.StarMessageUseCase) *StarMessageHandler {
	return &StarMessageHandler{uc}
}

func (h *StarMessageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const route = "/message/star"

	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.StarMessageRequest
	if err := decodeRequest(w, r, &req); err != nil {
		if requestAnswered(err) {
			return
		}
		hlog.FromRequest(r).Warn().Err(errDecodePayload).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), id, req)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", route).Msg("request failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtomessage.PresentStarMessage(rsp), nil)
}
