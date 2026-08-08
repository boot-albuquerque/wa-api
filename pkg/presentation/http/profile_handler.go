package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/usecase/profile"
	"wa-api/pkg/domain/apperr"

	"github.com/rs/zerolog/hlog"
)

// Context key "userinfo" é injetada pelo middleware authalice do upstream.
// O valor é um struct com método Get(key string) string.
// Ambos package main e package http referenciam a mesma string key.
type userInfo interface {
	Get(key string) string
}

// Erros-sentinela da fronteira desta rota. São apperr com categoria, e não
// strings soltas, para que RespondJSON derive o status da taxonomia — o
// mesmo mecanismo que os handlers de /session/* já usam. Antes da F83 estes
// dois caminhos saíam por http.Error, em text/plain, fora do envelope.
var (
	errUnauthorized     = apperr.New("unauthorized", apperr.CategoryUnauthorized, "unauthorized", false, nil)
	errMissingSessionID = apperr.New("missing_session_id", apperr.CategoryValidation, "missing session id", false, nil)
)

// cacheControlPerfil desliga o cache nas duas rotas de perfil: o estado da
// sessão muda sozinho (conectar, desconectar, deslogar), e uma resposta em
// cache faria alguém depurar um estado que já não existe.
const cacheControlPerfil = "no-store, no-cache, must-revalidate, max-age=0"

// ProfileUseCase define o contrato de uso para obtenção de perfil.
type ProfileUseCase interface {
	Execute(ctx context.Context, txtID string) (string, error)
}

// ProfileHandler é o handler HTTP para GET /session/profile.
type ProfileHandler struct {
	usecase ProfileUseCase
}

// NewProfileHandler cria o handler com o usecase injetado.
func NewProfileHandler(uc ProfileUseCase) *ProfileHandler {
	return &ProfileHandler{usecase: uc}
}

// ServeHTTP implementa http.Handler para GET /session/profile.
// Extrai txtID do contexto (injetado pelo middleware authalice do upstream).
func (h *ProfileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		hlog.FromRequest(r).Warn().
			Str("path", r.URL.Path).
			Msg("profile request without user info in context")
		RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}

	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().
			Str("path", r.URL.Path).
			Msg("profile request with empty session id")
		RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
		return
	}

	response, err := h.usecase.Execute(r.Context(), txtID)
	if err != nil {
		// O nível segue a categoria, como nos handlers de /session/*: "não há
		// sessão" é recusa de cliente e sai em warn; o resto é falha nossa e
		// sai em error. Um alerta calibrado sobre `error` dispararia em falso
		// a cada consulta de perfil sem sessão se os dois se misturassem.
		var appErr *apperr.AppError
		if errors.As(err, &appErr) && appErr.Category == apperr.CategoryValidation {
			hlog.FromRequest(r).Warn().Err(err).
				Str("user_id", txtID).
				Msg("get profile use case failed")
		} else {
			hlog.FromRequest(r).Error().Err(err).
				Str("user_id", txtID).
				Msg("get profile use case failed")
		}
		// RespondJSON deriva o status da categoria do apperr; o 500 aqui só
		// vale para erro sem taxonomia.
		RespondJSON(w, http.StatusInternalServerError, nil, err)
		return
	}

	// RespondJSON envelopa em {code,data,success} (ADR-002), igual às demais
	// rotas de /session/*. Antes este handler escrevia `response` cru no
	// ResponseWriter — o cliente wa-worker (que desembrulha body.data como
	// todo o resto da API) sempre lia data=undefined e devolvia pushname/
	// avatar vazios, mesmo com o wa-noise retornando os campos certos.
	w.Header().Set("Cache-Control", cacheControlPerfil)
	RespondJSON(w, http.StatusOK, json.RawMessage(response), nil)
}

// ProfileFullUseCase é o contrato de `/session/profile/full`.
type ProfileFullUseCase interface {
	Execute(ctx context.Context, txtID string) (*profile.ProfileFullResult, error)
}

// ProfileFullHandler serve GET /session/profile/full.
//
// Rota separada de /session/profile de propósito: esta agrega chamadas de
// REDE (recado, privacidade) e está sujeita a latência e rate limit, enquanto
// a outra lê só o store local. Juntá-las tornaria a rota barata refém da cara.
type ProfileFullHandler struct {
	usecase ProfileFullUseCase
}

// NewProfileFullHandler cria o handler com o usecase injetado.
func NewProfileFullHandler(uc ProfileFullUseCase) *ProfileFullHandler {
	return &ProfileFullHandler{usecase: uc}
}

func (h *ProfileFullHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		hlog.FromRequest(r).Warn().
			Str("path", r.URL.Path).
			Msg("profile request without user info in context")
		RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}
	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().
			Str("path", r.URL.Path).
			Msg("profile request with empty session id")
		RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
		return
	}

	result, err := h.usecase.Execute(r.Context(), txtID)
	if err != nil {
		// Mesmo critério de nível da rota irmã (F83): recusa de cliente em
		// warn, falha nossa em error.
		var appErr *apperr.AppError
		if errors.As(err, &appErr) && appErr.Category == apperr.CategoryValidation {
			hlog.FromRequest(r).Warn().Err(err).Str("user_id", txtID).Msg("get profile full use case failed")
		} else {
			hlog.FromRequest(r).Error().Err(err).Str("user_id", txtID).Msg("get profile full use case failed")
		}
		RespondJSON(w, http.StatusInternalServerError, nil, err)
		return
	}

	w.Header().Set("Cache-Control", cacheControlPerfil)
	RespondJSON(w, http.StatusOK, result, nil)
}
