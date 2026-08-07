package prekeys

import "time"

// Politica de contagem de prekeys.
const (
	// WantedCount e' quantas prekeys o cliente envia ao servidor num lote
	// normal. Era WantedPreKeyCount na raiz; o valor nao mudou.
	WantedCount = 50
	// MinCount e' o piso a partir do qual um novo lote e' enviado. Era
	// MinPreKeyCount na raiz; o valor nao mudou.
	MinCount = 5

	// InitialCount e' quantas prekeys sao enviadas no primeiro upload apos
	// o pareamento. E deliberadamente muito maior que WantedCount para
	// que a conta nasca com estoque suficiente para varias sessoes novas.
	InitialCount = 812

	// UploadDebounce e' a janela em que um segundo pedido de upload
	// reconfere a contagem no servidor antes de gerar chaves de novo, para
	// evitar a corrida de dois uploads simultaneos.
	UploadDebounce = 10 * time.Minute
)

// Tamanhos de wire das chaves do protocolo Signal, conforme o servidor os envia
// e espera. Toda leitura de no de prekey valida contra estes valores.
const (
	// RegistrationIDLength e' o registration ID em big-endian (uint32).
	// Exportada porque retry_receipt_send.go (raiz) codifica o mesmo campo
	// (Store.RegistrationID) com a mesma codificacao e reusa esta constante em
	// vez de declarar um segundo 4.
	RegistrationIDLength = 4

	// idLength e' o key ID no wire: 3 bytes, ou seja um uint32 truncado
	// para 24 bits. ToNode corta o byte mais significativo ao escrever e
	// NodeToPreKey o reintroduz zerado ao ler.
	idLength = 3
	// idPadLength e' quantos bytes zerados NodeToPreKey prefixa para
	// remontar o uint32 de 4 bytes a partir dos idLength do wire.
	idPadLength = RegistrationIDLength - idLength

	// pubLength e' o tamanho de uma chave publica Curve25519.
	pubLength = 32
	// signatureLength e' o tamanho de uma assinatura Ed25519 da signed prekey.
	signatureLength = 64
)
