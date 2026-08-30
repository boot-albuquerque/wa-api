package pairing

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/capabilityregistry"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// registry_test.go — a ORDEM das quatro perguntas, medida uma a uma.
//
// Os testes de fronteira (pkg/presentation/http/handlers) medem o RESULTADO
// pela rota registada. Este ficheiro mede a coisa que o resultado não mostra:
// que a resposta é a da PRIMEIRA pergunta a falhar, e não a de qualquer uma
// que também falharia.
//
// Isso importa porque a ordem é o que impede o pior caso. Se a leitura da
// sessão alvo corresse antes do parsing do engine, um pedido sem engine
// tocaria no banco; se a decisão de capacidade corresse depois da resolução do
// provider, um pedido para um engine que não serve a operação chegaria ao
// transporte. Nos dois casos o status final até podia ser o mesmo.

const (
	noiseID    = "sess-noise"
	headlessID = "sess-headless"
)

// engineReaderSpy conta as leituras da sessão alvo, para que "não se leu o
// banco" seja uma asserção e não uma esperança.
type engineReaderSpy struct {
	rows  map[string]domain.Engine
	calls int
	err   error
}

func (s *engineReaderSpy) ListUsers(_ context.Context, id string) ([]domain.UserListEntry, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	engine, ok := s.rows[id]
	if !ok {
		return nil, nil
	}
	return []domain.UserListEntry{{ID: id, Engine: engine}}, nil
}

func newReader() *engineReaderSpy {
	return &engineReaderSpy{rows: map[string]domain.Engine{
		noiseID:    domain.EngineNoise,
		headlessID: domain.EngineHeadless,
	}}
}

// registryWith builds a registry over the PRODUCTION capability matrix. A
// permissive stand-in would bless a headless pairing path that does not
// exist — ARMADILHAS.md #1.
func registryWith(reader SessionEngineReader, providers ...*Provider) *Registry {
	return NewRegistry(reader, capabilityregistry.NewCapabilityRegistry(), providers...)
}

func bothEnginesWired() []*Provider {
	return []*Provider{
		{Engine: domain.EngineNoise},
		{Engine: domain.EngineHeadless},
	}
}

func assertCode(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("erro = nil, quero o código %q", want)
	}
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("erro %v não é um *apperr.AppError — a fronteira HTTP deriva o status da categoria, "+
			"e um erro cru sairia 500", err)
	}
	if appErr.Code != want {
		t.Fatalf("code = %q, quero %q (mensagem: %s)", appErr.Code, want, appErr.Message)
	}
}

// TestResolve_InvalidEngineIsAnsweredBeforeReadingTheTargetSession é a
// asserção de ordem que mais custa se cair: sem ela, um pedido sem engine —
// que é o pedido de um cliente desactualizado, portanto frequente — pagaria uma
// ida ao banco por cada tentativa.
func TestResolve_InvalidEngineIsAnsweredBeforeReadingTheTargetSession(t *testing.T) {
	for _, raw := range []string{"", "foobar", "legacy_unknown", "wa_noise", "wa_headless", "NOISE"} {
		t.Run(raw, func(t *testing.T) {
			reader := newReader()
			r := registryWith(reader, bothEnginesWired()...)

			_, err := r.Resolve(context.Background(), noiseID, raw, domain.CapGetPairingQR)

			assertCode(t, err, CodeInvalidEngine)
			if reader.calls != 0 {
				t.Fatalf("a sessão alvo foi lida %d vez(es) para um pedido cujo engine nem é engine", reader.calls)
			}
		})
	}
}

// TestResolve_MismatchIsAnsweredBeforeTheCapabilityDecision: o pedido nomeia
// headless, o alvo é noise, e a capacidade PEDIDA é uma que headless
// também não serve. As duas recusas se aplicam; a que sai tem de ser a do
// engine, porque é a que diz ao cliente o que corrigir.
func TestResolve_MismatchIsAnsweredBeforeTheCapabilityDecision(t *testing.T) {
	r := registryWith(newReader(), bothEnginesWired()...)

	_, err := r.Resolve(context.Background(), noiseID, domain.EngineHeadless.String(), domain.CapGetPairingQR)

	assertCode(t, err, CodeEngineMismatch)
}

// TestResolve_CapabilityIsAnsweredBeforeTheProviderLookup: com o engine certo
// e a capacidade não servida, sai capability_not_supported — mesmo que o
// provider desse engine esteja registado e completo. Se a ordem se invertesse,
// um provider ligado por engano passaria a servir a operação.
//
// domain.CapCheckPairingStatus (não domain.CapGetPairingQR nem
// domain.CapRequestPairingCode) pela mesma razão de
// TestResolve_NeverFallsBackToTheOtherEngine: desde H145 (2026-08-29)
// get_pairing_qr é Supported para headless, e desde F380 (mesmo dia)
// request_pairing_code também passou a Supported — as duas na matriz de
// produção. check_pairing_status continua unknown.
func TestResolve_CapabilityIsAnsweredBeforeTheProviderLookup(t *testing.T) {
	r := registryWith(newReader(), bothEnginesWired()...)

	_, err := r.Resolve(context.Background(), headlessID, domain.EngineHeadless.String(), domain.CapCheckPairingStatus)

	assertCode(t, err, CodeCapabilityNotSupported)
}

// TestResolve_UnregisteredEngineIsEngineUnavailable: o engine é válido, é o do
// alvo, a matriz serve a capacidade — e este processo não tem provider. É a
// única forma de chegar a engine_unavailable pela via do registo, e distingue
// "não dá" (do cliente) de "não está ligado aqui" (de quem opera).
func TestResolve_UnregisteredEngineIsEngineUnavailable(t *testing.T) {
	// Só o headless registado: um pedido noise válido não encontra provider.
	r := registryWith(newReader(), &Provider{Engine: domain.EngineHeadless})

	_, err := r.Resolve(context.Background(), noiseID, domain.EngineNoise.String(), domain.CapGetPairingQR)

	assertCode(t, err, CodeEngineUnavailable)
}

// TestResolve_NilPortIsEngineUnavailable: o provider existe e a porta pedida é
// nil. Tem de ser recusa explícita — nunca um nil que estoura no chamador.
func TestResolve_NilPortIsEngineUnavailable(t *testing.T) {
	r := registryWith(newReader(), &Provider{Engine: domain.EngineNoise})

	if _, err := r.ResolveQRReader(context.Background(), noiseID, domain.EngineNoise.String()); true {
		assertCode(t, err, CodeEngineUnavailable)
	}
	if _, err := r.ResolvePhonePairer(context.Background(), noiseID, domain.EngineNoise.String()); true {
		assertCode(t, err, CodeEngineUnavailable)
	}
	if _, err := r.ResolveStarter(context.Background(), noiseID, domain.EngineNoise.String()); true {
		assertCode(t, err, CodeEngineUnavailable)
	}
}

// TestResolve_UnknownTargetIsNoSession: um alvo sem linha é recusado com o
// MESMO código que GetQRUseCase e CapabilityHandlers já respondiam para o
// mesmo facto. Um segundo nome para um facto é contrato pior.
func TestResolve_UnknownTargetIsNoSession(t *testing.T) {
	r := registryWith(newReader(), bothEnginesWired()...)

	_, err := r.Resolve(context.Background(), "nao-existe", domain.EngineNoise.String(), domain.CapGetPairingQR)

	assertCode(t, err, CodeNoSession)
}

// TestResolve_RepositoryFailurePropagatesRaw: a falha do banco NÃO vira um
// apperr de validação. A fronteira decide o nível e o status pela taxonomia, e
// classificar uma indisponibilidade como erro do cliente fá-la-ia sair 400 em
// nível warn — indistinguível de um engine mal escrito (F90).
func TestResolve_RepositoryFailurePropagatesRaw(t *testing.T) {
	boom := errors.New("pq: too many connections")
	reader := newReader()
	reader.err = boom
	r := registryWith(reader, bothEnginesWired()...)

	_, err := r.Resolve(context.Background(), noiseID, domain.EngineNoise.String(), domain.CapGetPairingQR)

	if !errors.Is(err, boom) {
		t.Fatalf("a causa do repositório perdeu-se: %v", err)
	}
	var appErr *apperr.AppError
	if errors.As(err, &appErr) {
		t.Fatalf("a falha do banco foi classificada como %q/%q — ela não é erro do cliente",
			appErr.Code, appErr.Category)
	}
}

// TestTargetEngine_LegacyUnknownReadsAsNoise: uma linha que o backfill do
// arranque ainda não tocou não pode virar erro para o cliente. O valor
// devolvido é o mesmo que o backfill atribuiria, e é o que
// CapabilityHandlers.sessionEngine já faz para o mesmo estado.
func TestTargetEngine_LegacyUnknownReadsAsNoise(t *testing.T) {
	reader := &engineReaderSpy{rows: map[string]domain.Engine{"antiga": domain.EngineLegacyUnknown}}
	r := registryWith(reader, bothEnginesWired()...)

	got, err := r.TargetEngine(context.Background(), "antiga")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got != domain.EngineNoise {
		t.Fatalf("TargetEngine = %q, quero %q — legacy_unknown descreve migração, não é resposta ao cliente",
			got, domain.EngineNoise)
	}
}

// TestResolve_NeverFallsBackToTheOtherEngine é a regra geral do projeto, dita
// nesta superfície: com noise plenamente ligado e headless vazio para
// uma capacidade que a matriz ainda não marca como suportada, um pedido
// headless legítimo é RECUSADO, nunca servido pelo outro.
//
// domain.CapCheckPairingStatus (não domain.CapGetPairingQR nem
// domain.CapRequestPairingCode) é a capacidade usada aqui de propósito:
// desde H145 (2026-08-29) get_pairing_qr passou a Supported para
// headless na matriz de produção, e desde F380 (mesmo dia)
// request_pairing_code também — as duas testariam a matriz, não o "nunca
// cai para o outro engine" que este teste existe para travar.
// check_pairing_status continua unknown para headless.
//
// A asserção é sobre a IDENTIDADE do provider devolvido, e não sobre o status:
// um Resolve que devolvesse o provider errado com nil de erro passaria em
// qualquer teste que só olhasse para o erro.
func TestResolve_NeverFallsBackToTheOtherEngine(t *testing.T) {
	noise := &Provider{Engine: domain.EngineNoise}
	r := registryWith(newReader(), noise, &Provider{Engine: domain.EngineHeadless})

	got, err := r.Resolve(context.Background(), headlessID, domain.EngineHeadless.String(), domain.CapCheckPairingStatus)
	if err == nil {
		t.Fatalf("Resolve devolveu o provider %q sem erro para um engine que não serve esta capacidade", got.Engine)
	}
	if got != nil {
		t.Fatalf("Resolve devolveu provider %q junto com o erro — o chamador poderia usá-lo", got.Engine)
	}

	// E a metade positiva, para que o teste não passe por o Resolve recusar
	// tudo: o mesmo registry serve o noise.
	served, err := r.Resolve(context.Background(), noiseID, domain.EngineNoise.String(), domain.CapCheckPairingStatus)
	if err != nil {
		t.Fatalf("o pedido noise legítimo foi recusado: %v", err)
	}
	if served != noise {
		t.Fatalf("Resolve devolveu %+v, quero exatamente o provider noise registado", served)
	}
}
