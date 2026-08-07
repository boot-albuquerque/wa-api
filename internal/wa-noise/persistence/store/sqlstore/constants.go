package sqlstore

import "time"

// Tamanhos, em bytes, do material criptografico persistido. As colunas
// correspondentes tem CHECK() no schema, entao um valor diferente aqui
// significa banco corrompido — ver ErrInvalidLength.
const (
	// curve25519KeyLength e' o tamanho de uma chave Curve25519 (privada ou
	// publica) e tambem o de uma identity key remota.
	curve25519KeyLength = 32
	// signedPreKeySignatureLength e' o tamanho da assinatura Ed25519 da
	// signed pre key.
	signedPreKeySignatureLength = 64
	// appStateHashLength e' o tamanho do LTHash de app state
	// (appstate.HashState.Hash e' [128]byte).
	appStateHashLength = 128
	// advSecretKeyLength e' o tamanho do segredo ADV sorteado em NewDevice.
	advSecretKeyLength = 32
	// initialSignedPreKeyID e' o ID da primeira signed pre key de um device novo.
	initialSignedPreKeyID = 1
	// ciphertextHashLength e' o tamanho do SHA-256 usado como chave do buffer
	// de eventos. Coincide com curve25519KeyLength, mas e' outro conceito.
	ciphertextHashLength = 32
)

// Tamanhos de lote das escritas em massa. Ambos existem para nao estourar o
// limite de placeholders/parametros por statement dos drivers SQL.
const (
	// mutationBatchSize e' quantos MACs de mutacao de app state vao por INSERT.
	mutationBatchSize = 400
	// contactBatchSize e' quantos contatos (ou telefones redigidos) vao por
	// INSERT em massa.
	contactBatchSize = 300
)

// Sintaxe de placeholder por dialeto, usada ao montar INSERTs de aridade
// variavel. Postgres usa $N; o driver de SQLite exige ?N para reutilizar o
// mesmo parametro em varias linhas.
const (
	mutationMACPlaceholderPostgres = "($1, $2, $3, $%d, $%d)"
	mutationMACPlaceholderSQLite   = "(?1, ?2, ?3, ?%d, ?%d)"
	// privacyTokenValuesTemplate e' o bloco VALUES de linha unica que
	// PutPrivacyTokens substitui pela lista real de tokens.
	privacyTokenValuesTemplate = "($1, $2, $3, $4, $5)"
)

// Nomes de coluna de whatsmeow_chat_settings interpolados em
// putChatSettingQuery. Sao interpolados (e nao passados como parametro) porque
// SQL nao aceita nome de coluna parametrizado; por isso precisam ser literais
// controlados por nos, nunca entrada externa.
const (
	chatSettingColumnMutedUntil = "muted_until"
	chatSettingColumnPinned     = "pinned"
	chatSettingColumnArchived   = "archived"
)

// mutedForeverDBValue e' o sentinela gravado em muted_until para
// store.MutedForever. Negativo porque qualquer valor >= 0 e' lido como um
// timestamp Unix real em GetChatSettings.
const mutedForeverDBValue = -1

// Janelas de retencao dos buffers de evento.
const (
	// bufferedEventRetention: os servidores do WhatsApp so' bufferizam eventos
	// por 14 dias, entao hashes mais antigos que isso nunca serao reusados.
	bufferedEventRetention = 14 * 24 * time.Hour
	// outgoingEventRetention e' por quanto tempo uma mensagem enviada fica no
	// buffer de retry.
	outgoingEventRetention = 7 * 24 * time.Hour
)

// decryptionTxnCallerSkip e' o numero de frames que dbutil deve pular ao
// atribuir o chamador de uma transacao aberta por DoDecryptionTxn, para que o
// log aponte para quem chamou e nao para o wrapper.
const decryptionTxnCallerSkip = 2

// signalAddressWildcardSuffix e' o sufixo LIKE que casa todos os dispositivos
// de um mesmo usuario Signal (`<user>:<device>`).
const signalAddressWildcardSuffix = ":%"
