package message

import (
	"sync/atomic"
	"time"

	"wa-api/internal/wa-noise/protocol/proto/waE2E"
)

// HistorySyncQueue e' o estado do loop de history sync: a fila de notificacoes
// e o flag de "loop rodando".
//
// Antes da extracao eram dois campos soltos de *Client
// (`historySyncNotifications` e `historySyncHandlerStarted`). Sao um tipo
// proprio agora porque so' fazem sentido juntos: o flag existe para garantir
// que haja NO MAXIMO um consumidor da fila, e o encerramento do loop precisa
// reexaminar a fila depois de zerar o flag (ver HandleHistorySyncNotificationLoop).
// Separados, nada impedia um chamador de mexer em um sem o outro.
//
// O ponteiro precisa ser ESTAVEL: o tipo contem um atomic.Bool, que nao pode
// ser copiado por valor. Por isso Transport.HistorySync devolve *HistorySyncQueue.
//
// A sincronizacao NAO mudou de forma na extracao: continua sendo um canal com
// buffer mais um atomic.Bool com CompareAndSwap, exatamente nos mesmos pontos.
type HistorySyncQueue struct {
	notifications chan *waE2E.HistorySyncNotification
	handlerActive atomic.Bool
}

// NewHistorySyncQueue cria a fila com o tamanho de buffer dado.
func NewHistorySyncQueue(bufferSize int) *HistorySyncQueue {
	return &HistorySyncQueue{
		notifications: make(chan *waE2E.HistorySyncNotification, bufferSize),
	}
}

// Cap e' a capacidade do buffer da fila. Existe para o teste de construcao do
// cliente, na raiz, poder conferir que o buffer foi dimensionado.
func (q *HistorySyncQueue) Cap() int { return cap(q.notifications) }

// Len e' quantas notificacoes estao enfileiradas agora.
func (q *HistorySyncQueue) Len() int { return len(q.notifications) }

// Ready diz se a fila foi inicializada. Usado pelo teste de construcao do
// cliente na raiz, que antes comparava o canal com nil.
func (q *HistorySyncQueue) Ready() bool { return q != nil && q.notifications != nil }

// DecryptBufferState guarda quando o buffer de eventos decifrados foi limpo
// pela ultima vez.
//
// Antes da extracao era o campo `lastDecryptedBufferClear time.Time` de
// *Client, lido e escrito SEM sincronizacao dentro de decryptMessages. A
// extracao preservou isso literalmente — campo simples, sem mutex — porque
// introduzir sincronizacao aqui mudaria comportamento em um caminho que este
// lote se comprometeu a nao mexer. Ver PATCHES.md, lote 9, "O que NAO foi
// extraido, e por que": a corrida potencial esta' registrada, nao corrigida.
//
// O ponteiro precisa ser estavel porque o campo e' escrito no lugar.
type DecryptBufferState struct {
	lastClear time.Time
}

// ShouldClear diz se ja' passou tempo suficiente desde a ultima limpeza e, em
// caso afirmativo, marca agora como a nova ultima limpeza.
//
// Os dois passos ficam juntos porque no original eram duas linhas coladas
// (`if time.Since(...) > interval && ctx.Err() == nil { cli.last... = time.Now() ... }`);
// a checagem de contexto continua sendo do chamador, que e' quem tem o ctx.
func (s *DecryptBufferState) ShouldClear() bool {
	if time.Since(s.lastClear) <= decryptedBufferClearInterval {
		return false
	}
	s.lastClear = time.Now()
	return true
}
