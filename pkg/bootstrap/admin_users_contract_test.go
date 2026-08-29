package bootstrap

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/infra/db"
	"wa-api/pkg/infra/noise/observability/applog"
	"wa-api/pkg/presentation/http/contracttest"
	"wa-api/pkg/presentation/http/handlers"
)

// Testes de CONTRATO PÚBLICO da família /admin (migração para DTO,
// docs/HTTP-DTO-CONVENTIONS.md).
//
// O que eles afirmam, e que um teste do handler cru NÃO afirma:
//
//  1. o pedido passa pela ROTA REGISTADA — registerAdminRoutes sobre um
//     gorilla/mux com o subrouter /admin e o middleware authAdmin, que é a
//     montagem de router.go:285-290. Handler montado à mão não exercita nem o
//     método, nem o padrão de caminho, nem a extração de `{id}`
//     (ARMADILHAS #2);
//  2. TODA chave do corpo, recursivamente, é snake_case minúsculo — afirmado
//     pelo helper partilhado, não por uma lista escrita à mão;
//  3. as chaves ANTIGAS desapareceram. "a chave nova existe" não prova
//     migração: um struct pode carregar as duas.
//
// O banco é SQLite real com o schema de produção e o repositório real. O
// dublê ficaria de fora justamente do que importa aqui: os nomes que o
// utilizador GRAVADO produz ao ser lido de volta.

// adminContractToken é o token que authAdmin exige. Constante nomeada porque
// aparece na montagem e em cada pedido (ADR-0004).
const adminContractToken = "token-admin-de-contrato"

// adminFixture é o router de /admin montado como a produção o monta, mais o
// banco por trás dele.
type adminFixture struct {
	db     *sqlx.DB
	router *mux.Router
}

// newAdminFixture monta as MESMAS rotas que registerAdminRoutes regista, com
// o subrouter /admin e authAdmin por cima — a chain que router.go aplica.
func newAdminFixture(t *testing.T) *adminFixture {
	t.Helper()

	anterior := appCtx
	appCtx = NewAppContext()
	appCtx.GlobalEncryptionKey = addUserTestEncryptionKey
	t.Cleanup(func() { appCtx = anterior })

	database := newChatHistoryDB(t)
	logger := applog.NewZerologAdapter(zerolog.New(nil))
	repo := db.NewUserRepository(database)

	// O leitor de estado da sessão é dublê porque a sessão de WhatsApp não
	// existe num teste; tudo o resto é a dependência real de
	// wiring_handlers.go.
	sessions := &contractsfake.SessionStatusReader{
		SessionStatusFunc: func(context.Context, string) (bool, bool) { return true, true },
	}

	userHandlers := handlers.NewUserHandlers(
		user.NewListUsersUseCase(repo, logger, sessions),
		user.NewAddUserUseCase(repo, hmacKeyEncryptor{}, s3SecretCipher{}, logger, true),
		user.NewEditUserUseCase(repo, s3SecretCipher{}, userInfoRepublisher{db: database}, logger),
		user.NewDeleteUserUseCase(repo, userInfoRepublisher{db: database}, logger),
		nil, nil, nil, nil, nil, nil, nil,
	)
	ch := &customHandlers{
		User: userHandlers,
		Misc: &handlers.MiscHandlers{
			DeleteUserComplete: handlers.NewDeleteUserCompleteHandler(
				user.NewDeleteUserCompleteUseCase(database.DB, &contractsfake.SessionController{}, userInfoRepublisher{db: database}, logger, t.TempDir()),
			),
		},
	}

	router := mux.NewRouter()
	adminRoutes := router.PathPrefix("/admin").Subrouter()
	adminRoutes.Use(authAdmin(adminContractToken))
	registerAdminRoutes(adminRoutes, ch)
	return &adminFixture{db: database, router: router}
}

// do dispara um pedido autenticado como admin pela rota registada.
func (f *adminFixture) do(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", adminContractToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

// corpoDeCriacao é o corpo de POST /admin/users com TODOS os campos
// preenchidos de propósito: um corpo com metade dos campos no zero deixaria
// metade do mapeamento por medir.
//
// Todas as chaves em snake_case, incluindo as dos objectos aninhados. Era
// aqui que estava a mistura: a rota LIA `proxyConfig`/`s3Config`/`hmacKey` e
// `proxyUrl`/`accessKey`/`pathStyle` em camelCase e RESPONDIA em snake_case.
const corpoDeCriacao = `{
  "name": "alice",
  "token": "token-da-alice",
  "webhook": "http://webhook.exemplo",
  "expiration": 3600,
  "events": "Message",
  "hmac_key": "chave-hmac-de-trinta-e-dois-carA",
  "history": 50,
  "engine": "wa_noise",
  "proxy_config": {"enabled": true, "proxy_url": "http://proxy.exemplo", "webhook_use_proxy": false},
  "s3_config": {
    "enabled": true, "endpoint": "http://s3.exemplo", "region": "us-east-1",
    "bucket": "balde", "access_key": "AKIA", "secret_key": "segredo",
    "path_style": true, "public_url": "http://cdn.exemplo",
    "media_delivery": "base64", "retention_days": 7
  }
}`

// chavesAntigasDaFamiliaAdmin são os nomes que esta migração tinha de FAZER
// DESAPARECER: o camelCase que o `encoding/json` emitia e as chaves dos
// map[string]any que o use case montava à mão.
var chavesAntigasDaFamiliaAdmin = []string{
	"loggedIn", "proxyUrl", "webhookUseProxy",
	"accessKey", "secretKey", "pathStyle", "publicUrl",
	"mediaDelivery", "retentionDays", "hmacKey",
	"proxyConfig", "s3Config", "access_key",
}

func TestAdminUsers_ContratoPublico_NomesCanonicos(t *testing.T) {
	f := newAdminFixture(t)

	criado := f.do(t, http.MethodPost, "/admin/users", corpoDeCriacao)
	if criado.Code != http.StatusOK {
		t.Fatalf("POST status = %d, quero 200; corpo: %s", criado.Code, criado.Body.String())
	}
	id := idDoCorpo(t, criado)

	casos := []struct {
		nome, metodo, caminho, corpo string
	}{
		{"criação", http.MethodPost, "", ""}, // o corpo já está em `criado`
		{"listagem", http.MethodGet, "/admin/users", ""},
		{"leitura de um", http.MethodGet, "/admin/users/" + id, ""},
		{"edição", http.MethodPut, "/admin/users/" + id, `{"name":"alice-2","history":0}`},
		{"remoção completa", http.MethodDelete, "/admin/users/" + id + "/full", ""},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			rec := criado
			if c.caminho != "" {
				rec = f.do(t, c.metodo, c.caminho, c.corpo)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, quero 200; corpo: %s", rec.Code, rec.Body.String())
			}
			contracttest.AssertPublicJSONUsesCanonicalNaming(t, rec.Body.Bytes())
			contracttest.AssertNoKeys(t, rec.Body.Bytes(), chavesAntigasDaFamiliaAdmin...)
		})
	}
}

// TestAdminUsers_ContratoPublico_DeleteSimples exercita a rota que o teste
// acima não pode exercitar na mesma sequência: apagar o utilizador impede
// tudo o que viesse depois.
func TestAdminUsers_ContratoPublico_DeleteSimples(t *testing.T) {
	f := newAdminFixture(t)
	id := idDoCorpo(t, f.do(t, http.MethodPost, "/admin/users", corpoDeCriacao))

	rec := f.do(t, http.MethodDelete, "/admin/users/"+id, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200; corpo: %s", rec.Code, rec.Body.String())
	}
	contracttest.AssertPublicJSONUsesCanonicalNaming(t, rec.Body.Bytes())

	var envelope struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("corpo inválido: %v", err)
	}
	if envelope.Data.Status != "deleted" {
		t.Errorf("status = %q, quero \"deleted\"", envelope.Data.Status)
	}
}

// TestAdminUsers_ContratoPublico_ValoresMapeados prova que o apresentador não
// só produziu as chaves certas como pôs os VALORES certos nelas — uma troca
// entre dois booleanos vizinhos passaria em todos os testes de nome.
func TestAdminUsers_ContratoPublico_ValoresMapeados(t *testing.T) {
	f := newAdminFixture(t)
	id := idDoCorpo(t, f.do(t, http.MethodPost, "/admin/users", corpoDeCriacao))

	rec := f.do(t, http.MethodGet, "/admin/users/"+id, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200; corpo: %s", rec.Code, rec.Body.String())
	}
	utilizador := primeiroDaListagem(t, rec)

	quero := map[string]any{
		"id":              id,
		"name":            "alice",
		"webhook":         "http://webhook.exemplo",
		"jid":             "",
		"connected":       true,
		"logged_in":       true,
		"expiration":      float64(3600),
		"events":          "Message",
		"hmac_configured": false, // a listagem não lê a coluna hmac_key
		"token":           "",    // sec/F20: a listagem nunca devolve o token
	}
	for chave, esperado := range quero {
		if got := utilizador[chave]; got != esperado {
			t.Errorf("%s = %#v, quero %#v", chave, got, esperado)
		}
	}

	proxy, _ := utilizador["proxy_config"].(map[string]any)
	if proxy == nil {
		t.Fatal("proxy_config ausente")
	}
	for chave, esperado := range map[string]any{
		"enabled": true, "proxy_url": "http://proxy.exemplo", "webhook_use_proxy": false,
	} {
		if got := proxy[chave]; got != esperado {
			t.Errorf("proxy_config.%s = %#v, quero %#v", chave, got, esperado)
		}
	}

	s3, _ := utilizador["s3_config"].(map[string]any)
	if s3 == nil {
		t.Fatal("s3_config ausente")
	}
	for chave, esperado := range map[string]any{
		"enabled": true, "endpoint": "http://s3.exemplo", "region": "us-east-1",
		"bucket": "balde", "path_style": true, "public_url": "http://cdn.exemplo",
		"media_delivery": "base64", "retention_days": float64(7),
	} {
		if got := s3[chave]; got != esperado {
			t.Errorf("s3_config.%s = %#v, quero %#v", chave, got, esperado)
		}
	}
}

// TestAdminUsers_ContratoPublico_NenhumSegredoNoCorpo é o limite que a
// remoção do `access_key` mascarado abriu: o corpo servido não pode conter
// nem a chave de acesso nem a secreta, nem sequer mascaradas — uma máscara
// re-enviada num PUT gravaria "***" na coluna real, agora que os nomes de
// pedido e de resposta coincidem.
func TestAdminUsers_ContratoPublico_NenhumSegredoNoCorpo(t *testing.T) {
	f := newAdminFixture(t)
	id := idDoCorpo(t, f.do(t, http.MethodPost, "/admin/users", corpoDeCriacao))

	rec := f.do(t, http.MethodGet, "/admin/users/"+id, "")
	corpo := rec.Body.String()
	for _, proibido := range []string{"AKIA", "segredo", `"***"`} {
		if strings.Contains(corpo, proibido) {
			t.Errorf("o corpo contém %q: %s", proibido, corpo)
		}
	}
	contracttest.AssertNoKeys(t, rec.Body.Bytes(), "access_key", "secret_key")
}

// TestAdminUsers_ContratoPublico_ListagemVaziaEhArray trava a decisão do
// apresentador sobre colecção vazia: `[]`, e nunca `null`. O use case
// devolvia nil numa instalação sem utilizadores, e só uma das duas formas se
// percorre sem verificação.
func TestAdminUsers_ContratoPublico_ListagemVaziaEhArray(t *testing.T) {
	f := newAdminFixture(t)

	rec := f.do(t, http.MethodGet, "/admin/users", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200; corpo: %s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Data any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("corpo inválido: %v", err)
	}
	lista, ok := envelope.Data.([]any)
	if !ok || len(lista) != 0 {
		t.Fatalf("data = %#v, quero []", envelope.Data)
	}
	if !strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Errorf("o corpo não traz `[]` literal: %s", rec.Body.String())
	}
}

// TestAdminUsers_ContratoPublico_CamposVaziosNaoSomem é a metade do corte que
// o `omitempty` escondia: `jid`, `qrcode`, `logged_in`, `expiration`,
// `events` e `hmac_configured` desapareciam do corpo quando vazios, e o
// cliente não conseguia distinguir "este utilizador não tem JID" de "esta
// versão deixou de mandar a chave".
func TestAdminUsers_ContratoPublico_CamposVaziosNaoSomem(t *testing.T) {
	f := newAdminFixture(t)
	// O mínimo aceite pela rota: tudo o resto fica no zero.
	rec := f.do(t, http.MethodPost, "/admin/users", `{"name":"bob","token":"tok-bob","engine":"wa_noise"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200; corpo: %s", rec.Code, rec.Body.String())
	}
	corpo := objetoData(t, rec)
	for _, chave := range []string{
		"id", "name", "token", "webhook", "jid", "qrcode", "connected",
		"logged_in", "expiration", "events", "hmac_configured",
		"proxy_config", "s3_config",
	} {
		if _, presente := corpo[chave]; !presente {
			t.Errorf("%s ausente: a chave tem de existir mesmo com valor zero", chave)
		}
	}
	s3, _ := corpo["s3_config"].(map[string]any)
	if s3 == nil {
		t.Fatal("s3_config ausente")
	}
	for _, chave := range []string{
		"enabled", "endpoint", "region", "bucket", "path_style",
		"public_url", "media_delivery", "retention_days",
	} {
		if _, presente := s3[chave]; !presente {
			t.Errorf("s3_config.%s ausente", chave)
		}
	}
}

// TestAdminUsers_ContratoPublico_EdicaoLeOsNomesCanonicos é o teste do
// CAMINHO DE SUCESSO do DTO de pedido: um PUT com os nomes novos tem de
// CHEGAR ao banco. Afirmar só que a resposta é 200 mediria a guarda — o PUT
// que ignorava o corpo em silêncio devolvia 200 na mesma (F210).
func TestAdminUsers_ContratoPublico_EdicaoLeOsNomesCanonicos(t *testing.T) {
	f := newAdminFixture(t)
	id := idDoCorpo(t, f.do(t, http.MethodPost, "/admin/users", corpoDeCriacao))

	rec := f.do(t, http.MethodPut, "/admin/users/"+id,
		`{"name":"alice-editada","s3_config":{"enabled":true,"bucket":"balde-novo"},`+
			`"proxy_config":{"enabled":true,"proxy_url":"http://proxy-novo"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200; corpo: %s", rec.Code, rec.Body.String())
	}

	depois := primeiroDaListagem(t, f.do(t, http.MethodGet, "/admin/users/"+id, ""))
	if depois["name"] != "alice-editada" {
		t.Errorf("name = %#v: o corpo do PUT não chegou ao banco", depois["name"])
	}
	s3, _ := depois["s3_config"].(map[string]any)
	if s3 == nil || s3["bucket"] != "balde-novo" {
		t.Errorf("s3_config = %#v: `s3_config` não foi lido", s3)
	}
	proxy, _ := depois["proxy_config"].(map[string]any)
	if proxy == nil || proxy["proxy_url"] != "http://proxy-novo" {
		t.Errorf("proxy_config = %#v: `proxy_config` não foi lido", proxy)
	}
}

// TestAdminUsers_ContratoPublico_RemocaoCompletaTrazDetalhe: o `details` era
// calculado pelo use case e descartado pelo handler, que servia só `rsp.Data`.
func TestAdminUsers_ContratoPublico_RemocaoCompletaTrazDetalhe(t *testing.T) {
	f := newAdminFixture(t)
	id := idDoCorpo(t, f.do(t, http.MethodPost, "/admin/users", corpoDeCriacao))

	rec := f.do(t, http.MethodDelete, "/admin/users/"+id+"/full", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200; corpo: %s", rec.Code, rec.Body.String())
	}
	corpo := objetoData(t, rec)
	if corpo["id"] != id {
		t.Errorf("id = %#v, quero %q", corpo["id"], id)
	}
	if corpo["details"] != "user instance removed completely" {
		t.Errorf("details = %#v", corpo["details"])
	}
}

// --- auxiliares -----------------------------------------------------------

// objetoData devolve `data` como objecto.
func objetoData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
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

// idDoCorpo lê `data.id` da resposta de criação.
func idDoCorpo(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("criação falhou com %d: %s", rec.Code, rec.Body.String())
	}
	id, _ := objetoData(t, rec)["id"].(string)
	if id == "" {
		t.Fatalf("data.id vazio: %s", rec.Body.String())
	}
	return id
}

// primeiroDaListagem lê o primeiro utilizador de `data`, que é um ARRAY mesmo
// quando a rota é a de um utilizador só — GET /admin/users/{id} devolve uma
// listagem filtrada, e não um objecto.
func primeiroDaListagem(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("corpo inválido: %v\n%s", err, rec.Body.String())
	}
	if len(envelope.Data) != 1 {
		t.Fatalf("data tem %d elementos, quero 1: %s", len(envelope.Data), rec.Body.String())
	}
	return envelope.Data[0]
}
