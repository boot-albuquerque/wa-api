package whatsmeow

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain/apperr"

	wa "go.mau.fi/whatsmeow"
)

// ErrNoSession é o erro tipado que toda porta de capacidade devolve quando
// não existe sessão WhatsApp para o txtID pedido. Nasce tipado por exigência
// da Fase 3 (portas novas não nascem devendo taxonomia de erro).
//
// Os use cases desta fase ainda o traduzem para o erro que já devolviam
// antes: propagá-lo até RespondJSON mudaria o status HTTP e o corpo do erro
// dessas rotas, que é contrato observável e território da Fase 4a.
func ErrNoSession(txtID string, cause error) *apperr.AppError {
	return apperr.New(
		"no_session",
		apperr.CategoryValidation,
		"no session",
		false,
		cause,
	)
}

// SessionGuardAdapter implementa appport.SessionGuard sobre o clientManager.
type SessionGuardAdapter struct {
	getClient func(txtID string) *wa.Client
}

// NewSessionGuardAdapter cria o adapter com a função de lookup.
// O parâmetro getClient é tipicamente clientManager.GetWhatsmeowClient.
func NewSessionGuardAdapter(getClient func(txtID string) *wa.Client) *SessionGuardAdapter {
	return &SessionGuardAdapter{getClient: getClient}
}

// EnsureSession reporta se há cliente whatsmeow para txtID, sem devolvê-lo.
func (a *SessionGuardAdapter) EnsureSession(_ context.Context, txtID string) error {
	if a.getClient(txtID) == nil {
		return ErrNoSession(txtID, nil)
	}
	return nil
}

// Verificação em tempo de compilação de que o adapter implementa a porta.
var _ appport.SessionGuard = (*SessionGuardAdapter)(nil)
