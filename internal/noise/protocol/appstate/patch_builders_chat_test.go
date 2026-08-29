package appstate

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/noise/protocol/proto/waCommon"
	"wa-api/internal/noise/protocol/types"
)

var (
	testChat   = types.NewJID("5511999999999", types.DefaultUserServer)
	testSender = types.NewJID("5511888888888", types.DefaultUserServer)
)

func TestBuildMuteWithDuration(t *testing.T) {
	before := time.Now().Add(time.Hour).UnixMilli()
	info := BuildMute(testChat, true, time.Hour)

	if info.Type != WAPatchRegularHigh {
		t.Errorf("Type = %q, esperado %q", info.Type, WAPatchRegularHigh)
	}
	if len(info.Mutations) != 1 {
		t.Fatalf("len(Mutations) = %d, esperado 1", len(info.Mutations))
	}
	mut := info.Mutations[0]
	if mut.Version != mutationVersionMute {
		t.Errorf("Version = %d, esperado %d", mut.Version, mutationVersionMute)
	}
	wantIndex := []string{IndexMute, testChat.String()}
	if len(mut.Index) != 2 || mut.Index[0] != wantIndex[0] || mut.Index[1] != wantIndex[1] {
		t.Errorf("Index = %v, esperado %v", mut.Index, wantIndex)
	}
	action := mut.Value.GetMuteAction()
	if !action.GetMuted() {
		t.Error("Muted deveria ser true")
	}
	if action.GetMuteEndTimestamp() < before {
		t.Errorf("MuteEndTimestamp = %d, esperado >= %d", action.GetMuteEndTimestamp(), before)
	}
}

func TestBuildMuteForeverUsesSentinel(t *testing.T) {
	info := BuildMute(testChat, true, 0)
	if got := info.Mutations[0].Value.GetMuteAction().GetMuteEndTimestamp(); got != muteForeverEndTimestamp {
		t.Errorf("MuteEndTimestamp = %d, esperado %d", got, muteForeverEndTimestamp)
	}
}

func TestBuildMuteAbsUnmuteKeepsNilTimestamp(t *testing.T) {
	info := BuildMuteAbs(testChat, false, nil)
	action := info.Mutations[0].Value.GetMuteAction()
	if action.GetMuted() {
		t.Error("Muted deveria ser false")
	}
	if action.MuteEndTimestamp != nil {
		t.Errorf("MuteEndTimestamp = %d, esperado nil ao desmutar", *action.MuteEndTimestamp)
	}
}

func TestBuildMuteAbsPreservesGivenTimestamp(t *testing.T) {
	info := BuildMuteAbs(testChat, true, proto.Int64(1234))
	if got := info.Mutations[0].Value.GetMuteAction().GetMuteEndTimestamp(); got != 1234 {
		t.Errorf("MuteEndTimestamp = %d, esperado 1234", got)
	}
}

func TestBuildPin(t *testing.T) {
	info := BuildPin(testChat, true)
	if info.Type != WAPatchRegularLow {
		t.Errorf("Type = %q, esperado %q", info.Type, WAPatchRegularLow)
	}
	mut := info.Mutations[0]
	if mut.Index[0] != IndexPin || mut.Version != mutationVersionPin {
		t.Errorf("Index=%v Version=%d", mut.Index, mut.Version)
	}
	if !mut.Value.GetPinAction().GetPinned() {
		t.Error("Pinned deveria ser true")
	}
}

func TestBuildArchiveAlsoUnpins(t *testing.T) {
	info := BuildArchive(testChat, true, time.Time{}, nil)
	if info.Type != WAPatchRegularLow {
		t.Errorf("Type = %q", info.Type)
	}
	if len(info.Mutations) != 2 {
		t.Fatalf("len(Mutations) = %d, esperado 2 (archive + unpin)", len(info.Mutations))
	}
	if info.Mutations[0].Index[0] != IndexArchive || info.Mutations[0].Version != mutationVersionArchive {
		t.Errorf("mutacao de archive inesperada: %v", info.Mutations[0].Index)
	}
	if !info.Mutations[0].Value.GetArchiveChatAction().GetArchived() {
		t.Error("Archived deveria ser true")
	}
	if info.Mutations[1].Index[0] != IndexPin {
		t.Errorf("segunda mutacao = %v, esperado pin", info.Mutations[1].Index)
	}
	if info.Mutations[1].Value.GetPinAction().GetPinned() {
		t.Error("arquivar deveria desafixar (Pinned=false)")
	}
}

func TestBuildArchiveUnarchiveDoesNotTouchPin(t *testing.T) {
	info := BuildArchive(testChat, false, time.Time{}, nil)
	if len(info.Mutations) != 1 {
		t.Fatalf("len(Mutations) = %d, esperado 1", len(info.Mutations))
	}
	if info.Mutations[0].Value.GetArchiveChatAction().GetArchived() {
		t.Error("Archived deveria ser false")
	}
}

func TestBuildMarkChatAsRead(t *testing.T) {
	info := BuildMarkChatAsRead(testChat, true, time.Time{}, nil)
	if info.Type != WAPatchRegularLow {
		t.Errorf("Type = %q", info.Type)
	}
	mut := info.Mutations[0]
	if mut.Index[0] != IndexMarkChatAsRead || mut.Version != mutationVersionMarkChatAsRead {
		t.Errorf("Index=%v Version=%d", mut.Index, mut.Version)
	}
	if !mut.Value.GetMarkChatAsReadAction().GetRead() {
		t.Error("Read deveria ser true")
	}
}

func TestBuildDeleteChatEncodesDeleteMediaFlag(t *testing.T) {
	withMedia := BuildDeleteChat(testChat, time.Time{}, nil, true)
	if withMedia.Type != WAPatchRegularHigh {
		t.Errorf("Type = %q", withMedia.Type)
	}
	mut := withMedia.Mutations[0]
	if mut.Version != mutationVersionDeleteChat {
		t.Errorf("Version = %d, esperado %d", mut.Version, mutationVersionDeleteChat)
	}
	if len(mut.Index) != 3 || mut.Index[2] != indexBoolTrue {
		t.Errorf("Index = %v, esperado flag %q ao final", mut.Index, indexBoolTrue)
	}

	withoutMedia := BuildDeleteChat(testChat, time.Time{}, nil, false)
	if withoutMedia.Mutations[0].Index[2] != indexBoolFalse {
		t.Errorf("flag = %q, esperado %q", withoutMedia.Mutations[0].Index[2], indexBoolFalse)
	}
}

func TestNewMessageRangeDefaultsToNow(t *testing.T) {
	before := time.Now().Unix()
	rng := newMessageRange(time.Time{}, nil)
	if rng.GetLastMessageTimestamp() < before {
		t.Errorf("LastMessageTimestamp = %d, esperado >= %d", rng.GetLastMessageTimestamp(), before)
	}
	if len(rng.GetMessages()) != 0 {
		t.Error("sem lastMessageKey nao deveria haver Messages")
	}
}

func TestNewMessageRangeIncludesKey(t *testing.T) {
	ts := time.Unix(1700000000, 0)
	key := &waCommon.MessageKey{ID: proto.String("MSGID")}
	rng := newMessageRange(ts, key)

	if rng.GetLastMessageTimestamp() != ts.Unix() {
		t.Errorf("LastMessageTimestamp = %d, esperado %d", rng.GetLastMessageTimestamp(), ts.Unix())
	}
	if len(rng.GetMessages()) != 1 {
		t.Fatalf("len(Messages) = %d, esperado 1", len(rng.GetMessages()))
	}
	if rng.GetMessages()[0].GetKey().GetID() != "MSGID" {
		t.Errorf("chave = %q", rng.GetMessages()[0].GetKey().GetID())
	}
	if rng.GetMessages()[0].GetTimestamp() != ts.Unix() {
		t.Error("timestamp da mensagem deveria acompanhar o do range")
	}
}
