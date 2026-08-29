package core

import "wa-api/internal/noise/capabilities/retry"

// Apelidos das constantes de politica de retry, que moraram aqui ate' a Fase
// F/G lote 5 e hoje vivem em internal/wa-noise/retry.
//
// Continuam existindo na raiz porque o dominio de retry NAO e' o unico a
// cita-las: maxOutgoingRetryReceipts e' lida por message_decrypt.go ao decidir
// se ainda vale pedir retry, e os nomes curtos deixam esse call site legivel.
// Os valores sao os mesmos objetos, nao copias.
const (
	maxIncomingRetryRequests        = retry.MaxIncomingRequests
	minRetryCountForSessionRecreate = retry.MinCountForSessionRecreate
	maxOutgoingRetryReceipts        = retry.MaxOutgoingReceipts
	retryReceiptVersion             = retry.ReceiptVersion

	retryStoreFormatWA = retry.StoreFormatWA
	retryStoreFormatFB = retry.StoreFormatFB

	retryStoreClearInterval = retry.StoreClearInterval

	// recentMessagesSize continua citada por client_test.go e pelo gate de
	// tamanho do buffer.
	recentMessagesSize = retry.RecentMessagesSize
)
