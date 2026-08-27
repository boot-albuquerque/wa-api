package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/domain"
	customhttp "wa-api/pkg/presentation/http"
	"wa-api/pkg/presentation/http/contracttest"
)

// Testes de contrato público da família de utilizadores, contactos e
// blocklist — o padrão de handler_group_info_contract_test.go aplicado às 12
// rotas migradas.
//
// O que estes testes afirmam, e que um teste do handler cru NÃO afirma:
//
//  1. o pedido passa pela ROTA REGISTADA, no mesmo mux/gorilla da produção;
//  2. TODA chave do corpo, recursivamente, é snake_case minúsculo — afirmado
//     pelo helper partilhado, não por uma lista que envelhece;
//  3. as chaves ANTIGAS desapareceram. "a chave nova existe" não prova
//     migração: um struct pode carregar as duas;
//  4. os VALORES foram para as chaves certas — uma troca entre dois booleanos
//     vizinhos passa em tudo o resto;
//  5. tempo zero é null e colecção vazia é `[]`.

// userFamilyRouter monta uma rota EXACTAMENTE como wiring_routes.go a monta.
func userFamilyRouter(t *testing.T, path string, h http.Handler, methods ...string) *mux.Router {
	t.Helper()
	registry := customhttp.NewHandlerRegistry()
	registry.Register(path, withContractUser(h), methods...)
	router := mux.NewRouter()
	registry.Apply(router)
	return router
}

// serveUserRoute dispara um pedido pela rota registada e devolve a resposta.
func serveUserRoute(t *testing.T, path string, h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	userFamilyRouter(t, path, h, method).ServeHTTP(rec, req)
	return rec
}

// dataOf decodifica o envelope e devolve `data` como mapa, falhando quando o
// envelope não é o canónico ou o estado não é 200.
func dataOf(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200; corpo: %s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Success bool           `json:"success"`
		Code    int            `json:"code"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("corpo não é o envelope canónico: %v\n%s", err, rec.Body.String())
	}
	if !envelope.Success || envelope.Code != 200 {
		t.Fatalf("envelope = success=%v code=%d, quero true/200", envelope.Success, envelope.Code)
	}
	contracttest.AssertPublicJSONUsesCanonicalNaming(t, rec.Body.Bytes())
	return envelope.Data
}

// --- POST /user/check ------------------------------------------------------

func TestCheckUser_ContratoPublico(t *testing.T) {
	f := uhNewFakes()
	f.contacts.IsOnWhatsAppFunc = func(context.Context, string, []string) ([]domain.WhatsAppCheck, error) {
		return []domain.WhatsAppCheck{
			{Query: "5511999999999", IsIn: true, JID: "5511999999999@s.whatsapp.net", VerifiedName: "Loja"},
			{Query: "5511888888888", IsIn: false},
		}, nil
	}

	rec := serveUserRoute(t, "/user/check", f.handlers().CheckUser(),
		http.MethodPost, "/user/check", `{"phone":["5511999999999","5511888888888"]}`)
	data := dataOf(t, rec)

	contracttest.AssertNoKeys(t, rec.Body.Bytes(),
		"Query", "IsIn", "IsInWhatsapp", "JID", "VerifiedName", "Phone")

	users, ok := data["users"].([]any)
	if !ok || len(users) != 2 {
		t.Fatalf("users = %#v, quero 2 elementos", data["users"])
	}
	primeiro, _ := users[0].(map[string]any)
	quero := map[string]any{
		"query":          "5511999999999",
		"is_in_whatsapp": true,
		"jid":            "5511999999999@s.whatsapp.net",
		"verified_name":  "Loja",
	}
	for chave, esperado := range quero {
		if got := primeiro[chave]; got != esperado {
			t.Errorf("users[0].%s = %#v, quero %#v", chave, got, esperado)
		}
	}
	// O segundo é o controlo do booleano: uma troca entre is_in_whatsapp e
	// qualquer vizinho passaria se só o primeiro fosse verificado.
	segundo, _ := users[1].(map[string]any)
	if segundo["is_in_whatsapp"] != false || segundo["verified_name"] != "" {
		t.Errorf("users[1] = %#v", segundo)
	}
}

// TestCheckUser_ListaVaziaNaoViraNull: `[]` e `null` são valores diferentes
// para todo cliente, e só um dos dois se percorre sem verificação.
func TestCheckUser_ListaVaziaNaoViraNull(t *testing.T) {
	f := uhNewFakes()
	f.contacts.IsOnWhatsAppFunc = func(context.Context, string, []string) ([]domain.WhatsAppCheck, error) {
		return nil, nil
	}

	rec := serveUserRoute(t, "/user/check", f.handlers().CheckUser(),
		http.MethodPost, "/user/check", `{"phone":["5511999999999"]}`)
	data := dataOf(t, rec)

	users, ok := data["users"].([]any)
	if !ok || len(users) != 0 {
		t.Fatalf("users = %#v, quero []", data["users"])
	}
}

// TestCheckUser_PedidoSemTelefoneERecusadoNaFronteira: o DTO de pedido valida
// antes de a rota falar com o WhatsApp.
func TestCheckUser_PedidoSemTelefoneERecusadoNaFronteira(t *testing.T) {
	f := uhNewFakes()

	rec := serveUserRoute(t, "/user/check", f.handlers().CheckUser(),
		http.MethodPost, "/user/check", `{"phone":[]}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quero 400; corpo: %s", rec.Code, rec.Body.String())
	}
	if len(f.contacts.IsOnWhatsAppCalls) != 0 {
		t.Error("a recusa de fronteira chegou à porta")
	}
	assertErrorCode(t, rec, "missing_phone")
}

// --- POST /user/info -------------------------------------------------------

func TestGetUserInfo_ContratoPublico(t *testing.T) {
	f := chNewFakes()
	f.contacts.GetUserInfoFunc = func(context.Context, string, []domain.JID) ([]domain.UserInfo, error) {
		return []domain.UserInfo{{
			JID:          "5511999999999@s.whatsapp.net",
			LID:          "111111111111111@lid",
			Status:       "disponível",
			PictureID:    "pic-1",
			VerifiedName: "Loja",
			Devices:      []domain.JID{"5511999999999.0:1@s.whatsapp.net"},
			PushName:     "Ana",
			BusinessName: "Loja da Ana",
		}}, nil
	}

	rec := serveUserRoute(t, "/user/info", f.userInfoHandler(),
		http.MethodPost, "/user/info", `{"phone":["5511999999999"]}`)
	data := dataOf(t, rec)

	// Estas eram as chaves que o types.UserInfo do SDK emitia quando o mapa
	// ia direto ao fio: sem tags `json`, o codificador usa o nome do campo Go.
	contracttest.AssertNoKeys(t, rec.Body.Bytes(),
		"VerifiedName", "Status", "PictureID", "Devices", "LID", "JID", "PushName", "BusinessName")

	users, ok := data["users"].([]any)
	if !ok || len(users) != 1 {
		t.Fatalf("users = %#v, quero 1 elemento", data["users"])
	}
	u, _ := users[0].(map[string]any)
	quero := map[string]any{
		"jid":           "5511999999999@s.whatsapp.net",
		"lid":           "111111111111111@lid",
		"status":        "disponível",
		"picture_id":    "pic-1",
		"verified_name": "Loja",
		"push_name":     "Ana",
		"business_name": "Loja da Ana",
	}
	for chave, esperado := range quero {
		if got := u[chave]; got != esperado {
			t.Errorf("users[0].%s = %#v, quero %#v", chave, got, esperado)
		}
	}
	devices, ok := u["devices"].([]any)
	if !ok || len(devices) != 1 || devices[0] != "5511999999999.0:1@s.whatsapp.net" {
		t.Errorf("users[0].devices = %#v", u["devices"])
	}
}

func TestGetUserInfo_SemDispositivosServeListaVazia(t *testing.T) {
	f := chNewFakes()
	f.contacts.GetUserInfoFunc = func(context.Context, string, []domain.JID) ([]domain.UserInfo, error) {
		return []domain.UserInfo{{JID: "5511999999999@s.whatsapp.net"}}, nil
	}

	rec := serveUserRoute(t, "/user/info", f.userInfoHandler(),
		http.MethodPost, "/user/info", `{"phone":["5511999999999"]}`)
	data := dataOf(t, rec)

	u := data["users"].([]any)[0].(map[string]any)
	if devices, ok := u["devices"].([]any); !ok || len(devices) != 0 {
		t.Errorf("devices = %#v, quero []", u["devices"])
	}
}

// --- GET /user/lid/{jid} ---------------------------------------------------

func TestGetUserLID_ContratoPublico(t *testing.T) {
	f := uhNewFakes()
	f.jids.ResolveQualifiedJIDFunc = func(_ context.Context, raw string) (domain.JID, error) {
		return domain.JID(raw), nil
	}

	rec := serveUserRoute(t, "/user/lid/{jid}", f.handlers().GetUserLID(),
		http.MethodGet, "/user/lid/5511999999999@s.whatsapp.net", "")
	data := dataOf(t, rec)

	contracttest.AssertNoKeys(t, rec.Body.Bytes(), "JID", "LID")
	if data["jid"] != "5511999999999@s.whatsapp.net" || data["lid"] != "5511999@lid" {
		t.Errorf("data = %#v", data)
	}
}

// --- GET /user/profile/{jid} -----------------------------------------------

func TestGetUserProfile_ContratoPublico(t *testing.T) {
	f := uhNewFakes()
	f.jids.ResolveJIDFunc = func(_ context.Context, raw string) (domain.JID, error) {
		if strings.Contains(raw, "@") {
			return domain.JID(raw), nil
		}
		return domain.JID(raw + "@s.whatsapp.net"), nil
	}
	f.contacts.IsOnWhatsAppFunc = func(context.Context, string, []string) ([]domain.WhatsAppCheck, error) {
		return []domain.WhatsAppCheck{{IsIn: true, VerifiedName: "Loja"}}, nil
	}
	f.contacts.GetUserInfoFunc = func(_ context.Context, _ string, jids []domain.JID) ([]domain.UserInfo, error) {
		return []domain.UserInfo{{JID: jids[0], Status: "disponível"}}, nil
	}
	f.contacts.GetProfilePictureFunc = func(context.Context, string, domain.JID, bool) (*domain.AvatarInfo, error) {
		return &domain.AvatarInfo{ID: "pic-1", URL: "https://example.com/pic-1"}, nil
	}

	rec := serveUserRoute(t, "/user/profile/{jid}", f.handlers().GetUserProfile(),
		http.MethodGet, "/user/profile/5511999999999", "")
	data := dataOf(t, rec)

	contracttest.AssertNoKeys(t, rec.Body.Bytes(),
		"JID", "LID", "Query", "OnWhatsApp", "VerifiedName", "AvatarURL", "AvatarID",
		"UserInfo", "Unavailable")

	quero := map[string]any{
		"jid":           "5511999999999@s.whatsapp.net",
		"lid":           "5511999@lid",
		"query":         "5511999999999",
		"on_whatsapp":   true,
		"verified_name": "Loja",
		"avatar_url":    "https://example.com/pic-1",
		"avatar_id":     "pic-1",
	}
	for chave, esperado := range quero {
		if got := data[chave]; got != esperado {
			t.Errorf("%s = %#v, quero %#v", chave, got, esperado)
		}
	}
	// Nada falhou: `unavailable` é `{}` e não null — um cliente que percorra
	// os motivos não deve ter de verificar null num mapa cuja ausência de
	// entradas já diz "nada falhou".
	indisponivel, presente := data["unavailable"]
	if !presente {
		t.Fatal("unavailable ausente")
	}
	if m, ok := indisponivel.(map[string]any); !ok || len(m) != 0 {
		t.Errorf("unavailable = %#v, quero {}", indisponivel)
	}
	if info, ok := data["user_info"].([]any); !ok || len(info) != 1 {
		t.Errorf("user_info = %#v, quero 1 elemento", data["user_info"])
	}
}

// TestGetUserProfile_OnWhatsAppNuloNaoViraFalso trava a decisão do ponteiro:
// "não pude perguntar" e "perguntei e não tem conta" são respostas diferentes,
// e um bool simples fundiria as duas em `false`.
func TestGetUserProfile_OnWhatsAppNuloNaoViraFalso(t *testing.T) {
	f := uhNewFakes()
	f.jids.ResolveJIDFunc = func(_ context.Context, raw string) (domain.JID, error) {
		return domain.JID(raw), nil
	}
	f.contacts.GetPNForLIDFunc = func(context.Context, string, domain.JID) (domain.JID, error) {
		return "", nil // sem mapeamento: a consulta de presença não pode ser feita
	}

	rec := serveUserRoute(t, "/user/profile/{jid}", f.handlers().GetUserProfile(),
		http.MethodGet, "/user/profile/111111111111111@lid", "")
	data := dataOf(t, rec)

	valor, presente := data["on_whatsapp"]
	if !presente {
		t.Fatal("on_whatsapp ausente: a chave tem de existir mesmo sem valor")
	}
	if valor != nil {
		t.Errorf("on_whatsapp = %#v, quero null", valor)
	}
	m, _ := data["unavailable"].(map[string]any)
	if _, ok := m["on_whatsapp"]; !ok {
		t.Errorf("unavailable = %#v, quero o motivo de on_whatsapp", data["unavailable"])
	}
	// Colecção vazia é `[]`, nunca null.
	if info, ok := data["user_info"].([]any); !ok || len(info) != 0 {
		t.Errorf("user_info = %#v, quero []", data["user_info"])
	}
}

// --- POST /user/block e POST /user/unblock ---------------------------------

func TestBlockUnblock_ContratoPublico(t *testing.T) {
	casos := []struct {
		nome    string
		path    string
		build   func(*uhFakes) http.Handler
		detalhe string
	}{
		{"block", "/user/block", func(f *uhFakes) http.Handler { return f.handlers().BlockUser() }, "User blocked"},
		{"unblock", "/user/unblock", func(f *uhFakes) http.Handler { return f.handlers().UnblockUser() }, "User unblocked"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			f := uhNewFakes()
			f.jids.ResolveJIDFunc = func(_ context.Context, raw string) (domain.JID, error) {
				return domain.JID(raw), nil
			}
			f.block.UpdateBlocklistFunc = func(context.Context, string, domain.JID, bool) (domain.BlocklistUpdate, error) {
				return domain.BlocklistUpdate{
					ResolvedJID:  "5511999999999@s.whatsapp.net",
					RequestedJID: "111111111111111@lid",
					Entries:      []string{"5511999999999@s.whatsapp.net"},
					DHash:        "h1",
				}, nil
			}

			rec := serveUserRoute(t, c.path, c.build(f), http.MethodPost, c.path,
				`{"jid":"111111111111111@lid"}`)
			data := dataOf(t, rec)

			contracttest.AssertNoKeys(t, rec.Body.Bytes(),
				"Details", "JID", "Blocklist", "DHash", "RequestedJID")

			if data["details"] != c.detalhe {
				t.Errorf("details = %#v, quero %q", data["details"], c.detalhe)
			}
			// A distinção entre pedido e efectivo é o valor do par: trocá-los
			// passaria em qualquer teste que só olhasse as chaves.
			if data["jid"] != "5511999999999@s.whatsapp.net" {
				t.Errorf("jid = %#v, quero o JID EFECTIVO", data["jid"])
			}
			if data["requested_jid"] != "111111111111111@lid" {
				t.Errorf("requested_jid = %#v, quero o JID PEDIDO", data["requested_jid"])
			}
			if data["dhash"] != "h1" {
				t.Errorf("dhash = %#v", data["dhash"])
			}
			lista, ok := data["blocklist"].([]any)
			if !ok || len(lista) != 1 {
				t.Errorf("blocklist = %#v", data["blocklist"])
			}
		})
	}
}

// TestBlock_SemAlvoERecusadoNaFronteira: o DTO de pedido valida antes de a
// rota falar com o WhatsApp.
func TestBlock_SemAlvoERecusadoNaFronteira(t *testing.T) {
	f := uhNewFakes()

	rec := serveUserRoute(t, "/user/block", f.handlers().BlockUser(),
		http.MethodPost, "/user/block", `{}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quero 400; corpo: %s", rec.Code, rec.Body.String())
	}
	if len(f.block.UpdateBlocklistCalls) != 0 {
		t.Error("a recusa de fronteira chegou à porta")
	}
	assertErrorCode(t, rec, "missing_phone_or_jid")
}

// TestBlock_AliasChatContinuaAResolver: o alias `chat` é lido pelo DTO de
// pedido, e não pelo tipo de domínio, desde a migração. Sem ResolveChat no
// DTO, todo cliente que usa o alias passaria a receber 400.
func TestBlock_AliasChatContinuaAResolver(t *testing.T) {
	f := uhNewFakes()
	f.jids.ResolveJIDFunc = func(_ context.Context, raw string) (domain.JID, error) {
		return domain.JID(raw), nil
	}
	f.block.UpdateBlocklistFunc = func(_ context.Context, _ string, target domain.JID, _ bool) (domain.BlocklistUpdate, error) {
		return domain.BlocklistUpdate{ResolvedJID: target, RequestedJID: target}, nil
	}

	rec := serveUserRoute(t, "/user/block", f.handlers().BlockUser(),
		http.MethodPost, "/user/block", `{"chat":"5511999999999"}`)
	data := dataOf(t, rec)

	if data["jid"] != "5511999999999" {
		t.Errorf("jid = %#v: o alias `chat` não chegou ao caso de uso", data["jid"])
	}
}

// --- POST /user/avatar -----------------------------------------------------

func TestGetAvatar_ContratoPublico(t *testing.T) {
	f := chNewFakes()

	rec := serveUserRoute(t, "/user/avatar", f.avatar(),
		http.MethodPost, "/user/avatar", `{"phone":"5511999999999","preview":true}`)
	data := dataOf(t, rec)

	if data["id"] != "pic-1" || data["url"] != "https://example.com/pic-1" {
		t.Errorf("data = %#v", data)
	}
	if len(f.contacts.GetProfilePictureCalls) != 1 || !f.contacts.GetProfilePictureCalls[0].Preview {
		t.Errorf("chamada = %+v, quero preview=true", f.contacts.GetProfilePictureCalls)
	}
}

// --- GET /user/contacts ----------------------------------------------------

func TestGetContacts_ContratoPublico(t *testing.T) {
	f := chNewFakes()
	f.contacts.GetAllContactsFunc = func(context.Context, string) ([]domain.Contact, int, error) {
		return []domain.Contact{
			{
				JID: "111111111111111@lid", LID: "111111111111111@lid",
				PN: "5511999999999@s.whatsapp.net", Found: true,
				FirstName: "Ana", FullName: "Ana Silva",
				PushName: "Aninha", BusinessName: "Loja da Ana", IsBusiness: true,
			},
			{JID: "5511888888888@s.whatsapp.net", PN: "5511888888888@s.whatsapp.net"},
		}, 2, nil
	}

	rec := serveUserRoute(t, "/user/contacts", f.contactsHandler(),
		http.MethodGet, "/user/contacts", "")
	data := dataOf(t, rec)

	// Chaves do types.ContactInfo do SDK, que era o que este rota servia.
	contracttest.AssertNoKeys(t, rec.Body.Bytes(),
		"Found", "FirstName", "FullName", "PushName", "BusinessName", "Contacts")

	if data["count"] != float64(2) {
		t.Errorf("count = %#v, quero 2", data["count"])
	}
	contatos, ok := data["contacts"].([]any)
	if !ok || len(contatos) != 2 {
		t.Fatalf("contacts = %#v, quero 2 elementos", data["contacts"])
	}
	primeiro, _ := contatos[0].(map[string]any)
	quero := map[string]any{
		"jid":           "111111111111111@lid",
		"lid":           "111111111111111@lid",
		"phone_number":  "5511999999999@s.whatsapp.net",
		"found":         true,
		"first_name":    "Ana",
		"full_name":     "Ana Silva",
		"push_name":     "Aninha",
		"business_name": "Loja da Ana",
		"is_business":   true,
	}
	for chave, esperado := range quero {
		if got := primeiro[chave]; got != esperado {
			t.Errorf("contacts[0].%s = %#v, quero %#v", chave, got, esperado)
		}
	}
	segundo, _ := contatos[1].(map[string]any)
	if segundo["found"] != false || segundo["is_business"] != false || segundo["lid"] != "" {
		t.Errorf("contacts[1] = %#v", segundo)
	}
}

func TestGetContacts_AgendaVaziaNaoViraNull(t *testing.T) {
	f := chNewFakes()
	f.contacts.GetAllContactsFunc = func(context.Context, string) ([]domain.Contact, int, error) {
		return []domain.Contact{}, 0, nil
	}

	rec := serveUserRoute(t, "/user/contacts", f.contactsHandler(),
		http.MethodGet, "/user/contacts", "")
	data := dataOf(t, rec)

	if lista, ok := data["contacts"].([]any); !ok || len(lista) != 0 {
		t.Errorf("contacts = %#v, quero []", data["contacts"])
	}
}

// --- GET /user/contacts/last-activity --------------------------------------

// TestContactsLastActivity_ChavesDeDadosViraramValores é a razão de esta rota
// ter mudado de FORMA e não só de nomes.
//
// Ela servia um mapa com o JID do chat na CHAVE. A regra de nomes do contrato
// (docs/HTTP-DTO-CONVENTIONS.md §8) vale para toda chave de objecto JSON, e um
// JID nunca casa com `^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$` — `@` e `.` não estão
// no alfabeto. Não havia renomear: o dado tinha de sair da chave.
func TestContactsLastActivity_ChavesDeDadosViraramValores(t *testing.T) {
	f := chNewFakes()
	quando := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)
	activity := &contractsfake.ChatActivityReader{
		GetLastActivityByUserFunc: func(context.Context, string) (map[string]time.Time, error) {
			return map[string]time.Time{
				"5511999999999@s.whatsapp.net": quando,
				"111111111111111@lid":          {},
			}, nil
		},
	}
	h := NewGetContactsLastActivityHandler(
		user.NewGetContactsLastActivityUseCase(activity, f.contacts, f.logger))

	rec := serveUserRoute(t, "/user/contacts/last-activity", h,
		http.MethodGet, "/user/contacts/last-activity", "")
	data := dataOf(t, rec)

	contracttest.AssertNoKeys(t, rec.Body.Bytes(),
		"5511999999999@s.whatsapp.net", "111111111111111@lid")

	chats, ok := data["chats"].([]any)
	if !ok || len(chats) != 2 {
		t.Fatalf("chats = %#v, quero 2 elementos", data["chats"])
	}
	// Ordenado por JID, e não pela ordem do mapa: a iteração de mapa em Go é
	// aleatória por desenho, e uma resposta que baralha não se consegue
	// diffar entre duas chamadas.
	primeiro, _ := chats[0].(map[string]any)
	if primeiro["jid"] != "111111111111111@lid" {
		t.Errorf("chats[0].jid = %#v: a ordenação por JID não aconteceu", primeiro["jid"])
	}
	// Tempo zero é null, e não "0001-01-01T00:00:00Z" — essa string é uma data
	// real no fio.
	valor, presente := primeiro["last_activity"]
	if !presente {
		t.Error("last_activity ausente: a chave tem de existir mesmo sem valor")
	}
	if valor != nil {
		t.Errorf("last_activity = %#v, quero null para tempo zero", valor)
	}
	segundo, _ := chats[1].(map[string]any)
	if segundo["last_activity"] != "2024-03-01T12:00:00Z" {
		t.Errorf("chats[1].last_activity = %#v", segundo["last_activity"])
	}
}

func TestContactsLastActivity_SemHistoricoServeListaVazia(t *testing.T) {
	f := chNewFakes()
	activity := &contractsfake.ChatActivityReader{}
	h := NewGetContactsLastActivityHandler(
		user.NewGetContactsLastActivityUseCase(activity, f.contacts, f.logger))

	rec := serveUserRoute(t, "/user/contacts/last-activity", h,
		http.MethodGet, "/user/contacts/last-activity", "")
	data := dataOf(t, rec)

	if chats, ok := data["chats"].([]any); !ok || len(chats) != 0 {
		t.Errorf("chats = %#v, quero []", data["chats"])
	}
}

// --- /user/privacy ---------------------------------------------------------

func TestGetPrivacySettings_ContratoPublico(t *testing.T) {
	pm := &contractsfake.PrivacyManager{
		GetPrivacySettingsFunc: func(context.Context, string) (domain.PrivacySettings, error) {
			return domain.PrivacySettings{
				GroupAdd: "contacts", LastSeen: "all", Status: "contacts",
				Profile: "all", ReadReceipts: "none", CallAdd: "known",
				Online: "match_last_seen", Messages: "all",
				Defense: "on_standard", Stickers: "contacts",
			}, nil
		},
	}
	h := NewGetPrivacySettingsHandler(
		user.NewGetPrivacySettingsUseCase(pm, &contractsfake.Logger{}))

	rec := serveUserRoute(t, "/user/privacy", h, http.MethodGet, "/user/privacy", "")
	data := dataOf(t, rec)

	// Chaves do types.PrivacySettings do SDK, que era o que esta rota servia.
	contracttest.AssertNoKeys(t, rec.Body.Bytes(),
		"GroupAdd", "LastSeen", "Status", "Profile", "ReadReceipts",
		"CallAdd", "Online", "Messages", "Defense", "Stickers")

	quero := map[string]any{
		"group_add": "contacts", "last_seen": "all", "status": "contacts",
		"profile": "all", "read_receipts": "none", "call_add": "known",
		"online": "match_last_seen", "messages": "all",
		"defense": "on_standard", "stickers": "contacts",
	}
	for chave, esperado := range quero {
		if got := data[chave]; got != esperado {
			t.Errorf("%s = %#v, quero %#v", chave, got, esperado)
		}
	}
}

func TestSetPrivacySetting_ContratoPublico(t *testing.T) {
	pm := &contractsfake.PrivacyManager{
		SetPrivacySettingFunc: func(context.Context, string, string, string) (domain.PrivacySettings, error) {
			return domain.PrivacySettings{GroupAdd: "contacts"}, nil
		},
	}
	h := NewSetPrivacySettingHandler(
		user.NewSetPrivacySettingUseCase(pm, &contractsfake.Logger{}))

	rec := serveUserRoute(t, "/user/privacy", h, http.MethodPost, "/user/privacy",
		`{"privacy_setting":"groupadd","value":"contacts"}`)
	data := dataOf(t, rec)

	if data["group_add"] != "contacts" {
		t.Errorf("group_add = %#v", data["group_add"])
	}
	// A resposta traz TODAS as configurações, não só a que mudou — as outras
	// vêm vazias porque o motor não as reportou, e vazio é distinguível de
	// qualquer valor aceite.
	if _, presente := data["last_seen"]; !presente {
		t.Error("last_seen ausente: a resposta traz o estado RESULTANTE de todas")
	}
}

// TestSetPrivacySetting_ValidaNaFronteira: o DTO de pedido recusa antes de a
// rota falar com o WhatsApp, com o código de erro do domínio.
func TestSetPrivacySetting_ValidaNaFronteira(t *testing.T) {
	pm := &contractsfake.PrivacyManager{}
	h := NewSetPrivacySettingHandler(
		user.NewSetPrivacySettingUseCase(pm, &contractsfake.Logger{}))

	rec := serveUserRoute(t, "/user/privacy", h, http.MethodPost, "/user/privacy",
		`{"privacy_setting":"inexistente","value":"all"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quero 400; corpo: %s", rec.Code, rec.Body.String())
	}
	if len(pm.SetPrivacySettingCalls) != 0 {
		t.Error("a recusa de fronteira chegou à porta")
	}
	assertErrorCode(t, rec, domain.InvalidPrivacySettingCode)
}

// assertErrorCode afirma o error.code do envelope de erro. É por ele que um
// cliente ramifica; a mensagem pode mudar.
func assertErrorCode(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	var envelope struct {
		Success bool `json:"success"`
		Error   struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("corpo não é o envelope canónico: %v\n%s", err, rec.Body.String())
	}
	if envelope.Success {
		t.Errorf("success = true num corpo de erro: %s", rec.Body.String())
	}
	if envelope.Error.Code != want {
		t.Errorf("error.code = %q, quero %q; corpo: %s",
			envelope.Error.Code, want, rec.Body.String())
	}
	contracttest.AssertPublicJSONUsesCanonicalNaming(t, rec.Body.Bytes())
}
