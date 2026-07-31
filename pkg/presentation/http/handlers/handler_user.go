package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	customhttp "wa-api/pkg/presentation/http"

	"wa-api/pkg/application/usecase/user"
)

// UserHandlers agrupa todos os handlers de usuário.
type UserHandlers struct {
	listUsers   *user.ListUsersUseCase
	addUser     *user.AddUserUseCase
	editUser    *user.EditUserUseCase
	deleteUser  *user.DeleteUserUseCase
	checkUser   *user.CheckUserUseCase
	getUser     *user.GetUserUseCase
	getUserLID  *user.GetUserLIDUseCase
	blockUser   *user.BlockUserUseCase
	unblockUser *user.UnblockUserUseCase
}

// NewUserHandlers cria uma nova instância de UserHandlers.
func NewUserHandlers(
	listUsers *user.ListUsersUseCase,
	addUser *user.AddUserUseCase,
	editUser *user.EditUserUseCase,
	deleteUser *user.DeleteUserUseCase,
	checkUser *user.CheckUserUseCase,
	getUser *user.GetUserUseCase,
	getUserLID *user.GetUserLIDUseCase,
	blockUser *user.BlockUserUseCase,
	unblockUser *user.UnblockUserUseCase,
) *UserHandlers {
	return &UserHandlers{
		listUsers:  listUsers,
		addUser:    addUser,
		editUser:   editUser,
		deleteUser: deleteUser,
		checkUser:  checkUser,
		getUser:    getUser,
		getUserLID: getUserLID,
		blockUser:  blockUser,
		unblockUser: unblockUser,
	}
}

// ListUsers retorna o handler para GET /admin/users.
func (h *UserHandlers) ListUsers() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userID := vars["id"]
		result, err := h.listUsers.Execute(r.Context(), domain.ListUsersRequest{UserID: userID})
		if err != nil {
			customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
			return
		}
		customhttp.RespondJSON(w, http.StatusOK, result, nil)
	})
}

// AddUser retorna o handler para POST /admin/users.
func (h *UserHandlers) AddUser() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req domain.AddUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
			return
		}
		result, err := h.addUser.Execute(r.Context(), req)
		if err != nil {
			customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
			return
		}
		customhttp.RespondJSON(w, http.StatusOK, result, nil)
	})
}

// EditUser retorna o handler para PUT /admin/users/{id}.
func (h *UserHandlers) EditUser() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		var req domain.EditUserRequest
		req.UserID = vars["id"]
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
			return
		}
		if err := h.editUser.Execute(r.Context(), req); err != nil {
			customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
			return
		}
		customhttp.RespondJSON(w, http.StatusOK, map[string]string{"status": "ok"}, nil)
	})
}

// DeleteUser retorna o handler para DELETE /admin/users/{id}.
func (h *UserHandlers) DeleteUser() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		req := domain.DeleteUserRequest{UserID: vars["id"]}
		if err := h.deleteUser.Execute(r.Context(), req); err != nil {
			customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
			return
		}
		customhttp.RespondJSON(w, http.StatusOK, map[string]string{"status": "deleted"}, nil)
	})
}

// CheckUser retorna o handler para POST /user/check.
func (h *UserHandlers) CheckUser() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
		if !ok || info == nil {
			customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
			return
		}
		txtID := info.Get("Id")
		if txtID == "" {
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
			return
		}
		var req domain.CheckUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
			return
		}
		result, err := h.checkUser.Execute(r.Context(), txtID, req)
		if err != nil {
			customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
			return
		}
		customhttp.RespondJSON(w, http.StatusOK, result, nil)
	})
}

// GetUser retorna o handler para POST /user/info.
func (h *UserHandlers) GetUser() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
		if !ok || info == nil {
			customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
			return
		}
		txtID := info.Get("Id")
		if txtID == "" {
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
			return
		}
		var req domain.CheckUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
			return
		}
		result, err := h.getUser.Execute(r.Context(), txtID, req)
		if err != nil {
			customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
			return
		}
		customhttp.RespondJSON(w, http.StatusOK, result, nil)
	})
}

// GetUserLID retorna o handler para POST /user/lid.
func (h *UserHandlers) GetUserLID() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
		if !ok || info == nil {
			customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
			return
		}
		txtID := info.Get("Id")
		if txtID == "" {
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
			return
		}
		var req domain.GetUserLIDRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
			return
		}
		result, err := h.getUserLID.Execute(r.Context(), txtID, req)
		if err != nil {
			customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
			return
		}
		customhttp.RespondJSON(w, http.StatusOK, result, nil)
	})
}

// BlockUser retorna o handler para POST /user/block.
func (h *UserHandlers) BlockUser() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
		if !ok || info == nil {
			customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
			return
		}
		txtID := info.Get("Id")
		if txtID == "" {
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
			return
		}
		var req domain.BlockUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
			return
		}
		result, err := h.blockUser.Execute(r.Context(), txtID, req)
		if err != nil {
			customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
			return
		}
		customhttp.RespondJSON(w, http.StatusOK, result, nil)
	})
}

// UnblockUser retorna o handler para POST /user/unblock.
func (h *UserHandlers) UnblockUser() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
		if !ok || info == nil {
			customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
			return
		}
		txtID := info.Get("Id")
		if txtID == "" {
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
			return
		}
		var req domain.UnblockUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
			return
		}
		result, err := h.unblockUser.Execute(r.Context(), txtID, req)
		if err != nil {
			customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
			return
		}
		customhttp.RespondJSON(w, http.StatusOK, result, nil)
	})
}
