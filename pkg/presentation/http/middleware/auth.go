// Package middleware provides HTTP middleware handlers extracted from
// root wa-api/auth.go during Phase 13d of the Clean Architecture refactor.
// Functions accept all dependencies as parameters — no root imports.
package middleware

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
	appport "wa-api/pkg/application/contracts"

	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"
)

// Values is a string map for user attributes carried through request context.
type Values struct {
	M map[string]string
}

// Get returns the value for key or empty string.
func (v Values) Get(key string) string {
	if v.M == nil {
		return ""
	}
	return v.M[key]
}

// NewValues creates a Values from a map.
func NewValues(m map[string]string) Values { return Values{M: m} }

// RespondJSON writes a legacy-compatible JSON envelope.
func RespondJSON(w http.ResponseWriter, status int, data interface{}) {
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
		panic("RespondJSON: " + err.Error())
	}
}

// AuthAdmin returns middleware that validates the Authorization header.
func AuthAdmin(adminToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("Authorization")
			tokenHash := sha256.Sum256([]byte(token))
			adminHash := sha256.Sum256([]byte(adminToken))
			if subtle.ConstantTimeCompare(tokenHash[:], adminHash[:]) != 1 {
				RespondJSON(w, http.StatusUnauthorized, errors.New("unauthorized"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AuthAlice returns middleware that looks up a user by token.
func AuthAlice(db *sql.DB, userCache *cache.Cache) func(http.Handler) http.Handler {
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
					RespondJSON(w, http.StatusInternalServerError, err)
					return
				}
				defer func() {
					if cerr := rows.Close(); cerr != nil {
						log.Warn().Err(cerr).Msg("error closing rows")
					}
				}()
				var history sql.NullInt64
				var s3Enabled, mediaDelivery string
				for rows.Next() {
					err = rows.Scan(&txtid, &name, &webhook, &jid, &events, &proxyURL, &qrcode, &history, &hasHmac, &s3Enabled, &mediaDelivery)
					if err != nil {
						RespondJSON(w, http.StatusInternalServerError, err)
						return
					}
					historyStr := "0"
					if history.Valid {
						historyStr = fmt.Sprintf("%d", history.Int64)
					}
					v := Values{M: map[string]string{
						"Id": txtid, "Name": name, "Jid": jid, "Webhook": webhook,
						"Token": token, "Proxy": proxyURL, "Events": events,
						"Qrcode": qrcode, "History": historyStr,
						"HasHmac":   strconv.FormatBool(hasHmac),
						"S3Enabled": s3Enabled, "MediaDelivery": mediaDelivery,
					}}
					userCache.Set(token, v, cache.NoExpiration)
					ctx = context.WithValue(r.Context(), appport.UserInfoKey, v)
				}
			} else {
				v := myuserinfo.(Values)
				ctx = context.WithValue(r.Context(), appport.UserInfoKey, v)
				txtid = v.Get("Id")
			}

			if txtid == "" {
				RespondJSON(w, http.StatusUnauthorized, errors.New("unauthorized"))
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ResolveConnectEvents decides subscription string on reconnect.
func ResolveConnectEvents(findInSlice func([]string, string) bool, supported []string, subscribe []string, existing string) (string, bool) {
	if len(subscribe) < 1 {
		return existing, false
	}
	var subscribed []string
	for _, arg := range subscribe {
		if !findInSlice(supported, arg) {
			log.Warn().Str("Type", arg).Msg("Event type discarded")
			continue
		}
		if !findInSlice(subscribed, arg) {
			subscribed = append(subscribed, arg)
		}
	}
	resolved := strings.Join(subscribed, ",")
	return resolved, resolved != existing
}
