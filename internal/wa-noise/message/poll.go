// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package message

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"go.mau.fi/util/random"
	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/send"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
)

// DecryptPollVote decifra um voto de enquete. O voto em si contem hashes
// SHA-256 das opcoes escolhidas.
func DecryptPollVote(ctx context.Context, t Transport, vote *events.Message) (*waE2E.PollVoteMessage, error) {
	pollUpdate := vote.Message.GetPollUpdateMessage()
	if pollUpdate == nil {
		return nil, ErrNotPollUpdateMessage
	}
	plaintext, err := DecryptSecret(ctx, t, vote, EncSecretPollVote, pollUpdate.GetVote(), pollUpdate.GetPollCreationMessageKey())
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt poll vote: %w", err)
	}
	var msg waE2E.PollVoteMessage
	err = proto.Unmarshal(plaintext, &msg)
	if err != nil {
		return nil, fmt.Errorf("failed to decode poll vote protobuf: %w", err)
	}
	return &msg, nil
}

// HashPollOptions hasheia nomes de opcao de enquete com SHA-256 para votacao.
func HashPollOptions(optionNames []string) [][]byte {
	optionHashes := make([][]byte, len(optionNames))
	for i, option := range optionNames {
		optionHash := sha256.Sum256([]byte(option))
		optionHashes[i] = optionHash[:]
	}
	return optionHashes
}

// BuildPollVote monta uma mensagem de voto de enquete.
//
// O erro de EncryptPollVote e' devolvido JUNTO com uma *waE2E.Message nao nula
// (embrulhando um PollUpdateMessage nil), exatamente como antes da extracao.
// Trocar para `return nil, err` mudaria o que chamadores que ignoram o erro
// veem.
func BuildPollVote(ctx context.Context, t Transport, pollInfo *types.MessageInfo, optionNames []string) (*waE2E.Message, error) {
	pollUpdate, err := EncryptPollVote(ctx, t, pollInfo, &waE2E.PollVoteMessage{
		SelectedOptions: HashPollOptions(optionNames),
	})
	return &waE2E.Message{PollUpdateMessage: pollUpdate}, err
}

// BuildPollCreation monta uma mensagem de criacao de enquete.
//
// `send.MessageSecretSize` e' importado, e nao redeclarado: e' o mesmo tamanho
// de segredo que o caminho de envio gera em send/prepare.go, e ter um unico
// dono do valor e' o que impede entrada e saida de divergirem.
func BuildPollCreation(name string, optionNames []string, selectableOptionCount int) *waE2E.Message {
	msgSecret := random.Bytes(send.MessageSecretSize)
	if selectableOptionCount < 0 || selectableOptionCount > len(optionNames) {
		selectableOptionCount = 0
	}
	options := make([]*waE2E.PollCreationMessage_Option, len(optionNames))
	for i, option := range optionNames {
		options[i] = &waE2E.PollCreationMessage_Option{OptionName: proto.String(option)}
	}
	return &waE2E.Message{
		PollCreationMessage: &waE2E.PollCreationMessage{
			Name:                   proto.String(name),
			Options:                options,
			SelectableOptionsCount: proto.Uint32(uint32(selectableOptionCount)),
		},
		MessageContextInfo: &waE2E.MessageContextInfo{
			MessageSecret: msgSecret,
		},
	}
}

// EncryptPollVote cifra um voto de enquete.
//
// A escolha do proprio JID depende do servidor do CRIADOR da enquete: se ele
// esta' em @s.whatsapp.net, o voto e' assinado com o PN; caso contrario, com o
// LID. Isso e' entrada de derivacao de chave — trocar quebra a decifragem do
// voto pelo outro lado.
func EncryptPollVote(ctx context.Context, t Transport, pollInfo *types.MessageInfo, vote *waE2E.PollVoteMessage) (*waE2E.PollUpdateMessage, error) {
	plaintext, err := proto.Marshal(vote)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal poll vote protobuf: %w", err)
	}
	ownID := t.OwnLID()
	if pollInfo.Sender.Server == types.DefaultUserServer {
		ownID = t.OwnID()
	}
	ciphertext, iv, err := EncryptSecret(ctx, t, ownID, pollInfo.Chat, pollInfo.Sender, pollInfo.ID, EncSecretPollVote, plaintext)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt poll vote: %w", err)
	}
	return &waE2E.PollUpdateMessage{
		PollCreationMessageKey: KeyFromInfo(pollInfo),
		Vote: &waE2E.PollEncValue{
			EncPayload: ciphertext,
			EncIV:      iv,
		},
		SenderTimestampMS: proto.Int64(time.Now().UnixMilli()),
	}, nil
}
