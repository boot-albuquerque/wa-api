package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/rs/zerolog/hlog"

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
		hlog.FromRequest(r).Warn().Err(err).
			Str("user_id", id).
			Msg("could not decode get avatar payload")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil {
		// ErrAvatarNotFound era sempre reportado como 500 — indistinguível de
		// falha real (sessão caída, JID inválido). "Contato sem foto pública"
		// é o caso comum, não um erro de servidor.
		if errors.Is(err, user.ErrAvatarNotFound) {
			hlog.FromRequest(r).Warn().Err(err).
				Str("user_id", id).
				Msg("get avatar: contact has no public photo")
			customhttp.RespondJSON(w, 404, nil, err)
			return
		}
		// ErrAvatarUnauthorized: o contato TEM foto, mas escondeu-a via
		// privacidade — distinto de "não tem foto" (404). 403 deixa o caller
		// (wa-worker) diferenciar "nunca vai ter foto" de "pode reaparecer se
		// a privacidade mudar", em vez de tratar os dois como o mesmo "sem avatar".
		if errors.Is(err, user.ErrAvatarUnauthorized) {
			hlog.FromRequest(r).Warn().Err(err).
				Str("user_id", id).
				Msg("get avatar: hidden by privacy settings")
			customhttp.RespondJSON(w, 403, nil, err)
			return
		}
		hlog.FromRequest(r).Error().Err(err).
			Str("user_id", id).
			Msg("get avatar use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// GetContactsLastActivityHandler expõe GetContactsLastActivityUseCase —
// timestamp da mensagem mais recente por chat_jid, derivado do backfill
// local de message_history (HistorySync pós-pareamento). Não decodifica
// body (GET, sem payload) e não depende de sessão whatsmeow ativa.
type GetContactsLastActivityHandler struct {
	uc *user.GetContactsLastActivityUseCase
}

func NewGetContactsLastActivityHandler(uc *user.GetContactsLastActivityUseCase) *GetContactsLastActivityHandler {
	return &GetContactsLastActivityHandler{uc: uc}
}
func (h *GetContactsLastActivityHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	rsp, err := h.uc.Execute(r.Context(), id)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).
			Str("user_id", id).
			Msg("get contacts last activity use case failed")
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
		hlog.FromRequest(r).Error().Err(err).
			Str("user_id", id).
			Msg("get contacts use case failed")
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
		hlog.FromRequest(r).Warn().Err(err).
			Str("user_id", id).
			Msg("could not decode get user info payload")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).
			Str("user_id", id).
			Msg("get user info use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// ContactHandlers agrupa os handlers de leitura de dados de contato
// (/user/info, /user/avatar, /user/contacts).
type ContactHandlers struct {
	Avatar       *GetAvatarHandler
	Contacts     *GetContactsHandler
	UserInfo     *GetUserInfoHandler
	LastActivity *GetContactsLastActivityHandler
}
