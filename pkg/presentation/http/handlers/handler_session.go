package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	customhttp "wa-api/pkg/presentation/http"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
	"wa-api/pkg/pairing"
	dtosession "wa-api/pkg/presentation/http/dto/session"

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
	// Cancelamento do cliente entra aqui (F90): o navegador desistiu da
	// requisição — aba fechada, navegação, fetch abortado. Não é falha do
	// servidor e não pode sair em `error`, senão uma troca de aba fica
	// indistinguível de banco fora do ar.
	if apperr.IsClientGaveUp(err) {
		return true
	}
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
	usecase            *session.ConnectUseCase
	pairing            *pairing.Registry
	CheckStartInFlight func(userID string) error // injected by bootstrap (F274)
}

// NewConnectHandler builds the handler over the pairing provider registry.
//
// Until 2026-08-27 it took two injected closures (StartSession, CheckOwnership)
// wired straight to the wa-noise orchestrator, with no engine in sight. Both
// now come from the provider the registry resolves for the engine the REQUEST
// names and the TARGET session records. See pkg/pairing and HOUSEKEEP F273.
func NewConnectHandler(uc *session.ConnectUseCase, reg *pairing.Registry) *ConnectHandler {
	return &ConnectHandler{usecase: uc, pairing: reg}
}

// WithCheckStartInFlight injects a synchronous pre-check (F274) for a
// pairing flow already in progress for this user.
//
// Without it, the guard lives entirely inside Start, which runs in a
// goroutine fired AFTER the handler already responded 200. A client hitting
// `GET /session/connect` while a previous pairing flow was still active (for
// example, right after `/session/disconnect` cut the transport but left the
// flow running) got `200 {"status":"connecting"}` for an attempt that never
// started — the same class of lie F108 closed for ownership.
func (h *ConnectHandler) WithCheckStartInFlight(fn func(userID string) error) *ConnectHandler {
	h.CheckStartInFlight = fn
	return h
}

func (h *ConnectHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	actorID, ok := sessionUser(w, r)
	if !ok {
		return
	}
	targetID := pairingTarget(r, actorID)
	hlog.FromRequest(r).Info().Str("handler", "Connect").Str("id", targetID).Msg("ConnectHandler called")

	starter, err := h.pairing.ResolveStarter(r.Context(), targetID, pairingEngineFromQuery(r))
	if err != nil {
		respondPairingRefusal(w, r, "Connect", targetID, err)
		return
	}

	// ConnectUseCase.Execute nunca retorna erro hoje (session/connect.go) - o
	// branch abaixo e' so' defesa contra uma mudanca futura que passe a
	// devolver um, e por isso fica com um unico nivel (Error), nao o mesmo
	// split Warn/Error dos outros handlers desta rota.
	if _, err := h.usecase.Execute(r.Context(), targetID, domain.ConnectRequest{}); err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("handler", "Connect").Str("user_id", targetID).Msg("session use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}

	// F274: in-flight check BEFORE ownership, mirroring the order Start
	// itself uses internally (inFlight.acquire runs before claimOwnership).
	// Unlike the ownership check, this one is a read-only PEEK — see
	// Orchestrator.CheckStartAvailable — so it does not touch the guard that
	// Start's own acquire, moments later inside the goroutine, still needs to
	// succeed.
	if h.CheckStartInFlight != nil {
		if flightErr := h.CheckStartInFlight(targetID); flightErr != nil {
			if isClientCausedSessionError(flightErr) {
				hlog.FromRequest(r).Warn().Err(flightErr).Str("handler", "Connect").Str("user_id", targetID).Msg("session start already in flight")
			} else {
				hlog.FromRequest(r).Error().Err(flightErr).Str("handler", "Connect").Str("user_id", targetID).Msg("session start already in flight")
			}
			customhttp.RespondJSON(w, 500, nil, flightErr)
			return
		}
	}

	// F108: ownership check BEFORE responding. Without this, the handler
	// fires startSession in a goroutine and responds 200 "connecting" without
	// knowing whether ownership was denied — the client receives success for
	// a request that will never connect. The check is idempotent: the
	// orchestrator claims the same lease again inside Start, succeeds because
	// the owner is the same process, and releases on failure via its own defer.
	if ownerErr := starter.CheckOwnership(r.Context(), targetID); ownerErr != nil {
		if isClientCausedSessionError(ownerErr) {
			hlog.FromRequest(r).Warn().Err(ownerErr).Str("handler", "Connect").Str("user_id", targetID).Msg("session ownership denied")
		} else {
			hlog.FromRequest(r).Error().Err(ownerErr).Str("handler", "Connect").Str("user_id", targetID).Msg("session ownership denied")
		}
		customhttp.RespondJSON(w, 500, nil, ownerErr)
		return
	}

	hlog.FromRequest(r).Info().Str("id", targetID).Msg("ConnectHandler starting WhatsApp client")
	// Fire-and-forget: start WhatsApp client in background. The token comes
	// from the ACTOR's context — it is the credential the transport presents,
	// which is a different question from which engine serves the session.
	// sessionUser ja' validou acima que o contexto tem um userInfo valido.
	info, _ := r.Context().Value(appport.UserInfoKey).(userInfo)
	starter.StartSession(r.Context(), targetID, info.Get("Token"))

	customhttp.RespondJSON(w, 200, dtosession.PresentConnect(nil), nil)
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
	customhttp.RespondJSON(w, 200, dtosession.PresentDisconnect(rsp), nil)
}

// GetQRHandler handles GET /session/qr?engine=...
//
// `engine` is a QUERY PARAMETER and not a body field because this route is a
// GET and stays one: it reads the code the engine has in offer and has no side
// effect, so it keeps caching, retries and browser navigation working. A GET
// with a body is not a contract this repository is going to start. See the
// GET-vs-POST note in HOUSEKEEP F281.
// codeAge tracks, per txtID, the code this handler last returned and since
// when — the state behind GetQRResponse.CodeAgeSeconds (HOUSEKEEP F375/
// F377). GetQRHandler is the only object common to BOTH engines that lives
// across separate HTTP requests: the use case and the PairingQRReader the
// registry resolves are both built PER CALL (see NewGetQRHandler's own
// comment on why), so neither can hold this across polls the way this
// handler can.
type codeAge struct {
	code  string
	since time.Time
}

type GetQRHandler struct {
	logger  appport.Logger
	pairing *pairing.Registry

	mu    sync.Mutex
	since map[string]codeAge
}

// NewGetQRHandler builds the handler over the pairing provider registry.
//
// The use case is built PER REQUEST, from the port the registry resolves,
// instead of once at wiring time from a fixed adapter — that fixed adapter was
// the defect (HOUSEKEEP F273). GetQRUseCase holds only its port and a logger,
// so building one costs a struct literal.
func NewGetQRHandler(l appport.Logger, reg *pairing.Registry) *GetQRHandler {
	return &GetQRHandler{logger: l, pairing: reg, since: make(map[string]codeAge)}
}

// trackCodeAge records the code just about to be returned for txtID and
// reports how long (seconds) it has ALREADY been the same code — 0 if it
// just changed, or if it is empty (nothing to age: not ready, or paired).
// A changed or empty code also DROPS the tracked entry, so a later pairing
// attempt for the same txtID starts its own clock instead of inheriting one
// from an unrelated earlier pairing screen — same reasoning as
// pkg/infra/wa-headless/pairing.QRReader.forgetCode (F374), independent
// tracker, same shape.
func (h *GetQRHandler) trackCodeAge(txtID, code string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	if code == "" {
		delete(h.since, txtID)
		return 0
	}
	prev, ok := h.since[txtID]
	if ok && prev.code == code {
		return int(time.Since(prev.since).Seconds())
	}
	h.since[txtID] = codeAge{code: code, since: time.Now()}
	return 0
}

func (h *GetQRHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	actorID, ok := sessionUser(w, r)
	if !ok {
		return
	}
	targetID := pairingTarget(r, actorID)

	reader, err := h.pairing.ResolveQRReader(r.Context(), targetID, pairingEngineFromQuery(r))
	if err != nil {
		respondPairingRefusal(w, r, "GetQR", targetID, err)
		return
	}

	rsp, err := session.NewGetQRUseCase(reader, h.logger).Execute(r.Context(), targetID)
	if err != nil {
		if isClientCausedSessionError(err) {
			hlog.FromRequest(r).Warn().Err(err).Str("handler", "GetQR").Str("user_id", targetID).Msg("session use case failed")
			customhttp.RespondJSON(w, 500, nil, err)
			return
		}
		hlog.FromRequest(r).Error().Err(err).Str("handler", "GetQR").Str("user_id", targetID).Msg("session use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	rsp.CodeAgeSeconds = h.trackCodeAge(targetID, rsp.QRCode)
	customhttp.RespondJSON(w, 200, dtosession.PresentGetQR(rsp), nil)
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
	customhttp.RespondJSON(w, 200, dtosession.PresentLogout(rsp), nil)
}

// PairPhoneHandler handles POST /session/pairphone.
//
// Body: {"engine":"wa_noise","phone":"5541999999999"}. `engine` is mandatory —
// see dtosession.PairPhoneRequest for the wire contract.
type PairPhoneHandler struct {
	logger  appport.Logger
	pairing *pairing.Registry
}

// NewPairPhoneHandler builds the handler over the pairing provider registry.
// Same reason as GetQR: the use case is built per request from the resolved
// port, because the wiring-time adapter was hardcoded to wa-noise (F273).
func NewPairPhoneHandler(l appport.Logger, reg *pairing.Registry) *PairPhoneHandler {
	return &PairPhoneHandler{logger: l, pairing: reg}
}

func (h *PairPhoneHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	actorID, ok := sessionUser(w, r)
	if !ok {
		return
	}
	targetID := pairingTarget(r, actorID)

	var req dtosession.PairPhoneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Str("path", r.URL.Path).Msg("session request rejected")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	domainReq := req.ToDomain()

	// The engine is resolved BEFORE the phone number is validated, and the
	// order is load-bearing: a request naming an engine that does not serve
	// pairing must be refused without a provider being touched, whatever else
	// is wrong with it. PairPhoneUseCase's own missing_phone guard runs after.
	pairer, err := h.pairing.ResolvePhonePairer(r.Context(), targetID, domainReq.Engine)
	if err != nil {
		respondPairingRefusal(w, r, "PairPhone", targetID, err)
		return
	}

	rsp, err := session.NewPairPhoneUseCase(pairer, h.logger).Execute(r.Context(), targetID, domainReq)
	if err != nil {
		if isClientCausedSessionError(err) {
			hlog.FromRequest(r).Warn().Err(err).Str("handler", "PairPhone").Str("user_id", targetID).Msg("session use case failed")
			customhttp.RespondJSON(w, 500, nil, err)
			return
		}
		hlog.FromRequest(r).Error().Err(err).Str("handler", "PairPhone").Str("user_id", targetID).Msg("session use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtosession.PresentPairPhone(rsp), nil)
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
	customhttp.RespondJSON(w, 200, dtosession.PresentGetStatus(rsp), nil)
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
	var req dtosession.SetStatusMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Str("path", r.URL.Path).Msg("session request rejected")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), id, req.ToDomain())
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
	customhttp.RespondJSON(w, 200, dtosession.PresentSetStatusMessage(rsp), nil)
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
	// F198: até 2026-08-21 o handler passava um pedido VAZIO. O DTO
	// documentava count, chat_jid, oldest_msg_id, oldest_msg_from_me e
	// oldest_msg_timestamp, e nenhum deles era lido — o corpo do cliente ia
	// para o lixo e a rota respondia 200.
	var req dtosession.RequestHistorySyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		hlog.FromRequest(r).Warn().Err(err).Str("handler", "RequestHistorySync").Msg("could not decode payload")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, err)
		return
	}

	rsp, err := h.usecase.Execute(r.Context(), id, req.ToDomain())
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
	customhttp.RespondJSON(w, 200, dtosession.PresentRequestHistorySync(rsp), nil)
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
	var req dtosession.SyncContactRosterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Str("path", r.URL.Path).Msg("session request rejected")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), id, req.ToDomain())
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
	customhttp.RespondJSON(w, 200, dtosession.PresentSyncContactRoster(rsp), nil)
}
