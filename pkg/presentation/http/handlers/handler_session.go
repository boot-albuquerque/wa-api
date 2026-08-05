package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	customhttp "wa-api/pkg/presentation/http"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"

	"github.com/rs/zerolog/hlog"

	"wa-api/pkg/application/usecase/session"
)

// isClientCausedSessionError decide o NIVEL do log do caminho de saida vindo
// do use case, seguindo a taxonomia da apperr: erro cuja categoria mapeia para
// 4xx e' falha do cliente (warn); qualquer outra e' falha real (error). Sem
// isso o 400 de `no_session` e o 500 de banco fora do ar sairiam no mesmo
// nivel. So' a DECISAO mora aqui — a cadeia hlog.FromRequest(r) fica inline em
// cada caminho de saida, porque cmd/logcov so' enxerga o log onde a cadeia
// literalmente esta'.
func isClientCausedSessionError(err error) bool {
	var appErr *apperr.AppError
	return errors.As(err, &appErr) && appErr.Category.HTTPStatus() < http.StatusInternalServerError
}

func sessionUser(w http.ResponseWriter, r *http.Request) (string, bool) {
	info, _ := r.Context().Value(appport.UserInfoKey).(userInfo)
	if info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).Str("path", r.URL.Path).Msg("session request rejected")
		customhttp.RespondJSON(w, 401, nil, errUnauthorized)
		return "", false
	}
	id := info.Get("Id")
	if id == "" {
		hlog.FromRequest(r).Warn().Err(errMissingSessionID).Str("path", r.URL.Path).Msg("session request rejected")
		customhttp.RespondJSON(w, 400, nil, errMissingSessionID)
		return "", false
	}
	return id, true
}

// ConnectHandler handles GET /session/connect. After validation, it spawns
// a goroutine to start the WhatsApp WebSocket connection.
type ConnectHandler struct {
	usecase     *session.ConnectUseCase
	StartClient func(userID, jid, token string, kill chan bool) // injected by bootstrap
}

func NewConnectHandler(uc *session.ConnectUseCase) *ConnectHandler {
	return &ConnectHandler{usecase: uc}
}

// WithStartClient injects the WhatsApp WebSocket launcher.
func (h *ConnectHandler) WithStartClient(fn func(userID, jid, token string, kill chan bool)) *ConnectHandler {
	h.StartClient = fn
	return h
}

func (h *ConnectHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	hlog.FromRequest(r).Info().Str("handler", "Connect").Str("id", id).Msg("ConnectHandler called")
	// ConnectUseCase.Execute nunca retorna erro hoje (session/connect.go) - o
	// branch abaixo e' so' defesa contra uma mudanca futura que passe a
	// devolver um, e por isso fica com um unico nivel (Error), nao o mesmo
	// split Warn/Error dos outros handlers desta rota.
	_, err := h.usecase.Execute(r.Context(), id, domain.ConnectRequest{})
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("handler", "Connect").Str("user_id", id).Msg("session use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	hlog.FromRequest(r).Info().Str("id", id).Bool("hasStartClient", h.StartClient != nil).Msg("ConnectHandler starting WhatsApp client")
	// Fire-and-forget: start WhatsApp client in background (QR code appears in terminal)
	if h.StartClient != nil {
		kill := make(chan bool, 1)
		go h.StartClient(id, "", "", kill)
	}
	customhttp.RespondJSON(w, 200, map[string]interface{}{"status": "connecting"}, nil)
}

// DisconnectHandler handles POST /session/disconnect/{id}
type DisconnectHandler struct{ usecase *session.DisconnectUseCase }

func NewDisconnectHandler(uc *session.DisconnectUseCase) *DisconnectHandler {
	return &DisconnectHandler{uc}
}
func (h *DisconnectHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), id, domain.DisconnectRequest{})
	if err != nil {
		if isClientCausedSessionError(err) {
			hlog.FromRequest(r).Warn().Err(err).Str("handler", "Disconnect").Str("user_id", id).Msg("session use case failed")
			customhttp.RespondJSON(w, 500, nil, err)
			return
		}
		hlog.FromRequest(r).Error().Err(err).Str("handler", "Disconnect").Str("user_id", id).Msg("session use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// GetQRHandler handles GET /session/qr/{id}
type GetQRHandler struct{ usecase *session.GetQRUseCase }

func NewGetQRHandler(uc *session.GetQRUseCase) *GetQRHandler { return &GetQRHandler{uc} }
func (h *GetQRHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), id)
	if err != nil {
		if isClientCausedSessionError(err) {
			hlog.FromRequest(r).Warn().Err(err).Str("handler", "GetQR").Str("user_id", id).Msg("session use case failed")
			customhttp.RespondJSON(w, 500, nil, err)
			return
		}
		hlog.FromRequest(r).Error().Err(err).Str("handler", "GetQR").Str("user_id", id).Msg("session use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// LogoutHandler handles POST /session/logout/{id}
type LogoutHandler struct{ usecase *session.LogoutUseCase }

func NewLogoutHandler(uc *session.LogoutUseCase) *LogoutHandler { return &LogoutHandler{uc} }
func (h *LogoutHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), id, domain.LogoutRequest{})
	if err != nil {
		if isClientCausedSessionError(err) {
			hlog.FromRequest(r).Warn().Err(err).Str("handler", "Logout").Str("user_id", id).Msg("session use case failed")
			customhttp.RespondJSON(w, 500, nil, err)
			return
		}
		hlog.FromRequest(r).Error().Err(err).Str("handler", "Logout").Str("user_id", id).Msg("session use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// PairPhoneHandler handles POST /session/pairphone/{id}
type PairPhoneHandler struct{ usecase *session.PairPhoneUseCase }

func NewPairPhoneHandler(uc *session.PairPhoneUseCase) *PairPhoneHandler {
	return &PairPhoneHandler{uc}
}
func (h *PairPhoneHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.PairPhoneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Str("path", r.URL.Path).Msg("session request rejected")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), id, req)
	if err != nil {
		if isClientCausedSessionError(err) {
			hlog.FromRequest(r).Warn().Err(err).Str("handler", "PairPhone").Str("user_id", id).Msg("session use case failed")
			customhttp.RespondJSON(w, 500, nil, err)
			return
		}
		hlog.FromRequest(r).Error().Err(err).Str("handler", "PairPhone").Str("user_id", id).Msg("session use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// GetStatusHandler handles GET /session/status/{id}
type GetStatusHandler struct{ usecase *session.GetStatusUseCase }

func NewGetStatusHandler(uc *session.GetStatusUseCase) *GetStatusHandler {
	return &GetStatusHandler{uc}
}
func (h *GetStatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), id)
	if err != nil {
		if isClientCausedSessionError(err) {
			hlog.FromRequest(r).Warn().Err(err).Str("handler", "GetStatus").Str("user_id", id).Msg("session use case failed")
			customhttp.RespondJSON(w, 500, nil, err)
			return
		}
		hlog.FromRequest(r).Error().Err(err).Str("handler", "GetStatus").Str("user_id", id).Msg("session use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// SetStatusMessageHandler handles POST /session/statusmessage/{id}
type SetStatusMessageHandler struct {
	usecase *session.SetStatusMessageUseCase
}

func NewSetStatusMessageHandler(uc *session.SetStatusMessageUseCase) *SetStatusMessageHandler {
	return &SetStatusMessageHandler{uc}
}
func (h *SetStatusMessageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.SetStatusMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Str("path", r.URL.Path).Msg("session request rejected")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), id, req)
	if err != nil {
		if isClientCausedSessionError(err) {
			hlog.FromRequest(r).Warn().Err(err).Str("handler", "SetStatusMessage").Str("user_id", id).Msg("session use case failed")
			customhttp.RespondJSON(w, 500, nil, err)
			return
		}
		hlog.FromRequest(r).Error().Err(err).Str("handler", "SetStatusMessage").Str("user_id", id).Msg("session use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// RequestHistorySyncHandler handles POST /session/historysync/{id}
type RequestHistorySyncHandler struct {
	usecase *session.RequestHistorySyncUseCase
}

func NewRequestHistorySyncHandler(uc *session.RequestHistorySyncUseCase) *RequestHistorySyncHandler {
	return &RequestHistorySyncHandler{uc}
}
func (h *RequestHistorySyncHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), id, domain.RequestHistorySyncRequest{})
	if err != nil {
		if isClientCausedSessionError(err) {
			hlog.FromRequest(r).Warn().Err(err).Str("handler", "RequestHistorySync").Str("user_id", id).Msg("session use case failed")
			customhttp.RespondJSON(w, 500, nil, err)
			return
		}
		hlog.FromRequest(r).Error().Err(err).Str("handler", "RequestHistorySync").Str("user_id", id).Msg("session use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// SyncContactRosterHandler handles POST /user/contacts/sync.
//
// Capacidade nova e distinta de RequestHistorySyncHandler: força o pull do
// patch de app-state que carrega a agenda de contatos, não histórico de
// mensagens.
type SyncContactRosterHandler struct {
	usecase *session.SyncContactRosterUseCase
}

func NewSyncContactRosterHandler(uc *session.SyncContactRosterUseCase) *SyncContactRosterHandler {
	return &SyncContactRosterHandler{uc}
}
func (h *SyncContactRosterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.SyncContactRosterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Str("path", r.URL.Path).Msg("session request rejected")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), id, req)
	if err != nil {
		if isClientCausedSessionError(err) {
			hlog.FromRequest(r).Warn().Err(err).Str("handler", "SyncContactRoster").Str("user_id", id).Msg("session use case failed")
			customhttp.RespondJSON(w, 500, nil, err)
			return
		}
		hlog.FromRequest(r).Error().Err(err).Str("handler", "SyncContactRoster").Str("user_id", id).Msg("session use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}
