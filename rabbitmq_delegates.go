package wuzapi

import "wuzapi/internal/infrastructure/messaging"

// Re-export types from internal/messaging
type WebhookFileErrorPayload = messaging.WebhookFileErrorPayload
type WebhookErrorPayload = messaging.WebhookErrorPayload

var (
	PublishToRabbit        = messaging.PublishToRabbit
	PublishFileErrorToQueue = messaging.PublishFileErrorToQueue
	PublishDataErrorToQueue = messaging.PublishDataErrorToQueue
)

// InitRabbitMQ wraps the internal function, injecting appCtx-specific config.
func InitRabbitMQ() {
	messaging.SetupDependencies(appCtx.UserInfoCache, webhookErrorQueueName)
	messaging.InitRabbitMQ()
}

func handleConnectionErrors() { messaging.HandleConnectionErrors() }

func sendToGlobalRabbit(jsonData []byte, token, userID string, queueName ...string) {
	messaging.SendToGlobalRabbit(jsonData, token, userID, queueName...)
}
