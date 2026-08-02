package user_test

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/domain"
)

func TestGetBlocklistUseCase_Execute(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")

	t.Run("sem sessão", func(t *testing.T) {
		t.Parallel()
		bm := &contractsfake.BlocklistManager{SessionGuard: contractsfake.FailSession(errNoSession)}
		logger := &contractsfake.Logger{}
		uc := user.NewGetBlocklistUseCase(bm, logger)

		if _, err := uc.Execute(context.Background(), "u1", domain.GetBlocklistRequest{}); !errors.Is(err, errNoSession) {
			t.Fatalf("err = %v, queria errNoSession", err)
		}
		assertNoSessionLog(t, logger, "u1")
	})

	t.Run("falha do adapter é embrulhada", func(t *testing.T) {
		t.Parallel()
		bm := &contractsfake.BlocklistManager{
			GetBlocklistFunc: func(context.Context, string) (domain.Blocklist, error) {
				return domain.Blocklist{}, boom
			},
		}
		logger := &contractsfake.Logger{}
		uc := user.NewGetBlocklistUseCase(bm, logger)

		_, err := uc.Execute(context.Background(), "u1", domain.GetBlocklistRequest{})
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, queria boom", err)
		}
		if !logger.Logged("Failed to get blocklist") {
			t.Error("esperava log de erro")
		}
	})

	t.Run("lista devolvida com o hash", func(t *testing.T) {
		t.Parallel()
		bm := &contractsfake.BlocklistManager{
			GetBlocklistFunc: func(context.Context, string) (domain.Blocklist, error) {
				return domain.Blocklist{JIDs: []string{"a@s.whatsapp.net", "b@s.whatsapp.net"}, DHash: "h1"}, nil
			},
		}
		logger := &contractsfake.Logger{}
		uc := user.NewGetBlocklistUseCase(bm, logger)

		got, err := uc.Execute(context.Background(), "u1", domain.GetBlocklistRequest{})
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if got["DHash"] != "h1" {
			t.Errorf("DHash = %v, queria h1", got["DHash"])
		}
		jids, ok := got["Blocklist"].([]string)
		if !ok || len(jids) != 2 {
			t.Fatalf("Blocklist = %#v", got["Blocklist"])
		}
		rec, ok := logger.Find("Retrieved blocklist")
		if !ok {
			t.Fatal("log de sucesso ausente")
		}
		if v, ok := rec.Keyval("count"); !ok || v != 2 {
			t.Errorf("keyval count = %v, queria 2", v)
		}
	})
}

// blockCase descreve um caso compartilhado por block e unblock: os dois use
// cases têm a mesma forma e diferem apenas no booleano enviado ao adapter e
// no texto de Details.
type blockCase struct {
	name        string
	session     error
	jid, phone  string
	resolveErr  error
	updateFunc  func(ctx context.Context, txtID string, target domain.JID, block bool) (domain.BlocklistUpdate, error)
	wantErr     bool
	wantIs      error
	wantJID     string
	wantReqJID  string
	wantTargetJ domain.JID
}

func blockCases(boom error) []blockCase {
	return []blockCase{
		{
			name:    "sem sessão",
			session: errNoSession,
			phone:   "5511",
			wantErr: true,
			wantIs:  errNoSession,
		},
		{
			name:    "nem Phone nem JID",
			wantErr: true,
		},
		{
			name:       "alvo que não parseia",
			phone:      "??",
			resolveErr: errors.New("formato ruim"),
			wantErr:    true,
		},
		{
			name:  "falha do adapter",
			phone: "5511",
			updateFunc: func(context.Context, string, domain.JID, bool) (domain.BlocklistUpdate, error) {
				return domain.BlocklistUpdate{}, boom
			},
			wantErr: true,
			wantIs:  boom,
		},
		{
			name:  "JID tem precedência sobre Phone",
			jid:   "  5599@s.whatsapp.net  ",
			phone: "5511",
			updateFunc: func(_ context.Context, _ string, target domain.JID, _ bool) (domain.BlocklistUpdate, error) {
				return domain.BlocklistUpdate{ResolvedJID: target, RequestedJID: target, DHash: "h"}, nil
			},
			wantJID:     "5599@s.whatsapp.net",
			wantTargetJ: "5599@s.whatsapp.net",
		},
		{
			name:  "resolvido difere do pedido e aparece na resposta",
			phone: "5511",
			updateFunc: func(context.Context, string, domain.JID, bool) (domain.BlocklistUpdate, error) {
				return domain.BlocklistUpdate{
					ResolvedJID:  "5511@s.whatsapp.net",
					RequestedJID: "777@lid",
					Entries:      []string{"5511@s.whatsapp.net"},
					DHash:        "h2",
				}, nil
			},
			wantJID:     "5511@s.whatsapp.net",
			wantReqJID:  "777@lid",
			wantTargetJ: "5511",
		},
	}
}

func TestBlockUserUseCase_Execute(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	for _, tt := range blockCases(boom) {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			bm := &contractsfake.BlocklistManager{UpdateBlocklistFunc: tt.updateFunc}
			if tt.session != nil {
				bm.SessionGuard = contractsfake.FailSession(tt.session)
			}
			jr := &contractsfake.JIDResolver{}
			if tt.resolveErr != nil {
				jr.ResolveQualifiedJIDFunc = func(context.Context, string) (domain.JID, error) {
					return "", tt.resolveErr
				}
			}
			logger := &contractsfake.Logger{}
			uc := user.NewBlockUserUseCase(bm, jr, logger)

			got, err := uc.Execute(context.Background(), "u1", domain.BlockUserRequest{JID: tt.jid, Phone: tt.phone})
			if tt.wantErr {
				if err == nil {
					t.Fatal("esperava erro")
				}
				if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
					t.Fatalf("err = %v, queria %v", err, tt.wantIs)
				}
				if errors.Is(err, errNoSession) {
					assertNoSessionLog(t, logger, "u1")
				}
				return
			}
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if got.Details != "User blocked" {
				t.Errorf("Details = %q, queria User blocked", got.Details)
			}
			if got.JID != tt.wantJID || got.RequestedJID != tt.wantReqJID {
				t.Errorf("JIDs = %q/%q, queria %q/%q", got.JID, got.RequestedJID, tt.wantJID, tt.wantReqJID)
			}
			if len(bm.UpdateBlocklistCalls) != 1 {
				t.Fatalf("UpdateBlocklist chamado %d vezes, queria 1", len(bm.UpdateBlocklistCalls))
			}
			call := bm.UpdateBlocklistCalls[0]
			if !call.Block {
				t.Error("Block = false, queria true")
			}
			if call.Target != tt.wantTargetJ {
				t.Errorf("alvo = %q, queria %q", call.Target, tt.wantTargetJ)
			}
		})
	}
}

func TestUnblockUserUseCase_Execute(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	for _, tt := range blockCases(boom) {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			bm := &contractsfake.BlocklistManager{UpdateBlocklistFunc: tt.updateFunc}
			if tt.session != nil {
				bm.SessionGuard = contractsfake.FailSession(tt.session)
			}
			jr := &contractsfake.JIDResolver{}
			if tt.resolveErr != nil {
				jr.ResolveQualifiedJIDFunc = func(context.Context, string) (domain.JID, error) {
					return "", tt.resolveErr
				}
			}
			logger := &contractsfake.Logger{}
			uc := user.NewUnblockUserUseCase(bm, jr, logger)

			got, err := uc.Execute(context.Background(), "u1", domain.UnblockUserRequest{JID: tt.jid, Phone: tt.phone})
			if tt.wantErr {
				if err == nil {
					t.Fatal("esperava erro")
				}
				if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
					t.Fatalf("err = %v, queria %v", err, tt.wantIs)
				}
				if errors.Is(err, errNoSession) {
					assertNoSessionLog(t, logger, "u1")
				}
				return
			}
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if got.Details != "User unblocked" {
				t.Errorf("Details = %q, queria User unblocked", got.Details)
			}
			if got.JID != tt.wantJID || got.RequestedJID != tt.wantReqJID {
				t.Errorf("JIDs = %q/%q, queria %q/%q", got.JID, got.RequestedJID, tt.wantJID, tt.wantReqJID)
			}
			if len(bm.UpdateBlocklistCalls) != 1 {
				t.Fatalf("UpdateBlocklist chamado %d vezes, queria 1", len(bm.UpdateBlocklistCalls))
			}
			if bm.UpdateBlocklistCalls[0].Block {
				t.Error("Block = true, queria false no unblock")
			}
		})
	}
}
