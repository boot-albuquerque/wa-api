package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"disparazap/internal/application/usecase"
	"disparazap/internal/shared/domain"
	customhttp "disparazap/internal/presentation/http"
)

// MiscHandlers agrupa os handlers de miscelânea (health, newsletter, privacy, calls, archive)
type MiscHandlers struct {
	Health                    *GetHealthHandler
	ListNewsletter            *ListNewsletterHandler
	DeleteUserComplete        *DeleteUserCompleteHandler
	RejectCall                *RejectCallHandler
	GetPrivacySettings        *GetPrivacySettingsHandler
	SetPrivacySetting         *SetPrivacySettingHandler
	RequestUnavailableMessage *RequestUnavailableMessageHandler
	ArchiveChat               *ArchiveChatHandler
}

// GetHealthHandler handles GET /health
type GetHealthHandler struct{ usecase *usecase.GetHealthUseCase }
func NewGetHealthHandler(uc *usecase.GetHealthUseCase) *GetHealthHandler { return &GetHealthHandler{uc} }
func (h *GetHealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rsp, err := h.usecase.Execute(r.Context())
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// ListNewsletterHandler handles GET /newsletter/list
type ListNewsletterHandler struct{ usecase *usecase.ListNewsletterUseCase }
func NewListNewsletterHandler(uc *usecase.ListNewsletterUseCase) *ListNewsletterHandler { return &ListNewsletterHandler{uc} }
func (h *ListNewsletterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	rsp, err := h.usecase.Execute(r.Context(), id)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// DeleteUserCompleteHandler handles DELETE /admin/users/{id}/complete
type DeleteUserCompleteHandler struct{ usecase *usecase.DeleteUserCompleteUseCase }
func NewDeleteUserCompleteHandler(uc *usecase.DeleteUserCompleteUseCase) *DeleteUserCompleteHandler { return &DeleteUserCompleteHandler{uc} }
func (h *DeleteUserCompleteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	uid := vars["id"]
	if uid == "" { customhttp.RespondJSON(w, 400, nil, errMissingID); return }
	rsp, err := h.usecase.Execute(r.Context(), uid)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// RejectCallHandler handles POST /chat/rejectcall
type RejectCallHandler struct{ usecase *usecase.RejectCallUseCase }
func NewRejectCallHandler(uc *usecase.RejectCallUseCase) *RejectCallHandler { return &RejectCallHandler{uc} }
func (h *RejectCallHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	var req domain.RejectCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	rsp, err := h.usecase.Execute(r.Context(), id, req)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// GetPrivacySettingsHandler handles GET /user/privacy
type GetPrivacySettingsHandler struct{ usecase *usecase.GetPrivacySettingsUseCase }
func NewGetPrivacySettingsHandler(uc *usecase.GetPrivacySettingsUseCase) *GetPrivacySettingsHandler { return &GetPrivacySettingsHandler{uc} }
func (h *GetPrivacySettingsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	rsp, err := h.usecase.Execute(r.Context(), id)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// SetPrivacySettingHandler handles POST /user/privacy
type SetPrivacySettingHandler struct{ usecase *usecase.SetPrivacySettingUseCase }
func NewSetPrivacySettingHandler(uc *usecase.SetPrivacySettingUseCase) *SetPrivacySettingHandler { return &SetPrivacySettingHandler{uc} }
func (h *SetPrivacySettingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	var req domain.SetPrivacySettingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	rsp, err := h.usecase.Execute(r.Context(), id, req)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// RequestUnavailableMessageHandler handles POST /chat/requestunavailablemessage
type RequestUnavailableMessageHandler struct{ usecase *usecase.RequestUnavailableMessageUseCase }
func NewRequestUnavailableMessageHandler(uc *usecase.RequestUnavailableMessageUseCase) *RequestUnavailableMessageHandler { return &RequestUnavailableMessageHandler{uc} }
func (h *RequestUnavailableMessageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	var req domain.RequestUnavailableMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	rsp, err := h.usecase.Execute(r.Context(), id, req)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// ArchiveChatHandler handles POST /chat/archive
type ArchiveChatHandler struct{ usecase *usecase.ArchiveChatUseCase }
func NewArchiveChatHandler(uc *usecase.ArchiveChatUseCase) *ArchiveChatHandler { return &ArchiveChatHandler{uc} }
func (h *ArchiveChatHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	var req domain.ArchiveChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	rsp, err := h.usecase.Execute(r.Context(), id, req)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}
