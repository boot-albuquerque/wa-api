package keys

// Constantes de tamanho e de clamping das chaves Curve25519, que antes
// apareciam como literais soltos.
const (
	// KeyLength e' o tamanho de uma chave Curve25519, publica ou privada.
	KeyLength = 32

	// SignatureLength e' o tamanho de uma assinatura XEdDSA.
	SignatureLength = 64

	// signedKeyLength e' o tamanho do buffer que Sign assina: a chave
	// publica precedida do byte de tipo de curva (ecc.DjbType). O
	// libsignal exige esse prefixo — assinar a chave crua produziria uma
	// assinatura que nenhum outro cliente valida.
	signedKeyLength = 1 + KeyLength

	// keyTypePrefixLength e' quantos bytes o prefixo de tipo ocupa nesse
	// buffer.
	keyTypePrefixLength = 1
)

// Clamping de Curve25519 (RFC 7748, secao 5): zera os tres bits baixos, zera o
// bit mais alto e liga o penultimo. Garante que a chave privada seja multipla
// do cofator e caia sempre na mesma faixa, o que barra ataques de subgrupo
// pequeno e de timing por tamanho variavel de escalar.
//
// Nao sao numeros arbitrarios: sao a mascara exigida pela curva, e mudar
// qualquer um deles produz chaves que os outros clientes rejeitam.
const (
	clampLowBitsMask   = 248 // 0b1111_1000: zera os 3 bits baixos do primeiro byte
	clampHighBitMask   = 127 // 0b0111_1111: zera o bit mais alto do ultimo byte
	clampSecondHighBit = 64  // 0b0100_0000: liga o penultimo bit do ultimo byte

	clampFirstByte = 0
	clampLastByte  = KeyLength - 1
)
