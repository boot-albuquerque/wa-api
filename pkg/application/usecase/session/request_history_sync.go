package session

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// defaultHistorySyncCount é o tamanho de lote recomendado pela própria
// biblioteca ("The recommended number of messages to request at a time is 50",
// message_builders.go:63). Vira constante em vez de literal porque atravessa a
// fronteira HTTP: é o valor que a rota usa quando o cliente não pede nenhum.
const defaultHistorySyncCount = 50

// RequestHistorySyncUseCase pede ao dispositivo principal as mensagens
// anteriores a uma âncora conhecida.
type RequestHistorySyncUseCase struct {
	history appport.HistorySyncRequester
	logger  appport.Logger
}

// NewRequestHistorySyncUseCase cria uma nova instância do usecase.
func NewRequestHistorySyncUseCase(h appport.HistorySyncRequester, l appport.Logger) *RequestHistorySyncUseCase {
	return &RequestHistorySyncUseCase{history: h, logger: l}
}

// Execute pede o histórico anterior à âncora recebida.
//
// F198: até 2026-08-21 este Execute apenas validava a sessão, logava
// "request history sync validated" e devolvia um resultado VAZIO com 200 —
// nenhum pedido era feito, e o cliente não tinha como distinguir isso de
// sucesso.
func (uc *RequestHistorySyncUseCase) Execute(ctx context.Context, txtID string, req domain.RequestHistorySyncRequest) (*domain.RequestHistorySyncResult, error) {
	if err := uc.history.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no noise session", "txtID", txtID, "error", err)
		return nil, err
	}

	// A âncora é obrigatória e a recusa é explícita: sem ela o protocolo não
	// tem "antes de quê" para responder. Aceitar o pedido sem âncora seria
	// reproduzir a F198 com mais passos — 200 e nada a acontecer.
	if req.ChatJid == "" {
		uc.logger.Warn(ctx, "history sync request refused: no chat_jid", "txtID", txtID)
		return nil, apperr.New("missing_chat_jid", apperr.CategoryValidation,
			"chat_jid é obrigatório: o pedido é por mensagens ANTERIORES a uma conhecida", false, nil)
	}
	if req.OldestMsgID == "" {
		uc.logger.Warn(ctx, "history sync request refused: no anchor message", "txtID", txtID, "chat_jid", req.ChatJid)
		return nil, apperr.New("missing_oldest_msg_id", apperr.CategoryValidation,
			"oldest_msg_id é obrigatório: é a âncora a partir da qual se pede para trás", false, nil)
	}

	count := req.Count
	if count <= 0 {
		count = defaultHistorySyncCount
	}

	anchor := appport.HistoryAnchor{
		ChatJID:   domain.JID(req.ChatJid),
		MessageID: req.OldestMsgID,
		FromMe:    req.OldestMsgFromMe,
		Timestamp: req.OldestMsgTimestamp,
	}

	id, err := uc.history.RequestHistorySync(ctx, txtID, anchor, count)
	if err != nil {
		uc.logger.Error(ctx, "history sync request failed", "txtID", txtID, "error", err)
		return nil, err
	}

	uc.logger.Info(ctx, "history sync requested", "txtID", txtID, "count", count, "request_id", id)

	// Details carrega o id do PEDIDO, não a contagem do que virá: a resposta
	// chega depois, como evento HistorySync ON_DEMAND. Preencher Count com o
	// que foi PEDIDO seria dizer ao cliente que já recebeu.
	return &domain.RequestHistorySyncResult{
		Details:            id,
		Count:              count,
		ChatJid:            req.ChatJid,
		OldestMsgID:        req.OldestMsgID,
		OldestMsgFromMe:    req.OldestMsgFromMe,
		OldestMsgTimestamp: req.OldestMsgTimestamp,
	}, nil
}
