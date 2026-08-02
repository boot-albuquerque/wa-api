package whatsmeow

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain/apperr"

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
	getClient waClientGetter
}

// NewSessionGuardAdapter cria o adapter com a função de lookup.
// O parâmetro getClient é tipicamente clientManager.GetWhatsmeowClient
// (convertido via clientForGetter).
func NewSessionGuardAdapter(getClient waClientGetter) *SessionGuardAdapter {
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

// SessionStatus devolve se a sessão de userID está conectada e autenticada.
//
// Substitui a interface ClientManagerAdapter que list_users.go declarava
// localmente, cujo GetWhatsmeowClient(id) devolvia interface{} — segundo o
// comentário do próprio arquivo, "to avoid circular deps" — apenas para ser
// comparado com nil antes das duas chamadas seguintes. Aqui o cliente não
// atravessa a fronteira.
func (a *SessionGuardAdapter) SessionStatus(_ context.Context, userID string) (bool, bool) {
	client := a.getClient(userID)
	if client == nil {
		return false, false
	}
	return client.IsConnected(), client.IsLoggedIn()
}

// Verificação em tempo de compilação da porta de status de sessão.
var _ appport.SessionStatusReader = (*SessionGuardAdapter)(nil)

// Logout encerra a autenticação da sessão.
//
// Preservado literalmente de DeleteUserCompleteUseCase: o Logout era chamado
// com context.Background(), e não com o contexto da requisição. Trocar por ctx
// é correção de lógica, não movimento — follow-up nomeado.
func (a *SessionGuardAdapter) Logout(_ context.Context, txtID string) error {
	client := a.getClient(txtID)
	if client == nil {
		return ErrNoSession(txtID, nil)
	}
	return client.Logout(context.Background())
}

// Disconnect derruba o transporte da sessão.
func (a *SessionGuardAdapter) Disconnect(_ context.Context, txtID string) error {
	client := a.getClient(txtID)
	if client == nil {
		return ErrNoSession(txtID, nil)
	}
	client.Disconnect()
	return nil
}

// Verificação em tempo de compilação da porta de controle de sessão.
var _ appport.SessionController = (*SessionGuardAdapter)(nil)
