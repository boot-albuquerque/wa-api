package bootstrap

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"

	"time"

	"github.com/go-resty/resty/v2"

	"github.com/rs/zerolog/log"
)

// callHookWithHmac entrega um evento ao webhook do usuário.
//
// UMA tentativa por chamada. Quando falha, a próxima é AGENDADA (ver
// dispatch_retry.go) em vez de esperada aqui dentro. A espera do backoff
// ficava num time.Sleep DENTRO desta função, e desde a F86 esta função roda
// num worker do pool de despacho — com os padrões antigos (5 tentativas, base
// 30s) um evento para um destino morto segurava um worker por 7,5 minutos.
func callHookWithHmac(myurl string, payload map[string]string, userID string, encryptedHmacKey []byte) {
	tentarWebhook(myurl, payload, userID, encryptedHmacKey, 0)
}

// montarRequisicaoWebhook monta corpo, assinatura e requisição de UMA
// tentativa. Extraída do laço original sem alteração de comportamento: o
// formato (json ou form) e a assinatura HMAC seguem exatamente como estavam.
func montarRequisicaoWebhook(client *resty.Client, payload map[string]string, userID string, encryptedHmacKey []byte) (interface{}, *resty.Request) {
	var body interface{} = payload
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
	return body, req
}

// tentarWebhook executa UMA tentativa e decide o que vem depois: sucesso,
// reagendamento ou caminho terminal (fila de erro).
func tentarWebhook(myurl string, payload map[string]string, userID string, encryptedHmacKey []byte, tentativa int) {
	log.Info().Str("url", myurl).Str("userID", userID).Int("attempt", tentativa+1).
		Msg("Sending POST to client with retry logic")

	client := clientManager.GetHTTPClient(userID)
	if client == nil {
		log.Warn().Str("url", myurl).Str("userID", userID).Msg("HTTP client is nil for user, skipping webhook")
		return
	}

	body, req := montarRequisicaoWebhook(client, payload, userID, encryptedHmacKey)
	resp, postErr := req.Post(myurl)

	var lastError error
	switch {
	case postErr != nil:
		lastError = postErr
		log.Error().Err(postErr).Int("attempt", tentativa+1).Str("url", myurl).
			Msg("Webhook failed due to network/IO error")
	case resp.StatusCode() < 200 || resp.StatusCode() >= 300:
		lastError = fmt.Errorf("unexpected status code: %d. Body: %s", resp.StatusCode(), string(resp.Body()))
		log.Error().
			Int("status", resp.StatusCode()).
			Int("attempt", tentativa+1).
			Str("url", myurl).
			Msg("Webhook failed due to non-2xx status code")
	default:
		log.Info().Int("status", resp.StatusCode()).Str("url", myurl).Msg("Webhook call successful")
		return
	}

	if agendarProximaTentativa(myurl, payload, userID, encryptedHmacKey, tentativa+1) {
		return
	}
	entregarWebhookNaFilaDeErro(myurl, body, userID, encryptedHmacKey, lastError)
}

// entregarWebhookNaFilaDeErro é o caminho terminal, preservado como estava.
//
// ATENÇÃO ao contar com ele: dentro deste repositório NÃO existe consumidor
// desta fila. Se algo a consome, é do lado do deployment.
func entregarWebhookNaFilaDeErro(myurl string, body interface{}, userID string, encryptedHmacKey []byte, lastError error) {
	if lastError == nil {
		return
	}
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
