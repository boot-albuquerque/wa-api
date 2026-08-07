// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"go.mau.fi/util/random"
	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
)

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
	pollUpdate := vote.Message.GetPollUpdateMessage()
	if pollUpdate == nil {
		return nil, ErrNotPollUpdateMessage
	}
	plaintext, err := cli.decryptMsgSecret(ctx, vote, EncSecretPollVote, pollUpdate.GetVote(), pollUpdate.GetPollCreationMessageKey())
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

// HashPollOptions hashes poll option names using SHA-256 for voting.
// This is used by BuildPollVote to convert selected option names to hashes.
func HashPollOptions(optionNames []string) [][]byte {
	optionHashes := make([][]byte, len(optionNames))
	for i, option := range optionNames {
		optionHash := sha256.Sum256([]byte(option))
		optionHashes[i] = optionHash[:]
	}
	return optionHashes
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
	pollUpdate, err := cli.EncryptPollVote(ctx, pollInfo, &waE2E.PollVoteMessage{
		SelectedOptions: HashPollOptions(optionNames),
	})
	return &waE2E.Message{PollUpdateMessage: pollUpdate}, err
}

// BuildPollCreation builds a poll creation message with the given poll name, options and maximum number of selections.
// The built message can be sent normally using Client.SendMessage.
//
//	resp, err := cli.SendMessage(context.Background(), chat, cli.BuildPollCreation("meow?", []string{"yes", "no"}, 1))
func (cli *Client) BuildPollCreation(name string, optionNames []string, selectableOptionCount int) *waE2E.Message {
	msgSecret := random.Bytes(messageSecretSize)
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

// EncryptPollVote encrypts a poll vote message. This is a slightly lower-level function, using BuildPollVote is recommended.
func (cli *Client) EncryptPollVote(ctx context.Context, pollInfo *types.MessageInfo, vote *waE2E.PollVoteMessage) (*waE2E.PollUpdateMessage, error) {
	plaintext, err := proto.Marshal(vote)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal poll vote protobuf: %w", err)
	}
	ownID := cli.getOwnLID()
	if pollInfo.Sender.Server == types.DefaultUserServer {
		ownID = cli.getOwnID()
	}
	ciphertext, iv, err := cli.encryptMsgSecret(ctx, ownID, pollInfo.Chat, pollInfo.Sender, pollInfo.ID, EncSecretPollVote, plaintext)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt poll vote: %w", err)
	}
	return &waE2E.PollUpdateMessage{
		PollCreationMessageKey: getKeyFromInfo(pollInfo),
		Vote: &waE2E.PollEncValue{
			EncPayload: ciphertext,
			EncIV:      iv,
		},
		SenderTimestampMS: proto.Int64(time.Now().UnixMilli()),
	}, nil
}
