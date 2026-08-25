package history

import (
	"encoding/json"
	"testing"
	"time"

	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
	"wa-api/pkg/infra/db"

	"github.com/jmoiron/sqlx"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

const (
	testUserID    = "u1"
	testChatJID   = "5511999999999@s.whatsapp.net"
	testSenderJID = "5511999999999:42@lid"
	testMessageID = "OUTGOING-001"
)

func newRecorder(t *testing.T, limit int) (*OutgoingRecorder, *sqlx.DB) {
	t.Helper()
	appDB := newSyncDB(t)
	storeDB := openSQLite(t)
	rec := NewOutgoingRecorder(appDB, storeDB, db.SaveMessageToHistory, db.TrimMessageHistory,
		func(string) int { return limit })
	return rec, appDB
}

func readDataJSON(t *testing.T, appDB *sqlx.DB, userID, messageID string) string {
	t.Helper()
	var raw string
	err := appDB.QueryRow(
		`SELECT datajson FROM message_history WHERE user_id = ? AND message_id = ?`,
		userID, messageID).Scan(&raw)
	if err != nil {
		t.Fatalf("reading datajson: %v", err)
	}
	return raw
}

func readSenderJID(t *testing.T, appDB *sqlx.DB, userID, messageID string) string {
	t.Helper()
	var raw string
	err := appDB.QueryRow(
		`SELECT sender_jid FROM message_history WHERE user_id = ? AND message_id = ?`,
		userID, messageID).Scan(&raw)
	if err != nil {
		t.Fatalf("reading sender_jid: %v", err)
	}
	return raw
}

func countRows(t *testing.T, appDB *sqlx.DB, userID, chatJID string) int {
	t.Helper()
	var n int
	err := appDB.QueryRow(
		`SELECT COUNT(*) FROM message_history WHERE user_id = ? AND chat_jid = ?`,
		userID, chatJID).Scan(&n)
	if err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	return n
}

func sampleTextMsg() *waE2E.Message {
	return &waE2E.Message{
		Conversation: proto.String("hello from API"),
	}
}

func samplePollMsg() *waE2E.Message {
	return &waE2E.Message{
		PollCreationMessage: &waE2E.PollCreationMessage{
			Name: proto.String("Favorite color?"),
			Options: []*waE2E.PollCreationMessage_Option{
				{OptionName: proto.String("Red")},
				{OptionName: proto.String("Blue")},
			},
			SelectableOptionsCount: proto.Uint32(1),
		},
	}
}

// T1 — Record persists a message whose datajson is usable by
// SendForwardedMessage (CAP-55) and GetPollSenderJID (F228).
func TestOutgoingRecorder_PersistsUsableDataJSON(t *testing.T) {
	rec, appDB := newRecorder(t, 100)
	ts := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)

	rec.Record(testUserID, testChatJID, testSenderJID, testMessageID, "text", "hello from API", sampleTextMsg(), ts)

	// sender_jid is the wire form, not "me" — this is what F228 reads
	gotSender := readSenderJID(t, appDB, testUserID, testMessageID)
	if gotSender != testSenderJID {
		t.Fatalf("sender_jid = %q, want %q", gotSender, testSenderJID)
	}

	raw := readDataJSON(t, appDB, testUserID, testMessageID)

	// datajson must deserialize into events.Message with a non-nil Message
	var evt events.Message
	if err := json.Unmarshal([]byte(raw), &evt); err != nil {
		t.Fatalf("unmarshal datajson: %v", err)
	}
	if evt.Message == nil {
		t.Fatal("datajson deserialized to events.Message with nil Message proto")
	}
	if evt.Message.GetConversation() != "hello from API" {
		t.Fatalf("proto text = %q, want %q", evt.Message.GetConversation(), "hello from API")
	}

	// Info fields
	if evt.Info.ID != types.MessageID(testMessageID) {
		t.Fatalf("Info.ID = %q, want %q", evt.Info.ID, testMessageID)
	}
	if !evt.Info.IsFromMe {
		t.Fatal("Info.IsFromMe should be true for outgoing message")
	}
	if evt.Info.Chat.String() != testChatJID {
		t.Fatalf("Info.Chat = %q, want %q", evt.Info.Chat.String(), testChatJID)
	}
	if evt.Info.Sender.String() != testSenderJID {
		t.Fatalf("Info.Sender = %q, want %q", evt.Info.Sender.String(), testSenderJID)
	}
}

// T2 — Record + forward round-trip: datajson produced by Record survives
// the same json.Unmarshal that SendForwardedMessage does, and the proto
// content is intact.
func TestOutgoingRecorder_ForwardRoundTrip(t *testing.T) {
	rec, appDB := newRecorder(t, 100)
	ts := time.Now()

	msg := sampleTextMsg()
	rec.Record(testUserID, testChatJID, testSenderJID, testMessageID, "text", "hello from API", msg, ts)

	raw := readDataJSON(t, appDB, testUserID, testMessageID)

	// Exact same deserialization that forward.go does
	var evt events.Message
	if err := json.Unmarshal([]byte(raw), &evt); err != nil {
		t.Fatalf("forward unmarshal failed: %v", err)
	}
	if evt.Message == nil {
		t.Fatal("forward path would fail: evt.Message is nil")
	}

	// The proto content must survive the round-trip
	if evt.Message.GetConversation() != "hello from API" {
		t.Fatalf("forward proto text = %q, want %q", evt.Message.GetConversation(), "hello from API")
	}
}

// T2b — Poll message round-trip: the poll proto survives the datajson
// round-trip, and sender_jid is available for F228.
func TestOutgoingRecorder_PollRoundTrip(t *testing.T) {
	rec, appDB := newRecorder(t, 100)
	ts := time.Now()

	rec.Record(testUserID, testChatJID, testSenderJID, "POLL-001", "poll", "Favorite color?", samplePollMsg(), ts)

	gotSender := readSenderJID(t, appDB, testUserID, "POLL-001")
	if gotSender != testSenderJID {
		t.Fatalf("poll sender_jid = %q, want %q (F228 needs the wire form)", gotSender, testSenderJID)
	}

	raw := readDataJSON(t, appDB, testUserID, "POLL-001")
	var evt events.Message
	if err := json.Unmarshal([]byte(raw), &evt); err != nil {
		t.Fatalf("poll datajson unmarshal: %v", err)
	}
	if evt.Message == nil || evt.Message.PollCreationMessage == nil {
		t.Fatal("poll proto did not survive datajson round-trip")
	}
	if evt.Message.PollCreationMessage.GetName() != "Favorite color?" {
		t.Fatalf("poll name = %q, want %q",
			evt.Message.PollCreationMessage.GetName(), "Favorite color?")
	}
}

// T3 — historyLimit=0 means the recorder does NOT persist.
func TestOutgoingRecorder_LimitZeroDoesNotSave(t *testing.T) {
	rec, appDB := newRecorder(t, 0)

	rec.Record(testUserID, testChatJID, testSenderJID, testMessageID, "text", "hello", sampleTextMsg(), time.Now())

	if n := countRows(t, appDB, testUserID, testChatJID); n != 0 {
		t.Fatalf("rows = %d, want 0 (limit is zero)", n)
	}
}

// T3b — negative limit also means disabled.
func TestOutgoingRecorder_LimitNegativeDoesNotSave(t *testing.T) {
	rec, appDB := newRecorder(t, -1)

	rec.Record(testUserID, testChatJID, testSenderJID, testMessageID, "text", "hello", sampleTextMsg(), time.Now())

	if n := countRows(t, appDB, testUserID, testChatJID); n != 0 {
		t.Fatalf("rows = %d, want 0 (limit is negative)", n)
	}
}

// T4 — trim works: with limit=2, only the 2 most recent messages survive.
func TestOutgoingRecorder_TrimKeepsLimitMessages(t *testing.T) {
	rec, appDB := newRecorder(t, 2)
	ts := time.Now()

	for i := 0; i < 4; i++ {
		id := "MSG-" + string(rune('A'+i))
		rec.Record(testUserID, testChatJID, testSenderJID, id, "text", "msg", sampleTextMsg(), ts.Add(time.Duration(i)*time.Second))
	}

	n := countRows(t, appDB, testUserID, testChatJID)
	if n != 2 {
		t.Fatalf("rows after trim = %d, want 2", n)
	}
}

// T5 — DB failure on save does not panic; Record returns silently.
func TestOutgoingRecorder_SaveFailureDoesNotPanic(t *testing.T) {
	appDB := openSQLite(t) // no schema — save will fail
	storeDB := openSQLite(t)
	rec := NewOutgoingRecorder(appDB, storeDB,
		db.SaveMessageToHistory, db.TrimMessageHistory,
		func(string) int { return 100 })

	// Must not panic
	rec.Record(testUserID, testChatJID, testSenderJID, testMessageID, "text", "hello", sampleTextMsg(), time.Now())
}

// T5b — DB failure on trim does not propagate: save succeeds, trim fails.
func TestOutgoingRecorder_TrimFailureDoesNotPanic(t *testing.T) {
	appDB := newSyncDB(t)
	brokenStoreDB := openSQLite(t) // no store schema — trim will fail
	rec := NewOutgoingRecorder(appDB, brokenStoreDB,
		db.SaveMessageToHistory, db.TrimMessageHistory,
		func(string) int { return 100 })

	rec.Record(testUserID, testChatJID, testSenderJID, testMessageID, "text", "hello", sampleTextMsg(), time.Now())

	// The message must still be persisted despite trim failure
	if n := countRows(t, appDB, testUserID, testChatJID); n != 1 {
		t.Fatalf("rows = %d, want 1 (save should succeed even if trim fails)", n)
	}
}

// T6 — Negative control: if sender_jid were "me" instead of the wire form,
// F228's GetPollSenderJID would return the wrong value. This test verifies
// that the defect (passing "me") WOULD break the assertion.
func TestOutgoingRecorder_NegativeControl_SenderNotMe(t *testing.T) {
	rec, appDB := newRecorder(t, 100)

	rec.Record(testUserID, testChatJID, testSenderJID, testMessageID, "text", "hello", sampleTextMsg(), time.Now())

	gotSender := readSenderJID(t, appDB, testUserID, testMessageID)
	if gotSender == "me" {
		t.Fatal("NEGATIVE CONTROL: sender_jid is 'me' — F228 would fail to resolve poll sender identity")
	}
}

// T6b — Negative control: if datajson were empty, SendForwardedMessage would
// fail to deserialize. This test verifies that datajson is NOT empty.
func TestOutgoingRecorder_NegativeControl_DataJSONNotEmpty(t *testing.T) {
	rec, appDB := newRecorder(t, 100)

	rec.Record(testUserID, testChatJID, testSenderJID, testMessageID, "text", "hello", sampleTextMsg(), time.Now())

	raw := readDataJSON(t, appDB, testUserID, testMessageID)
	if raw == "" || raw == "{}" {
		t.Fatal("NEGATIVE CONTROL: datajson is empty — SendForwardedMessage (CAP-55) would fail")
	}
}

// T6c — Negative control: inject the defect (saveFn that passes "me") and
// verify the sender_jid assertion catches it.
func TestOutgoingRecorder_NegativeControl_DefectDetected(t *testing.T) {
	appDB := newSyncDB(t)
	storeDB := openSQLite(t)

	// Simulate the OLD defect: save always writes "me" as sender_jid
	defectSave := func(dbh *sqlx.DB, userID, chatJID, senderJID, messageID, messageType, textContent, mediaLink, quotedMessageID, dataJSON, senderPushName string) error {
		return db.SaveMessageToHistory(dbh, userID, chatJID, "me", messageID, messageType, textContent, mediaLink, quotedMessageID, dataJSON, senderPushName)
	}
	rec := NewOutgoingRecorder(appDB, storeDB, defectSave, db.TrimMessageHistory,
		func(string) int { return 100 })

	rec.Record(testUserID, testChatJID, testSenderJID, testMessageID, "text", "hello", sampleTextMsg(), time.Now())

	gotSender := readSenderJID(t, appDB, testUserID, testMessageID)
	if gotSender != "me" {
		t.Fatal("defect injection should have written 'me'")
	}
	// This is the assertion that T1/T6 use — the defect WOULD make it fail
	if gotSender == testSenderJID {
		t.Fatal("defect was not detectable")
	}
}

// T6d — Negative control: inject the defect (empty datajson) and verify
// the forward path would fail.
func TestOutgoingRecorder_NegativeControl_EmptyDataJSONDetected(t *testing.T) {
	appDB := newSyncDB(t)
	storeDB := openSQLite(t)

	// Simulate the OLD defect: save always writes empty datajson
	defectSave := func(dbh *sqlx.DB, userID, chatJID, senderJID, messageID, messageType, textContent, mediaLink, quotedMessageID, dataJSON, senderPushName string) error {
		return db.SaveMessageToHistory(dbh, userID, chatJID, senderJID, messageID, messageType, textContent, mediaLink, quotedMessageID, "", senderPushName)
	}
	rec := NewOutgoingRecorder(appDB, storeDB, defectSave, db.TrimMessageHistory,
		func(string) int { return 100 })

	rec.Record(testUserID, testChatJID, testSenderJID, testMessageID, "text", "hello", sampleTextMsg(), time.Now())

	var raw string
	err := appDB.QueryRow(
		`SELECT COALESCE(datajson, '') FROM message_history WHERE user_id = ? AND message_id = ?`,
		testUserID, testMessageID).Scan(&raw)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if raw != "" {
		t.Fatal("defect injection should have written empty datajson")
	}

	// Forward path would fail on empty datajson
	var evt events.Message
	if err := json.Unmarshal([]byte(raw), &evt); err != nil {
		// Expected: empty string fails unmarshal — the defect is detectable
		return
	}
	if evt.Message == nil {
		return // Also detectable
	}
	t.Fatal("defect was not detectable by forward path")
}
