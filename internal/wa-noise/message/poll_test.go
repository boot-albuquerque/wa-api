// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package message

import (
	"bytes"
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/capabilities/send"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// --- HashPollOptions ---

// O voto viaja como SHA-256 do *nome* da opcao; quem conta os votos casa os
// hashes com os nomes da enquete original.
func TestHashPollOptions(t *testing.T) {
	names := []string{"sim", "nao", "talvez"}
	hashes := HashPollOptions(names)

	if len(hashes) != len(names) {
		t.Fatalf("%d hashes para %d opcoes", len(hashes), len(names))
	}
	for i, name := range names {
		want := sha256.Sum256([]byte(name))
		if !bytes.Equal(hashes[i], want[:]) {
			t.Errorf("opcao %q: hash = %X, queria %X", name, hashes[i], want)
		}
	}
}

// A ordem e' significativa (o hash i corresponde a' opcao i) e opcoes iguais
// dao hashes iguais — nao ha' sal por posicao.
func TestHashPollOptionsIsPositionalAndUnsalted(t *testing.T) {
	hashes := HashPollOptions([]string{"sim", "sim"})
	if !bytes.Equal(hashes[0], hashes[1]) {
		t.Fatal("a mesma opcao em posicoes diferentes deu hashes diferentes")
	}
	if got := HashPollOptions(nil); len(got) != 0 {
		t.Errorf("nil deu %d hashes", len(got))
	}
}

// --- BuildPollCreation ---

func TestBuildPollCreationShape(t *testing.T) {
	msg := cliBuildPoll(t, "almoco?", []string{"pizza", "sushi"}, 1)
	poll := msg.GetPollCreationMessage()

	if poll.GetName() != "almoco?" {
		t.Errorf("nome = %q", poll.GetName())
	}
	if len(poll.GetOptions()) != 2 {
		t.Fatalf("%d opcoes", len(poll.GetOptions()))
	}
	for i, want := range []string{"pizza", "sushi"} {
		if got := poll.GetOptions()[i].GetOptionName(); got != want {
			t.Errorf("opcao %d = %q, queria %q", i, got, want)
		}
	}
	if poll.GetSelectableOptionsCount() != 1 {
		t.Errorf("selectableOptionsCount = %d", poll.GetSelectableOptionsCount())
	}
	// Sem o message secret no MessageContextInfo ninguem consegue cifrar um
	// voto para esta enquete.
	if len(msg.GetMessageContextInfo().GetMessageSecret()) != send.MessageSecretSize {
		t.Fatalf("len(segredo) = %d, queria %d", len(msg.GetMessageContextInfo().GetMessageSecret()), send.MessageSecretSize)
	}
}

// Cada enquete tem que nascer com um segredo proprio, senao um voto de uma
// enquete decifraria o de outra.
func TestBuildPollCreationSecretIsFresh(t *testing.T) {
	a := cliBuildPoll(t, "a", []string{"x"}, 1).GetMessageContextInfo().GetMessageSecret()
	b := cliBuildPoll(t, "a", []string{"x"}, 1).GetMessageContextInfo().GetMessageSecret()
	if bytes.Equal(a, b) {
		t.Fatal("duas enquetes nasceram com o mesmo message secret")
	}
}

// 0 significa "sem limite" no protocolo; contagens invalidas (negativas ou
// maiores que o numero de opcoes) sao normalizadas para 0 em vez de irem cruas.
func TestBuildPollCreationSelectableCountClamping(t *testing.T) {
	options := []string{"a", "b", "c"}
	tests := []struct {
		in   int
		want uint32
	}{
		{-1, 0},
		{0, 0},
		{1, 1},
		{3, 3},
		{4, 0},
		{99, 0},
	}
	for _, tc := range tests {
		got := cliBuildPoll(t, "p", options, tc.in).GetPollCreationMessage().GetSelectableOptionsCount()
		if got != tc.want {
			t.Errorf("entrada %d: count = %d, queria %d", tc.in, got, tc.want)
		}
	}
}

func cliBuildPoll(t *testing.T, name string, options []string, count int) *waE2E.Message {
	t.Helper()
	return BuildPollCreation(name, options, count)
}

// --- EncryptPollVote / DecryptPollVote ---

// Ida e volta completa: BuildPollVote cifra os hashes das opcoes e
// DecryptPollVote os devolve. E' o unico caminho de msgsecret que da' para
// exercitar de ponta a ponta sem sessao Signal.
func TestPollVoteRoundTrip(t *testing.T) {
	stub := &stubMsgSecretStore{secret: testSecret, origSender: testOtherJID}
	f := newFakeTransport().withSecrets(stub)
	pollInfo := &types.MessageInfo{
		MessageSource: types.MessageSource{Chat: testGroupJID, Sender: testOtherJID, IsGroup: true},
		ID:            "POLL1",
	}

	voteMsg, err := BuildPollVote(context.Background(), f, pollInfo, []string{"pizza"})
	if err != nil {
		t.Fatalf("BuildPollVote: %v", err)
	}
	update := voteMsg.GetPollUpdateMessage()
	if update.GetPollCreationMessageKey().GetID() != "POLL1" {
		t.Errorf("chave da enquete = %q", update.GetPollCreationMessageKey().GetID())
	}
	if len(update.GetVote().GetEncIV()) != msgSecretIVSize {
		t.Errorf("len(iv) = %d", len(update.GetVote().GetEncIV()))
	}

	// Quem vota e' o proprio cliente: o remetente do evento tem que ser o
	// mesmo JID que EncryptPollVote escolheu (LID, porque a enquete veio de
	// um JID de telefone... nao: veio de pn, entao usa o proprio pn).
	voteEvt := &events.Message{
		Info: types.MessageInfo{MessageSource: types.MessageSource{
			Chat: testGroupJID, Sender: testOwnJID,
		}},
		Message: voteMsg,
	}
	vote, err := DecryptPollVote(context.Background(), f, voteEvt)
	if err != nil {
		t.Fatalf("DecryptPollVote: %v", err)
	}
	want := sha256.Sum256([]byte("pizza"))
	if len(vote.GetSelectedOptions()) != 1 || !bytes.Equal(vote.GetSelectedOptions()[0], want[:]) {
		t.Fatalf("opcoes = %X, queria [%X]", vote.GetSelectedOptions(), want)
	}
}

// A escolha do proprio JID depende do server de quem criou a enquete: enquete
// vinda de @s.whatsapp.net vota com o telefone, vinda de @lid vota com o LID.
// Errar isso derivaria uma chave que o criador da enquete nao consegue refazer.
func TestEncryptPollVoteOwnJIDFollowsPollSender(t *testing.T) {
	tests := []struct {
		name       string
		pollSender types.JID
		wantOwn    func(Transport) types.JID
	}{
		{"enquete de pn vota com pn", testOtherJID, Transport.OwnID},
		{"enquete de lid vota com lid", types.NewJID("55443322", types.HiddenUserServer), Transport.OwnLID},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeTransport().withSecrets(&stubMsgSecretStore{secret: testSecret, origSender: tc.pollSender})
			pollInfo := &types.MessageInfo{
				MessageSource: types.MessageSource{Chat: testGroupJID, Sender: tc.pollSender, IsGroup: true},
				ID:            "POLL1",
			}
			update, err := EncryptPollVote(context.Background(), f, pollInfo, &waE2E.PollVoteMessage{
				SelectedOptions: HashPollOptions([]string{"pizza"}),
			})
			if err != nil {
				t.Fatalf("erro: %v", err)
			}
			// Decripta declarando como votante o JID esperado; se a escolha
			// interna tivesse sido a outra, o AAD nao bateria.
			voteEvt := &events.Message{
				Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: testGroupJID, Sender: tc.wantOwn(f)}},
				Message: &waE2E.Message{PollUpdateMessage: update},
			}
			if _, err = DecryptPollVote(context.Background(), f, voteEvt); err != nil {
				t.Fatalf("decrypt com o JID esperado falhou: %v", err)
			}
		})
	}
}

func TestEncryptPollVoteTimestamp(t *testing.T) {
	f := newFakeTransport().withSecrets(&stubMsgSecretStore{secret: testSecret, origSender: testOtherJID})
	pollInfo := &types.MessageInfo{
		MessageSource: types.MessageSource{Chat: testGroupJID, Sender: testOtherJID, IsGroup: true},
		ID:            "POLL1",
	}
	before := time.Now().UnixMilli()
	update, err := EncryptPollVote(context.Background(), f, pollInfo, &waE2E.PollVoteMessage{})
	after := time.Now().UnixMilli()
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if ts := update.GetSenderTimestampMS(); ts < before || ts > after {
		t.Fatalf("timestamp %d fora de [%d, %d]", ts, before, after)
	}
}

// BuildPollVote propaga o erro do encrypt, mas ainda devolve a mensagem
// (envelope com PollUpdateMessage nil) — comportamento do upstream, travado
// aqui para que uma mudanca acidental apareca.
func TestBuildPollVotePropagatesError(t *testing.T) {
	f := newFakeTransport().withSecrets(&stubMsgSecretStore{secret: nil})
	pollInfo := &types.MessageInfo{
		MessageSource: types.MessageSource{Chat: testGroupJID, Sender: testOtherJID, IsGroup: true},
		ID:            "POLL1",
	}
	msg, err := BuildPollVote(context.Background(), f, pollInfo, []string{"pizza"})
	if err == nil {
		t.Fatal("esperava erro")
	}
	if msg == nil || msg.PollUpdateMessage != nil {
		t.Fatalf("msg = %v, queria envelope com PollUpdateMessage nil", msg)
	}
}
