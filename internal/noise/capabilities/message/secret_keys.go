package message

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/noise/protocol/proto/waCommon"
	"wa-api/internal/noise/protocol/types"
	"wa-api/internal/noise/protocol/types/events"
	hkdfutil "wa-api/internal/noise/security/hkdf"
)

// SecretType e' o rotulo de caso de uso que entra na derivacao HKDF da chave de
// segredo de mensagem. A raiz mantem o nome historico `MsgSecretType` por
// apelido de tipo.
//
// Os valores sao ENTRADA DE HKDF: mudar qualquer string abaixo muda a chave
// derivada e quebra a decifragem de tudo que ja' foi enviado com ela.
type SecretType string

const (
	EncSecretPollVote      SecretType = "Poll Vote"
	EncSecretReaction      SecretType = "Enc Reaction"
	EncSecretComment       SecretType = "Enc Comment"
	EncSecretReportToken   SecretType = "Report Token"
	EncSecretEventResponse SecretType = "Event Response"
	EncSecretEventEdit     SecretType = "Event Edit"
	EncSecretMessageEdit   SecretType = "Message Edit"
	EncSecretPollEdit      SecretType = "Poll Edit"
	EncSecretPollAddOption SecretType = "Poll Add Option"
	EncSecretBotMsg        SecretType = "Bot Message"
)

// EncryptedSecret e' a parte comum dos payloads cifrados por segredo de
// mensagem (reaction, comment, poll vote, secret encrypted, bot). A raiz mantem
// o nome historico `messageEncryptedSecret` por apelido de tipo, porque
// internals.go (gerado) o cita.
type EncryptedSecret interface {
	GetEncIV() []byte
	GetEncPayload() []byte
}

// ApplyBotMessageHKDF deriva do segredo de mensagem a chave-base do caminho de
// mensagem de bot. E' um passo A MAIS que o caminho normal nao tem: a saida
// dela e' que entra como `origMsgSecret` em GenerateSecretKey.
func ApplyBotMessageHKDF(messageSecret []byte) []byte {
	return hkdfutil.SHA256(messageSecret, nil, []byte(EncSecretBotMsg), msgSecretKeyLength)
}

// GenerateSecretKey deriva a chave AES-GCM e o dado adicional autenticado de um
// segredo de mensagem.
//
// A concatenacao abaixo e' ENTRADA DE HKDF, na ordem exata do fio: id da
// mensagem original, remetente original (sem device), remetente da modificacao
// (sem device) e o rotulo do caso de uso. Trocar a ordem, ou usar a forma AD do
// JID, muda a chave.
func GenerateSecretKey(
	modificationType SecretType, modificationSender types.JID,
	origMsgID types.MessageID, origMsgSender types.JID, origMsgSecret []byte,
) ([]byte, []byte) {
	origMsgSenderStr := origMsgSender.ToNonAD().String()
	modificationSenderStr := modificationSender.ToNonAD().String()

	useCaseSecret := make([]byte, 0, len(origMsgID)+len(origMsgSenderStr)+len(modificationSenderStr)+len(modificationType))
	useCaseSecret = append(useCaseSecret, origMsgID...)
	useCaseSecret = append(useCaseSecret, origMsgSenderStr...)
	useCaseSecret = append(useCaseSecret, modificationSenderStr...)
	useCaseSecret = append(useCaseSecret, modificationType...)

	secretKey := hkdfutil.SHA256(origMsgSecret, nil, useCaseSecret, msgSecretKeyLength)
	var additionalData []byte
	switch modificationType {
	case EncSecretPollVote, EncSecretEventResponse, "":
		additionalData = fmt.Appendf(nil, "%s\x00%s", origMsgID, modificationSenderStr)
	}

	return secretKey, additionalData
}

// OrigSenderFromKey descobre quem mandou a mensagem original referenciada por
// uma MessageKey.
func OrigSenderFromKey(msg *events.Message, key *waCommon.MessageKey) (types.JID, error) {
	if key.GetFromMe() {
		// fromMe sempre quer dizer que a enquete e o voto sairam do mesmo usuario.
		//
		// Aqui havia um TODO dizendo que isto "esta' errado se a MessageKey usou
		// @s.whatsapp.net mas o evento novo veio de @lid". A preocupacao e'
		// legitima — a forma do JID entra como string na entrada do HKDF de
		// GenerateSecretKey, entao PN e LID derivam chaves diferentes — mas ela
		// JA' esta' coberta uma camada acima, e o TODO foi escrito sem
		// referencia a essa cobertura.
		//
		// DecryptSecret (secret_crypto.go:39-44) tenta a derivacao com o
		// remetente devolvido daqui e, se ela falhar a autenticacao, REFAZ com
		// o storedOrigSender — a identidade sob a qual o segredo foi de fato
		// gravado no store. E' precisamente o caso PN/LID divergente.
		//
		// Chutar aqui entre ownID e ownLID seria um segundo palpite empilhado
		// sobre um fallback que ja' funciona, e sem informacao nova: qual das
		// duas identidades foi usada no envio original nao esta' na MessageKey
		// quando fromMe.
		//
		// O que continua fragil e' o GATILHO do fallback, que compara
		// substring da mensagem de erro ("message authentication failed") em
		// vez de um sentinela. Isso ja' esta' registrado no doc de
		// DecryptSecret e nao e' este o lugar de corrigir.
		return msg.Info.Sender, nil
	} else if msg.Info.Chat.Server == types.DefaultUserServer || msg.Info.Chat.Server == types.HiddenUserServer {
		sender, err := types.ParseJID(key.GetRemoteJID())
		if err != nil {
			return types.EmptyJID, fmt.Errorf("failed to parse JID %q of original message sender: %w", key.GetRemoteJID(), err)
		}
		return sender, nil
	} else {
		// A ordem importa: antes o erro do ParseJID era sobrescrito pelo
		// "unexpected server" derivado do JID zerado que ele devolve, e a causa
		// real do parse quebrado nunca chegava ao log.
		sender, err := types.ParseJID(key.GetParticipant())
		if err != nil {
			return types.EmptyJID, fmt.Errorf("failed to parse JID %q of original message sender: %w", key.GetParticipant(), err)
		}
		if sender.Server != types.DefaultUserServer && sender.Server != types.HiddenUserServer {
			return types.EmptyJID, fmt.Errorf("failed to parse JID %q of original message sender: %w", key.GetParticipant(), errUnexpectedOrigSenderServer)
		}
		return sender, nil
	}
}

// KeyFromInfo monta a MessageKey que referencia a propria mensagem descrita por
// `msgInfo`. Ao contrario de BuildKey, o participant aqui e' o JID COM device.
func KeyFromInfo(msgInfo *types.MessageInfo) *waCommon.MessageKey {
	creationKey := &waCommon.MessageKey{
		RemoteJID: proto.String(msgInfo.Chat.String()),
		FromMe:    proto.Bool(msgInfo.IsFromMe),
		ID:        proto.String(msgInfo.ID),
	}
	if msgInfo.IsGroup {
		creationKey.Participant = proto.String(msgInfo.Sender.String())
	}
	return creationKey
}
