package notification

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// ---------------------------------------------------------------------------
// F271 — a jid that is PRESENT but impossible must be 400, not 500.
//
// Measured in the field on 2026-08-26, POST /newsletter/info:
//
//	{}                                    -> 400 missing_jid       (correct)
//	{"jid":"   "}                         -> 500 newsletter_failed (wanted 400)
//	{"jid":"nao-e-jid"}                   -> 500 newsletter_failed (wanted 400)
//	{"jid":"554192421234@s.whatsapp.net"} -> 500 newsletter_failed (wanted 400)
//
// Only the ABSENCE was refused. The three values below are the exact ones
// measured, not approximations of them: the whole point of the entry is that a
// blank, free text and a well formed jid from the WRONG server all reached the
// adapter and came back as an internal failure.
// ---------------------------------------------------------------------------

// measuredBadJIDs are the field values from the F271 entry.
var measuredBadJIDs = []domain.JID{
	"   ",
	"nao-e-jid",
	"554192421234@s.whatsapp.net",
}

// validChannelJID is the success control: without it, a rule strict enough to
// refuse every jid would pass every refusal test in this file.
const validChannelJID domain.JID = "120363000000000000@newsletter"

// opsRequiringChannelJID enumerates, by name, the fifteen operations that take
// a channel jid. It is written out instead of derived so that the test asserts
// the SET, not whatever the table happens to contain — a rule dropped from one
// row would otherwise silently shrink the coverage of every test below.
var opsRequiringChannelJID = []NewsletterOp{
	NewsletterOpInfo,
	NewsletterOpFollow,
	NewsletterOpUnfollow,
	NewsletterOpMute,
	NewsletterOpMessages,
	NewsletterOpUpdates,
	NewsletterOpMarkViewed,
	NewsletterOpReact,
	NewsletterOpSubscribe,
	NewsletterOpDemote,
	NewsletterOpChangeOwner,
	NewsletterOpDelete,
	NewsletterOpAdminInvite,
	NewsletterOpAdminInviteAccept,
	NewsletterOpAdminInviteRevoke,
}

// requestWithJID builds a request that satisfies every requirement of the
// operation EXCEPT the jid rule under test, so that a failure can only come
// from the jid. Filling the other fields matters: with them empty, an op like
// react would fail on missing_server_id and the test would read as green while
// never exercising the jid at all.
func requestWithJID(op NewsletterOp, jid domain.JID) NewsletterRequest {
	return NewsletterRequest{
		Op:         op,
		JID:        jid,
		ServerIDs:  []int{1},
		ServerID:   1,
		UserJID:    "5516900000000@s.whatsapp.net",
		ConfirmJID: jid,
	}
}

// appErrorOf unwraps the taxonomy error, which is what carries the code and
// the category the HTTP boundary turns into a status.
func appErrorOf(t *testing.T, err error) *apperr.AppError {
	t.Helper()
	if err == nil {
		t.Fatal("no error returned")
	}
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("error is not an *apperr.AppError: %v", err)
	}
	return appErr
}

// TestNewsletter_MalformedJID_IsValidation is the test of the defect: the three
// measured values, on all fifteen operations, must be refused as validation —
// which is what makes the boundary answer 400 instead of 500.
func TestNewsletter_MalformedJID_IsValidation(t *testing.T) {
	if len(opsRequiringChannelJID) != 15 {
		t.Fatalf("the channel-jid family has 15 operations, the list has %d", len(opsRequiringChannelJID))
	}

	for _, op := range opsRequiringChannelJID {
		for _, jid := range measuredBadJIDs {
			t.Run(string(op)+"/"+string(jid), func(t *testing.T) {
				err := validateNewsletter(requestWithJID(op, jid))
				appErr := appErrorOf(t, err)

				if appErr.Code != codeInvalidNewsletter {
					t.Fatalf("code = %q, want %q", appErr.Code, codeInvalidNewsletter)
				}
				if appErr.Category != apperr.CategoryValidation {
					t.Fatalf("category = %q, want %q (this is what produces 400)",
						appErr.Category, apperr.CategoryValidation)
				}
				if appErr.Retryable {
					t.Fatal("retryable = true; the same jid fails forever, so retrying is never right")
				}
			})
		}
	}
}

// TestNewsletter_MissingJID_StillMissingJID guards against trading one wrong
// code for another: the ABSENCE was already correct and must stay correct.
func TestNewsletter_MissingJID_StillMissingJID(t *testing.T) {
	for _, op := range opsRequiringChannelJID {
		t.Run(string(op), func(t *testing.T) {
			err := validateNewsletter(requestWithJID(op, ""))
			appErr := appErrorOf(t, err)

			if appErr.Code != codeMissingJID {
				t.Fatalf("code = %q, want %q", appErr.Code, codeMissingJID)
			}
			if appErr.Category != apperr.CategoryValidation {
				t.Fatalf("category = %q, want %q", appErr.Category, apperr.CategoryValidation)
			}
		})
	}
}

// TestNewsletter_ValidChannelJID_PassesValidation is the SUCCESS path. A rule
// that refused everything would satisfy both tests above; only this one says
// the rule still admits a real channel.
func TestNewsletter_ValidChannelJID_PassesValidation(t *testing.T) {
	for _, op := range opsRequiringChannelJID {
		t.Run(string(op), func(t *testing.T) {
			if err := validateNewsletter(requestWithJID(op, validChannelJID)); err != nil {
				t.Fatalf("valid channel jid refused: %v", err)
			}
		})
	}
}

// TestNewsletter_DeleteRevealsMissingJIDFirst keeps the ORDER of validation
// that the field measured: `{"confirmJID":"x"}` answers missing_jid, not
// missing_confirm_jid. Inserting the new rule between the two would have been
// invisible to every other test here.
func TestNewsletter_DeleteValidationOrder(t *testing.T) {
	t.Run("absent jid reveals missing_jid before confirm", func(t *testing.T) {
		err := validateNewsletter(NewsletterRequest{Op: NewsletterOpDelete, ConfirmJID: "x"})
		if code := appErrorOf(t, err).Code; code != codeMissingJID {
			t.Fatalf("code = %q, want %q", code, codeMissingJID)
		}
	})
	t.Run("malformed jid is refused before confirm", func(t *testing.T) {
		err := validateNewsletter(NewsletterRequest{Op: NewsletterOpDelete, JID: "nao-e-jid", ConfirmJID: "x"})
		if code := appErrorOf(t, err).Code; code != codeInvalidNewsletter {
			t.Fatalf("code = %q, want %q", code, codeInvalidNewsletter)
		}
	})
	t.Run("valid jid with mismatched confirm still reveals missing_confirm_jid", func(t *testing.T) {
		err := validateNewsletter(NewsletterRequest{
			Op: NewsletterOpDelete, JID: validChannelJID, ConfirmJID: "999999@newsletter"})
		if code := appErrorOf(t, err).Code; code != "missing_confirm_jid" {
			t.Fatalf("code = %q, want %q", code, "missing_confirm_jid")
		}
	})
}

// TestNewsletter_OpsWithoutChannelJID_Unaffected: create takes no identifier
// and info_invite takes an invite CODE. Applying the channel rule to them
// would break two working routes.
func TestNewsletter_OpsWithoutChannelJID_Unaffected(t *testing.T) {
	if err := validateNewsletter(NewsletterRequest{Op: NewsletterOpCreate, Name: "Canal X"}); err != nil {
		t.Fatalf("create refused: %v", err)
	}
	if err := validateNewsletter(NewsletterRequest{Op: NewsletterOpInfoInvite, Invite: "AbCd1234"}); err != nil {
		t.Fatalf("info_invite refused: %v", err)
	}
}

// TestNewsletter_JIDRulesAreNeverSplit reads the table itself: every row that
// demands a jid must ALSO carry the malformed-jid rule. This is the test of the
// CAUSE — the defect was one rule where two were needed, and a future row
// written with `requireJID` alone would reintroduce it on that one operation
// while every other test in this file kept passing.
func TestNewsletter_JIDRulesAreNeverSplit(t *testing.T) {
	for op, rules := range newsletterRequirements {
		var hasMissing, hasInvalid bool
		for _, rule := range rules {
			switch rule.code {
			case codeMissingJID:
				hasMissing = true
			case codeInvalidNewsletter:
				hasInvalid = true
			}
		}
		if hasMissing != hasInvalid {
			t.Errorf("op %q: missing_jid=%v invalid_newsletter_jid=%v; the two rules must travel together",
				op, hasMissing, hasInvalid)
		}
	}
}

// ---------------------------------------------------------------------------
// Follow-up to F271 — "O que a correcção NÃO cobriu": userJID is required by
// four operations (demote, change_owner, admin_invite, admin_invite_revoke)
// and never gained a form rule, so the same measured values that broke the
// channel jid still reach the adapter through userJID and come back as
// `500 newsletter_failed`.
// ---------------------------------------------------------------------------

// measuredBadUserJIDs are the values measured in the field against
// POST /newsletter/admin-invite (HOUSEKEEP.md F271, follow-up section).
var measuredBadUserJIDs = []domain.JID{
	"   ",
	"nao-e-jid",
}

// validUserJID is the success control for the userJID rule.
const validUserJID domain.JID = "5516900000000@s.whatsapp.net"

// opsRequiringUserJID enumerates, by name, the four operations that take a
// target userJID. Written out rather than derived for the same reason
// opsRequiringChannelJID is: the test must assert the SET.
var opsRequiringUserJID = []NewsletterOp{
	NewsletterOpDemote,
	NewsletterOpChangeOwner,
	NewsletterOpAdminInvite,
	NewsletterOpAdminInviteRevoke,
}

// requestWithUserJID builds a request that satisfies every requirement of the
// operation EXCEPT the userJID rule under test.
func requestWithUserJID(op NewsletterOp, userJID domain.JID) NewsletterRequest {
	return NewsletterRequest{
		Op:      op,
		JID:     validChannelJID,
		UserJID: userJID,
	}
}

// TestNewsletter_MalformedUserJID_IsValidation is the test of the defect: the
// two measured values, on all four operations that take a userJID, must be
// refused as validation — which is what makes the boundary answer 400 instead
// of 500.
func TestNewsletter_MalformedUserJID_IsValidation(t *testing.T) {
	if len(opsRequiringUserJID) != 4 {
		t.Fatalf("the user-jid family has 4 operations, the list has %d", len(opsRequiringUserJID))
	}

	for _, op := range opsRequiringUserJID {
		for _, jid := range measuredBadUserJIDs {
			t.Run(string(op)+"/"+string(jid), func(t *testing.T) {
				err := validateNewsletter(requestWithUserJID(op, jid))
				appErr := appErrorOf(t, err)

				if appErr.Code != codeInvalidUserJID {
					t.Fatalf("code = %q, want %q", appErr.Code, codeInvalidUserJID)
				}
				if appErr.Category != apperr.CategoryValidation {
					t.Fatalf("category = %q, want %q (this is what produces 400)",
						appErr.Category, apperr.CategoryValidation)
				}
				if appErr.Retryable {
					t.Fatal("retryable = true; the same jid fails forever, so retrying is never right")
				}
			})
		}
	}
}

// TestNewsletter_MissingUserJID_StillMissingUserJID guards against trading one
// wrong code for another: the ABSENCE was already correct and must stay
// correct.
func TestNewsletter_MissingUserJID_StillMissingUserJID(t *testing.T) {
	for _, op := range opsRequiringUserJID {
		t.Run(string(op), func(t *testing.T) {
			err := validateNewsletter(requestWithUserJID(op, ""))
			appErr := appErrorOf(t, err)

			if appErr.Code != codeMissingUserJID {
				t.Fatalf("code = %q, want %q", appErr.Code, codeMissingUserJID)
			}
			if appErr.Category != apperr.CategoryValidation {
				t.Fatalf("category = %q, want %q", appErr.Category, apperr.CategoryValidation)
			}
		})
	}
}

// TestNewsletter_ValidUserJID_PassesValidation is the SUCCESS path. A rule
// that refused everything would satisfy both tests above; only this one says
// the rule still admits a real user jid.
func TestNewsletter_ValidUserJID_PassesValidation(t *testing.T) {
	for _, op := range opsRequiringUserJID {
		t.Run(string(op), func(t *testing.T) {
			if err := validateNewsletter(requestWithUserJID(op, validUserJID)); err != nil {
				t.Fatalf("valid user jid refused: %v", err)
			}
		})
	}
}

// TestNewsletter_UserJIDRulesAreNeverSplit reads the table itself: every row
// that demands a userJID must ALSO carry the malformed-userJID rule — the
// same causal test as TestNewsletter_JIDRulesAreNeverSplit, for the sibling
// field.
func TestNewsletter_UserJIDRulesAreNeverSplit(t *testing.T) {
	for op, rules := range newsletterRequirements {
		var hasMissing, hasInvalid bool
		for _, rule := range rules {
			switch rule.code {
			case codeMissingUserJID:
				hasMissing = true
			case codeInvalidUserJID:
				hasInvalid = true
			}
		}
		if hasMissing != hasInvalid {
			t.Errorf("op %q: missing_user_jid=%v invalid_user_jid=%v; the two rules must travel together",
				op, hasMissing, hasInvalid)
		}
	}
}

// TestNewsletter_MalformedUserJID_NeverReachesThePort closes the loop through
// Execute: the refusal has to happen BEFORE dispatch.
func TestNewsletter_MalformedUserJID_NeverReachesThePort(t *testing.T) {
	for _, jid := range measuredBadUserJIDs {
		t.Run(string(jid), func(t *testing.T) {
			nr := &contractsfake.NewsletterReader{}
			nr.SessionGuard = contractsfake.FailSession(nil)
			uc := NewNewsletterOpsUseCase(nr, &contractsfake.Logger{})

			_, err := uc.Execute(context.Background(), "u1",
				NewsletterRequest{Op: NewsletterOpAdminInvite, JID: validChannelJID, UserJID: jid})

			appErr := appErrorOf(t, err)
			if appErr.Code != codeInvalidUserJID {
				t.Fatalf("code = %q, want %q", appErr.Code, codeInvalidUserJID)
			}
			if len(nr.NewsletterCalls) != 0 {
				t.Fatalf("malformed user jid reached the port: %+v", nr.NewsletterCalls)
			}
		})
	}
}

// TestNewsletter_MalformedJID_NeverReachesThePort closes the loop through
// Execute: the refusal has to happen BEFORE dispatch, or the adapter still
// pays for the bad request even if the status is now right.
func TestNewsletter_MalformedJID_NeverReachesThePort(t *testing.T) {
	for _, jid := range measuredBadJIDs {
		t.Run(string(jid), func(t *testing.T) {
			nr := &contractsfake.NewsletterReader{}
			nr.SessionGuard = contractsfake.FailSession(nil)
			uc := NewNewsletterOpsUseCase(nr, &contractsfake.Logger{})

			_, err := uc.Execute(context.Background(), "u1",
				NewsletterRequest{Op: NewsletterOpInfo, JID: jid})

			appErr := appErrorOf(t, err)
			if appErr.Code != codeInvalidNewsletter {
				t.Fatalf("code = %q, want %q", appErr.Code, codeInvalidNewsletter)
			}
			if len(nr.NewsletterCalls) != 0 {
				t.Fatalf("malformed jid reached the port: %+v", nr.NewsletterCalls)
			}
		})
	}
}
