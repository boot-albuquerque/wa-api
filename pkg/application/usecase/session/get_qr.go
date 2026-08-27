package session

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetQRUseCase encapsula a validação e leitura do QR code de pareamento.
type GetQRUseCase struct {
	qr     appport.PairingQRReader
	logger appport.Logger
}

// NewGetQRUseCase cria uma nova instância do usecase.
//
// Até 2026-08-27 recebia (SessionGuard, UserRepository) e lia `users.qrcode`
// aqui dentro. A leitura passou para trás de appport.PairingQRReader, que é uma
// porta POR ENGINE: a coluna só tem valor porque o listener de QR de UM engine a
// escreve, e um engine sem esse listener respondia 200 com código vazio para
// sempre. Ver o comentário da porta e HOUSEKEEP F273.
func NewGetQRUseCase(qr appport.PairingQRReader, l appport.Logger) *GetQRUseCase {
	return &GetQRUseCase{
		qr:     qr,
		logger: l,
	}
}

// Execute valida se o cliente está disponível e devolve o QR code que o engine
// desta sessão tem em oferta.
//
// A ordem é carga: EnsureSession ANTES da leitura. Ela é a mesma de antes da
// migração para a porta, e inverter as duas devolveria um QR para uma sessão que
// o transporte não serve — o teste de sucesso continuaria passando.
func (uc *GetQRUseCase) Execute(ctx context.Context, txtID string) (*domain.GetQRResult, error) {
	if err := uc.qr.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no session for QR read", "txtID", txtID, "error", err)
		return nil, err
	}

	code, err := uc.qr.PairingQR(ctx, txtID)
	if err != nil {
		uc.logger.Error(ctx, "failed to read QR code", "txtID", txtID, "error", err)
		return nil, err
	}

	uc.logger.Info(ctx, "get QR validated", "txtID", txtID, "hasQR", code != "")
	return &domain.GetQRResult{QRCode: code}, nil
}
