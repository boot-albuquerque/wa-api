// Package port define as interfaces (ports) que os usecases consomem.
// Implementações concretas (adapters) vivem em internal/infrastructure/.
package port

import (
	"context"

	"disparazap/internal/shared/domain"
)

// MessagingPort abstrai operações de mensageria (RabbitMQ).
type MessagingPort interface {
	// PublishMessage publica uma mensagem no exchange.
	PublishMessage(ctx context.Context, routingKey string, body []byte) error

	// PublishEvent publica um evento de webhook.
	PublishEvent(ctx context.Context, event domain.Webhook, payload []byte) error

	// PublishError publica um erro para fila de dead-letter.
	PublishError(ctx context.Context, errType string, payload []byte) error
}
