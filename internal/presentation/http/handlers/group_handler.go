package handlers

import (
	"encoding/json"
	"net/http"

	customhttp "disparazap/internal/presentation/http"

	"disparazap/internal/application/usecase"
	"disparazap/internal/shared/domain"
)

// GroupHandlers agrupa os handlers de grupo.
type GroupHandlers struct {
	GetGroupRequestParticipants    *GetGroupRequestParticipantsHandler
	UpdateGroupRequestParticipants *UpdateGroupRequestParticipantsHandler
	SetGroupJoinApprovalMode       *SetGroupJoinApprovalModeHandler
	ListGroups                     *ListGroupsHandler
	GetGroupInfo                   *GetGroupInfoHandler
	GetGroupInviteLink             *GetGroupInviteLinkHandler
	GetGroupInviteInfo             *GetGroupInviteInfoHandler
}

// GetGroupRequestParticipantsHandler lists group join requests
type GetGroupRequestParticipantsHandler struct{ usecase *usecase.GroupRequestUseCase }
func NewGetGroupRequestParticipantsHandler(uc *usecase.GroupRequestUseCase) *GetGroupRequestParticipantsHandler { return &GetGroupRequestParticipantsHandler{uc} }
func (h *GetGroupRequestParticipantsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	var req domain.GetGroupRequestParticipantsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	rsp, err := h.usecase.ExecuteGetGroupRequestParticipants(r.Context(), id, req)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// UpdateGroupRequestParticipantsHandler approves/rejects join requests
type UpdateGroupRequestParticipantsHandler struct{ usecase *usecase.GroupRequestUseCase }
func NewUpdateGroupRequestParticipantsHandler(uc *usecase.GroupRequestUseCase) *UpdateGroupRequestParticipantsHandler { return &UpdateGroupRequestParticipantsHandler{uc} }
func (h *UpdateGroupRequestParticipantsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	var req domain.UpdateGroupRequestParticipantsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	rsp, err := h.usecase.ExecuteUpdateGroupRequestParticipants(r.Context(), id, req)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// SetGroupJoinApprovalModeHandler toggles join approval requirement
type SetGroupJoinApprovalModeHandler struct{ usecase *usecase.GroupRequestUseCase }
func NewSetGroupJoinApprovalModeHandler(uc *usecase.GroupRequestUseCase) *SetGroupJoinApprovalModeHandler { return &SetGroupJoinApprovalModeHandler{uc} }
func (h *SetGroupJoinApprovalModeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	var req domain.SetGroupJoinApprovalModeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	rsp, err := h.usecase.ExecuteSetGroupJoinApprovalMode(r.Context(), id, req)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// ListGroupsHandler lists groups
type ListGroupsHandler struct{ usecase *usecase.ListGroupsUseCase }
func NewListGroupsHandler(uc *usecase.ListGroupsUseCase) *ListGroupsHandler { return &ListGroupsHandler{uc} }
func (h *ListGroupsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	rsp, err := h.usecase.Execute(r.Context(), id, domain.ListGroupsRequest{})
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// GetGroupInfoHandler gets group info
type GetGroupInfoHandler struct{ usecase *usecase.GetGroupInfoUseCase }
func NewGetGroupInfoHandler(uc *usecase.GetGroupInfoUseCase) *GetGroupInfoHandler { return &GetGroupInfoHandler{uc} }
func (h *GetGroupInfoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	var req domain.GetGroupInfoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	rsp, err := h.usecase.Execute(r.Context(), id, req)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// GetGroupInviteLinkHandler gets invite link
type GetGroupInviteLinkHandler struct{ usecase *usecase.GetGroupInviteLinkUseCase }
func NewGetGroupInviteLinkHandler(uc *usecase.GetGroupInviteLinkUseCase) *GetGroupInviteLinkHandler { return &GetGroupInviteLinkHandler{uc} }
func (h *GetGroupInviteLinkHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	var req domain.GetGroupInviteLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	rsp, err := h.usecase.Execute(r.Context(), id, req)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// GetGroupInviteInfoHandler gets invite info
type GetGroupInviteInfoHandler struct{ usecase *usecase.GetGroupInviteInfoUseCase }
func NewGetGroupInviteInfoHandler(uc *usecase.GetGroupInviteInfoUseCase) *GetGroupInviteInfoHandler { return &GetGroupInviteInfoHandler{uc} }
func (h *GetGroupInviteInfoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	var req domain.GetGroupInviteInfoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	rsp, err := h.usecase.Execute(r.Context(), id, req)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}
