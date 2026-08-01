package handlers

import (
	"encoding/json"
	"net/http"

	"wa-api/pkg/domain"
	customhttp "wa-api/pkg/presentation/http"

	"wa-api/pkg/application/usecase/user"
)

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

// ContactHandlers agrupa os handlers de leitura de dados de contato
// (/user/info, /user/avatar, /user/contacts).
type ContactHandlers struct {
	Avatar   *GetAvatarHandler
	Contacts *GetContactsHandler
	UserInfo *GetUserInfoHandler
}
