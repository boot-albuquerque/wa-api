package storage

import (
	"strings"
	"testing"
	"time"
)

// TestGenerateS3Key verifies the date partition segments of the generated key.
// The reference layout in Go is "2006-01-02"; using "05"/"25" produced seconds
// and a non-layout literal, corrupting the month/day partitions.
func TestGenerateS3Key(t *testing.T) {
	fixed := time.Date(2024, time.March, 7, 15, 45, 59, 0, time.UTC)

	m := &S3Manager{}
	key := m.generateS3KeyAt(fixed, "user1", "5511999999999@s.whatsapp.net", "msg123", "image/jpeg", true)

	want := "users/user1/inbox/5511999999999_s.whatsapp.net/2024/03/07/images/msg123.jpg"
	if key != want {
		t.Fatalf("unexpected key\n got: %s\nwant: %s", key, want)
	}

	for _, seg := range []string{"/2024/", "/03/", "/07/"} {
		if !strings.Contains(key, seg) {
			t.Errorf("key %q missing date segment %q", key, seg)
		}
	}
	if strings.Contains(key, "/59/") {
		t.Errorf("key %q contains seconds where month was expected", key)
	}
}

// TestGenerateS3KeyUsesCurrentDate ensures the exported entrypoint delegates to
// the time-injected implementation and yields today's partitions.
func TestGenerateS3KeyCurrentDate(t *testing.T) {
	m := &S3Manager{}
	now := time.Now()
	key := m.GenerateS3Key("user1", "123@s.whatsapp.net", "msg1", "application/pdf", false)

	if !strings.Contains(key, "/"+now.Format("2006")+"/"+now.Format("01")+"/"+now.Format("02")+"/") {
		t.Errorf("key %q does not contain current date partition", key)
	}
}

// TestGenerateS3Key_SanitizesMessageID proves sec/F28: whatsmeow does not
// validate the format of MessageInfo.ID (confirmed against its source —
// types/jid.go's MessageID is a plain string alias, and message.go sets it
// straight from the incoming stanza's raw "id" attribute). An attacker who
// controls that attribute could otherwise inject "/" or ".." and traverse
// out of the direction/date partitions this key is built from.
func TestGenerateS3Key_SanitizesMessageID(t *testing.T) {
	fixed := time.Date(2024, time.March, 7, 0, 0, 0, 0, time.UTC)
	m := &S3Manager{}

	tests := []struct {
		name      string
		messageID string
	}{
		{"path traversal", "../../../etc/passwd"},
		{"absolute path injection", "/other-user/inbox/2024/01/01/images/x"},
		{"embedded slash", "foo/bar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := m.generateS3KeyAt(fixed, "user1", "5511999999999@s.whatsapp.net", tt.messageID, "image/jpeg", true)

			if !strings.HasPrefix(key, "users/user1/inbox/5511999999999_s.whatsapp.net/2024/03/07/images/") {
				t.Fatalf("key %q escaped its own partition prefix for messageID %q", key, tt.messageID)
			}
			if strings.Contains(key, "..") {
				t.Errorf("key %q still contains \"..\" for messageID %q", key, tt.messageID)
			}
			// Exactly one "/" after the fixed prefix (the extension has none):
			// anything more means the sanitized messageID smuggled in extra
			// path segments.
			suffix := strings.TrimPrefix(key, "users/user1/inbox/5511999999999_s.whatsapp.net/2024/03/07/images/")
			if strings.Contains(suffix, "/") {
				t.Errorf("key %q has extra path segments after sanitization, suffix=%q", key, suffix)
			}
		})
	}
}

func TestSanitizeS3KeyComponent(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"clean id passes through", "msg123", "msg123"},
		{"slash replaced", "a/b", "a_b"},
		{"dot-dot-slash replaced entirely", "../secret", "___secret"},
		{"backslash replaced", `a\b`, "a_b"},
		{"all dots replaced", "...", "___"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeS3KeyComponent(tt.input); got != tt.want {
				t.Errorf("sanitizeS3KeyComponent(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
