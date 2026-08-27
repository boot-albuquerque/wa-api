package group_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/group"
	"wa-api/pkg/domain"
)

type mgmtFakes struct {
	life *contractsfake.GroupLifecycle
	set  *contractsfake.GroupSettings
	jids *contractsfake.JIDResolver
	log  *contractsfake.Logger
	uc   *group.GroupManagementUseCase
}

func newMgmt() *mgmtFakes {
	f := &mgmtFakes{
		life: &contractsfake.GroupLifecycle{},
		set:  &contractsfake.GroupSettings{},
		jids: &contractsfake.JIDResolver{},
		log:  &contractsfake.Logger{},
	}
	f.uc = group.NewGroupManagementUseCase(f.life, f.set, f.set, f.set, f.set, f.jids, f.log)
	return f
}

// failJID faz o resolver recusar qualquer entrada.
func failJID(err error) *contractsfake.JIDResolver {
	return &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", err },
	}
}

// TestGroupManagement_Ensure cobre a guarda de sessão compartilhada por todas
// as operações: cada método público tem de recusar antes de tocar a porta de
// escrita, propagando o erro tipado e deixando um Error estruturado.
func TestGroupManagement_SemSessaoRecusaAntesDeEscrever(t *testing.T) {
	ops := map[string]func(*group.GroupManagementUseCase) error{
		"CreateGroup": func(uc *group.GroupManagementUseCase) error {
			_, err := uc.CreateGroup(context.Background(), "u1", "g", []string{"5511987654321"}, domain.CreateGroupOpts{})
			return err
		},
		"JoinGroup": func(uc *group.GroupManagementUseCase) error {
			_, err := uc.JoinGroup(context.Background(), "u1", "code")
			return err
		},
		"LeaveGroup": func(uc *group.GroupManagementUseCase) error {
			return uc.LeaveGroup(context.Background(), "u1", "g@g.us")
		},
		"SetGroupName": func(uc *group.GroupManagementUseCase) error {
			return uc.SetGroupName(context.Background(), "u1", "g@g.us", "n")
		},
		"SetGroupTopic": func(uc *group.GroupManagementUseCase) error {
			return uc.SetGroupTopic(context.Background(), "u1", "g@g.us", "t")
		},
		"SetGroupPhoto": func(uc *group.GroupManagementUseCase) error {
			return uc.SetGroupPhoto(context.Background(), "u1", "g@g.us", []byte("x"))
		},
		"RemoveGroupPhoto": func(uc *group.GroupManagementUseCase) error {
			return uc.RemoveGroupPhoto(context.Background(), "u1", "g@g.us")
		},
		"SetGroupAnnounce": func(uc *group.GroupManagementUseCase) error {
			return uc.SetGroupAnnounce(context.Background(), "u1", "g@g.us", true)
		},
		"SetGroupLocked": func(uc *group.GroupManagementUseCase) error {
			return uc.SetGroupLocked(context.Background(), "u1", "g@g.us", true)
		},
		"SetDisappearingTimer": func(uc *group.GroupManagementUseCase) error {
			return uc.SetDisappearingTimer(context.Background(), "u1", "g@g.us", "24h")
		},
		"UpdateGroupParticipants": func(uc *group.GroupManagementUseCase) error {
			_, err := uc.UpdateGroupParticipants(context.Background(), "u1", "g@g.us", "add", []string{"5511987654321"})
			return err
		},
	}

	for name, call := range ops {
		t.Run(name, func(t *testing.T) {
			f := newMgmt()
			// A guarda vive em GroupSettings — é a porta que ensure consulta.
			f.set.SessionGuard = contractsfake.FailSession(sessErr())

			err := call(f.uc)
			if err == nil {
				t.Fatal("esperava erro de sessão")
			}
			assertCode(t, err, "no_session")
			assertLog(t, f.log, wantLog{contractsfake.LevelWarn, "no wanoise session", []string{"txtID", "error"}})

			// Nenhuma escrita pode ter acontecido.
			if n := len(f.life.CreateGroupCalls) + len(f.life.JoinGroupCalls) + len(f.life.LeaveGroupCalls) +
				len(f.set.SetGroupNameCalls) + len(f.set.SetGroupTopicCalls) + len(f.set.SetGroupPhotoCalls) +
				len(f.set.SetGroupAnnounceCalls) + len(f.set.SetGroupLockedCalls) +
				len(f.set.SetDisappearingTimerCalls) + len(f.set.UpdateGroupParticipantsCalls); n != 0 {
				t.Errorf("%d escritas ocorreram apesar da sessão recusada", n)
			}
		})
	}
}

// TestGroupManagement_JIDInvalido cobre parseJID e parseJIDs: cada operação
// que recebe um JID tem de recusar com log antes de escrever.
func TestGroupManagement_JIDInvalidoRecusaComLog(t *testing.T) {
	boom := errors.New("bad jid")

	tests := []struct {
		name    string
		call    func(*group.GroupManagementUseCase) error
		wantMsg string
		wantKey []string
	}{
		{
			name: "LeaveGroup",
			call: func(uc *group.GroupManagementUseCase) error {
				return uc.LeaveGroup(context.Background(), "u1", "@@")
			},
			wantMsg: "could not parse JID",
			wantKey: []string{"jid", "error"},
		},
		{
			name: "SetGroupName",
			call: func(uc *group.GroupManagementUseCase) error {
				return uc.SetGroupName(context.Background(), "u1", "@@", "n")
			},
			wantMsg: "could not parse JID",
			wantKey: []string{"jid", "error"},
		},
		{
			name: "SetGroupTopic",
			call: func(uc *group.GroupManagementUseCase) error {
				return uc.SetGroupTopic(context.Background(), "u1", "@@", "t")
			},
			wantMsg: "could not parse JID",
		},
		{
			name: "SetGroupPhoto",
			call: func(uc *group.GroupManagementUseCase) error {
				return uc.SetGroupPhoto(context.Background(), "u1", "@@", []byte("x"))
			},
			wantMsg: "could not parse JID",
		},
		{
			name: "RemoveGroupPhoto",
			call: func(uc *group.GroupManagementUseCase) error {
				return uc.RemoveGroupPhoto(context.Background(), "u1", "@@")
			},
			wantMsg: "could not parse JID",
		},
		{
			name: "SetGroupAnnounce",
			call: func(uc *group.GroupManagementUseCase) error {
				return uc.SetGroupAnnounce(context.Background(), "u1", "@@", true)
			},
			wantMsg: "could not parse JID",
		},
		{
			name: "SetGroupLocked",
			call: func(uc *group.GroupManagementUseCase) error {
				return uc.SetGroupLocked(context.Background(), "u1", "@@", false)
			},
			wantMsg: "could not parse JID",
		},
		{
			name: "SetDisappearingTimer",
			call: func(uc *group.GroupManagementUseCase) error {
				return uc.SetDisappearingTimer(context.Background(), "u1", "@@", "7d")
			},
			wantMsg: "could not parse JID",
		},
		{
			name: "CreateGroup passa pela lista de participantes",
			call: func(uc *group.GroupManagementUseCase) error {
				_, err := uc.CreateGroup(context.Background(), "u1", "g", []string{"ok", "@@"}, domain.CreateGroupOpts{})
				return err
			},
			wantMsg: "could not parse participant list",
			wantKey: []string{"index", "total", "error"},
		},
		{
			name: "UpdateGroupParticipants recusa o groupJID",
			call: func(uc *group.GroupManagementUseCase) error {
				_, err := uc.UpdateGroupParticipants(context.Background(), "u1", "@@", "add", []string{"5511987654321"})
				return err
			},
			wantMsg: "could not parse JID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newMgmt()
			f.jids = failJID(boom)
			f.uc = group.NewGroupManagementUseCase(f.life, f.set, f.set, f.set, f.set, f.jids, f.log)

			if err := tt.call(f.uc); err == nil {
				t.Fatal("esperava erro de JID")
			}
			assertLog(t, f.log, wantLog{contractsfake.LevelError, tt.wantMsg, tt.wantKey})
		})
	}
}

// TestGroupManagement_ParseJIDsIndice fixa que o log da lista de
// participantes identifica QUAL entrada reprovou — a informação que parseJID,
// sozinho, não tem.
func TestGroupManagement_ParseJIDsLogaOIndiceQueReprovou(t *testing.T) {
	f := newMgmt()
	f.jids = &contractsfake.JIDResolver{
		ResolveJIDFunc: func(_ context.Context, raw string) (domain.JID, error) {
			if raw == "ruim" {
				return "", errors.New("bad")
			}
			return domain.JID(raw), nil
		},
	}
	f.uc = group.NewGroupManagementUseCase(f.life, f.set, f.set, f.set, f.set, f.jids, f.log)

	if _, err := f.uc.CreateGroup(context.Background(), "u1", "g", []string{"a", "b", "ruim"}, domain.CreateGroupOpts{}); err == nil {
		t.Fatal("esperava erro")
	}
	rec, ok := f.log.FindLevel(contractsfake.LevelError, "could not parse participant list")
	if !ok {
		t.Fatalf("log ausente: %v", f.log.Messages())
	}
	if v, _ := rec.Keyval("index"); v != 2 {
		t.Errorf("index = %v, quero 2", v)
	}
	if v, _ := rec.Keyval("total"); v != 3 {
		t.Errorf("total = %v, quero 3", v)
	}
	if len(f.life.CreateGroupCalls) != 0 {
		t.Error("CreateGroup não deveria ter sido chamado")
	}
}

// TestGroupManagement_FalhaDaPorta cobre o caminho de saída de cada operação:
// erro da porta é logado com contexto e propagado verbatim.
func TestGroupManagement_FalhaDaPortaLogaEPropaga(t *testing.T) {
	boom := errors.New("upstream down")

	tests := []struct {
		name    string
		arrange func(*mgmtFakes)
		call    func(*group.GroupManagementUseCase) error
		wantMsg string
		wantKey []string
	}{
		{
			name: "CreateGroup",
			arrange: func(f *mgmtFakes) {
				f.life.CreateGroupFunc = func(context.Context, string, string, []domain.JID, domain.CreateGroupOpts) (*domain.CreatedGroup, error) {
					return nil, boom
				}
			},
			call: func(uc *group.GroupManagementUseCase) error {
				_, err := uc.CreateGroup(context.Background(), "u1", "g", []string{"5511987654321"}, domain.CreateGroupOpts{})
				return err
			},
			wantMsg: "failed to create group",
			wantKey: []string{"txtID", "name", "participants", "is_parent", "error"},
		},
		{
			name: "JoinGroup",
			arrange: func(f *mgmtFakes) {
				f.life.JoinGroupFunc = func(context.Context, string, string) (any, error) { return nil, boom }
			},
			call: func(uc *group.GroupManagementUseCase) error {
				_, err := uc.JoinGroup(context.Background(), "u1", "code")
				return err
			},
			wantMsg: "failed to join group",
			wantKey: []string{"txtID", "error"},
		},
		{
			name: "LeaveGroup",
			arrange: func(f *mgmtFakes) {
				f.life.LeaveGroupFunc = func(context.Context, string, domain.JID) error { return boom }
			},
			call: func(uc *group.GroupManagementUseCase) error {
				return uc.LeaveGroup(context.Background(), "u1", "g@g.us")
			},
			wantMsg: "failed to leave group",
			wantKey: []string{"txtID", "groupJID", "error"},
		},
		{
			name: "SetGroupName",
			arrange: func(f *mgmtFakes) {
				f.set.SetGroupNameFunc = func(context.Context, string, domain.JID, string) error { return boom }
			},
			call: func(uc *group.GroupManagementUseCase) error {
				return uc.SetGroupName(context.Background(), "u1", "g@g.us", "n")
			},
			wantMsg: "failed to set group name",
			wantKey: []string{"txtID", "groupJID", "error"},
		},
		{
			name: "SetGroupTopic",
			arrange: func(f *mgmtFakes) {
				f.set.SetGroupTopicFunc = func(context.Context, string, domain.JID, string) error { return boom }
			},
			call: func(uc *group.GroupManagementUseCase) error {
				return uc.SetGroupTopic(context.Background(), "u1", "g@g.us", "t")
			},
			wantMsg: "failed to set group topic",
			wantKey: []string{"txtID", "groupJID", "error"},
		},
		{
			name: "SetGroupPhoto",
			arrange: func(f *mgmtFakes) {
				f.set.SetGroupPhotoFunc = func(context.Context, string, domain.JID, []byte) error { return boom }
			},
			call: func(uc *group.GroupManagementUseCase) error {
				return uc.SetGroupPhoto(context.Background(), "u1", "g@g.us", []byte("xy"))
			},
			wantMsg: "failed to set group photo",
			wantKey: []string{"txtID", "groupJID", "bytes", "error"},
		},
		{
			name: "RemoveGroupPhoto",
			arrange: func(f *mgmtFakes) {
				f.set.SetGroupPhotoFunc = func(context.Context, string, domain.JID, []byte) error { return boom }
			},
			call: func(uc *group.GroupManagementUseCase) error {
				return uc.RemoveGroupPhoto(context.Background(), "u1", "g@g.us")
			},
			wantMsg: "failed to remove group photo",
			wantKey: []string{"txtID", "groupJID", "error"},
		},
		{
			name: "SetGroupAnnounce",
			arrange: func(f *mgmtFakes) {
				f.set.SetGroupAnnounceFunc = func(context.Context, string, domain.JID, bool) error { return boom }
			},
			call: func(uc *group.GroupManagementUseCase) error {
				return uc.SetGroupAnnounce(context.Background(), "u1", "g@g.us", true)
			},
			wantMsg: "failed to set group announce",
			wantKey: []string{"txtID", "groupJID", "announce", "error"},
		},
		{
			name: "SetGroupLocked",
			arrange: func(f *mgmtFakes) {
				f.set.SetGroupLockedFunc = func(context.Context, string, domain.JID, bool) error { return boom }
			},
			call: func(uc *group.GroupManagementUseCase) error {
				return uc.SetGroupLocked(context.Background(), "u1", "g@g.us", true)
			},
			wantMsg: "failed to set group locked",
			wantKey: []string{"txtID", "groupJID", "locked", "error"},
		},
		{
			name: "SetDisappearingTimer",
			arrange: func(f *mgmtFakes) {
				f.set.SetDisappearingTimerFunc = func(context.Context, string, domain.JID, time.Duration, time.Time) error {
					return boom
				}
			},
			call: func(uc *group.GroupManagementUseCase) error {
				return uc.SetDisappearingTimer(context.Background(), "u1", "g@g.us", "24h")
			},
			wantMsg: "failed to set disappearing timer",
			wantKey: []string{"txtID", "groupJID", "duration", "error"},
		},
		{
			name: "UpdateGroupParticipants",
			arrange: func(f *mgmtFakes) {
				f.set.UpdateGroupParticipantsFunc = func(context.Context, string, domain.JID, []domain.JID, domain.ParticipantAction) (domain.ParticipantsUpdate, error) {
					return domain.ParticipantsUpdate{}, boom
				}
			},
			call: func(uc *group.GroupManagementUseCase) error {
				_, err := uc.UpdateGroupParticipants(context.Background(), "u1", "g@g.us", "add", []string{"5511987654321"})
				return err
			},
			wantMsg: "failed to update group participants",
			wantKey: []string{"txtID", "groupJID", "action", "participants", "error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newMgmt()
			tt.arrange(f)

			err := tt.call(f.uc)
			if !errors.Is(err, boom) {
				t.Fatalf("erro = %v, quero a causa propagada verbatim", err)
			}
			assertLog(t, f.log, wantLog{contractsfake.LevelError, tt.wantMsg, tt.wantKey})
		})
	}
}

// TestGroupManagement_CaminhoFeliz cobre o retorno de sucesso de cada
// operação e os argumentos que chegam à porta.
func TestGroupManagement_CaminhoFeliz(t *testing.T) {
	ctx := context.Background()

	t.Run("CreateGroup repassa nome e participantes resolvidos", func(t *testing.T) {
		f := newMgmt()
		f.life.CreateGroupFunc = func(context.Context, string, string, []domain.JID, domain.CreateGroupOpts) (*domain.CreatedGroup, error) {
			return &domain.CreatedGroup{Group: &domain.GroupInfo{Name: "meu grupo"}, Created: true}, nil
		}

		res, err := f.uc.CreateGroup(ctx, "u1", "meu grupo", []string{"5511987654321", "5522987654321"}, domain.CreateGroupOpts{})
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if res == nil || res.Group == nil || res.Group.Name != "meu grupo" || !res.Created {
			t.Errorf("res = %+v", res)
		}
		call := f.life.CreateGroupCalls[0]
		if call.Name != "meu grupo" || len(call.Participants) != 2 ||
			call.Participants[0] != domain.JID("5511987654321@s.whatsapp.net") || call.Participants[1] != domain.JID("5522987654321@s.whatsapp.net") {
			t.Errorf("chamada = %+v", call)
		}
		assertNoLevel(t, f.log, contractsfake.LevelError)
	})

	t.Run("JoinGroup repassa o code", func(t *testing.T) {
		f := newMgmt()
		f.life.JoinGroupFunc = func(context.Context, string, string) (any, error) { return "joined", nil }

		res, err := f.uc.JoinGroup(ctx, "u1", "invite-code")
		if err != nil || res != "joined" {
			t.Fatalf("res=%v err=%v", res, err)
		}
		if f.life.JoinGroupCalls[0].Code != "invite-code" {
			t.Errorf("code = %q", f.life.JoinGroupCalls[0].Code)
		}
	})

	t.Run("LeaveGroup", func(t *testing.T) {
		f := newMgmt()
		if err := f.uc.LeaveGroup(ctx, "u1", "g@g.us"); err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if f.life.LeaveGroupCalls[0].Group != domain.JID("g@g.us") {
			t.Errorf("group = %v", f.life.LeaveGroupCalls[0].Group)
		}
	})

	t.Run("SetGroupName", func(t *testing.T) {
		f := newMgmt()
		if err := f.uc.SetGroupName(ctx, "u1", "g@g.us", "novo"); err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if f.set.SetGroupNameCalls[0].Name != "novo" {
			t.Errorf("name = %q", f.set.SetGroupNameCalls[0].Name)
		}
	})

	t.Run("SetGroupTopic", func(t *testing.T) {
		f := newMgmt()
		if err := f.uc.SetGroupTopic(ctx, "u1", "g@g.us", "assunto"); err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if f.set.SetGroupTopicCalls[0].Topic != "assunto" {
			t.Errorf("topic = %q", f.set.SetGroupTopicCalls[0].Topic)
		}
	})

	t.Run("SetGroupPhoto envia os bytes", func(t *testing.T) {
		f := newMgmt()
		if err := f.uc.SetGroupPhoto(ctx, "u1", "g@g.us", []byte("jpeg")); err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if string(f.set.SetGroupPhotoCalls[0].Photo) != "jpeg" {
			t.Errorf("photo = %q", f.set.SetGroupPhotoCalls[0].Photo)
		}
	})

	t.Run("RemoveGroupPhoto envia nil, que é o contrato de remoção", func(t *testing.T) {
		f := newMgmt()
		if err := f.uc.RemoveGroupPhoto(ctx, "u1", "g@g.us"); err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if f.set.SetGroupPhotoCalls[0].Photo != nil {
			t.Errorf("photo deveria ser nil, veio %q", f.set.SetGroupPhotoCalls[0].Photo)
		}
	})

	t.Run("SetGroupAnnounce", func(t *testing.T) {
		f := newMgmt()
		if err := f.uc.SetGroupAnnounce(ctx, "u1", "g@g.us", true); err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if !f.set.SetGroupAnnounceCalls[0].Announce {
			t.Error("announce = false")
		}
	})

	t.Run("SetGroupLocked", func(t *testing.T) {
		f := newMgmt()
		if err := f.uc.SetGroupLocked(ctx, "u1", "g@g.us", true); err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if !f.set.SetGroupLockedCalls[0].Locked {
			t.Error("locked = false")
		}
	})
}

// TestGroupManagement_DisappearingTimerTraduzDuracao fixa a tabela de
// tradução e o aviso do caso default, que desliga o timer em silêncio.
func TestGroupManagement_DisappearingTimerTraduzDuracao(t *testing.T) {
	tests := []struct {
		duration string
		want     time.Duration
		wantWarn bool
	}{
		{"24h", 24 * time.Hour, false},
		{"7d", 7 * 24 * time.Hour, false},
		{"90d", 90 * 24 * time.Hour, false},
		{"off", 0, true},
		{"", 0, true},
		{"lixo", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.duration, func(t *testing.T) {
			f := newMgmt()
			if err := f.uc.SetDisappearingTimer(context.Background(), "u1", "g@g.us", tt.duration); err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			call := f.set.SetDisappearingTimerCalls[0]
			if call.Duration != tt.want {
				t.Errorf("Duration = %v, quero %v", call.Duration, tt.want)
			}
			if call.At.IsZero() {
				t.Error("At não foi preenchido")
			}
			rec, ok := f.log.FindLevel(contractsfake.LevelWarn, "unrecognized disappearing timer duration, disabling")
			if ok != tt.wantWarn {
				t.Fatalf("warn presente = %v, quero %v (registros=%v)", ok, tt.wantWarn, f.log.Messages())
			}
			if tt.wantWarn {
				if v, _ := rec.Keyval("duration"); v != tt.duration {
					t.Errorf("duration no log = %v", v)
				}
				if !rec.HasKey("groupJID") {
					t.Error("warn sem groupJID")
				}
			}
		})
	}
}

// TestGroupManagement_UpdateParticipantsAction fixa a regra preservada do
// upstream: só "add" adiciona; qualquer outro valor remove.
func TestGroupManagement_UpdateParticipantsTraduzAction(t *testing.T) {
	tests := []struct {
		action string
		want   domain.ParticipantAction
	}{
		{"add", domain.ParticipantAdd},
		{"remove", domain.ParticipantRemove},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			f := newMgmt()
			f.set.UpdateGroupParticipantsFunc = func(context.Context, string, domain.JID, []domain.JID, domain.ParticipantAction) (domain.ParticipantsUpdate, error) {
				return domain.ParticipantsUpdate{
					Participants: []domain.GroupParticipant{{JID: "5511987654321@s.whatsapp.net"}},
					Confirmed:    true,
				}, nil
			}

			res, err := f.uc.UpdateGroupParticipants(context.Background(), "u1", "g@g.us", tt.action, []string{"5511987654321", "5522987654321"})
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if len(res.Participants) != 1 || res.Participants[0].JID != "5511987654321@s.whatsapp.net" {
				t.Errorf("res.Participants = %#v", res.Participants)
			}
			if !res.Confirmed {
				t.Errorf("res.Confirmed = false com Reason %q; este dublê imita o "+
					"socket, que confirma na propria chamada", res.Reason)
			}
			call := f.set.UpdateGroupParticipantsCalls[0]
			if call.Action != tt.want {
				t.Errorf("Action = %q, quero %q", call.Action, tt.want)
			}
			if len(call.Participants) != 2 {
				t.Errorf("participantes = %v", call.Participants)
			}
			if call.Group != domain.JID("g@g.us") {
				t.Errorf("group = %v", call.Group)
			}
		})
	}
}

// F247: unknown actions must be rejected, not silently treated as remove.
func TestGroupManagement_UpdateParticipantsRejectsUnknownAction(t *testing.T) {
	for _, action := range []string{"", "qualquer-coisa", "approve", "promote"} {
		t.Run(action, func(t *testing.T) {
			f := newMgmt()
			_, err := f.uc.UpdateGroupParticipants(context.Background(), "u1", "g@g.us", action, []string{"5511987654321"})
			if err == nil {
				t.Fatal("expected error for unknown action, got nil")
			}
			if !strings.Contains(err.Error(), "unknown participant action") {
				t.Errorf("error = %q, want substring %q", err.Error(), "unknown participant action")
			}
			if len(f.set.UpdateGroupParticipantsCalls) != 0 {
				t.Error("unknown action reached the port")
			}
		})
	}
}

// TestGroupManagement_UpdateParticipantsListaInvalida cobre o parseJIDs
// depois do parseJID do grupo — a ordem importa, e a lista reprovada não pode
// chegar à porta.
func TestGroupManagement_UpdateParticipantsListaInvalida(t *testing.T) {
	f := newMgmt()
	f.jids = &contractsfake.JIDResolver{
		ResolveJIDFunc: func(_ context.Context, raw string) (domain.JID, error) {
			if raw == "ruim" {
				return "", errors.New("bad")
			}
			return domain.JID(raw), nil
		},
	}
	f.uc = group.NewGroupManagementUseCase(f.life, f.set, f.set, f.set, f.set, f.jids, f.log)

	if _, err := f.uc.UpdateGroupParticipants(context.Background(), "u1", "g@g.us", "add", []string{"ruim"}); err == nil {
		t.Fatal("esperava erro")
	}
	if len(f.set.UpdateGroupParticipantsCalls) != 0 {
		t.Error("a porta foi chamada com lista inválida")
	}
	assertLog(t, f.log, wantLog{contractsfake.LevelError, "could not parse participant list", []string{"index", "total"}})
}

// TestGroupManagement_CreateGroupSemParticipantes fixa que a lista vazia é
// aceita — parseJIDs devolve slice vazio, não erro.
func TestGroupManagement_CreateGroupSemParticipantes(t *testing.T) {
	f := newMgmt()
	f.life.CreateGroupFunc = func(_ context.Context, _ string, _ string, p []domain.JID, _ domain.CreateGroupOpts) (*domain.CreatedGroup, error) {
		if len(p) != 0 {
			t.Errorf("participantes = %v, quero vazio", p)
		}
		return &domain.CreatedGroup{Group: &domain.GroupInfo{}, Created: true}, nil
	}

	if _, err := f.uc.CreateGroup(context.Background(), "u1", "só eu", nil, domain.CreateGroupOpts{}); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	assertNoLevel(t, f.log, contractsfake.LevelError)
}

// F237: CreateGroup passes community opts through to the port.
func TestGroupManagement_CreateGroupPassesOpts(t *testing.T) {
	f := newMgmt()
	f.life.CreateGroupFunc = func(_ context.Context, _ string, _ string, _ []domain.JID, opts domain.CreateGroupOpts) (*domain.CreatedGroup, error) {
		if !opts.IsParent {
			t.Error("opts.IsParent not forwarded")
		}
		return &domain.CreatedGroup{Group: &domain.GroupInfo{}, Created: true}, nil
	}
	opts := domain.CreateGroupOpts{IsParent: true}
	if _, err := f.uc.CreateGroup(context.Background(), "u1", "community", nil, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	call := f.life.CreateGroupCalls[0]
	if !call.Opts.IsParent {
		t.Error("CreateGroupCalls[0].Opts.IsParent = false")
	}
}

// F237: CreateGroup forwards LinkedParentJID through opts.
func TestGroupManagement_CreateGroupLinkedParent(t *testing.T) {
	f := newMgmt()
	f.life.CreateGroupFunc = func(_ context.Context, _ string, _ string, _ []domain.JID, opts domain.CreateGroupOpts) (*domain.CreatedGroup, error) {
		if opts.LinkedParentJID != "120363@g.us" {
			t.Errorf("LinkedParentJID = %q", opts.LinkedParentJID)
		}
		return &domain.CreatedGroup{Group: &domain.GroupInfo{}, Created: true}, nil
	}
	opts := domain.CreateGroupOpts{LinkedParentJID: "120363@g.us"}
	if _, err := f.uc.CreateGroup(context.Background(), "u1", "sub", []string{"5511987654321"}, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
