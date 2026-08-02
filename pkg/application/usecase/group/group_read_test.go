package group_test

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/group"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// sessErr é o erro tipado que as portas de capacidade devolvem quando não há
// sessão whatsmeow. Os use cases desta fase o propagam verbatim — o teste
// assere o Code, nunca o texto.
func sessErr() *apperr.AppError {
	return apperr.New("no_session", apperr.CategoryValidation, "no session", false, nil)
}

// wantLog descreve a asserção de log de um caso: nível, mensagem e as chaves
// estruturadas que o registro tem de carregar.
type wantLog struct {
	level string
	msg   string
	keys  []string
}

// assertLog confere que o registro pedido existe, que é estruturado (L3) e
// que carrega as chaves declaradas.
func assertLog(t *testing.T, l *contractsfake.Logger, want wantLog) {
	t.Helper()
	if want.msg == "" {
		return
	}
	rec, ok := l.FindLevel(want.level, want.msg)
	if !ok {
		t.Fatalf("log %s/%q ausente; registros = %v", want.level, want.msg, l.Messages())
	}
	if !rec.IsStructured() {
		t.Errorf("log %q não estruturado: keyvals=%v", want.msg, rec.Keyvals)
	}
	for _, k := range want.keys {
		if !rec.HasKey(k) {
			t.Errorf("log %q sem a chave %q: keyvals=%v", want.msg, k, rec.Keyvals)
		}
	}
}

// assertNoLog confere que nenhum registro daquele nível foi emitido.
func assertNoLevel(t *testing.T, l *contractsfake.Logger, level string) {
	t.Helper()
	if recs := l.ByLevel(level); len(recs) > 0 {
		t.Errorf("nível %s inesperado: %v", level, recs)
	}
}

// assertCode confere que err carrega o Code de apperr esperado.
func assertCode(t *testing.T, err error, code string) {
	t.Helper()
	var ae *apperr.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("erro não é *apperr.AppError: %T (%v)", err, err)
	}
	if ae.Code != code {
		t.Errorf("Code = %q, quero %q", ae.Code, code)
	}
}

func TestGetGroupInfoUseCase_Execute(t *testing.T) {
	boom := errors.New("directory down")

	tests := []struct {
		name       string
		req        domain.GetGroupInfoRequest
		dir        func(*contractsfake.GroupDirectory)
		jids       *contractsfake.JIDResolver
		wantErr    bool
		wantCode   string
		wantLog    wantLog
		wantCalled bool
	}{
		{
			name:    "groupJID vazio rejeita antes de tocar a porta",
			req:     domain.GetGroupInfoRequest{},
			wantErr: true,
			wantLog: wantLog{contractsfake.LevelWarn, "missing groupJID in request", []string{"txtID"}},
		},
		{
			name: "JID irresolvível rejeita com aviso",
			req:  domain.GetGroupInfoRequest{GroupJID: "@@"},
			jids: &contractsfake.JIDResolver{ResolveJIDFunc: func(context.Context, string) (domain.JID, error) {
				return "", boom
			}},
			wantErr: true,
			wantLog: wantLog{contractsfake.LevelWarn, "could not parse group JID", []string{"txtID", "groupJID", "error"}},
		},
		{
			name: "sem sessão propaga o erro tipado da porta",
			req:  domain.GetGroupInfoRequest{GroupJID: "123@g.us"},
			dir: func(d *contractsfake.GroupDirectory) {
				d.SessionGuard = contractsfake.FailSession(sessErr())
			},
			wantErr:  true,
			wantCode: "no_session",
			wantLog:  wantLog{contractsfake.LevelError, "no whatsmeow session", []string{"txtID", "error"}},
		},
		{
			name: "falha da porta vira erro logado",
			req:  domain.GetGroupInfoRequest{GroupJID: "123@g.us"},
			dir: func(d *contractsfake.GroupDirectory) {
				d.GetGroupInfoFunc = func(context.Context, string, domain.JID) (any, error) { return nil, boom }
			},
			wantErr: true,
			wantLog: wantLog{contractsfake.LevelError, "failed to get group info", []string{"txtID", "groupJID", "error"}},
		},
		{
			name: "sucesso devolve o info e loga em info",
			req:  domain.GetGroupInfoRequest{GroupJID: "123@g.us"},
			dir: func(d *contractsfake.GroupDirectory) {
				d.GetGroupInfoFunc = func(context.Context, string, domain.JID) (any, error) { return "info", nil }
			},
			wantLog:    wantLog{contractsfake.LevelInfo, "group info retrieved", []string{"txtID", "groupJID"}},
			wantCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := &contractsfake.GroupDirectory{}
			if tt.dir != nil {
				tt.dir(dir)
			}
			jids := tt.jids
			if jids == nil {
				jids = &contractsfake.JIDResolver{}
			}
			log := &contractsfake.Logger{}
			uc := group.NewGetGroupInfoUseCase(dir, jids, log)

			res, err := uc.Execute(context.Background(), "u1", tt.req)

			if tt.wantErr && err == nil {
				t.Fatal("esperava erro")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if tt.wantCode != "" {
				assertCode(t, err, tt.wantCode)
			}
			if !tt.wantErr {
				if res == nil || res.GroupInfo != "info" {
					t.Errorf("resultado = %+v", res)
				}
			}
			if tt.wantCalled && len(dir.GetGroupInfoCalls) != 1 {
				t.Errorf("GetGroupInfo chamado %d vezes", len(dir.GetGroupInfoCalls))
			}
			assertLog(t, log, tt.wantLog)
		})
	}
}

func TestGetGroupInviteLinkUseCase_Execute(t *testing.T) {
	boom := errors.New("link unavailable")

	tests := []struct {
		name     string
		req      domain.GetGroupInviteLinkRequest
		dir      func(*contractsfake.GroupDirectory)
		jids     *contractsfake.JIDResolver
		wantErr  bool
		wantCode string
		wantLink string
		wantLog  wantLog
	}{
		{
			name:    "groupJID vazio",
			req:     domain.GetGroupInviteLinkRequest{},
			wantErr: true,
			wantLog: wantLog{contractsfake.LevelWarn, "missing groupJID in request", []string{"txtID"}},
		},
		{
			name: "JID irresolvível",
			req:  domain.GetGroupInviteLinkRequest{GroupJID: "@@"},
			jids: &contractsfake.JIDResolver{ResolveJIDFunc: func(context.Context, string) (domain.JID, error) {
				return "", boom
			}},
			wantErr: true,
			wantLog: wantLog{contractsfake.LevelWarn, "could not parse group JID", []string{"txtID", "groupJID", "error"}},
		},
		{
			name: "sem sessão",
			req:  domain.GetGroupInviteLinkRequest{GroupJID: "123@g.us"},
			dir: func(d *contractsfake.GroupDirectory) {
				d.SessionGuard = contractsfake.FailSession(sessErr())
			},
			wantErr:  true,
			wantCode: "no_session",
			wantLog:  wantLog{contractsfake.LevelError, "no whatsmeow session", []string{"txtID", "error"}},
		},
		{
			name: "porta falha",
			req:  domain.GetGroupInviteLinkRequest{GroupJID: "123@g.us"},
			dir: func(d *contractsfake.GroupDirectory) {
				d.GetGroupInviteLinkFunc = func(context.Context, string, domain.JID) (string, error) { return "", boom }
			},
			wantErr: true,
			wantLog: wantLog{contractsfake.LevelError, "failed to get group invite link", []string{"txtID", "groupJID", "error"}},
		},
		{
			name: "sucesso",
			req:  domain.GetGroupInviteLinkRequest{GroupJID: "123@g.us"},
			dir: func(d *contractsfake.GroupDirectory) {
				d.GetGroupInviteLinkFunc = func(context.Context, string, domain.JID) (string, error) {
					return "https://chat.whatsapp.com/abc", nil
				}
			},
			wantLink: "https://chat.whatsapp.com/abc",
			wantLog:  wantLog{contractsfake.LevelInfo, "group invite link retrieved", []string{"txtID", "groupJID"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := &contractsfake.GroupDirectory{}
			if tt.dir != nil {
				tt.dir(dir)
			}
			jids := tt.jids
			if jids == nil {
				jids = &contractsfake.JIDResolver{}
			}
			log := &contractsfake.Logger{}
			uc := group.NewGetGroupInviteLinkUseCase(dir, jids, log)

			res, err := uc.Execute(context.Background(), "u1", tt.req)

			if tt.wantErr {
				if err == nil {
					t.Fatal("esperava erro")
				}
				if tt.wantCode != "" {
					assertCode(t, err, tt.wantCode)
				}
			} else {
				if err != nil {
					t.Fatalf("erro inesperado: %v", err)
				}
				if res.InviteLink != tt.wantLink {
					t.Errorf("InviteLink = %q, quero %q", res.InviteLink, tt.wantLink)
				}
			}
			assertLog(t, log, tt.wantLog)
		})
	}
}

func TestGetGroupInviteInfoUseCase_Execute(t *testing.T) {
	boom := errors.New("invite expired")

	tests := []struct {
		name     string
		req      domain.GetGroupInviteInfoRequest
		dir      func(*contractsfake.GroupDirectory)
		wantErr  bool
		wantCode string
		wantLog  wantLog
	}{
		{
			name:    "code vazio",
			req:     domain.GetGroupInviteInfoRequest{},
			wantErr: true,
			wantLog: wantLog{contractsfake.LevelWarn, "missing invite code in request", []string{"txtID"}},
		},
		{
			name: "sem sessão",
			req:  domain.GetGroupInviteInfoRequest{Code: "abc"},
			dir: func(d *contractsfake.GroupDirectory) {
				d.SessionGuard = contractsfake.FailSession(sessErr())
			},
			wantErr:  true,
			wantCode: "no_session",
			wantLog:  wantLog{contractsfake.LevelError, "no whatsmeow session", []string{"txtID", "error"}},
		},
		{
			name: "porta falha",
			req:  domain.GetGroupInviteInfoRequest{Code: "abc"},
			dir: func(d *contractsfake.GroupDirectory) {
				d.GetGroupInfoFromLinkFunc = func(context.Context, string, string) (any, error) { return nil, boom }
			},
			wantErr: true,
			wantLog: wantLog{contractsfake.LevelError, "failed to get group invite info", []string{"txtID", "error"}},
		},
		{
			name: "sucesso",
			req:  domain.GetGroupInviteInfoRequest{Code: "abc"},
			dir: func(d *contractsfake.GroupDirectory) {
				d.GetGroupInfoFromLinkFunc = func(context.Context, string, string) (any, error) { return "invite", nil }
			},
			wantLog: wantLog{contractsfake.LevelInfo, "group invite info retrieved", []string{"txtID"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := &contractsfake.GroupDirectory{}
			if tt.dir != nil {
				tt.dir(dir)
			}
			log := &contractsfake.Logger{}
			uc := group.NewGetGroupInviteInfoUseCase(dir, log)

			res, err := uc.Execute(context.Background(), "u1", tt.req)

			if tt.wantErr {
				if err == nil {
					t.Fatal("esperava erro")
				}
				if tt.wantCode != "" {
					assertCode(t, err, tt.wantCode)
				}
			} else {
				if err != nil {
					t.Fatalf("erro inesperado: %v", err)
				}
				if res.InviteInfo != "invite" {
					t.Errorf("InviteInfo = %v", res.InviteInfo)
				}
				// O code chega verbatim à porta.
				if got := dir.GetGroupInfoFromLinkCalls[0].Code; got != "abc" {
					t.Errorf("code repassado = %q", got)
				}
			}
			assertLog(t, log, tt.wantLog)
		})
	}
}

func TestListGroupsUseCase_Execute(t *testing.T) {
	boom := errors.New("list failed")

	tests := []struct {
		name     string
		dir      func(*contractsfake.GroupDirectory)
		wantErr  bool
		wantCode string
		wantLog  wantLog
	}{
		{
			name: "sem sessão",
			dir: func(d *contractsfake.GroupDirectory) {
				d.SessionGuard = contractsfake.FailSession(sessErr())
			},
			wantErr:  true,
			wantCode: "no_session",
			wantLog:  wantLog{contractsfake.LevelError, "no whatsmeow session", []string{"txtID", "error"}},
		},
		{
			name: "porta falha",
			dir: func(d *contractsfake.GroupDirectory) {
				d.ListJoinedGroupsFunc = func(context.Context, string) (any, int, error) { return nil, 0, boom }
			},
			wantErr: true,
			wantLog: wantLog{contractsfake.LevelError, "failed to get groups", []string{"txtID", "error"}},
		},
		{
			name: "sucesso carrega a contagem no log",
			dir: func(d *contractsfake.GroupDirectory) {
				d.ListJoinedGroupsFunc = func(context.Context, string) (any, int, error) {
					return []string{"a", "b"}, 2, nil
				}
			},
			wantLog: wantLog{contractsfake.LevelInfo, "groups listed successfully", []string{"txtID", "count"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := &contractsfake.GroupDirectory{}
			if tt.dir != nil {
				tt.dir(dir)
			}
			log := &contractsfake.Logger{}
			uc := group.NewListGroupsUseCase(dir, log)

			res, err := uc.Execute(context.Background(), "u1", domain.ListGroupsRequest{})

			if tt.wantErr {
				if err == nil {
					t.Fatal("esperava erro")
				}
				if tt.wantCode != "" {
					assertCode(t, err, tt.wantCode)
				}
				if res != nil {
					t.Errorf("resultado deveria ser nil no erro: %+v", res)
				}
			} else if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			assertLog(t, log, tt.wantLog)
		})
	}
}

// TestListGroupsUseCase_CountNoLog fixa que a contagem devolvida pela porta
// chega ao log, e não ao resultado — o resultado carrega só os grupos.
func TestListGroupsUseCase_CountVaiParaOLogNaoParaOResultado(t *testing.T) {
	dir := &contractsfake.GroupDirectory{
		ListJoinedGroupsFunc: func(context.Context, string) (any, int, error) {
			return []string{"a"}, 7, nil
		},
	}
	log := &contractsfake.Logger{}
	uc := group.NewListGroupsUseCase(dir, log)

	res, err := uc.Execute(context.Background(), "u1", domain.ListGroupsRequest{})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	rec, ok := log.FindLevel(contractsfake.LevelInfo, "groups listed successfully")
	if !ok {
		t.Fatal("log de sucesso ausente")
	}
	if v, _ := rec.Keyval("count"); v != 7 {
		t.Errorf("count no log = %v, quero 7", v)
	}
	if got, ok := res.Groups.([]string); !ok || len(got) != 1 {
		t.Errorf("Groups = %v", res.Groups)
	}
	assertNoLevel(t, log, contractsfake.LevelError)
}
