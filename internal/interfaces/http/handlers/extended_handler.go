package handlers

import (
	"encoding/json"
	"net/http"

	customhttp "wuzapi/internal/interfaces/http"
	"wuzapi/internal/application/usecase"
	"wuzapi/internal/domain"
)

// --- Download handlers ---

type DownloadImageHandler struct{ uc *usecase.DownloadImageUseCase }
func NewDownloadImageHandler(uc *usecase.DownloadImageUseCase) *DownloadImageHandler {
	return &DownloadImageHandler{uc: uc}
}
func (h *DownloadImageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	var req domain.DownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

type DownloadVideoHandler struct{ uc *usecase.DownloadVideoUseCase }
func NewDownloadVideoHandler(uc *usecase.DownloadVideoUseCase) *DownloadVideoHandler {
	return &DownloadVideoHandler{uc: uc}
}
func (h *DownloadVideoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	var req domain.DownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

type DownloadAudioHandler struct{ uc *usecase.DownloadAudioUseCase }
func NewDownloadAudioHandler(uc *usecase.DownloadAudioUseCase) *DownloadAudioHandler {
	return &DownloadAudioHandler{uc: uc}
}
func (h *DownloadAudioHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	var req domain.DownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

type DownloadDocumentHandler struct{ uc *usecase.DownloadDocumentUseCase }
func NewDownloadDocumentHandler(uc *usecase.DownloadDocumentUseCase) *DownloadDocumentHandler {
	return &DownloadDocumentHandler{uc: uc}
}
func (h *DownloadDocumentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	var req domain.DownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

type DownloadStickerHandler struct{ uc *usecase.DownloadStickerUseCase }
func NewDownloadStickerHandler(uc *usecase.DownloadStickerUseCase) *DownloadStickerHandler {
	return &DownloadStickerHandler{uc: uc}
}
func (h *DownloadStickerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	var req domain.DownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// --- Presence/Chat handlers ---

type SendPresenceHandler struct{ uc *usecase.SendPresenceUseCase }
func NewSendPresenceHandler(uc *usecase.SendPresenceUseCase) *SendPresenceHandler {
	return &SendPresenceHandler{uc: uc}
}
func (h *SendPresenceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	var req domain.SendPresenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	if err := h.uc.Execute(r.Context(), id, req); err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, map[string]string{"Details": "Presence sent"}, nil)
}

type SubscribePresenceHandler struct{ uc *usecase.SubscribePresenceUseCase }
func NewSubscribePresenceHandler(uc *usecase.SubscribePresenceUseCase) *SubscribePresenceHandler {
	return &SubscribePresenceHandler{uc: uc}
}
func (h *SubscribePresenceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	var req domain.SubscribePresenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	if err := h.uc.Execute(r.Context(), id, req); err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, map[string]string{"Details": "Presence subscription updated"}, nil)
}

type ChatPresenceHandler struct{ uc *usecase.ChatPresenceUseCase }
func NewChatPresenceHandler(uc *usecase.ChatPresenceUseCase) *ChatPresenceHandler {
	return &ChatPresenceHandler{uc: uc}
}
func (h *ChatPresenceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	var req domain.ChatPresenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	if err := h.uc.Execute(r.Context(), id, req); err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, map[string]string{"Details": "Chat presence sent"}, nil)
}

type MarkReadHandler struct{ uc *usecase.MarkReadUseCase }
func NewMarkReadHandler(uc *usecase.MarkReadUseCase) *MarkReadHandler {
	return &MarkReadHandler{uc: uc}
}
func (h *MarkReadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	var req domain.MarkReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	if err := h.uc.Execute(r.Context(), id, req); err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, map[string]string{"Details": "Message marked as read"}, nil)
}

// --- User info handlers ---

type ReactHandler struct{ uc *usecase.ReactUseCase }
func NewReactHandler(uc *usecase.ReactUseCase) *ReactHandler {
	return &ReactHandler{uc: uc}
}
func (h *ReactHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	var req domain.ReactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

type GetAvatarHandler struct{ uc *usecase.GetAvatarUseCase }
func NewGetAvatarHandler(uc *usecase.GetAvatarUseCase) *GetAvatarHandler {
	return &GetAvatarHandler{uc: uc}
}
func (h *GetAvatarHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	var req domain.GetAvatarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

type GetContactsHandler struct{ uc *usecase.GetContactsUseCase }
func NewGetContactsHandler(uc *usecase.GetContactsUseCase) *GetContactsHandler {
	return &GetContactsHandler{uc: uc}
}
func (h *GetContactsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	rsp, err := h.uc.Execute(r.Context(), id, domain.GetContactsRequest{})
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

type GetUserInfoHandler struct{ uc *usecase.GetUserUseCase }
func NewGetUserInfoHandler(uc *usecase.GetUserUseCase) *GetUserInfoHandler {
	return &GetUserInfoHandler{uc: uc}
}
func (h *GetUserInfoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	var req domain.CheckUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// ExtendedHandlers groups all handlers that were added during Phase 4 migration.
type ExtendedHandlers struct {
	DownloadImage      *DownloadImageHandler
	DownloadVideo      *DownloadVideoHandler
	DownloadAudio      *DownloadAudioHandler
	DownloadDocument   *DownloadDocumentHandler
	DownloadSticker    *DownloadStickerHandler
	SendPresence       *SendPresenceHandler
	SubscribePresence  *SubscribePresenceHandler
	ChatPresence       *ChatPresenceHandler
	MarkRead           *MarkReadHandler
	React              *ReactHandler
	GetAvatar          *GetAvatarHandler
	GetContacts        *GetContactsHandler
	GetUserInfo        *GetUserInfoHandler
}
