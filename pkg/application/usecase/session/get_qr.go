package session

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/qrimage"
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
// desta sessão tem em oferta, SEMPRE como a imagem em data URI que a rota
// promete.
//
// A ordem é carga: EnsureSession ANTES da leitura. Ela é a mesma de antes da
// migração para a porta, e inverter as duas devolveria um QR para uma sessão que
// o transporte não serve — o teste de sucesso continuaria passando.
//
// # Por que a normalização acontece AQUI, e não em cada adapter
//
// Os dois engines entregam formas diferentes pela mesma porta — wa_noise já
// tem a imagem (o orquestrador escreve o PNG em users.qrcode), wa_headless só
// tem a string crua que leu da página. As duas são `string`, no mesmo campo
// `qr_code`, então nada no tipo, na porta ou em domain.GetQRResult conseguia
// distingui-las, e a divergência entrou em produção sem sinal nenhum.
//
// O custo foi medido na F373: um consumidor a quem se disse "os dois engines
// respondem a mesma forma" desenhou o que recebeu como PAYLOAD de QR, e para
// wa_noise isso deu um código impecável codificando os 1858 caracteres do
// próprio data URI — o telefone lê, o WhatsApp recusa. Nada no console
// acusava nada.
//
// Este é o ÚNICO ponto por onde os dois engines passam, e é por isso que a
// garantia mora aqui: um engine novo herda-a sem precisar de saber que ela
// existe, que é exatamente o que faltou quando wa_headless chegou.
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

	// Depois da leitura ter dado certo, e nunca antes: uma falha de
	// codificação é uma falha de RESPOSTA, e reportá-la como código vazio
	// diria "ainda não há QR" para uma sessão que tem um.
	image, err := qrimage.EnsureDataURI(code)
	if err != nil {
		uc.logger.Error(ctx, "failed to render QR image", "txtID", txtID, "error", err)
		return nil, err
	}

	uc.logger.Info(ctx, "get QR validated", "txtID", txtID, "hasQR", image != "")
	return &domain.GetQRResult{QRCode: image}, nil
}
