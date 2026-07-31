package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	customhttp "wa-api/internal/presentation/http"

	"wa-api/internal/application/usecase/group"
)

// GroupManagementHandlers groups all group write-operation handlers.
type GroupManagementHandlers struct {
	CreateGroup              *groupHandler
	GroupJoin                *groupHandler
	GroupLeave               *groupHandler
	SetGroupName             *groupHandler
	SetGroupTopic            *groupHandler
	SetGroupPhoto            *groupHandler
	RemoveGroupPhoto         *groupHandler
	SetGroupAnnounce         *groupHandler
	SetGroupLocked           *groupHandler
	SetDisappearingTimer     *groupHandler
	UpdateGroupParticipants  *groupHandler
}

// groupHandler is a generic handler that delegates to GroupManagementUseCase.
type groupHandler struct {
	uc       *group.GroupManagementUseCase
	handler  func(uc *group.GroupManagementUseCase, w http.ResponseWriter, r *http.Request, id string)
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
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return false
	}
	return true
}

func handleCreateGroup(uc *group.GroupManagementUseCase, w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Name         string   `json:"name"`
		Participants []string `json:"participants"`
	}
	if !decodeAndRespond(w, r, &req) { return }
	if req.Name == "" { customhttp.RespondJSON(w, 400, nil, fmt.Errorf("missing name")); return }
	if len(req.Participants) < 1 { customhttp.RespondJSON(w, 400, nil, fmt.Errorf("missing participants")); return }
	rsp, err := uc.CreateGroup(r.Context(), id, req.Name, req.Participants)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

func handleGroupJoin(uc *group.GroupManagementUseCase, w http.ResponseWriter, r *http.Request, id string) {
	var req struct{ Code string `json:"code"` }
	if !decodeAndRespond(w, r, &req) { return }
	if req.Code == "" { customhttp.RespondJSON(w, 400, nil, fmt.Errorf("missing code")); return }
	_, err := uc.JoinGroup(r.Context(), id, req.Code)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, map[string]string{"Details": "Group joined successfully"}, nil)
}

func handleGroupLeave(uc *group.GroupManagementUseCase, w http.ResponseWriter, r *http.Request, id string) {
	var req struct{ GroupJID string `json:"groupJID"` }
	if !decodeAndRespond(w, r, &req) { return }
	if req.GroupJID == "" { customhttp.RespondJSON(w, 400, nil, fmt.Errorf("missing groupJID")); return }
	if err := uc.LeaveGroup(r.Context(), id, req.GroupJID); err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, map[string]string{"Details": "Group left successfully"}, nil)
}

func handleSetGroupName(uc *group.GroupManagementUseCase, w http.ResponseWriter, r *http.Request, id string) {
	var req struct{ GroupJID, Name string }
	if !decodeAndRespond(w, r, &req) { return }
	if req.Name == "" { customhttp.RespondJSON(w, 400, nil, fmt.Errorf("missing name")); return }
	if err := uc.SetGroupName(r.Context(), id, req.GroupJID, req.Name); err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, map[string]string{"Details": "Group name set successfully"}, nil)
}

func handleSetGroupTopic(uc *group.GroupManagementUseCase, w http.ResponseWriter, r *http.Request, id string) {
	var req struct{ GroupJID, Topic string }
	if !decodeAndRespond(w, r, &req) { return }
	if req.Topic == "" { customhttp.RespondJSON(w, 400, nil, fmt.Errorf("missing topic")); return }
	if err := uc.SetGroupTopic(r.Context(), id, req.GroupJID, req.Topic); err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, map[string]string{"Details": "Group topic set successfully"}, nil)
}

func handleSetGroupPhoto(uc *group.GroupManagementUseCase, w http.ResponseWriter, r *http.Request, id string) {
	var req struct{ GroupJID, Photo string }
	if !decodeAndRespond(w, r, &req) { return }
	if err := uc.SetGroupPhoto(r.Context(), id, req.GroupJID, []byte(req.Photo)); err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, map[string]string{"Details": "Group photo set successfully"}, nil)
}

func handleRemoveGroupPhoto(uc *group.GroupManagementUseCase, w http.ResponseWriter, r *http.Request, id string) {
	var req struct{ GroupJID string `json:"groupjid"` }
	if !decodeAndRespond(w, r, &req) { return }
	if err := uc.RemoveGroupPhoto(r.Context(), id, req.GroupJID); err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, map[string]string{"Details": "Group photo removed successfully"}, nil)
}

func handleSetGroupAnnounce(uc *group.GroupManagementUseCase, w http.ResponseWriter, r *http.Request, id string) {
	var req struct{ GroupJID string; Announce bool }
	if !decodeAndRespond(w, r, &req) { return }
	if err := uc.SetGroupAnnounce(r.Context(), id, req.GroupJID, req.Announce); err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, map[string]interface{}{"Details": "Group announce set successfully"}, nil)
}

func handleSetGroupLocked(uc *group.GroupManagementUseCase, w http.ResponseWriter, r *http.Request, id string) {
	var req struct{ GroupJID string; Locked bool }
	if !decodeAndRespond(w, r, &req) { return }
	if err := uc.SetGroupLocked(r.Context(), id, req.GroupJID, req.Locked); err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, map[string]interface{}{"Details": "Group lock updated"}, nil)
}

func handleSetDisappearingTimer(uc *group.GroupManagementUseCase, w http.ResponseWriter, r *http.Request, id string) {
	var req struct{ GroupJID string `json:"groupjid"`; Duration string `json:"duration"` }
	if !decodeAndRespond(w, r, &req) { return }
	if err := uc.SetDisappearingTimer(r.Context(), id, req.GroupJID, req.Duration); err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, map[string]interface{}{"Details": "Disappearing timer set"}, nil)
}

func handleUpdateGroupParticipants(uc *group.GroupManagementUseCase, w http.ResponseWriter, r *http.Request, id string) {
	var req struct{ GroupJID string; Phone []string; Action string }
	if !decodeAndRespond(w, r, &req) { return }
	if len(req.Phone) < 1 { customhttp.RespondJSON(w, 400, nil, fmt.Errorf("missing phones")); return }
	if req.Action == "" { customhttp.RespondJSON(w, 400, nil, fmt.Errorf("missing action")); return }
	_, err := uc.UpdateGroupParticipants(r.Context(), id, req.GroupJID, req.Action, req.Phone)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, map[string]interface{}{"Details": "Participants updated"}, nil)
}
