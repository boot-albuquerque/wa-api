package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wa-api/pkg/application/usecase/group"
	"wa-api/pkg/domain"
	customhttp "wa-api/pkg/presentation/http"
	"wa-api/pkg/presentation/http/contracttest"

	"github.com/gorilla/mux"
)

// Teste de contrato das VINTE rotas da família de grupo e comunidade que a
// migração para DTO cobriu — todas menos /group/info, que é a implementação de
// referência e tem o seu próprio ficheiro.
//
// O padrão é o de handler_group_info_contract_test.go, e o que ele afirma é o
// mesmo: pedido pela ROTA REGISTADA, toda chave em snake_case minúsculo
// recursivamente, e as chaves ANTIGAS desaparecidas — "a chave nova existe"
// não prova migração, porque um struct pode carregar as duas.

// chavesAntigasDaFamilia são os nomes que estas rotas serviam ANTES: nomes de
// campo Go emitidos pelo `encoding/json` sobre structs de protocolo sem tag, e
// os `Details` escritos à mão nos mapas dos handlers de escrita.
var chavesAntigasDaFamilia = []string{
	"Details", "Result", "GroupJID", "groupJID", "groupjid", "CommunityJID",
	"communityJID", "InviteLink", "InviteInfo", "Groups", "SubGroups",
	"Participants", "Phone", "Action", "Code", "JID", "Name", "Topic",
	"IsAdmin", "IsSuperAdmin", "PhoneNumber", "LID", "RequestedAt",
	"IsDefaultSubGroup", "AddRequest", "Error",
}

// familiaRouter monta as rotas EXACTAMENTE como wiring_routes.go as monta, com
// os métodos que ele regista — /group/requestparticipants é GET, e um teste que
// a montasse como POST não exercitaria o caminho de query string.
func familiaRouter(t *testing.T, f *grpFakes, m *grpMgmtFakes, c *comFakes) *mux.Router {
	t.Helper()
	registry := customhttp.NewHandlerRegistry()

	read := group.NewGroupRequestUseCase(f.requests, f.jids, f.logger)
	mgmt := m.handlers()
	comRead := group.NewCommunityReadUseCase(c.directory, c.jids, c.logger)
	comWrite := group.NewCommunityWriteUseCase(c.lifecycle, c.jids, c.logger)

	reg := func(path string, h http.Handler, method string) {
		registry.Register(path, withContractUser(h), method)
	}

	reg("/group/requestparticipants", NewGetGroupRequestParticipantsHandler(read), http.MethodGet)
	reg("/group/updaterequestparticipants", NewUpdateGroupRequestParticipantsHandler(read), http.MethodPost)
	reg("/group/joinapprovalmode", NewSetGroupJoinApprovalModeHandler(read), http.MethodPost)
	reg("/group/list", NewListGroupsHandler(group.NewListGroupsUseCase(f.directory, f.logger)), http.MethodPost)
	reg("/group/invitelink", NewGetGroupInviteLinkHandler(group.NewGetGroupInviteLinkUseCase(f.directory, f.jids, f.logger)), http.MethodPost)
	reg("/group/inviteinfo", NewGetGroupInviteInfoHandler(group.NewGetGroupInviteInfoUseCase(f.directory, f.logger)), http.MethodPost)

	reg("/group/create", mgmt.CreateGroup, http.MethodPost)
	reg("/group/join", mgmt.GroupJoin, http.MethodPost)
	reg("/group/leave", mgmt.GroupLeave, http.MethodPost)
	reg("/group/name", mgmt.SetGroupName, http.MethodPost)
	reg("/group/topic", mgmt.SetGroupTopic, http.MethodPost)
	reg("/group/photo", mgmt.SetGroupPhoto, http.MethodPost)
	reg("/group/photo/remove", mgmt.RemoveGroupPhoto, http.MethodPost)
	reg("/group/announce", mgmt.SetGroupAnnounce, http.MethodPost)
	reg("/group/locked", mgmt.SetGroupLocked, http.MethodPost)
	reg("/group/ephemeral", mgmt.SetDisappearingTimer, http.MethodPost)
	reg("/group/updateparticipants", mgmt.UpdateGroupParticipants, http.MethodPost)

	reg("/community/subgroups", NewGetCommunitySubGroupsHandler(comRead), http.MethodPost)
	reg("/community/participants", NewGetCommunityParticipantsHandler(comRead), http.MethodPost)
	reg("/community/link", NewCommunityLinkGroupHandler(comWrite), http.MethodPost)
	reg("/community/unlink", NewCommunityUnlinkGroupHandler(comWrite), http.MethodPost)

	router := mux.NewRouter()
	registry.Apply(router)
	return router
}

// familiaCase é uma rota migrada, com o pedido que a leva ao 200.
type familiaCase struct {
	nome   string
	metodo string
	alvo   string
	corpo  string
	// arrange enche os dublês com dados MEDIDOS onde a resposta os carrega —
	// um dublê no zero deixaria metade do mapeamento por medir.
	arrange func(*grpFakes, *grpMgmtFakes, *comFakes)
}

const familiaJID = "120363000000000000@g.us"

func familiaCasos() []familiaCase {
	return []familiaCase{
		{
			nome: "GetGroupRequestParticipants", metodo: http.MethodGet,
			alvo: "/group/requestparticipants?group_jid=" + familiaJID, corpo: "",
			arrange: func(f *grpFakes, _ *grpMgmtFakes, _ *comFakes) {
				f.requests.GetRequestParticipantsFunc = func(context.Context, string, domain.JID) ([]domain.GroupJoinRequest, error) {
					return []domain.GroupJoinRequest{{
						JID:         "5511999999999@s.whatsapp.net",
						RequestedAt: time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC),
						AddedByJID:  "5511888888888@s.whatsapp.net",
						Method:      "InviteLink",
					}}, nil
				}
			},
		},
		{
			nome: "UpdateGroupRequestParticipants", metodo: http.MethodPost,
			alvo:  "/group/updaterequestparticipants",
			corpo: `{"group_jid":"` + familiaJID + `","phone":["5511999999999@s.whatsapp.net"],"action":"approve"}`,
		},
		{
			nome: "SetGroupJoinApprovalMode", metodo: http.MethodPost,
			alvo:  "/group/joinapprovalmode",
			corpo: `{"group_jid":"` + familiaJID + `","mode":true}`,
		},
		{
			nome: "ListGroups", metodo: http.MethodPost, alvo: "/group/list", corpo: "",
			arrange: func(f *grpFakes, _ *grpMgmtFakes, _ *comFakes) {
				f.directory.ListJoinedGroupsFunc = func(context.Context, string) ([]*domain.GroupInfo, int, error) {
					return []*domain.GroupInfo{grupoDeReferencia()}, 1, nil
				}
			},
		},
		{
			nome: "GetGroupInviteLink", metodo: http.MethodPost, alvo: "/group/invitelink",
			corpo: `{"group_jid":"` + familiaJID + `"}`,
			arrange: func(f *grpFakes, _ *grpMgmtFakes, _ *comFakes) {
				f.directory.GetGroupInviteLinkFunc = func(context.Context, string, domain.JID) (string, error) {
					return "https://chat.whatsapp.com/ABC", nil
				}
			},
		},
		{
			nome: "GetGroupInviteInfo", metodo: http.MethodPost, alvo: "/group/inviteinfo",
			corpo: `{"code":"AbCdEf"}`,
			arrange: func(f *grpFakes, _ *grpMgmtFakes, _ *comFakes) {
				f.directory.GetGroupInfoFromLinkFunc = func(context.Context, string, string) (*domain.GroupInfo, error) {
					return grupoDeReferencia(), nil
				}
			},
		},
		{
			nome: "CreateGroup", metodo: http.MethodPost, alvo: "/group/create",
			corpo: `{"name":"Equipa","participants":["5511999999999"]}`,
			arrange: func(_ *grpFakes, m *grpMgmtFakes, _ *comFakes) {
				m.lifecycle.CreateGroupFunc = func(context.Context, string, string, []domain.JID, domain.CreateGroupOpts) (*domain.CreatedGroup, error) {
					return &domain.CreatedGroup{Group: grupoDeReferencia(), Created: true}, nil
				}
			},
		},
		{nome: "GroupJoin", metodo: http.MethodPost, alvo: "/group/join", corpo: `{"code":"AbCdEf"}`},
		{nome: "GroupLeave", metodo: http.MethodPost, alvo: "/group/leave", corpo: `{"group_jid":"` + familiaJID + `"}`},
		{nome: "SetGroupName", metodo: http.MethodPost, alvo: "/group/name",
			corpo: `{"group_jid":"` + familiaJID + `","name":"Equipa"}`},
		{nome: "SetGroupTopic", metodo: http.MethodPost, alvo: "/group/topic",
			corpo: `{"group_jid":"` + familiaJID + `","topic":"Assuntos"}`},
		{nome: "SetGroupPhoto", metodo: http.MethodPost, alvo: "/group/photo",
			corpo: `{"group_jid":"` + familiaJID + `","photo":"anBlZy1waG90by1kYXRh"}`},
		{nome: "RemoveGroupPhoto", metodo: http.MethodPost, alvo: "/group/photo/remove",
			corpo: `{"group_jid":"` + familiaJID + `"}`},
		{nome: "SetGroupAnnounce", metodo: http.MethodPost, alvo: "/group/announce",
			corpo: `{"group_jid":"` + familiaJID + `","announce":true}`},
		{nome: "SetGroupLocked", metodo: http.MethodPost, alvo: "/group/locked",
			corpo: `{"group_jid":"` + familiaJID + `","locked":true}`},
		{nome: "SetDisappearingTimer", metodo: http.MethodPost, alvo: "/group/ephemeral",
			corpo: `{"group_jid":"` + familiaJID + `","duration":"24h"}`},
		{
			nome: "UpdateGroupParticipants", metodo: http.MethodPost, alvo: "/group/updateparticipants",
			corpo: `{"group_jid":"` + familiaJID + `","phone":["5511999999999"],"action":"add"}`,
			arrange: func(_ *grpFakes, m *grpMgmtFakes, _ *comFakes) {
				m.settings.UpdateGroupParticipantsFunc = func(context.Context, string, domain.JID, []domain.JID, domain.ParticipantAction) (domain.ParticipantsUpdate, error) {
					return domain.ParticipantsUpdate{
						Participants: []domain.GroupParticipant{{
							JID:         "5511999999999@s.whatsapp.net",
							PhoneNumber: "5511999999999@s.whatsapp.net",
							LID:         "111111111111111@lid",
							IsAdmin:     true,
						}},
						Confirmed: true,
					}, nil
				}
			},
		},
		{
			nome: "GetCommunitySubGroups", metodo: http.MethodPost, alvo: "/community/subgroups",
			corpo: `{"community_jid":"` + familiaJID + `"}`,
			arrange: func(_ *grpFakes, _ *grpMgmtFakes, c *comFakes) {
				c.directory.GetSubGroupsFunc = func(context.Context, string, domain.JID) ([]domain.CommunitySubGroup, error) {
					return []domain.CommunitySubGroup{
						{JID: "120363111111111111@g.us", Name: "Avisos", IsDefaultSubGroup: true},
					}, nil
				}
			},
		},
		{
			nome: "GetCommunityParticipants", metodo: http.MethodPost, alvo: "/community/participants",
			corpo: `{"community_jid":"` + familiaJID + `"}`,
			arrange: func(_ *grpFakes, _ *grpMgmtFakes, c *comFakes) {
				c.directory.GetLinkedGroupsParticipantsFunc = func(context.Context, string, domain.JID) ([]domain.JID, error) {
					return []domain.JID{"5511999999999@s.whatsapp.net"}, nil
				}
			},
		},
		{nome: "CommunityLink", metodo: http.MethodPost, alvo: "/community/link",
			corpo: `{"community_jid":"` + familiaJID + `","group_jid":"120363111111111111@g.us"}`},
		{nome: "CommunityUnlink", metodo: http.MethodPost, alvo: "/community/unlink",
			corpo: `{"community_jid":"` + familiaJID + `","group_jid":"120363111111111111@g.us"}`},
	}
}

// serveFamilia corre um caso pela rota registada e devolve o gravador.
func serveFamilia(t *testing.T, tc familiaCase) *httptest.ResponseRecorder {
	t.Helper()
	f, m, c := newGrpFakes(), newGrpMgmtFakes(), newComFakes()
	if tc.arrange != nil {
		tc.arrange(f, m, c)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(tc.metodo, tc.alvo, strings.NewReader(tc.corpo))
	familiaRouter(t, f, m, c).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200; corpo: %s", rec.Code, rec.Body.String())
	}
	return rec
}

// TestFamiliaGrupo_ContratoPublico_NomesCanonicos: toda chave de todo corpo
// servido pela família, recursivamente, é snake_case minúsculo.
func TestFamiliaGrupo_ContratoPublico_NomesCanonicos(t *testing.T) {
	for _, tc := range familiaCasos() {
		t.Run(tc.nome, func(t *testing.T) {
			rec := serveFamilia(t, tc)
			contracttest.AssertPublicJSONUsesCanonicalNaming(t, rec.Body.Bytes())
		})
	}
}

// TestFamiliaGrupo_ContratoPublico_ChavesAntigasSumiram: e as chaves de antes
// desapareceram. Sem isto, um struct que carregasse as duas passaria acima.
func TestFamiliaGrupo_ContratoPublico_ChavesAntigasSumiram(t *testing.T) {
	for _, tc := range familiaCasos() {
		t.Run(tc.nome, func(t *testing.T) {
			rec := serveFamilia(t, tc)
			contracttest.AssertNoKeys(t, rec.Body.Bytes(), chavesAntigasDaFamilia...)
		})
	}
}

// dadosDaFamilia devolve o `data` do envelope canónico, já descodificado.
func dadosDaFamilia(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope struct {
		Success bool           `json:"success"`
		Code    int            `json:"code"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("corpo não é o envelope canónico: %v\n%s", err, rec.Body.String())
	}
	if !envelope.Success || envelope.Code != 200 {
		t.Fatalf("envelope = %+v, quero success=true code=200", envelope)
	}
	return envelope.Data
}

// caso devolve o caso com este nome, para os testes que afirmam VALORES.
func caso(t *testing.T, nome string) familiaCase {
	t.Helper()
	for _, tc := range familiaCasos() {
		if tc.nome == nome {
			return tc
		}
	}
	t.Fatalf("caso %q não existe na tabela", nome)
	return familiaCase{}
}

// TestFamiliaGrupo_ValoresMapeados prova que o apresentador pôs os valores
// certos nas chaves certas. Uma troca entre dois campos vizinhos passaria em
// todos os testes de NOME acima.
func TestFamiliaGrupo_ValoresMapeados(t *testing.T) {
	t.Run("invite_link", func(t *testing.T) {
		data := dadosDaFamilia(t, serveFamilia(t, caso(t, "GetGroupInviteLink")))
		if data["invite_link"] != "https://chat.whatsapp.com/ABC" {
			t.Errorf("invite_link = %#v", data["invite_link"])
		}
	})

	t.Run("group_list", func(t *testing.T) {
		data := dadosDaFamilia(t, serveFamilia(t, caso(t, "ListGroups")))
		grupos, ok := data["groups"].([]any)
		if !ok || len(grupos) != 1 {
			t.Fatalf("groups = %#v, quero 1 elemento", data["groups"])
		}
		g, _ := grupos[0].(map[string]any)
		if g["jid"] != "120363000000000000@g.us" || g["name"] != "Equipa" {
			t.Errorf("groups[0] = %#v", g)
		}
		// O grupo de referência é o MESMO que /group/info serve, então a lista
		// tem de carregar a árvore inteira, participantes incluídos.
		partes, ok := g["participants"].([]any)
		if !ok || len(partes) != 2 {
			t.Fatalf("groups[0].participants = %#v", g["participants"])
		}
		p0, _ := partes[0].(map[string]any)
		if p0["is_super_admin"] != true || p0["phone_number"] != "5511999999999@s.whatsapp.net" {
			t.Errorf("groups[0].participants[0] = %#v", p0)
		}
	})

	t.Run("invite_info", func(t *testing.T) {
		data := dadosDaFamilia(t, serveFamilia(t, caso(t, "GetGroupInviteInfo")))
		info, ok := data["invite_info"].(map[string]any)
		if !ok {
			t.Fatalf("invite_info = %#v", data["invite_info"])
		}
		if info["jid"] != "120363000000000000@g.us" || info["is_join_approval_required"] != true {
			t.Errorf("invite_info = %#v", info)
		}
	})

	t.Run("create_group", func(t *testing.T) {
		data := dadosDaFamilia(t, serveFamilia(t, caso(t, "CreateGroup")))
		if data["created"] != true {
			t.Errorf("created = %#v, quero true", data["created"])
		}
		info, ok := data["group_info"].(map[string]any)
		if !ok || info["name"] != "Equipa" {
			t.Errorf("group_info = %#v", data["group_info"])
		}
	})

	t.Run("update_participants", func(t *testing.T) {
		data := dadosDaFamilia(t, serveFamilia(t, caso(t, "UpdateGroupParticipants")))
		if data["confirmed"] != true || data["reason"] != "" {
			t.Errorf("confirmed=%#v reason=%#v", data["confirmed"], data["reason"])
		}
		partes, ok := data["participants"].([]any)
		if !ok || len(partes) != 1 {
			t.Fatalf("participants = %#v", data["participants"])
		}
		p, _ := partes[0].(map[string]any)
		// É o MESMO tipo de participante que /group/info serve: uma segunda
		// forma paralela obrigaria o cliente a aprender duas.
		if p["is_admin"] != true || p["lid"] != "111111111111111@lid" ||
			p["phone_number"] != "5511999999999@s.whatsapp.net" {
			t.Errorf("participants[0] = %#v", p)
		}
	})

	t.Run("join_requests", func(t *testing.T) {
		data := dadosDaFamilia(t, serveFamilia(t, caso(t, "GetGroupRequestParticipants")))
		pedidos, ok := data["requests"].([]any)
		if !ok || len(pedidos) != 1 {
			t.Fatalf("requests = %#v", data["requests"])
		}
		r, _ := pedidos[0].(map[string]any)
		if r["jid"] != "5511999999999@s.whatsapp.net" || r["method"] != "InviteLink" ||
			r["added_by_jid"] != "5511888888888@s.whatsapp.net" ||
			r["requested_at"] != "2024-03-01T12:00:00Z" {
			t.Errorf("requests[0] = %#v", r)
		}
	})

	t.Run("community_subgroups", func(t *testing.T) {
		data := dadosDaFamilia(t, serveFamilia(t, caso(t, "GetCommunitySubGroups")))
		subs, ok := data["sub_groups"].([]any)
		if !ok || len(subs) != 1 {
			t.Fatalf("sub_groups = %#v", data["sub_groups"])
		}
		s, _ := subs[0].(map[string]any)
		if s["jid"] != "120363111111111111@g.us" || s["name"] != "Avisos" ||
			s["is_default_sub_group"] != true {
			t.Errorf("sub_groups[0] = %#v", s)
		}
	})

	t.Run("community_participants", func(t *testing.T) {
		data := dadosDaFamilia(t, serveFamilia(t, caso(t, "GetCommunityParticipants")))
		ps, ok := data["participants"].([]any)
		if !ok || len(ps) != 1 || ps[0] != "5511999999999@s.whatsapp.net" {
			t.Errorf("participants = %#v", data["participants"])
		}
	})

	t.Run("details_das_escritas", func(t *testing.T) {
		// As dez escritas que só confirmam respondem `details`, minúsculo — era
		// `Details` em todas elas, escrito à mão num map.
		for _, nome := range []string{
			"GroupJoin", "GroupLeave", "SetGroupName", "SetGroupTopic", "SetGroupPhoto",
			"RemoveGroupPhoto", "SetGroupAnnounce", "SetGroupLocked", "SetDisappearingTimer",
			"UpdateGroupRequestParticipants", "SetGroupJoinApprovalMode",
			"CommunityLink", "CommunityUnlink",
		} {
			data := dadosDaFamilia(t, serveFamilia(t, caso(t, nome)))
			texto, ok := data["details"].(string)
			if !ok || texto == "" {
				t.Errorf("%s: data.details = %#v", nome, data["details"])
			}
		}
	})
}

// TestFamiliaGrupo_VazioEZero: colecção vazia é `[]` e nunca `null`, e tempo
// desconhecido é `null` e nunca "0001-01-01T00:00:00Z" — essa string é uma data
// real no fio, e um cliente que a analise recebe o ano 1 em vez de um campo
// que o motor não conhece.
func TestFamiliaGrupo_VazioEZero(t *testing.T) {
	t.Run("listas vazias saem como []", func(t *testing.T) {
		casos := map[string]familiaCase{
			"groups":       caso(t, "ListGroups"),
			"requests":     caso(t, "GetGroupRequestParticipants"),
			"sub_groups":   caso(t, "GetCommunitySubGroups"),
			"participants": caso(t, "GetCommunityParticipants"),
		}
		for chave, tc := range casos {
			tc.arrange = nil // dublê no zero-value: nada devolvido pela porta
			data := dadosDaFamilia(t, serveFamilia(t, tc))
			lista, ok := data[chave].([]any)
			if !ok || len(lista) != 0 {
				t.Errorf("%s = %#v, quero []", chave, data[chave])
			}
		}
	})

	t.Run("requested_at zero sai como null", func(t *testing.T) {
		tc := caso(t, "GetGroupRequestParticipants")
		tc.arrange = func(f *grpFakes, _ *grpMgmtFakes, _ *comFakes) {
			f.requests.GetRequestParticipantsFunc = func(context.Context, string, domain.JID) ([]domain.GroupJoinRequest, error) {
				// O motor headless não reporta o momento em todo registo.
				return []domain.GroupJoinRequest{{JID: "5511999999999@s.whatsapp.net"}}, nil
			}
		}
		data := dadosDaFamilia(t, serveFamilia(t, tc))
		pedidos, _ := data["requests"].([]any)
		if len(pedidos) != 1 {
			t.Fatalf("requests = %#v", data["requests"])
		}
		r, _ := pedidos[0].(map[string]any)
		valor, presente := r["requested_at"]
		if !presente {
			t.Fatal("requested_at ausente: a chave tem de existir mesmo sem valor")
		}
		if valor != nil {
			t.Errorf("requested_at = %#v, quero null para tempo zero", valor)
		}
	})

	t.Run("participants vazio na atualização sai como []", func(t *testing.T) {
		tc := caso(t, "UpdateGroupParticipants")
		tc.arrange = func(_ *grpFakes, m *grpMgmtFakes, _ *comFakes) {
			// É o desfecho do motor headless: mudou, e não conseguiu confirmar.
			m.settings.UpdateGroupParticipantsFunc = func(context.Context, string, domain.JID, []domain.JID, domain.ParticipantAction) (domain.ParticipantsUpdate, error) {
				return domain.ParticipantsUpdate{Confirmed: false, Reason: "não observável nesta sessão"}, nil
			}
		}
		data := dadosDaFamilia(t, serveFamilia(t, tc))
		lista, ok := data["participants"].([]any)
		if !ok || len(lista) != 0 {
			t.Errorf("participants = %#v, quero []", data["participants"])
		}
		if data["confirmed"] != false || data["reason"] == "" {
			t.Errorf("confirmed=%#v reason=%#v: a incerteza tem de chegar declarada",
				data["confirmed"], data["reason"])
		}
	})
}
