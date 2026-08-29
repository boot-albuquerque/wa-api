package bootstrap

import (
	"testing"

	"github.com/patrickmn/go-cache"

	"wa-api/internal/noise"
	"wa-api/internal/noise/persistence/store"
	waCommon "wa-api/internal/noise/protocol/proto/waCommon"
	waE2E "wa-api/internal/noise/protocol/proto/waE2E"
	"wa-api/internal/noise/protocol/proto/waHistorySync"
	waWeb "wa-api/internal/noise/protocol/proto/waWeb"
	"wa-api/internal/noise/protocol/types"
	"wa-api/internal/noise/protocol/types/events"
)

// F230. history = 0 must block BOTH paths: real-time AND sync. Before this
// fix, persistHistorySync never checked the flag, writing 20 755 rows with
// the setting at zero.

// handlerComHistoricoN creates a UserEventHandler with the given history
// limit. The WAClient carries a Store with an ID so persistHistorySync can
// resolve the account-owner JID without panicking.
func handlerComHistoricoN(t *testing.T, userID string, historyLimit int) *UserEventHandler {
	t.Helper()
	if appCtx.UserInfoCache == nil {
		appCtx.UserInfoCache = cache.New(cache.NoExpiration, cache.NoExpiration)
	}
	appCtx.UserInfoCache.Set(userID, *userValues(userID, historyLimit), cache.NoExpiration)
	t.Cleanup(func() { appCtx.UserInfoCache.Delete(userID) })

	ownerJID := types.NewJID("5511000000000", types.DefaultUserServer)
	dev := &store.Device{ID: &ownerJID}
	return &UserEventHandler{
		UserID:   userID,
		DB:       discardTestDB(t),
		WAClient: noise.NewClient(dev, nil),
	}
}

// syncConversation builds a minimal HistorySync conversation payload with
// one text message, suitable for feeding to persistHistorySync.
func syncConversation(chatJID, msgID, text string) []*waHistorySync.Conversation {
	ts := uint64(1724300000)
	convID := chatJID
	return []*waHistorySync.Conversation{{
		ID: &convID,
		Messages: []*waHistorySync.HistorySyncMsg{{
			Message: &waWeb.WebMessageInfo{
				Key: &waCommon.MessageKey{
					RemoteJID: &chatJID,
					FromMe:    boolProto(false),
					ID:        &msgID,
				},
				Message:          &waE2E.Message{Conversation: &text},
				MessageTimestamp: &ts,
			},
		}},
	}}
}

func countHistoryRows(t *testing.T, evh *UserEventHandler, userID string) int {
	t.Helper()
	var n int
	err := evh.DB.QueryRow(
		`SELECT COUNT(*) FROM message_history WHERE user_id = ?`, userID,
	).Scan(&n)
	if err != nil {
		t.Fatalf("count message_history: %v", err)
	}
	return n
}

// --- Test 1: history = 0 → sync does NOT save (central assertion) ---
//
// Negative control executed: removing the historyLimitForUser guard from
// persistHistorySync causes this test to fail with:
//
//   eventhandler_history_flag_test.go:84: history = 0 but sync saved 1 rows; want 0 (F230)

func TestHistorySync_FlagZero_DoesNotSave(t *testing.T) {
	evh := handlerComHistoricoN(t, "u-f230-zero", 0)
	chat := "5511999999999@s.whatsapp.net"
	evh.persistHistorySync(syncConversation(chat, "MSG-BLOCKED", "should not persist"))

	n := countHistoryRows(t, evh, "u-f230-zero")
	if n != 0 {
		t.Fatalf("history = 0 but sync saved %d rows; want 0 (F230)", n)
	}
}

// --- Test 2: history > 0 → sync saves, as before ---

func TestHistorySync_FlagPositive_Saves(t *testing.T) {
	evh := handlerComHistoricoN(t, "u-f230-pos", 100)
	chat := "5511999999999@s.whatsapp.net"
	evh.persistHistorySync(syncConversation(chat, "MSG-SAVED", "should persist"))

	n := countHistoryRows(t, evh, "u-f230-pos")
	if n != 1 {
		t.Fatalf("history = 100 but sync saved %d rows; want 1", n)
	}
}

// --- Test 3: real-time path still respects history = 0 (regression guard) ---

func TestRealTime_FlagZero_DoesNotSave(t *testing.T) {
	evh := handlerComHistoricoN(t, "u-f230-rt0", 0)
	evt := &events.Message{
		Info: types.MessageInfo{
			ID:   "RT-BLOCKED",
			Type: "text",
			MessageSource: types.MessageSource{
				Chat:   types.NewJID("5511999999999", types.DefaultUserServer),
				Sender: types.NewJID("5511888888888", types.DefaultUserServer),
			},
		},
		Message: &waE2E.Message{Conversation: proto("real-time blocked")},
	}
	evh.saveMessageHistory(evt, &eventState{postmap: map[string]any{}})

	n := countHistoryRows(t, evh, "u-f230-rt0")
	if n != 0 {
		t.Fatalf("real-time: history = 0 but saved %d rows; want 0", n)
	}
}

// --- Test 4: real-time path with history > 0 saves (regression guard) ---

func TestRealTime_FlagPositive_Saves(t *testing.T) {
	evh := handlerComHistoricoN(t, "u-f230-rt1", 50)
	evt := &events.Message{
		Info: types.MessageInfo{
			ID:   "RT-SAVED",
			Type: "text",
			MessageSource: types.MessageSource{
				Chat:   types.NewJID("5511999999999", types.DefaultUserServer),
				Sender: types.NewJID("5511888888888", types.DefaultUserServer),
			},
		},
		Message: &waE2E.Message{Conversation: proto("real-time saved")},
	}
	evh.saveMessageHistory(evt, &eventState{postmap: map[string]any{}})

	n := countHistoryRows(t, evh, "u-f230-rt1")
	if n != 1 {
		t.Fatalf("real-time: history = 50 but saved %d rows; want 1", n)
	}
}
