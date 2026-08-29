package appstate

import "fmt"

// O blob de uma SyncdValue vem desserializado de um patch de app state, ou
// seja, direto do servidor: e' entrada nao confiavel. O layout esperado e'
//
//	[ IV (16) | ciphertext (>=0) | value MAC (32) ]
//
// e o codigo corta as duas pontas por indice. Sem checar o tamanho antes,
// `blob[len(blob)-macLength:]` com um blob mais curto que 32 bytes derrubava a
// goroutine de app state por `slice bounds out of range` — F19 em
// HOUSEKEEP.md. As funcoes abaixo sao o unico caminho para esses cortes.

// trailingValueMAC devolve os ultimos macLength bytes do blob.
//
// what nomeia a origem do blob para que o erro diga qual mutacao chegou curta,
// e nao apenas que "algum" blob estava curto.
func trailingValueMAC(blob []byte, what string) ([]byte, error) {
	if len(blob) < macLength {
		return nil, fmt.Errorf("%w: %s tem %d bytes, minimo %d", ErrShortMutationBlob, what, len(blob), macLength)
	}
	return blob[len(blob)-macLength:], nil
}

// splitValueMAC separa o blob em (conteudo cifrado com IV, value MAC).
//
// Exige tambem o IV, porque quem chama corta `content[:cbcIVLength]` logo em
// seguida: validar so' o MAC aqui apenas mudaria o panic de lugar.
func splitValueMAC(blob []byte, what string) (content, valueMAC []byte, err error) {
	if len(blob) < macLength+cbcIVLength {
		return nil, nil, fmt.Errorf("%w: %s tem %d bytes, minimo %d (IV %d + MAC %d)",
			ErrShortMutationBlob, what, len(blob), macLength+cbcIVLength, cbcIVLength, macLength)
	}
	return blob[:len(blob)-macLength], blob[len(blob)-macLength:], nil
}
