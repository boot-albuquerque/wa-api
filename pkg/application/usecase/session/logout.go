package session

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// LogoutUseCase DESVINCULA o aparelho: a sessão some de "Aparelhos
// conectados" no celular e um novo pareamento por QR passa a ser necessário.
// É irreversível sem o telefone à mão — distinto de DisconnectUseCase, que
// só derruba o transporte.
//
// A porta é SessionController, e não SessionGuard, pelo mesmo motivo da
// F79: até então este use case só conseguia validar, e devolvia 200 sem
// desvincular coisa alguma.
type LogoutUseCase struct {
	sessions appport.SessionLogouter
	detacher appport.SessionDetacher
	logger   appport.Logger
}

// NewLogoutUseCase cria uma nova instância do usecase.
func NewLogoutUseCase(sc appport.SessionLogouter, d appport.SessionDetacher, l appport.Logger) *LogoutUseCase {
	return &LogoutUseCase{
		sessions: sc,
		detacher: d,
		logger:   l,
	}
}

// Execute encerra a autenticação da sessão de txtID no WhatsApp.
func (uc *LogoutUseCase) Execute(ctx context.Context, txtID string, req domain.LogoutRequest) (*domain.LogoutResult, error) {
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	if err := uc.sessions.Logout(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "logout failed", "txtID", txtID, "error", err)
		return nil, err
	}

	// Solta a sessão DEPOIS do logout bem-sucedido (F80).
	//
	// O logout iniciado pelo TELEFONE emite *events.LoggedOut, que aciona o
	// kill-channel e tira o cliente dos registries. O iniciado pela API não
	// emite evento nenhum: o store é apagado, mas o cliente continua
	// registrado com estado em memória obsoleto, e /session/status seguia
	// respondendo loggedIn=true para uma sessão que já não existia.
	//
	// Detach é idempotente e é o único escritor de users.connected neste
	// caminho, então chamá-lo aqui alinha os dois fluxos sem duplicar
	// escrita: o do telefone continua passando pelo kill-channel, este passa
	// direto, e ambos terminam no mesmo lugar.
	uc.detacher.Detach(txtID)

	uc.logger.Info(ctx, "logged out", "txtID", txtID)
	return &domain.LogoutResult{}, nil
}
