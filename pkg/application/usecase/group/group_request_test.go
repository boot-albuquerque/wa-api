package group_test

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/group"
	"wa-api/pkg/domain"
)

type reqFakes struct {
	reqs *contractsfake.GroupRequests
	jids *contractsfake.JIDResolver
	log  *contractsfake.Logger
	uc   *group.GroupRequestUseCase
}

func newReq() *reqFakes {
	f := &reqFakes{
		reqs: &contractsfake.GroupRequests{},
		jids: &contractsfake.JIDResolver{},
		log:  &contractsfake.Logger{},
	}
	f.uc = group.NewGroupRequestUseCase(f.reqs, f.jids, f.log)
	return f
}

func (f *reqFakes) rebuild() {
	f.uc = group.NewGroupRequestUseCase(f.reqs, f.jids, f.log)
}

// failQualifiedJID faz o resolver qualificado recusar tudo.
func failQualifiedJID(err error) *contractsfake.JIDResolver {
	return &contractsfake.JIDResolver{
		ResolveQualifiedJIDFunc: func(context.Context, string) (domain.JID, error) { return "", err },
	}
}

func TestGroupRequest_ExecuteGetGroupRequestParticipants(t *testing.T) {
	boom := errors.New("upstream down")

	tests := []struct {
		name     string
		req      domain.GetGroupRequestParticipantsRequest
		arrange  func(*reqFakes)
		wantErr  bool
		wantCode string
		wantLog  wantLog
	}{
		{
			name:    "groupJID vazio",
			req:     domain.GetGroupRequestParticipantsRequest{},
			wantErr: true,
			wantLog: wantLog{contractsfake.LevelWarn, "missing groupJID in request", []string{"user_id"}},
		},
		{
			name: "sem sessão propaga o erro tipado",
			req:  domain.GetGroupRequestParticipantsRequest{GroupJID: "g@g.us"},
			arrange: func(f *reqFakes) {
				f.reqs.SessionGuard = contractsfake.FailSession(sessErr())
			},
			wantErr:  true,
			wantCode: "no_session",
			wantLog:  wantLog{contractsfake.LevelWarn, "no wanoise session", []string{"user_id", "error"}},
		},
		{
			name: "JID irresolvível",
			req:  domain.GetGroupRequestParticipantsRequest{GroupJID: "@@"},
			arrange: func(f *reqFakes) {
				f.jids = failQualifiedJID(boom)
				f.rebuild()
			},
			wantErr: true,
			wantLog: wantLog{contractsfake.LevelWarn, "could not parse group JID", []string{"user_id", "group_jid", "error"}},
		},
		{
			name: "porta falha",
			req:  domain.GetGroupRequestParticipantsRequest{GroupJID: "g@g.us"},
			arrange: func(f *reqFakes) {
				f.reqs.GetRequestParticipantsFunc = func(context.Context, string, domain.JID) ([]domain.GroupJoinRequest, error) {
					return nil, boom
				}
			},
			wantErr: true,
			wantLog: wantLog{contractsfake.LevelError, "failed to get group request participants", []string{"user_id", "group_jid", "error"}},
		},
		// O caso "resposta não serializável" saiu com a migração para DTO: o use
		// case já não serializa nada — devolve um tipo de domínio, e quem o
		// transforma em JSON é o apresentador da fronteira HTTP. O ramo que este
		// caso cobria (json.Marshal falhar sobre +Inf vindo do motor) deixou de
		// existir por construção, e não por alguém o ter apagado.
		{
			name: "sucesso devolve as solicitações da porta",
			req:  domain.GetGroupRequestParticipantsRequest{GroupJID: "g@g.us"},
			arrange: func(f *reqFakes) {
				f.reqs.GetRequestParticipantsFunc = func(context.Context, string, domain.JID) ([]domain.GroupJoinRequest, error) {
					return []domain.GroupJoinRequest{{JID: "5511@s.whatsapp.net", Method: "InviteLink"}}, nil
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newReq()
			if tt.arrange != nil {
				tt.arrange(f)
			}

			out, err := f.uc.ExecuteGetGroupRequestParticipants(context.Background(), "u1", tt.req)

			if tt.wantErr {
				if err == nil {
					t.Fatal("esperava erro")
				}
				if tt.wantCode != "" {
					assertCode(t, err, tt.wantCode)
				}
				if out != nil {
					t.Errorf("saída deveria ser nil no erro: %+v", out)
				}
			} else {
				if err != nil {
					t.Fatalf("erro inesperado: %v", err)
				}
				if out == nil || len(out.Requests) != 1 {
					t.Fatalf("saída = %+v", out)
				}
				if out.Requests[0].JID != "5511@s.whatsapp.net" || out.Requests[0].Method != "InviteLink" {
					t.Errorf("solicitação = %#v", out.Requests[0])
				}
			}
			assertLog(t, f.log, tt.wantLog)
		})
	}
}

func TestGroupRequest_ExecuteUpdateGroupRequestParticipants(t *testing.T) {
	boom := errors.New("upstream down")
	base := domain.UpdateGroupRequestParticipantsRequest{
		GroupJID: "g@g.us",
		Phone:    []string{"55A@s.whatsapp.net"},
		Action:   "approve",
	}

	withReq := func(mut func(*domain.UpdateGroupRequestParticipantsRequest)) domain.UpdateGroupRequestParticipantsRequest {
		r := base
		r.Phone = append([]string(nil), base.Phone...)
		mut(&r)
		return r
	}

	tests := []struct {
		name     string
		req      domain.UpdateGroupRequestParticipantsRequest
		arrange  func(*reqFakes)
		wantErr  bool
		wantCode string
		wantLog  wantLog
	}{
		{
			name: "sem sessão recusa antes de validar o payload",
			req:  domain.UpdateGroupRequestParticipantsRequest{},
			arrange: func(f *reqFakes) {
				f.reqs.SessionGuard = contractsfake.FailSession(sessErr())
			},
			wantErr:  true,
			wantCode: "no_session",
			wantLog:  wantLog{contractsfake.LevelWarn, "no wanoise session", []string{"user_id", "error"}},
		},
		{
			name:    "groupJID vazio",
			req:     withReq(func(r *domain.UpdateGroupRequestParticipantsRequest) { r.GroupJID = "" }),
			wantErr: true,
			wantLog: wantLog{contractsfake.LevelWarn, "missing groupJID in request", []string{"user_id"}},
		},
		{
			name:    "lista de telefones vazia",
			req:     withReq(func(r *domain.UpdateGroupRequestParticipantsRequest) { r.Phone = nil }),
			wantErr: true,
			wantLog: wantLog{contractsfake.LevelWarn, "missing phone list in request", []string{"user_id", "group_jid"}},
		},
		{
			name:    "action vazia",
			req:     withReq(func(r *domain.UpdateGroupRequestParticipantsRequest) { r.Action = "" }),
			wantErr: true,
			wantLog: wantLog{contractsfake.LevelWarn, "missing action in request", []string{"user_id", "group_jid"}},
		},
		{
			name:    "action desconhecida",
			req:     withReq(func(r *domain.UpdateGroupRequestParticipantsRequest) { r.Action = "talvez" }),
			wantErr: true,
			wantLog: wantLog{contractsfake.LevelWarn, "invalid action in request", []string{"user_id", "group_jid", "action"}},
		},
		{
			name: "groupJID irresolvível",
			req:  base,
			arrange: func(f *reqFakes) {
				f.jids = failQualifiedJID(boom)
				f.rebuild()
			},
			wantErr: true,
			wantLog: wantLog{contractsfake.LevelWarn, "could not parse group JID", []string{"user_id", "group_jid", "error"}},
		},
		{
			name: "telefone irresolvível",
			req:  withReq(func(r *domain.UpdateGroupRequestParticipantsRequest) { r.Phone = []string{"ok", "ruim"} }),
			arrange: func(f *reqFakes) {
				f.jids = &contractsfake.JIDResolver{
					ResolveQualifiedJIDFunc: func(_ context.Context, raw string) (domain.JID, error) {
						if raw == "ruim" {
							return "", boom
						}
						return domain.JID(raw), nil
					},
				}
				f.rebuild()
			},
			wantErr: true,
			wantLog: wantLog{contractsfake.LevelWarn, "could not parse phone", []string{"user_id", "index", "error"}},
		},
		{
			name: "porta falha",
			req:  base,
			arrange: func(f *reqFakes) {
				f.reqs.UpdateRequestParticipantsFunc = func(context.Context, string, domain.JID, []domain.JID, domain.RequestAction) error {
					return boom
				}
			},
			wantErr: true,
			wantLog: wantLog{contractsfake.LevelError, "failed to update group request participants", []string{"user_id", "group_jid", "action", "error"}},
		},
		{
			name: "sucesso",
			req:  base,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newReq()
			if tt.arrange != nil {
				tt.arrange(f)
			}

			res, err := f.uc.ExecuteUpdateGroupRequestParticipants(context.Background(), "u1", tt.req)

			if tt.wantErr {
				if err == nil {
					t.Fatal("esperava erro")
				}
				if tt.wantCode != "" {
					assertCode(t, err, tt.wantCode)
				}
				if len(f.reqs.UpdateRequestParticipantsCalls) > 0 && tt.wantLog.msg != "failed to update group request participants" {
					t.Error("a porta foi chamada num caminho de rejeição")
				}
			} else {
				if err != nil {
					t.Fatalf("erro inesperado: %v", err)
				}
				if res.Details == "" {
					t.Error("Details vazio")
				}
			}
			assertLog(t, f.log, tt.wantLog)
		})
	}
}

// TestGroupRequest_UpdateTraduzAction fixa o mapeamento approve/reject para
// domain.RequestAction e o repasse da lista resolvida.
func TestGroupRequest_UpdateTraduzAction(t *testing.T) {
	tests := []struct {
		action string
		want   domain.RequestAction
	}{
		{"approve", domain.RequestApprove},
		{"reject", domain.RequestReject},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			f := newReq()
			_, err := f.uc.ExecuteUpdateGroupRequestParticipants(context.Background(), "u1",
				domain.UpdateGroupRequestParticipantsRequest{
					GroupJID: "g@g.us",
					Phone:    []string{"55A@s.whatsapp.net", "55B@s.whatsapp.net"},
					Action:   tt.action,
				})
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			call := f.reqs.UpdateRequestParticipantsCalls[0]
			if call.Action != tt.want {
				t.Errorf("Action = %q, quero %q", call.Action, tt.want)
			}
			if len(call.Participants) != 2 || call.Participants[1] != domain.JID("55B@s.whatsapp.net") {
				t.Errorf("participantes = %v", call.Participants)
			}
			if call.Group != domain.JID("g@g.us") {
				t.Errorf("group = %v", call.Group)
			}
			assertNoLevel(t, f.log, contractsfake.LevelError)
		})
	}
}

func TestGroupRequest_ExecuteSetGroupJoinApprovalMode(t *testing.T) {
	boom := errors.New("upstream down")

	tests := []struct {
		name     string
		req      domain.SetGroupJoinApprovalModeRequest
		arrange  func(*reqFakes)
		wantErr  bool
		wantCode string
		wantLog  wantLog
	}{
		{
			name: "sem sessão",
			req:  domain.SetGroupJoinApprovalModeRequest{GroupJID: "g@g.us", Mode: true},
			arrange: func(f *reqFakes) {
				f.reqs.SessionGuard = contractsfake.FailSession(sessErr())
			},
			wantErr:  true,
			wantCode: "no_session",
			wantLog:  wantLog{contractsfake.LevelWarn, "no wanoise session", []string{"user_id", "error"}},
		},
		{
			name:    "groupJID vazio",
			req:     domain.SetGroupJoinApprovalModeRequest{},
			wantErr: true,
			wantLog: wantLog{contractsfake.LevelWarn, "missing groupJID in request", []string{"user_id"}},
		},
		{
			name: "JID irresolvível",
			req:  domain.SetGroupJoinApprovalModeRequest{GroupJID: "@@"},
			arrange: func(f *reqFakes) {
				f.jids = failQualifiedJID(boom)
				f.rebuild()
			},
			wantErr: true,
			wantLog: wantLog{contractsfake.LevelWarn, "could not parse group JID", []string{"user_id", "group_jid", "error"}},
		},
		{
			name: "porta falha",
			req:  domain.SetGroupJoinApprovalModeRequest{GroupJID: "g@g.us", Mode: true},
			arrange: func(f *reqFakes) {
				f.reqs.SetJoinApprovalModeFunc = func(context.Context, string, domain.JID, bool) error { return boom }
			},
			wantErr: true,
			wantLog: wantLog{contractsfake.LevelError, "failed to set group join approval mode", []string{"user_id", "group_jid", "mode", "error"}},
		},
		{
			name: "sucesso",
			req:  domain.SetGroupJoinApprovalModeRequest{GroupJID: "g@g.us", Mode: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newReq()
			if tt.arrange != nil {
				tt.arrange(f)
			}

			res, err := f.uc.ExecuteSetGroupJoinApprovalMode(context.Background(), "u1", tt.req)

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
				if res.Details == "" {
					t.Error("Details vazio")
				}
				call := f.reqs.SetJoinApprovalModeCalls[0]
				if !call.Mode || call.Group != domain.JID("g@g.us") {
					t.Errorf("chamada = %+v", call)
				}
			}
			assertLog(t, f.log, tt.wantLog)
		})
	}
}

// TestGroupRequest_SetJoinApprovalModeFalso fixa que Mode=false chega à porta
// — o zero-value do bool não pode ser confundido com "campo ausente".
func TestGroupRequest_SetJoinApprovalModeAceitaFalse(t *testing.T) {
	f := newReq()
	if _, err := f.uc.ExecuteSetGroupJoinApprovalMode(context.Background(), "u1",
		domain.SetGroupJoinApprovalModeRequest{GroupJID: "g@g.us", Mode: false}); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(f.reqs.SetJoinApprovalModeCalls) != 1 || f.reqs.SetJoinApprovalModeCalls[0].Mode {
		t.Errorf("chamadas = %+v", f.reqs.SetJoinApprovalModeCalls)
	}
}
