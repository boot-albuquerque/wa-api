package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/group"
	"wa-api/pkg/domain"
)

// Cobertura dos onze handlers de ESCRITA de grupo (handler_group_mgmt.go).
//
// Todos compartilham o mesmo groupHandler generico, o mesmo decodeAndRespond e
// o mesmo GroupManagementUseCase — e por isso a divergencia entre eles e' o
// que a tabela persegue: um handler que esqueca de validar um campo, ou que
// responda >=400 sem logar a causa, aparece como uma unica linha vermelha.

// grpMgmtFakes reune os fakes que GroupManagementUseCase consome.
type grpMgmtFakes struct {
	lifecycle *contractsfake.GroupLifecycle
	settings  *contractsfake.GroupSettings
	jids      *contractsfake.JIDResolver
	logger    *contractsfake.Logger
}

func newGrpMgmtFakes() *grpMgmtFakes {
	return &grpMgmtFakes{
		lifecycle: &contractsfake.GroupLifecycle{},
		settings:  &contractsfake.GroupSettings{},
		jids:      &contractsfake.JIDResolver{},
		logger:    &contractsfake.Logger{},
	}
}

func (f *grpMgmtFakes) failSession(err error) {
	f.lifecycle.EnsureSessionFunc = func(context.Context, string) error { return err }
	f.settings.EnsureSessionFunc = func(context.Context, string) error { return err }
}

// handlers monta o conjunto real, pelo mesmo construtor que o bootstrap usa.
func (f *grpMgmtFakes) handlers() *GroupManagementHandlers {
	return NewGroupManagementHandlers(
		group.NewGroupManagementUseCase(f.lifecycle, f.settings, f.settings, f.settings, f.settings, f.jids, f.logger))
}

// grpMgmtMissing e' um caso de campo obrigatorio ausente: o corpo que o omite
// e a substring que o log tem de carregar como causa.
type grpMgmtMissing struct {
	name    string
	body    string
	wantErr string
}

// grpMgmtCase descreve uma das onze operacoes.
type grpMgmtCase struct {
	name   string
	path   string
	body   string
	pick   func(*GroupManagementHandlers) http.Handler
	failOp func(*grpMgmtFakes, error)
	// missing enumera as validacoes de 400 DESTE handler. Vazio significa
	// que a operacao nao valida campo algum na fronteira.
	missing []grpMgmtMissing
}

const grpMgmtJID = "120363@g.us"

// errGrpJID e' a recusa do resolver de JID — o caminho de saida que passa pelo
// resolver e nunca chega a' porta de escrita.
var errGrpJID = errors.New("malformed group jid")

func grpMgmtCases() []grpMgmtCase {
	return []grpMgmtCase{
		{
			name: "CreateGroup",
			path: "/group/create",
			body: `{"name":"squad","participants":["5511999999999"]}`,
			pick: func(h *GroupManagementHandlers) http.Handler { return h.CreateGroup },
			failOp: func(f *grpMgmtFakes, err error) {
				f.lifecycle.CreateGroupFunc = func(context.Context, string, string, []domain.JID, domain.CreateGroupOpts) (*domain.CreatedGroup, error) {
					return nil, err
				}
			},
			missing: []grpMgmtMissing{
				{"sem name", `{"participants":["5511999999999"]}`, "missing name"},
				{"sem participants", `{"name":"squad"}`, "missing participants"},
				// F101: o tamanho da lista passava e o ELEMENTO vazio descia
				// ate' o parser de JID, que entrava em panico nele.
				{"participante vazio", `{"name":"squad","participants":[""]}`, "empty participants at index 0"},
				// O indice tem de ser o REAL, e nao um zero constante: por isso
				// o vazio aqui esta na segunda posicao.
				{"participante vazio no meio", `{"name":"squad","participants":["5511999999999",""]}`, "empty participants at index 1"},
			},
		},
		{
			name: "GroupJoin",
			path: "/group/join",
			body: `{"code":"AbCdEf"}`,
			pick: func(h *GroupManagementHandlers) http.Handler { return h.GroupJoin },
			failOp: func(f *grpMgmtFakes, err error) {
				f.lifecycle.JoinGroupFunc = func(context.Context, string, string) (any, error) { return nil, err }
			},
			missing: []grpMgmtMissing{
				{"sem code", `{}`, "missing code"},
			},
		},
		{
			name: "GroupLeave",
			path: "/group/leave",
			body: `{"group_jid":"` + grpMgmtJID + `"}`,
			pick: func(h *GroupManagementHandlers) http.Handler { return h.GroupLeave },
			failOp: func(f *grpMgmtFakes, err error) {
				f.lifecycle.LeaveGroupFunc = func(context.Context, string, domain.JID) error { return err }
			},
			missing: []grpMgmtMissing{
				{"sem group_jid", `{}`, "missing group_jid"},
			},
		},
		{
			name: "SetGroupName",
			path: "/group/name",
			body: `{"group_jid":"` + grpMgmtJID + `","name":"squad"}`,
			pick: func(h *GroupManagementHandlers) http.Handler { return h.SetGroupName },
			failOp: func(f *grpMgmtFakes, err error) {
				f.settings.SetGroupNameFunc = func(context.Context, string, domain.JID, string) error { return err }
			},
			missing: []grpMgmtMissing{
				{"sem Name", `{"group_jid":"` + grpMgmtJID + `"}`, "missing name"},
			},
		},
		{
			name: "SetGroupTopic",
			path: "/group/topic",
			body: `{"group_jid":"` + grpMgmtJID + `","topic":"assunto"}`,
			pick: func(h *GroupManagementHandlers) http.Handler { return h.SetGroupTopic },
			failOp: func(f *grpMgmtFakes, err error) {
				f.settings.SetGroupTopicFunc = func(context.Context, string, domain.JID, string) error { return err }
			},
			missing: []grpMgmtMissing{
				{"sem Topic", `{"group_jid":"` + grpMgmtJID + `"}`, "missing topic"},
			},
		},
		{
			name: "SetGroupPhoto",
			path: "/group/photo",
			body: `{"group_jid":"` + grpMgmtJID + `","photo":"anBlZy1waG90by1kYXRh"}`,
			pick: func(h *GroupManagementHandlers) http.Handler { return h.SetGroupPhoto },
			failOp: func(f *grpMgmtFakes, err error) {
				f.settings.SetGroupPhotoFunc = func(context.Context, string, domain.JID, []byte) error { return err }
			},
			missing: []grpMgmtMissing{
				{"sem Photo", `{"group_jid":"` + grpMgmtJID + `"}`, "missing photo"},
			},
		},
		{
			name: "RemoveGroupPhoto",
			path: "/group/photo/remove",
			body: `{"group_jid":"` + grpMgmtJID + `"}`,
			pick: func(h *GroupManagementHandlers) http.Handler { return h.RemoveGroupPhoto },
			failOp: func(f *grpMgmtFakes, err error) {
				f.settings.SetGroupPhotoFunc = func(context.Context, string, domain.JID, []byte) error { return err }
			},
		},
		{
			name: "SetGroupAnnounce",
			path: "/group/announce",
			body: `{"group_jid":"` + grpMgmtJID + `","announce":true}`,
			pick: func(h *GroupManagementHandlers) http.Handler { return h.SetGroupAnnounce },
			failOp: func(f *grpMgmtFakes, err error) {
				f.settings.SetGroupAnnounceFunc = func(context.Context, string, domain.JID, bool) error { return err }
			},
		},
		{
			name: "SetGroupLocked",
			path: "/group/locked",
			body: `{"group_jid":"` + grpMgmtJID + `","locked":true}`,
			pick: func(h *GroupManagementHandlers) http.Handler { return h.SetGroupLocked },
			failOp: func(f *grpMgmtFakes, err error) {
				f.settings.SetGroupLockedFunc = func(context.Context, string, domain.JID, bool) error { return err }
			},
		},
		{
			name: "SetDisappearingTimer",
			path: "/group/disappearing",
			body: `{"group_jid":"` + grpMgmtJID + `","duration":"24h"}`,
			pick: func(h *GroupManagementHandlers) http.Handler { return h.SetDisappearingTimer },
			failOp: func(f *grpMgmtFakes, err error) {
				f.settings.SetDisappearingTimerFunc = func(context.Context, string, domain.JID, time.Duration, time.Time) error {
					return err
				}
			},
		},
		{
			name: "UpdateGroupParticipants",
			path: "/group/participants/update",
			body: `{"group_jid":"` + grpMgmtJID + `","phone":["5511999999999"],"action":"add"}`,
			pick: func(h *GroupManagementHandlers) http.Handler { return h.UpdateGroupParticipants },
			failOp: func(f *grpMgmtFakes, err error) {
				f.settings.UpdateGroupParticipantsFunc = func(context.Context, string, domain.JID, []domain.JID, domain.ParticipantAction) (domain.ParticipantsUpdate, error) {
					return domain.ParticipantsUpdate{}, err
				}
			},
			missing: []grpMgmtMissing{
				{"sem Phone", `{"group_jid":"` + grpMgmtJID + `"}`, "missing phones"},
				{"sem Action", `{"group_jid":"` + grpMgmtJID + `","phone":["5511999999999"]}`, "missing action"},
				{"phone vazio", `{"group_jid":"` + grpMgmtJID + `","phone":[""],"action":"add"}`, "empty phones at index 0"},
				// F101: este handler nunca validou GroupJID, entao o campo
				// ausente tambem alcancava o parser.
				{"sem group_jid", `{"phone":["5511999999999"],"action":"add"}`, "missing group_jid"},
			},
		},
	}
}

// grpMgmtServe roda a operacao na cadeia request-scoped de producao.
func grpMgmtServe(tc grpMgmtCase, f *grpMgmtFakes, body string) (*httptest.ResponseRecorder, *logCapture) {
	h, capture := logassert.Wrap(tc.pick(f.handlers()))
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(body))
	h.ServeHTTP(rec, withUser(r, "user-1"))
	return rec, capture
}

// TestGroupMgmtHandlers_Success: caminho feliz das onze operacoes.
func TestGroupMgmtHandlers_Success(t *testing.T) {
	for _, tc := range grpMgmtCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := newGrpMgmtFakes()
			rec, _ := grpMgmtServe(tc, f, tc.body)

			if rec.Code != http.StatusOK {
				t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
			}
			if env := decodeEnvelope(t, rec); !env.Success {
				t.Fatalf("envelope.success=false num caminho feliz: %s", rec.Body.String())
			}
		})
	}
}

// TestGroupMgmtHandlers_MalformedBody trava o unico decodeAndRespond que as
// onze operacoes compartilham: 400 e log warn com a causa do decode.
func TestGroupMgmtHandlers_MalformedBody(t *testing.T) {
	for _, tc := range grpMgmtCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := newGrpMgmtFakes()
			rec, capture := grpMgmtServe(tc, f, `{"group_jid": "1203`)

			assertErrorEnvelope(t, rec, http.StatusBadRequest)
			got := logassert.OutcomeLogged(t, capture.Records(t))
			if got.str("level") != "warn" {
				t.Fatalf("rejeicao causada pelo cliente logada em %q, quero warn", got.str("level"))
			}
			if len(f.lifecycle.CreateGroupCalls)+len(f.settings.SetGroupNameCalls) != 0 {
				t.Fatal("corpo malformado alcancou a porta")
			}
		})
	}
}

// TestGroupMgmtHandlers_MissingRequiredField: cada validacao de fronteira e'
// 400 e loga a causa. O log carrega a MESMA causa que o handler decidiu.
func TestGroupMgmtHandlers_MissingRequiredField(t *testing.T) {
	for _, tc := range grpMgmtCases() {
		for _, m := range tc.missing {
			t.Run(tc.name+"/"+m.name, func(t *testing.T) {
				f := newGrpMgmtFakes()
				rec, capture := grpMgmtServe(tc, f, m.body)

				assertErrorEnvelope(t, rec, http.StatusBadRequest)
				got := logassert.OutcomeLogged(t, capture.Records(t), m.wantErr)
				if got.str("level") != "warn" {
					t.Fatalf("rejeicao causada pelo cliente logada em %q, quero warn", got.str("level"))
				}
			})
		}
	}
}

// TestGroupMgmtHandlers_SessionFailure: a guarda de sessao do use case reprova
// e o handler responde 500 logando em nivel error.
func TestGroupMgmtHandlers_SessionFailure(t *testing.T) {
	for _, tc := range grpMgmtCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := newGrpMgmtFakes()
			f.failSession(grpErrNoSession)

			rec, capture := grpMgmtServe(tc, f, tc.body)

			assertErrorEnvelope(t, rec, http.StatusInternalServerError)
			got := logassert.OutcomeLogged(t, capture.Records(t), grpErrNoSession.Error())
			if got.str("level") != "error" {
				t.Fatalf("falha de dependencia logada em %q, quero error", got.str("level"))
			}
		})
	}
}

// TestGroupMgmtHandlers_UseCaseFailure: a operacao falha na porta, depois da
// guarda. A causa da porta tem de chegar ao log.
func TestGroupMgmtHandlers_UseCaseFailure(t *testing.T) {
	for _, tc := range grpMgmtCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := newGrpMgmtFakes()
			tc.failOp(f, grpErrPort)

			rec, capture := grpMgmtServe(tc, f, tc.body)

			assertErrorEnvelope(t, rec, http.StatusInternalServerError)
			got := logassert.OutcomeLogged(t, capture.Records(t), grpErrPort.Error())
			if got.str("level") != "error" {
				t.Fatalf("falha de porta logada em %q, quero error", got.str("level"))
			}
		})
	}
}

// TestGroupMgmtHandlers_JIDResolutionFailure: o JID do grupo nao resolve. E' o
// caminho de saida que passa pelo resolver e nao pela porta de escrita.
// 400 desde a F66: um JID que nao faz parse foi ENVIADO pelo cliente, e o
// remedio esta no payload dele. Este teste exigia 500.
func TestGroupMgmtHandlers_JIDResolutionFailure(t *testing.T) {
	const cause = "malformed group jid"
	for _, tc := range grpMgmtCases() {
		if tc.name == "CreateGroup" || tc.name == "GroupJoin" {
			// Nenhuma das duas resolve um JID de grupo: CreateGroup so'
			// resolve participantes, e GroupJoin trabalha sobre o codigo
			// de convite.
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			f := newGrpMgmtFakes()
			f.jids.ResolveJIDFunc = func(context.Context, string) (domain.JID, error) {
				return "", errGrpJID
			}

			rec, capture := grpMgmtServe(tc, f, tc.body)

			assertErrorEnvelope(t, rec, http.StatusBadRequest)
			logassert.OutcomeLogged(t, capture.Records(t), cause)
		})
	}
}

// TestGroupMgmtHandlers_NeverLogSecrets: clausula (d) do co-gate D sobre o
// caminho que mais loga desta camada.
func TestGroupMgmtHandlers_NeverLogSecrets(t *testing.T) {
	for _, tc := range grpMgmtCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := newGrpMgmtFakes()
			tc.failOp(f, grpErrPort)

			h, capture := logassert.Wrap(tc.pick(f.handlers()))
			rec := httptest.NewRecorder()
			body := `{"group_jid":"` + grpMgmtJID + `","group_jid":"` + grpMgmtJID +
				`","name":"squad","name":"squad","topic":"t","code":"AbCdEf",` +
				`"participants":["5511999999999"],"phone":["5511999999999"],"action":"add",` +
				`"secret":"` + logassertGlobalEncryptionKey + `","hmac":"` + logassertGlobalHMACKey + `"}`
			r := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(body))
			r.Header.Set("Authorization", logassertAdminToken)
			h.ServeHTTP(rec, withUser(r, "user-1"))

			if rec.Code < 400 {
				t.Fatalf("o caso de vazamento precisa de um caminho de saida; status %d", rec.Code)
			}
			logassert.NoSecrets(t, capture.Records(t))
		})
	}
}

// F225: chat alias for anonymous structs that carry GroupJID.
// These handlers use manual ChatAlias resolution (not ChatResolver interface).

type grpMgmtChatAliasCase struct {
	name     string
	chatBody string
	pick     func(*GroupManagementHandlers) http.Handler
}

func grpMgmtChatAliasCases() []grpMgmtChatAliasCase {
	return []grpMgmtChatAliasCase{
		{
			name:     "GroupLeave",
			chatBody: `{"chat":"` + grpMgmtJID + `"}`,
			pick:     func(h *GroupManagementHandlers) http.Handler { return h.GroupLeave },
		},
		{
			name:     "SetGroupName",
			chatBody: `{"chat":"` + grpMgmtJID + `","name":"squad"}`,
			pick:     func(h *GroupManagementHandlers) http.Handler { return h.SetGroupName },
		},
		{
			name:     "SetGroupTopic",
			chatBody: `{"chat":"` + grpMgmtJID + `","topic":"assunto"}`,
			pick:     func(h *GroupManagementHandlers) http.Handler { return h.SetGroupTopic },
		},
		{
			name:     "RemoveGroupPhoto",
			chatBody: `{"chat":"` + grpMgmtJID + `"}`,
			pick:     func(h *GroupManagementHandlers) http.Handler { return h.RemoveGroupPhoto },
		},
		{
			name:     "SetGroupAnnounce",
			chatBody: `{"chat":"` + grpMgmtJID + `","announce":true}`,
			pick:     func(h *GroupManagementHandlers) http.Handler { return h.SetGroupAnnounce },
		},
		{
			name:     "SetGroupLocked",
			chatBody: `{"chat":"` + grpMgmtJID + `","locked":true}`,
			pick:     func(h *GroupManagementHandlers) http.Handler { return h.SetGroupLocked },
		},
		{
			name:     "SetDisappearingTimer",
			chatBody: `{"chat":"` + grpMgmtJID + `","duration":"24h"}`,
			pick:     func(h *GroupManagementHandlers) http.Handler { return h.SetDisappearingTimer },
		},
		{
			name:     "UpdateGroupParticipants",
			chatBody: `{"chat":"` + grpMgmtJID + `","phone":["5511999999999"],"action":"add"}`,
			pick:     func(h *GroupManagementHandlers) http.Handler { return h.UpdateGroupParticipants },
		},
	}
}

func TestGroupMgmtHandlers_ChatAlias_Success(t *testing.T) {
	for _, tc := range grpMgmtChatAliasCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := newGrpMgmtFakes()
			h, _ := logassert.Wrap(tc.pick(f.handlers()))
			rec := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/group/test", strings.NewReader(tc.chatBody))
			h.ServeHTTP(rec, withUser(r, "user-1"))

			if rec.Code != http.StatusOK {
				t.Fatalf("chat alias produced status %d, want 200 (body: %s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestGroupMgmtHandlers_ChatAlias_LegacyWins(t *testing.T) {
	f := newGrpMgmtFakes()
	var captured string
	f.lifecycle.LeaveGroupFunc = func(_ context.Context, _ string, jid domain.JID) error {
		captured = string(jid)
		return nil
	}
	body := `{"group_jid":"legacy@g.us","chat":"alias@g.us"}`
	h, _ := logassert.Wrap(f.handlers().GroupLeave)
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/group/leave", strings.NewReader(body))
	h.ServeHTTP(rec, withUser(r, "user-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if !strings.HasPrefix(captured, "legacy") {
		t.Fatalf("legacy field should win: got %q", captured)
	}
}

// F247/F263: unknown participant action returns 400 from the handler, not
// 500. "promote" left this list in F263 — it is a valid action now, covered
// by TestUpdateGroupParticipants_PromoteAndDemote below.
func TestUpdateGroupParticipants_RejectsUnknownAction(t *testing.T) {
	for _, action := range []string{"approve", "qualquer-coisa", "ADD"} {
		t.Run(action, func(t *testing.T) {
			f := newGrpMgmtFakes()
			body := `{"group_jid":"` + grpMgmtJID + `","phone":["5511999999999"],"action":"` + action + `"}`
			rec, capture := grpMgmtServe(grpMgmtCase{
				name: "UpdateGroupParticipants",
				path: "/group/updateparticipants",
				body: body,
				pick: func(h *GroupManagementHandlers) http.Handler { return h.UpdateGroupParticipants },
			}, f, body)

			assertErrorEnvelope(t, rec, http.StatusBadRequest)
			got := logassert.OutcomeLogged(t, capture.Records(t), "unknown participant action")
			if got.str("level") != "warn" {
				t.Fatalf("client error logged at %q, want warn", got.str("level"))
			}
			if len(f.settings.UpdateGroupParticipantsCalls) != 0 {
				t.Fatal("unknown action reached the port")
			}
		})
	}
}

// F249: successful UpdateGroupParticipants includes per-participant result.
func TestUpdateGroupParticipants_ReturnsResult(t *testing.T) {
	f := newGrpMgmtFakes()
	f.settings.UpdateGroupParticipantsFunc = func(_ context.Context, _ string, _ domain.JID, _ []domain.JID, _ domain.ParticipantAction) (domain.ParticipantsUpdate, error) {
		return domain.ParticipantsUpdate{
			Participants: []domain.GroupParticipant{{JID: "5511999999999@s.whatsapp.net", IsAdmin: true}},
			Confirmed:    true,
		}, nil
	}
	body := `{"group_jid":"` + grpMgmtJID + `","phone":["5511999999999"],"action":"add"}`
	rec, _ := grpMgmtServe(grpMgmtCase{
		name: "UpdateGroupParticipants",
		path: "/group/updateparticipants",
		body: body,
		pick: func(h *GroupManagementHandlers) http.Handler { return h.UpdateGroupParticipants },
	}, f, body)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	respBody := rec.Body.String()
	// A lista sai TIPADA e em snake_case: era aqui que o struct de protocolo
	// do noise chegava ao fio com os nomes de campo do Go.
	if !strings.Contains(respBody, `"phone_number"`) || !strings.Contains(respBody, `"is_admin":true`) {
		t.Errorf("response should carry the mapped participant: %s", respBody)
	}
	if !strings.Contains(respBody, `"confirmed":true`) {
		t.Errorf("response should contain confirmed field: %s", respBody)
	}
}

// F263: promote and demote reach the port with the right domain.ParticipantAction,
// through the SAME handler chain grpMgmtServe uses for every other operation
// in this file (production request-scoped chain, not a raw use case call) —
// the boundary half of the F263 fix. The use-case-level translation is
// TestGroupManagement_UpdateParticipantsTraduzAction.
func TestUpdateGroupParticipants_PromoteAndDemote(t *testing.T) {
	tests := []struct {
		action string
		want   domain.ParticipantAction
	}{
		{"promote", domain.ParticipantPromote},
		{"demote", domain.ParticipantDemote},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			f := newGrpMgmtFakes()
			f.settings.UpdateGroupParticipantsFunc = func(_ context.Context, _ string, _ domain.JID, _ []domain.JID, _ domain.ParticipantAction) (domain.ParticipantsUpdate, error) {
				return domain.ParticipantsUpdate{
					Participants: []domain.GroupParticipant{{JID: "5511999999999@s.whatsapp.net", IsAdmin: tt.action == "promote"}},
					Confirmed:    true,
				}, nil
			}
			body := `{"group_jid":"` + grpMgmtJID + `","phone":["5511999999999"],"action":"` + tt.action + `"}`
			rec, _ := grpMgmtServe(grpMgmtCase{
				name: "UpdateGroupParticipants",
				path: "/group/updateparticipants",
				body: body,
				pick: func(h *GroupManagementHandlers) http.Handler { return h.UpdateGroupParticipants },
			}, f, body)

			if rec.Code != http.StatusOK {
				t.Fatalf("status %d, want 200 (body: %s)", rec.Code, rec.Body.String())
			}
			if len(f.settings.UpdateGroupParticipantsCalls) != 1 {
				t.Fatalf("port called %d time(s), want 1", len(f.settings.UpdateGroupParticipantsCalls))
			}
			if got := f.settings.UpdateGroupParticipantsCalls[0].Action; got != tt.want {
				t.Fatalf("Action = %q, want %q", got, tt.want)
			}
		})
	}
}

// F248: SetGroupPhoto rejects non-base64 input with 400.
func TestSetGroupPhoto_RejectsNonBase64(t *testing.T) {
	f := newGrpMgmtFakes()
	body := `{"group_jid":"` + grpMgmtJID + `","photo":"not-valid-base64!!!"}`
	rec, capture := grpMgmtServe(grpMgmtCase{
		name: "SetGroupPhoto",
		path: "/group/photo",
		body: body,
		pick: func(h *GroupManagementHandlers) http.Handler { return h.SetGroupPhoto },
	}, f, body)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	logassert.OutcomeLogged(t, capture.Records(t), "base64")
	if len(f.settings.SetGroupPhotoCalls) != 0 {
		t.Fatal("invalid base64 reached the port")
	}
}

// F248: SetGroupPhoto decodes base64 before passing to the port.
func TestSetGroupPhoto_DecodesBase64(t *testing.T) {
	f := newGrpMgmtFakes()
	var captured []byte
	f.settings.SetGroupPhotoFunc = func(_ context.Context, _ string, _ domain.JID, data []byte) error {
		captured = data
		return nil
	}
	body := `{"group_jid":"` + grpMgmtJID + `","photo":"anBlZy1waG90by1kYXRh"}`
	rec, _ := grpMgmtServe(grpMgmtCase{
		name: "SetGroupPhoto",
		path: "/group/photo",
		body: body,
		pick: func(h *GroupManagementHandlers) http.Handler { return h.SetGroupPhoto },
	}, f, body)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if string(captured) != "jpeg-photo-data" {
		t.Errorf("port received %q, want %q", string(captured), "jpeg-photo-data")
	}
}

// F237: CreateGroup accepts is_parent to create a community.
func TestCreateGroup_IsParent(t *testing.T) {
	f := newGrpMgmtFakes()
	var captured domain.CreateGroupOpts
	f.lifecycle.CreateGroupFunc = func(_ context.Context, _ string, _ string, _ []domain.JID, opts domain.CreateGroupOpts) (*domain.CreatedGroup, error) {
		captured = opts
		return &domain.CreatedGroup{Group: &domain.GroupInfo{Name: "My Community", IsParent: true}, Created: true}, nil
	}
	body := `{"name":"My Community","is_parent":true}`
	rec, _ := grpMgmtServe(grpMgmtCase{
		name: "CreateGroup",
		path: "/group/create",
		body: body,
		pick: func(h *GroupManagementHandlers) http.Handler { return h.CreateGroup },
	}, f, body)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if !captured.IsParent {
		t.Error("opts.IsParent not set")
	}
	call := f.lifecycle.CreateGroupCalls[0]
	if len(call.Participants) != 0 {
		t.Errorf("community creation should accept empty participants, got %d", len(call.Participants))
	}
}

// F237: CreateGroup accepts linked_parent_jid to create a group inside a community.
func TestCreateGroup_LinkedParentJID(t *testing.T) {
	f := newGrpMgmtFakes()
	var captured domain.CreateGroupOpts
	f.lifecycle.CreateGroupFunc = func(_ context.Context, _ string, _ string, _ []domain.JID, opts domain.CreateGroupOpts) (*domain.CreatedGroup, error) {
		captured = opts
		return &domain.CreatedGroup{Group: &domain.GroupInfo{Name: "Sub Group"}, Created: true}, nil
	}
	body := `{"name":"Sub Group","participants":["5511999999999"],"linked_parent_jid":"120363@g.us"}`
	rec, _ := grpMgmtServe(grpMgmtCase{
		name: "CreateGroup",
		path: "/group/create",
		body: body,
		pick: func(h *GroupManagementHandlers) http.Handler { return h.CreateGroup },
	}, f, body)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if captured.LinkedParentJID != "120363@g.us" {
		t.Errorf("opts.LinkedParentJID = %q, want 120363@g.us", captured.LinkedParentJID)
	}
}

// F237: is_parent and linked_parent_jid are mutually exclusive.
func TestCreateGroup_MutuallyExclusive(t *testing.T) {
	f := newGrpMgmtFakes()
	body := `{"name":"Bad","is_parent":true,"linked_parent_jid":"120363@g.us"}`
	rec, capture := grpMgmtServe(grpMgmtCase{
		name: "CreateGroup",
		path: "/group/create",
		body: body,
		pick: func(h *GroupManagementHandlers) http.Handler { return h.CreateGroup },
	}, f, body)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	logassert.OutcomeLogged(t, capture.Records(t), "mutually exclusive")
	if len(f.lifecycle.CreateGroupCalls) != 0 {
		t.Fatal("mutually exclusive fields reached the port")
	}
}

// F237: is_parent=true relaxes participants requirement.
func TestCreateGroup_IsParent_NoParticipantsRequired(t *testing.T) {
	f := newGrpMgmtFakes()
	f.lifecycle.CreateGroupFunc = func(context.Context, string, string, []domain.JID, domain.CreateGroupOpts) (*domain.CreatedGroup, error) {
		return &domain.CreatedGroup{Group: &domain.GroupInfo{Name: "Community"}, Created: true}, nil
	}
	body := `{"name":"Community","is_parent":true,"participants":[]}`
	rec, _ := grpMgmtServe(grpMgmtCase{
		name: "CreateGroup",
		path: "/group/create",
		body: body,
		pick: func(h *GroupManagementHandlers) http.Handler { return h.CreateGroup },
	}, f, body)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 — is_parent should relax participants requirement (body: %s)", rec.Code, rec.Body.String())
	}
}

// TestGroupMgmtHandlers_RejectWithoutSession: mesma guarda, sobre o
// groupHandler generico que as onze operacoes compartilham.
func TestGroupMgmtHandlers_RejectWithoutSession(t *testing.T) {
	for _, tc := range grpMgmtCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := newGrpMgmtFakes()
			h, _ := logassert.Wrap(tc.pick(f.handlers()))

			semUser := httptest.NewRecorder()
			h.ServeHTTP(semUser, httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body)))
			assertErrorEnvelope(t, semUser, http.StatusUnauthorized)

			semID := httptest.NewRecorder()
			h.ServeHTTP(semID, withUser(httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body)), ""))
			assertErrorEnvelope(t, semID, http.StatusBadRequest)

			if n := len(f.lifecycle.EnsureSessionCalls) + len(f.settings.EnsureSessionCalls); n != 0 {
				t.Fatalf("requisicao sem sessao alcancou a porta %d vez(es)", n)
			}
		})
	}
}
