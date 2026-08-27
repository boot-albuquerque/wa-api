package user

import (
	"context"

	waclient "wa-api/pkg/infra/wa-noise/client"

	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/pkg/domain"
)

// GetPrivacySettings devolve as configurações atuais, já normalizadas.
//
// Antes da migração de DTO este método devolvia `any` com o
// types.PrivacySettings do SDK, e esse struct ia inteiro para o fio — sem
// etiquetas `json`, o codificador emitia `GroupAdd`, `LastSeen`,
// `ReadReceipts`. A normalização acontece aqui porque é onde o vendor para.
func (a *UserAdapter) GetPrivacySettings(ctx context.Context, txtID string) (domain.PrivacySettings, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return domain.PrivacySettings{}, err
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()

	settings, err := client.TryFetchPrivacySettings(ctxWithTimeout, false)
	if err != nil {
		return domain.PrivacySettings{}, err
	}
	return mapPrivacySettings(settings), nil
}

// SetPrivacySetting altera uma configuração de privacidade.
func (a *UserAdapter) SetPrivacySetting(ctx context.Context, txtID, name, value string) (domain.PrivacySettings, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return domain.PrivacySettings{}, err
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()

	settings, err := client.SetPrivacySetting(ctxWithTimeout,
		types.PrivacySettingType(name), types.PrivacySetting(value))
	if err != nil {
		return domain.PrivacySettings{}, err
	}
	return mapPrivacySettings(&settings), nil
}

// mapPrivacySettings converte o struct do SDK, campo a campo.
//
// À mão, e não por reflexão: acrescentar um campo ao types.PrivacySettings do
// vendor tem de ser uma DECISÃO nossa de o expor, não uma chave nova que
// aparece sozinha na resposta pública no próximo rebase.
func mapPrivacySettings(s *types.PrivacySettings) domain.PrivacySettings {
	if s == nil {
		return domain.PrivacySettings{}
	}
	return domain.PrivacySettings{
		GroupAdd:     string(s.GroupAdd),
		LastSeen:     string(s.LastSeen),
		Status:       string(s.Status),
		Profile:      string(s.Profile),
		ReadReceipts: string(s.ReadReceipts),
		CallAdd:      string(s.CallAdd),
		Online:       string(s.Online),
		Messages:     string(s.Messages),
		Defense:      string(s.Defense),
		Stickers:     string(s.Stickers),
	}
}
