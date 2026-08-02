package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/hlog"

	"wa-api/pkg/domain"
	customhttp "wa-api/pkg/presentation/http"

	"wa-api/pkg/application/usecase/chat"
	"wa-api/pkg/application/usecase/notification"
	"wa-api/pkg/application/usecase/user"
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
type GetHealthHandler struct {
	usecase *notification.GetHealthUseCase
}

func NewGetHealthHandler(uc *notification.GetHealthUseCase) *GetHealthHandler {
	return &GetHealthHandler{uc}
}
func (h *GetHealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rsp, err := h.usecase.Execute(r.Context())
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", "/health").Msg("request failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// ListNewsletterHandler handles GET /newsletter/list
type ListNewsletterHandler struct {
	usecase *notification.ListNewsletterUseCase
}

func NewListNewsletterHandler(uc *notification.ListNewsletterUseCase) *ListNewsletterHandler {
	return &ListNewsletterHandler{uc}
}
func (h *ListNewsletterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), id)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", "/newsletter/list").Msg("request failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// DeleteUserCompleteHandler handles DELETE /admin/users/{id}/complete
type DeleteUserCompleteHandler struct {
	usecase *user.DeleteUserCompleteUseCase
}

func NewDeleteUserCompleteHandler(uc *user.DeleteUserCompleteUseCase) *DeleteUserCompleteHandler {
	return &DeleteUserCompleteHandler{uc}
}
func (h *DeleteUserCompleteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const route = "/admin/users/{id}/complete"

	vars := mux.Vars(r)
	uid := vars["id"]
	if uid == "" {
		hlog.FromRequest(r).Warn().Err(errMissingID).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, 400, nil, errMissingID)
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), uid)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", route).Msg("request failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// RejectCallHandler handles POST /chat/rejectcall
type RejectCallHandler struct{ usecase *chat.RejectCallUseCase }

func NewRejectCallHandler(uc *chat.RejectCallUseCase) *RejectCallHandler {
	return &RejectCallHandler{uc}
}
func (h *RejectCallHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const route = "/chat/rejectcall"

	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.RejectCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// GetPrivacySettingsHandler handles GET /user/privacy
type GetPrivacySettingsHandler struct {
	usecase *user.GetPrivacySettingsUseCase
}

func NewGetPrivacySettingsHandler(uc *user.GetPrivacySettingsUseCase) *GetPrivacySettingsHandler {
	return &GetPrivacySettingsHandler{uc}
}
func (h *GetPrivacySettingsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), id)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", "/user/privacy").Msg("request failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// SetPrivacySettingHandler handles POST /user/privacy
type SetPrivacySettingHandler struct {
	usecase *user.SetPrivacySettingUseCase
}

func NewSetPrivacySettingHandler(uc *user.SetPrivacySettingUseCase) *SetPrivacySettingHandler {
	return &SetPrivacySettingHandler{uc}
}
func (h *SetPrivacySettingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const route = "/user/privacy"

	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.SetPrivacySettingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// RequestUnavailableMessageHandler handles POST /chat/requestunavailablemessage
type RequestUnavailableMessageHandler struct {
	usecase *chat.RequestUnavailableMessageUseCase
}

func NewRequestUnavailableMessageHandler(uc *chat.RequestUnavailableMessageUseCase) *RequestUnavailableMessageHandler {
	return &RequestUnavailableMessageHandler{uc}
}
func (h *RequestUnavailableMessageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const route = "/chat/requestunavailablemessage"

	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.RequestUnavailableMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// ArchiveChatHandler handles POST /chat/archive
type ArchiveChatHandler struct{ usecase *chat.ArchiveChatUseCase }

func NewArchiveChatHandler(uc *chat.ArchiveChatUseCase) *ArchiveChatHandler {
	return &ArchiveChatHandler{uc}
}
func (h *ArchiveChatHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const route = "/chat/archive"

	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.ArchiveChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
	customhttp.RespondJSON(w, 200, rsp, nil)
}
