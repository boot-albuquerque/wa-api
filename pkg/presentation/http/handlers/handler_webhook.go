package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	appport "wa-api/pkg/application/contracts"

	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"
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
	RespondJSON     func(w http.ResponseWriter, status int, data interface{})
}

// GetWebhookHandler handles GET /webhook
type GetWebhookHandler struct{ ctx *WebhookHandlerContext }

func NewGetWebhookHandler(ctx *WebhookHandlerContext) *GetWebhookHandler {
	return &GetWebhookHandler{ctx}
}

func (h *GetWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		h.ctx.RespondJSON(w, http.StatusUnauthorized, errUnauthorized)
		return
	}
	txtid := info.Get("Id")
	rows, err := h.ctx.DB.Query("SELECT webhook,events FROM users WHERE id=$1 LIMIT 1", txtid)
	if err != nil {
		h.ctx.RespondJSON(w, http.StatusInternalServerError, fmt.Errorf("could not get webhook: %v", err))
		return
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("failed to close rows")
		}
	}()
	var webhook, events string
	for rows.Next() {
		if err := rows.Scan(&webhook, &events); err != nil {
			h.ctx.RespondJSON(w, http.StatusInternalServerError, fmt.Errorf("could not get webhook: %s", err))
			return
		}
	}
	if err := rows.Err(); err != nil {
		h.ctx.RespondJSON(w, http.StatusInternalServerError, fmt.Errorf("could not get webhook: %s", err))
		return
	}
	eventarray := strings.Split(events, ",")
	response := map[string]interface{}{"webhook": webhook, "subscribe": eventarray}
	respJSON, _ := json.Marshal(response)
	h.ctx.RespondJSON(w, http.StatusOK, string(respJSON))
}

// SetWebhookHandler handles POST /webhook
type SetWebhookHandler struct{ ctx *WebhookHandlerContext }

func NewSetWebhookHandler(ctx *WebhookHandlerContext) *SetWebhookHandler {
	return &SetWebhookHandler{ctx}
}

func (h *SetWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		h.ctx.RespondJSON(w, http.StatusUnauthorized, errUnauthorized)
		return
	}
	txtid := info.Get("Id")
	token := info.Get("Token")

	var t struct {
		WebhookURL string   `json:"webhookurl"`
		Events     []string `json:"events,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		h.ctx.RespondJSON(w, http.StatusBadRequest, fmt.Errorf("could not decode payload"))
		return
	}

	webhook := t.WebhookURL
	var eventstring string
	if len(t.Events) > 0 {
		var validEvents []string
		for _, event := range t.Events {
			if !h.ctx.FindInSlice(h.ctx.SupportedEvents, event) {
				log.Warn().Str("Type", event).Msg("Event type discarded")
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
			h.ctx.RespondJSON(w, http.StatusInternalServerError, fmt.Errorf("could not set webhook: %v", err))
			return
		}
		if len(validEvents) > 0 {
			log.Info().Strs("events", validEvents).Str("user", txtid).Msg("Updated event subscriptions")
		}
	} else {
		_, err := h.ctx.DB.Exec("UPDATE users SET webhook=$1 WHERE id=$2", webhook, txtid)
		if err != nil {
			h.ctx.RespondJSON(w, http.StatusInternalServerError, fmt.Errorf("could not set webhook: %v", err))
			return
		}
	}

	v := h.ctx.UpdateUserInfo(info, "Webhook", webhook)
	v = h.ctx.UpdateUserInfo(v, "Events", eventstring)
	h.ctx.UserCache.Set(token, v, cache.NoExpiration)

	response := map[string]interface{}{"webhook": webhook}
	respJSON, _ := json.Marshal(response)
	h.ctx.RespondJSON(w, http.StatusOK, string(respJSON))
}

// UpdateWebhookHandler handles PUT /webhook
type UpdateWebhookHandler struct{ ctx *WebhookHandlerContext }

func NewUpdateWebhookHandler(ctx *WebhookHandlerContext) *UpdateWebhookHandler {
	return &UpdateWebhookHandler{ctx}
}

func (h *UpdateWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		h.ctx.RespondJSON(w, http.StatusUnauthorized, errUnauthorized)
		return
	}
	txtid := info.Get("Id")
	token := info.Get("Token")

	var t struct {
		WebhookURL string   `json:"webhook"`
		Events     []string `json:"events,omitempty"`
		Active     bool     `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		h.ctx.RespondJSON(w, http.StatusBadRequest, fmt.Errorf("could not decode payload"))
		return
	}

	webhook := t.WebhookURL
	var eventstring string
	var validEvents []string
	for _, event := range t.Events {
		if !h.ctx.FindInSlice(h.ctx.SupportedEvents, event) {
			log.Warn().Str("Type", event).Msg("Event type discarded")
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
			h.ctx.RespondJSON(w, http.StatusInternalServerError, fmt.Errorf("could not update webhook: %v", err))
			return
		}
		if len(validEvents) > 0 {
			log.Info().Strs("events", validEvents).Str("user", txtid).Msg("Updated event subscriptions")
		}
	} else {
		_, err := h.ctx.DB.Exec("UPDATE users SET webhook=$1 WHERE id=$2", webhook, txtid)
		if err != nil {
			h.ctx.RespondJSON(w, http.StatusInternalServerError, fmt.Errorf("could not update webhook: %v", err))
			return
		}
	}

	v := h.ctx.UpdateUserInfo(info, "Webhook", webhook)
	v = h.ctx.UpdateUserInfo(v, "Events", eventstring)
	h.ctx.UserCache.Set(token, v, cache.NoExpiration)

	response := map[string]interface{}{"webhook": webhook, "events": validEvents, "active": t.Active}
	respJSON, _ := json.Marshal(response)
	h.ctx.RespondJSON(w, http.StatusOK, string(respJSON))
}

// DeleteWebhookHandler handles DELETE /webhook
type DeleteWebhookHandler struct{ ctx *WebhookHandlerContext }

func NewDeleteWebhookHandler(ctx *WebhookHandlerContext) *DeleteWebhookHandler {
	return &DeleteWebhookHandler{ctx}
}

func (h *DeleteWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		h.ctx.RespondJSON(w, http.StatusUnauthorized, errUnauthorized)
		return
	}
	txtid := info.Get("Id")
	token := info.Get("Token")

	if _, err := h.ctx.DB.Exec("UPDATE users SET webhook='', events='' WHERE id=$1", txtid); err != nil {
		h.ctx.RespondJSON(w, http.StatusInternalServerError, fmt.Errorf("could not delete webhook: %v", err))
		return
	}

	v := h.ctx.UpdateUserInfo(info, "Webhook", "")
	v = h.ctx.UpdateUserInfo(v, "Events", "")
	h.ctx.UserCache.Set(token, v, cache.NoExpiration)

	response := map[string]interface{}{"Details": "Webhook and events deleted successfully"}
	respJSON, _ := json.Marshal(response)
	h.ctx.RespondJSON(w, http.StatusOK, string(respJSON))
}
