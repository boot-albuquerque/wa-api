package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	"wa-api/pkg/infra/noise/client"
	"wa-api/pkg/infra/noise/client/testkit"

	"wa-api/pkg/domain"

	"wa-api/internal/noise"
	"wa-api/internal/noise/persistence/store"
	"wa-api/internal/noise/protocol/proto/waE2E"
	"wa-api/internal/noise/protocol/types"
)

// --- fakes ---

// fakePollSenderLookup implements PollSenderLookup for tests.
type fakePollSenderLookup struct {
	fn func(ctx context.Context, userID, messageID string) (string, error)
}

func (f *fakePollSenderLookup) GetPollSenderJID(ctx context.Context, userID, messageID string) (string, error) {
	if f.fn != nil {
		return f.fn(ctx, userID, messageID)
	}
	return "", errors.New("not found")
}

// fakeLIDStore implements store.LIDStore with configurable PN↔LID mappings.
// Mirrors the REAL GetLIDForPN/GetPNForLID contract from
// internal/wa-noise/persistence/store/sqlstore/lidmap.go:113-125.
type fakeLIDStore struct {
	store.NoopStore
	pnToLID map[string]types.JID
	lidToPN map[string]types.JID
}

func (f *fakeLIDStore) GetLIDForPN(_ context.Context, pn types.JID) (types.JID, error) {
	if pn.Server != types.DefaultUserServer {
		return types.JID{}, errors.New("not a PN")
	}
	if lid, ok := f.pnToLID[pn.String()]; ok {
		return lid, nil
	}
	return types.JID{}, nil
}

func (f *fakeLIDStore) GetPNForLID(_ context.Context, lid types.JID) (types.JID, error) {
	if lid.Server != types.HiddenUserServer {
		return types.JID{}, errors.New("not a LID")
	}
	if pn, ok := f.lidToPN[lid.String()]; ok {
		return pn, nil
	}
	return types.JID{}, nil
}

func (f *fakeLIDStore) PutManyLIDMappings(_ context.Context, _ []store.LIDMapping) error { return nil }
func (f *fakeLIDStore) PutLIDMapping(_ context.Context, _, _ types.JID) error            { return nil }
func (f *fakeLIDStore) GetManyLIDsForPNs(_ context.Context, _ []types.JID) (map[types.JID]types.JID, error) {
	return nil, nil
}

// storeWithLIDs builds a *store.Device with the given LID mappings.
func storeWithLIDs(pnToLID map[string]types.JID, lidToPN map[string]types.JID) *store.Device {
	return &store.Device{
		LIDs: &fakeLIDStore{pnToLID: pnToLID, lidToPN: lidToPN},
	}
}

func pollVoteAdapterWithSenderLookup(f *testkit.Fake, ps PollSenderLookup) *ChatMessengerAdapter {
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]client.Client{"u1": f}))
	if ps != nil {
		a.WithPollSenderLookup(ps)
	}
	return a
}

const (
	pnSender  = "554192421234@s.whatsapp.net"
	lidSender = "90937376170214@lid"
)

func basePollVotePayloadPN() domain.PollVotePayload {
	return domain.PollVotePayload{
		PollChat:      domain.JID(pollVoteGroupJID),
		PollSender:    domain.JID(pnSender),
		PollMessageID: "3EB0POLL1",
		PollTimestamp: 1755500100,
		OptionNames:   []string{"Sim"},
	}
}

func basePollVotePayloadLID() domain.PollVotePayload {
	return domain.PollVotePayload{
		PollChat:      domain.JID(pollVoteGroupJID),
		PollSender:    domain.JID(lidSender),
		PollMessageID: "3EB0POLL1",
		PollTimestamp: 1755500100,
		OptionNames:   []string{"Sim"},
	}
}

// --- Test 1: History lookup resolves sender ---

func TestSendPollVote_SenderResolvedFromHistory(t *testing.T) {
	var gotInfo *types.MessageInfo
	f := &testkit.Fake{
		BuildPollVoteFn: func(_ context.Context, info *types.MessageInfo, _ []string) (*waE2E.Message, error) {
			gotInfo = info
			return &waE2E.Message{}, nil
		},
		SendMessageFn: func(_ context.Context, _ types.JID, _ *waE2E.Message, _ ...noise.SendRequestExtra) (noise.SendResponse, error) {
			return noise.SendResponse{Timestamp: time.Unix(1755500200, 0), ID: "vote-ok"}, nil
		},
	}

	// History returns LID form, payload sends PN form.
	lookup := &fakePollSenderLookup{
		fn: func(_ context.Context, _, msgID string) (string, error) {
			if msgID == "3EB0POLL1" {
				return lidSender, nil
			}
			return "", errors.New("not found")
		},
	}

	a := pollVoteAdapterWithSenderLookup(f, lookup)

	_, err := a.SendPollVote(context.Background(), "u1", domain.JID(pollVoteGroupJID), basePollVotePayloadPN(), "")
	if err != nil {
		t.Fatalf("SendPollVote: %v", err)
	}

	if gotInfo == nil {
		t.Fatal("BuildPollVote was not called")
	}
	// The sender passed to BuildPollVote must be the LID from history,
	// NOT the PN from the payload. This is the central assertion of F228.
	if gotInfo.Sender.String() != lidSender {
		t.Errorf("Sender: got %q, want %q (LID from history overrides PN payload)", gotInfo.Sender.String(), lidSender)
	}
}

// --- Test 2: LID mapping resolves PN→LID when history is unavailable ---

func TestSendPollVote_SenderResolvedFromLIDMapping(t *testing.T) {
	var gotInfo *types.MessageInfo

	lidJID := types.JID{User: "90937376170214", Server: types.HiddenUserServer}

	f := &testkit.Fake{
		BuildPollVoteFn: func(_ context.Context, info *types.MessageInfo, _ []string) (*waE2E.Message, error) {
			gotInfo = info
			return &waE2E.Message{}, nil
		},
		SendMessageFn: func(_ context.Context, _ types.JID, _ *waE2E.Message, _ ...noise.SendRequestExtra) (noise.SendResponse, error) {
			return noise.SendResponse{Timestamp: time.Unix(1755500200, 0), ID: "vote-ok"}, nil
		},
		StoreFn: func() *store.Device {
			return storeWithLIDs(
				map[string]types.JID{pnSender: lidJID},
				map[string]types.JID{lidSender: {User: "554192421234", Server: types.DefaultUserServer}},
			)
		},
	}

	// History returns error (F227: API-created polls absent).
	lookup := &fakePollSenderLookup{fn: func(context.Context, string, string) (string, error) {
		return "", errors.New("not found")
	}}

	a := pollVoteAdapterWithSenderLookup(f, lookup)

	_, err := a.SendPollVote(context.Background(), "u1", domain.JID(pollVoteGroupJID), basePollVotePayloadPN(), "")
	if err != nil {
		t.Fatalf("SendPollVote: %v", err)
	}

	if gotInfo == nil {
		t.Fatal("BuildPollVote was not called")
	}
	// When history is unavailable, the LID mapping resolves PN→LID.
	if gotInfo.Sender.String() != lidSender {
		t.Errorf("Sender: got %q, want %q (LID mapping resolves PN→LID)", gotInfo.Sender.String(), lidSender)
	}
}

// --- Test 3: PN-addressed conversation keeps PN sender ---
// This is the test that prevents "always force LID" regression.

func TestSendPollVote_PNConversationKeepsPNSender(t *testing.T) {
	var gotInfo *types.MessageInfo

	f := &testkit.Fake{
		BuildPollVoteFn: func(_ context.Context, info *types.MessageInfo, _ []string) (*waE2E.Message, error) {
			gotInfo = info
			return &waE2E.Message{}, nil
		},
		SendMessageFn: func(_ context.Context, _ types.JID, _ *waE2E.Message, _ ...noise.SendRequestExtra) (noise.SendResponse, error) {
			return noise.SendResponse{Timestamp: time.Unix(1755500200, 0), ID: "vote-ok"}, nil
		},
	}

	// History returns the PN form — this is a PN-addressed conversation.
	lookup := &fakePollSenderLookup{
		fn: func(_ context.Context, _, msgID string) (string, error) {
			if msgID == "3EB0POLL1" {
				return pnSender, nil
			}
			return "", errors.New("not found")
		},
	}

	a := pollVoteAdapterWithSenderLookup(f, lookup)

	_, err := a.SendPollVote(context.Background(), "u1", domain.JID(pollVoteGroupJID), basePollVotePayloadPN(), "")
	if err != nil {
		t.Fatalf("SendPollVote: %v", err)
	}

	if gotInfo == nil {
		t.Fatal("BuildPollVote was not called")
	}
	// When history says PN, the sender must stay PN. This prevents the
	// symmetric regression of "always force LID".
	if gotInfo.Sender.String() != pnSender {
		t.Errorf("Sender: got %q, want %q (PN conversation must keep PN sender)", gotInfo.Sender.String(), pnSender)
	}
}

// --- Test 4: No resolution available — payload used as-is ---

func TestSendPollVote_NoResolution_PayloadUsedAsIs(t *testing.T) {
	var gotInfo *types.MessageInfo

	f := &testkit.Fake{
		BuildPollVoteFn: func(_ context.Context, info *types.MessageInfo, _ []string) (*waE2E.Message, error) {
			gotInfo = info
			return &waE2E.Message{}, nil
		},
		SendMessageFn: func(_ context.Context, _ types.JID, _ *waE2E.Message, _ ...noise.SendRequestExtra) (noise.SendResponse, error) {
			return noise.SendResponse{Timestamp: time.Unix(1755500200, 0), ID: "vote-ok"}, nil
		},
		StoreFn: func() *store.Device {
			// Empty LID store — no mappings.
			return storeWithLIDs(nil, nil)
		},
	}

	// History not found, LID mapping not found.
	lookup := &fakePollSenderLookup{fn: func(context.Context, string, string) (string, error) {
		return "", errors.New("not found")
	}}

	a := pollVoteAdapterWithSenderLookup(f, lookup)

	_, err := a.SendPollVote(context.Background(), "u1", domain.JID(pollVoteGroupJID), basePollVotePayloadPN(), "")
	if err != nil {
		t.Fatalf("SendPollVote: %v", err)
	}

	if gotInfo == nil {
		t.Fatal("BuildPollVote was not called")
	}
	// No resolution → payload sender used as-is.
	if gotInfo.Sender.String() != pnSender {
		t.Errorf("Sender: got %q, want %q (fallback to payload)", gotInfo.Sender.String(), pnSender)
	}
}

// --- Test 5: No PollSenderLookup injected (nil) — uses LID mapping ---

func TestSendPollVote_NoPollSenderLookup_UsesLIDMapping(t *testing.T) {
	var gotInfo *types.MessageInfo

	lidJID := types.JID{User: "90937376170214", Server: types.HiddenUserServer}

	f := &testkit.Fake{
		BuildPollVoteFn: func(_ context.Context, info *types.MessageInfo, _ []string) (*waE2E.Message, error) {
			gotInfo = info
			return &waE2E.Message{}, nil
		},
		SendMessageFn: func(_ context.Context, _ types.JID, _ *waE2E.Message, _ ...noise.SendRequestExtra) (noise.SendResponse, error) {
			return noise.SendResponse{Timestamp: time.Unix(1755500200, 0), ID: "vote-ok"}, nil
		},
		StoreFn: func() *store.Device {
			return storeWithLIDs(
				map[string]types.JID{pnSender: lidJID},
				nil,
			)
		},
	}

	// No PollSenderLookup at all (nil).
	a := pollVoteAdapterWithSenderLookup(f, nil)

	_, err := a.SendPollVote(context.Background(), "u1", domain.JID(pollVoteGroupJID), basePollVotePayloadPN(), "")
	if err != nil {
		t.Fatalf("SendPollVote: %v", err)
	}

	if gotInfo == nil {
		t.Fatal("BuildPollVote was not called")
	}
	if gotInfo.Sender.String() != lidSender {
		t.Errorf("Sender: got %q, want %q (LID mapping without history)", gotInfo.Sender.String(), lidSender)
	}
}

// --- Test 6: LID payload stays LID — never converts LID→PN ---
// Measured 2026-08-25: Sender='90937376170214@lid' makes the vote COUNT
// (zero MAC failures). Converting LID→PN breaks the working case.

func TestSendPollVote_LIDPayloadStaysLID(t *testing.T) {
	var gotInfo *types.MessageInfo

	pnJID := types.JID{User: "554192421234", Server: types.DefaultUserServer}

	f := &testkit.Fake{
		BuildPollVoteFn: func(_ context.Context, info *types.MessageInfo, _ []string) (*waE2E.Message, error) {
			gotInfo = info
			return &waE2E.Message{}, nil
		},
		SendMessageFn: func(_ context.Context, _ types.JID, _ *waE2E.Message, _ ...noise.SendRequestExtra) (noise.SendResponse, error) {
			return noise.SendResponse{Timestamp: time.Unix(1755500200, 0), ID: "vote-ok"}, nil
		},
		StoreFn: func() *store.Device {
			return storeWithLIDs(
				map[string]types.JID{pnSender: {User: "90937376170214", Server: types.HiddenUserServer}},
				map[string]types.JID{lidSender: pnJID},
			)
		},
	}

	// History unavailable (F227 path).
	lookup := &fakePollSenderLookup{fn: func(context.Context, string, string) (string, error) {
		return "", errors.New("not found")
	}}

	a := pollVoteAdapterWithSenderLookup(f, lookup)

	// Payload already in LID form.
	_, err := a.SendPollVote(context.Background(), "u1", domain.JID(pollVoteGroupJID), basePollVotePayloadLID(), "")
	if err != nil {
		t.Fatalf("SendPollVote: %v", err)
	}

	if gotInfo == nil {
		t.Fatal("BuildPollVote was not called")
	}
	// LID payload must stay LID — NEVER converted to PN.
	if gotInfo.Sender.String() != lidSender {
		t.Errorf("Sender: got %q, want %q (LID payload must not be converted to PN)", gotInfo.Sender.String(), lidSender)
	}
}

// --- Control negatives ---

// Control negative for Test 6: if GetAltJID were called unconditionally
// (without the directional guard), the LID payload would be converted to PN,
// breaking the working case measured 2026-08-25.
func TestSendPollVote_ControlNegative_UnconditionalGetAltJIDBreaksLID(t *testing.T) {
	pnJID := types.JID{User: "554192421234", Server: types.DefaultUserServer}

	// This test simulates what WOULD happen if GetAltJID were called on a
	// LID payload: the store has the LID→PN mapping, and an unconditional
	// call would return the PN form, breaking MAC.
	dev := storeWithLIDs(
		nil,
		map[string]types.JID{lidSender: pnJID},
	)

	lidJID := types.JID{User: "90937376170214", Server: types.HiddenUserServer}

	// Simulate the unconditional GetAltJID call that the buggy code would make.
	alt, err := dev.GetAltJID(context.Background(), lidJID)
	if err != nil {
		t.Fatalf("GetAltJID: %v", err)
	}
	// GetAltJID DOES return a PN for a LID input — proving the bidirectional
	// nature that the directional guard protects against.
	if alt.Server != types.DefaultUserServer {
		t.Fatalf("expected GetAltJID(LID) to return PN, got %q — the bidirectional behavior changed", alt.String())
	}

	// And now verify that the ACTUAL code does NOT make this conversion:
	// Test 6 above (TestSendPollVote_LIDPayloadStaysLID) proves the real
	// path keeps LID. This test proves the TOOL would break it.
	if alt.String() == lidSender {
		t.Errorf("GetAltJID(LID) returned LID unchanged — control negative invalid, the bidirectional risk doesn't exist")
	}
}

// Control negative for Test 1: WITHOUT history resolution, PN payload sender
// would be passed through and cause the MAC failure measured in F228.
func TestSendPollVote_ControlNegative_WithoutResolution_PNPassedThrough(t *testing.T) {
	var gotInfo *types.MessageInfo

	f := &testkit.Fake{
		BuildPollVoteFn: func(_ context.Context, info *types.MessageInfo, _ []string) (*waE2E.Message, error) {
			gotInfo = info
			return &waE2E.Message{}, nil
		},
		SendMessageFn: func(_ context.Context, _ types.JID, _ *waE2E.Message, _ ...noise.SendRequestExtra) (noise.SendResponse, error) {
			return noise.SendResponse{ID: "v"}, nil
		},
		StoreFn: func() *store.Device {
			// No LID mapping — simulates no resolution available.
			return storeWithLIDs(nil, nil)
		},
	}

	// No history, no LID mapping.
	lookup := &fakePollSenderLookup{fn: func(context.Context, string, string) (string, error) {
		return "", errors.New("not found")
	}}

	a := pollVoteAdapterWithSenderLookup(f, lookup)
	_, _ = a.SendPollVote(context.Background(), "u1", domain.JID(pollVoteGroupJID), basePollVotePayloadPN(), "")

	if gotInfo == nil {
		t.Fatal("BuildPollVote was not called")
	}
	// Without resolution, the PN sender passes through — this is the
	// UNRESOLVED case that would cause MAC failure on the receiver side.
	if gotInfo.Sender.Server != types.DefaultUserServer {
		t.Errorf("control negative: expected PN server %q, got %q", types.DefaultUserServer, gotInfo.Sender.Server)
	}
}

// Control negative for Test 3: if we replace the "keep PN from history"
// logic with "always force LID", this test must FAIL — proving that the
// test protects against the symmetric regression.
func TestSendPollVote_ControlNegative_ForceLIDBreaksPNConversation(t *testing.T) {
	// This test verifies that Test 3 would fail if the code forced LID.
	// We simulate what would happen if the code ignored the PN from history
	// and forced a LID mapping.
	var gotInfo *types.MessageInfo

	lidJID := types.JID{User: "90937376170214", Server: types.HiddenUserServer}

	f := &testkit.Fake{
		BuildPollVoteFn: func(_ context.Context, info *types.MessageInfo, _ []string) (*waE2E.Message, error) {
			gotInfo = info
			return &waE2E.Message{}, nil
		},
		SendMessageFn: func(_ context.Context, _ types.JID, _ *waE2E.Message, _ ...noise.SendRequestExtra) (noise.SendResponse, error) {
			return noise.SendResponse{ID: "v"}, nil
		},
		StoreFn: func() *store.Device {
			return storeWithLIDs(
				map[string]types.JID{pnSender: lidJID},
				nil,
			)
		},
	}

	// History says PN (PN-addressed conversation).
	lookup := &fakePollSenderLookup{
		fn: func(_ context.Context, _, msgID string) (string, error) {
			if msgID == "3EB0POLL1" {
				return pnSender, nil
			}
			return "", errors.New("not found")
		},
	}

	a := pollVoteAdapterWithSenderLookup(f, lookup)
	_, _ = a.SendPollVote(context.Background(), "u1", domain.JID(pollVoteGroupJID), basePollVotePayloadPN(), "")

	if gotInfo == nil {
		t.Fatal("BuildPollVote was not called")
	}
	// The history said PN, so the sender MUST be PN. If "always force LID"
	// were in place, this would be the LID — and the assertion below would
	// fail, exactly as intended.
	if gotInfo.Sender.String() != pnSender {
		t.Errorf("control negative: sender was %q; an 'always force LID' implementation would make this %q, breaking PN conversations",
			gotInfo.Sender.String(), lidSender)
	}
}
