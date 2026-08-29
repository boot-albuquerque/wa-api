package chat

import (
	"encoding/json"
	"testing"

	waE2E "wa-api/internal/noise/protocol/proto/waE2E"
	"wa-api/internal/noise/protocol/types/events"

	"google.golang.org/protobuf/proto"
)

// TestApplyForwardContext_TextNoScore: forwarding a plain text message that
// was never forwarded. Score should become 1 (first forward), IsForwarded
// true. The Conversation field must be converted to ExtendedTextMessage
// because Conversation has no ContextInfo.
func TestApplyForwardContext_TextNoScore(t *testing.T) {
	msg := &waE2E.Message{
		Conversation: proto.String("hello world"),
	}
	applyForwardContext(msg)

	if msg.Conversation != nil {
		t.Error("Conversation should be nil after conversion to ExtendedTextMessage")
	}
	etm := msg.GetExtendedTextMessage()
	if etm == nil {
		t.Fatal("ExtendedTextMessage should be set")
	}
	if etm.GetText() != "hello world" {
		t.Errorf("text: got %q, want %q", etm.GetText(), "hello world")
	}
	ci := etm.GetContextInfo()
	if ci == nil {
		t.Fatal("ContextInfo should be set")
	}
	if !ci.GetIsForwarded() {
		t.Error("IsForwarded should be true")
	}
	if ci.GetForwardingScore() != 1 {
		t.Errorf("ForwardingScore: got %d, want 1", ci.GetForwardingScore())
	}
}

// TestApplyForwardContext_ScoreIncremented: a message with existing score 3
// becomes 4 on forward. This is the central assertion — the score is
// DERIVED, not caller-supplied.
func TestApplyForwardContext_ScoreIncremented(t *testing.T) {
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String("already forwarded"),
			ContextInfo: &waE2E.ContextInfo{
				IsForwarded:     proto.Bool(true),
				ForwardingScore: proto.Uint32(3),
			},
		},
	}
	applyForwardContext(msg)

	ci := msg.GetExtendedTextMessage().GetContextInfo()
	if ci.GetForwardingScore() != 4 {
		t.Errorf("ForwardingScore: got %d, want 4 (3+1)", ci.GetForwardingScore())
	}
	if !ci.GetIsForwarded() {
		t.Error("IsForwarded should stay true")
	}
}

// NEGATIVE CONTROL for TestApplyForwardContext_ScoreIncremented:
// If the increment is replaced by "use caller's score", this test fails.
// Mutation: change `existingScore + 1` to a fixed value like `99`.
// Expected failure: "ForwardingScore: got 99, want 4 (3+1)".

// TestApplyForwardContext_ImageMediaPreserved: all media encryption fields
// survive the forward. No re-upload needed.
func TestApplyForwardContext_ImageMediaPreserved(t *testing.T) {
	mediaKey := []byte("01234567890123456789012345678901")
	encSHA := []byte("abcdefghijklmnopqrstuvwxyz012345")
	fileSHA := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ012345")

	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			URL:           proto.String("https://mmg.whatsapp.net/test"),
			DirectPath:    proto.String("/o1/v/t24/test"),
			MediaKey:      mediaKey,
			FileEncSHA256: encSHA,
			FileSHA256:    fileSHA,
			Mimetype:      proto.String("image/jpeg"),
			FileLength:    proto.Uint64(12345),
		},
	}
	applyForwardContext(msg)

	img := msg.GetImageMessage()
	if img == nil {
		t.Fatal("ImageMessage should still be set")
	}

	if img.GetURL() != "https://mmg.whatsapp.net/test" {
		t.Errorf("URL changed: %q", img.GetURL())
	}
	if img.GetDirectPath() != "/o1/v/t24/test" {
		t.Errorf("DirectPath changed: %q", img.GetDirectPath())
	}
	if !bytesEqual(img.GetMediaKey(), mediaKey) {
		t.Error("MediaKey changed")
	}
	if !bytesEqual(img.GetFileEncSHA256(), encSHA) {
		t.Error("FileEncSHA256 changed")
	}
	if !bytesEqual(img.GetFileSHA256(), fileSHA) {
		t.Error("FileSHA256 changed")
	}
	if img.GetMimetype() != "image/jpeg" {
		t.Errorf("Mimetype changed: %q", img.GetMimetype())
	}
	if img.GetFileLength() != 12345 {
		t.Errorf("FileLength changed: %d", img.GetFileLength())
	}

	ci := img.GetContextInfo()
	if ci == nil {
		t.Fatal("ContextInfo should be set on image")
	}
	if !ci.GetIsForwarded() {
		t.Error("IsForwarded should be true")
	}
	if ci.GetForwardingScore() != 1 {
		t.Errorf("ForwardingScore: got %d, want 1 (first forward)", ci.GetForwardingScore())
	}
}

// TestApplyForwardContext_JSONRoundTrip: the real path — datajson from the
// database is deserialized and then forwarded. Tests that the JSON round-trip
// preserves the message and that applyForwardContext works on the result.
//
// The datajson shape matches what eventhandler_message.go:322 produces:
// json.Marshal(events.Message) where events.Message has a Message field
// of type *waE2E.Message.
//
// Production path for write: pkg/infra/db/message_history.go:23 (SaveMessageToHistory).
func TestApplyForwardContext_JSONRoundTrip(t *testing.T) {
	original := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			URL:           proto.String("https://mmg.whatsapp.net/roundtrip"),
			DirectPath:    proto.String("/roundtrip/path"),
			MediaKey:      []byte("media-key-32-bytes-padding-here!"),
			FileEncSHA256: []byte("enc-sha256-32-bytes-padding-xxx!"),
			FileSHA256:    []byte("file-sha256-32-bytes-padding-xx!"),
			Mimetype:      proto.String("image/png"),
			FileLength:    proto.Uint64(99999),
			ContextInfo: &waE2E.ContextInfo{
				IsForwarded:     proto.Bool(true),
				ForwardingScore: proto.Uint32(2),
			},
		},
	}

	evt := events.Message{Message: original}
	jsonBytes, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var restored events.Message
	if err := json.Unmarshal(jsonBytes, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	msg := restored.Message
	if msg == nil {
		t.Fatal("restored Message is nil")
	}

	applyForwardContext(msg)

	img := msg.GetImageMessage()
	if img == nil {
		t.Fatal("ImageMessage gone after round-trip")
	}
	if !bytesEqual(img.GetMediaKey(), original.GetImageMessage().GetMediaKey()) {
		t.Error("MediaKey changed in round-trip")
	}
	if !bytesEqual(img.GetFileEncSHA256(), original.GetImageMessage().GetFileEncSHA256()) {
		t.Error("FileEncSHA256 changed in round-trip")
	}
	if !bytesEqual(img.GetFileSHA256(), original.GetImageMessage().GetFileSHA256()) {
		t.Error("FileSHA256 changed in round-trip")
	}
	if img.GetURL() != "https://mmg.whatsapp.net/roundtrip" {
		t.Errorf("URL: got %q", img.GetURL())
	}
	if img.GetDirectPath() != "/roundtrip/path" {
		t.Errorf("DirectPath: got %q", img.GetDirectPath())
	}

	ci := img.GetContextInfo()
	if ci.GetForwardingScore() != 3 {
		t.Errorf("ForwardingScore: got %d, want 3 (2+1)", ci.GetForwardingScore())
	}
	if !ci.GetIsForwarded() {
		t.Error("IsForwarded should be true")
	}
}

// TestApplyForwardContext_DocumentPreserved: document message fields survive.
func TestApplyForwardContext_DocumentPreserved(t *testing.T) {
	msg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			URL:           proto.String("https://mmg.whatsapp.net/doc"),
			DirectPath:    proto.String("/doc/path"),
			MediaKey:      []byte("doc-media-key-32-bytes-padding!!"),
			FileEncSHA256: []byte("doc-enc-sha256-32-bytes-paddin!"),
			FileSHA256:    []byte("doc-file-sha256-32-bytes-paddi!"),
			Mimetype:      proto.String("application/pdf"),
			FileLength:    proto.Uint64(54321),
			FileName:      proto.String("report.pdf"),
		},
	}
	applyForwardContext(msg)

	doc := msg.GetDocumentMessage()
	if doc.GetFileName() != "report.pdf" {
		t.Errorf("FileName: got %q", doc.GetFileName())
	}
	if doc.GetMimetype() != "application/pdf" {
		t.Errorf("Mimetype: got %q", doc.GetMimetype())
	}
	ci := doc.GetContextInfo()
	if ci == nil || ci.GetForwardingScore() != 1 || !ci.GetIsForwarded() {
		t.Errorf("forward context not applied: %+v", ci)
	}
}

// TestApplyForwardContext_AudioPreserved: audio message fields survive.
func TestApplyForwardContext_AudioPreserved(t *testing.T) {
	msg := &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL:      proto.String("https://mmg.whatsapp.net/audio"),
			MediaKey: []byte("audio-media-key-32-bytes-paddi!"),
			Mimetype: proto.String("audio/ogg; codecs=opus"),
			PTT:      proto.Bool(true),
			Seconds:  proto.Uint32(15),
		},
	}
	applyForwardContext(msg)

	audio := msg.GetAudioMessage()
	if !audio.GetPTT() {
		t.Error("PTT flag lost")
	}
	if audio.GetSeconds() != 15 {
		t.Errorf("Seconds: got %d", audio.GetSeconds())
	}
	ci := audio.GetContextInfo()
	if ci == nil || !ci.GetIsForwarded() || ci.GetForwardingScore() != 1 {
		t.Errorf("forward context: %+v", ci)
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
