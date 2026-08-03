package bootstrap

import (
	"encoding/base64"
	"encoding/json"
	"slices"
	"strings"

	"github.com/rs/zerolog/log"

	"wa-api/pkg/infra/storage"
)

func ensureS3ClientForUser(userID string) {
	storage.GetS3Manager().EnsureClientFromDB(userID)
}

func sendToGlobalWebHook(jsonData []byte, token string, userID string) {
	jsonDataStr := string(jsonData)

	instance_name := ""
	userinfo, found := appCtx.UserInfoCache.Get(token)
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
		callHookWithHmac(appCtx.GlobalWebhook, globalData, userID, appCtx.GlobalHMACKeyEncrypted)
	}
}

func sendToUserWebHookWithHmac(webhookurl string, path string, jsonData []byte, userID string, token string, encryptedHmacKey []byte) {

	instance_name := ""
	userinfo, found := appCtx.UserInfoCache.Get(token)
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
			safeGo("callHookWithHmac", func() { callHookWithHmac(webhookurl, data, userID, encryptedHmacKey) })
		} else {
			if err := callHookFileWithHmac(webhookurl, data, userID, path, encryptedHmacKey); err != nil {
				log.Error().Err(err).Msg("Error calling hook file")
			}
		}
	} else {
		log.Warn().Str("userid", userID).Msg("No webhook set for user")
	}
}

func updateAndGetUserSubscriptions(mycli *MyClient) ([]string, error) {
	// Get updated events from cache/database
	currentEvents := ""
	userinfo2, found2 := appCtx.UserInfoCache.Get(mycli.Token)
	if found2 {
		currentEvents = userinfo2.(Values).Get("Events")
	} else {
		// If not in cache, get from database
		if err := mycli.DB.Get(&currentEvents, "SELECT events FROM users WHERE id=$1", mycli.UserID); err != nil {
			log.Warn().Err(err).Str("userID", mycli.UserID).Msg("Could not get events from DB")
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

func getUserWebhookUrl(token string) string {
	webhookurl := ""
	myuserinfo, found := appCtx.UserInfoCache.Get(token)
	if !found {
		log.Warn().Str("token", token).Msg("Could not call webhook as there is no user for this token")
	} else {
		webhookurl = myuserinfo.(Values).Get("Webhook")
	}
	return webhookurl
}

func sendEventWithWebHook(mycli *MyClient, postmap map[string]interface{}, path string) {
	webhookurl := getUserWebhookUrl(mycli.Token)

	// Get updated events from cache/database
	subscribedEvents, err := updateAndGetUserSubscriptions(mycli)
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
		Str("userID", mycli.UserID).
		Str("eventType", eventType).
		Strs("subscribedEvents", subscribedEvents).
		Msg("Checking event subscription")

	// Check if the current event is in the subscriptions
	checkIfSubscribedInEvent := checkIfSubscribedToEvent(subscribedEvents, postmap["type"].(string), mycli.UserID)
	if !checkIfSubscribedInEvent {
		return
	}

	// Real-time push to any live /session/ws connection — the same event,
	// same subscription gate, as a fourth delivery channel alongside the
	// per-user webhook, global webhook, and RabbitMQ below. Best-effort and
	// fully additive: a stuck/absent WS client never blocks webhook
	// delivery (BroadcastToUser is itself non-blocking per-connection, see
	// wsBroadcastTimeout), and REST polling of /session/status and
	// /session/qr is untouched either way.
	safeGo("sendToWS", func() { clientManager.BroadcastToUser(mycli.UserID, postmap) })

	// In stdio mode, send as JSON-RPC notification instead of HTTP webhook
	if mycli.mode == Stdio {
		if mycli.NotifyFn != nil {
			mycli.NotifyFn(eventType, postmap)
		}
		return
	}

	// Prepare webhook data
	jsonData, err := json.Marshal(postmap)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal postmap to JSON")
		return
	}

	// Get HMAC key for this user
	var encryptedHmacKey []byte
	if userinfo, found := appCtx.UserInfoCache.Get(mycli.Token); found {
		encryptedB64 := userinfo.(Values).Get("HmacKeyEncrypted")
		if encryptedB64 != "" {
			var err error
			encryptedHmacKey, err = base64.StdEncoding.DecodeString(encryptedB64)
			if err != nil {
				log.Error().Err(err).Msg("Failed to decode HMAC key from cache")
			}
		}
	}

	sendToUserWebHookWithHmac(webhookurl, path, jsonData, mycli.UserID, mycli.Token, encryptedHmacKey)

	// Get global webhook if configured
	safeGo("sendToGlobalWebHook", func() { sendToGlobalWebHook(jsonData, mycli.Token, mycli.UserID) })

	safeGo("sendToGlobalRabbit", func() { sendToGlobalRabbit(jsonData, mycli.Token, mycli.UserID) })
}
