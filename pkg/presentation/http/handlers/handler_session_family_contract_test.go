package handlers

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/notification"
	"wa-api/pkg/application/usecase/session"
	"wa-api/pkg/application/usecase/storage"
	"wa-api/pkg/capabilityregistry"
	"wa-api/pkg/domain"
	"wa-api/pkg/pairing"
	customhttp "wa-api/pkg/presentation/http"
	"wa-api/pkg/presentation/http/contracttest"

	"github.com/gorilla/mux"
)

// Testes de contrato PÚBLICO da família sessão + configuração de webhook +
// configuração de armazenamento, no padrão de
// handler_group_info_contract_test.go. O que eles afirmam, e que os testes de
// eixo já existentes NÃO afirmam:
//
//  1. o pedido passa pela ROTA REGISTADA, no mesmo mux/gorilla que a produção
//     usa (ARMADILHAS #2);
//  2. TODA chave do corpo, recursivamente, é snake_case minúsculo — afirmado
//     pelo helper partilhado, não por uma lista escrita à mão que envelhece;
//  3. as chaves ANTIGAS desapareceram. "a chave nova existe" não prova
//     migração: um struct pode carregar as duas;
//  4. os VALORES chegaram aos campos certos, e o que era omitempty aparece
//     mesmo no zero.

// familyRouter monta UMA rota exactamente como wiring_routes.go a monta.
func familyRouter(t *testing.T, path string, h http.Handler, method string) *mux.Router {
	t.Helper()
	registry := customhttp.NewHandlerRegistry()
	registry.Register(path, withContractUser(h), method)
	router := mux.NewRouter()
	registry.Apply(router)
	return router
}

// serveFamily faz o pedido pela rota registada e devolve o gravador.
func serveFamily(t *testing.T, router *mux.Router, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

// familyData decodifica `data` do envelope, exigindo 200 e success=true.
func familyData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
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
	if !envelope.Success || envelope.Code != http.StatusOK {
		t.Fatalf("envelope = %+v, quero success=true code=200", envelope)
	}
	return envelope.Data
}

// --- GET /session/status ------------------------------------------------

// statusEntry é o registo que ListUsers devolve para uma sessão com TUDO
// preenchido. Todos os campos de propósito: um dublê com metade dos campos no
// zero deixaria metade do mapeamento por medir.
func statusEntry() domain.UserListEntry {
	return domain.UserListEntry{
		ID:          "u1",
		Name:        "Sessão de teste",
		Webhook:     "https://example.com/hook",
		JID:         "5511999999999@s.whatsapp.net",
		QRCode:      "2@codigo-de-pareamento",
		ProxyURL:    "socks5://203.0.113.10:1080",
		HasProxyURL: true,
		Events:      "Message,ReadReceipt",
		History:     30,
		S3: domain.S3Config{
			Enabled:       true,
			Endpoint:      "https://s3.example.com",
			Region:        "us-east-1",
			Bucket:        "mybucket",
			PathStyle:     true,
			PublicURL:     "https://cdn.example.com",
			MediaDelivery: domain.MediaDeliveryBoth,
			RetentionDays: 7,
		},
	}
}

func statusRouter(t *testing.T) *mux.Router {
	t.Helper()
	// connected=true e loggedIn=FALSE, e a diferença é o que faz o teste
	// morder: com os dois verdadeiros, trocar `connected` por `logged_in` no
	// apresentador passaria em tudo (medido — o controlo negativo com os dois
	// iguais devolveu `ok`).
	status := &contractsfake.SessionStatusReader{
		SessionStatusFunc: func(context.Context, string) (bool, bool) { return true, false },
	}
	users := &contractsfake.UserRepository{
		ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
			return []domain.UserListEntry{statusEntry()}, nil
		},
	}
	uc := session.NewGetStatusUseCase(status, users, &contractsfake.Logger{})
	return familyRouter(t, "/session/status", NewGetStatusHandler(uc), http.MethodGet)
}

func TestSessionStatus_ContratoPublico_NomesCanonicos(t *testing.T) {
	rec := serveFamily(t, statusRouter(t), http.MethodGet, "/session/status", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200; corpo: %s", rec.Code, rec.Body.String())
	}
	contracttest.AssertPublicJSONUsesCanonicalNaming(t, rec.Body.Bytes())
}

// TestSessionStatus_ContratoPublico_ChavesAntigasSumiram: `loggedIn` e
// `proxyUrl` eram as duas grafias camelCase desta resposta, e `qrcode` era o
// nome antigo do QR. `token` nunca deve aparecer: o campo existe no domínio
// para o registo ser lido numa consulta, não para o segredo voltar em cada
// sondagem de estado.
func TestSessionStatus_ContratoPublico_ChavesAntigasSumiram(t *testing.T) {
	rec := serveFamily(t, statusRouter(t), http.MethodGet, "/session/status", "")
	contracttest.AssertNoKeys(t, rec.Body.Bytes(),
		"loggedIn", "LoggedIn", "proxyUrl", "ProxyURL", "qrcode", "QRCode",
		"token", "Token", "Connected", "Webhook", "Events",
	)
}

func TestSessionStatus_ContratoPublico_ValoresMapeados(t *testing.T) {
	data := familyData(t, serveFamily(t, statusRouter(t), http.MethodGet, "/session/status", ""))
	entry := statusEntry()

	quero := map[string]any{
		"id":              entry.ID,
		"name":            entry.Name,
		"connected":       true,
		"logged_in":       false,
		"jid":             entry.JID,
		"webhook":         entry.Webhook,
		"events":          entry.Events,
		"proxy_url":       entry.ProxyURL,
		"qr_code":         entry.QRCode,
		"history":         "30",
		"hmac_configured": false,
	}
	for chave, esperado := range quero {
		if got := data[chave]; got != esperado {
			t.Errorf("data.%s = %#v, quero %#v", chave, got, esperado)
		}
	}

	proxy, ok := data["proxy_config"].(map[string]any)
	if !ok {
		t.Fatalf("proxy_config = %#v, quero objecto", data["proxy_config"])
	}
	if proxy["enabled"] != true || proxy["proxy_url"] != entry.ProxyURL {
		t.Errorf("proxy_config = %#v", proxy)
	}

	s3, ok := data["s3_config"].(map[string]any)
	if !ok {
		t.Fatalf("s3_config = %#v, quero objecto", data["s3_config"])
	}
	queroS3 := map[string]any{
		"enabled":        true,
		"endpoint":       entry.S3.Endpoint,
		"region":         entry.S3.Region,
		"bucket":         entry.S3.Bucket,
		"path_style":     true,
		"public_url":     entry.S3.PublicURL,
		"media_delivery": domain.MediaDeliveryBoth,
		"retention_days": float64(7),
	}
	for chave, esperado := range queroS3 {
		if got := s3[chave]; got != esperado {
			t.Errorf("s3_config.%s = %#v, quero %#v", chave, got, esperado)
		}
	}
	// Nenhuma credencial no resumo de estado, nem sequer mascarada: quem
	// acrescentar uma chave nova aqui parte este teste.
	if len(s3) != len(queroS3) {
		t.Errorf("s3_config tem %d chaves (%v), quero exactamente %d", len(s3), s3, len(queroS3))
	}
}

// --- GET /session/qr ----------------------------------------------------

func TestSessionQR_ContratoPublico(t *testing.T) {
	users := &contractsfake.UserRepository{
		ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
			return []domain.UserListEntry{{ID: "u1", Engine: domain.EngineWaNoise}}, nil
		},
	}
	qr := &contractsfake.PairingQRReader{
		PairingQRFunc: func(context.Context, string) (string, error) {
			// Forma REAL do adapter wa_noise: le' users.qrcode, onde o
			// orquestrador ja' gravou o PNG codificado. Ver F373.
			return qrImageOf(t, qrCodePersistido), nil
		},
	}
	reg := pairing.NewRegistry(users, capabilityregistry.NewCapabilityRegistry(),
		&pairing.Provider{Engine: domain.EngineWaNoise, QRReader: qr},
		&pairing.Provider{Engine: domain.EngineWaHeadless})
	router := familyRouter(t, "/session/qr", NewGetQRHandler(&contractsfake.Logger{}, reg), http.MethodGet)

	rec := serveFamily(t, router, http.MethodGet, "/session/qr?engine=wa_noise", "")
	data := familyData(t, rec)

	contracttest.AssertPublicJSONUsesCanonicalNaming(t, rec.Body.Bytes())
	contracttest.AssertNoKeys(t, rec.Body.Bytes(), "QRCode", "qrcode", "qrCode")
	if data["qr_code"] != qrImageOf(t, qrCodePersistido) {
		t.Errorf("qr_code = %#v", data["qr_code"])
	}
}

// --- POST /session/pairphone --------------------------------------------

func TestSessionPairPhone_ContratoPublico(t *testing.T) {
	pp := &contractsfake.PhonePairer{
		RequestPairingCodeFunc: func(context.Context, string, string) (string, error) {
			return "WXYZ-2468", nil
		},
	}
	users := &contractsfake.UserRepository{
		ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
			return []domain.UserListEntry{{ID: "u1", Engine: domain.EngineWaNoise}}, nil
		},
	}
	reg := pairing.NewRegistry(users, capabilityregistry.NewCapabilityRegistry(),
		&pairing.Provider{Engine: domain.EngineWaNoise, PhonePairer: pp},
		&pairing.Provider{Engine: domain.EngineWaHeadless})
	router := familyRouter(t, "/session/pairphone", NewPairPhoneHandler(&contractsfake.Logger{}, reg), http.MethodPost)

	rec := serveFamily(t, router, http.MethodPost, "/session/pairphone", `{"engine":"wa_noise","phone":"5511999999999"}`)
	data := familyData(t, rec)

	contracttest.AssertPublicJSONUsesCanonicalNaming(t, rec.Body.Bytes())
	contracttest.AssertNoKeys(t, rec.Body.Bytes(), "LinkingCode", "linkingCode")
	if data["linking_code"] != "WXYZ-2468" {
		t.Errorf("linking_code = %#v", data["linking_code"])
	}
	// O telefone do corpo com a chave NOVA chega à porta.
	if len(pp.RequestPairingCodeCalls) != 1 || pp.RequestPairingCodeCalls[0].Phone != "5511999999999" {
		t.Errorf("a porta recebeu %+v: a chave `phone` do DTO não alimentou o comando", pp.RequestPairingCodeCalls)
	}
}

// --- /webhook -----------------------------------------------------------

func TestGetWebhook_ContratoPublico(t *testing.T) {
	spec := &webhookRowsSpec{rows: [][]driver.Value{{"https://example.com/hook", "Message,ReadReceipt"}}}
	db := &webhookFakeDB{rowsDB: newWebhookRowsDB(t, spec)}
	router := familyRouter(t, "/webhook", NewGetWebhookHandler(newWebhookTestContext(db)), http.MethodGet)

	rec := serveFamily(t, router, http.MethodGet, "/webhook", "")
	data := familyData(t, rec)

	contracttest.AssertPublicJSONUsesCanonicalNaming(t, rec.Body.Bytes())
	if data["webhook"] != "https://example.com/hook" {
		t.Errorf("webhook = %#v", data["webhook"])
	}
	eventos, ok := data["subscribe"].([]any)
	if !ok || len(eventos) != 2 || eventos[0] != "Message" || eventos[1] != "ReadReceipt" {
		t.Errorf("subscribe = %#v", data["subscribe"])
	}
}

// TestUpdateWebhook_ContratoPublico trava a decisão do apresentador sobre a
// lista vazia: `[]` e nunca `null`. O handler constrói validEvents com
// append-a-nil, então um cliente que mande só eventos desconhecidos recebia
// `"events": null` — e só uma das duas formas se percorre sem verificação.
func TestUpdateWebhook_ContratoPublico(t *testing.T) {
	db := &webhookFakeDB{}
	router := familyRouter(t, "/webhook", NewUpdateWebhookHandler(newWebhookTestContext(db)), http.MethodPut)

	rec := serveFamily(t, router, http.MethodPut, "/webhook",
		`{"webhook":"https://example.com/hook","events":["NaoExiste"],"active":true}`)
	data := familyData(t, rec)

	contracttest.AssertPublicJSONUsesCanonicalNaming(t, rec.Body.Bytes())
	if lista, ok := data["events"].([]any); !ok || len(lista) != 0 {
		t.Errorf("events = %#v, quero []", data["events"])
	}
	if data["active"] != true {
		t.Errorf("active = %#v", data["active"])
	}
}

func TestDeleteWebhook_ContratoPublico(t *testing.T) {
	db := &webhookFakeDB{}
	router := familyRouter(t, "/webhook", NewDeleteWebhookHandler(newWebhookTestContext(db)), http.MethodDelete)

	rec := serveFamily(t, router, http.MethodDelete, "/webhook", "")
	data := familyData(t, rec)

	contracttest.AssertPublicJSONUsesCanonicalNaming(t, rec.Body.Bytes())
	contracttest.AssertNoKeys(t, rec.Body.Bytes(), "Details")
	if data["details"] != webhookDeletedDetails {
		t.Errorf("details = %#v", data["details"])
	}
}

// --- GET /webhook/history ----------------------------------------------

// TestGetHistory_ContratoPublico: `history` sai mesmo valendo zero. Zero
// significa "histórico desligado", que é a resposta mais comum da rota, e o
// omitempty antigo fazia-a desaparecer do corpo (F165).
func TestGetHistory_ContratoPublico(t *testing.T) {
	uc := storage.NewGetHistoryUseCase(&contractsfake.SessionGuard{}, &contractsfake.HistoryConfigStore{}, silentLogger{})
	router := familyRouter(t, "/webhook/history", NewGetHistoryHandler(uc), http.MethodGet)

	rec := serveFamily(t, router, http.MethodGet, "/webhook/history", "")
	data := familyData(t, rec)

	contracttest.AssertPublicJSONUsesCanonicalNaming(t, rec.Body.Bytes())
	contracttest.AssertNoKeys(t, rec.Body.Bytes(), "History", "Details")
	valor, presente := data["history"]
	if !presente {
		t.Fatal("history ausente: a chave tem de existir mesmo no zero, senão o cliente não distingue desligado de não-reportado")
	}
	if valor != float64(0) {
		t.Errorf("history = %#v, quero 0", valor)
	}
}

// --- GET /s3/config -----------------------------------------------------

func TestGetS3Config_ContratoPublico(t *testing.T) {
	// O store é semeado sob o MESMO id que withContractUser injecta ("u1"):
	// enabledS3Store() usa "42", da tabela de eixos, e um store semeado sob
	// outro id devolveria a configuração VAZIA — o teste passaria a medir a
	// máscara sobre nada.
	store := enabledS3Store()
	store.Stored[contractUserID] = store.Stored["42"]
	uc := storage.NewGetS3ConfigUseCase(&contractsfake.SessionGuard{}, store, silentLogger{})
	router := familyRouter(t, "/s3/config", NewGetS3ConfigHandler(uc), http.MethodGet)

	rec := serveFamily(t, router, http.MethodGet, "/s3/config", "")
	data := familyData(t, rec)

	contracttest.AssertPublicJSONUsesCanonicalNaming(t, rec.Body.Bytes())
	contracttest.AssertNoKeys(t, rec.Body.Bytes(), "Details", "Enabled", "AccessKey", "secret_key", "SecretKey")
	if data["access_key"] != domain.MaskedS3AccessKey {
		t.Errorf("access_key = %#v, quero %q", data["access_key"], domain.MaskedS3AccessKey)
	}
	if data["enabled"] != true || data["bucket"] != "b" || data["region"] != "us-east-1" {
		t.Errorf("a leitura não devolveu a configuração gravada: %#v", data)
	}
}

// --- POST /proxy/set ----------------------------------------------------

// TestSetProxy_ContratoPublico_Habilitar trava as três chaves que eram
// PascalCase (`Details`, `Set`, `ProxyURL`).
func TestSetProxy_ContratoPublico_Habilitar(t *testing.T) {
	router := familyRouter(t, "/proxy/set",
		newTestSetProxyHandler(&contractsfake.SessionStatusReader{}), http.MethodPost)

	rec := serveFamily(t, router, http.MethodPost, "/proxy/set",
		`{"enable":true,"proxy_url":"http://203.0.113.10:3128","webhook_use_proxy":true}`)
	data := familyData(t, rec)

	contracttest.AssertPublicJSONUsesCanonicalNaming(t, rec.Body.Bytes())
	contracttest.AssertNoKeys(t, rec.Body.Bytes(), "Details", "Set", "ProxyURL", "proxyURL")
	if data["set"] != true {
		t.Errorf("set = %#v, quero true", data["set"])
	}
	if data["proxy_url"] != "http://203.0.113.10:3128" {
		t.Errorf("proxy_url = %#v", data["proxy_url"])
	}
	if data["webhook_use_proxy"] != true {
		t.Errorf("webhook_use_proxy = %#v", data["webhook_use_proxy"])
	}
}

// TestSetProxy_ContratoPublico_Desabilitar mede o ramo em que o omitempty
// antigo APAGAVA três campos do corpo. Hoje eles vêm no zero, e
// webhook_use_proxy vem `null` — o ramo de desabilitação não resolve a
// bandeira, então não tem um valor verdadeiro a reportar.
func TestSetProxy_ContratoPublico_Desabilitar(t *testing.T) {
	router := familyRouter(t, "/proxy/set",
		newTestSetProxyHandler(&contractsfake.SessionStatusReader{}), http.MethodPost)

	rec := serveFamily(t, router, http.MethodPost, "/proxy/set", `{"enable":false}`)
	data := familyData(t, rec)

	contracttest.AssertPublicJSONUsesCanonicalNaming(t, rec.Body.Bytes())
	for _, chave := range []string{"details", "set", "proxy_url", "webhook_use_proxy"} {
		if _, presente := data[chave]; !presente {
			t.Errorf("%s ausente: o omitempty voltou e o cliente deixou de distinguir desligado de não-reportado", chave)
		}
	}
	if data["set"] != false || data["proxy_url"] != "" {
		t.Errorf("data = %#v, quero set=false e proxy_url=\"\"", data)
	}
	if data["webhook_use_proxy"] != nil {
		t.Errorf("webhook_use_proxy = %#v, quero null", data["webhook_use_proxy"])
	}
}

// --- GET /health --------------------------------------------------------

// TestHealth_ContratoPublico: `memory_stats` deixou de ser um
// map[string]interface{} montado no use case — as suas chaves não apareciam
// em auditoria de etiqueta nenhuma — e `version` perdeu o omitempty.
func TestHealth_ContratoPublico(t *testing.T) {
	db := newWebhookRowsDB(t, &webhookRowsSpec{rows: [][]driver.Value{{"7", ""}}})
	uc := notification.NewGetHealthUseCase(db, healthCounter{}, &contractsfake.Logger{}, "")
	router := familyRouter(t, "/health", NewGetHealthHandler(uc), http.MethodGet)

	rec := serveFamily(t, router, http.MethodGet, "/health", "")
	data := familyData(t, rec)

	contracttest.AssertPublicJSONUsesCanonicalNaming(t, rec.Body.Bytes())
	if _, presente := data["version"]; !presente {
		t.Error("version ausente: o omitempty voltou, e um binário sem versão gravada deixou de ser distinguível de um sem o campo")
	}
	mem, ok := data["memory_stats"].(map[string]any)
	if !ok {
		t.Fatalf("memory_stats = %#v, quero objecto", data["memory_stats"])
	}
	for _, chave := range []string{"alloc_mb", "total_alloc_mb", "sys_mb", "num_gc"} {
		if _, presente := mem[chave]; !presente {
			t.Errorf("memory_stats.%s ausente: %#v", chave, mem)
		}
	}
	if data["connected_users"] != float64(2) || data["logged_in_users"] != float64(1) {
		t.Errorf("as contagens não chegaram: %#v", data)
	}
}

// healthCounter é o dublê de appport.SessionCounter com contagens DISTINTAS
// entre si: três valores iguais deixariam uma troca entre dois campos vizinhos
// passar em todos os testes.
type healthCounter struct{}

func (healthCounter) CountSessions(context.Context) (domain.SessionCounts, error) {
	return domain.SessionCounts{Total: 3, Connected: 2, LoggedIn: 1}, nil
}

var _ appport.SessionCounter = healthCounter{}
var _ = sql.ErrNoRows
