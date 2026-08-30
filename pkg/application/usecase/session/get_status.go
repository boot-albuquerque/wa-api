package session

import (
	"context"
	"fmt"
	"strconv"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// GetStatusUseCase encapsula a validação e leitura do status de sessão.
type GetStatusUseCase struct {
	status appport.SessionStatusReader
	users  appport.UserRepository
	logger appport.Logger
}

// NewGetStatusUseCase cria uma nova instância do usecase.
//
// NÃO recebe SessionGuard, e isso é deliberado desde a F196: um construtor que
// aceitasse a porta sem a usar convidaria o próximo a religá-la, que é o
// defeito de volta.
func NewGetStatusUseCase(status appport.SessionStatusReader, users appport.UserRepository, l appport.Logger) *GetStatusUseCase {
	return &GetStatusUseCase{
		status: status,
		users:  users,
		logger: l,
	}
}

// Execute valida se o cliente está disponível e devolve o status ao vivo da
// sessão (connected/loggedIn, via noise) somado ao registro persistido
// (jid, webhook, qrcode, ...). Antes, Execute só validava a sessão e devolvia
// GetStatusResult{} vazio — todo caller via connected=false/loggedIn=false
// sempre, mesmo com a sessão pareada; um adapter cliente que dependa deste
// endpoint para detectar a transição QR→autenticado nunca via a mudança.
func (uc *GetStatusUseCase) Execute(ctx context.Context, txtID string) (*domain.GetStatusResult, error) {
	// F196: NÃO há guarda de sessão aqui, e a ausência é o conserto.
	//
	// EnsureSession exige um cliente VIVO no registry. Com ela, perguntar o
	// estado de uma sessão desconectada devolvia 400 "no session" — a resposta
	// mais inútil possível no único momento em que alguém pergunta o estado:
	// quando a sessão NÃO está de pé. Pior, era incoerente: GET
	// /session/connect com o MESMO token respondia 200, porque connect é
	// justamente quem cria o cliente e não passa pela guarda.
	//
	// SessionStatus já devolve (false, false) quando não há cliente
	// (runtime/session/guard.go:74), portanto "desconectada" tem
	// representação. Quem decide se a sessão EXISTE é o registo no banco,
	// abaixo — que é a pergunta certa.
	connected, loggedIn := uc.status.SessionStatus(ctx, txtID)

	entries, err := uc.users.ListUsers(ctx, txtID)
	if err != nil {
		// Ver F90: cancelamento do cliente não é erro do servidor.
		if apperr.IsClientGaveUp(err) {
			uc.logger.Info(ctx, "session status read abandoned by the client", "txtID", txtID)
		} else {
			uc.logger.Error(ctx, "failed to read session record", "txtID", txtID, "error", err)
		}
		return nil, fmt.Errorf("database error: %w", err)
	}
	if len(entries) == 0 {
		uc.logger.Error(ctx, "no user record for session", "txtID", txtID)
		return nil, apperr.New("no_session", apperr.CategoryValidation, "no session", false, nil)
	}
	entry := entries[0]

	uc.logger.Info(ctx, "get status validated", "txtID", txtID, "connected", connected, "loggedIn", loggedIn)
	return &domain.GetStatusResult{
		ID:        entry.ID,
		Name:      entry.Name,
		Connected: connected,
		LoggedIn:  loggedIn,
		Jid:       entry.JID,
		Webhook:   entry.Webhook,
		Events:    entry.Events,
		ProxyURL:  entry.ProxyURL,
		Qrcode:    entry.QRCode,
		History:   strconv.Itoa(entry.History),
		ProxyConfig: domain.ProxySummary{
			Enabled:  entry.HasProxyURL,
			ProxyURL: entry.ProxyURL,
		},
		S3Config: domain.S3Summary{
			Enabled:       entry.S3.Enabled,
			Endpoint:      entry.S3.Endpoint,
			Region:        entry.S3.Region,
			Bucket:        entry.S3.Bucket,
			PathStyle:     entry.S3.PathStyle,
			PublicURL:     entry.S3.PublicURL,
			MediaDelivery: entry.S3.MediaDelivery,
			RetentionDays: entry.S3.RetentionDays,
		},
	}, nil
}
