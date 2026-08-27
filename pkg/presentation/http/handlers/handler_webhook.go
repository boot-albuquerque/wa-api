package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
	customhttp "wa-api/pkg/presentation/http"

	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/hlog"
)

// WebhookHandlerDB is the minimal DB interface webhook handlers need.
type WebhookHandlerDB interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
	Exec(query string, args ...interface{}) (sql.Result, error)
}

// WebhookHandlerContext bundles the dependencies webhook handlers need.
type WebhookHandlerContext struct {
	DB              WebhookHandlerDB
	UserCache       *cache.Cache
	SupportedEvents []string
	FindInSlice     func(slice []string, val string) bool
	UpdateUserInfo  func(info interface{}, key, value string) interface{}

	// PublishUserInfo writes the updated Values to BOTH userinfo caches (F164).
	// The webhook handlers call it instead of UserCache.Set, so that a
	// webhook configuration change is visible to the dispatch side (keyed by
	// userID) as well as the HTTP side (keyed by token).
	//
	// This field is REQUIRED. A nil PublishUserInfo panics at construction
	// time (NewSetWebhookHandler et al.) — the same pattern as
	// bootstrap.NewRouter for Deps.UserCache (router.go:75-83). The F164
	// defect was exactly this: writing only one cache. A nil fallback that
	// writes only the token cache reintroduces it one layer up.
	PublishUserInfo func(userID, token string, values interface{})
}

// webhookStoreError types a failure of the webhook row's persistence.
//
// The four codes are distinct per operation, and the driver's error is WRAPPED
// rather than interpolated into Message: a database error text carries the
// statement, and with it column names, the connection target and, on a
// constraint violation, the offending value. RespondJSON never serialises the
// wrapped chain — but writing it into Message would put it on the wire by
// hand, past the very guard that exists to stop it.
func webhookStoreError(code, cause string, err error) error {
	return apperr.New(code, apperr.CategoryInternal,
		"Não foi possível ler ou gravar a configuração de webhook.", true,
		fmt.Errorf("%s: %w", cause, err))
}

// publishValues writes the updated values through PublishUserInfo.
func (ctx *WebhookHandlerContext) publishValues(userID, token string, values interface{}) {
	ctx.PublishUserInfo(userID, token, values)
}

// Os call sites abaixo inlineiam a cadeia hlog.FromRequest(r)...Msg(...) por
// completo (em vez de um helper compartilhado) porque cmd/logcov exige que a
// cadeia inteira, da chamada de nivel ate' o .Msg terminal, seja UMA unica
// expressao cuja raiz e' literalmente hlog.FromRequest(r) — nao uma variavel,
// nem uma chamada de funcao auxiliar (cmd/logcov/rules.go:317-414).

// GetWebhookHandler handles GET /webhook
type GetWebhookHandler struct{ ctx *WebhookHandlerContext }

func NewGetWebhookHandler(ctx *WebhookHandlerContext) *GetWebhookHandler {
	return &GetWebhookHandler{ctx}
}

func (h *GetWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).
			Str("handler", "GetWebhook").
			Msg("webhook request rejected")
		customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}
	txtid := info.Get("Id")
	rows, err := h.ctx.DB.Query("SELECT webhook,events FROM users WHERE id=$1 LIMIT 1", txtid)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).
			Str("handler", "GetWebhook").
			Str("op", "select").
			Str("user_id", txtid).
			Msg("webhook database operation failed")
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, webhookStoreError(CodeWebhookReadFailed, "could not get webhook", err))
		return
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			hlog.FromRequest(r).Warn().Err(closeErr).
				Str("handler", "GetWebhook").
				Msg("failed to close rows")
		}
	}()
	var webhook, events string
	for rows.Next() {
		if err := rows.Scan(&webhook, &events); err != nil {
			hlog.FromRequest(r).Error().Err(err).
				Str("handler", "GetWebhook").
				Str("op", "scan").
				Str("user_id", txtid).
				Msg("webhook database operation failed")
			customhttp.RespondJSON(w, http.StatusInternalServerError, nil, webhookStoreError(CodeWebhookReadFailed, "could not get webhook", err))
			return
		}
	}
	if err := rows.Err(); err != nil {
		hlog.FromRequest(r).Error().Err(err).
			Str("handler", "GetWebhook").
			Str("op", "iterate").
			Str("user_id", txtid).
			Msg("webhook database operation failed")
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, webhookStoreError(CodeWebhookReadFailed, "could not get webhook", err))
		return
	}
	eventarray := strings.Split(events, ",")
	response := map[string]interface{}{"webhook": webhook, "subscribe": eventarray}
	customhttp.RespondJSON(w, http.StatusOK, response, nil)
}

// SetWebhookHandler handles POST /webhook
type SetWebhookHandler struct{ ctx *WebhookHandlerContext }

func NewSetWebhookHandler(ctx *WebhookHandlerContext) *SetWebhookHandler {
	requirePublishUserInfo(ctx)
	return &SetWebhookHandler{ctx}
}

func (h *SetWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).
			Str("handler", "SetWebhook").
			Msg("webhook request rejected")
		customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}
	txtid := info.Get("Id")
	token := info.Get("Token")

	var t domain.WebhookConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		hlog.FromRequest(r).Warn().Err(err).
			Str("handler", "SetWebhook").
			Msg("webhook request rejected")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
		return
	}

	webhook := t.ResolveURL()
	var eventstring string
	if len(t.Events) > 0 {
		var validEvents []string
		for _, event := range t.Events {
			if !h.ctx.FindInSlice(h.ctx.SupportedEvents, event) {
				hlog.FromRequest(r).Warn().Str("Type", event).Msg("Event type discarded")
				continue
			}
			validEvents = append(validEvents, event)
		}
		eventstring = strings.Join(validEvents, ",")
		if eventstring == "," || eventstring == "" {
			eventstring = ""
		}
		_, err := h.ctx.DB.Exec("UPDATE users SET webhook=$1, events=$2 WHERE id=$3", webhook, eventstring, txtid)
		if err != nil {
			hlog.FromRequest(r).Error().Err(err).
				Str("handler", "SetWebhook").
				Str("op", "update").
				Str("user_id", txtid).
				Msg("webhook database operation failed")
			customhttp.RespondJSON(w, http.StatusInternalServerError, nil, webhookStoreError(CodeWebhookWriteFailed, "could not set webhook", err))
			return
		}
		if len(validEvents) > 0 {
			hlog.FromRequest(r).Info().Strs("events", validEvents).Str("user", txtid).Msg("Updated event subscriptions")
		}
	} else {
		_, err := h.ctx.DB.Exec("UPDATE users SET webhook=$1 WHERE id=$2", webhook, txtid)
		if err != nil {
			hlog.FromRequest(r).Error().Err(err).
				Str("handler", "SetWebhook").
				Str("op", "update").
				Str("user_id", txtid).
				Msg("webhook database operation failed")
			customhttp.RespondJSON(w, http.StatusInternalServerError, nil, webhookStoreError(CodeWebhookWriteFailed, "could not set webhook", err))
			return
		}
	}

	v := h.ctx.UpdateUserInfo(info, "Webhook", webhook)
	v = h.ctx.UpdateUserInfo(v, "Events", eventstring)
	h.ctx.publishValues(txtid, token, v)

	response := map[string]interface{}{"webhook": webhook}
	customhttp.RespondJSON(w, http.StatusOK, response, nil)
}

// UpdateWebhookHandler handles PUT /webhook
type UpdateWebhookHandler struct{ ctx *WebhookHandlerContext }

func NewUpdateWebhookHandler(ctx *WebhookHandlerContext) *UpdateWebhookHandler {
	requirePublishUserInfo(ctx)
	return &UpdateWebhookHandler{ctx}
}

func (h *UpdateWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).
			Str("handler", "UpdateWebhook").
			Msg("webhook request rejected")
		customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}
	txtid := info.Get("Id")
	token := info.Get("Token")

	var t domain.WebhookConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		hlog.FromRequest(r).Warn().Err(err).
			Str("handler", "UpdateWebhook").
			Msg("webhook request rejected")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
		return
	}

	webhook := t.ResolveURL()
	var eventstring string
	var validEvents []string
	for _, event := range t.Events {
		if !h.ctx.FindInSlice(h.ctx.SupportedEvents, event) {
			hlog.FromRequest(r).Warn().Str("Type", event).Msg("Event type discarded")
			continue
		}
		validEvents = append(validEvents, event)
	}
	eventstring = strings.Join(validEvents, ",")
	if eventstring == "," || eventstring == "" {
		eventstring = ""
	}

	if !t.Active {
		webhook = ""
		eventstring = ""
	}

	if len(t.Events) > 0 {
		_, err := h.ctx.DB.Exec("UPDATE users SET webhook=$1, events=$2 WHERE id=$3", webhook, eventstring, txtid)
		if err != nil {
			hlog.FromRequest(r).Error().Err(err).
				Str("handler", "UpdateWebhook").
				Str("op", "update").
				Str("user_id", txtid).
				Msg("webhook database operation failed")
			customhttp.RespondJSON(w, http.StatusInternalServerError, nil, webhookStoreError(CodeWebhookUpdateFailed, "could not update webhook", err))
			return
		}
		if len(validEvents) > 0 {
			hlog.FromRequest(r).Info().Strs("events", validEvents).Str("user", txtid).Msg("Updated event subscriptions")
		}
	} else {
		_, err := h.ctx.DB.Exec("UPDATE users SET webhook=$1 WHERE id=$2", webhook, txtid)
		if err != nil {
			hlog.FromRequest(r).Error().Err(err).
				Str("handler", "UpdateWebhook").
				Str("op", "update").
				Str("user_id", txtid).
				Msg("webhook database operation failed")
			customhttp.RespondJSON(w, http.StatusInternalServerError, nil, webhookStoreError(CodeWebhookUpdateFailed, "could not update webhook", err))
			return
		}
	}

	v := h.ctx.UpdateUserInfo(info, "Webhook", webhook)
	v = h.ctx.UpdateUserInfo(v, "Events", eventstring)
	h.ctx.publishValues(txtid, token, v)

	response := map[string]interface{}{"webhook": webhook, "events": validEvents, "active": t.Active}
	customhttp.RespondJSON(w, http.StatusOK, response, nil)
}

// DeleteWebhookHandler handles DELETE /webhook
type DeleteWebhookHandler struct{ ctx *WebhookHandlerContext }

func NewDeleteWebhookHandler(ctx *WebhookHandlerContext) *DeleteWebhookHandler {
	requirePublishUserInfo(ctx)
	return &DeleteWebhookHandler{ctx}
}

// requirePublishUserInfo panics if the context does not wire PublishUserInfo.
// Without it, publishValues nil-dereferences at request time — but that is
// the WRONG failure mode: a nil fallback that writes only one cache would
// silently reintroduce the F164 defect. Fail at construction, not at
// request time. Same pattern as bootstrap.NewRouter (router.go:75-83).
func requirePublishUserInfo(ctx *WebhookHandlerContext) {
	if ctx.PublishUserInfo == nil {
		panic("handlers: WebhookHandlerContext.PublishUserInfo is required (F164)")
	}
}

func (h *DeleteWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).
			Str("handler", "DeleteWebhook").
			Msg("webhook request rejected")
		customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}
	txtid := info.Get("Id")
	token := info.Get("Token")

	if _, err := h.ctx.DB.Exec("UPDATE users SET webhook='', events='' WHERE id=$1", txtid); err != nil {
		hlog.FromRequest(r).Error().Err(err).
			Str("handler", "DeleteWebhook").
			Str("op", "update").
			Str("user_id", txtid).
			Msg("webhook database operation failed")
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, webhookStoreError(CodeWebhookDeleteFailed, "could not delete webhook", err))
		return
	}

	v := h.ctx.UpdateUserInfo(info, "Webhook", "")
	v = h.ctx.UpdateUserInfo(v, "Events", "")
	h.ctx.publishValues(txtid, token, v)

	response := map[string]interface{}{"Details": "Webhook and events deleted successfully"}
	customhttp.RespondJSON(w, http.StatusOK, response, nil)
}
