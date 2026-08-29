package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/hlog"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
	customhttp "wa-api/pkg/presentation/http"
	dtoadmin "wa-api/pkg/presentation/http/dto/admin"
	dtouser "wa-api/pkg/presentation/http/dto/user"

	"wa-api/pkg/application/usecase/user"
)

// UserHandlers agrupa todos os handlers de usuário.
type UserHandlers struct {
	listUsers      *user.ListUsersUseCase
	addUser        *user.AddUserUseCase
	editUser       *user.EditUserUseCase
	deleteUser     *user.DeleteUserUseCase
	checkUser      *user.CheckUserUseCase
	getUser        *user.GetUserUseCase
	getUserLID     *user.GetUserLIDUseCase
	getUserProfile *user.GetUserProfileUseCase
	listChats      *user.ListChatsUseCase
	blockUser      *user.BlockUserUseCase
	unblockUser    *user.UnblockUserUseCase
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
	getUserProfile *user.GetUserProfileUseCase,
	listChats *user.ListChatsUseCase,
	blockUser *user.BlockUserUseCase,
	unblockUser *user.UnblockUserUseCase,
) *UserHandlers {
	return &UserHandlers{
		listUsers:      listUsers,
		addUser:        addUser,
		editUser:       editUser,
		deleteUser:     deleteUser,
		checkUser:      checkUser,
		getUser:        getUser,
		getUserLID:     getUserLID,
		getUserProfile: getUserProfile,
		listChats:      listChats,
		blockUser:      blockUser,
		unblockUser:    unblockUser,
	}
}

// ListUsers retorna o handler para GET /admin/users.
func (h *UserHandlers) ListUsers() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// mux.Vars, and not r.PathValue: the router is gorilla/mux. The `id`
		// is the user id of GET /admin/users/{id}; on GET /admin/users it is
		// empty, which the use case reads as "every user".
		vars := mux.Vars(r)
		userID := vars["id"]
		result, err := h.listUsers.Execute(r.Context(), domain.ListUsersInput{UserID: userID})
		if err != nil {
			hlog.FromRequest(r).Error().Err(err).
				Str("path", r.URL.Path).
				Msg("use case failed")
			customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
			return
		}
		customhttp.RespondJSON(w, http.StatusOK, dtoadmin.PresentListUsers(result), nil)
	})
}

// AddUser retorna o handler para POST /admin/users.
func (h *UserHandlers) AddUser() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req dtoadmin.AddUserRequest
		if err := decodeRequest(w, r, &req); err != nil {
			if requestAnswered(err) {
				return
			}
			hlog.FromRequest(r).Warn().Err(err).
				Str("path", r.URL.Path).
				Msg("could not decode payload")
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
			return
		}
		if err := req.Validate(); err != nil {
			hlog.FromRequest(r).Warn().Err(err).
				Str("path", r.URL.Path).
				Msg("add user rejected: invalid payload")
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, err)
			return
		}
		result, err := h.addUser.Execute(r.Context(), req.ToDomain())
		if err != nil {
			// ErrDuplicateToken era sempre reportado como 500 — o caller não
			// tinha como distinguir "token já existe" (chamada idempotente,
			// re-provisionar o mesmo usuário) de um erro real de servidor.
			// Provisionamento repetido do mesmo token é o caso comum de um
			// client que reconecta/reenvia (ex.: retry de pareamento).
			if errors.Is(err, user.ErrDuplicateToken) {
				hlog.FromRequest(r).Warn().Err(err).
					Str("path", r.URL.Path).
					Msg("add user rejected: token already provisioned")
				customhttp.RespondJSON(w, http.StatusConflict, nil, err)
				return
			}
			hlog.FromRequest(r).Error().Err(err).
				Str("path", r.URL.Path).
				Msg("add user use case failed")
			customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
			return
		}
		customhttp.RespondJSON(w, http.StatusOK, dtoadmin.PresentUserPointer(result), nil)
	})
}

// EditUser retorna o handler para PUT /admin/users/{id}.
func (h *UserHandlers) EditUser() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The id comes from the PATH and never from the body: two sources for
		// the same identity is one source too many.
		userID := mux.Vars(r)["id"]
		var req dtoadmin.EditUserRequest
		if err := decodeRequest(w, r, &req); err != nil {
			if requestAnswered(err) {
				return
			}
			hlog.FromRequest(r).Warn().Err(err).
				Str("path", r.URL.Path).
				Msg("could not decode payload")
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
			return
		}
		if err := req.Validate(); err != nil {
			hlog.FromRequest(r).Warn().Err(err).
				Str("user_id", userID).
				Msg("edit user rejected: invalid payload")
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, err)
			return
		}
		if err := h.editUser.Execute(r.Context(), req.ToDomain(userID)); err != nil {
			hlog.FromRequest(r).Error().Err(err).
				Str("user_id", userID).
				Msg("edit user use case failed")
			customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
			return
		}
		customhttp.RespondJSON(w, http.StatusOK, dtoadmin.PresentEditUser(), nil)
	})
}

// DeleteUser retorna o handler para DELETE /admin/users/{id}.
func (h *UserHandlers) DeleteUser() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		req := domain.DeleteUserInput{UserID: vars["id"]}
		if err := h.deleteUser.Execute(r.Context(), req); err != nil {
			hlog.FromRequest(r).Error().Err(err).
				Str("user_id", req.UserID).
				Msg("delete user use case failed")
			customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
			return
		}
		customhttp.RespondJSON(w, http.StatusOK, dtoadmin.PresentDeleteUser(), nil)
	})
}

// CheckUser retorna o handler para POST /user/check.
func (h *UserHandlers) CheckUser() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
		if !ok || info == nil {
			hlog.FromRequest(r).Warn().Err(errUnauthorized).
				Str("path", r.URL.Path).
				Msg("request without user info in context")
			customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
			return
		}
		txtID := info.Get("Id")
		if txtID == "" {
			hlog.FromRequest(r).Warn().Err(errMissingSessionID).
				Str("path", r.URL.Path).
				Msg("request with empty session id")
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
			return
		}
		var req dtouser.CheckUserRequest
		if err := decodeRequest(w, r, &req); err != nil {
			if requestAnswered(err) {
				return
			}
			hlog.FromRequest(r).Warn().Err(err).
				Str("path", r.URL.Path).
				Msg("could not decode payload")
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
			return
		}
		if err := req.Validate(); err != nil {
			hlog.FromRequest(r).Warn().Err(err).
				Str("path", r.URL.Path).
				Msg("payload rejected")
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, err)
			return
		}
		result, err := h.checkUser.Execute(r.Context(), txtID, req.ToDomain())
		if err != nil {
			hlog.FromRequest(r).Error().Err(err).
				Str("path", r.URL.Path).
				Msg("use case failed")
			customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
			return
		}
		customhttp.RespondJSON(w, http.StatusOK, dtouser.PresentCheckUser(result), nil)
	})
}

// GetUser retorna o handler para POST /user/info.
func (h *UserHandlers) GetUser() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
		if !ok || info == nil {
			hlog.FromRequest(r).Warn().Err(errUnauthorized).
				Str("path", r.URL.Path).
				Msg("request without user info in context")
			customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
			return
		}
		txtID := info.Get("Id")
		if txtID == "" {
			hlog.FromRequest(r).Warn().Err(errMissingSessionID).
				Str("path", r.URL.Path).
				Msg("request with empty session id")
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
			return
		}
		var req dtouser.CheckUserRequest
		if err := decodeRequest(w, r, &req); err != nil {
			if requestAnswered(err) {
				return
			}
			hlog.FromRequest(r).Warn().Err(err).
				Str("path", r.URL.Path).
				Msg("could not decode payload")
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
			return
		}
		if err := req.Validate(); err != nil {
			hlog.FromRequest(r).Warn().Err(err).
				Str("path", r.URL.Path).
				Msg("payload rejected")
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, err)
			return
		}
		input := req.ToDomain()
		input.Phone = normalizePhones(input.Phone)
		result, err := h.getUser.Execute(r.Context(), txtID, input)
		if err != nil {
			hlog.FromRequest(r).Error().Err(err).
				Str("path", r.URL.Path).
				Msg("use case failed")
			customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
			return
		}
		customhttp.RespondJSON(w, http.StatusOK, dtouser.PresentGetUserInfo(result), nil)
	})
}

// GetUserLID retorna o handler para GET /user/lid/{jid}.
func (h *UserHandlers) GetUserLID() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
		if !ok || info == nil {
			hlog.FromRequest(r).Warn().Err(errUnauthorized).
				Str("path", r.URL.Path).
				Msg("request without user info in context")
			customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
			return
		}
		txtID := info.Get("Id")
		if txtID == "" {
			hlog.FromRequest(r).Warn().Err(errMissingSessionID).
				Str("path", r.URL.Path).
				Msg("request with empty session id")
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
			return
		}
		// O JID vem do CAMINHO, como a rota (`/user/lid/{jid}`) e o próprio
		// campo do domínio (`JID string // from URL`) sempre disseram. Até a
		// F81 este handler decodificava o corpo: um GET não tem corpo, o
		// decode falhava com EOF e a rota devolvia 400 em toda chamada.
		//
		// mux.Vars, e NÃO r.PathValue: o router é gorilla/mux
		// (bootstrap/router.go:237), que guarda as variáveis no contexto sob
		// chave própria. r.PathValue só funciona com o ServeMux nativo, e
		// aqui devolveria string vazia — um 400 diferente, igualmente
		// inútil. ListUsers, neste mesmo arquivo, já usa mux.Vars.
		req := domain.GetUserLIDRequest{JID: mux.Vars(r)["jid"]}
		if req.JID == "" {
			hlog.FromRequest(r).Warn().Err(errMissingJID).
				Str("path", r.URL.Path).
				Msg("request without jid in path")
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingJID)
			return
		}
		result, err := h.getUserLID.Execute(r.Context(), txtID, req)
		if err != nil {
			hlog.FromRequest(r).Error().Err(err).
				Str("path", r.URL.Path).
				Msg("use case failed")
			customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
			return
		}
		customhttp.RespondJSON(w, http.StatusOK, dtouser.PresentGetUserLID(result), nil)
	})
}

// GetUserProfile retorna o handler para GET /user/profile/{jid}.
//
// Aceita telefone, JID de telefone ou LID no caminho — o use case descobre
// qual chegou e resolve a contraparte. mux.Vars, e nao r.PathValue: o router
// e' gorilla/mux (ver F81, onde essa troca era o erro que quase entrou no
// lugar do defeito).
func (h *UserHandlers) GetUserProfile() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
		if !ok || info == nil {
			hlog.FromRequest(r).Warn().Err(errUnauthorized).
				Str("path", r.URL.Path).
				Msg("request without user info in context")
			customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
			return
		}
		txtID := info.Get("Id")
		if txtID == "" {
			hlog.FromRequest(r).Warn().Err(errMissingSessionID).
				Str("path", r.URL.Path).
				Msg("request with empty session id")
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
			return
		}
		alvo := mux.Vars(r)["jid"]
		if alvo == "" {
			hlog.FromRequest(r).Warn().Err(errMissingJID).
				Str("path", r.URL.Path).
				Msg("request without jid in path")
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingJID)
			return
		}
		if !isPlausibleJIDOrPhone(alvo) {
			err := apperr.New(CodeInvalidJID, apperr.CategoryValidation,
				"jid must be a phone number or a qualified JID (user@server)", false, nil)
			hlog.FromRequest(r).Warn().Err(err).
				Str("path", r.URL.Path).Str("jid", alvo).
				Msg("malformed jid in path")
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, err)
			return
		}
		result, err := h.getUserProfile.Execute(r.Context(), txtID, alvo)
		if err != nil {
			hlog.FromRequest(r).Warn().Err(err).
				Str("path", r.URL.Path).
				Msg("use case failed")
			customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
			return
		}
		customhttp.RespondJSON(w, http.StatusOK, dtouser.PresentUserProfile(result), nil)
	})
}

// ListChats retorna o handler para GET /chat/list.
//
// limit e offset vêm da query string. Valor não numérico é tratado como
// AUSENTE, não como erro: `?limit=abc` recebe o padrão em vez de um 400 —
// a rota é de leitura e recusar a chamada inteira por um parâmetro
// decorativo seria desproporcional. O use case corrige faixa.
func (h *UserHandlers) ListChats() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
		if !ok || info == nil {
			hlog.FromRequest(r).Warn().Err(errUnauthorized).
				Str("path", r.URL.Path).
				Msg("request without user info in context")
			customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
			return
		}
		txtID := info.Get("Id")
		if txtID == "" {
			hlog.FromRequest(r).Warn().Err(errMissingSessionID).
				Str("path", r.URL.Path).
				Msg("request with empty session id")
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
			return
		}
		limit := inteiroDaQuery(r, "limit")
		offset := inteiroDaQuery(r, "offset")

		result, err := h.listChats.Execute(r.Context(), txtID, limit, offset)
		if err != nil {
			hlog.FromRequest(r).Warn().Err(err).
				Str("path", r.URL.Path).
				Msg("use case failed")
			customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
			return
		}
		customhttp.RespondJSON(w, http.StatusOK, result, nil)
	})
}

// isPlausibleJIDOrPhone rejects inputs that are clearly neither a phone
// number nor a qualified JID.  A phone is digits only with 7-20 chars; a
// JID contains '@'.  The check is deliberately lenient — deep validation
// happens in the use case / wa-noise layer — but it catches the measured
// case (F238): a garbage string that triggers a 500 in the downstream.
func isPlausibleJIDOrPhone(s string) bool {
	if strings.ContainsRune(s, '@') {
		return true
	}
	if len(s) < 7 || len(s) > 20 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

const whatsappSuffix = "@s.whatsapp.net"

// normalizePhones appends the WhatsApp suffix to bare phone numbers in the
// slice, leaving already-qualified JIDs untouched. This is the handler-level
// decision from F242/F225: the rest of the API accepts bare phones, so
// /user/info must too.
func normalizePhones(phones []string) []string {
	out := make([]string, len(phones))
	for i, p := range phones {
		if !strings.Contains(p, "@") {
			out[i] = p + whatsappSuffix
		} else {
			out[i] = p
		}
	}
	return out
}

// inteiroDaQuery devolve 0 quando o parâmetro falta OU não é número — os
// dois casos significam "não pedi", e o use case aplica o padrão.
func inteiroDaQuery(r *http.Request, nome string) int {
	bruto := r.URL.Query().Get(nome)
	if bruto == "" {
		return 0
	}
	n, err := strconv.Atoi(bruto)
	if err != nil {
		return 0
	}
	return n
}

// BlockUser retorna o handler para POST /user/block.
func (h *UserHandlers) BlockUser() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
		if !ok || info == nil {
			hlog.FromRequest(r).Warn().Err(errUnauthorized).
				Str("path", r.URL.Path).
				Msg("request without user info in context")
			customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
			return
		}
		txtID := info.Get("Id")
		if txtID == "" {
			hlog.FromRequest(r).Warn().Err(errMissingSessionID).
				Str("path", r.URL.Path).
				Msg("request with empty session id")
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
			return
		}
		var req dtouser.BlockUserRequest
		if err := decodeRequest(w, r, &req); err != nil {
			if requestAnswered(err) {
				return
			}
			hlog.FromRequest(r).Warn().Err(err).
				Str("path", r.URL.Path).
				Msg("could not decode payload")
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
			return
		}
		if err := req.Validate(); err != nil {
			hlog.FromRequest(r).Warn().Err(err).
				Str("path", r.URL.Path).
				Msg("payload rejected")
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, err)
			return
		}
		result, err := h.blockUser.Execute(r.Context(), txtID, req.ToBlockDomain())
		if err != nil {
			hlog.FromRequest(r).Error().Err(err).
				Str("path", r.URL.Path).
				Msg("use case failed")
			customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
			return
		}
		customhttp.RespondJSON(w, http.StatusOK, dtouser.PresentBlockResult(result), nil)
	})
}

// UnblockUser retorna o handler para POST /user/unblock.
func (h *UserHandlers) UnblockUser() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
		if !ok || info == nil {
			hlog.FromRequest(r).Warn().Err(errUnauthorized).
				Str("path", r.URL.Path).
				Msg("request without user info in context")
			customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
			return
		}
		txtID := info.Get("Id")
		if txtID == "" {
			hlog.FromRequest(r).Warn().Err(errMissingSessionID).
				Str("path", r.URL.Path).
				Msg("request with empty session id")
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
			return
		}
		var req dtouser.BlockUserRequest
		if err := decodeRequest(w, r, &req); err != nil {
			if requestAnswered(err) {
				return
			}
			hlog.FromRequest(r).Warn().Err(err).
				Str("path", r.URL.Path).
				Msg("could not decode payload")
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
			return
		}
		if err := req.Validate(); err != nil {
			hlog.FromRequest(r).Warn().Err(err).
				Str("path", r.URL.Path).
				Msg("payload rejected")
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, err)
			return
		}
		result, err := h.unblockUser.Execute(r.Context(), txtID, req.ToUnblockDomain())
		if err != nil {
			hlog.FromRequest(r).Error().Err(err).
				Str("path", r.URL.Path).
				Msg("use case failed")
			customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
			return
		}
		customhttp.RespondJSON(w, http.StatusOK, dtouser.PresentUnblockResult(result), nil)
	})
}
