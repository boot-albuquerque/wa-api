package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/session"
	"wa-api/pkg/domain"
	"wa-api/pkg/pairing"
)

// handler_pairing_engine_test.go — o defeito da F273 e a sua trava.
//
// O defeito medido: GetQR, Connect e PairPhone estavam ligados em
// pkg/bootstrap/wiring_handlers.go:165 a UM adaptador wa-noise fixo
// (wasession.NewSessionGuardAdapter), sem condicional nenhuma por engine. Uma
// sessão criada com engine=headless persistia certo, aparecia certo em
// GET /session/capabilities, e parava a parear pelo socket — porque nada entre
// o handler e o adaptador alguma vez leu a coluna.
//
// Os testes deste ficheiro medem a CAUSA e não o sintoma: não perguntam "que
// status saiu", perguntam QUAL PROVIDER FOI TOCADO. Um teste que só olhasse
// para o corpo da resposta passaria com o defeito no lugar, porque o adaptador
// errado responde 200 com a mesma forma.
//
// Todos correm pela ROTA REGISTADA (mux), e não pelo handler cru: o router é
// gorilla/mux, e uma rota montada sem padrão não exercita extração de
// parâmetro — foi assim que a F81 sobreviveu. Ver ARMADILHAS.md #2.

const (
	// noiseSession e headlessSession são duas sessões PERSISTIDAS, cada uma
	// com o seu engine na coluna users.engine.
	noiseSession    = "sess-noise"
	headlessSession = "sess-headless"

	// engineQuery e' o par nome=valor que as duas rotas GET da superfície de
	// pareamento aceitam.
	engineQueryNoise    = "?engine=noise"
	engineQueryHeadless = "?engine=headless"
)

// pairingRouter monta os três handlers de pareamento nas rotas registadas e
// injeta o ACTOR no contexto, como o middleware de autenticação faz.
//
// actorID é quem pede; o alvo sai da rota quando ela declara `{id}`, e do actor
// quando não declara — que é exatamente a regra de handlers.pairingTarget. As
// duas formas são registadas aqui de propósito: a self-service é a que existe
// em produção, e a de `{id}` é a que torna a distinção actor/alvo observável.
func pairingRouter(t *testing.T, h *pairingHarness, actorID string) *mux.Router {
	t.Helper()
	log := &contractsfake.Logger{}

	qr := NewGetQRHandler(log, h.registry)
	pair := NewPairPhoneHandler(log, h.registry)
	connect := NewConnectHandler(session.NewConnectUseCase(log), h.registry)

	inject := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(
				r.Context(), appport.UserInfoKey, sessionSecretUser{id: actorID})))
		})
	}

	router := mux.NewRouter()
	router.Handle("/session/qr", inject(qr)).Methods(http.MethodGet)
	router.Handle("/session/qr/{id}", inject(qr)).Methods(http.MethodGet)
	router.Handle("/session/pairphone", inject(pair)).Methods(http.MethodPost)
	router.Handle("/session/pairphone/{id}", inject(pair)).Methods(http.MethodPost)
	router.Handle("/session/connect", inject(connect)).Methods(http.MethodGet)
	router.Handle("/session/connect/{id}", inject(connect)).Methods(http.MethodGet)
	return router
}

func servePairing(t *testing.T, router *mux.Router, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

// --- 1 e 2: QR por engine ------------------------------------------------

// TestPairingQR_WaNoise_CallsOnlyNoiseProvider é a metade positiva do defeito:
// o pedido nomeia wa_noise, a sessão alvo é wa_noise, e SÓ o provider wa-noise
// é tocado.
func TestPairingQR_WaNoise_CallsOnlyNoiseProvider(t *testing.T) {
	h := newPairingHarness(t, sessionRow{noiseSession, domain.EngineNoise})
	rec := servePairing(t, pairingRouter(t, h, noiseSession), http.MethodGet, "/session/qr"+engineQueryNoise, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200 (corpo %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "2@noise") {
		t.Errorf("corpo = %s, quero o QR do provider wa_noise (2@noise)", rec.Body.String())
	}
	h.assertOnlyNoiseCalled(t)
}

// TestPairingQR_WaHeadless_CapabilityNotSupported é o caso REAL medido, e não
// o simétrico que o enunciado antecipava.
//
// Medição (2026-08-27): não existe implementação de PairingQRReader para
// wa_headless em lado nenhum da árvore, e nada de pkg/infra/wa-headless é
// sequer construído em pkg/bootstrap — `grep -rn NewDisconnector pkg/bootstrap`
// devolve zero fora de testes. A matriz de capacidades regista isso como
// not_implemented com evidência confirmada, e é a matriz REAL que este teste
// consulta (ver pairing_testkit_test.go).
//
// Por isso o esperado é 422 capability_not_supported — e, sobretudo, ZERO
// chamadas a QUALQUER provider. O provider wa_headless está totalmente ligado
// no harness: se a recusa viesse de uma porta nil em vez da matriz, este teste
// continuaria a passar pelo motivo errado, e é por isso que o harness o liga.
func TestPairingQR_WaHeadless_CapabilityNotSupported(t *testing.T) {
	h := newPairingHarness(t, sessionRow{headlessSession, domain.EngineWaHeadless})
	rec := servePairing(t, pairingRouter(t, h, headlessSession), http.MethodGet, "/session/qr"+engineQueryHeadless, "")

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, quero 422 (corpo %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), pairing.CodeCapabilityNotSupported) {
		t.Errorf("corpo = %s, quero o código %q", rec.Body.String(), pairing.CodeCapabilityNotSupported)
	}
	h.assertNoProviderCalled(t)
}

// --- 3 e 4: PairPhone por engine -----------------------------------------

func TestPairingPhone_WaNoise_CallsOnlyNoiseProvider(t *testing.T) {
	h := newPairingHarness(t, sessionRow{noiseSession, domain.EngineNoise})
	rec := servePairing(t, pairingRouter(t, h, noiseSession), http.MethodPost, "/session/pairphone",
		`{"engine":"noise","phone":"5511999999999"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200 (corpo %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "NOISE-CODE") {
		t.Errorf("corpo = %s, quero o código do provider wa_noise", rec.Body.String())
	}
	h.assertOnlyNoiseCalled(t)
}

// TestPairingPhone_WaHeadless_CapabilityNotSupported: mesma medição que o QR.
// A linha request_pairing_code da matriz marca wa_headless como `unknown`
// (pkg/capabilityregistry/matrix.go), e unknown NUNCA é Supported — a ausência
// de evidência é propagada, não engolida. 422, zero providers tocados.
func TestPairingPhone_WaHeadless_CapabilityNotSupported(t *testing.T) {
	h := newPairingHarness(t, sessionRow{headlessSession, domain.EngineWaHeadless})
	rec := servePairing(t, pairingRouter(t, h, headlessSession), http.MethodPost, "/session/pairphone",
		`{"engine":"headless","phone":"5511999999999"}`)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, quero 422 (corpo %s)", rec.Code, rec.Body.String())
	}
	h.assertNoProviderCalled(t)
}

// --- 6, 7, 8: engine ausente ou inválido ---------------------------------

// TestPairingQR_MissingEngine_400 e os dois seguintes travam a PRIMEIRA das
// quatro perguntas de pkg/pairing: sem engine não se lê sequer a sessão alvo.
func TestPairingQR_MissingEngine_400(t *testing.T) {
	h := newPairingHarness(t, sessionRow{noiseSession, domain.EngineNoise})
	rec := servePairing(t, pairingRouter(t, h, noiseSession), http.MethodGet, "/session/qr", "")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quero 400 (corpo %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), pairing.CodeInvalidEngine) {
		t.Errorf("corpo = %s, quero o código %q", rec.Body.String(), pairing.CodeInvalidEngine)
	}
	h.assertNoProviderCalled(t)
}

func TestPairingPhone_MissingEngine_400(t *testing.T) {
	h := newPairingHarness(t, sessionRow{noiseSession, domain.EngineNoise})
	rec := servePairing(t, pairingRouter(t, h, noiseSession), http.MethodPost, "/session/pairphone",
		`{"phone":"5511999999999"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quero 400 (corpo %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), pairing.CodeInvalidEngine) {
		t.Errorf("corpo = %s, quero o código %q", rec.Body.String(), pairing.CodeInvalidEngine)
	}
	h.assertNoProviderCalled(t)
}

// TestPairing_UnknownEngineValues_400 cobre o conjunto todo de valores que não
// são engine, um por um — `legacy_unknown` incluído como um valor qualquer
// que não é `noise` nem `headless`, não porque exista como estado nomeado no
// domínio desta worktree (não existe, ver pkg/domain/user.go: só há
// EngineNoise/EngineWaHeadless, sem legacy_unknown/backfill).
func TestPairing_UnknownEngineValues_400(t *testing.T) {
	// "noise%20" e' o valor com espaco a' direita, percent-encoded: o
	// parser do dominio nao apara nada, e um valor que precisa de ser
	// reparado e' um valor cujo autor nao sabe o que quis dizer.
	for _, raw := range []string{"foobar", "legacy_unknown", "NOISE", "noise%20", "wa_noise", "wa_headless", ""} {
		t.Run(raw, func(t *testing.T) {
			h := newPairingHarness(t, sessionRow{noiseSession, domain.EngineNoise})
			rec := servePairing(t, pairingRouter(t, h, noiseSession), http.MethodGet, "/session/qr?engine="+raw, "")

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("engine=%q: status = %d, quero 400 (corpo %s)", raw, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), pairing.CodeInvalidEngine) {
				t.Errorf("engine=%q: corpo = %s, quero o código %q", raw, rec.Body.String(), pairing.CodeInvalidEngine)
			}
			h.assertNoProviderCalled(t)
		})
	}
}

// --- 9: divergência entre o pedido e a sessão alvo ------------------------

// TestPairing_EngineMismatch_409: o pedido nomeia um engine REAL, e não o da
// sessão alvo. 409, e nenhum provider tocado.
//
// A redundância que este teste protege é deliberada: o engine já está gravado,
// e o pedido tem de o confirmar. Sem a confirmação, um cliente que acredita
// estar a parear em headless recebe 200 de um pareamento por socket, e só
// descobre pelo comportamento da sessão depois.
func TestPairing_EngineMismatch_409(t *testing.T) {
	h := newPairingHarness(t, sessionRow{noiseSession, domain.EngineNoise})
	rec := servePairing(t, pairingRouter(t, h, noiseSession), http.MethodGet, "/session/qr"+engineQueryHeadless, "")

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, quero 409 (corpo %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), pairing.CodeEngineMismatch) {
		t.Errorf("corpo = %s, quero o código %q", rec.Body.String(), pairing.CodeEngineMismatch)
	}
	h.assertNoProviderCalled(t)
}

// TestPairing_EngineMismatch_PairPhone_409 é o mesmo pela rota do corpo JSON —
// a ordem importa e é o que se está a travar: a recusa acontece ANTES da
// validação do telefone, então um corpo SEM telefone continua a dar 409 e não
// 400 missing_phone.
func TestPairing_EngineMismatch_PairPhone_409(t *testing.T) {
	h := newPairingHarness(t, sessionRow{headlessSession, domain.EngineWaHeadless})
	rec := servePairing(t, pairingRouter(t, h, headlessSession), http.MethodPost, "/session/pairphone",
		`{"engine":"noise"}`)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, quero 409 — a recusa de engine tem de correr ANTES da validação do telefone (corpo %s)",
			rec.Code, rec.Body.String())
	}
	h.assertNoProviderCalled(t)
}

// --- 10: o teste central — ACTOR não é ALVO -------------------------------

// TestPairing_UsesTargetEngineNotActorEngine é a asserção mais importante deste
// ficheiro, porque é a forma geral do defeito.
//
// O montagem: o token que autoriza pertence a uma sessão wa_noise (o ACTOR). A
// sessão a parear, identificada pela ROTA, é wa_headless (o ALVO), e o pedido
// nomeia wa_headless. Se a resolução lesse o engine do actor — do contexto, do
// cache do middleware, do token — ela concluiria wa_noise, veria divergência
// contra o pedido e responderia 409; ou pior, pararia a sessão do alvo pelo
// socket e responderia 200.
//
// O esperado é o engine do ALVO: wa_headless, que nesta build não serve QR, e
// portanto 422 capability_not_supported. Ou seja, o teste distingue TRÊS
// resultados, e só um deles é o correto:
//
//	409 engine_mismatch  -> leu o engine do ACTOR (o defeito)
//	200 com 2@noise      -> serviu pelo provider do ACTOR (o defeito, pior)
//	422 not_supported    -> leu o engine do ALVO (correto)
//
// É por isso que a divergência entre actor e alvo tem de ser observável para o
// teste existir. Ver handlers.pairingTarget: nenhuma rota registada em produção
// declara `{id}` hoje, e a distinção está escrita à mesma, porque a alternativa
// é redescobri-la — e voltar a errá-la — no dia em que uma rota de operador
// aparecer.
func TestPairing_UsesTargetEngineNotActorEngine(t *testing.T) {
	h := newPairingHarness(t,
		sessionRow{noiseSession, domain.EngineNoise},
		sessionRow{headlessSession, domain.EngineWaHeadless},
	)
	// O actor é a sessão wa_noise; o alvo, pela rota, é a wa_headless.
	router := pairingRouter(t, h, noiseSession)
	rec := servePairing(t, router, http.MethodGet, "/session/qr/"+headlessSession+engineQueryHeadless, "")

	if rec.Code == http.StatusConflict {
		t.Fatalf("status = 409 engine_mismatch: a resolução comparou o pedido contra o engine do ACTOR (%q) "+
			"em vez do engine do ALVO (%q). É exatamente o defeito da F273 (corpo %s)",
			domain.EngineNoise, domain.EngineWaHeadless, rec.Body.String())
	}
	if rec.Code == http.StatusOK {
		t.Fatalf("status = 200: o pedido foi servido, e o único provider capaz de servir QR nesta build é o do "+
			"ACTOR (wa_noise). O alvo é wa_headless (corpo %s)", rec.Body.String())
	}
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, quero 422 — o engine do ALVO é wa_headless, que não serve QR nesta build (corpo %s)",
			rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), pairing.CodeCapabilityNotSupported) {
		t.Errorf("corpo = %s, quero o código %q", rec.Body.String(), pairing.CodeCapabilityNotSupported)
	}
	h.assertNoProviderCalled(t)
}

// TestPairing_UsesTargetEngineNotActorEngine_Positive é a metade de SUCESSO da
// mesma distinção, e existe porque três defeitos deste repositório viviam atrás
// de suítes que só exercitavam a guarda (ARMADILHAS.md #2).
//
// Actor wa_headless, alvo wa_noise, pedido wa_noise: tem de servir, e tem de
// servir pelo provider do ALVO.
func TestPairing_UsesTargetEngineNotActorEngine_Positive(t *testing.T) {
	h := newPairingHarness(t,
		sessionRow{noiseSession, domain.EngineNoise},
		sessionRow{headlessSession, domain.EngineWaHeadless},
	)
	router := pairingRouter(t, h, headlessSession)
	rec := servePairing(t, router, http.MethodGet, "/session/qr/"+noiseSession+engineQueryNoise, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200 — o alvo é wa_noise e o pedido nomeia wa_noise (corpo %s)",
			rec.Code, rec.Body.String())
	}
	h.assertOnlyNoiseCalled(t)
}

// --- Connect ---------------------------------------------------------------

// TestPairingConnect_WaNoise_CallsOnlyNoiseProvider trava também a ORDEM que a
// F108 pagou para descobrir: CheckOwnership antes de StartSession. Inverter as
// duas devolve 200 na mesma e continua a passar em qualquer teste que só olhe
// para o status.
func TestPairingConnect_WaNoise_CallsOnlyNoiseProvider(t *testing.T) {
	h := newPairingHarness(t, sessionRow{noiseSession, domain.EngineNoise})

	starter := &contractsfake.SessionStarter{}
	h.registry = pairing.NewRegistry(
		usersFor(sessionRow{noiseSession, domain.EngineNoise}),
		capabilityRegistryForTest(),
		&pairing.Provider{Engine: domain.EngineNoise, Starter: starter, QRReader: h.noise, PhonePairer: h.noise},
		h.headless.provider(),
	)

	rec := servePairing(t, pairingRouter(t, h, noiseSession), http.MethodGet, "/session/connect"+engineQueryNoise, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200 (corpo %s)", rec.Code, rec.Body.String())
	}
	if len(starter.CheckOwnershipCalls) != 1 {
		t.Fatalf("CheckOwnership calls = %d, quero 1 (F108: a posse é verificada ANTES da resposta)", len(starter.CheckOwnershipCalls))
	}
	if len(starter.StartSessionCalls) != 1 {
		t.Fatalf("StartSession calls = %d, quero 1", len(starter.StartSessionCalls))
	}
	if h.headless.calls != 0 {
		t.Errorf("wa_headless provider calls = %d, quero 0", h.headless.calls)
	}
}

// TestPairingConnect_WaHeadless_CapabilityNotSupported: connect_session é
// not_implemented para wa_headless na matriz real (nada de wa-headless é
// construído em pkg/bootstrap). 422, e o starter nunca é chamado.
func TestPairingConnect_WaHeadless_CapabilityNotSupported(t *testing.T) {
	h := newPairingHarness(t, sessionRow{headlessSession, domain.EngineWaHeadless})
	rec := servePairing(t, pairingRouter(t, h, headlessSession), http.MethodGet, "/session/connect"+engineQueryHeadless, "")

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, quero 422 (corpo %s)", rec.Code, rec.Body.String())
	}
	h.assertNoProviderCalled(t)
}

// TestPairing_UnknownTargetSession_400: um alvo sem linha não é um alvo. A
// leitura acontece DEPOIS de o engine ser aceite e ANTES de qualquer provider.
func TestPairing_UnknownTargetSession_400(t *testing.T) {
	h := newPairingHarness(t, sessionRow{noiseSession, domain.EngineNoise})
	rec := servePairing(t, pairingRouter(t, h, noiseSession), http.MethodGet, "/session/qr/nao-existe"+engineQueryNoise, "")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quero 400 (corpo %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), pairing.CodeNoSession) {
		t.Errorf("corpo = %s, quero o código %q", rec.Body.String(), pairing.CodeNoSession)
	}
	h.assertNoProviderCalled(t)
}
