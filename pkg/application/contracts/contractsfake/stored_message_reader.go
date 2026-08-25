package contractsfake

import (
	"context"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// StoredMessageReader is the fake of port.StoredMessageReader.
type StoredMessageReader struct {
	GetStoredMessageFunc func(ctx context.Context, userID, messageID string) (*domain.StoredMessageData, error)
}

var _ port.StoredMessageReader = (*StoredMessageReader)(nil)

// GetStoredMessage implements port.StoredMessageReader.
func (f *StoredMessageReader) GetStoredMessage(ctx context.Context, userID, messageID string) (*domain.StoredMessageData, error) {
	if f.GetStoredMessageFunc != nil {
		return f.GetStoredMessageFunc(ctx, userID, messageID)
	}
	return nil, nil
}
