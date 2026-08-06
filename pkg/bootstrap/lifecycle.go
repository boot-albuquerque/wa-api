package bootstrap

import (
	"encoding/base64"
	"fmt"
	"slices"
	"strings"

	wmhelpers "wa-api/pkg/infra/whatsmeow"

	"github.com/jmoiron/sqlx"
	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"
)

// db field declaration as *sqlx.DB
type MyClient struct {
	WAClient       *whatsmeow.Client
	EventHandlerID uint32
	UserID         string
	Token          string
	DB             *sqlx.DB
	NotifyFn       func(method string, params map[string]interface{})
	mode           ServerMode
}

// safeGo runs fn in a new goroutine with a defer recover so a panic inside
// fire-and-forget side-effects (webhook delivery, MQ push) cannot crash
// the whole process. Losing one delivery is preferable to taking wa-api
// down for every connected user.
var safeGo = wmhelpers.SafeGo

// Webhook functions extracted to lifecycle_webhook.go

func checkIfSubscribedToEvent(subscribedEvents []string, eventType string, userId string) bool {
	if !slices.Contains(subscribedEvents, eventType) && !slices.Contains(subscribedEvents, "All") {
		log.Warn().
			Str("type", eventType).
			Strs("subscribedEvents", subscribedEvents).
			Str("userID", userId).
			Msg("Skipping webhook. Not subscribed for this type")
		return false
	}
	return true
}

// Connects to Whatsapp Websocket on server startup if last state was connected
func (s *server) connectOnStartup() {
	rows, err := s.DB.Queryx("SELECT id,name,token,jid,webhook,events,proxy_url,CASE WHEN s3_enabled THEN 'true' ELSE 'false' END AS s3_enabled,media_delivery,COALESCE(history, 0) as history,hmac_key FROM users WHERE connected=1")
	if err != nil {
		log.Error().Err(err).Msg("DB Problem")
		return
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			log.Warn().Err(cerr).Msg("Failed to close rows")
		}
	}()
	for rows.Next() {
		txtid := ""
		token := ""
		jid := ""
		name := ""
		webhook := ""
		events := ""
		proxy_url := ""
		s3_enabled := ""
		media_delivery := ""
		var history int
		var hmac_key []byte
		err = rows.Scan(&txtid, &name, &token, &jid, &webhook, &events, &proxy_url, &s3_enabled, &media_delivery, &history, &hmac_key)
		if err != nil {
			log.Error().Err(err).Msg("DB Problem")
			return
		} else {
			hmacKeyEncrypted := ""
			if len(hmac_key) > 0 {
				hmacKeyEncrypted = base64.StdEncoding.EncodeToString(hmac_key)
			}

			log.Info().Str("token", token).Msg("Connect to Whatsapp on startup")
			v := Values{M: map[string]string{
				"Id":               txtid,
				"Name":             name,
				"Jid":              jid,
				"Webhook":          webhook,
				"Token":            token,
				"Proxy":            proxy_url,
				"Events":           events,
				"S3Enabled":        s3_enabled,
				"MediaDelivery":    media_delivery,
				"History":          fmt.Sprintf("%d", history),
				"HmacKeyEncrypted": hmacKeyEncrypted,
			}}
			appCtx.UserInfoCache.Set(token, v, cache.NoExpiration)
			// Gets and set subscription to webhook events
			eventarray := strings.Split(events, ",")

			var subscribedEvents []string
			if len(eventarray) == 1 && eventarray[0] == "" {
				subscribedEvents = []string{}
			} else {
				for _, arg := range eventarray {
					if !slices.Contains(supportedEventTypes, arg) {
						log.Warn().Str("Type", arg).Msg("Event type discarded")
						continue
					}
					if !slices.Contains(subscribedEvents, arg) {
						subscribedEvents = append(subscribedEvents, arg)
					}
				}

			}
			eventstring := strings.Join(subscribedEvents, ",")
			log.Info().Str("events", eventstring).Str("jid", jid).Msg("Attempt to connect")
			// Goroutine porque startSession bloqueia enquanto o pareamento
			// estiver ativo — na subida, uma sessao em QR nao pode segurar
			// as demais linhas do SELECT.
			go s.startSession(txtid, token)
		}
	}
	err = rows.Err()
	if err != nil {
		log.Error().Err(err).Msg("DB Problem")
	}
}
