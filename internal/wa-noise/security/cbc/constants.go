package cbcutil

// Constantes do modo CBC usado pela midia do WhatsApp, que antes apareciam
// como literais soltos.
//
// O que NAO esta aqui: aes.BlockSize, que ja' e' uma constante nomeada da
// stdlib e diz exatamente o que e'.
const (
	// streamBufferSize e' o tamanho do buffer de leitura de DecryptFile e
	// EncryptStream. E' multiplo de aes.BlockSize, o que e' obrigatorio:
	// CryptBlocks exige que o buffer seja multiplo do bloco, e um tamanho
	// que nao fosse quebraria a cifra no meio de um bloco.
	streamBufferSize = 32 * 1024

	// mediaMACLength e' quantos bytes do HMAC-SHA256 sao anexados ao fim do
	// arquivo cifrado. O WhatsApp trunca o MAC de 32 bytes para 10 — o
	// valor faz parte do protocolo de midia, nao e' escolha nossa.
	mediaMACLength = 10
)
