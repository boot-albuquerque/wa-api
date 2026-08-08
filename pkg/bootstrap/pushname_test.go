package bootstrap

import (
	"testing"

	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/proto/waHistorySync"
	"wa-api/internal/wa-noise/protocol/proto/waWeb"
	"wa-api/internal/wa-noise/protocol/types"
	wanoise "wa-api/internal/wa-noise"
)

// F84: o WebMessageInfo do HistorySync JÁ CARREGA o pushName (campo 19 do
// protobuf), e resolverPushName consultava só o roster local — vazio para
// `@lid` na esmagadora maioria dos casos. Gravávamos string vazia em toda
// conversa nova.
//
// Medido no banco real antes da correção: 261 de 1717 conversas individuais
// tinham nome, e `datajson` guardava `.Info.PushName = ''` mesmo em chats
// cujo remetente tinha nome.
//
// A guarda contra vazio vem da Evolution API (issue #2426); a ordem de
// camadas, do guia de LID do Baileys (issue #2414), que adverte que o cache
// de contatos não é confiável sozinho.

func msgCom(pushName string) *waHistorySync.HistorySyncMsg {
	return &waHistorySync.HistorySyncMsg{
		Message: &waWeb.WebMessageInfo{PushName: &pushName},
	}
}

const remetente = "5511999@s.whatsapp.net"

func remetenteJID(t *testing.T) types.JID {
	t.Helper()
	j, err := types.ParseJID(remetente)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return j
}

// handlerComRoster monta um handler cujo roster devolve o nome informado.
// Roster vazio ("") reproduz o estado real das identidades @lid.
func handlerComRoster(nomeNoRoster string) *UserEventHandler {
	cs := &fakeContactStore{}
	if nomeNoRoster != "" {
		j, _ := types.ParseJID(remetente)
		cs.contacts = map[types.JID]types.ContactInfo{j: {Found: true, PushName: nomeNoRoster}}
	}
	dev := &store.Device{Contacts: cs}
	return &UserEventHandler{WAClient: wanoise.NewClient(dev, nil)}
}

// TestPushName_VemDoProtobuf é o teste da F84. O roster está vazio, como está
// em produção para @lid, e o nome tem de sair mesmo assim.
func TestPushName_VemDoProtobuf(t *testing.T) {
	evh := handlerComRoster("")

	got := evh.resolverPushName(msgCom("Maria do Protobuf"), false, remetenteJID(t))

	if got != "Maria do Protobuf" {
		t.Errorf("pushName = %q, quero o do protobuf — voltamos a depender so' do roster?", got)
	}
}

// TestPushName_ProtobufVenceORoster fixa a ORDEM. O protobuf chega junto da
// mensagem e é o dado mais fresco; o roster pode estar desatualizado.
func TestPushName_ProtobufVenceORoster(t *testing.T) {
	evh := handlerComRoster("Nome Velho do Roster")

	got := evh.resolverPushName(msgCom("Nome Novo"), false, remetenteJID(t))

	if got != "Nome Novo" {
		t.Errorf("pushName = %q, quero o do protobuf — a ordem das camadas inverteu", got)
	}
}

// TestPushName_VazioNaoVenceORoster é a guarda da Evolution API. Um protobuf
// sem nome NÃO pode apagar o nome que o roster conhece.
func TestPushName_VazioNaoVenceORoster(t *testing.T) {
	evh := handlerComRoster("Nome do Roster")

	got := evh.resolverPushName(msgCom(""), false, remetenteJID(t))

	if got != "Nome do Roster" {
		t.Errorf("pushName = %q, quero o do roster — o vazio venceu um nome que existia", got)
	}
}

// TestPushName_MensagemPropriaNaoResolve: quem enviou é o dono da sessão, e
// não há remetente a nomear. Sem esta guarda, gravaríamos o nome do
// destinatário como se fosse do remetente.
func TestPushName_MensagemPropriaNaoResolve(t *testing.T) {
	evh := handlerComRoster("Nome do Roster")

	if got := evh.resolverPushName(msgCom("Qualquer"), true, remetenteJID(t)); got != "" {
		t.Errorf("pushName = %q para mensagem propria, quero vazio", got)
	}
}

// TestPushName_SemClienteNaoEntraEmPanico: o handler pode não ter cliente
// (caminhos de teste e de sessão ainda não montada), e uma mensagem sem
// protobuf preenchido é comum no histórico.
func TestPushName_SemClienteNaoEntraEmPanico(t *testing.T) {
	evh := &UserEventHandler{}

	if got := evh.resolverPushName(nil, false, remetenteJID(t)); got != "" {
		t.Errorf("pushName = %q sem cliente e sem protobuf, quero vazio", got)
	}
}
