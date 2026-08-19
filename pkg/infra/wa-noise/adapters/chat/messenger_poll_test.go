package chat

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	waclient "wa-api/pkg/infra/wa-noise/client"
	"wa-api/pkg/infra/wa-noise/client/testkit"

	"wa-api/pkg/domain"

	wanoise "wa-api/internal/wa-noise"
	wamessage "wa-api/internal/wa-noise/capabilities/message"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
)

// Este arquivo cobre SendPoll (CAP-14) no nível em que os argumentos do wire
// ficam visíveis: o terceiro argumento de BuildPollCreation
// (selectableOptionCount), e o par (msgID, opções) que o adapter memoriza
// depois do envio. Acima daqui nenhum dos dois é observável.
//
// O fake NÃO monta um protobuf próprio: testkit.Fake.BuildPollCreation
// delega para o construtor REAL
// (internal/wa-noise/capabilities/message/poll.go:65), que é para onde
// (*core.Client).BuildPollCreation também delega
// (internal/wa-noise/core/msgsecret_poll.go:65). ARMADILHA 1: um dublê mais
// permissivo esconderia exatamente a troca de argumento que este arquivo
// existe para pegar.

const pollGroupJID = "120363313346913103@g.us"

// pollTestOptions são as opções de referência. Acento e espaço de propósito:
// é o texto exato que atravessa SHA-256 no wire.
func pollTestOptions() []string { return []string{"Almoço às 12h", "Almoço às 13h"} }

// pollRecorderCall é uma chamada a SetPollOptions.
type pollRecorderCall struct {
	UserID  string
	MsgID   string
	Options []string
}

// pollRecorder é o dublê de PollOptionRecorder. Guarda o que o registrador de
// produção guarda — ver pkg/infra/wa-noise/registry/manager.go:154 e
// registry/userclients/userclients.go:70 — e nada além disso.
type pollRecorder struct {
	Calls []pollRecorderCall
}

func (r *pollRecorder) SetPollOptions(userID, msgID string, options []string) {
	r.Calls = append(r.Calls, pollRecorderCall{UserID: userID, MsgID: msgID, Options: options})
}

func pollAdapter(f *testkit.Fake, rec PollOptionRecorder) *ChatMessengerAdapter {
	return NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": f})).WithPollOptions(rec)
}

func TestChatMessengerAdapter_SendPoll_NoSession(t *testing.T) {
	rec := &pollRecorder{}
	a := NewChatMessengerAdapter(testkit.GetterWith(nil)).WithPollOptions(rec)

	_, err := a.SendPoll(context.Background(), "u1", pollGroupJID, domain.PollPayload{Name: "Qual?", Options: pollTestOptions()}, "")
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("SendPoll code = %q", testkit.AppErrCode(err))
	}
	if n := len(rec.Calls); n != 0 {
		t.Errorf("sem sessao, mas SetPollOptions foi chamado %d vez(es)", n)
	}
}

// TestChatMessengerAdapter_SendPoll_SelectableOptionCountIsOne trava o
// terceiro argumento de BuildPollCreation. O histórico sempre passou 1
// (escolha ÚNICA, `git show 41bc8e2^:handlers.go`, linha 2796); qualquer
// outro valor muda o contrato da enquete no aparelho de quem vota, e nenhum
// outro teste desta capability o observa — acima do adapter esse número não
// existe.
func TestChatMessengerAdapter_SendPoll_SelectableOptionCountIsOne(t *testing.T) {
	var gotName string
	var gotOptions []string
	var gotCount int
	f := &testkit.Fake{
		BuildPollCreationFn: func(name string, optionNames []string, selectableOptionCount int) *waE2E.Message {
			gotName, gotOptions, gotCount = name, optionNames, selectableOptionCount
			return &waE2E.Message{}
		},
		SendMessageFn: func(context.Context, types.JID, *waE2E.Message, ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			return wanoise.SendResponse{ID: "poll-wire-id", Timestamp: time.Unix(1755500110, 0)}, nil
		},
	}
	a := pollAdapter(f, &pollRecorder{})

	if _, err := a.SendPoll(context.Background(), "u1", pollGroupJID,
		domain.PollPayload{Name: "Que horas almoçamos?", Options: pollTestOptions()}, ""); err != nil {
		t.Fatalf("SendPoll: %v", err)
	}

	if gotCount != 1 {
		t.Errorf("selectableOptionCount = %d, quero 1 (escolha UNICA, contrato historico)", gotCount)
	}
	if gotName != "Que horas almoçamos?" {
		t.Errorf("name = %q, quero o Header do payload", gotName)
	}
	if len(gotOptions) != 2 || gotOptions[0] != "Almoço às 12h" || gotOptions[1] != "Almoço às 13h" {
		t.Errorf("optionNames = %q, quero %q na mesma ordem", gotOptions, pollTestOptions())
	}
}

// TestChatMessengerAdapter_SendPoll_RealBuilderProducesSelectableOne é o par
// do teste acima medido pelo construtor REAL, sem BuildPollCreationFn: o que
// chega a SendMessage tem de ser um PollCreationMessage com
// SelectableOptionsCount=1, as opções na ordem e MessageSecret preenchido
// (sem ele o voto não é decifrável).
func TestChatMessengerAdapter_SendPoll_RealBuilderProducesSelectableOne(t *testing.T) {
	var sent *waE2E.Message
	f := &testkit.Fake{
		SendMessageFn: func(_ context.Context, _ types.JID, m *waE2E.Message, _ ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			sent = m
			return wanoise.SendResponse{ID: "poll-wire-id", Timestamp: time.Unix(1755500110, 0)}, nil
		},
	}
	a := pollAdapter(f, &pollRecorder{})

	if _, err := a.SendPoll(context.Background(), "u1", pollGroupJID,
		domain.PollPayload{Name: "Qual?", Options: pollTestOptions()}, ""); err != nil {
		t.Fatalf("SendPoll: %v", err)
	}

	pc := sent.GetPollCreationMessage()
	if pc == nil {
		t.Fatalf("a mensagem enviada nao e' um PollCreationMessage: %+v", sent)
	}
	if got := pc.GetSelectableOptionsCount(); got != 1 {
		t.Errorf("SelectableOptionsCount = %d, quero 1", got)
	}
	if got := pc.GetName(); got != "Qual?" {
		t.Errorf("Name = %q, quero %q", got, "Qual?")
	}
	want := pollTestOptions()
	if len(pc.GetOptions()) != len(want) {
		t.Fatalf("Options: %d, quero %d", len(pc.GetOptions()), len(want))
	}
	for i, opt := range pc.GetOptions() {
		if opt.GetOptionName() != want[i] {
			t.Errorf("Options[%d] = %q, quero %q", i, opt.GetOptionName(), want[i])
		}
	}
	if len(sent.GetMessageContextInfo().GetMessageSecret()) == 0 {
		t.Error("MessageContextInfo.MessageSecret vazio: sem ele o voto nao e' decifravel")
	}
}

// TestChatMessengerAdapter_SendPoll_RemembersOptionsUnderServerID é O TESTE
// DO PASSO QUE NINGUÉM ADIVINHA. O voto chega indexado pelo ID que o
// SERVIDOR conhece (pkg/bootstrap/eventhandler_message.go:117), então as
// opções têm de ficar guardadas sob resp.ID — não sob o id que o chamador
// pediu, que pode nem existir.
func TestChatMessengerAdapter_SendPoll_RemembersOptionsUnderServerID(t *testing.T) {
	f := &testkit.Fake{
		SendMessageFn: func(context.Context, types.JID, *waE2E.Message, ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			return wanoise.SendResponse{ID: "id-que-o-sdk-usou", Timestamp: time.Unix(1755500110, 0)}, nil
		},
	}
	rec := &pollRecorder{}
	a := pollAdapter(f, rec)

	res, err := a.SendPoll(context.Background(), "u1", pollGroupJID,
		domain.PollPayload{Name: "Qual?", Options: pollTestOptions()}, "id-pedido-pelo-cliente")
	if err != nil {
		t.Fatalf("SendPoll: %v", err)
	}

	if n := len(rec.Calls); n != 1 {
		t.Fatalf("SetPollOptions chamado %d vez(es), quero exatamente 1", n)
	}
	call := rec.Calls[0]
	if call.UserID != "u1" {
		t.Errorf("userID guardado = %q, quero %q", call.UserID, "u1")
	}
	if call.MsgID != "id-que-o-sdk-usou" {
		t.Errorf("msgID guardado = %q, quero o ID que o SERVIDOR devolveu (%q); com o id pedido pelo cliente o voto nunca encontra as opcoes",
			call.MsgID, "id-que-o-sdk-usou")
	}
	if call.MsgID != res.ID {
		t.Errorf("msgID guardado (%q) diverge do devolvido ao chamador (%q)", call.MsgID, res.ID)
	}
	want := pollTestOptions()
	if len(call.Options) != len(want) {
		t.Fatalf("opcoes guardadas: %d, quero %d", len(call.Options), len(want))
	}
	for i := range want {
		if call.Options[i] != want[i] {
			t.Errorf("opcao guardada [%d] = %q, quero %q", i, call.Options[i], want[i])
		}
	}
}

// TestChatMessengerAdapter_SendPoll_StoredOptionsResolveTheVoteHashes é o
// teste de PONTA A PONTA do casamento hash->texto, no mesmo espírito do que
// pkg/bootstrap/eventhandler_message.go:130 faz. Refaz aqui a mesma
// aritmética que o handler de eventos faz — SHA-256 do texto guardado — e
// exige que ela resolva os hashes que o wire produz para essas opções
// (internal/wa-noise/capabilities/message/poll.go:38, HashPollOptions).
//
// Se as opções não forem guardadas, ou forem guardadas alteradas (trim,
// normalização de acento, ordem trocada por um mapa), este teste morde: um
// hash fica sem texto e o operador recebe voto ilegível.
func TestChatMessengerAdapter_SendPoll_StoredOptionsResolveTheVoteHashes(t *testing.T) {
	f := &testkit.Fake{
		SendMessageFn: func(context.Context, types.JID, *waE2E.Message, ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			return wanoise.SendResponse{ID: "poll-msg-1", Timestamp: time.Unix(1755500110, 0)}, nil
		},
	}
	rec := &pollRecorder{}
	a := pollAdapter(f, rec)

	options := pollTestOptions()
	if _, err := a.SendPoll(context.Background(), "u1", pollGroupJID,
		domain.PollPayload{Name: "Qual?", Options: options}, ""); err != nil {
		t.Fatalf("SendPoll: %v", err)
	}
	if len(rec.Calls) != 1 {
		t.Fatalf("SetPollOptions chamado %d vez(es), quero 1 — sem opcoes guardadas o voto e' ilegivel", len(rec.Calls))
	}
	stored := rec.Calls[0].Options

	// O índice hash->texto, EXATAMENTE como eventhandler_message.go:130 o
	// monta a partir do que GetPollOptions devolve.
	optionsByHash := make(map[string]string, len(stored))
	for _, opt := range stored {
		sum := sha256.Sum256([]byte(opt))
		optionsByHash[string(sum[:])] = opt
	}

	// Os hashes que o wire produz para essas opções — a função REAL do
	// fork (internal/wa-noise/capabilities/message/poll.go:38), a mesma que
	// BuildPollVote usa para montar o voto. Não uma reimplementação local:
	// ARMADILHA 1, o dublê da regra tem de ser a regra.
	for i, h := range wamessage.HashPollOptions(options) {
		got, found := optionsByHash[string(h)]
		if !found {
			t.Fatalf("o hash da opcao %d (%q) nao casou com nenhuma opcao guardada; o voto chegaria sem texto", i, options[i])
		}
		if got != options[i] {
			t.Errorf("hash da opcao %d resolveu para %q, quero %q", i, got, options[i])
		}
	}
}

// TestChatMessengerAdapter_SendPoll_SendFailureNeverRemembers é o teste da
// ORDEM: memorizar antes de o envio dar certo deixaria entrada órfã para uma
// enquete que nunca existiu.
func TestChatMessengerAdapter_SendPoll_SendFailureNeverRemembers(t *testing.T) {
	sendErr := errors.New("send: boom")
	f := &testkit.Fake{
		SendMessageFn: func(context.Context, types.JID, *waE2E.Message, ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			return wanoise.SendResponse{}, sendErr
		},
	}
	rec := &pollRecorder{}
	a := pollAdapter(f, rec)

	res, err := a.SendPoll(context.Background(), "u1", pollGroupJID,
		domain.PollPayload{Name: "Qual?", Options: pollTestOptions()}, "")
	if !errors.Is(err, sendErr) {
		t.Fatalf("erro do envio nao chegou ao chamador: %#v", err)
	}
	if res.ID != "" || !res.Timestamp.IsZero() {
		t.Errorf("envio falhou mas o resultado veio preenchido: %+v", res)
	}
	if n := len(rec.Calls); n != 0 {
		t.Errorf("envio falhou, mas SetPollOptions foi chamado %d vez(es)", n)
	}
}

// TestChatMessengerAdapter_SendPoll_ClientIDIsForwarded: quando o chamador
// pede um id, ele vai como SendRequestExtra{ID}; sem id, nenhum extra é
// passado e o SDK gera o seu.
func TestChatMessengerAdapter_SendPoll_ClientIDIsForwarded(t *testing.T) {
	cases := map[string]struct {
		id        string
		wantExtra int
	}{
		"com_id": {"3EB0FORCADO", 1},
		"sem_id": {"", 0},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var gotExtra []wanoise.SendRequestExtra
			f := &testkit.Fake{
				SendMessageFn: func(_ context.Context, _ types.JID, _ *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
					gotExtra = extra
					return wanoise.SendResponse{ID: "poll-wire-id"}, nil
				},
			}
			a := pollAdapter(f, &pollRecorder{})

			if _, err := a.SendPoll(context.Background(), "u1", pollGroupJID,
				domain.PollPayload{Name: "Qual?", Options: pollTestOptions()}, tc.id); err != nil {
				t.Fatalf("SendPoll: %v", err)
			}
			if len(gotExtra) != tc.wantExtra {
				t.Fatalf("extras = %d, quero %d", len(gotExtra), tc.wantExtra)
			}
			if tc.wantExtra == 1 && string(gotExtra[0].ID) != tc.id {
				t.Errorf("extra.ID = %q, quero %q", gotExtra[0].ID, tc.id)
			}
		})
	}
}

// TestChatMessengerAdapter_SendPoll_InvalidJIDNeverSends: JID inválido não
// pode alcançar SendMessage nem o registrador.
func TestChatMessengerAdapter_SendPoll_InvalidJIDNeverSends(t *testing.T) {
	sendCalled := false
	f := &testkit.Fake{
		SendMessageFn: func(context.Context, types.JID, *waE2E.Message, ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			sendCalled = true
			return wanoise.SendResponse{}, nil
		},
	}
	rec := &pollRecorder{}
	a := pollAdapter(f, rec)

	if _, err := a.SendPoll(context.Background(), "u1", domain.JID(string([]byte{0x00})),
		domain.PollPayload{Name: "Qual?", Options: pollTestOptions()}, ""); err == nil {
		t.Skip("wajid.ParseJID não falhou; caminho de erro raro")
	}
	if sendCalled {
		t.Fatal("SendPoll chamou client.SendMessage com JID invalido")
	}
	if n := len(rec.Calls); n != 0 {
		t.Fatalf("JID invalido, mas SetPollOptions foi chamado %d vez(es)", n)
	}
}

// TestChatMessengerAdapter_SendPoll_WithoutRecorderRefusesToSend trava a
// recusa deliberada: um adapter sem registrador de opções não envia enquete
// nenhuma. Enquete criada cujo voto ninguém consegue ler é pior que enquete
// não criada — e um wiring que esquecesse WithPollOptions produziria
// exatamente isso, em silêncio.
func TestChatMessengerAdapter_SendPoll_WithoutRecorderRefusesToSend(t *testing.T) {
	sendCalled := false
	f := &testkit.Fake{
		SendMessageFn: func(context.Context, types.JID, *waE2E.Message, ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			sendCalled = true
			return wanoise.SendResponse{ID: "nao-devia-chegar-aqui"}, nil
		},
	}
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": f}))

	_, err := a.SendPoll(context.Background(), "u1", pollGroupJID,
		domain.PollPayload{Name: "Qual?", Options: pollTestOptions()}, "")
	if !errors.Is(err, errNoPollOptionRecorder) {
		t.Fatalf("sem registrador, SendPoll devolveu %#v; quero errNoPollOptionRecorder", err)
	}
	if sendCalled {
		t.Fatal("sem registrador, SendPoll ainda assim enviou a enquete")
	}
}

// TestChatMessengerAdapter_WithPollOptions_ReturnsSameAdapter: o
// encadeamento do wiring devolve o próprio adapter, não uma cópia — uma
// cópia deixaria o registrador ligado num objeto descartado.
func TestChatMessengerAdapter_WithPollOptions_ReturnsSameAdapter(t *testing.T) {
	a := NewChatMessengerAdapter(testkit.GetterWith(nil))
	if got := a.WithPollOptions(&pollRecorder{}); got != a {
		t.Fatalf("WithPollOptions devolveu %p, quero o proprio adapter %p", got, a)
	}
}
