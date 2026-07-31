package whatsmeow

import (
	"context"
	"fmt"

	appport "wa-api/internal/contracts"

	wa "go.mau.fi/whatsmeow"
)

// ClientProviderAdapter implementa appport.ClientProvider usando o
// clientManager global do upstream (package main).
type ClientProviderAdapter struct {
	getClient func(txtID string) *wa.Client
}

// NewClientProviderAdapter cria o adapter com a função de lookup.
// O parâmetro getClient é tipicamente clientManager.GetWhatsmeowClient.
func NewClientProviderAdapter(getClient func(txtID string) *wa.Client) *ClientProviderAdapter {
	return &ClientProviderAdapter{getClient: getClient}
}

// GetWhatsmeowClient obtém o cliente whatsmeow pelo txtID.
// Retorna erro se não houver cliente conectado (nil).
func (a *ClientProviderAdapter) GetWhatsmeowClient(ctx context.Context, txtID string) (*wa.Client, error) {
	client := a.getClient(txtID)
	if client == nil {
		return nil, fmt.Errorf("whatsmeow: no client for txtID %s", txtID)
	}
	return client, nil
}

// Verificação em tempo de compilação de que ClientProviderAdapter implementa a interface.
var _ appport.ClientProvider = (*ClientProviderAdapter)(nil)
