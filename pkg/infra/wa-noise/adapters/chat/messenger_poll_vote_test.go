package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	waclient "wa-api/pkg/infra/wa-noise/client"
	"wa-api/pkg/infra/wa-noise/client/testkit"

	"wa-api/pkg/domain"

	wanoise "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
)

const pollVoteGroupJID = "120363313346913103@g.us"
const pollVoteSenderJID = "5511999999999@s.whatsapp.net"

func pollVoteAdapter(f *testkit.Fake) *ChatMessengerAdapter {
	return NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": f}))
}

func validPollVotePayload() domain.PollVotePayload {
	return domain.PollVotePayload{
		PollChat:      domain.JID(pollVoteGroupJID),
		PollSender:    domain.JID(pollVoteSenderJID),
		PollMessageID: "3EB0POLL1",
		PollTimestamp: 1755500100,
		OptionNames:   []string{"12h"},
	}
}

func TestChatMessengerAdapter_SendPollVote_NoSession(t *testing.T) {
	a := NewChatMessengerAdapter(testkit.GetterWith(nil))

	_, err := a.SendPollVote(context.Background(), "u1", domain.JID(pollVoteGroupJID), validPollVotePayload(), "")
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("SendPollVote code = %q, want no_session", testkit.AppErrCode(err))
	}
}

func TestChatMessengerAdapter_SendPollVote_BuildPollVoteReceivesCorrectArgs(t *testing.T) {
	var gotInfo *types.MessageInfo
	var gotOptions []string

	f := &testkit.Fake{
		BuildPollVoteFn: func(_ context.Context, info *types.MessageInfo, optionNames []string) (*waE2E.Message, error) {
			gotInfo = info
			gotOptions = optionNames
			return &waE2E.Message{}, nil
		},
	}
	a := pollVoteAdapter(f)

	_, err := a.SendPollVote(context.Background(), "u1", domain.JID(pollVoteGroupJID), validPollVotePayload(), "")
	if err != nil {
		t.Fatalf("SendPollVote: %v", err)
	}

	if gotInfo == nil {
		t.Fatal("BuildPollVote nao foi chamado")
	}
	if gotInfo.Chat.String() != pollVoteGroupJID {
		t.Errorf("Chat: got %q, want %q", gotInfo.Chat.String(), pollVoteGroupJID)
	}
	if gotInfo.Sender.String() != pollVoteSenderJID {
		t.Errorf("Sender: got %q, want %q", gotInfo.Sender.String(), pollVoteSenderJID)
	}
	if !gotInfo.IsGroup {
		t.Error("IsGroup: got false, want true (group JID has GroupServer)")
	}
	if string(gotInfo.ID) != "3EB0POLL1" {
		t.Errorf("ID: got %q, want %q", gotInfo.ID, "3EB0POLL1")
	}
	if gotInfo.Timestamp.Unix() != 1755500100 {
		t.Errorf("Timestamp: got %d, want %d", gotInfo.Timestamp.Unix(), 1755500100)
	}
	if len(gotOptions) != 1 || gotOptions[0] != "12h" {
		t.Errorf("optionNames: got %q, want [12h]", gotOptions)
	}
}

func TestChatMessengerAdapter_SendPollVote_IsGroupFalseForDM(t *testing.T) {
	var gotInfo *types.MessageInfo

	f := &testkit.Fake{
		BuildPollVoteFn: func(_ context.Context, info *types.MessageInfo, _ []string) (*waE2E.Message, error) {
			gotInfo = info
			return &waE2E.Message{}, nil
		},
	}
	a := pollVoteAdapter(f)

	payload := validPollVotePayload()
	payload.PollChat = domain.JID("5511888888888@s.whatsapp.net")

	_, err := a.SendPollVote(context.Background(), "u1", domain.JID("5511888888888@s.whatsapp.net"), payload, "")
	if err != nil {
		t.Fatalf("SendPollVote: %v", err)
	}

	if gotInfo.IsGroup {
		t.Error("IsGroup: got true, want false (DM JID has DefaultUserServer)")
	}
}

func TestChatMessengerAdapter_SendPollVote_SendsToRecipient(t *testing.T) {
	var sentTo types.JID

	f := &testkit.Fake{
		BuildPollVoteFn: func(_ context.Context, _ *types.MessageInfo, _ []string) (*waE2E.Message, error) {
			return &waE2E.Message{}, nil
		},
		SendMessageFn: func(_ context.Context, to types.JID, _ *waE2E.Message, _ ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			sentTo = to
			return wanoise.SendResponse{
				Timestamp: time.Unix(1755500200, 0),
				ID:        "vote-msg-id",
			}, nil
		},
	}
	a := pollVoteAdapter(f)

	result, err := a.SendPollVote(context.Background(), "u1", domain.JID(pollVoteGroupJID), validPollVotePayload(), "")
	if err != nil {
		t.Fatalf("SendPollVote: %v", err)
	}

	if sentTo.String() != pollVoteGroupJID {
		t.Errorf("SendMessage recipient: got %q, want %q", sentTo.String(), pollVoteGroupJID)
	}
	if result.ID != "vote-msg-id" {
		t.Errorf("result.ID: got %q, want %q", result.ID, "vote-msg-id")
	}
	if result.Timestamp.Unix() != 1755500200 {
		t.Errorf("result.Timestamp: got %d, want %d", result.Timestamp.Unix(), 1755500200)
	}
}

func TestChatMessengerAdapter_SendPollVote_ClientSuppliedIDForwarded(t *testing.T) {
	var gotExtra []wanoise.SendRequestExtra

	f := &testkit.Fake{
		BuildPollVoteFn: func(_ context.Context, _ *types.MessageInfo, _ []string) (*waE2E.Message, error) {
			return &waE2E.Message{}, nil
		},
		SendMessageFn: func(_ context.Context, _ types.JID, _ *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			gotExtra = extra
			return wanoise.SendResponse{ID: "server-id"}, nil
		},
	}
	a := pollVoteAdapter(f)

	_, err := a.SendPollVote(context.Background(), "u1", domain.JID(pollVoteGroupJID), validPollVotePayload(), "client-id-42")
	if err != nil {
		t.Fatalf("SendPollVote: %v", err)
	}

	if len(gotExtra) != 1 || string(gotExtra[0].ID) != "client-id-42" {
		t.Errorf("extra: got %+v, want [{ID:client-id-42}]", gotExtra)
	}
}

func TestChatMessengerAdapter_SendPollVote_EmptyIDNoExtra(t *testing.T) {
	var gotExtra []wanoise.SendRequestExtra

	f := &testkit.Fake{
		BuildPollVoteFn: func(_ context.Context, _ *types.MessageInfo, _ []string) (*waE2E.Message, error) {
			return &waE2E.Message{}, nil
		},
		SendMessageFn: func(_ context.Context, _ types.JID, _ *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			gotExtra = extra
			return wanoise.SendResponse{}, nil
		},
	}
	a := pollVoteAdapter(f)

	_, err := a.SendPollVote(context.Background(), "u1", domain.JID(pollVoteGroupJID), validPollVotePayload(), "")
	if err != nil {
		t.Fatalf("SendPollVote: %v", err)
	}

	if len(gotExtra) != 0 {
		t.Errorf("extra: got %+v, want empty (no client ID)", gotExtra)
	}
}

func TestChatMessengerAdapter_SendPollVote_BuildError(t *testing.T) {
	buildErr := errors.New("encryption failed")
	f := &testkit.Fake{
		BuildPollVoteFn: func(_ context.Context, _ *types.MessageInfo, _ []string) (*waE2E.Message, error) {
			return nil, buildErr
		},
	}
	a := pollVoteAdapter(f)

	_, err := a.SendPollVote(context.Background(), "u1", domain.JID(pollVoteGroupJID), validPollVotePayload(), "")
	if !errors.Is(err, buildErr) {
		t.Errorf("SendPollVote error: got %v, want %v", err, buildErr)
	}
}

func TestChatMessengerAdapter_SendPollVote_SendError(t *testing.T) {
	sendErr := errors.New("server rejected vote")
	f := &testkit.Fake{
		BuildPollVoteFn: func(_ context.Context, _ *types.MessageInfo, _ []string) (*waE2E.Message, error) {
			return &waE2E.Message{}, nil
		},
		SendMessageFn: func(_ context.Context, _ types.JID, _ *waE2E.Message, _ ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			return wanoise.SendResponse{}, sendErr
		},
	}
	a := pollVoteAdapter(f)

	_, err := a.SendPollVote(context.Background(), "u1", domain.JID(pollVoteGroupJID), validPollVotePayload(), "")
	if !errors.Is(err, sendErr) {
		t.Errorf("SendPollVote error: got %v, want %v", err, sendErr)
	}
}
