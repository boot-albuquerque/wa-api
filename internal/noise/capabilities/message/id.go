package message

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/util/random"

	"wa-api/internal/noise/protocol/types"
)

// GenerateID gera um ID de mensagem novo.
//
// `ownID` e' o JID da sessao (pode vir zerado — a raiz devolve EmptyJID quando
// o cliente e' nil, e o zerado simplesmente nao entra no material do hash);
// `isMessenger` diz se a sessao e' Messenger, caso em que o ID e' o numerico do
// Facebook em vez do hash do WhatsApp Web.
//
// Os dois parametros substituem os `cli.MessengerConfig != nil` e
// `cli.getOwnID()` que a versao em *Client fazia — a fachada da raiz continua
// fazendo as mesmas duas consultas, na mesma ordem.
func GenerateID(ownID types.JID, isMessenger bool) types.MessageID {
	if isMessenger {
		return types.MessageID(strconv.FormatInt(GenerateFacebookID(), 10))
	}
	data := make([]byte, webMessageIDTimestampLength, webMessageIDTimestampLength+20+webMessageIDRandomLength)
	binary.BigEndian.PutUint64(data, uint64(time.Now().Unix()))
	if !ownID.IsEmpty() {
		data = append(data, []byte(ownID.User)...)
		data = append(data, []byte(webMessageIDJIDSuffix)...)
	}
	data = append(data, random.Bytes(webMessageIDRandomLength)...)
	hash := sha256.Sum256(data)
	return WebMessageIDPrefix + strings.ToUpper(hex.EncodeToString(hash[:webMessageIDHashLength]))
}

// GenerateFacebookID gera o ID de mensagem numerico do Messenger: unix time em
// milissegundos deslocado, com os bits baixos aleatorios.
func GenerateFacebookID() int64 {
	const randomMask = (1 << facebookMessageIDRandomBits) - 1
	return (time.Now().UnixMilli() << facebookMessageIDRandomBits) | (int64(binary.BigEndian.Uint32(random.Bytes(4))) & randomMask)
}

// GenerateLegacyID gera o ID de mensagem do formato antigo do WhatsApp Web, que
// e' so' aleatorio em hex, sem hash.
//
// Deprecated: o WhatsApp Web passou a usar um hash de timestamp, id de usuario
// e bytes aleatorios. Use GenerateID.
func GenerateLegacyID() types.MessageID {
	return WebMessageIDPrefix + strings.ToUpper(hex.EncodeToString(random.Bytes(legacyMessageIDRandomLength)))
}
