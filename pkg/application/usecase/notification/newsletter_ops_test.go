package notification

import (
	"context"
	"errors"
	"testing"
	"time"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// F231 — a duração da subscrição em SEGUNDOS, não em nanossegundos.
//
// O defeito: o inteiro por baixo de time.Duration é nanossegundos, e o
// encoding/json serializava-o tal e qual. Uma subscrição de 90 segundos saía
// 90000000000, que lido como segundos são 2854 anos.
//
// ONDE A F231 VIVE AGORA. Este ficheiro tinha três testes; dois deles
// serializavam `NewsletterResult` com `json.Marshal` e afirmavam a chave
// `duration_seconds` do corpo. Isso deixou de fazer sentido na migração para
// DTO: `NewsletterResult` já não tem etiquetas `json` e já não é o formato de
// fio. A conversão passou para o apresentador, e é lá que a F231 está travada —
// ver TestPresentNewsletterSubscribe_DuracaoEmSegundos em
// pkg/presentation/http/dto/newsletter.
//
// O que sobra AQUI é a metade que continua a ser da aplicação: que o use case
// entrega a duração que a porta lhe deu, sem a truncar nem a converter.

func TestNewsletterOps_Execute_CarregaDuracaoDaPorta(t *testing.T) {
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
	if result.Duration != 90*time.Second {
		t.Fatalf("Duration = %v, quero 90s", result.Duration)
	}
	// As outras cargas ficam vazias: uma subscrição não devolve canal nem
	// publicações, e um resultado que trouxesse ambos faria o apresentador
	// escolher a forma errada sem que nada o acusasse.
	if result.Metadata != nil || result.Messages != nil {
		t.Fatalf("subscribe encheu carga que não é dele: %+v", result)
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

// ---------------------------------------------------------------------------
// F233(b) — admin invite validation
// ---------------------------------------------------------------------------

func TestNewsletterOps_AdminInvite_RequiresJIDAndUserJID(t *testing.T) {
	nr := &contractsfake.NewsletterReader{}
	nr.SessionGuard = contractsfake.FailSession(nil)
	uc := NewNewsletterOpsUseCase(nr, &contractsfake.Logger{})

	_, err := uc.Execute(context.Background(), "u1", NewsletterRequest{
		Op:  NewsletterOpAdminInvite,
		JID: "120363000000000000@newsletter",
	})
	if err == nil {
		t.Fatal("expected validation error for missing userJID")
	}

	_, err = uc.Execute(context.Background(), "u1", NewsletterRequest{
		Op: NewsletterOpAdminInvite,
	})
	if err == nil {
		t.Fatal("expected validation error for missing jid")
	}

	_, err = uc.Execute(context.Background(), "u1", NewsletterRequest{
		Op:      NewsletterOpAdminInvite,
		JID:     "120363000000000000@newsletter",
		UserJID: "5516900000000@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("admin_invite with valid fields failed: %v", err)
	}
}

func TestNewsletterOps_AdminInviteAccept_RequiresJID(t *testing.T) {
	nr := &contractsfake.NewsletterReader{}
	nr.SessionGuard = contractsfake.FailSession(nil)
	uc := NewNewsletterOpsUseCase(nr, &contractsfake.Logger{})

	_, err := uc.Execute(context.Background(), "u1", NewsletterRequest{
		Op: NewsletterOpAdminInviteAccept,
	})
	if err == nil {
		t.Fatal("expected validation error for missing jid")
	}

	_, err = uc.Execute(context.Background(), "u1", NewsletterRequest{
		Op:  NewsletterOpAdminInviteAccept,
		JID: "120363000000000000@newsletter",
	})
	if err != nil {
		t.Fatalf("admin_invite_accept with valid jid failed: %v", err)
	}
}

func TestNewsletterOps_AdminInviteRevoke_RequiresJIDAndUserJID(t *testing.T) {
	nr := &contractsfake.NewsletterReader{}
	nr.SessionGuard = contractsfake.FailSession(nil)
	uc := NewNewsletterOpsUseCase(nr, &contractsfake.Logger{})

	_, err := uc.Execute(context.Background(), "u1", NewsletterRequest{
		Op:  NewsletterOpAdminInviteRevoke,
		JID: "120363000000000000@newsletter",
	})
	if err == nil {
		t.Fatal("expected validation error for missing userJID")
	}

	_, err = uc.Execute(context.Background(), "u1", NewsletterRequest{
		Op:      NewsletterOpAdminInviteRevoke,
		JID:     "120363000000000000@newsletter",
		UserJID: "5516900000000@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("admin_invite_revoke with valid fields failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// F262 — the five shared newsletter validation messages must be EN-US
// (root CLAUDE.md, "Idioma do código"). They were in Portuguese until
// 2026-08-27; this locks the translated text so it cannot silently regress.
//
// Negative control run 2026-08-27: reverting requireJID's message to "jid do
// canal é obrigatório" made TestNewsletterOps_ValidationMessagesAreEnglish
// fail with:
//
//	newsletter_ops_test.go:296: requireJID (op info, empty jid): message =
//	"jid do canal é obrigatório", want "channel jid is required"
//
// confirming the assertion actually inspects the live string and not a
// stale copy.
// ---------------------------------------------------------------------------

func TestNewsletterOps_ValidationMessagesAreEnglish(t *testing.T) {
	cases := []struct {
		name    string
		req     NewsletterRequest
		wantMsg string
	}{
		{
			name:    "requireJID (op info, empty jid)",
			req:     NewsletterRequest{Op: NewsletterOpInfo},
			wantMsg: "channel jid is required",
		},
		{
			name:    "missing_name (op create)",
			req:     NewsletterRequest{Op: NewsletterOpCreate},
			wantMsg: "channel name is required",
		},
		{
			name:    "missing_invite (op info_invite)",
			req:     NewsletterRequest{Op: NewsletterOpInfoInvite},
			wantMsg: "invite code is required",
		},
		{
			name: "missing_server_ids (op mark_viewed)",
			req: NewsletterRequest{
				Op:  NewsletterOpMarkViewed,
				JID: "120363000000000000@newsletter",
			},
			wantMsg: "at least one server_id is required",
		},
		{
			name: "missing_server_id (op react)",
			req: NewsletterRequest{
				Op:  NewsletterOpReact,
				JID: "120363000000000000@newsletter",
			},
			wantMsg: "server_id is required",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateNewsletter(tc.req)
			if err == nil {
				t.Fatalf("validateNewsletter(%+v) = nil, want an error", tc.req)
			}
			var appErr *apperr.AppError
			if !errors.As(err, &appErr) {
				t.Fatalf("validateNewsletter error is not *apperr.AppError: %v (%T)", err, err)
			}
			if appErr.Message != tc.wantMsg {
				t.Fatalf("%s: message = %q, want %q", tc.name, appErr.Message, tc.wantMsg)
			}
		})
	}
}
