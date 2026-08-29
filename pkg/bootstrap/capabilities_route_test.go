package bootstrap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/justinas/alice"
	"github.com/patrickmn/go-cache"

	"wa-api/pkg/capabilityregistry"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/db"
	"wa-api/pkg/presentation/http/handlers"
)

// Itens 55/56 do prompt arquitetural — GET /session/capabilities e GET
// /admin/capabilities.
//
// Como o resto deste arquivo de testes de rota, exercitam a ROTA REGISTRADA
// (registerCustomRoutes / registerAdminRoutes + gorilla/mux), com o mesmo
// authAlice/authAdmin de produção — não o handler cru.

const (
	capRouteTokenNoise    = "token-cap-wa-noise"
	capRouteUserNoise     = "user-cap-wa-noise"
	capRouteTokenHeadless = "token-cap-wa-headless"
	capRouteUserHeadless  = "user-cap-wa-headless"
	capRouteAdminToken    = "admin-token-cap"
)

// capabilitiesFixture monta as duas famílias de rota (session autenticada
// via authAlice, admin via authAdmin) sobre o MESMO customHandlers, como
// buildRouter faz em produção.
type capabilitiesFixture struct {
	db     *sqlx.DB
	router *mux.Router
}

func (f *capabilitiesFixture) seedUser(t *testing.T, id, token string, engine domain.Engine) {
	t.Helper()
	if _, err := f.db.Exec(f.db.Rebind(
		`INSERT INTO users (id, name, token, token_hash, history, proxy_url, engine) VALUES (?, ?, ?, ?, ?, ?, ?)`),
		id, "tenant "+id, token, domain.HashToken(token), 0, "", string(engine)); err != nil {
		t.Fatalf("seed user %s: %v", id, err)
	}
}

func (f *capabilitiesFixture) do(t *testing.T, method, target, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	if token != "" {
		req.Header.Set("token", token)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func (f *capabilitiesFixture) doAdmin(t *testing.T, method, target, authHeader string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

type sessionCapabilitiesEnvelope struct {
	Data struct {
		AccountType  string            `json:"account_type"`
		Capabilities map[string]string `json:"capabilities"`
	} `json:"data"`
}

// TESTE 1 — GET /session/capabilities sem autenticação: 401, nenhuma engine
// exposta.
func TestCapabilitiesRoute_Session_SemAuth401(t *testing.T) {
	f := newCapabilitiesFixtureReal(t)
	rec := f.do(t, http.MethodGet, "/session/capabilities", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, quero 401 (corpo: %s)", rec.Code, rec.Body.String())
	}
}

// TESTE 2 — GET /session/capabilities autenticado com sessão wa_noise vs
// wa_headless: mapas DIFERENTES, refletindo a engine certa.
//
// Casos concretos, medidos no matrix.go (não suposição):
//   - send_carousel é supported no wa_noise (F216, confirmado).
//   - set_group_photo é engine_unsupported no wa_headless (H140, confirmado).
func TestCapabilitiesRoute_Session_RefleteAEngineDaSessao(t *testing.T) {
	f := newCapabilitiesFixtureReal(t)

	recNoise := f.do(t, http.MethodGet, "/session/capabilities", capRouteTokenNoise)
	if recNoise.Code != http.StatusOK {
		t.Fatalf("wa_noise: status = %d, quero 200 (corpo: %s)", recNoise.Code, recNoise.Body.String())
	}
	var envNoise sessionCapabilitiesEnvelope
	if err := json.Unmarshal(recNoise.Body.Bytes(), &envNoise); err != nil {
		t.Fatalf("decode wa_noise: %v (corpo: %s)", err, recNoise.Body.String())
	}
	if envNoise.Data.AccountType != domain.AccountTypeUnknown.String() {
		t.Errorf("account_type = %q, quero %q (detecção real ainda não existe)", envNoise.Data.AccountType, domain.AccountTypeUnknown.String())
	}
	if got := envNoise.Data.Capabilities[domain.CapSendCarousel.String()]; got != domain.StatusSupported.String() {
		t.Errorf("wa_noise send_carousel = %q, quero %q (F216)", got, domain.StatusSupported.String())
	}

	recHeadless := f.do(t, http.MethodGet, "/session/capabilities", capRouteTokenHeadless)
	if recHeadless.Code != http.StatusOK {
		t.Fatalf("wa_headless: status = %d, quero 200 (corpo: %s)", recHeadless.Code, recHeadless.Body.String())
	}
	var envHeadless sessionCapabilitiesEnvelope
	if err := json.Unmarshal(recHeadless.Body.Bytes(), &envHeadless); err != nil {
		t.Fatalf("decode wa_headless: %v (corpo: %s)", err, recHeadless.Body.String())
	}
	if got := envHeadless.Data.Capabilities[domain.CapSetGroupPhoto.String()]; got != domain.StatusEngineUnsupported.String() {
		t.Errorf("wa_headless set_group_photo = %q, quero %q (H140)", got, domain.StatusEngineUnsupported.String())
	}

	if len(envNoise.Data.Capabilities) != len(envHeadless.Data.Capabilities) {
		t.Fatalf("tamanhos diferentes: wa_noise=%d wa_headless=%d — as duas devem cobrir as mesmas 88 capabilities",
			len(envNoise.Data.Capabilities), len(envHeadless.Data.Capabilities))
	}
	if envNoise.Data.Capabilities[domain.CapSendCarousel.String()] == envHeadless.Data.Capabilities[domain.CapSendCarousel.String()] {
		t.Errorf("send_carousel veio igual nas duas engines (%q) — o mapa não está refletindo a engine da sessão",
			envNoise.Data.Capabilities[domain.CapSendCarousel.String()])
	}
}

// TESTE 3 — nenhuma capability em status "unknown" faz a rota travar/panicar.
// unknown é o caso mais comum hoje (~40 capabilities no wa_headless).
func TestCapabilitiesRoute_Session_StatusUnknownNaoPanica(t *testing.T) {
	f := newCapabilitiesFixtureReal(t)
	rec := f.do(t, http.MethodGet, "/session/capabilities", capRouteTokenHeadless)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	var env sessionCapabilitiesEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v (corpo: %s)", err, rec.Body.String())
	}
	unknownCount := 0
	for _, status := range env.Data.Capabilities {
		if status == domain.StatusUnknown.String() {
			unknownCount++
		}
	}
	if unknownCount == 0 {
		t.Fatalf("esperava capabilities unknown no wa_headless (linha de base do matrix), veio 0 — a fixture mudou?")
	}
}

// TESTE 4 — GET /admin/capabilities sem token admin: recusado.
func TestCapabilitiesRoute_Admin_SemAuthRecusado(t *testing.T) {
	f := newCapabilitiesFixtureReal(t)
	rec := f.doAdmin(t, http.MethodGet, "/admin/capabilities", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, quero 401 (corpo: %s)", rec.Code, rec.Body.String())
	}
}

// TESTE 5 — GET /admin/capabilities com token errado: recusado.
func TestCapabilitiesRoute_Admin_TokenErradoRecusado(t *testing.T) {
	f := newCapabilitiesFixtureReal(t)
	rec := f.doAdmin(t, http.MethodGet, "/admin/capabilities", "token-errado")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, quero 401 (corpo: %s)", rec.Code, rec.Body.String())
	}
}

type adminCapabilitiesEnvelope struct {
	Data struct {
		Capabilities []struct {
			Capability  string `json:"capability"`
			Engine      string `json:"engine"`
			AccountType string `json:"account_type"`
			Supported   bool   `json:"supported"`
			Status      string `json:"status"`
			Reason      string `json:"reason"`
			Evidence    string `json:"evidence"`
		} `json:"capabilities"`
		Total int `json:"total"`
	} `json:"data"`
}

// TESTE 6 — GET /admin/capabilities com token admin: contém as 88
// capabilities x 2 engines x 3 account types, com reason/evidence
// preenchidos, e casos concretos conhecidos.
func TestCapabilitiesRoute_Admin_ComAuthContemAMatrizCompleta(t *testing.T) {
	f := newCapabilitiesFixtureReal(t)
	rec := f.doAdmin(t, http.MethodGet, "/admin/capabilities", capRouteAdminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	var env adminCapabilitiesEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v (corpo: %s)", err, rec.Body.String())
	}

	wantTotal := len(capabilityregistry.PortCoverage)
	// PortCoverage tem uma entrada por (port method, capability); capabilities
	// distintas podem ser menos que len(PortCoverage) só se dois métodos
	// mapeassem para a mesma capability, o que não é o caso hoje (1:1 — ver
	// domain/capability.go). Multiplicamos por 2 engines x 3 account types.
	distinct := map[string]bool{}
	for _, c := range capabilityregistry.PortCoverage {
		distinct[c.String()] = true
	}
	wantTotal = len(distinct) * 2 * 3
	if env.Data.Total != wantTotal || len(env.Data.Capabilities) != wantTotal {
		t.Fatalf("total = %d (len=%d), quero %d (%d capabilities x 2 engines x 3 account types)",
			env.Data.Total, len(env.Data.Capabilities), wantTotal, len(distinct))
	}

	// A EVIDÊNCIA confirmada (F216/H140) só vale para a linha
	// account_type=unknown: matrix.expand rebaixa personal/business para
	// not_tested deliberadamente (ver matrix.go, doc de expand) — account-type
	// detection ainda não existe, então nada testou essas duas linhas
	// especificamente. Filtrar por unknown é o que torna esta asserção capaz
	// de distinguir "a evidência sumiu" de "a evidência foi rebaixada como
	// esperado para um tipo de conta que ninguém mediu".
	foundConfirmedSupported := false
	foundEngineUnsupported := false
	for _, row := range env.Data.Capabilities {
		if row.AccountType != domain.AccountTypeUnknown.String() {
			continue
		}
		if row.Capability == domain.CapSendCarousel.String() && row.Engine == domain.EngineNoise.String() {
			if row.Status != domain.StatusSupported.String() || row.Evidence != domain.EvidenceConfirmed.String() {
				t.Errorf("send_carousel/wa_noise/unknown = status=%q evidence=%q, quero supported/confirmed (F216)", row.Status, row.Evidence)
			}
			if row.Reason == "" {
				t.Error("send_carousel/wa_noise veio sem reason — a superfície admin precisa dele")
			}
			foundConfirmedSupported = true
		}
		if row.Capability == domain.CapSetGroupPhoto.String() && row.Engine == domain.EngineHeadless.String() {
			if row.Status != domain.StatusEngineUnsupported.String() || row.Evidence != domain.EvidenceConfirmed.String() {
				t.Errorf("set_group_photo/wa_headless/unknown = status=%q evidence=%q, quero engine_unsupported/confirmed (H140)", row.Status, row.Evidence)
			}
			foundEngineUnsupported = true
		}
	}
	if !foundConfirmedSupported {
		t.Error("não encontrei a linha send_carousel/wa_noise na matriz")
	}
	if !foundEngineUnsupported {
		t.Error("não encontrei a linha set_group_photo/wa_headless na matriz")
	}
}

// TESTE 7 — status "unknown" na matriz do admin também não trava/panica: a
// própria existência da resposta 200 no teste 6 já prova isso (rows com
// unknown estão misturadas na mesma resposta), mas este teste isola a
// asserção para que uma regressão futura aponte direto para ela.
func TestCapabilitiesRoute_Admin_StatusUnknownNaoPanica(t *testing.T) {
	f := newCapabilitiesFixtureReal(t)
	rec := f.doAdmin(t, http.MethodGet, "/admin/capabilities", capRouteAdminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	var env adminCapabilitiesEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v (corpo: %s)", err, rec.Body.String())
	}
	unknownCount := 0
	for _, row := range env.Data.Capabilities {
		if row.Status == domain.StatusUnknown.String() {
			unknownCount++
		}
	}
	if unknownCount == 0 {
		t.Fatalf("esperava linhas unknown na matriz completa, veio 0 — a fixture mudou?")
	}
}

// newCapabilitiesFixtureReal monta authAlice com o mesmo cache global
// (appCtx.UserInfoCache-backed userinfocache) que o resto dos testes de rota
// deste pacote usa, restaurando-o depois — ver session_config_route_test.go
// para o mesmo padrão.
func newCapabilitiesFixtureReal(t *testing.T) *capabilitiesFixture {
	t.Helper()

	anteriorCtx := appCtx
	appCtx = NewAppContext()
	t.Cleanup(func() { appCtx = anteriorCtx })

	anteriorCache := userinfocache
	userinfocache = cache.New(5*time.Minute, 10*time.Minute)
	t.Cleanup(func() { userinfocache = anteriorCache })

	database := newChatHistoryDB(t)
	userRepo := db.NewUserRepository(database)

	ch := emptyCustomHandlers()
	ch.Capability = handlers.NewCapabilityHandlers(userRepo, capabilityregistry.NewCapabilityRegistry())
	ch.AdminToken = capRouteAdminToken

	router := mux.NewRouter()

	adminRoutes := router.PathPrefix("/admin").Subrouter()
	adminRoutes.Use(authAdmin(capRouteAdminToken))
	registerAdminRoutes(adminRoutes, ch)

	registerCustomRoutes(router, alice.New(authAlice(database.DB, userinfocache, nil)), ch)

	f := &capabilitiesFixture{db: database, router: router}
	f.seedUser(t, capRouteUserNoise, capRouteTokenNoise, domain.EngineNoise)
	f.seedUser(t, capRouteUserHeadless, capRouteTokenHeadless, domain.EngineHeadless)
	return f
}
