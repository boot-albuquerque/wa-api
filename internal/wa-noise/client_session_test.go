// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/proto/waCommon"
	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/proto/waWeb"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types"
)

// recordingLIDStore anota os pares gravados para que a orientacao LID/PN possa
// ser conferida — trocar os dois grava o mapeamento invertido, que depois faz o
// cliente derivar chaves com o identificador errado.
type recordingLIDStore struct {
	lid, pn types.JID
	calls   int
	err     error
}

func (s *recordingLIDStore) PutManyLIDMappings(context.Context, []store.LIDMapping) error { return nil }
func (s *recordingLIDStore) PutLIDMapping(_ context.Context, lid, pn types.JID) error {
	s.calls++
	s.lid, s.pn = lid, pn
	return s.err
}

func (s *recordingLIDStore) GetPNForLID(context.Context, types.JID) (types.JID, error) {
	return types.EmptyJID, nil
}

func (s *recordingLIDStore) GetLIDForPN(context.Context, types.JID) (types.JID, error) {
	return types.EmptyJID, nil
}

func (s *recordingLIDStore) GetManyLIDsForPNs(context.Context, []types.JID) (map[types.JID]types.JID, error) {
	return nil, nil
}

// --- StoreLIDPNMapping ---

// A funcao aceita o par em qualquer ordem e tem que normalizar: o LID sempre no
// primeiro parametro do store, o PN sempre no segundo.
func TestStoreLIDPNMappingNormalizesOrder(t *testing.T) {
	lid := types.NewJID("11223344556677", types.HiddenUserServer)
	pn := types.NewJID("5511999999999", types.DefaultUserServer)

	for _, tc := range []struct {
		name          string
		first, second types.JID
	}{
		{name: "lid primeiro", first: lid, second: pn},
		{name: "pn primeiro", first: pn, second: lid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lidStore := &recordingLIDStore{}
			cli := connTestClient()
			cli.Store.LIDs = lidStore

			cli.StoreLIDPNMapping(t.Context(), tc.first, tc.second)

			if lidStore.calls != 1 {
				t.Fatalf("PutLIDMapping chamado %d vezes, queria 1", lidStore.calls)
			}
			if lidStore.lid != lid {
				t.Errorf("lid gravado = %s, queria %s", lidStore.lid, lid)
			}
			if lidStore.pn != pn {
				t.Errorf("pn gravado = %s, queria %s", lidStore.pn, pn)
			}
		})
	}
}

// Pares que nao sao exatamente um LID e um PN nao podem ser gravados: gravar um
// grupo ou dois PNs como se fosse mapeamento corromperia a tabela.
func TestStoreLIDPNMappingRejectsInvalidPairs(t *testing.T) {
	lid := types.NewJID("11223344556677", types.HiddenUserServer)
	pn := types.NewJID("5511999999999", types.DefaultUserServer)
	group := types.NewJID("123456789", types.GroupServer)

	cases := []struct {
		name          string
		first, second types.JID
	}{
		{name: "dois PNs", first: pn, second: pn},
		{name: "dois LIDs", first: lid, second: lid},
		{name: "grupo e PN", first: group, second: pn},
		{name: "PN e grupo", first: pn, second: group},
		{name: "vazios", first: types.EmptyJID, second: types.EmptyJID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lidStore := &recordingLIDStore{}
			cli := connTestClient()
			cli.Store.LIDs = lidStore

			cli.StoreLIDPNMapping(t.Context(), tc.first, tc.second)

			if lidStore.calls != 0 {
				t.Errorf("gravou %d mapeamento(s) para um par invalido", lidStore.calls)
			}
		})
	}
}

// Erro do store vira log, nao panic nem propagacao — a funcao nao devolve erro
// de proposito, porque quem a chama nao tem o que fazer com ele.
func TestStoreLIDPNMappingSwallowsStoreError(t *testing.T) {
	lidStore := &recordingLIDStore{err: errors.New("banco fora do ar")}
	cli := connTestClient()
	cli.Store.LIDs = lidStore

	cli.StoreLIDPNMapping(
		t.Context(),
		types.NewJID("11223344556677", types.HiddenUserServer),
		types.NewJID("5511999999999", types.DefaultUserServer),
	)

	if lidStore.calls != 1 {
		t.Errorf("PutLIDMapping chamado %d vezes, queria 1", lidStore.calls)
	}
}

// --- getUnifiedSessionID ---

// O ID e' um inteiro em base 10 dentro de uma janela de uma semana em
// milissegundos. Sair da janela mudaria o formato que o servidor espera.
func TestGetUnifiedSessionIDIsWithinWeek(t *testing.T) {
	cli := connTestClient()

	for i := 0; i < 5; i++ {
		got := cli.getUnifiedSessionID()
		parsed, err := strconv.ParseInt(got, 10, 64)
		if err != nil {
			t.Fatalf("id %q nao e' inteiro em base 10: %v", got, err)
		}
		if parsed < 0 || parsed >= week.Milliseconds() {
			t.Errorf("id = %d, fora de [0, %d)", parsed, week.Milliseconds())
		}
	}
}

// O offset de relogio do servidor entra na conta: e' o que faz o ID bater com o
// que o servidor calcularia, mesmo com o relogio local adiantado ou atrasado.
func TestGetUnifiedSessionIDUsesServerTimeOffset(t *testing.T) {
	cli := connTestClient()
	base := cli.getUnifiedSessionID()

	cli.serverTimeOffset.Store(int64(6 * time.Hour))
	shifted := cli.getUnifiedSessionID()

	if base == shifted {
		t.Error("o offset de relogio do servidor nao afetou o ID")
	}
}

// --- ParseWebMessage ---

func webMsg(key *waCommon.MessageKey, participant string) *waWeb.WebMessageInfo {
	return &waWeb.WebMessageInfo{
		Key:              key,
		Participant:      proto.String(participant),
		MessageTimestamp: proto.Uint64(uint64(time.Now().Unix())),
		Message:          &waE2E.Message{Conversation: proto.String("oi")},
	}
}

// A resolucao do remetente tem seis ramos e cada um grava um Sender diferente.
// Errar qualquer um atribui a mensagem a' pessoa errada.
func TestParseWebMessageSenderResolution(t *testing.T) {
	cli := connTestClient()
	own := cli.getOwnID().ToNonAD()
	other := types.NewJID("5511888888888", types.DefaultUserServer)
	group := types.NewJID("123456789", types.GroupServer)

	t.Run("propria mensagem usa o JID proprio", func(t *testing.T) {
		evt, err := cli.ParseWebMessage(group, webMsg(
			&waCommon.MessageKey{FromMe: proto.Bool(true), ID: proto.String("A")}, ""))
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if evt.Info.Sender != own {
			t.Errorf("Sender = %s, queria %s", evt.Info.Sender, own)
		}
		if !evt.Info.IsFromMe {
			t.Error("IsFromMe deveria ser true")
		}
	})

	t.Run("DM usa o proprio chat como remetente", func(t *testing.T) {
		evt, err := cli.ParseWebMessage(other, webMsg(
			&waCommon.MessageKey{FromMe: proto.Bool(false), ID: proto.String("B")}, ""))
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if evt.Info.Sender != other {
			t.Errorf("Sender = %s, queria %s", evt.Info.Sender, other)
		}
		if evt.Info.IsGroup {
			t.Error("DM nao pode vir marcada como grupo")
		}
	})

	t.Run("grupo usa Participant do topo", func(t *testing.T) {
		evt, err := cli.ParseWebMessage(group, webMsg(
			&waCommon.MessageKey{FromMe: proto.Bool(false), ID: proto.String("C")},
			other.String()))
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if evt.Info.Sender != other {
			t.Errorf("Sender = %s, queria %s", evt.Info.Sender, other)
		}
		if !evt.Info.IsGroup {
			t.Error("IsGroup deveria ser true para chat de grupo")
		}
	})

	t.Run("grupo cai para Key.Participant", func(t *testing.T) {
		msg := webMsg(&waCommon.MessageKey{
			FromMe:      proto.Bool(false),
			ID:          proto.String("D"),
			Participant: proto.String(other.String()),
		}, "")
		msg.Participant = nil

		evt, err := cli.ParseWebMessage(group, msg)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if evt.Info.Sender != other {
			t.Errorf("Sender = %s, queria %s", evt.Info.Sender, other)
		}
	})

	t.Run("grupo sem participant nenhum e' erro", func(t *testing.T) {
		msg := webMsg(&waCommon.MessageKey{
			FromMe: proto.Bool(false), ID: proto.String("E"),
		}, "")
		msg.Participant = nil

		_, err := cli.ParseWebMessage(group, msg)
		if err == nil {
			t.Fatal("deveria falhar sem remetente identificavel")
		}
	})
}

// Participant malformado tem que virar erro, nao um JID zerado que depois seria
// gravado como se fosse valido.
func TestParseWebMessageInvalidParticipant(t *testing.T) {
	cli := connTestClient()
	group := types.NewJID("123456789", types.GroupServer)

	_, err := cli.ParseWebMessage(group, webMsg(
		&waCommon.MessageKey{FromMe: proto.Bool(false), ID: proto.String("F")},
		"1.2.3@s.whatsapp.net"))

	if err == nil {
		t.Fatal("participant malformado deveria virar erro")
	}
}

// Sem chat explicito, o chat vem do RemoteJID da chave.
func TestParseWebMessageDerivesChatFromRemoteJID(t *testing.T) {
	cli := connTestClient()
	other := types.NewJID("5511888888888", types.DefaultUserServer)

	evt, err := cli.ParseWebMessage(types.EmptyJID, webMsg(&waCommon.MessageKey{
		FromMe:    proto.Bool(false),
		ID:        proto.String("G"),
		RemoteJID: proto.String(other.String()),
	}, ""))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if evt.Info.Chat != other {
		t.Errorf("Chat = %s, queria %s", evt.Info.Chat, other)
	}
}

// Sem chat e com RemoteJID ilegivel nao da' para seguir.
func TestParseWebMessageWithoutChatOrRemoteJID(t *testing.T) {
	cli := connTestClient()
	_, err := cli.ParseWebMessage(types.EmptyJID, webMsg(
		&waCommon.MessageKey{FromMe: proto.Bool(false), ID: proto.String("H")}, ""))
	if err == nil {
		t.Fatal("deveria falhar sem chat e sem RemoteJID")
	}
}

// Mensagem propria com store sem JID: nao da' para saber quem somos.
func TestParseWebMessageFromMeWithoutOwnJID(t *testing.T) {
	cli := connTestClient()
	cli.Store.ID = nil
	group := types.NewJID("123456789", types.GroupServer)

	_, err := cli.ParseWebMessage(group, webMsg(
		&waCommon.MessageKey{FromMe: proto.Bool(true), ID: proto.String("I")}, ""))

	if !errors.Is(err, ErrNotLoggedIn) {
		t.Errorf("err = %v, queria ErrNotLoggedIn", err)
	}
}

// Uma edicao tem que ser desembrulhada: o evento passa a carregar o ID e o
// conteudo da mensagem *editada*, nao os do envelope de protocolo. Sem isso o
// histórico mostraria o envelope em vez do texto novo.
func TestParseWebMessageUnwrapsEdit(t *testing.T) {
	cli := connTestClient()
	other := types.NewJID("5511888888888", types.DefaultUserServer)

	msg := webMsg(&waCommon.MessageKey{
		FromMe: proto.Bool(false), ID: proto.String("ENVELOPE"),
	}, "")
	msg.Message = &waE2E.Message{
		ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
			Key:  &waCommon.MessageKey{ID: proto.String("ORIGINAL")},
			EditedMessage: &waE2E.Message{
				Conversation: proto.String("texto corrigido"),
			},
		},
	}

	evt, err := cli.ParseWebMessage(other, msg)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if evt.Info.ID != "ORIGINAL" {
		t.Errorf("ID = %q, queria \"ORIGINAL\" (o da mensagem editada)", evt.Info.ID)
	}
	if evt.Message.GetConversation() != "texto corrigido" {
		t.Errorf("conteudo = %q, queria o texto editado", evt.Message.GetConversation())
	}
}

// O parent de comentario, quando presente, tem que virar metadado de thread.
func TestParseWebMessageCommentMetadata(t *testing.T) {
	cli := connTestClient()
	other := types.NewJID("5511888888888", types.DefaultUserServer)
	parent := types.NewJID("5511777777777", types.DefaultUserServer)

	msg := webMsg(&waCommon.MessageKey{
		FromMe: proto.Bool(false), ID: proto.String("J"),
	}, "")
	msg.CommentMetadata = &waWeb.CommentMetadata{
		CommentParentKey: &waCommon.MessageKey{
			ID:          proto.String("PAI"),
			Participant: proto.String(parent.String()),
		},
	}

	evt, err := cli.ParseWebMessage(other, msg)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if evt.Info.MsgMetaInfo.ThreadMessageID != "PAI" {
		t.Errorf("ThreadMessageID = %q, queria \"PAI\"", evt.Info.MsgMetaInfo.ThreadMessageID)
	}
	if evt.Info.MsgMetaInfo.ThreadMessageSenderJID != parent {
		t.Errorf("ThreadMessageSenderJID = %s, queria %s",
			evt.Info.MsgMetaInfo.ThreadMessageSenderJID, parent)
	}
}

// --- Logout ---

// Logout com credenciais Messenger nao e' suportado, e sem JID proprio nao ha'
// o que deslogar. Os dois cortes acontecem antes de qualquer IQ, entao dao para
// testar sem socket.
func TestLogoutGuards(t *testing.T) {
	var nilCli *Client
	if err := nilCli.Logout(t.Context()); !errors.Is(err, ErrClientIsNil) {
		t.Errorf("err = %v, queria ErrClientIsNil", err)
	}

	cli := connTestClient()
	cli.MessengerConfig = &MessengerConfig{}
	if err := cli.Logout(t.Context()); err == nil {
		t.Error("logout com credenciais Messenger deveria falhar")
	}

	cli = connTestClient()
	cli.Store.ID = nil
	if err := cli.Logout(t.Context()); !errors.Is(err, ErrNotLoggedIn) {
		t.Errorf("err = %v, queria ErrNotLoggedIn", err)
	}
}
