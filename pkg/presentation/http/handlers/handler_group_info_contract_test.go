package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/usecase/group"
	"wa-api/pkg/domain"
	customhttp "wa-api/pkg/presentation/http"
	"wa-api/pkg/presentation/http/contracttest"

	"github.com/gorilla/mux"
)

// Este ficheiro é a REFERÊNCIA do padrão de teste de contrato para as seis
// famílias que vão migrar para DTO. O que ele afirma, e que um teste do
// handler cru NÃO afirma:
//
//  1. o pedido passa pela ROTA REGISTADA, no mesmo mux/gorilla que a produção
//     usa. Handler montado à mão não exercita nem o método, nem o padrão de
//     caminho, nem a extração de parâmetros (ARMADILHAS #2);
//  2. TODA chave do corpo, recursivamente, é snake_case minúsculo — afirmado
//     pelo helper partilhado, não por uma lista escrita à mão que envelhece;
//  3. as chaves ANTIGAS desapareceram. "a chave nova existe" não prova
//     migração: um struct pode carregar as duas.

// contractUserID é o id de sessão que withContractUser injecta. Constante
// porque os dublês das outras famílias precisam de semear as suas lojas sob
// exactamente este id — semear sob outro devolve o registo vazio, e o teste
// passa a medir o vazio (ADR-0004).
const contractUserID = "u1"

// contractUser satisfaz a interface userInfo que sessionUser lê do contexto.
type contractUser struct{ id string }

func (u contractUser) Get(key string) string {
	if key == "Id" {
		return u.id
	}
	return ""
}

// withContractUser injeta a sessão autenticada que o middleware de auth
// injectaria em produção — sem ela todo o pedido morre em 401 e o teste
// mediria a guarda, não o contrato (ARMADILHAS #2: teste o caminho de
// SUCESSO).
func withContractUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), appport.UserInfoKey, contractUser{id: contractUserID})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// groupInfoRouter monta a rota EXACTAMENTE como wiring_routes.go a monta:
// registry.Register("/group/info", ..., "POST") aplicado a um mux.Router.
func groupInfoRouter(t *testing.T, f *grpFakes) *mux.Router {
	t.Helper()
	registry := customhttp.NewHandlerRegistry()
	registry.Register("/group/info",
		withContractUser(NewGetGroupInfoHandler(group.NewGetGroupInfoUseCase(f.directory, f.jids, f.logger))),
		http.MethodPost)
	router := mux.NewRouter()
	registry.Apply(router)
	return router
}

// grupoDeReferencia é o domain.GroupInfo que o adaptador wa-noise produz para
// um grupo real: dono, nome e tópico datados, dois participantes, um deles
// administrador. Todos os campos preenchidos de propósito — um dublê com
// metade dos campos no zero deixaria metade do mapeamento por medir.
func grupoDeReferencia() *domain.GroupInfo {
	criado := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)
	return &domain.GroupInfo{
		JID:                    "120363000000000000@g.us",
		OwnerJID:               "5511999999999@s.whatsapp.net",
		Name:                   "Equipa",
		NameSetAt:              criado.Add(time.Hour),
		NameSetBy:              "5511999999999@s.whatsapp.net",
		Topic:                  "Assuntos da equipa",
		TopicSetAt:             criado.Add(2 * time.Hour),
		TopicSetBy:             "5511999999999@s.whatsapp.net",
		IsLocked:               true,
		IsAnnounce:             false,
		IsEphemeral:            true,
		DisappearingTimer:      86400,
		IsIncognito:            false,
		IsSuspended:            false,
		IsParent:               false,
		LinkedParentJID:        "120363111111111111@g.us",
		IsDefaultSubGroup:      false,
		IsJoinApprovalRequired: true,
		MemberAddMode:          "admin_add",
		CreatedAt:              criado,
		ParticipantCount:       2,
		Participants: []domain.GroupParticipant{
			{
				JID:         "5511999999999@s.whatsapp.net",
				PhoneNumber: "5511999999999@s.whatsapp.net",
				LID:         "111111111111111@lid",
				IsAdmin:     true, IsSuperAdmin: true,
			},
			{
				JID:         "222222222222222@lid",
				PhoneNumber: "5511888888888@s.whatsapp.net",
				LID:         "222222222222222@lid",
				DisplayName: "Anónimo",
			},
		},
	}
}

// TestGetGroupInfo_ContratoPublico_NomesCanonicos é a afirmação (1)+(2): o
// corpo servido pela rota registada usa snake_case minúsculo em toda a
// árvore, incluindo dentro do array de participantes.
func TestGetGroupInfo_ContratoPublico_NomesCanonicos(t *testing.T) {
	f := newGrpFakes()
	f.directory.GetGroupInfoFunc = func(context.Context, string, domain.JID) (*domain.GroupInfo, error) {
		return grupoDeReferencia(), nil
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/group/info",
		strings.NewReader(`{"group_jid":"120363000000000000@g.us"}`))
	groupInfoRouter(t, f).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200; corpo: %s", rec.Code, rec.Body.String())
	}
	contracttest.AssertPublicJSONUsesCanonicalNaming(t, rec.Body.Bytes())
}

// TestGetGroupInfo_ContratoPublico_ChavesAntigasSumiram é a afirmação (3).
//
// `JID`, `Name`, `Participants`, `IsAdmin` e companhia eram o que o struct de
// protocolo do wa-noise emitia quando era serializado directamente — sem tags
// `json`, o codificador usa o nome do campo Go. São essas as chaves que a
// migração tinha de FAZER DESAPARECER, e "a chave nova existe" não o prova.
func TestGetGroupInfo_ContratoPublico_ChavesAntigasSumiram(t *testing.T) {
	f := newGrpFakes()
	f.directory.GetGroupInfoFunc = func(context.Context, string, domain.JID) (*domain.GroupInfo, error) {
		return grupoDeReferencia(), nil
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/group/info",
		strings.NewReader(`{"group_jid":"120363000000000000@g.us"}`))
	groupInfoRouter(t, f).ServeHTTP(rec, req)

	contracttest.AssertNoKeys(t, rec.Body.Bytes(),
		"JID", "OwnerJID", "Name", "Topic", "Participants", "ParticipantCount",
		"IsAdmin", "IsSuperAdmin", "PhoneNumber", "LID", "GroupName", "GroupTopic",
		"groupJID", "GroupJID",
	)
}

// TestGetGroupInfo_ContratoPublico_ValoresMapeados prova que o apresentador
// não só produziu as chaves certas como pôs os VALORES certos nelas — uma
// troca entre dois booleanos vizinhos passaria em todos os testes acima.
func TestGetGroupInfo_ContratoPublico_ValoresMapeados(t *testing.T) {
	f := newGrpFakes()
	f.directory.GetGroupInfoFunc = func(context.Context, string, domain.JID) (*domain.GroupInfo, error) {
		return grupoDeReferencia(), nil
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/group/info",
		strings.NewReader(`{"group_jid":"120363000000000000@g.us"}`))
	groupInfoRouter(t, f).ServeHTTP(rec, req)

	var envelope struct {
		Success bool `json:"success"`
		Code    int  `json:"code"`
		Data    struct {
			GroupInfo map[string]any `json:"group_info"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("corpo não é o envelope canónico: %v\n%s", err, rec.Body.String())
	}
	if !envelope.Success || envelope.Code != 200 {
		t.Fatalf("envelope = %+v, quero success=true code=200", envelope)
	}
	gi := envelope.Data.GroupInfo
	if gi == nil {
		t.Fatal("data.group_info ausente")
	}

	quero := map[string]any{
		"jid":                       "120363000000000000@g.us",
		"owner_jid":                 "5511999999999@s.whatsapp.net",
		"name":                      "Equipa",
		"topic":                     "Assuntos da equipa",
		"is_locked":                 true,
		"is_announce":               false,
		"is_ephemeral":              true,
		"disappearing_timer":        float64(86400),
		"is_join_approval_required": true,
		"linked_parent_jid":         "120363111111111111@g.us",
		"member_add_mode":           "admin_add",
		"participant_count":         float64(2),
		"created_at":                "2024-03-01T12:00:00Z",
	}
	for chave, esperado := range quero {
		if got := gi[chave]; got != esperado {
			t.Errorf("group_info.%s = %#v, quero %#v", chave, got, esperado)
		}
	}

	participantes, ok := gi["participants"].([]any)
	if !ok || len(participantes) != 2 {
		t.Fatalf("participants = %#v, quero 2 elementos", gi["participants"])
	}
	primeiro, _ := participantes[0].(map[string]any)
	if primeiro["is_admin"] != true || primeiro["is_super_admin"] != true {
		t.Errorf("participants[0] = %#v, quero is_admin e is_super_admin verdadeiros", primeiro)
	}
	if primeiro["lid"] != "111111111111111@lid" {
		t.Errorf("participants[0].lid = %#v", primeiro["lid"])
	}
	segundo, _ := participantes[1].(map[string]any)
	if segundo["display_name"] != "Anónimo" || segundo["is_admin"] != false {
		t.Errorf("participants[1] = %#v", segundo)
	}
}

// TestGetGroupInfo_PedidoAceitaGroupJIDSnakeCase é a afirmação de que o
// PEDIDO de /group/info também usa snake_case — H-DTO-GROUP-INFO: esta era a
// única rota da família que ainda aceitava `groupJID` no corpo, decodificando
// direto em domain.GetGroupInfoRequest em vez de passar por um DTO de pedido.
func TestGetGroupInfo_PedidoAceitaGroupJIDSnakeCase(t *testing.T) {
	f := newGrpFakes()
	f.directory.GetGroupInfoFunc = func(context.Context, string, domain.JID) (*domain.GroupInfo, error) {
		return grupoDeReferencia(), nil
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/group/info",
		strings.NewReader(`{"group_jid":"120363000000000000@g.us"}`))
	groupInfoRouter(t, f).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200 com group_jid; corpo: %s", rec.Code, rec.Body.String())
	}
}

// TestGetGroupInfo_ChaveAntigaGroupJIDCamelSumiu prova que `groupJID`, sozinha,
// já não resolve o pedido: cai na validação de campo obrigatório do use case,
// e não é mais aceita como sinônimo silencioso de group_jid.
func TestGetGroupInfo_ChaveAntigaGroupJIDCamelSumiu(t *testing.T) {
	f := newGrpFakes()
	f.directory.GetGroupInfoFunc = func(context.Context, string, domain.JID) (*domain.GroupInfo, error) {
		return grupoDeReferencia(), nil
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/group/info",
		strings.NewReader(`{"groupJID":"120363000000000000@g.us"}`))
	groupInfoRouter(t, f).ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("groupJID sozinho ainda resolveu o grupo: status 200, corpo: %s", rec.Body.String())
	}
}

// TestGetGroupInfo_ContratoPublico_ZeroNaoViraDataFalsa trava a decisão do
// apresentador sobre tempo ausente: null, e não "0001-01-01T00:00:00Z".
//
// É o motor headless que produz este caso — ele vê o jid e o título de uma
// conversa, e mais nada — então não é hipotético.
func TestGetGroupInfo_ContratoPublico_ZeroNaoViraDataFalsa(t *testing.T) {
	f := newGrpFakes()
	f.directory.GetGroupInfoFunc = func(context.Context, string, domain.JID) (*domain.GroupInfo, error) {
		return &domain.GroupInfo{
			JID:          "120363000000000000@g.us",
			Name:         "Equipa",
			Participants: []domain.GroupParticipant{},
		}, nil
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/group/info",
		strings.NewReader(`{"group_jid":"120363000000000000@g.us"}`))
	groupInfoRouter(t, f).ServeHTTP(rec, req)

	var envelope struct {
		Data struct {
			GroupInfo map[string]any `json:"group_info"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("corpo inválido: %v", err)
	}
	for _, chave := range []string{"created_at", "name_set_at", "topic_set_at"} {
		valor, presente := envelope.Data.GroupInfo[chave]
		if !presente {
			t.Errorf("%s ausente: a chave tem de existir mesmo sem valor, senão o cliente não distingue ausente de desconhecido", chave)
		}
		if valor != nil {
			t.Errorf("%s = %#v, quero null para tempo zero", chave, valor)
		}
	}
	// A lista vazia é `[]` e nunca `null`: só uma das duas se percorre sem
	// verificação.
	if lista, ok := envelope.Data.GroupInfo["participants"].([]any); !ok || len(lista) != 0 {
		t.Errorf("participants = %#v, quero []", envelope.Data.GroupInfo["participants"])
	}
}
