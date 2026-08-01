package handlers

import (
	"encoding/json"
	"net/http"

	"wa-api/pkg/domain"
	customhttp "wa-api/pkg/presentation/http"

	"wa-api/pkg/application/usecase/message"
)

type SendPresenceHandler struct{ uc *message.SendPresenceUseCase }

func NewSendPresenceHandler(uc *message.SendPresenceUseCase) *SendPresenceHandler {
	return &SendPresenceHandler{uc: uc}
}
func (h *SendPresenceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.SendPresenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	if err := h.uc.Execute(r.Context(), id, req); err != nil {
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, map[string]string{"Details": "Presence sent"}, nil)
}

type SubscribePresenceHandler struct {
	uc *message.SubscribePresenceUseCase
}

func NewSubscribePresenceHandler(uc *message.SubscribePresenceUseCase) *SubscribePresenceHandler {
	return &SubscribePresenceHandler{uc: uc}
}
func (h *SubscribePresenceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.SubscribePresenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	if err := h.uc.Execute(r.Context(), id, req); err != nil {
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, map[string]string{"Details": "Presence subscription updated"}, nil)
}

type ChatPresenceHandler struct{ uc *message.ChatPresenceUseCase }

func NewChatPresenceHandler(uc *message.ChatPresenceUseCase) *ChatPresenceHandler {
	return &ChatPresenceHandler{uc: uc}
}
func (h *ChatPresenceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.ChatPresenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	if err := h.uc.Execute(r.Context(), id, req); err != nil {
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, map[string]string{"Details": "Chat presence sent"}, nil)
}

type MarkReadHandler struct{ uc *message.MarkReadUseCase }

func NewMarkReadHandler(uc *message.MarkReadUseCase) *MarkReadHandler {
	return &MarkReadHandler{uc: uc}
}
func (h *MarkReadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.MarkReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	if err := h.uc.Execute(r.Context(), id, req); err != nil {
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, map[string]string{"Details": "Message marked as read"}, nil)
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
