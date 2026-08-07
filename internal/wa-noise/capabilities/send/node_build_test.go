package send

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
)

func childTags(nodes []waBinary.Node) []string {
	tags := make([]string, len(nodes))
	for i, n := range nodes {
		tags[i] = n.Tag
	}
	return tags
}

func hasTag(nodes []waBinary.Node, tag string) bool {
	for _, n := range nodes {
		if n.Tag == tag {
			return true
		}
	}
	return false
}

// --- marshalMessage ---

// Contrato central do envio: DM/broadcast levam a copia DeviceSentMessage para
// os proprios dispositivos; grupo e newsletter nao.
func TestMarshalMessageDeviceSentCopyPerServer(t *testing.T) {
	msg := &waE2E.Message{Conversation: proto.String("oi")}
	cases := map[string]struct {
		to      types.JID
		wantDSM bool
	}{
		"pn":         {types.JID{User: "1", Server: types.DefaultUserServer}, true},
		"lid":        {types.JID{User: "1", Server: types.HiddenUserServer}, true},
		"broadcast":  {types.JID{User: "status", Server: types.BroadcastServer}, true},
		"grupo":      {types.JID{User: "1", Server: types.GroupServer}, false},
		"newsletter": {types.JID{User: "1", Server: types.NewsletterServer}, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			plaintext, dsm, err := MarshalMessage(tc.to, msg)
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if len(plaintext) == 0 {
				t.Fatal("plaintext vazio")
			}
			if (len(dsm) > 0) != tc.wantDSM {
				t.Fatalf("dsm presente=%v, queria=%v", len(dsm) > 0, tc.wantDSM)
			}
			if !tc.wantDSM {
				return
			}
			var decoded waE2E.Message
			if err := proto.Unmarshal(dsm, &decoded); err != nil {
				t.Fatalf("dsm nao desserializa: %v", err)
			}
			if got := decoded.GetDeviceSentMessage().GetDestinationJID(); got != tc.to.String() {
				t.Errorf("DestinationJID = %q, want %q", got, tc.to.String())
			}
			if got := decoded.GetDeviceSentMessage().GetMessage().GetConversation(); got != "oi" {
				t.Errorf("Conversation embrulhada = %q", got)
			}
		})
	}
}

// O MessageContextInfo (que carrega o message secret) e' replicado no envelope
// externo do DSM, nao so' na mensagem interna — se isso quebrar, o proprio
// dispositivo perde a chave de segredo da mensagem.
func TestMarshalMessageDeviceSentKeepsContextInfo(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	msg := &waE2E.Message{
		Conversation:       proto.String("oi"),
		MessageContextInfo: &waE2E.MessageContextInfo{MessageSecret: secret},
	}
	_, dsm, err := MarshalMessage(types.JID{User: "1", Server: types.DefaultUserServer}, msg)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	var decoded waE2E.Message
	if err := proto.Unmarshal(dsm, &decoded); err != nil {
		t.Fatalf("dsm nao desserializa: %v", err)
	}
	if string(decoded.GetMessageContextInfo().GetMessageSecret()) != string(secret) {
		t.Error("MessageContextInfo nao foi copiado para o envelope do DSM")
	}
}

// Revoke de newsletter chega aqui com message == nil (sendNewsletter zera a
// mensagem antes de marshalar); isso tem que devolver tudo vazio sem erro.
func TestMarshalMessageNilNewsletter(t *testing.T) {
	plaintext, dsm, err := MarshalMessage(types.JID{User: "1", Server: types.NewsletterServer}, nil)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if plaintext != nil || dsm != nil {
		t.Errorf("plaintext=%v dsm=%v, queria ambos nil", plaintext, dsm)
	}
}

// --- getMessageContent ---

func TestGetMessageContentBaseNodeOnly(t *testing.T) {
	tr := newFakeTransport()
	base := waBinary.Node{Tag: participantsNodeTag}
	content := mustMessageContent(t,
		tr, base, &waE2E.Message{Conversation: proto.String("oi")},
		waBinary.Attrs{msgAttrType: msgTypeText}, false, NodeExtraParams{},
	)
	if got := childTags(content); len(got) != 1 || got[0] != participantsNodeTag {
		t.Errorf("filhos = %v, queria so' <participants>", got)
	}
}

func TestGetMessageContentPollMeta(t *testing.T) {
	tr := newFakeTransport()
	cases := map[string]struct {
		message  *waE2E.Message
		wantType string
	}{
		"criacao": {&waE2E.Message{PollCreationMessage: &waE2E.PollCreationMessage{}}, pollTypeCreation},
		"voto":    {&waE2E.Message{PollUpdateMessage: &waE2E.PollUpdateMessage{}}, pollTypeVote},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			content := mustMessageContent(t,
				tr, waBinary.Node{Tag: participantsNodeTag}, tc.message,
				waBinary.Attrs{msgAttrType: msgTypePoll}, false, NodeExtraParams{},
			)
			var meta *waBinary.Node
			for i := range content {
				if content[i].Tag == metaNodeTag {
					meta = &content[i]
				}
			}
			if meta == nil {
				t.Fatalf("faltou <meta>, filhos = %v", childTags(content))
			}
			if got := meta.Attrs[metaAttrPollType]; got != tc.wantType {
				t.Errorf("polltype = %v, want %q", got, tc.wantType)
			}
		})
	}
}

// Mensagem que nao e' enquete nao pode ganhar <meta polltype>, mesmo tendo
// PollUpdateMessage preenchido — quem decide e' o atributo `type`.
func TestGetMessageContentNoPollMetaForNonPollType(t *testing.T) {
	tr := newFakeTransport()
	content := mustMessageContent(t,
		tr, waBinary.Node{Tag: participantsNodeTag},
		&waE2E.Message{PollUpdateMessage: &waE2E.PollUpdateMessage{}},
		waBinary.Attrs{msgAttrType: msgTypeText}, false, NodeExtraParams{},
	)
	if hasTag(content, metaNodeTag) {
		t.Errorf("nao deveria haver <meta>, filhos = %v", childTags(content))
	}
}

// A ordem dos filhos e' contrato de protocolo: base, identidade, poll, bot,
// meta, adicionais, biz.
func TestGetMessageContentChildOrder(t *testing.T) {
	tr := newFakeTransport()
	extra := NodeExtraParams{
		botNode:         &waBinary.Node{Tag: botNodeTag},
		metaNode:        &waBinary.Node{Tag: metaNodeTag},
		additionalNodes: &[]waBinary.Node{{Tag: "custom"}},
	}
	content := mustMessageContent(t,
		tr, waBinary.Node{Tag: participantsNodeTag},
		&waE2E.Message{PollCreationMessage: &waE2E.PollCreationMessage{}},
		waBinary.Attrs{msgAttrType: msgTypePoll}, false, extra,
	)
	want := []string{participantsNodeTag, metaNodeTag, botNodeTag, metaNodeTag, "custom"}
	got := childTags(content)
	if len(got) != len(want) {
		t.Fatalf("filhos = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("filhos = %v, want %v", got, want)
		}
	}
}

func TestGetMessageContentButtonNode(t *testing.T) {
	tr := newFakeTransport()
	content := mustMessageContent(t,
		tr, waBinary.Node{Tag: participantsNodeTag},
		&waE2E.Message{ButtonsMessage: &waE2E.ButtonsMessage{}},
		waBinary.Attrs{msgAttrType: msgTypeText}, false, NodeExtraParams{},
	)
	if !hasTag(content, bizNodeTag) {
		t.Fatalf("faltou <biz>, filhos = %v", childTags(content))
	}
	if content[len(content)-1].Tag != bizNodeTag {
		t.Errorf("<biz> deve ser o ultimo filho, filhos = %v", childTags(content))
	}
}

// --- copyAttrs ---

// copyAttrs sobrescreve o destino. Isso importa em encryptMessageForDevice:
// ela roda *depois* de o `type` ser definido a partir de ciphertext.Type(),
// entao um extraAttrs com "type" mandaria o tipo errado para o fio. Nenhum
// chamador de hoje passa "type"; o teste trava a semantica caso mude.
func TestCopyAttrsOverwritesDestination(t *testing.T) {
	dst := waBinary.Attrs{encAttrType: encTypePreKeyMsg, encAttrVersion: encVersionSignal}
	copyAttrs(waBinary.Attrs{encAttrType: encTypeMsg, encAttrMediaType: "image"}, dst)
	if dst[encAttrType] != encTypeMsg {
		t.Errorf("type = %v, copyAttrs deveria sobrescrever", dst[encAttrType])
	}
	if dst[encAttrVersion] != encVersionSignal {
		t.Errorf("v = %v, nao deveria mudar", dst[encAttrVersion])
	}
	if dst[encAttrMediaType] != "image" {
		t.Errorf("mediatype = %v", dst[encAttrMediaType])
	}
}

func TestCopyAttrsEmptySource(t *testing.T) {
	dst := waBinary.Attrs{encAttrType: encTypeMsg}
	copyAttrs(nil, dst)
	copyAttrs(waBinary.Attrs{}, dst)
	if len(dst) != 1 || dst[encAttrType] != encTypeMsg {
		t.Errorf("dst = %v, nao deveria mudar", dst)
	}
}

// mustMessageContent embrulha MessageContent nos testes que exercitam o caminho
// feliz: desde a correcao da F41 ela devolve erro, e nenhum destes casos monta
// um transporte sem Account.
func mustMessageContent(
	t *testing.T,
	tr Transport,
	baseNode waBinary.Node,
	message *waE2E.Message,
	msgAttrs waBinary.Attrs,
	includeIdentity bool,
	extraParams NodeExtraParams,
) []waBinary.Node {
	t.Helper()
	content, err := MessageContent(tr, baseNode, message, msgAttrs, includeIdentity, extraParams)
	if err != nil {
		t.Fatalf("MessageContent: %v", err)
	}
	return content
}

// includeIdentity com Store.Account vazio ia em silencio para o fio: o no'
// <device-identity> saia com conteudo vazio porque proto.Marshal(nil) nao
// devolve erro (F41).
func TestMessageContentSemContaDevolveErro(t *testing.T) {
	tr := newFakeTransport()
	tr.store.Account = nil
	content, err := MessageContent(
		tr, waBinary.Node{Tag: participantsNodeTag},
		&waE2E.Message{Conversation: proto.String("oi")},
		waBinary.Attrs{msgAttrType: msgTypeText}, true, NodeExtraParams{},
	)
	if !errors.Is(err, ErrNoDeviceIdentity) {
		t.Fatalf("err = %v, esperava ErrNoDeviceIdentity", err)
	}
	if content != nil {
		t.Errorf("content = %v, esperava nil", content)
	}
}
