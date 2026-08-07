package send

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
)

func device(user string, n uint16) types.JID {
	return types.JID{User: user, Server: types.DefaultUserServer, Device: n}
}

// --- participantListHashV2 ---

// O phash e' comparado literalmente contra o que o servidor devolve, entao a
// forma exata (prefixo, base64 sem padding, 6 bytes do SHA-256 da concatenacao
// ordenada) e' contrato de fio, nao detalhe interno.
func TestParticipantListHashV2Shape(t *testing.T) {
	participants := []types.JID{device("1", 0), device("2", 1)}
	got := ParticipantListHashV2(participants)

	prefix, encoded, ok := strings.Cut(got, ":")
	if !ok {
		t.Fatalf("phash %q nao tem o separador", got)
	}
	if prefix != participantListHashPrefix {
		t.Errorf("prefixo = %q, want %q", prefix, participantListHashPrefix)
	}
	raw, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("nao e' base64 raw std: %v", err)
	}
	if len(raw) != participantListHashLength {
		t.Fatalf("len = %d, want %d", len(raw), participantListHashLength)
	}

	// Reimplementacao independente do algoritmo.
	sum := sha256.Sum256([]byte(participants[0].ADString() + participants[1].ADString()))
	if string(raw) != string(sum[:participantListHashLength]) {
		t.Error("os bytes nao batem com o SHA-256 da concatenacao ordenada")
	}
}

// A lista chega em ordem arbitraria (mapa de dispositivos), entao a ordenacao
// interna e' o que garante que cliente e servidor cheguem ao mesmo hash.
func TestParticipantListHashV2IgnoresInputOrder(t *testing.T) {
	a := []types.JID{device("1", 0), device("2", 1), device("3", 0)}
	b := []types.JID{device("3", 0), device("1", 0), device("2", 1)}
	if ParticipantListHashV2(a) != ParticipantListHashV2(b) {
		t.Error("o hash deve independer da ordem de entrada")
	}
}

func TestParticipantListHashV2SensitiveToMembership(t *testing.T) {
	base := ParticipantListHashV2([]types.JID{device("1", 0)})
	cases := map[string][]types.JID{
		"dispositivo a mais": {device("1", 0), device("1", 1)},
		"outro usuario":      {device("2", 0)},
		"outro device id":    {device("1", 1)},
	}
	for name, participants := range cases {
		t.Run(name, func(t *testing.T) {
			if ParticipantListHashV2(participants) == base {
				t.Error("hash colidiu com a lista base")
			}
		})
	}
}

func TestParticipantListHashV2Empty(t *testing.T) {
	got := ParticipantListHashV2(nil)
	if !strings.HasPrefix(got, participantListHashPrefix+":") {
		t.Errorf("phash de lista vazia = %q", got)
	}
	if got != ParticipantListHashV2([]types.JID{}) {
		t.Error("nil e slice vazio devem dar o mesmo hash")
	}
}

// --- applyRequestExtraNodes ---

func TestApplyRequestExtraNodesEmptyRequest(t *testing.T) {
	var extra NodeExtraParams
	applyRequestExtraNodes(&RequestExtra{}, &extra)
	if extra.metaNode != nil || extra.additionalNodes != nil {
		t.Errorf("request sem Meta/AdditionalNodes nao deve gerar nos: %+v", extra)
	}
}

// Meta presente mas com todos os campos zerados ainda cria o <meta> vazio —
// comportamento do upstream, travado aqui de proposito.
func TestApplyRequestExtraNodesEmptyMetaStillCreatesNode(t *testing.T) {
	var extra NodeExtraParams
	applyRequestExtraNodes(&RequestExtra{Meta: &types.MsgMetaInfo{}}, &extra)
	if extra.metaNode == nil {
		t.Fatal("Meta nao nil deve criar o no <meta>")
	}
	if extra.metaNode.Tag != metaNodeTag || len(extra.metaNode.Attrs) != 0 {
		t.Errorf("no = %+v, queria <meta> sem atributos", extra.metaNode)
	}
}

func TestApplyRequestExtraNodesMetaAttrs(t *testing.T) {
	deprecated := true
	var extra NodeExtraParams
	applyRequestExtraNodes(&RequestExtra{Meta: &types.MsgMetaInfo{
		DeprecatedLIDSession:   &deprecated,
		ThreadMessageID:        "THREAD1",
		ThreadMessageSenderJID: sendTestUserJID,
	}}, &extra)
	if extra.metaNode == nil {
		t.Fatal("faltou o no <meta>")
	}
	attrs := extra.metaNode.Attrs
	if attrs[metaAttrDeprecatedLIDSession] != true {
		t.Errorf("%s = %v", metaAttrDeprecatedLIDSession, attrs[metaAttrDeprecatedLIDSession])
	}
	if attrs[metaAttrThreadMsgID] != types.MessageID("THREAD1") {
		t.Errorf("%s = %v", metaAttrThreadMsgID, attrs[metaAttrThreadMsgID])
	}
	if attrs[metaAttrThreadMsgSenderJID] != sendTestUserJID {
		t.Errorf("%s = %v", metaAttrThreadMsgSenderJID, attrs[metaAttrThreadMsgSenderJID])
	}
}

// ThreadMessageID vazio nao pode escrever nem o id nem o sender — sao um par.
func TestApplyRequestExtraNodesThreadAttrsAreAPair(t *testing.T) {
	var extra NodeExtraParams
	applyRequestExtraNodes(&RequestExtra{Meta: &types.MsgMetaInfo{
		ThreadMessageSenderJID: sendTestUserJID,
	}}, &extra)
	if _, ok := extra.metaNode.Attrs[metaAttrThreadMsgSenderJID]; ok {
		t.Error("sender sem thread id nao deve ir para o no")
	}
}

func TestApplyRequestExtraNodesAdditionalNodes(t *testing.T) {
	nodes := []waBinary.Node{{Tag: "custom"}}
	var extra NodeExtraParams
	applyRequestExtraNodes(&RequestExtra{AdditionalNodes: &nodes}, &extra)
	if extra.additionalNodes != &nodes {
		t.Error("AdditionalNodes deve ser repassado por referencia")
	}
}
