// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"

	"wa-api/internal/wa-noise/message"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// O dominio de enquete vive em internal/wa-noise/message/ desde a Fase F/G
// lote 9. Os metodos abaixo sao API publica do pacote e continuam na raiz.

// DecryptPollVote decrypts a poll update message. The vote itself includes SHA-256 hashes of the selected options.
//
//	if evt.Message.GetPollUpdateMessage() != nil {
//		pollVote, err := cli.DecryptPollVote(evt)
//		if err != nil {
//			fmt.Println(":(", err)
//			return
//		}
//		fmt.Println("Selected hashes:")
//		for _, hash := range pollVote.GetSelectedOptions() {
//			fmt.Printf("- %X\n", hash)
//		}
//	}
func (cli *Client) DecryptPollVote(ctx context.Context, vote *events.Message) (*waE2E.PollVoteMessage, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return message.DecryptPollVote(ctx, cli.msgT(), vote)
}

// HashPollOptions hashes poll option names using SHA-256 for voting.
// This is used by BuildPollVote to convert selected option names to hashes.
func HashPollOptions(optionNames []string) [][]byte {
	return message.HashPollOptions(optionNames)
}

// BuildPollVote builds a poll vote message using the given poll message info and option names.
// The built message can be sent normally using Client.SendMessage.
//
// For example, to vote for the first option after receiving a message event (*events.Message):
//
//	if evt.Message.GetPollCreationMessage() != nil {
//		pollVoteMsg, err := cli.BuildPollVote(&evt.Info, []string{evt.Message.GetPollCreationMessage().GetOptions()[0].GetOptionName()})
//		if err != nil {
//			fmt.Println(":(", err)
//			return
//		}
//		resp, err := cli.SendMessage(context.Background(), evt.Info.Chat, pollVoteMsg)
//	}
func (cli *Client) BuildPollVote(ctx context.Context, pollInfo *types.MessageInfo, optionNames []string) (*waE2E.Message, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return message.BuildPollVote(ctx, cli.msgT(), pollInfo, optionNames)
}

// BuildPollCreation builds a poll creation message with the given poll name, options and maximum number of selections.
// The built message can be sent normally using Client.SendMessage.
//
//	resp, err := cli.SendMessage(context.Background(), chat, cli.BuildPollCreation("meow?", []string{"yes", "no"}, 1))
func (cli *Client) BuildPollCreation(name string, optionNames []string, selectableOptionCount int) *waE2E.Message {
	return message.BuildPollCreation(name, optionNames, selectableOptionCount)
}

// EncryptPollVote encrypts a poll vote message. This is a slightly lower-level function, using BuildPollVote is recommended.
func (cli *Client) EncryptPollVote(ctx context.Context, pollInfo *types.MessageInfo, vote *waE2E.PollVoteMessage) (*waE2E.PollUpdateMessage, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return message.EncryptPollVote(ctx, cli.msgT(), pollInfo, vote)
}
