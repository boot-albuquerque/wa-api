package bootstrap

import (
	"strings"
	"testing"
	"time"
)

// TestProcessMediaGuardsNilCtx asserts the boundary contract that matters:
// processMedia must RETURN (not panic) when the media context is not
// configured, so that the caller — handleEvent — keeps running and still
// performs history persistence, webhook dispatch and RabbitMQ publish.
//
// The full event-handler boundary test is not viable here: handleEvent
// dereferences evh.WAClient (DecryptSecretEncryptedMessage), reads appCtx
// caches and writes message history to a *sqlx.DB, none of which can be
// stood up without a real wa-noise SDK session and a database. The
// observable proxy for the fix is the WARN line below plus the absence of a
// panic across the call.
func TestProcessMediaGuardsNilCtx(t *testing.T) {
	buf := captureLogInto(t)

	evh := &UserEventHandler{UserID: "user-42", Token: "tok"}
	postmap := map[string]interface{}{}

	// Belt-and-suspenders: wa-noise's Client.Download on a nil receiver
	// returns an error rather than panicking, so this guard is defense in
	// depth, not the mechanism that prevents a panic here. It still must
	// fire and skip cleanly instead of falling through to a real download.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("processMedia panicked instead of skipping: %v", r)
		}
	}()

	// msg is nil on purpose: the guard must fire before anything touches it.
	evh.processMedia(nil, "image/jpeg", ".jpg", 2*time.Minute,
		true, "5511999@s.whatsapp.net", "MSGID1",
		mediaS3Config{Enabled: "false", MediaDelivery: "base64"},
		postmap, nil)

	out := buf.String()
	if !strings.Contains(out, "media processing skipped: WhatsApp client not configured") {
		t.Fatalf("expected WARN guard line, got log output: %q", out)
	}
	if !strings.Contains(out, `"level":"warn"`) {
		t.Fatalf("expected guard to be logged at warn level, got: %q", out)
	}
	if len(postmap) != 0 {
		t.Fatalf("expected postmap untouched when media is skipped, got: %v", postmap)
	}
}
