package bootstrap

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"runtime/debug"
	"sync"

	"time"

	"github.com/go-resty/resty/v2"
	"golang.org/x/sync/singleflight"

	"github.com/patrickmn/go-cache"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"

	"wa-api/internal/infra/storage"
	"wa-api/internal/infra/auth"
	"wa-api/internal/infra/helpers"
	"wa-api/internal/infra/media/opengraph"
	"wa-api/internal/infra/media/sticker"
)

const (
	openGraphFetchTimeout    = 5 * time.Second
	fetchImageMaxBytes       = 16 * 1024 * 1024  // 16MB
	fetchVideoMaxBytes       = 100 * 1024 * 1024 // 100MB
	fetchAudioMaxBytes       = 16 * 1024 * 1024  // 16MB
	fetchDocumentMaxBytes    = 100 * 1024 * 1024 // 100MB
	openGraphPageMaxBytes    = 2 * 1024 * 1024   // 2MB
	openGraphImageMaxBytes   = 10 * 1024 * 1024  // 10MB
	openGraphUserFetchLimit  = 20   // Limit concurrent Open Graph fetches per user

)



// ProxyConfig holds per-user proxy settings for WhatsApp and webhook delivery.
type ProxyConfig struct {
	Enabled         bool  `json:"enabled"`
	ProxyURL        string `json:"proxyURL"`
	WebhookUseProxy *bool `json:"webhookUseProxy,omitempty"`
}

func resolveWebhookUseProxy(perUser *bool) bool {
	if perUser != nil {
		return *perUser
	}
	return appCtx.GlobalWebhookUseProxy
}

func proxyConfigResponse(proxyURL string, webhookUseProxy bool) map[string]interface{} {
	return map[string]interface{}{
		"enabled":           proxyURL != "",
		"proxy_url":         proxyURL,
		"webhook_use_proxy": webhookUseProxy,
	}
}

type openGraphResult struct {
	Title       string
	Description string
	ImageData   []byte // small inline thumbnail (JPEGThumbnail field)
	HQImageData []byte // larger thumbnail uploaded to WA media servers for the big preview card
	HQWidth     uint32
	HQHeight    uint32
}

type UserSemaphoreManager struct {
	pools sync.Map
}

func NewUserSemaphoreManager() *UserSemaphoreManager {
	return &UserSemaphoreManager{}
}

func (usm *UserSemaphoreManager) ForUser(userID string) chan struct{} {
	// LoadOrStore provides an atomic way to get or create a semaphore.
	pool, _ := usm.pools.LoadOrStore(userID, make(chan struct{}, openGraphUserFetchLimit))
	return pool.(chan struct{})
}

var (
	userSemaphoreManager = NewUserSemaphoreManager()

	openGraphGroup singleflight.Group

	openGraphCache = cache.New(5*time.Minute, 10*time.Minute) // Cache Open Graph data for 5 minutes, cleanup every 10 minutes

)

var Find = helpers.Find

var isHTTPURL = helpers.IsHTTPURL

func fetchURLBytes(ctx context.Context, url string, limit int64) ([]byte, string, error) { return opengraph.FetchURLBytes(ctx, globalHTTPClient, url, limit) }

func getOpenGraphData(ctx context.Context, urlStr string, userID string) openGraphResult {
	// Check cache first
	if cachedData, found := openGraphCache.Get(urlStr); found {
		if data, ok := cachedData.(openGraphResult); ok {
			log.Debug().Str("url", urlStr).Msg("Open Graph data fetched from cache")
			return data
		}
	}

	v, err, _ := openGraphGroup.Do(urlStr, func() (res any, err error) {
		ctx, cancel := context.WithTimeout(ctx, openGraphFetchTimeout)
		defer cancel()

		// Acquire a token from the semaphore pool
		userPool := userSemaphoreManager.ForUser(userID)
		select {
		case userPool <- struct{}{}:
			defer func() { <-userPool }()
		case <-ctx.Done():
			log.Warn().Str("url", urlStr).Msg("Open Graph data fetch timed out while waiting for a worker")
			return nil, ctx.Err()
		}

		// Recover from panics and convert to error
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				log.Error().
					Interface("panic_info", r).
					Str("url", urlStr).
					Bytes("stack", stack).
					Msg("Panic recovered while fetching Open Graph data")
				err = fmt.Errorf("panic: %v", r)
			}
		}()

		// Fetch Open Graph data
		result := fetchOpenGraphData(ctx, urlStr)

		// Store in cache
		openGraphCache.Set(urlStr, result, cache.DefaultExpiration)

		return result, nil
	})

	if err != nil {
		log.Error().Err(err).Str("url", urlStr).Msg("Error fetching Open Graph data via singleflight")
		return openGraphResult{}
	}

	if v == nil {
		return openGraphResult{}
	}

	return v.(openGraphResult)
}

// Update entry in User map
func updateUserInfo(values interface{}, field string, value string) interface{} {
	log.Debug().Str("field", field).Str("value", value).Msg("User info updated")
	// Copy-on-write: the map inside Values is shared — it lives in
	// appCtx.UserInfoCache and is handed to request goroutines via the request
	// context. Mutating it in place races with concurrent readers (Values.Get)
	// and can crash the process with "concurrent map read and map write".
	// Build a fresh map and return a new Values; callers persist it via
	// appCtx.UserInfoCache.Set. Use a comma-ok assertion so a nil or unexpected value
	// can't panic — it falls back to the zero Values (nil map), handled below.
	old, _ := values.(Values)
	m := make(map[string]string, len(old.M)+1)
	for k, v := range old.M {
		m[k] = v
	}
	m[field] = value
	return Values{M: m}
}

// webhook for regular messages
func callHook(myurl string, payload map[string]string, userID string) {
	callHookWithHmac(myurl, payload, userID, nil)
}

// webhook for regular messages with HMAC
func callHookWithHmac(myurl string, payload map[string]string, userID string, encryptedHmacKey []byte) {
	log.Info().Str("url", myurl).Str("userID", userID).Msg("Sending POST to client with retry logic")

	client := clientManager.GetHTTPClient(userID)
	if client == nil {
		log.Warn().Str("url", myurl).Str("userID", userID).Msg("HTTP client is nil for user, skipping webhook")
		return
	}

	// Retry settings
	maxRetries := 1
	if appCtx.WebhookRetryEnabled {
		maxRetries = appCtx.WebhookRetryCount
	}

	var lastError error

	var body interface{} = payload

	// Starts the retry loop.
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoffFactor := 1 << uint(attempt-1)

			// Calculate the final delay.
			delayDuration := time.Duration(appCtx.WebhookRetryDelaySeconds) * time.Second * time.Duration(backoffFactor)

			log.Warn().
				Int("attempt", attempt+1).
				Str("url", myurl).
				Dur("delay", delayDuration).
				Msg("Retrying webhook request with exponential backoff...")

			time.Sleep(delayDuration)
		}

		var req *resty.Request
		var hmacSignature string
		var marshalErr error

		format := os.Getenv("WEBHOOK_FORMAT")

		if format == "json" {
			var jsonBody []byte

			if jsonStr, ok := payload["jsonData"]; ok {
				var postmap map[string]interface{}

				if err := json.Unmarshal([]byte(jsonStr), &postmap); err == nil {
					if instanceName, ok := payload["instanceName"]; ok {
						postmap["instanceName"] = instanceName
					}
					postmap["userID"] = userID
					body = postmap
				}
			}

			// Marshal body to JSON for HMAC signature
			jsonBody, marshalErr = json.Marshal(body)
			if marshalErr != nil {
				log.Error().Err(marshalErr).Msg("Failed to marshal body for HMAC")
			}

			// Generate HMAC signature if key exists
			if len(encryptedHmacKey) > 0 && len(jsonBody) > 0 {
				var err error
				hmacSignature, err = generateHmacSignature(jsonBody, encryptedHmacKey)
				if err != nil {
					log.Error().Err(err).Msg("Failed to generate HMAC signature")
				}
			}

			req = client.R().SetHeader("Content-Type", "application/json").SetBody(body)

		} else {

			if len(encryptedHmacKey) > 0 {
				formData := url.Values{}
				for k, v := range payload {
					formData.Add(k, v)
				}
				formString := formData.Encode()
				var err error
				hmacSignature, err = generateHmacSignature([]byte(formString), encryptedHmacKey)
				if err != nil {
					log.Error().Err(err).Msg("Failed to generate HMAC signature")
				}
			}
			req = client.R().SetFormData(payload)
			body = payload
		}

		if hmacSignature != "" {
			req.SetHeader("x-hmac-signature", hmacSignature)
		}

		resp, postErr := req.Post(myurl)

		lastError = postErr

		if postErr != nil {
			log.Error().Err(postErr).Int("attempt", attempt+1).Str("url", myurl).Msg("Webhook failed due to network/IO error")
			continue
		}

		if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
			lastError = fmt.Errorf("unexpected status code: %d. Body: %s", resp.StatusCode(), string(resp.Body()))
			log.Error().
				Int("status", resp.StatusCode()).
				Int("attempt", attempt+1).
				Str("url", myurl).
				Msg("Webhook failed due to non-2xx status code")

			if !appCtx.WebhookRetryEnabled {
				break
			}
			continue
		}

		log.Info().Int("status", resp.StatusCode()).Str("url", myurl).Msg("Webhook call successful")
		return
	}

	if lastError != nil {
		log.Error().Str("url", myurl).Msg("Webhook permanently failed after all retries. Sending to error queue...")

		errorPayloadMap := make(map[string]interface{})
		if p, ok := body.(map[string]string); ok {

			for k, v := range p {
				errorPayloadMap[k] = v
			}
		} else if p, ok := body.(map[string]interface{}); ok {

			errorPayloadMap = p
		}

		errorPayload := WebhookErrorPayload{
			URL:              myurl,
			Payload:          errorPayloadMap,
			UserID:           userID,
			EncryptedHmacKey: hex.EncodeToString(encryptedHmacKey),
			AttemptTime:      time.Now(),
			ErrorMessage:     lastError.Error(),
		}

		PublishDataErrorToQueue(errorPayload)
	}
}

// webhook for messages with file attachments
func callHookFile(myurl string, payload map[string]string, userID string, file string) error {
	return callHookFileWithHmac(myurl, payload, userID, file, nil)
}

// webhook for messages with file attachments and HMAC
func callHookFileWithHmac(myurl string, payload map[string]string, userID string, file string, encryptedHmacKey []byte) error {
	log.Info().Str("file", file).Str("url", myurl).Msg("Sending POST with retry logic")

	client := clientManager.GetHTTPClient(userID)
	if client == nil {
		log.Warn().Str("url", myurl).Str("userID", userID).Msg("HTTP client is nil for user, skipping file webhook")
		return fmt.Errorf("http client is nil for user %s", userID)
	}

	maxRetries := 1
	if appCtx.WebhookRetryEnabled {
		maxRetries = appCtx.WebhookRetryCount
	}

	var lastError error

	finalPayload := make(map[string]string)
	for k, v := range payload {
		finalPayload[k] = v
	}
	finalPayload["file"] = file

	// 2. Loop Retry
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoffFactor := 1 << uint(attempt-1)

			delayDuration := time.Duration(appCtx.WebhookRetryDelaySeconds) * time.Second * time.Duration(backoffFactor)

			log.Warn().
				Int("attempt", attempt+1).
				Str("url", myurl).
				Dur("delay", delayDuration).
				Msg("Retrying file webhook request with exponential backoff...")

			time.Sleep(delayDuration)
		}

		var hmacSignature string
		var jsonPayload []byte

		if len(encryptedHmacKey) > 0 {
			var err error
			jsonPayload, err = json.Marshal(finalPayload)
			if err != nil {
				log.Error().Err(err).Msg("Failed to marshal payload for HMAC")
			} else {
				hmacSignature, err = generateHmacSignature(jsonPayload, encryptedHmacKey)
				if err != nil {
					log.Error().Err(err).Msg("Failed to generate HMAC signature")
				}
			}
		}

		req := client.R().
			SetFiles(map[string]string{
				"file": file,
			}).
			SetFormData(finalPayload)

		if hmacSignature != "" {
			req.SetHeader("x-hmac-signature", hmacSignature)
		}

		resp, postErr := req.Post(myurl)

		lastError = postErr

		if postErr != nil {
			log.Error().Err(postErr).Int("attempt", attempt+1).Str("url", myurl).Msg("File webhook failed due to network/IO error")
			continue
		}

		if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
			lastError = fmt.Errorf("unexpected status code: %d. Body: %s", resp.StatusCode(), string(resp.Body()))
			log.Error().
				Int("status", resp.StatusCode()).
				Int("attempt", attempt+1).
				Str("url", myurl).
				Msg("File webhook failed due to non-2xx status code")

			if !appCtx.WebhookRetryEnabled {
				break
			}
			continue
		}

		log.Info().Int("status", resp.StatusCode()).Str("url", myurl).Msg("File webhook call successful")
		return nil
	}

	if lastError != nil {
		log.Error().Str("url", myurl).Msg("File webhook permanently failed after all retries. Sending to error queue...")

		errorPayloadMap := make(map[string]interface{})
		for k, v := range finalPayload {
			errorPayloadMap[k] = v
		}

		errorPayload := WebhookFileErrorPayload{
			URL:              myurl,
			Payload:          errorPayloadMap,
			UserID:           userID,
			EncryptedHmacKey: hex.EncodeToString(encryptedHmacKey),
			FilePath:         file,
			AttemptTime:      time.Now(),
			ErrorMessage:     lastError.Error(),
		}

		PublishFileErrorToQueue(errorPayload)

		return fmt.Errorf("webhook failed permanently: %w", lastError)
	}

	return nil
}

// ProcessOutgoingMedia handles media processing for outgoing messages with S3 support
func ProcessOutgoingMedia(userID string, contactJID string, messageID string, data []byte, mimeType string, fileName string, db *sqlx.DB) (map[string]interface{}, error) {
	// Check if S3 is enabled for this user
	var s3Config struct {
		Enabled       bool   `db:"s3_enabled"`
		MediaDelivery string `db:"media_delivery"`
	}
	err := db.Get(&s3Config, "SELECT s3_enabled, media_delivery FROM users WHERE id = $1", userID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get S3 config")
		s3Config.Enabled = false
		s3Config.MediaDelivery = "base64"
	}

	// Process S3 upload if enabled
	if s3Config.Enabled && (s3Config.MediaDelivery == "s3" || s3Config.MediaDelivery == "both") {
		ensureS3ClientForUser(userID)
		// Process S3 upload (outgoing messages are always in outbox)
		s3Data, err := storage.GetS3Manager().ProcessMediaForS3(
			context.Background(),
			userID,
			contactJID,
			messageID,
			data,
			mimeType,
			fileName,
			false, // isIncoming = false for sent messages
		)
		if err != nil {
			log.Error().Err(err).Msg("Failed to upload media to S3")
			// Continue even if S3 upload fails
		} else {
			return s3Data, nil
		}
	}

	return nil, nil
}





// HMAC crypto delegates to internal/infrastructure/auth
func generateHmacSignature(payload, encryptedKey []byte) (string, error) {
	return auth.GenerateHmacSignature(payload, encryptedKey, []byte(appCtx.GlobalEncryptionKey))
}
func encryptHMACKey(plainText string) ([]byte, error) {
	return auth.EncryptHMACKey(plainText, appCtx.GlobalEncryptionKey)
}
func decryptHMACKey(encryptedData []byte) (string, error) {
	return auth.DecryptHMACKey(encryptedData, []byte(appCtx.GlobalEncryptionKey))
}

var extractFirstURL = helpers.ExtractFirstURL
func fetchOpenGraphData(ctx context.Context, urlStr string) openGraphResult {
	oResult := opengraph.FetchOpenGraphData(ctx, globalHTTPClient, urlStr)
	return openGraphResult{Title: oResult.Title, Description: oResult.Description, ImageData: oResult.ImageData, HQImageData: oResult.HQImageData, HQWidth: oResult.HQWidth, HQHeight: oResult.HQHeight}
}

var encodeJPEGThumbnail = sticker.EncodeJPEGThumbnail

func fetchOpenGraphImage(ctx context.Context, pageURL *url.URL, imageURLStr string, result *openGraphResult) {
	oResult := opengraph.Result{Title: result.Title, Description: result.Description, ImageData: result.ImageData, HQImageData: result.HQImageData, HQWidth: result.HQWidth, HQHeight: result.HQHeight}
	opengraph.FetchOpenGraphImage(ctx, globalHTTPClient, pageURL, imageURLStr, &oResult)
	result.ImageData = oResult.ImageData; result.HQImageData = oResult.HQImageData; result.HQWidth = oResult.HQWidth; result.HQHeight = oResult.HQHeight
}

var runFFmpegConversion = sticker.RunFFmpegConversion

var convertVideoStickerToWebP = sticker.ConvertVideoStickerToWebP

var convertImageToWebP = sticker.ConvertImageToWebP

var processStickerData = sticker.ProcessStickerData

var convertToWebPSticker = sticker.ConvertToWebPSticker

var embedStickerEXIF = sticker.EmbedStickerEXIF

var buildStickerMetadata = sticker.BuildStickerMetadata

var buildWhatsAppEXIF = sticker.BuildWhatsAppEXIF

var injectWebPEXIF = sticker.InjectWebPEXIF

var isValidWebP = sticker.IsValidWebP

var parseWebPChunks = sticker.ParseWebPChunks

var ensureVP8XWithEXIF = sticker.EnsureVP8XWithEXIF

var createVP8XChunk = sticker.CreateVP8XChunk

var putUint24LE = sticker.PutUint24LE

var assembleWebP = sticker.AssembleWebP

var writeChunk = sticker.WriteChunk
