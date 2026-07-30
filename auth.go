package wuzapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"
)

// Values is a simple string map used to carry user attributes through the
// request context. Keys are set during authentication and consumed by
// downstream handlers.
type Values struct {
	m map[string]string
}

// Get returns the value for a key or the empty string when missing.
func (v Values) Get(key string) string { return v.m[key] }

// respondJSON writes a legacy-compatible JSON envelope.
// Signature matches the old s.Respond() from handlers.go.
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	dataenvelope := map[string]interface{}{"code": status}
	if err, ok := data.(error); ok {
		dataenvelope["error"] = err.Error()
		dataenvelope["success"] = false
	} else if s, ok := data.(string); ok {
		var mydata map[string]interface{}
		if err := json.Unmarshal([]byte(s), &mydata); err == nil {
			dataenvelope["data"] = mydata
		} else {
			var mySlice []interface{}
			if err := json.Unmarshal([]byte(s), &mySlice); err == nil {
				dataenvelope["data"] = mySlice
			} else {
				log.Error().Err(err).Msg("error unmarshalling JSON")
			}
		}
		dataenvelope["success"] = true
	} else {
		dataenvelope["data"] = data
		dataenvelope["success"] = true
	}

	if err := json.NewEncoder(w).Encode(dataenvelope); err != nil {
		panic("respondJSON: " + err.Error())
	}
}

// authAdmin returns middleware that validates the Authorization header against
// the admin token via constant-time comparison. Unauthorized requests get a
// 401 JSON response.
func authAdmin(adminToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("Authorization")
			tokenHash := sha256.Sum256([]byte(token))
			adminHash := sha256.Sum256([]byte(adminToken))
			if subtle.ConstantTimeCompare(tokenHash[:], adminHash[:]) != 1 {
				respondJSON(w, http.StatusUnauthorized, errors.New("unauthorized"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// authAlice returns middleware that looks up a user by token (header or query
// param), caches the attributes in userCache, and injects a Values instance
// into the request context under the "userinfo" key. Unauthorized requests get
// a 401 JSON response.
func authAlice(db *sql.DB, userCache *cache.Cache) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var ctx context.Context
			txtid := ""
			name := ""
			webhook := ""
			jid := ""
			events := ""
			proxyURL := ""
			qrcode := ""
			var hasHmac bool

			token := r.Header.Get("token")
			if token == "" {
				token = strings.Join(r.URL.Query()["token"], "")
			}

			myuserinfo, found := userCache.Get(token)
			if !found {
				log.Info().Msg("Looking for user information in DB")
				rows, err := db.Query(
					"SELECT id,name,webhook,jid,events,proxy_url,qrcode,history,"+
						"hmac_key IS NOT NULL AND length(hmac_key) > 0,"+
						"CASE WHEN s3_enabled THEN 'true' ELSE 'false' END,"+
						"COALESCE(media_delivery, 'base64') "+
						"FROM users WHERE token=$1 LIMIT 1",
					token,
				)
				if err != nil {
					respondJSON(w, http.StatusInternalServerError, err)
					return
				}
				defer rows.Close()
				var history sql.NullInt64
				var s3Enabled, mediaDelivery string
				for rows.Next() {
					err = rows.Scan(&txtid, &name, &webhook, &jid, &events, &proxyURL, &qrcode, &history, &hasHmac, &s3Enabled, &mediaDelivery)
					if err != nil {
						respondJSON(w, http.StatusInternalServerError, err)
						return
					}
					historyStr := "0"
					if history.Valid {
						historyStr = fmt.Sprintf("%d", history.Int64)
					}
					log.Debug().Str("userId", txtid).Bool("historyValid", history.Valid).Int64("historyValue", history.Int64).Str("historyStr", historyStr).Msg("User authentication - history debug")

					v := Values{m: map[string]string{
						"Id":            txtid,
						"Name":          name,
						"Jid":           jid,
						"Webhook":       webhook,
						"Token":         token,
						"Proxy":         proxyURL,
						"Events":        events,
						"Qrcode":        qrcode,
						"History":       historyStr,
						"HasHmac":       strconv.FormatBool(hasHmac),
						"S3Enabled":     s3Enabled,
						"MediaDelivery": mediaDelivery,
					}}

					userCache.Set(token, v, cache.NoExpiration)
					log.Info().Str("name", name).Msg("User info name from DB")
					ctx = context.WithValue(r.Context(), "userinfo", v)
				}
			} else {
				v := myuserinfo.(Values)
				ctx = context.WithValue(r.Context(), "userinfo", v)
				log.Info().Str("name", v.Get("Name")).Msg("User info name from Cache")
				txtid = v.Get("Id")
			}

			if txtid == "" {
				respondJSON(w, http.StatusUnauthorized, errors.New("unauthorized"))
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// resolveConnectEvents decides which event-subscription string to persist when a
// client (re)connects. With no subscribe list the existing subscriptions are
// preserved instead of being overwritten with an empty value (issue #305);
// changed reports whether the stored value needs updating.
func resolveConnectEvents(subscribe []string, existing string) (eventstring string, changed bool) {
	if len(subscribe) < 1 {
		return existing, false
	}
	var subscribedEvents []string
	for _, arg := range subscribe {
		if !Find(supportedEventTypes, arg) {
			log.Warn().Str("Type", arg).Msg("Event type discarded")
			continue
		}
		if !Find(subscribedEvents, arg) {
			subscribedEvents = append(subscribedEvents, arg)
		}
	}
	resolved := strings.Join(subscribedEvents, ",")
	return resolved, resolved != existing
}
