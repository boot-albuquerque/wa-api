package newsletter

import (
	"context"
	"encoding/json"
	"strings"

	"wa-api/internal/noise/protocol/types"
)

type respGetNewsletterInfo struct {
	Newsletter *types.NewsletterMetadata `json:"xwa2_newsletter"`
}

// JIDInput e InviteInput montam o campo "input" da consulta MEX de metadados. O
// convite aceita tanto o link completo quanto so' o codigo, entao o prefixo e'
// removido antes de ir para o wire.
func JIDInput(jid types.JID) map[string]any {
	return map[string]any{
		"key":  jid.String(),
		"type": types.NewsletterKeyTypeJID,
	}
}

func InviteInput(key string) map[string]any {
	return map[string]any{
		"key":  strings.TrimPrefix(key, LinkPrefix),
		"type": types.NewsletterKeyTypeInvite,
	}
}

// GetInfo busca os metadados de um canal a partir do "input" montado por
// JIDInput ou InviteInput.
//
// Repare que o erro do SendMexIQ nao aborta a decodificacao: uma resposta
// GraphQL com erros ainda traz "data" parcial, e nesse caso o erro do servidor
// e' o que volta ao chamador. Um erro de unmarshal so' substitui o erro de rede
// quando nao houve erro de rede.
func GetInfo(ctx context.Context, t Transport, input map[string]any, fetchViewerMeta bool) (*types.NewsletterMetadata, error) {
	data, err := SendMexIQ(ctx, t, queryFetchNewsletter, map[string]any{
		"fetch_creation_time":   true,
		"fetch_full_image":      true,
		"fetch_viewer_metadata": fetchViewerMeta,
		"input":                 input,
	})
	var respData respGetNewsletterInfo
	if data != nil {
		jsonErr := json.Unmarshal(data, &respData)
		if err == nil && jsonErr != nil {
			err = jsonErr
		}
	}
	return respData.Newsletter, err
}

type respGetSubscribedNewsletters struct {
	Newsletters []*types.NewsletterMetadata `json:"xwa2_newsletter_subscribed"`
}

// GetSubscribed busca os metadados de todos os canais que o usuario segue.
// Mesma politica de erro de GetInfo.
func GetSubscribed(ctx context.Context, t Transport) ([]*types.NewsletterMetadata, error) {
	data, err := SendMexIQ(ctx, t, querySubscribedNewsletters, map[string]any{})
	var respData respGetSubscribedNewsletters
	if data != nil {
		jsonErr := json.Unmarshal(data, &respData)
		if err == nil && jsonErr != nil {
			err = jsonErr
		}
	}
	return respData.Newsletters, err
}
