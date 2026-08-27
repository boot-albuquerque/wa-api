package handlers

import (
	"errors"
	"net/http"

	customhttp "wa-api/pkg/presentation/http"
	dtogroup "wa-api/pkg/presentation/http/dto/group"

	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"

	"github.com/rs/zerolog/hlog"

	"wa-api/pkg/application/usecase/group"
)

// Os caminhos de saida deste arquivo logam a causa INLINE, com a cadeia
// hlog.FromRequest(r) inteira num unico ponto: Warn para a rejeicao causada
// pelo cliente (>=400 abaixo de 500) e Error para a falha de dependencia
// (>=500). O registro de fronteira do router ja' diz QUE houve o status; estes
// dizem POR QUE. Extrair a cadeia para um helper apagaria o log da funcao aos
// olhos de cmd/logcov, que exige a cadeia no proprio caminho de saida.

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
type GetGroupRequestParticipantsHandler struct{ usecase *group.GroupRequestUseCase }

func NewGetGroupRequestParticipantsHandler(uc *group.GroupRequestUseCase) *GetGroupRequestParticipantsHandler {
	return &GetGroupRequestParticipantsHandler{uc}
}
func (h *GetGroupRequestParticipantsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var body dtogroup.GroupTargetRequest

	decodeErr := decodeRequest(w, r, &body)
	if requestAnswered(decodeErr) {
		return
	}
	if decodeErr != nil {
		body = dtogroup.GroupTargetRequest{}
	}
	if body.GroupJID == "" {
		if v := r.URL.Query().Get("group_jid"); v != "" {
			body.GroupJID = v
		}
	}
	if body.GroupJID == "" && body.ChatAlias == "" {
		if v := r.URL.Query().Get("chat"); v != "" {
			body.ChatAlias = v
			body.ResolveChat()
		}
	}
	if decodeErr != nil && body.GroupJID == "" {
		hlog.FromRequest(r).Warn().Err(decodeErr).Str("route", r.URL.Path).Msg("could not decode group request participants payload")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.usecase.ExecuteGetGroupRequestParticipants(r.Context(), id, body.ToRequestParticipantsDomain())
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("get group request participants failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtogroup.PresentGetGroupRequestParticipants(rsp), nil)
}

// UpdateGroupRequestParticipantsHandler approves/rejects join requests
type UpdateGroupRequestParticipantsHandler struct{ usecase *group.GroupRequestUseCase }

func NewUpdateGroupRequestParticipantsHandler(uc *group.GroupRequestUseCase) *UpdateGroupRequestParticipantsHandler {
	return &UpdateGroupRequestParticipantsHandler{uc}
}
func (h *UpdateGroupRequestParticipantsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var body dtogroup.UpdateGroupRequestParticipantsRequest
	if err := decodeRequest(w, r, &body); err != nil {
		if requestAnswered(err) {
			return
		}
		hlog.FromRequest(r).Warn().Err(err).Str("route", r.URL.Path).Msg("could not decode update group request participants payload")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.usecase.ExecuteUpdateGroupRequestParticipants(r.Context(), id, body.ToDomain())
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("update group request participants failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtogroup.PresentAcknowledgement(rsp.Details), nil)
}

// SetGroupJoinApprovalModeHandler toggles join approval requirement
type SetGroupJoinApprovalModeHandler struct{ usecase *group.GroupRequestUseCase }

func NewSetGroupJoinApprovalModeHandler(uc *group.GroupRequestUseCase) *SetGroupJoinApprovalModeHandler {
	return &SetGroupJoinApprovalModeHandler{uc}
}
func (h *SetGroupJoinApprovalModeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var body dtogroup.SetGroupJoinApprovalModeRequest
	if err := decodeRequest(w, r, &body); err != nil {
		if requestAnswered(err) {
			return
		}
		hlog.FromRequest(r).Warn().Err(err).Str("route", r.URL.Path).Msg("could not decode set group join approval mode payload")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.usecase.ExecuteSetGroupJoinApprovalMode(r.Context(), id, body.ToDomain())
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("set group join approval mode failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtogroup.PresentAcknowledgement(rsp.Details), nil)
}

// ListGroupsHandler lists groups
type ListGroupsHandler struct{ usecase *group.ListGroupsUseCase }

func NewListGroupsHandler(uc *group.ListGroupsUseCase) *ListGroupsHandler {
	return &ListGroupsHandler{uc}
}
func (h *ListGroupsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), id, domain.ListGroupsRequest{})
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("list groups failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtogroup.PresentListGroups(rsp), nil)
}

// GetGroupInfoHandler gets group info
type GetGroupInfoHandler struct{ usecase *group.GetGroupInfoUseCase }

func NewGetGroupInfoHandler(uc *group.GetGroupInfoUseCase) *GetGroupInfoHandler {
	return &GetGroupInfoHandler{uc}
}
func (h *GetGroupInfoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.GetGroupInfoRequest
	if err := decodeRequest(w, r, &req); err != nil {
		if requestAnswered(err) {
			return
		}
		hlog.FromRequest(r).Warn().Err(err).Str("route", r.URL.Path).Msg("could not decode get group info payload")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), id, req)
	if err != nil {
		var appErr *apperr.AppError
		if errors.As(err, &appErr) {
			hlog.FromRequest(r).Warn().Err(err).Str("route", r.URL.Path).Msg("get group info rejected")
		} else {
			hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("get group info failed")
		}
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtogroup.PresentGetGroupInfo(rsp), nil)
}

// GetGroupInviteLinkHandler gets invite link
type GetGroupInviteLinkHandler struct {
	usecase *group.GetGroupInviteLinkUseCase
}

func NewGetGroupInviteLinkHandler(uc *group.GetGroupInviteLinkUseCase) *GetGroupInviteLinkHandler {
	return &GetGroupInviteLinkHandler{uc}
}
func (h *GetGroupInviteLinkHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var body dtogroup.GroupTargetRequest
	if err := decodeRequest(w, r, &body); err != nil {
		if requestAnswered(err) {
			return
		}
		hlog.FromRequest(r).Warn().Err(err).Str("route", r.URL.Path).Msg("could not decode get group invite link payload")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), id, body.ToInviteLinkDomain())
	if err != nil {
		var appErr *apperr.AppError
		if errors.As(err, &appErr) {
			hlog.FromRequest(r).Warn().Err(err).Str("route", r.URL.Path).Msg("get group invite link rejected")
		} else {
			hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("get group invite link failed")
		}
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtogroup.PresentGetGroupInviteLink(rsp), nil)
}

// GetGroupInviteInfoHandler gets invite info
type GetGroupInviteInfoHandler struct {
	usecase *group.GetGroupInviteInfoUseCase
}

func NewGetGroupInviteInfoHandler(uc *group.GetGroupInviteInfoUseCase) *GetGroupInviteInfoHandler {
	return &GetGroupInviteInfoHandler{uc}
}
func (h *GetGroupInviteInfoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var body dtogroup.GetGroupInviteInfoRequest
	if err := decodeRequest(w, r, &body); err != nil {
		if requestAnswered(err) {
			return
		}
		hlog.FromRequest(r).Warn().Err(err).Str("route", r.URL.Path).Msg("could not decode get group invite info payload")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), id, body.ToDomain())
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("get group invite info failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtogroup.PresentGetGroupInviteInfo(rsp), nil)
}
