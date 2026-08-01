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
