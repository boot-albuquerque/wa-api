package pairing

// Constantes do fluxo de pareamento (QR e codigo de telefone). Vieram de
// pair_constants.go na raiz; os valores nao mudaram, so' o prefixo `pair`
// caiu, ja' que agora e' o nome do pacote.
const (
	// qrDataFormat monta o payload do QR code lido pelo celular:
	// ref, chave Noise publica, chave de identidade publica, adv secret e tipo de cliente.
	qrDataFormat = "https://wa.me/settings/linked_devices#%s,%s,%s,%s,%s"

	// mainDeviceID e' o ID de dispositivo do aparelho principal (o celular)
	// dentro de um LID: o companion sempre tem device != 0.
	mainDeviceID = 0

	// identityKeyLength e' o tamanho de uma chave publica Curve25519 de identidade.
	identityKeyLength = 32
)

// Codigos e textos do no <error> devolvido ao servidor quando o pareamento falha.
// Os codigos espelham status HTTP por convencao do protocolo do WhatsApp.
const (
	errCodeInternal     = 500
	errCodeUnauthorized = 401

	errTextInternal          = "internal-error"
	errTextHMACMismatch      = "hmac-mismatch"
	errTextSignatureMismatch = "signature-mismatch"
)

// Parametros criptograficos do pareamento por codigo de telefone (link code).
const (
	// codePBKDF2Iterations e' o custo do PBKDF2 que deriva a chave de cifragem
	// a partir do codigo de 8 caracteres. Escrito como 2<<16 no upstream.
	codePBKDF2Iterations = 2 << 16

	// codeSaltLength, codeIVLength e codeKeyLength descrevem o blob
	// de 80 bytes trocado nos dois sentidos: salt || IV || pubkey cifrada.
	codeSaltLength = 32
	codeIVLength   = 16
	codeKeyLength  = 32

	// Offsets derivados dos comprimentos acima, para que o fatiamento do blob
	// continue sendo [0:32], [32:48], [48:80] sem literal solto.
	codeSaltEnd       = codeSaltLength
	codeIVEnd         = codeSaltEnd + codeIVLength
	codeWrappedKeyEnd = codeIVEnd + codeKeyLength

	// codeRawLength e' o numero de bytes aleatorios que viram o codigo de
	// pareamento; 5 bytes = 8 caracteres no base32 customizado do WhatsApp.
	codeRawLength = 5
	// codeGroupLength e' onde o codigo exibido ao usuario recebe o hifen.
	codeGroupLength = 4

	// codeAdvSecretRandomLength e codeKeyBundleSaltLength sao entradas
	// aleatorias do bundle de chaves enviado ao aparelho principal.
	codeAdvSecretRandomLength = 32
	codeKeyBundleSaltLength   = 32
	// codeKeyBundleNonceLength e' o nonce do AES-GCM que cifra o bundle.
	codeKeyBundleNonceLength = 12
	// codeKeyBundleKeyLength e' o tamanho da chave AES derivada por HKDF.
	codeKeyBundleKeyLength = 32
	// codeAdvSecretLength e' o tamanho do adv secret derivado por HKDF.
	codeAdvSecretLength = 32

	// codePhoneMinLength e' o menor comprimento aceito para o numero de
	// telefone ja' limpo de nao-digitos (o upstream recusa `len <= 6`).
	codePhoneMinLength = 7
	// codePhoneTrunkPrefix indica numero nacional, nao internacional.
	codePhoneTrunkPrefix = "0"
)

// Rotulos HKDF do pareamento por codigo. Sao strings de dominio do protocolo:
// mudar qualquer byte quebra a compatibilidade com o aparelho principal.
const (
	codeKeyBundleHKDFInfo = "link_code_pairing_key_bundle_encryption_key"
	codeAdvSecretHKDFInfo = "adv_secret"
)

// codeBase32Alphabet e' o alfabeto base32 customizado do WhatsApp para o
// codigo de pareamento: sem 0/O, 1/I e U, para evitar leitura ambigua.
const codeBase32Alphabet = "123456789ABCDEFGHJKLMNPQRSTVWXYZ"
