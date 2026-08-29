package handlers

import (
	"context"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/capabilityregistry"
	"wa-api/pkg/domain"
	"wa-api/pkg/pairing"
	"wa-api/pkg/qrimage"
)

// The harness the engine-explicit pairing tests share.
//
// # Why the capability registry is the REAL one
//
// Every registry built here uses capabilityregistry.NewCapabilityRegistry() —
// the production matrix, not a permissive stand-in. It is the single most
// important property of this harness: a fake matrix that answered "supported"
// for everything would bless a wa_headless pairing path that does not exist,
// and the test asserting 422 would be asserting the fake. ARMADILHAS.md #1.
//
// The consequence is that the wa_headless expectations in these tests are
// MEASUREMENTS of the real matrix. Since 2026-08-29 (HOUSEKEEP H145) that
// matrix marks get_pairing_qr and connect_session as Supported for
// wa_headless — pkg/infra/wa-headless/pairing.QRReader/Starter exist and are
// wired in production — so requests naming wa_headless for those two now
// reach the headless spy instead of being refused. request_pairing_code
// remains unknown; a phone-pairing test still expects 422.

const (
	// qrCodeNoise / qrCodeHeadless são os códigos de pareamento que cada spy
	// representa. Nomeados porque cada um aparece na semeadura E na asserção,
	// e literal repetido é o mesmo bug à espera de divergir (ADR-0004).
	qrCodeNoise    = "2@noise"
	qrCodeHeadless = "2@headless"

	// qrCodePersistido é o código que os testes da família /session usam
	// quando o engine não é o ponto — só a forma da resposta é.
	qrCodePersistido = "2@codigo-de-pareamento"
)

// qrImageOf é a imagem que `GET /session/pair/qr` responde para `code`,
// construída com o codificador DA PRODUÇÃO (pkg/qrimage) e não com um literal
// escrito à mão.
//
// Importa qual: a rota documenta uma imagem em data URI
// (api/openapi/schemas/sessao.yaml), e a F373 mediu o custo de um consumidor
// que assumiu outra coisa — desenhou o data URI como PAYLOAD de QR e produziu
// um código impecável que o WhatsApp recusou. Uma constante à mão aqui seria
// um dublê que não atravessa a transformação do caminho real.
func qrImageOf(t *testing.T, code string) string {
	t.Helper()
	s, err := qrimage.Encode(code)
	if err != nil {
		t.Fatalf("qrimage.Encode(%q) = %v", code, err)
	}
	return s
}

// pairingSpy is a provider port that records every call.
//
// One type implements all three ports, so a single spy can be handed to a
// provider as QR reader, phone pairer and starter at once. That is what makes
// "zero calls to ANY provider" a one-line assertion instead of three that can
// drift apart.
type pairingSpy struct {
	engine domain.Engine
	calls  int
	qr     string
	code   string
	err    error
}

func (s *pairingSpy) EnsureSession(context.Context, string) error {
	s.calls++
	return s.err
}

func (s *pairingSpy) PairingQR(context.Context, string) (string, error) {
	s.calls++
	return s.qr, s.err
}

func (s *pairingSpy) IsPaired(context.Context, string) (bool, error) {
	s.calls++
	return false, s.err
}

func (s *pairingSpy) RequestPairingCode(context.Context, string, string) (string, error) {
	s.calls++
	return s.code, s.err
}

func (s *pairingSpy) CheckOwnership(context.Context, string) error {
	s.calls++
	return s.err
}

func (s *pairingSpy) StartSession(context.Context, string, string) { s.calls++ }

// providerFor builds a pairing.Provider whose three ports are all this spy.
func (s *pairingSpy) provider() *pairing.Provider {
	return &pairing.Provider{
		Engine:      s.engine,
		QRReader:    s,
		PhonePairer: s,
		Starter:     s,
	}
}

// sessionRow is one persisted session record: the id and the engine it was
// CREATED with. The engine column is what pairing resolution reads, and reading
// it from anywhere else is the defect these tests exist to trap.
type sessionRow struct {
	id     string
	engine domain.Engine
}

// usersFor builds the persisted-session double: id -> engine, and nothing
// else. Its rule is the REAL one — pairing.Registry.TargetEngine reads
// entries[0].Engine from appport.UserRepository.ListUsers, and an empty slice
// means "no such session" — so the double answers the same two shapes the
// production repository answers.
func usersFor(rows ...sessionRow) *contractsfake.UserRepository {
	byID := make(map[string]domain.Engine, len(rows))
	for _, r := range rows {
		byID[r.id] = r.engine
	}
	return &contractsfake.UserRepository{
		ListUsersFunc: func(_ context.Context, id string) ([]domain.UserListEntry, error) {
			engine, ok := byID[id]
			if !ok {
				return nil, nil
			}
			return []domain.UserListEntry{{ID: id, Engine: engine, QRCode: "qr-" + id}}, nil
		},
	}
}

// capabilityRegistryForTest is the PRODUCTION matrix, named so the choice is
// visible at every call site. See this file's doc comment on why it is never a
// permissive stand-in.
func capabilityRegistryForTest() *capabilityregistry.CapabilityRegistry {
	return capabilityregistry.NewCapabilityRegistry()
}

// pairingHarness builds a registry over a set of persisted session rows and a
// spy per engine, and hands back both so a test can assert call counts.
type pairingHarness struct {
	registry *pairing.Registry
	noise    *pairingSpy
	headless *pairingSpy
}

// newPairingHarness wires the registry the handlers will use.
//
// Both engines get a FULLY WIRED spy provider, including wa_headless — which
// this build has no real adapter for. That is deliberate and is the sharper
// version of the test: if wa_headless is refused, it must be refused by the
// capability matrix and not by the accident of an unwired port. A harness that
// left the headless ports nil would pass the same assertions for the wrong
// reason (engine_unavailable instead of capability_not_supported), and would
// stop noticing the day somebody marked the capability supported by mistake.
func newPairingHarness(t *testing.T, rows ...sessionRow) *pairingHarness {
	t.Helper()

	// Os dois spies devolvem FORMAS DIFERENTES de propósito, porque os dois
	// adapters reais devolvem formas diferentes — e foi ignorar isso que
	// custou a F373. wa_noise lê users.qrcode, onde o orquestrador já gravou
	// o PNG codificado (pkg/application/session/orchestrator.go, onPairingQR:
	// "A coluna guarda a IMAGEM"); wa_headless lê a string crua da página
	// (pkg/infra/wa-headless/pairing/qr.go). Um dublê que devolvesse a mesma
	// forma pelos dois nunca exercitaria a normalização que existe justamente
	// porque elas divergem.
	noise := &pairingSpy{engine: domain.EngineNoise, qr: qrImageOf(t, qrCodeNoise), code: "NOISE-CODE"}
	headless := &pairingSpy{engine: domain.EngineHeadless, qr: qrCodeHeadless, code: "HEADLESS-CODE"}

	return &pairingHarness{
		registry: pairing.NewRegistry(usersFor(rows...), capabilityRegistryForTest(),
			noise.provider(), headless.provider()),
		noise:    noise,
		headless: headless,
	}
}

// assertOnlyNoiseCalled fails unless the wa-noise spy was touched and the
// wa-headless one was not.
func (h *pairingHarness) assertOnlyNoiseCalled(t *testing.T) {
	t.Helper()
	if h.noise.calls == 0 {
		t.Errorf("wa_noise provider calls = 0, want >0 — the request named wa_noise and nothing served it")
	}
	if h.headless.calls != 0 {
		t.Errorf("wa_headless provider calls = %d, want 0 — a wa_noise request reached the other engine (cross-engine fallback is forbidden)", h.headless.calls)
	}
}

// assertOnlyHeadlessCalled fails unless the wa-headless spy was touched and
// the wa-noise one was not — the mirror of assertOnlyNoiseCalled, added
// 2026-08-29 (HOUSEKEEP H145) once get_pairing_qr/connect_session became
// reachable for wa_headless.
func (h *pairingHarness) assertOnlyHeadlessCalled(t *testing.T) {
	t.Helper()
	if h.headless.calls == 0 {
		t.Errorf("wa_headless provider calls = 0, want >0 — the request named wa_headless and nothing served it")
	}
	if h.noise.calls != 0 {
		t.Errorf("wa_noise provider calls = %d, want 0 — a wa_headless request reached the other engine (cross-engine fallback is forbidden)", h.noise.calls)
	}
}

// assertNoProviderCalled fails unless BOTH spies are untouched. It is the
// assertion behind every refusal in pkg/pairing: the request must die before a
// provider is reached, not after.
func (h *pairingHarness) assertNoProviderCalled(t *testing.T) {
	t.Helper()
	if h.noise.calls != 0 || h.headless.calls != 0 {
		t.Errorf("provider calls: wa_noise=%d wa_headless=%d, want 0/0 — the request was refused AFTER touching a provider",
			h.noise.calls, h.headless.calls)
	}
}
