package handlers

import (
	"encoding/json"
	"net/http"

	"wa-api/pkg/domain"
	customhttp "wa-api/pkg/presentation/http"

	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/application/usecase/user"
)

// --- Download handlers ---

type DownloadImageHandler struct{ uc *message.DownloadImageUseCase }

func NewDownloadImageHandler(uc *message.DownloadImageUseCase) *DownloadImageHandler {
	return &DownloadImageHandler{uc: uc}
}
func (h *DownloadImageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.DownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil {
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

type DownloadVideoHandler struct{ uc *message.DownloadVideoUseCase }

func NewDownloadVideoHandler(uc *message.DownloadVideoUseCase) *DownloadVideoHandler {
	return &DownloadVideoHandler{uc: uc}
}
func (h *DownloadVideoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.DownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil {
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

type DownloadAudioHandler struct{ uc *message.DownloadAudioUseCase }

func NewDownloadAudioHandler(uc *message.DownloadAudioUseCase) *DownloadAudioHandler {
	return &DownloadAudioHandler{uc: uc}
}
func (h *DownloadAudioHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.DownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil {
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

type DownloadDocumentHandler struct {
	uc *message.DownloadDocumentUseCase
}

func NewDownloadDocumentHandler(uc *message.DownloadDocumentUseCase) *DownloadDocumentHandler {
	return &DownloadDocumentHandler{uc: uc}
}
func (h *DownloadDocumentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.DownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil {
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

type DownloadStickerHandler struct {
	uc *message.DownloadStickerUseCase
}

func NewDownloadStickerHandler(uc *message.DownloadStickerUseCase) *DownloadStickerHandler {
	return &DownloadStickerHandler{uc: uc}
}
func (h *DownloadStickerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.DownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil {
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// --- Presence/Chat handlers ---

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

// --- User info handlers ---

type ReactHandler struct{ uc *message.ReactUseCase }

func NewReactHandler(uc *message.ReactUseCase) *ReactHandler {
	return &ReactHandler{uc: uc}
}
func (h *ReactHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.ReactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil {
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

type GetAvatarHandler struct{ uc *user.GetAvatarUseCase }

func NewGetAvatarHandler(uc *user.GetAvatarUseCase) *GetAvatarHandler {
	return &GetAvatarHandler{uc: uc}
}
func (h *GetAvatarHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.GetAvatarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil {
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

type GetContactsHandler struct{ uc *user.GetContactsUseCase }

func NewGetContactsHandler(uc *user.GetContactsUseCase) *GetContactsHandler {
	return &GetContactsHandler{uc: uc}
}
func (h *GetContactsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	rsp, err := h.uc.Execute(r.Context(), id, domain.GetContactsRequest{})
	if err != nil {
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

type GetUserInfoHandler struct{ uc *user.GetUserUseCase }

func NewGetUserInfoHandler(uc *user.GetUserUseCase) *GetUserInfoHandler {
	return &GetUserInfoHandler{uc: uc}
}
func (h *GetUserInfoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.CheckUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil {
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// ExtendedHandlers groups all handlers that were added during Phase 4 migration.
type ExtendedHandlers struct {
	DownloadImage     *DownloadImageHandler
	DownloadVideo     *DownloadVideoHandler
	DownloadAudio     *DownloadAudioHandler
	DownloadDocument  *DownloadDocumentHandler
	DownloadSticker   *DownloadStickerHandler
	SendPresence      *SendPresenceHandler
	SubscribePresence *SubscribePresenceHandler
	ChatPresence      *ChatPresenceHandler
	MarkRead          *MarkReadHandler
	React             *ReactHandler
	GetAvatar         *GetAvatarHandler
	GetContacts       *GetContactsHandler
	GetUserInfo       *GetUserInfoHandler
}
