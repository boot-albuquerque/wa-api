package bootstrap

import (
	"encoding/base64"
	"encoding/json"
	"slices"
	"strings"

	"github.com/rs/zerolog/log"

	dbpkg "wa-api/pkg/infra/db"
	"wa-api/pkg/infra/storage"
)

func ensureS3ClientForUser(userID string) {
	storage.GetS3Manager().EnsureClientFromDB(userID)
}

func sendToGlobalWebHook(jsonData []byte, userID string) {
	jsonDataStr := string(jsonData)

	instance_name := ""
	userinfo, found := appCtx.UserInfoCache.Get(userID)
	if found {
		instance_name = userinfo.(Values).Get("Name")
	}

	if appCtx.GlobalWebhook != "" {
		log.Info().Str("url", appCtx.GlobalWebhook).Msg("Calling global webhook")
		// Add extra information for the global webhook
		globalData := map[string]string{
			"jsonData":     jsonDataStr,
			"userID":       userID,
			"instanceName": instance_name,
		}
		callHookWithHmac(appCtx.GlobalWebhook, globalData, userID, appCtx.GlobalHMACKeyEncrypted, dbpkg.HMACScopeGlobal)
	}
}

func sendToUserWebHookWithHmac(webhookurl string, path string, jsonData []byte, userID string, encryptedHmacKey []byte) {

	instance_name := ""
	userinfo, found := appCtx.UserInfoCache.Get(userID)
	if found {
		instance_name = userinfo.(Values).Get("Name")
	}
	data := map[string]string{
		"jsonData":     string(jsonData),
		"userID":       userID,
		"instanceName": instance_name,
	}

	log.Debug().Interface("webhookData", data).Msg("Data being sent to webhook")

	if webhookurl != "" {
		log.Info().Str("url", webhookurl).Msg("Calling user webhook")

		if path == "" {
			dispatchGo("callHookWithHmac", len(jsonData), func() { callHookWithHmac(webhookurl, data, userID, encryptedHmacKey, dbpkg.HMACScopeUser) })
		} else {
			if err := callHookFileWithHmac(webhookurl, data, userID, path, encryptedHmacKey); err != nil {
				log.Error().Err(err).Msg("Error calling hook file")
			}
		}
	} else {
		log.Warn().Str("userid", userID).Msg("No webhook set for user")
	}
}

func updateAndGetUserSubscriptions(evh *UserEventHandler) ([]string, error) {
	// Get updated events from cache/database
	currentEvents := ""
	userinfo2, found2 := appCtx.UserInfoCache.Get(evh.UserID)
	if found2 {
		currentEvents = userinfo2.(Values).Get("Events")
	} else {
		// If not in cache, get from database
		if err := evh.DB.Get(&currentEvents, "SELECT events FROM users WHERE id=$1", evh.UserID); err != nil {
			log.Warn().Err(err).Str("userID", evh.UserID).Msg("Could not get events from DB")
			return nil, err // Propagate the error
		}
	}

	// Update client subscriptions if changed
	eventarray := strings.Split(currentEvents, ",")
	var subscribedEvents []string
	if len(eventarray) == 1 && eventarray[0] == "" {
		subscribedEvents = []string{}
	} else {
		for _, arg := range eventarray {
			arg = strings.TrimSpace(arg)
			if arg != "" && slices.Contains(supportedEventTypes, arg) {
				subscribedEvents = append(subscribedEvents, arg)
			}
		}
	}

	return subscribedEvents, nil
}

func getUserWebhookUrl(userID string) string {
	webhookurl := ""
	myuserinfo, found := appCtx.UserInfoCache.Get(userID)
	if !found {
		log.Warn().Str("userid", userID).Msg("Could not call webhook as there is no cached info for this user")
	} else {
		webhookurl = myuserinfo.(Values).Get("Webhook")
	}
	return webhookurl
}

func sendEventWithWebHook(evh *UserEventHandler, postmap map[string]interface{}, path string) {
	webhookurl := getUserWebhookUrl(evh.UserID)

	// Get updated events from cache/database
	subscribedEvents, err := updateAndGetUserSubscriptions(evh)
	if err != nil {
		return
	}

	eventType, ok := postmap["type"].(string)
	if !ok {
		log.Error().Msg("Event type is not a string in postmap")
		return
	}

	// Log subscription details for debugging
	log.Debug().
		Str("userID", evh.UserID).
		Str("eventType", eventType).
		Strs("subscribedEvents", subscribedEvents).
		Msg("Checking event subscription")

	// Check if the current event is in the subscriptions
	checkIfSubscribedInEvent := checkIfSubscribedToEvent(subscribedEvents, postmap["type"].(string), evh.UserID)
	if !checkIfSubscribedInEvent {
		return
	}

	// O Marshal acontece AQUI, antes do primeiro despacho, e não mais logo
	// antes do webhook: a fila de despacho se limita por BYTES (ver F86), e
	// `len(jsonData)` é a única medida honesta do payload que cada closure
	// mantém vivo. Sem ele, o despacho do WS — que é justamente quem carrega
	// os lotes de HistorySync, os maiores medidos — entraria na contabilidade
	// como tamanho estimado, furando a proteção na rajada que ela existe para
	// conter.
	//
	// O custo é um Marshal a mais no modo Stdio, que antes retornava sem
	// serializar. É volume baixo nesse modo, e paga a contabilidade exata nos
	// quatro despachos.
	jsonData, err := json.Marshal(postmap)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal postmap to JSON")
		return
	}
	tamanhoPayload := len(jsonData)

	// Real-time push to any live /session/ws connection — the same event,
	// same subscription gate, as a fourth delivery channel alongside the
	// per-user webhook, global webhook, and RabbitMQ below. Best-effort and
	// fully additive: a stuck/absent WS client never blocks webhook
	// delivery (BroadcastToUser is itself non-blocking per-connection, see
	// wsBroadcastTimeout), and REST polling of /session/status and
	// /session/qr is untouched either way.
	dispatchGo("sendToWS", tamanhoPayload, func() { clientManager.BroadcastToUser(evh.UserID, postmap) })

	// In stdio mode, send as JSON-RPC notification instead of HTTP webhook
	if evh.mode == Stdio {
		if evh.NotifyFn != nil {
			evh.NotifyFn(eventType, postmap)
		}
		return
	}

	// Get HMAC key for this user
	var encryptedHmacKey []byte
	if userinfo, found := appCtx.UserInfoCache.Get(evh.UserID); found {
		encryptedB64 := userinfo.(Values).Get(userInfoHmacKeyField)
		if encryptedB64 != "" {
			var err error
			encryptedHmacKey, err = base64.StdEncoding.DecodeString(encryptedB64)
			if err != nil {
				log.Error().Err(err).Msg("Failed to decode HMAC key from cache")
			}
		}
	}

	sendToUserWebHookWithHmac(webhookurl, path, jsonData, evh.UserID, encryptedHmacKey)

	// Get global webhook if configured
	dispatchGo("sendToGlobalWebHook", tamanhoPayload, func() { sendToGlobalWebHook(jsonData, evh.UserID) })

	dispatchGo("sendToGlobalRabbit", tamanhoPayload, func() { sendToGlobalRabbit(jsonData, evh.UserID) })
}
