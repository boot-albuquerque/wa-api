package whatsmeow

import (
	"context"
	"time"

	"wa-api/internal/waclient/types"
)

// GetPrivacySettings devolve as configurações atuais.
func (a *UserAdapter) GetPrivacySettings(ctx context.Context, txtID string) (any, error) {
	client, err := a.client(txtID)
	if err != nil {
		return nil, err
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return client.TryFetchPrivacySettings(ctxWithTimeout, false)
}

// SetPrivacySetting altera uma configuração de privacidade.
func (a *UserAdapter) SetPrivacySetting(ctx context.Context, txtID, name, value string) (any, error) {
	client, err := a.client(txtID)
	if err != nil {
		return nil, err
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return client.SetPrivacySetting(ctxWithTimeout, types.PrivacySettingType(name), types.PrivacySetting(value))
}
