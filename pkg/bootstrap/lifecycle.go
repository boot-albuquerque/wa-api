package bootstrap

import (
	"encoding/base64"
	"fmt"
	"slices"
	"strings"

	"wa-api/pkg/infra/wa-noise/runtime/safego"

	wanoise "wa-api/internal/wa-noise"

	"github.com/jmoiron/sqlx"
	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"
)

// db field declaration as *sqlx.DB
type UserEventHandler struct {
	WAClient       *wanoise.Client
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
var safeGo = safego.SafeGo

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
	// Mesma lista de colunas de ensureUserInfoCached: os dois preenchem a
	// MESMA estrutura, e divergir faria a entrada nascer incompleta
	// dependendo do caminho por onde o usuário passou (F70).
	rows, err := s.DB.Queryx("SELECT " + userInfoColumns + " FROM users WHERE connected=1")
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
			// Posse ANTES de qualquer outra coisa (ADR-0005 D2).
			//
			// O filtro fica aqui, e nao junto do Connect, porque o bloco
			// abaixo popula o UserInfoCache: filtrar so' na hora de conectar
			// deixaria este processo com dados de sessoes que nao sao dele em
			// memoria, e qualquer caminho que consulte o cache passaria a
			// responder por sessao alheia.
			//
			// Em `single` claimSessionOwnership devolve true sempre — nao ha
			// com quem competir.
			if !claimSessionOwnership(s.Leases, txtid) {
				continue
			}

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
