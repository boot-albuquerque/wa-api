package notification

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/domain"
)

// F231: duration_seconds must serialize as SECONDS, not nanoseconds.
//
// The defect: time.Duration's underlying integer is nanoseconds, and
// encoding/json serializes it as such. A 90-second subscription came out as
// 90000000000, which read as seconds is 2854 years.

func TestNewsletterResult_DurationSeconds_SerializesAsSeconds(t *testing.T) {
	r := NewsletterResult{
		DurationSeconds: int64((90 * time.Second).Seconds()),
		Status:          "sent",
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal = %v", err)
	}

	ds, ok := got["duration_seconds"]
	if !ok {
		t.Fatal("duration_seconds missing from JSON output")
	}
	if ds != float64(90) {
		t.Fatalf("duration_seconds = %v, want 90 (got nanoseconds?)", ds)
	}
}

// F231: zero duration is omitted by omitempty. This is correct: the field
// only has meaning for the subscribe operation, and non-subscribe operations
// return zero. Showing "duration_seconds: 0" on follow/unfollow/info would
// be noise.
func TestNewsletterResult_ZeroDuration_OmittedFromJSON(t *testing.T) {
	r := NewsletterResult{
		DurationSeconds: 0,
		Status:          "sent",
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal = %v", err)
	}

	if _, present := got["duration_seconds"]; present {
		t.Fatalf("duration_seconds present in JSON for zero duration; want omitted")
	}
}

// F231: the conversion in Execute must produce the right integer.
// 90 seconds → DurationSeconds = 90, not 90000000000.
//
// This test exercises the ACTUAL conversion path through Execute, not the
// struct directly. The port fake returns 90*time.Second from
// SubscribeNewsletterLiveUpdates, and the result must show 90 — not the
// nanosecond integer that encoding/json would produce from time.Duration.
func TestNewsletterOps_Execute_DurationInSeconds(t *testing.T) {
	nr := &contractsfake.NewsletterReader{
		SubscribeLiveFunc: func(_ context.Context, _ string, _ domain.JID) (time.Duration, error) {
			return 90 * time.Second, nil
		},
	}
	nr.SessionGuard = contractsfake.FailSession(nil)
	logger := &contractsfake.Logger{}

	uc := NewNewsletterOpsUseCase(nr, logger)
	result, err := uc.Execute(context.Background(), "u1", NewsletterRequest{
		Op:  NewsletterOpSubscribe,
		JID: "120363000000000000@newsletter",
	})
	if err != nil {
		t.Fatalf("Execute = %v", err)
	}
	if result.DurationSeconds != 90 {
		t.Fatalf("DurationSeconds = %d, want 90 (got nanoseconds?)", result.DurationSeconds)
	}

	b, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal = %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(b, &wire); err != nil {
		t.Fatalf("Unmarshal = %v", err)
	}
	if wire["duration_seconds"] != float64(90) {
		t.Fatalf("wire duration_seconds = %v, want 90", wire["duration_seconds"])
	}
}

// ---------------------------------------------------------------------------
// F233b — demote, change_owner, delete validation
// ---------------------------------------------------------------------------

func TestNewsletterOps_Demote_RequiresJIDAndUserJID(t *testing.T) {
	nr := &contractsfake.NewsletterReader{}
	nr.SessionGuard = contractsfake.FailSession(nil)
	uc := NewNewsletterOpsUseCase(nr, &contractsfake.Logger{})

	_, err := uc.Execute(context.Background(), "u1", NewsletterRequest{
		Op: NewsletterOpDemote,
	})
	if err == nil {
		t.Fatal("expected validation error for missing jid")
	}

	_, err = uc.Execute(context.Background(), "u1", NewsletterRequest{
		Op:  NewsletterOpDemote,
		JID: "120363000000000000@newsletter",
	})
	if err == nil {
		t.Fatal("expected validation error for missing userJID")
	}

	_, err = uc.Execute(context.Background(), "u1", NewsletterRequest{
		Op:      NewsletterOpDemote,
		JID:     "120363000000000000@newsletter",
		UserJID: "5516900000000@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("demote with valid fields failed: %v", err)
	}
}

func TestNewsletterOps_ChangeOwner_RequiresJIDAndUserJID(t *testing.T) {
	nr := &contractsfake.NewsletterReader{}
	nr.SessionGuard = contractsfake.FailSession(nil)
	uc := NewNewsletterOpsUseCase(nr, &contractsfake.Logger{})

	_, err := uc.Execute(context.Background(), "u1", NewsletterRequest{
		Op:  NewsletterOpChangeOwner,
		JID: "120363000000000000@newsletter",
	})
	if err == nil {
		t.Fatal("expected validation error for missing userJID")
	}

	_, err = uc.Execute(context.Background(), "u1", NewsletterRequest{
		Op:      NewsletterOpChangeOwner,
		JID:     "120363000000000000@newsletter",
		UserJID: "5516900000000@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("change_owner with valid fields failed: %v", err)
	}
}

func TestNewsletterOps_Delete_RequiresJIDAndConfirmJID(t *testing.T) {
	nr := &contractsfake.NewsletterReader{}
	nr.SessionGuard = contractsfake.FailSession(nil)
	uc := NewNewsletterOpsUseCase(nr, &contractsfake.Logger{})

	_, err := uc.Execute(context.Background(), "u1", NewsletterRequest{
		Op:  NewsletterOpDelete,
		JID: "120363000000000000@newsletter",
	})
	if err == nil {
		t.Fatal("expected validation error for missing confirmJID")
	}

	_, err = uc.Execute(context.Background(), "u1", NewsletterRequest{
		Op:         NewsletterOpDelete,
		JID:        "120363000000000000@newsletter",
		ConfirmJID: "999999@newsletter",
	})
	if err == nil {
		t.Fatal("expected validation error for mismatched confirmJID")
	}

	_, err = uc.Execute(context.Background(), "u1", NewsletterRequest{
		Op:         NewsletterOpDelete,
		JID:        "120363000000000000@newsletter",
		ConfirmJID: "120363000000000000@newsletter",
	})
	if err != nil {
		t.Fatalf("delete with matching confirmJID failed: %v", err)
	}
}
