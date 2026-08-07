package user

import (
	"context"
	"wa-api/pkg/infra/wa-noise/waclient"

	"wa-api/internal/wa-noise/types"
)

// GetPrivacySettings devolve as configurações atuais.
func (a *UserAdapter) GetPrivacySettings(ctx context.Context, txtID string) (any, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, err
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()

	return client.TryFetchPrivacySettings(ctxWithTimeout, false)
}

// SetPrivacySetting altera uma configuração de privacidade.
func (a *UserAdapter) SetPrivacySetting(ctx context.Context, txtID, name, value string) (any, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, err
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()

	return client.SetPrivacySetting(ctxWithTimeout, types.PrivacySettingType(name), types.PrivacySetting(value))
}
