package messaging

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	mwpkg "wa-api/pkg/presentation/http/middleware"

	"github.com/patrickmn/go-cache"
	"github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
)

// Type definitions for webhook error payloads
type WebhookFileErrorPayload struct {
	URL              string                 `json:"url"`
	Payload          map[string]interface{} `json:"payload"`
	UserID           string                 `json:"userID"`
	EncryptedHmacKey string                 `json:"encryptedHmacKey"`
	FilePath         string                 `json:"filePath"`
	AttemptTime      time.Time              `json:"attemptTime"`
	ErrorMessage     string                 `json:"errorMessage"`
}

type WebhookErrorPayload struct {
	URL              string                 `json:"url"`
	Payload          map[string]interface{} `json:"payload"`
	UserID           string                 `json:"userID"`
	EncryptedHmacKey string                 `json:"encryptedHmacKey"`
	AttemptTime      time.Time              `json:"attemptTime"`
	ErrorMessage     string                 `json:"errorMessage"`
}

// Values type for user info cache (alias to the middleware type actually
// stored in the cache by all callers).
type Values = mwpkg.Values

var (
	rabbitMu sync.RWMutex // protects RabbitConn, RabbitChannel, RabbitEnabled

	RabbitConn           amqpConnection
	RabbitChannel        amqpChannel
	RabbitEnabled        bool
	RabbitQueue          string
	userInfoCache        *cache.Cache
	webhookErrorQueuePtr *string
)

// safeGo runs fn in a goroutine with panic recovery.
func safeGo(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error().Str("goroutine", name).Interface("panic", r).Msg("panic recovered in messaging goroutine")
			}
		}()
		fn()
	}()
}

func getRabbitEnabled() bool {
	rabbitMu.RLock()
	defer rabbitMu.RUnlock()
	return RabbitEnabled
}

func getRabbitChannel() amqpChannel {
	rabbitMu.RLock()
	defer rabbitMu.RUnlock()
	return RabbitChannel
}

func setRabbitState(conn amqpConnection, ch amqpChannel, enabled bool) {
	rabbitMu.Lock()
	defer rabbitMu.Unlock()
	RabbitConn = conn
	RabbitChannel = ch
	RabbitEnabled = enabled
}

func setRabbitDisabled() {
	rabbitMu.Lock()
	defer rabbitMu.Unlock()
	RabbitEnabled = false
}

// MaxRetries e RetryInterval sao variaveis, e nao constantes, para que a
// suite possa encurtar o backoff sem esperar 30s reais por caso de teste.
// Producao nunca as reatribui.
var (
	MaxRetries    = 10
	RetryInterval = 3 * time.Second
)

// SetupDependencies sets up dependencies from the main package
func SetupDependencies(uic *cache.Cache, errorQueuePtr *string) {
	userInfoCache = uic
	webhookErrorQueuePtr = errorQueuePtr
}

// Call this in main() or initialization
func InitRabbitMQ() {
	rabbitURL := os.Getenv("RABBITMQ_URL")
	RabbitQueue = os.Getenv("RABBITMQ_QUEUE")

	if RabbitQueue == "" {
		RabbitQueue = "whatsapp_events" // default queue
	}

	if rabbitURL == "" {
		setRabbitState(nil, nil, false)
		log.Info().Str("queue", RabbitQueue).Msg("RABBITMQ_URL is not set. RabbitMQ publishing disabled.")
		return
	}

	// Attempt to connect with retry
	for attempt := 1; attempt <= MaxRetries; attempt++ {
		log.Info().
			Int("attempt", attempt).
			Int("max_retries", MaxRetries).
			Msg("Attempting to connect to RabbitMQ")

		conn, err := dialAMQP(rabbitURL)
		if err != nil {
			log.Warn().
				Err(err).
				Int("attempt", attempt).
				Int("max_retries", MaxRetries).
				Str("queue", RabbitQueue).
				Msg("Failed to connect to RabbitMQ")

			if attempt < MaxRetries {
				log.Info().
					Dur("retry_in", RetryInterval).
					Msg("Retrying RabbitMQ connection")
				time.Sleep(RetryInterval)
				continue
			}

			// Last attempt failed
			setRabbitState(nil, nil, false)
			log.Error().
				Err(err).
				Str("queue", RabbitQueue).
				Msg("Could not connect to RabbitMQ after all retries. RabbitMQ disabled.")
			return
		}

		// Connection successful, attempt to open channel
		channel, err := conn.openChannel()
		if err != nil {
			if closeErr := conn.Close(); closeErr != nil {
				log.Warn().Err(closeErr).Str("queue", RabbitQueue).Msg("Failed to close RabbitMQ connection")
			}
			log.Warn().
				Err(err).
				Int("attempt", attempt).
				Str("queue", RabbitQueue).
				Msg("Failed to open RabbitMQ channel")

			if attempt < MaxRetries {
				log.Info().
					Dur("retry_in", RetryInterval).
					Msg("Retrying RabbitMQ connection")
				time.Sleep(RetryInterval)
				continue
			}

			// Last attempt failed
			setRabbitState(nil, nil, false)
			log.Error().
				Err(err).
				Str("queue", RabbitQueue).
				Msg("Could not open RabbitMQ channel after all retries. RabbitMQ disabled.")
			return
		}

		// Success!
		setRabbitState(conn, channel, true)

		log.Info().
			Str("queue", RabbitQueue).
			Int("attempt", attempt).
			Msg("RabbitMQ connection established successfully")

		// Setup handler for automatic reconnection on errors
		safeGo("rabbitmq-handle-errors", func() { HandleConnectionErrors(conn) })
		return
	}
}

// Monitor connection errors and attempt reconnection.
// A conexao monitorada e' recebida por parametro — antes a goroutine lia o
// global RabbitConn sem o mutex, o que era corrida com setRabbitState e
// podia monitorar uma conexao diferente da que a originou.
func HandleConnectionErrors(conn amqpConnection) {
	notifyClose := conn.NotifyClose(make(chan *amqp091.Error))

	for err := range notifyClose {
		log.Error().
			Err(err).
			Str("queue", RabbitQueue).
			Msg("RabbitMQ connection closed unexpectedly. Attempting reconnection...")

		setRabbitDisabled()

		// Attempt to reconnect
		for attempt := 1; attempt <= MaxRetries; attempt++ {
			log.Info().
				Int("attempt", attempt).
				Msg("Reconnecting to RabbitMQ")

			time.Sleep(RetryInterval)

			rabbitURL := os.Getenv("RABBITMQ_URL")
			newConn, err := dialAMQP(rabbitURL)
			if err != nil {
				log.Warn().
					Err(err).
					Int("attempt", attempt).
					Str("queue", RabbitQueue).
					Msg("Reconnection failed")
				continue
			}

			channel, err := newConn.openChannel()
			if err != nil {
				if closeErr := newConn.Close(); closeErr != nil {
					log.Warn().Err(closeErr).Str("queue", RabbitQueue).Msg("Failed to close RabbitMQ connection")
				}
				log.Warn().
					Err(err).
					Int("attempt", attempt).
					Str("queue", RabbitQueue).
					Msg("Failed to open channel on reconnection")
				continue
			}

			// Reconnection successful
			setRabbitState(newConn, channel, true)

			log.Info().Str("queue", RabbitQueue).Int("attempt", attempt).Msg("RabbitMQ reconnected successfully")

			// Restart monitoring (single goroutine per reconnect, safer than before)
			safeGo("rabbitmq-handle-errors", func() { HandleConnectionErrors(newConn) })
			return
		}

		log.Error().Str("queue", RabbitQueue).Int("max_retries", MaxRetries).Msg("Failed to reconnect to RabbitMQ after all retries")
		return
	}
}

// Optionally, allow overriding the queue per message
func PublishToRabbit(data []byte, queueOverride ...string) error {
	if !getRabbitEnabled() {
		return nil
	}
	channel := getRabbitChannel()
	if channel == nil {
		return nil
	}
	queueName := RabbitQueue
	if len(queueOverride) > 0 && queueOverride[0] != "" {
		queueName = queueOverride[0]
	}
	// Declare queue (idempotent)
	_, err := channel.QueueDeclare(
		queueName,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		log.Error().Err(err).Str("queue", queueName).Msg("Could not declare RabbitMQ queue")
		return err
	}
	err = channel.Publish(
		"",        // exchange (default)
		queueName, // routing key = queue
		false,     // mandatory
		false,     // immediate
		amqp091.Publishing{
			ContentType:  "application/json",
			Body:         data,
			DeliveryMode: amqp091.Persistent,
		},
	)
	if err != nil {
		log.Error().Err(err).Str("queue", queueName).Msg("Could not publish to RabbitMQ")
	} else {
		log.Debug().Str("queue", queueName).Msg("Published message to RabbitMQ")
	}
	return err
}

func SendToGlobalRabbit(jsonData []byte, token string, userID string, queueName ...string) {
	if !getRabbitEnabled() {
		// Check if RabbitMQ is configured but disabled due to connection issues
		rabbitURL := os.Getenv("RABBITMQ_URL")
		rabbitQueueEnv := os.Getenv("RABBITMQ_QUEUE")

		if rabbitURL != "" || rabbitQueueEnv != "" {
			urlSet := "no"
			if rabbitURL != "" {
				urlSet = "yes"
			}
			queueSet := "no"
			if rabbitQueueEnv != "" {
				queueSet = "yes"
			}
			log.Error().
				Str("rabbitmq_url_set", urlSet).
				Str("rabbitmq_queue_set", queueSet).
				Msg("RabbitMQ is configured but disabled due to connection failure. Event not published to queue.")
		} else {
			log.Debug().Str("queue", RabbitQueue).Msg("RabbitMQ not configured. Event not published to queue.")
		}
		return
	}

	// Extract instance information
	instance_name := ""
	if userInfoCache != nil {
		userinfo, found := userInfoCache.Get(token)
		if found {
			if v, ok := userinfo.(Values); ok {
				instance_name = v.Get("Name")
			}
		}
	}

	// Parse the original JSON into a map
	var originalData map[string]interface{}
	err := json.Unmarshal(jsonData, &originalData)
	if err != nil {
		log.Error().Err(err).Str("queue", RabbitQueue).Str("user_id", userID).Int("payload_bytes", len(jsonData)).Msg("Failed to unmarshal original JSON data for RabbitMQ")
		return
	}

	// Add the new fields directly to the original data
	originalData["userID"] = userID
	originalData["instanceName"] = instance_name

	// Marshal back to JSON
	enhancedJSON, err := json.Marshal(originalData)
	if err != nil {
		log.Error().Err(err).Str("queue", RabbitQueue).Str("user_id", userID).Msg("Failed to marshal enhanced data for RabbitMQ")
		return
	}

	err = PublishToRabbit(enhancedJSON, queueName...)
	if err != nil {
		log.Error().Err(err).Str("queue", RabbitQueue).Str("user_id", userID).Msg("Failed to publish to RabbitMQ")
	}
}

func PublishFileErrorToQueue(payload WebhookFileErrorPayload) {
	if webhookErrorQueuePtr == nil {
		log.Error().Str("user_id", payload.UserID).Str("file_path", payload.FilePath).Msg("Webhook error queue not configured")
		return
	}

	queueName := *webhookErrorQueuePtr

	body, err := json.Marshal(payload)
	if err != nil {
		log.Error().Err(err).Str("queue", queueName).Str("user_id", payload.UserID).Str("file_path", payload.FilePath).Msg("Failed to marshal file error payload for RabbitMQ")
		return
	}

	err = PublishToRabbit(body, queueName)
	if err != nil {
		log.Error().Err(err).Str("queue", queueName).Str("user_id", payload.UserID).Str("file_path", payload.FilePath).Msg("Failed to publish file error payload to queue")
	} else {
		log.Info().Str("queue", queueName).Msg("File error payload successfully published to queue")
	}
}

func PublishDataErrorToQueue(payload WebhookErrorPayload) {
	if webhookErrorQueuePtr == nil {
		log.Error().Str("user_id", payload.UserID).Str("webhook_url", payload.URL).Msg("Webhook error queue not configured")
		return
	}

	queueName := *webhookErrorQueuePtr
	body, err := json.Marshal(payload)
	if err != nil {
		log.Error().Err(err).Str("queue", queueName).Str("user_id", payload.UserID).Str("webhook_url", payload.URL).Msg("Failed to marshal data error payload for RabbitMQ")
		return
	}
	err = PublishToRabbit(body, queueName)
	if err != nil {
		log.Error().Err(err).Str("queue", queueName).Str("user_id", payload.UserID).Str("webhook_url", payload.URL).Msg("Failed to publish data error payload to queue")
	} else {
		log.Info().Str("queue", queueName).Msg("Data error payload successfully published to queue")
	}
}
