package core

// Parametros numericos do caminho de recibo. Tags e atributos do XML binario
// usados uma unica vez no ponto onde o no e' montado continuam literais — ver
// PATCHES.md (Fase E, lotes 1-4).
const (
	// activeDeliveryReceiptsOff e activeDeliveryReceiptsForced sao dois dos tres
	// estados do contador sendActiveReceipts: 0 = envia recibo type="inactive",
	// 2 = envia recibo ativo mesmo sem presenca marcada
	// (SetForceActiveDeliveryReceipts). O estado intermediario 1 e' escrito por
	// SendPresence em presence.go, que faz CompareAndSwap(0,1)/(1,0) — ou seja
	// o forcado (2) nao e' desligado por presenca, so' por
	// SetForceActiveDeliveryReceipts(false).
	activeDeliveryReceiptsOff    uint32 = 0
	activeDeliveryReceiptsForced uint32 = 2

	// ackNoError e' o valor de `error` que faz sendAck omitir o atributo
	// `error` do no <ack>. Os codigos diferentes de zero sao os Nack* logo
	// abaixo, em receipt.go.
	ackNoError = 0
)
