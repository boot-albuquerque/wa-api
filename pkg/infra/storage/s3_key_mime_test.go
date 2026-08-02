package storage

import (
	"strings"
	"testing"
	"time"
)

// TestGenerateS3Key_MediaTypeAndExtension pins the two independent switches in
// generateS3KeyAt: the media-type folder (derived from the mime prefix) and the
// file extension (derived from a substring match). They disagree on purpose for
// some inputs — an "audio/ogg" lands in "audio" with ".ogg" — so both are
// asserted per case rather than inferred from one another.
func TestGenerateS3Key_MediaTypeAndExtension(t *testing.T) {
	fixed := time.Date(2024, time.March, 7, 0, 0, 0, 0, time.UTC)
	m := &S3Manager{}

	tests := []struct {
		mimeType  string
		wantMedia string
		wantExt   string
	}{
		{"image/jpeg", "images", ".jpg"},
		{"image/jpg", "images", ".jpg"},
		{"image/png", "images", ".png"},
		{"image/gif", "images", ".gif"},
		{"image/webp", "images", ".webp"},
		{"video/mp4", "videos", ".mp4"},
		{"video/webm", "videos", ".webm"},
		{"audio/ogg", "audio", ".ogg"},
		{"audio/opus", "audio", ".opus"},
		{"application/pdf", "documents", ".pdf"},
		{"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "documents", ".xlsx"},
		{"application/vnd.ms-excel", "documents", ".xls"},
		// "application/msword" would NOT match: the switch looks for the
		// substring "doc", and "msword" does not contain it. The real
		// WhatsApp docx mime does, via ".document".
		{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", "documents", ".doc"},
		{"application/msword", "documents", ".bin"},
		{"application/vnd.openxmlformats-officedocument.wordprocessingml.docx", "documents", ".docx"},
		{"application/octet-stream", "documents", ".bin"},
		{"", "documents", ".bin"},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			key := m.generateS3KeyAt(fixed, "user1", "5511@s.whatsapp.net", "msg1", tt.mimeType, false)

			if !strings.Contains(key, "/"+tt.wantMedia+"/") {
				t.Errorf("key %q missing media folder %q", key, tt.wantMedia)
			}
			if !strings.HasSuffix(key, tt.wantExt) {
				t.Errorf("key %q does not end with %q", key, tt.wantExt)
			}
		})
	}
}

// TestGenerateS3Key_Direction covers the outbox/inbox branch and the JID
// cleanup of both "@" and ":".
func TestGenerateS3Key_DirectionAndJIDCleanup(t *testing.T) {
	fixed := time.Date(2024, time.March, 7, 0, 0, 0, 0, time.UTC)
	m := &S3Manager{}

	out := m.generateS3KeyAt(fixed, "u", "5511:12@s.whatsapp.net", "m", "image/png", false)
	if !strings.HasPrefix(out, "users/u/outbox/5511_12_s.whatsapp.net/") {
		t.Errorf("outgoing key %q has wrong direction or uncleaned JID", out)
	}

	in := m.generateS3KeyAt(fixed, "u", "5511:12@s.whatsapp.net", "m", "image/png", true)
	if !strings.HasPrefix(in, "users/u/inbox/5511_12_s.whatsapp.net/") {
		t.Errorf("incoming key %q has wrong direction or uncleaned JID", in)
	}
}
