package handlers

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"

	customhttp "wa-api/pkg/presentation/http"
	dtogroup "wa-api/pkg/presentation/http/dto/group"

	"github.com/rs/zerolog/hlog"

	"wa-api/pkg/application/usecase/group"
	"wa-api/pkg/domain/apperr"
)

// GroupManagementHandlers groups all group write-operation handlers.
type GroupManagementHandlers struct {
	CreateGroup             *groupHandler
	GroupJoin               *groupHandler
	GroupLeave              *groupHandler
	SetGroupName            *groupHandler
	SetGroupTopic           *groupHandler
	SetGroupPhoto           *groupHandler
	RemoveGroupPhoto        *groupHandler
	SetGroupAnnounce        *groupHandler
	SetGroupLocked          *groupHandler
	SetDisappearingTimer    *groupHandler
	UpdateGroupParticipants *groupHandler
}

// groupHandler is a generic handler that delegates to GroupManagementUseCase.
type groupHandler struct {
	uc      *group.GroupManagementUseCase
	handler func(uc *group.GroupManagementUseCase, w http.ResponseWriter, r *http.Request, id string)
}

func (h *groupHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	h.handler(h.uc, w, r, id)
}

func newGroupHandler(uc *group.GroupManagementUseCase, h func(*group.GroupManagementUseCase, http.ResponseWriter, *http.Request, string)) *groupHandler {
	return &groupHandler{uc: uc, handler: h}
}

// NewGroupManagementHandlers creates all group management handlers.
func NewGroupManagementHandlers(uc *group.GroupManagementUseCase) *GroupManagementHandlers {
	return &GroupManagementHandlers{
		CreateGroup:             newGroupHandler(uc, handleCreateGroup),
		GroupJoin:               newGroupHandler(uc, handleGroupJoin),
		GroupLeave:              newGroupHandler(uc, handleGroupLeave),
		SetGroupName:            newGroupHandler(uc, handleSetGroupName),
		SetGroupTopic:           newGroupHandler(uc, handleSetGroupTopic),
		SetGroupPhoto:           newGroupHandler(uc, handleSetGroupPhoto),
		RemoveGroupPhoto:        newGroupHandler(uc, handleRemoveGroupPhoto),
		SetGroupAnnounce:        newGroupHandler(uc, handleSetGroupAnnounce),
		SetGroupLocked:          newGroupHandler(uc, handleSetGroupLocked),
		SetDisappearingTimer:    newGroupHandler(uc, handleSetDisappearingTimer),
		UpdateGroupParticipants: newGroupHandler(uc, handleUpdateGroupParticipants),
	}
}

func decodeAndRespond(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if err := decodeRequest(w, r, v); err != nil {
		if requestAnswered(err) {
			return false
		}
		hlog.FromRequest(r).Warn().Err(err).Str("route", r.URL.Path).Msg("could not decode group management payload")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return false
	}
	return true
}

// rejectMissingField e' o caminho de saida 400 compartilhado pelas validacoes
// de campo obrigatorio deste arquivo: loga a causa (S-http) e responde com o
// MESMO erro que foi logado — nao ha' divergencia possivel entre os dois.
// rejectEmptyElement rejects a list whose LENGTH passed but whose CONTENT
// carries an empty entry. F101: validating only len(list) let participants:[""]
// reach the JID parser, which crashed on it; the index says WHICH entry failed.
func rejectEmptyElement(w http.ResponseWriter, r *http.Request, code, field string, index int, logMsg string) {
	err := apperr.New(code, apperr.CategoryValidation,
		fmt.Sprintf("Entrada vazia em %q, na posição %d.", field, index), false,
		fmt.Errorf("empty %s at index %d", field, index))
	hlog.FromRequest(r).Warn().Err(err).Str("route", r.URL.Path).Msg(logMsg)
	customhttp.RespondJSON(w, 400, nil, err)
}

// firstEmpty reports the index of the first empty entry, or -1 when there is none.
func firstEmpty(in []string) int {
	for i, v := range in {
		if v == "" {
			return i
		}
	}
	return -1
}

// rejectMissingField takes the error CODE explicitly instead of deriving it
// from `field`. Deriving would be shorter and wrong twice over: the wire
// spells the same field `groupJID` on one route and `groupjid` on another, and
// neither spelling survives the canonical snake_case rule that every public
// enumerated value obeys (docs/HTTP-DTO-CONVENTIONS.md §8). One condition, one
// code, written where a reader can grep it.
//
// The pt-BR Message is what the client reads; the English cause is what the
// log carries — AppError.Error() concatenates the two, so the outcome log
// keeps naming the condition the way this repository's logs already do.
func rejectMissingField(w http.ResponseWriter, r *http.Request, code, field, logMsg string) {
	err := apperr.New(code, apperr.CategoryValidation,
		fmt.Sprintf("Campo obrigatório ausente: %q.", field), false,
		fmt.Errorf("missing %s", field))
	hlog.FromRequest(r).Warn().Err(err).Str("route", r.URL.Path).Msg(logMsg)
	customhttp.RespondJSON(w, 400, nil, err)
}

func handleCreateGroup(uc *group.GroupManagementUseCase, w http.ResponseWriter, r *http.Request, id string) {
	var req dtogroup.CreateGroupRequest
	if !decodeAndRespond(w, r, &req) {
		return
	}
	if req.Name == "" {
		rejectMissingField(w, r, CodeMissingName, "name", "create group request rejected")
		return
	}
	if req.IsParent && req.LinkedParentJID != "" {
		err := apperr.New(CodeMutuallyExclusiveParent, apperr.CategoryValidation,
			"Os campos \"is_parent\" e \"linked_parent_jid\" não podem ser enviados juntos.", false,
			errors.New("is_parent and linked_parent_jid are mutually exclusive"))
		hlog.FromRequest(r).Warn().Err(err).Str("route", r.URL.Path).Msg("create group request rejected")
		customhttp.RespondJSON(w, 400, nil, err)
		return
	}
	if !req.IsParent && len(req.Participants) < 1 {
		rejectMissingField(w, r, CodeMissingParticipants, "participants", "create group request rejected")
		return
	}
	if i := firstEmpty(req.Participants); i >= 0 {
		rejectEmptyElement(w, r, CodeEmptyParticipant, "participants", i, "create group request rejected")
		return
	}
	rsp, err := uc.CreateGroup(r.Context(), id, req.Name, req.Participants, req.ToDomainOpts())
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("create group failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtogroup.PresentCreatedGroup(rsp), nil)
}

func handleGroupJoin(uc *group.GroupManagementUseCase, w http.ResponseWriter, r *http.Request, id string) {
	var req dtogroup.JoinGroupRequest
	if !decodeAndRespond(w, r, &req) {
		return
	}
	if req.Code == "" {
		rejectMissingField(w, r, CodeMissingInviteCode, "code", "join group request rejected")
		return
	}
	_, err := uc.JoinGroup(r.Context(), id, req.Code)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("join group failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtogroup.PresentAcknowledgement("Group joined successfully"), nil)
}

func handleGroupLeave(uc *group.GroupManagementUseCase, w http.ResponseWriter, r *http.Request, id string) {
	var req dtogroup.GroupTargetRequest
	if !decodeAndRespond(w, r, &req) {
		return
	}
	if req.GroupJID == "" {
		rejectMissingField(w, r, CodeMissingGroupJID, "group_jid", "leave group request rejected")
		return
	}
	if err := uc.LeaveGroup(r.Context(), id, req.GroupJID); err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("leave group failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtogroup.PresentAcknowledgement("Group left successfully"), nil)
}

func handleSetGroupName(uc *group.GroupManagementUseCase, w http.ResponseWriter, r *http.Request, id string) {
	var req dtogroup.SetGroupNameRequest
	if !decodeAndRespond(w, r, &req) {
		return
	}
	if req.Name == "" {
		rejectMissingField(w, r, CodeMissingName, "name", "set group name request rejected")
		return
	}
	if err := uc.SetGroupName(r.Context(), id, req.GroupJID, req.Name); err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("set group name failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtogroup.PresentAcknowledgement("Group name set successfully"), nil)
}

func handleSetGroupTopic(uc *group.GroupManagementUseCase, w http.ResponseWriter, r *http.Request, id string) {
	var req dtogroup.SetGroupTopicRequest
	if !decodeAndRespond(w, r, &req) {
		return
	}
	if req.Topic == "" {
		rejectMissingField(w, r, CodeMissingTopic, "topic", "set group topic request rejected")
		return
	}
	if err := uc.SetGroupTopic(r.Context(), id, req.GroupJID, req.Topic); err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("set group topic failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtogroup.PresentAcknowledgement("Group topic set successfully"), nil)
}

func handleSetGroupPhoto(uc *group.GroupManagementUseCase, w http.ResponseWriter, r *http.Request, id string) {
	var req dtogroup.SetGroupPhotoRequest
	if !decodeAndRespond(w, r, &req) {
		return
	}
	if req.Photo == "" {
		rejectMissingField(w, r, CodeMissingPhoto, "photo", "set group photo request rejected")
		return
	}
	photoBytes, err := base64.StdEncoding.DecodeString(req.Photo)
	if err != nil {
		hlog.FromRequest(r).Warn().Err(err).Str("route", r.URL.Path).Msg("photo is not valid base64")
		// The decoder's own error is WRAPPED, not interpolated into Message:
		// it quotes the offending byte, which is request data. It belongs in
		// the log, which is where the wrapped cause goes, and never on the wire.
		customhttp.RespondJSON(w, 400, nil, apperr.New(CodeInvalidPhotoEncoding, apperr.CategoryValidation,
			"O campo \"photo\" precisa estar codificado em base64.", false,
			fmt.Errorf("photo must be base64-encoded: %w", err)))
		return
	}
	if err := uc.SetGroupPhoto(r.Context(), id, req.GroupJID, photoBytes); err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("set group photo failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtogroup.PresentAcknowledgement("Group photo set successfully"), nil)
}

func handleRemoveGroupPhoto(uc *group.GroupManagementUseCase, w http.ResponseWriter, r *http.Request, id string) {
	var req dtogroup.GroupTargetRequest
	if !decodeAndRespond(w, r, &req) {
		return
	}
	if err := uc.RemoveGroupPhoto(r.Context(), id, req.GroupJID); err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("remove group photo failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtogroup.PresentAcknowledgement("Group photo removed successfully"), nil)
}

func handleSetGroupAnnounce(uc *group.GroupManagementUseCase, w http.ResponseWriter, r *http.Request, id string) {
	var req dtogroup.SetGroupAnnounceRequest
	if !decodeAndRespond(w, r, &req) {
		return
	}
	if err := uc.SetGroupAnnounce(r.Context(), id, req.GroupJID, req.Announce); err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("set group announce failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtogroup.PresentAcknowledgement("Group announce set successfully"), nil)
}

func handleSetGroupLocked(uc *group.GroupManagementUseCase, w http.ResponseWriter, r *http.Request, id string) {
	var req dtogroup.SetGroupLockedRequest
	if !decodeAndRespond(w, r, &req) {
		return
	}
	if err := uc.SetGroupLocked(r.Context(), id, req.GroupJID, req.Locked); err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("set group locked failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtogroup.PresentAcknowledgement("Group lock updated"), nil)
}

func handleSetDisappearingTimer(uc *group.GroupManagementUseCase, w http.ResponseWriter, r *http.Request, id string) {
	var req dtogroup.SetDisappearingTimerRequest
	if !decodeAndRespond(w, r, &req) {
		return
	}
	if err := uc.SetDisappearingTimer(r.Context(), id, req.GroupJID, req.Duration); err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("set disappearing timer failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtogroup.PresentAcknowledgement("Disappearing timer set"), nil)
}

func handleUpdateGroupParticipants(uc *group.GroupManagementUseCase, w http.ResponseWriter, r *http.Request, id string) {
	var req dtogroup.UpdateGroupParticipantsRequest
	if !decodeAndRespond(w, r, &req) {
		return
	}
	if len(req.Phone) < 1 {
		rejectMissingField(w, r, CodeMissingPhones, "phones", "update group participants request rejected")
		return
	}
	if i := firstEmpty(req.Phone); i >= 0 {
		rejectEmptyElement(w, r, CodeEmptyPhone, "phones", i, "update group participants request rejected")
		return
	}
	if req.Action == "" {
		rejectMissingField(w, r, CodeMissingAction, "action", "update group participants request rejected")
		return
	}
	if req.GroupJID == "" {
		rejectMissingField(w, r, CodeMissingGroupJID, "group_jid", "update group participants request rejected")
		return
	}
	update, err := uc.UpdateGroupParticipants(r.Context(), id, req.GroupJID, req.Action, req.Phone)
	if err != nil {
		var appErr *apperr.AppError
		if errors.As(err, &appErr) && appErr.Category == apperr.CategoryValidation {
			hlog.FromRequest(r).Warn().Err(err).Str("route", r.URL.Path).Msg("update group participants rejected")
			customhttp.RespondJSON(w, 400, nil, err)
		} else {
			hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("update group participants failed")
			customhttp.RespondJSON(w, 500, nil, err)
		}
		return
	}
	customhttp.RespondJSON(w, 200, dtogroup.PresentParticipantsUpdate(update, "Participants updated"), nil)
}
