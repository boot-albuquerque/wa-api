package send

// Nove constantes deste arquivo sao EXPORTADAS. Nao e' API nova: sao exatamente
// as que a raiz continua usando fora do caminho de envio (message_decrypt.go,
// message_decrypt_session.go, message.go, msgsecret_poll.go). A raiz as
// reexporta com os nomes historicos e minusculos em send_constants.go, por
// atribuicao, para que o valor do wire tenha UM dono. A alternativa —
// redeclarar `encTypeMsg = "msg"` dos dois lados da fronteira — e' a
// divergencia silenciosa que a Fase F/G existe para impedir.

// Tags dos nos que compoem um <message> de saida, nos dois caminhos (waE2E e
// v3/FB).
const (
	messageNodeTag        = "message"
	participantsNodeTag   = "participants"
	participantToNodeTag  = "to"
	plaintextNodeTag      = "plaintext"
	deviceIdentityNodeTag = "device-identity"
	bizNodeTag            = "biz"
	frankingNodeTag       = "franking"
	frankingTagNodeTag    = "franking_tag"
	traceNodeTag          = "trace"
	traceRequestIDNodeTag = "request_id"
	tcTokenNodeTag        = "tctoken"
	csTokenNodeTag        = "cstoken"

	// EncNodeTag e' a tag do no que carrega um ciphertext Signal. Lido tambem
	// pelo caminho de decifragem da raiz.
	EncNodeTag = "enc"
	encNodeTag = EncNodeTag
)

// Atributos do stanza <message> de saida.
const (
	msgAttrID               = "id"
	msgAttrType             = "type"
	msgAttrTo               = "to"
	msgAttrCategory         = "category"
	msgAttrEdit             = "edit"
	msgAttrPHash            = "phash"
	msgAttrMediaID          = "media_id"
	msgAttrAddressingMode   = "addressing_mode"
	msgAttrPushPriority     = "push_priority"
	msgAttrPrivacySensitive = "privacy_sensitive"
)

// Valores dos atributos acima. Os valores de tipo de mensagem espelham o que
// msgattrs.GetTypeFromMessage devolve; sao comparados, nunca produzidos, por
// este pacote.
const (
	msgTypeText     = "text"
	msgTypePoll     = "poll"
	msgTypeReaction = "reaction"

	// MsgCategoryPeer e' o `category` das mensagens de protocolo entre os
	// proprios dispositivos. Lido tambem por message.go, na raiz.
	MsgCategoryPeer = "peer"
	msgCategoryPeer = MsgCategoryPeer

	// pushPriorityHigh pede ao servidor que acorde o destinatario para pedidos
	// de chave de app state; pushPriorityHighForce faz o mesmo para o history
	// sync sob demanda, que alem disso e' marcado como privacy sensitive.
	pushPriorityHigh      = "high"
	pushPriorityHighForce = "high_force"
	privacySensitiveOn    = "1"
)

// Atributos do <enc>, mais o unico atributo do <to> que o embrulha por
// dispositivo. Os tres exportados sao lidos de volta pelo caminho de
// decifragem da raiz.
const (
	EncAttrVersion     = "v"
	EncAttrType        = "type"
	EncAttrDecryptFail = "decrypt-fail"

	encAttrVersion     = EncAttrVersion
	encAttrType        = EncAttrType
	encAttrDecryptFail = EncAttrDecryptFail

	encAttrMediaType = "mediatype"

	participantToAttrJID = "jid"
)

// Valores do atributo `type` do <enc>: mensagem Signal normal, mensagem que
// tambem estabelece a sessao a partir de um bundle de prekey, e mensagem de
// grupo (sender key). Os tres sao lidos de volta por message_decrypt.go.
const (
	EncTypeMsg       = "msg"
	EncTypePreKeyMsg = "pkmsg"
	EncTypeSenderKey = "skmsg"

	encTypeMsg       = EncTypeMsg
	encTypePreKeyMsg = EncTypePreKeyMsg
	encTypeSenderKey = EncTypeSenderKey
)

// Valores do atributo `v` do <enc>. O caminho waE2E e' a versao 2; o caminho
// de grupo v3/FB escreve "3" como string aqui (os nos v3 por dispositivo usam
// o FBMessageVersion numerico — ver fb_encrypt.go).
const (
	encVersionSignal = "2"
	encVersionFB     = "3"
)

// Atributos e valores do <meta> dentro do stanza. E' um no diferente do <meta>
// montado a partir de SendRequestExtra.Meta (ver prepare.go), embora os dois
// usem metaNodeTag.
const (
	metaAttrAppData     = "appdata"
	metaAttrPollType    = "polltype"
	metaAttrDecryptFail = "decrypt-fail"

	metaAppDataDefault = "default"
	pollTypeCreation   = "creation"
	pollTypeVote       = "vote"
)

// participantListHashPrefix e' o prefixo de versao do hash da lista de
// participantes enviado no atributo `phash`, e participantListHashLength o
// numero de bytes do digest SHA-256 que entram nele.
const (
	participantListHashPrefix = "2"
	participantListHashLength = 6
)

// frankingKeySize e' o tamanho em bytes da chave HMAC aleatoria gerada por
// mensagem v3/FB para produzir a tag de franking.
const frankingKeySize = 32

// frankingVersion e' a versao do esquema de franking anunciada nos metadados
// da aplicacao da mensagem v3/FB.
const frankingVersion = 0

// Constantes do preparo da mensagem (antes em send_prepare.go).
const (
	// MessageSecretSize e' o tamanho em bytes do segredo aleatorio gerado para
	// mensagens que precisam de um (mensagens de bot e mensagens que carregam
	// reporting token). Tambem usado por msgsecret_poll.go, na raiz.
	MessageSecretSize = 32
	messageSecretSize = MessageSecretSize
	// defaultBotPersonaID e' o identificador de persona que o WhatsApp espera
	// em BotMetadata quando a requisicao nao configurou um.
	defaultBotPersonaID = "867051314767696$760019659443059"
	// botNodeTag e' a tag do filho que carrega a invocacao cifrada do bot
	// inline dentro do stanza.
	botNodeTag = "bot"
	// metaNodeTag e' a tag do filho que carrega metadados por requisicao.
	metaNodeTag = "meta"
)

// Nomes dos atributos do <meta> montado a partir de SendRequestExtra.Meta.
const (
	metaAttrDeprecatedLIDSession = "deprecated_lid_session"
	metaAttrThreadMsgID          = "thread_msg_id"
	metaAttrThreadMsgSenderJID   = "thread_msg_sender_jid"
)

// Nomes dos atributos do no de ack do servidor (antes em send_ack.go).
const (
	ackAttrServerID = "server_id"
	ackAttrTime     = "t"
	ackAttrError    = "error"
	ackAttrPHash    = "phash"
)

// retryFrameContext e' o contexto legivel passado a RetryFrame quando o
// servidor responde um envio com um no de desconexao.
const retryFrameContext = "message send"

// FBMessageVersion e' o `v` numerico do <enc> no caminho v3/FB. Continua sendo
// API publica da raiz com o mesmo valor e o mesmo tipo (constante sem tipo).
const FBMessageVersion = 3
