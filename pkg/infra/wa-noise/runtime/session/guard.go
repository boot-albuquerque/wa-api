package session

import (
	"context"
	"errors"

	wanoise "wa-api/internal/wa-noise"
	waclient "wa-api/pkg/infra/wa-noise/client"

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
	getClient waclient.Getter
}

// NewSessionGuardAdapter cria o adapter com a função de lookup.
// O parâmetro getClient é tipicamente clientManager.GetWaNoiseClient
// (convertido via clientForGetter).
func NewSessionGuardAdapter(getClient waclient.Getter) *SessionGuardAdapter {
	return &SessionGuardAdapter{getClient: getClient}
}

// Client devolve o cliente da sessão txtID ou o erro tipado de sessão
// ausente. É o ponto único por onde os adapters de capacidade (group/, user/,
// chat/, misc/, presence/) resolvem a sessão: antes cada um repetia o par
// lookup-mais-nil-check, e o getter em si era um campo não exportado — o que
// deixou de ser viável quando esses adapters passaram a viver em subpacotes
// próprios.
func (a *SessionGuardAdapter) Client(txtID string) (waclient.Client, error) {
	client := a.getClient(txtID)
	if client == nil {
		return nil, ErrNoSession(txtID, nil)
	}
	return client, nil
}

// EnsureSession reporta se há cliente wa-noise para txtID, sem devolvê-lo.
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
// localmente, cujo GetWaNoiseClient(id) devolvia interface{} — segundo o
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

	// Logout FALA com o WhatsApp: manda um IQ `remove-companion-device` antes
	// de qualquer coisa. Sem transporte vivo não há o que enviar, e o SDK
	// devolve "error sending logout request: websocket not connected" — erro
	// CRU, que a fronteira HTTP transformava em 500 opaco (F93).
	//
	// A checagem é de ESTADO, não de texto de erro: casar a mensagem do SDK
	// quebra silenciosamente quando ele muda a frase, e essa é a classe de
	// acoplamento que só falha em produção.
	//
	// 409 e não 400: a requisição está correta, só não pode ser atendida NESTE
	// estado. A mensagem diz o caminho de saída, porque a resposta anterior não
	// dizia — o remédio é reconectar antes, e isso era conhecimento de
	// implementação.
	if !client.IsConnected() {
		return apperr.New(
			apperr.CodeSessionNotConnected,
			apperr.CategoryConflict,
			"session has no live connection; call /session/connect before logging out",
			false,
			nil,
		)
	}

	err := client.Logout(context.Background())

	// Transporte vivo mas NUNCA emparelhado: o store não tem device JID, e o
	// SDK devolve a sentinela crua wanoise.ErrNotLoggedIn (F275). Igual ao
	// ramo acima, a checagem é por ESTADO (errors.Is contra a sentinela
	// reexportada em internal/wa-noise/main.go), não por texto — a mesma
	// regra que o comentário logo acima já enuncia.
	//
	// 409 e não 500: também aqui a requisição está correta, só não pode ser
	// atendida NESTE estado — desta vez porque não há o que desemparelhar, e
	// não porque falta transporte.
	if errors.Is(err, wanoise.ErrNotLoggedIn) {
		return apperr.New(
			apperr.CodeSessionNotPaired,
			apperr.CategoryConflict,
			"session has a live connection but was never paired; there is no device to log out",
			false,
			err,
		)
	}

	return err
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
