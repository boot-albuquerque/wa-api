package retry

import (
	"context"
	"time"

	"wa-api/internal/wa-noise/protocol/types"
)

// CancelDelayedFromPhone cancela o pedido adiado de reenvio da mensagem msgID,
// se houver um em andamento. E' chamado quando a mensagem finalmente chega pelo
// caminho normal.
//
// A guarda de configuracao vem antes de qualquer toque no estado, como no
// original: com o reenvio desligado nunca houve pedido para cancelar.
func CancelDelayedFromPhone(t Transport, msgID types.MessageID) {
	if !t.RerequestFromPhoneEnabled() {
		return
	}
	t.State().CancelPendingPhone(msgID)
}

// DelayedRequestFromPhone espera RerequestDelay e, se ninguem cancelar nesse
// meio tempo, pede ao proprio telefone o reenvio da mensagem.
//
// Bloqueia — o call site original a chamava com `go`. Um segundo pedido para o
// mesmo ID enquanto o primeiro espera e' descartado (RegisterPendingPhone
// devolve false).
//
// Detalhe do upstream preservado: o `defer cancel()` e' registrado ANTES de o
// cancel entrar no mapa, entao ele dispara na saida da funcao mesmo no caminho
// feliz. Isso e' inofensivo porque o contexto ja' cumpriu seu papel, mas
// significa que o ctx passado adiante para RequestUnavailableMessage e' o mesmo
// que sera' cancelado na saida.
func DelayedRequestFromPhone(t Transport, info MessageRef) {
	if !t.RerequestFromPhoneEnabled() {
		return
	}
	ctx, cancel := context.WithCancel(t.BackgroundCtx())
	defer cancel()
	if !t.State().RegisterPendingPhone(info.ID, cancel) {
		return
	}
	defer t.State().UnregisterPendingPhone(info.ID)
	select {
	case <-time.After(t.RerequestDelay()):
	case <-ctx.Done():
		t.Log().Debugf("Cancelled delayed request for message %s from phone", info.ID)
		return
	}
	ImmediateRequestFromPhone(ctx, t, info)
}

// ImmediateRequestFromPhone pede agora ao proprio telefone o reenvio da
// mensagem.
//
// Nao checa RerequestFromPhoneEnabled de proposito: o caminho de decriptacao
// tambem a chama diretamente, sem essa condicao, quando SynchronousAck esta'
// ligado. Comportamento do upstream.
func ImmediateRequestFromPhone(ctx context.Context, t Transport, info MessageRef) {
	err := t.RequestUnavailableMessage(ctx, info.Chat, info.Sender, info.ID)
	if err != nil {
		t.Log().Warnf("Failed to send request for unavailable message %s to phone: %v", info.ID, err)
	} else {
		t.Log().Debugf("Requested message %s from phone", info.ID)
	}
}

// ClearDelayedRequests cancela todos os pedidos adiados pendentes. Chamado na
// desconexao.
func ClearDelayedRequests(t Transport) {
	t.State().CancelAllPendingPhone()
}

// MessageRef e' a fatia de types.MessageInfo que este dominio usa. Existe para
// que as assinaturas nao arrastem o MessageInfo inteiro (que traz
// MessageSource, tipos de midia e outros campos irrelevantes aqui) e para que
// os testes montem o caso em uma linha.
//
// Type e IsFromMe so' sao lidos por SendReceipt, para marcar o recibo de uma
// mensagem de par (peer_msg) com category="peer".
type MessageRef struct {
	Chat     types.JID
	Sender   types.JID
	ID       types.MessageID
	Type     string
	IsFromMe bool
}
