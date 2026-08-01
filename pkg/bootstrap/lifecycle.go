package bootstrap

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	wmhelpers "wa-api/pkg/infra/whatsmeow"

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

	"wa-api/pkg/infra/storage"
)

// webhookTLSSkipVerify reports whether outgoing webhook deliveries should skip
// TLS certificate verification. Defaults to false (verification enabled) and is
// only disabled when WA_API_WEBHOOK_TLS_SKIP_VERIFY is explicitly set to
// "true"/"1", which is an insecure, deliberately registered choice (it allows
// delivery to consumers with self-signed certificates while exposing webhook
// traffic to man-in-the-middle attacks). Evaluated once, warning logged once.
var webhookTLSSkipVerify = sync.OnceValue(func() bool {
	v := strings.ToLower(os.Getenv("WA_API_WEBHOOK_TLS_SKIP_VERIFY"))
	skip := v == "true" || v == "1"
	if skip {
		log.Warn().
			Str("env", "WA_API_WEBHOOK_TLS_SKIP_VERIFY").
			Msg("INSECURE: webhook TLS certificate verification is DISABLED by explicit configuration. " +
				"Webhook deliveries are vulnerable to man-in-the-middle attacks. Unset this variable in production.")
	}
	return skip
})

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

var parseJID = wmhelpers.ParseJID
var getPlatformTypeEnum = wmhelpers.GetPlatformTypeEnum

func (s *server) startClient(userID string, textjid string, token string, kill chan bool) {
	log.Info().Str("userid", userID).Str("jid", textjid).Msg("[startClient] ENTRY - Starting WhatsApp WebSocket")
	defer func() {
		if r := recover(); r != nil {
			log.Error().Str("userid", userID).Interface("panic", r).Msg("[startClient] PANIC caught")
		}
	}()

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
		WAClient:       client,
		EventHandlerID: 1,
		UserID:         userID,
		Token:          token,
		DB:             s.DB,
		NotifyFn:       s.SendNotification,
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
	webhookClient.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: webhookTLSSkipVerify()})
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
				if err := client.SetProxyAddress(parsed.String(), whatsmeow.SetProxyOptions{}); err != nil {
					log.Warn().Err(err).Str("proxy", proxyURL).Msg("Failed to set HTTP/HTTPS proxy address, skipping proxy setup")
				} else {
					log.Info().Msg("HTTP/HTTPS proxy configured for WhatsApp connection")
				}
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
				switch evt.Event {
				case "code":
					// Display QR code in terminal (useful for testing/developing)
					// Skip in stdio mode to avoid breaking JSON-RPC
					if *logType != "json" && s.Mode != Stdio {
						qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout) //nolint:forbidigo // renderização visual do QR no terminal; não deixa o payload em texto pesquisável
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
							log.Info().Str("userID", userID).Int("qrLen", len(base64qrcode)).Msg("update cache userinfo with qr code")
						}
					}

					//send QR code with webhook
					postmap := make(map[string]interface{})
					postmap["event"] = evt.Event
					postmap["qrCodeBase64"] = base64qrcode
					postmap["type"] = "QR"

					sendEventWithWebHook(&mycli, postmap, "")

				case "timeout":
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
				case "success":
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
				default:
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
