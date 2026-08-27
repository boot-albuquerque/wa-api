package handlers

import (
	"net/http"

	"wa-api/pkg/domain"
	customhttp "wa-api/pkg/presentation/http"
	dtomessage "wa-api/pkg/presentation/http/dto/message"

	"wa-api/pkg/application/usecase/message"

	"github.com/rs/zerolog/hlog"
)

type SendPresenceHandler struct{ uc *message.SendPresenceUseCase }

func NewSendPresenceHandler(uc *message.SendPresenceUseCase) *SendPresenceHandler {
	return &SendPresenceHandler{uc: uc}
}
func (h *SendPresenceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const route = "/user/presence"

	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.SendPresenceRequest
	if err := decodeRequest(w, r, &req); err != nil {
		if requestAnswered(err) {
			return
		}
		hlog.FromRequest(r).Warn().Err(errDecodePayload).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	if err := h.uc.Execute(r.Context(), id, req); err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", route).Msg("request failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtomessage.PresentAction(dtomessage.DetailsPresenceSent), nil)
}

type SubscribePresenceHandler struct {
	uc *message.SubscribePresenceUseCase
}

func NewSubscribePresenceHandler(uc *message.SubscribePresenceUseCase) *SubscribePresenceHandler {
	return &SubscribePresenceHandler{uc: uc}
}
func (h *SubscribePresenceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const route = "/user/presence/subscribe"

	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.SubscribePresenceRequest
	if err := decodeRequest(w, r, &req); err != nil {
		if requestAnswered(err) {
			return
		}
		hlog.FromRequest(r).Warn().Err(errDecodePayload).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	if err := h.uc.Execute(r.Context(), id, req); err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", route).Msg("request failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtomessage.PresentAction(dtomessage.DetailsPresenceSubscribed), nil)
}

type ChatPresenceHandler struct{ uc *message.ChatPresenceUseCase }

func NewChatPresenceHandler(uc *message.ChatPresenceUseCase) *ChatPresenceHandler {
	return &ChatPresenceHandler{uc: uc}
}
func (h *ChatPresenceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const route = "/chat/presence"

	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.ChatPresenceRequest
	if err := decodeRequest(w, r, &req); err != nil {
		if requestAnswered(err) {
			return
		}
		hlog.FromRequest(r).Warn().Err(errDecodePayload).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	if err := h.uc.Execute(r.Context(), id, req); err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", route).Msg("request failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtomessage.PresentAction(dtomessage.DetailsChatPresenceSent), nil)
}

type MarkReadHandler struct{ uc *message.MarkReadUseCase }

func NewMarkReadHandler(uc *message.MarkReadUseCase) *MarkReadHandler {
	return &MarkReadHandler{uc: uc}
}
func (h *MarkReadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const route = "/chat/markread"

	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.MarkReadRequest
	if err := decodeRequest(w, r, &req); err != nil {
		if requestAnswered(err) {
			return
		}
		hlog.FromRequest(r).Warn().Err(errDecodePayload).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	if err := h.uc.Execute(r.Context(), id, req); err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", route).Msg("request failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtomessage.PresentAction(dtomessage.DetailsMessageMarkedRead), nil)
}

// PresenceHandlers agrupa os handlers de presenca (/user/presence,
// /user/presence/subscribe, /chat/presence) e de marcacao de leitura
// (/chat/markread).
type PresenceHandlers struct {
	Send      *SendPresenceHandler
	Subscribe *SubscribePresenceHandler
	Chat      *ChatPresenceHandler
	MarkRead  *MarkReadHandler
}
