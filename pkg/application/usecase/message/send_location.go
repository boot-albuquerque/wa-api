package message

import (
	"context"
	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SendLocationUseCase envia uma localização de verdade: monta o
// LocationMessage a partir dos campos escalares do request (sem upload, sem
// fetch, sem conversão) e o envia pelo wa-noise. Só devolve
// domain.StatusSent depois que o envio retorna sucesso — nunca antes (mesma
// disciplina de SendMessageUseCase, CAP-01).
type SendLocationUseCase struct {
	messages appport.SimpleMessenger
	jids     appport.JIDResolver
	logger   appport.Logger
}

// NewSendLocationUseCase cria uma nova instância do usecase.
func NewSendLocationUseCase(sm appport.SimpleMessenger, jr appport.JIDResolver, l appport.Logger) *SendLocationUseCase {
	return &SendLocationUseCase{
		messages: sm,
		jids:     jr,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios, resolve o destinatário e envia a
// localização pela porta de verdade.
//
// A validação é `== nil`, e não `== 0` (F121, corrigido em 2026-08-22).
//
// O comportamento HISTÓRICO (`git show 41bc8e2^:handlers.go`, linha ~1913)
// validava `== 0` e por isso confundia "campo ausente" com "valor zero": um
// ponto sobre o equador ou o meridiano de Greenwich era recusado com "missing
// Latitude". Zero é coordenada válida.
//
// A correção só AMPLIA: omitir o campo continua a dar 400; mandar 0 passa a ser
// aceite. Nenhum cliente perde comportamento — foi isso que a tornou barata.
func (uc *SendLocationUseCase) Execute(ctx context.Context, txtID string, req domain.SendLocationRequest) (*domain.SendLocationResult, error) {
	if req.Phone == "" {
		return nil, apperr.New("missing_phone", apperr.CategoryValidation, "missing Phone in payload", false, nil)
	}
	if req.Latitude == nil {
		return nil, apperr.New("missing_latitude", apperr.CategoryValidation, "missing Latitude in payload", false, nil)
	}
	if req.Longitude == nil {
		return nil, apperr.New("missing_longitude", apperr.CategoryValidation, "missing Longitude in payload", false, nil)
	}

	if err := uc.messages.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	recipient, err := uc.jids.ResolveJID(ctx, req.Phone)
	if err != nil {
		uc.logger.Warn(ctx, "invalid phone in send location payload", "txtID", txtID, "error", err)
		return nil, apperr.New("invalid_phone", apperr.CategoryValidation, "could not parse Phone", false, nil)
	}

	payload := domain.LocationPayload{Latitude: *req.Latitude, Longitude: *req.Longitude, Name: req.Name}

	sent, err := uc.messages.SendLocation(ctx, txtID, recipient, payload, req.ReplyTo, req.ID)
	if err != nil {
		uc.logger.Error(ctx, "failed to send location message", "txtID", txtID, "error", err)
		return nil, err
	}

	result := &domain.SendLocationResult{
		MessageID: sent.ID,
		Timestamp: sent.Timestamp.Unix(),
		Status:    domain.StatusSent,
	}

	uc.logger.Info(ctx, "location sent", "msgID", result.MessageID)
	return result, nil
}
