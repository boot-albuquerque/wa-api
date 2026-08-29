package user_test

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/domain"
)

func TestGetPrivacySettingsUseCase_Execute(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	tests := []struct {
		name     string
		session  error
		getFunc  func(ctx context.Context, txtID string) (domain.PrivacySettings, error)
		wantErr  bool
		wantIs   error
		wantLog  string
		wantData domain.PrivacySettings
	}{
		{
			name:    "sem sessão",
			session: errNoSession,
			wantErr: true,
			wantIs:  errNoSession,
		},
		{
			name: "falha do adapter é embrulhada e logada",
			getFunc: func(context.Context, string) (domain.PrivacySettings, error) {
				return domain.PrivacySettings{}, boom
			},
			wantErr: true,
			wantIs:  boom,
			wantLog: "failed to get privacy settings",
		},
		{
			name: "configurações devolvidas como vieram",
			getFunc: func(context.Context, string) (domain.PrivacySettings, error) {
				return domain.PrivacySettings{LastSeen: "contacts"}, nil
			},
			wantData: domain.PrivacySettings{LastSeen: "contacts"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pm := &contractsfake.PrivacyManager{GetPrivacySettingsFunc: tt.getFunc}
			if tt.session != nil {
				pm.SessionGuard = contractsfake.FailSession(tt.session)
			}
			logger := &contractsfake.Logger{}
			uc := user.NewGetPrivacySettingsUseCase(pm, logger)

			got, err := uc.Execute(context.Background(), "u1")
			if tt.wantErr {
				if !errors.Is(err, tt.wantIs) {
					t.Fatalf("err = %v, queria %v", err, tt.wantIs)
				}
				if errors.Is(err, errNoSession) {
					assertNoSessionLog(t, logger, "u1")
				}
				if tt.wantLog != "" && !logger.Logged(tt.wantLog) {
					t.Errorf("log %q ausente; houve %v", tt.wantLog, logger.Messages())
				}
				return
			}
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if got != tt.wantData {
				t.Errorf("settings = %v, queria %v", got, tt.wantData)
			}
		})
	}
}

func TestSetPrivacySettingUseCase_Execute(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	tests := []struct {
		name     string
		session  error
		req      domain.SetPrivacySettingRequest
		setFunc  func(ctx context.Context, txtID, name, value string) (domain.PrivacySettings, error)
		wantErr  bool
		wantIs   error
		wantCall bool
	}{
		{
			name:    "sem sessão",
			session: errNoSession,
			req:     domain.SetPrivacySettingRequest{PrivacySetting: "last", Value: "all"},
			wantErr: true,
			wantIs:  errNoSession,
		},
		{
			name:    "nome de configuração desconhecido",
			req:     domain.SetPrivacySettingRequest{PrivacySetting: "inexistente", Value: "all"},
			wantErr: true,
		},
		{
			name:    "valor fora do permitido para a configuração",
			req:     domain.SetPrivacySettingRequest{PrivacySetting: "readreceipts", Value: "contacts"},
			wantErr: true,
		},
		{
			name: "falha do adapter é embrulhada",
			req:  domain.SetPrivacySettingRequest{PrivacySetting: "online", Value: "match_last_seen"},
			setFunc: func(context.Context, string, string, string) (domain.PrivacySettings, error) {
				return domain.PrivacySettings{}, boom
			},
			wantErr:  true,
			wantIs:   boom,
			wantCall: true,
		},
		{
			name: "configuração aplicada",
			req:  domain.SetPrivacySettingRequest{PrivacySetting: "groupadd", Value: "contacts"},
			setFunc: func(context.Context, string, string, string) (domain.PrivacySettings, error) {
				return domain.PrivacySettings{GroupAdd: "contacts"}, nil
			},
			wantCall: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pm := &contractsfake.PrivacyManager{SetPrivacySettingFunc: tt.setFunc}
			if tt.session != nil {
				pm.SessionGuard = contractsfake.FailSession(tt.session)
			}
			logger := &contractsfake.Logger{}
			uc := user.NewSetPrivacySettingUseCase(pm, logger)

			got, err := uc.Execute(context.Background(), "u1", tt.req)
			if gotCall := len(pm.SetPrivacySettingCalls) == 1; gotCall != tt.wantCall {
				t.Errorf("adapter chamado = %v, queria %v", gotCall, tt.wantCall)
			}
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
			// The port answers with the RESULTING state of every setting, not
			// only the one that changed — so the assertion is on that state.
			if got.GroupAdd != "contacts" {
				t.Errorf("resultado = %+v, queria groupadd=contacts", got)
			}
			call := pm.SetPrivacySettingCalls[0]
			if call.Name != tt.req.PrivacySetting || call.Value != tt.req.Value {
				t.Errorf("chamada = %+v, queria o par do request", call)
			}
		})
	}
}
