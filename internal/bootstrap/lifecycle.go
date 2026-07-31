package bootstrap

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strings"
	"time"

	"wa-api/internal/infra/whatsapp/client"

	"github.com/go-resty/resty/v2"
	"github.com/jmoiron/sqlx"
	"github.com/mdp/qrterminal/v3"
	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	waLog "go.mau.fi/whatsmeow/util/log"
	"golang.org/x/net/proxy"

	"wa-api/internal/infra/storage")

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
// the whole process. Losing one delivery is preferable to taking wuzapi
// down for every connected user.
var safeGo = client.SafeGo

// ensureS3ClientForUser loads S3 config from DB and initializes client if not already present (lazy init for reconnect-after-restart)
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

func sendToUserWebHook(webhookurl string, path string, jsonData []byte, userID string, token string) {
	sendToUserWebHookWithHmac(webhookurl, path, jsonData, userID, token, nil)
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
			if arg != "" && Find(supportedEventTypes, arg) {
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

func checkIfSubscribedToEvent(subscribedEvents []string, eventType string, userId string) bool {
	if !Find(subscribedEvents, eventType) && !Find(subscribedEvents, "All") {
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
	defer rows.Close()
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
					if !Find(supportedEventTypes, arg) {
						log.Warn().Str("Type", arg).Msg("Event type discarded")
						continue
					}
					if !Find(subscribedEvents, arg) {
						subscribedEvents = append(subscribedEvents, arg)
					}
				}

			}
			eventstring := strings.Join(subscribedEvents, ",")
			log.Info().Str("events", eventstring).Str("jid", jid).Msg("Attempt to connect")
			kill := make(chan bool, 1)
			appCtx.KillChannel.Set(txtid, kill)
			go s.startClient(txtid, jid, token, kill)

			// Initialize S3 client if configured
			go func(userID string) {
				storage.GetS3Manager().EnsureClientFromDB(userID)
			}(txtid)
		}
	}
	err = rows.Err()
	if err != nil {
		log.Error().Err(err).Msg("DB Problem")
	}
}

var parseJID = client.ParseJID
var getPlatformTypeEnum = client.GetPlatformTypeEnum

func (s *server) startClient(userID string, textjid string, token string, kill chan bool) {
	log.Info().Str("userid", userID).Str("jid", textjid).Msg("Starting websocket connection to Whatsapp")

	// Connection retry constants
	const maxConnectionRetries = 3
	const connectionRetryBaseWait = 5 * time.Second

	var deviceStore *store.Device
	var err error

	// First handle the device store initialization
	if textjid != "" {
		jid, _ := parseJID(textjid)
		deviceStore, err = container.GetDevice(context.Background(), jid)
		if err != nil {
			log.Error().Err(err).Msg("Failed to get device")
			deviceStore = container.NewDevice()
		}
	} else {
		log.Warn().Msg("No jid found. Creating new device")
		deviceStore = container.NewDevice()
	}

	if deviceStore == nil {
		log.Warn().Msg("No store found. Creating new one")
		deviceStore = container.NewDevice()
	}

	clientLog := waLog.Stdout("Client", *waDebug, *colorOutput)

	// Create the client with initialized deviceStore
	var client *whatsmeow.Client
	if *waDebug != "" {
		client = whatsmeow.NewClient(deviceStore, clientLog)
	} else {
		client = whatsmeow.NewClient(deviceStore, nil)
	}

	// Now we can use the client with the manager
	clientManager.SetWhatsmeowClient(userID, client)

	store.DeviceProps.PlatformType = getPlatformTypeEnum(*platformType)
	store.DeviceProps.Os = osName

	mycli := MyClient{
		WAClient: client,
		EventHandlerID: 1,
		UserID: userID,
		Token: token,
		DB: s.DB,
		NotifyFn: s.SendNotification,
		mode:           s.Mode,
	}
	mycli.EventHandlerID = mycli.WAClient.AddEventHandler(mycli.myEventHandler)

	// Store the MyClient in clientManager
	clientManager.SetMyClient(userID, &mycli)

	// Webhook HTTP client for outgoing webhook deliveries.
	webhookClient := resty.New()
	webhookClient.SetRedirectPolicy(resty.FlexibleRedirectPolicy(15))
	if *waDebug == "DEBUG" {
		webhookClient.SetDebug(true)
	}
	webhookClient.SetTimeout(30 * time.Second)
	webhookClient.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	webhookClient.OnError(func(req *resty.Request, err error) {
		if v, ok := err.(*resty.ResponseError); ok {
			// v.Response contains the last response from the server
			// v.Err contains the original error
			log.Debug().Str("response", v.Response.String()).Msg("resty error")
			log.Error().Err(v.Err).Msg("resty error")
		}
	})

	var proxyURL string
	webhookUseProxy := appCtx.GlobalWebhookUseProxy
	err = s.DB.QueryRow(
		"SELECT proxy_url, COALESCE(webhook_use_proxy, true) FROM users WHERE id=$1",
		userID,
	).Scan(&proxyURL, &webhookUseProxy)
	if err != nil && err != sql.ErrNoRows {
		log.Error().Err(err).Str("user_id", userID).Msg("Failed to query proxy settings from database")
	}
	if err == nil && proxyURL != "" {
		parsed, perr := url.Parse(proxyURL)
		if perr != nil {
			log.Warn().Err(perr).Str("proxy", proxyURL).Msg("Invalid proxy URL, skipping proxy setup")
		} else {
			log.Info().Str("proxy", proxyURL).Bool("webhook_use_proxy", webhookUseProxy).Msg("Configuring proxy")

			if parsed.Scheme == "socks5" || parsed.Scheme == "socks5h" {
				dialer, derr := proxy.FromURL(parsed, nil)
				if derr != nil {
					log.Warn().Err(derr).Str("proxy", proxyURL).Msg("Failed to build SOCKS proxy dialer, skipping proxy setup")
				} else {
					client.SetSOCKSProxy(dialer, whatsmeow.SetProxyOptions{})
					log.Info().Msg("SOCKS proxy configured for WhatsApp connection")
				}
			} else {
				client.SetProxyAddress(parsed.String(), whatsmeow.SetProxyOptions{})
				log.Info().Msg("HTTP/HTTPS proxy configured for WhatsApp connection")
			}

			if webhookUseProxy {
				webhookClient.SetProxy(proxyURL)
				log.Info().Msg("Proxy configured for webhook delivery client")
			} else {
				log.Info().Msg("Webhook delivery client bypassing proxy")
			}
		}
	}
	clientManager.SetHTTPClient(userID, webhookClient)

	// Initialize S3 client if configured (needed when user reconnects after container restart - connectOnStartup only runs for connected=1)
	storage.GetS3Manager().EnsureClientFromDB(userID)

	if client.Store.ID == nil {
		// No ID stored, new login
		qrChan, err := client.GetQRChannel(context.Background())
		if err != nil {
			// This error means that we're already logged in, so ignore it.
			if !errors.Is(err, whatsmeow.ErrQRStoreContainsID) {
				log.Error().Err(err).Msg("Failed to get QR channel")
				return
			}
		} else {
			err = client.Connect() // Must connect to generate QR code
			if err != nil {
				log.Error().Err(err).Msg("Failed to connect client")
				return
			}

			myuserinfo, found := appCtx.UserInfoCache.Get(token)

			for evt := range qrChan {
				if evt.Event == "code" {
					// Display QR code in terminal (useful for testing/developing)
					// Skip in stdio mode to avoid breaking JSON-RPC
					if *logType != "json" && s.Mode != Stdio {
						qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
						fmt.Println("QR code:\n", evt.Code)
					}
					// Store encoded/embeded base64 QR on database for retrieval with the /qr endpoint
					image, _ := qrcode.Encode(evt.Code, qrcode.Medium, 256)
					base64qrcode := "data:image/png;base64," + base64.StdEncoding.EncodeToString(image)
					sqlStmt := `UPDATE users SET qrcode=$1 WHERE id=$2`
					_, err := s.DB.Exec(sqlStmt, base64qrcode, userID)
					if err != nil {
						log.Error().Err(err).Msg(sqlStmt)
					} else {
						if found {
							v := updateUserInfo(myuserinfo, "Qrcode", base64qrcode)
							appCtx.UserInfoCache.Set(token, v, cache.NoExpiration)
							log.Info().Str("qrcode", base64qrcode).Msg("update cache userinfo with qr code")
						}
					}

					//send QR code with webhook
					postmap := make(map[string]interface{})
					postmap["event"] = evt.Event
					postmap["qrCodeBase64"] = base64qrcode
					postmap["type"] = "QR"

					sendEventWithWebHook(&mycli, postmap, "")

				} else if evt.Event == "timeout" {
					// Clear QR code from DB on timeout
					// Send webhook notifying QR timeout before cleanup
					postmap := make(map[string]interface{})
					postmap["event"] = evt.Event
					postmap["type"] = "QRTimeout"
					sendEventWithWebHook(&mycli, postmap, "")

					sqlStmt := `UPDATE users SET qrcode='' WHERE id=$1`
					_, err := s.DB.Exec(sqlStmt, userID)
					if err != nil {
						log.Error().Err(err).Msg(sqlStmt)
					} else {
						if found {
							v := updateUserInfo(myuserinfo, "Qrcode", "")
							appCtx.UserInfoCache.Set(token, v, cache.NoExpiration)
						}
					}
					log.Warn().Msg("QR timeout killing channel")
					clientManager.DeleteWhatsmeowClient(userID)
					clientManager.DeleteMyClient(userID)
					clientManager.DeleteHTTPClient(userID)
					appCtx.KillChannel.Signal(userID)
				} else if evt.Event == "success" {
					log.Info().Msg("QR pairing ok!")
					// Clear QR code after pairing
					sqlStmt := `UPDATE users SET qrcode='', connected=1 WHERE id=$1`
					_, err := s.DB.Exec(sqlStmt, userID)
					if err != nil {
						log.Error().Err(err).Msg(sqlStmt)
					} else {
						if found {
							v := updateUserInfo(myuserinfo, "Qrcode", "")
							appCtx.UserInfoCache.Set(token, v, cache.NoExpiration)
						}
					}
				} else {
					log.Info().Str("event", evt.Event).Msg("Login event")
				}
			}
		}

	} else {
		// Already logged in, just connect
		log.Info().Msg("Already logged in, just connect")

		// Retry logic with linear backoff
		var lastErr error

		for attempt := 0; attempt < maxConnectionRetries; attempt++ {
			if attempt > 0 {
				waitTime := time.Duration(attempt) * connectionRetryBaseWait
				log.Warn().
					Int("attempt", attempt+1).
					Int("max_retries", maxConnectionRetries).
					Dur("wait_time", waitTime).
					Msg("Retrying connection after delay")
				time.Sleep(waitTime)
			}

			err = client.Connect()
			if err == nil {
				log.Info().
					Int("attempt", attempt+1).
					Msg("Successfully connected to WhatsApp")
				break
			}

			lastErr = err
			log.Warn().
				Err(err).
				Int("attempt", attempt+1).
				Int("max_retries", maxConnectionRetries).
				Msg("Failed to connect to WhatsApp")
		}

		if lastErr != nil {
			log.Error().
				Err(lastErr).
				Str("userid", userID).
				Int("attempts", maxConnectionRetries).
				Msg("Failed to connect to WhatsApp after all retry attempts")

			clientManager.DeleteWhatsmeowClient(userID)
			clientManager.DeleteMyClient(userID)
			clientManager.DeleteHTTPClient(userID)

			sqlStmt := `UPDATE users SET qrcode='', connected=0 WHERE id=$1`
			_, dbErr := s.DB.Exec(sqlStmt, userID)
			if dbErr != nil {
				log.Error().Err(dbErr).Msg("Failed to update user status after connection error")
			}

			// Use the existing mycli instance from outer scope
			postmap := make(map[string]interface{})
			postmap["event"] = "ConnectFailure"
			postmap["error"] = lastErr.Error()
			postmap["type"] = "ConnectFailure"
			postmap["attempts"] = maxConnectionRetries
			postmap["reason"] = "Failed to connect after retry attempts"
			sendEventWithWebHook(&mycli, postmap, "")

			return
		}
	}

	// Keep the session goroutine alive until a kill signal arrives. Block on the
	// channel (passed in directly, so this goroutine always owns its own channel
	// even if a reconnect replaces the map entry) instead of polling — this parks
	// the goroutine with zero CPU and no per-second mutex access.
	<-kill
	log.Info().Str("userid", userID).Msg("Received kill signal")
	client.Disconnect()
	clientManager.DeleteWhatsmeowClient(userID)
	clientManager.DeleteMyClient(userID)
	clientManager.DeleteHTTPClient(userID)
	if _, err := s.DB.Exec(`UPDATE users SET qrcode='', connected=0 WHERE id=$1`, userID); err != nil {
		log.Error().Err(err).Msg("failed to mark user disconnected on kill")
	}
	appCtx.KillChannel.Delete(userID, kill)
}

var fileToBase64 = client.FileToBase64

