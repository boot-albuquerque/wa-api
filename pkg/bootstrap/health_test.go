package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ADR-0005 D6. O que estes testes travam é a SEPARAÇÃO: /health/live responde
// pelo processo e /health/ready responde por "consigo servir". Uma sonda que
// respondesse as duas coisas com o mesmo valor é exatamente o defeito que o
// Exp 1 mediu — processo saudável, sessão morta, banco dizendo connected=1.

type fakePinger struct{ err error }

func (f fakePinger) PingContext(context.Context) error { return f.err }

func TestReadiness_DatabaseDownIsNotReady(t *testing.T) {
	probe := buildReadinessProbe(fakePinger{err: errors.New("connection refused")}, nil)

	report := probe(context.Background())

	if report.Ready() {
		t.Fatal("pod reportado como pronto com o banco fora do ar; o k8s mandaria tráfego para quem não consegue servir sessão nenhuma")
	}
	if got := report.Checks["database"]; got != "unreachable" {
		t.Errorf("checks[database] = %q, want %q", got, "unreachable")
	}
}

// TestReadiness_ErrorDetailNeverLeaksToTheBody: a sonda é NÃO autenticada (o
// kubelet não carrega token), e erro de driver carrega host, porta e usuário.
// O detalhe vai para o log, que é autenticado; o corpo leva código.
func TestReadiness_ErrorDetailNeverLeaksToTheBody(t *testing.T) {
	secret := "dial tcp 10.0.0.7:5432: connect: connection refused (user=waapi)"
	probe := buildReadinessProbe(fakePinger{err: errors.New(secret)}, nil)

	body, _ := json.Marshal(probe(context.Background()))

	for _, leak := range []string{"10.0.0.7", "5432", "waapi", "dial tcp"} {
		if body := string(body); strings.Contains(body, leak) {
			t.Errorf("o corpo da sonda pública vazou %q: %s", leak, body)
		}
	}
}

func TestReadiness_HealthyDatabaseIsReady(t *testing.T) {
	probe := buildReadinessProbe(fakePinger{}, nil)

	report := probe(context.Background())

	if !report.Ready() {
		t.Fatalf("pod saudável reportado como não pronto: %+v", report)
	}
	if got := report.Checks["database"]; got != checkOK {
		t.Errorf("checks[database] = %q, want %q", got, checkOK)
	}
}

// TestReadiness_SingleModeOmitsOwnershipCheck: em `single` não há posse a
// coordenar e o leaseManager é nil por design. A checagem some do relatório em
// vez de aparecer como "ok" — uma checagem que sempre diz ok ensina a ignorá-la.
func TestReadiness_SingleModeOmitsOwnershipCheck(t *testing.T) {
	report := buildReadinessProbe(fakePinger{}, nil)(context.Background())

	if _, present := report.Checks["session_ownership"]; present {
		t.Error("modo single reportou checagem de posse; não há posse a coordenar com um processo só")
	}
}

// TestReadiness_StalledHeartbeatIsNotReady é a razão de a checagem de posse
// existir. Um heartbeat morto é a pior falha do mecanismo e a mais silenciosa:
// os leases param de ser renovados, expiram do ponto de vista das outras
// réplicas, e elas assumem sessões que ESTE pod ainda está servindo — os dois
// donos vivos da F89, que o WhatsApp resolve matando um de vez.
func TestReadiness_StalledHeartbeatIsNotReady(t *testing.T) {
	manager := newLeaseManager(newFakeLeaseStore(), "pod-A", 15*time.Second, 5*time.Second, nil)
	// Último tique bem além da tolerância de três intervalos.
	manager.lastTick = time.Now().Add(-time.Minute)

	report := buildReadinessProbe(fakePinger{}, func() *leaseManager { return manager })(context.Background())

	if report.Ready() {
		t.Fatal("pod com heartbeat de posse morto reportado como pronto")
	}
	if got := report.Checks["session_ownership"]; got != "heartbeat_stalled" {
		t.Errorf("checks[session_ownership] = %q, want %q", got, "heartbeat_stalled")
	}
}

// TestReadiness_FreshProcessIsReadyBeforeTheFirstTick é o controle na direção
// oposta, e sem ele a checagem acima derrubaria todo deploy: um processo que
// acabou de subir ainda não teve o primeiro tique, e reportá-lo como doente
// falharia a sonda por um intervalo inteiro em cada réplica nova.
func TestReadiness_FreshProcessIsReadyBeforeTheFirstTick(t *testing.T) {
	manager := newLeaseManager(newFakeLeaseStore(), "pod-A", 15*time.Second, 5*time.Second, nil)
	// lastTick no zero: RunHeartbeat ainda não tiquetaqueou.

	report := buildReadinessProbe(fakePinger{}, func() *leaseManager { return manager })(context.Background())

	if !report.Ready() {
		t.Fatalf("processo recém-subido reportado como não pronto: %+v", report)
	}
	if got := report.Checks["session_ownership"]; got != checkOK {
		t.Errorf("checks[session_ownership] = %q, want %q", got, checkOK)
	}
}

// TestReadiness_LiveHeartbeatIsReady fecha o terceiro estado: tiquetaqueando
// dentro da tolerância.
func TestReadiness_LiveHeartbeatIsReady(t *testing.T) {
	manager := newLeaseManager(newFakeLeaseStore(), "pod-A", 15*time.Second, 5*time.Second, nil)
	manager.lastTick = time.Now()

	if !buildReadinessProbe(fakePinger{}, func() *leaseManager { return manager })(context.Background()).Ready() {
		t.Fatal("pod com heartbeat vivo reportado como não pronto")
	}
}

// TestReadinessHandler_AnswersServiceUnavailable: 503, não 500. O pod está
// temporariamente incapaz de servir, e 5xx-como-bug seria lido como "este
// processo quebrou, reinicie" quando a causa costuma ser dependência que volta.
func TestReadinessHandler_AnswersServiceUnavailable(t *testing.T) {
	handler := readinessHandler(func(context.Context) ReadinessReport {
		return ReadinessReport{Status: statusNotReady, Checks: map[string]string{"database": "unreachable"}}
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var got ReadinessReport
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("corpo não é JSON: %v (%s)", err, rec.Body.String())
	}
	if got.Checks["database"] != "unreachable" {
		t.Errorf("o motivo não chegou ao corpo: %+v", got)
	}
}

// TestProbesAreSeparate é o teste do D6 propriamente dito: as duas sondas têm
// de poder DIVERGIR. Com o banco fora do ar, o processo continua vivo (200 em
// /health/live) e o pod deixa de estar pronto (503 em /health/ready).
//
// Uma sonda só, ou duas que sempre concordam, é o defeito que o Exp 1 mediu.
func TestProbesAreSeparate(t *testing.T) {
	d := minimalDeps()
	d.Ready = buildReadinessProbe(fakePinger{err: errors.New("db down")}, nil)
	router := NewRouter(d)

	for _, tc := range []struct {
		path string
		want int
	}{
		{"/health/live", http.StatusOK},
		{"/livez", http.StatusOK}, // alias antigo: o HEALTHCHECK do Dockerfile aponta para ele
		{"/health/ready", http.StatusServiceUnavailable},
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.path, rec.Code, tc.want)
		}
	}
}

// TestProbesNeedNoToken: o kubelet não carrega credencial. Se a sonda entrasse
// na chain autenticada, toda réplica nasceria não-pronta — e desde a F100 a
// requisição sem token é recusada com 401 ainda mais cedo.
func TestProbesNeedNoToken(t *testing.T) {
	d := minimalDeps()
	d.Ready = buildReadinessProbe(fakePinger{}, nil)
	router := NewRouter(d)

	for _, path := range []string{"/health/live", "/health/ready"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code == http.StatusUnauthorized {
			t.Errorf("%s exigiu token; o kubelet não tem como mandar um", path)
		}
	}
}

// TestReadiness_OwnershipCheckSurvivesWiringOrder fixa o defeito que a bancada
// achou e o teste unitário não: o roteador é montado por s.routes() e o
// leaseManager é instalado por setupSessionOwnership QUATRO LINHAS DEPOIS.
//
// Com a sonda recebendo *leaseManager por VALOR, ela capturava nil para sempre,
// e em `multi` o relatório saía {"database":"ok"} — idêntico a um processo
// `single` saudável. A checagem não falhava: ela simplesmente não existia.
//
// Os testes acima passavam porque entregam o manager já pronto. Nenhum deles
// exercitava a ORDEM de fiação, que é onde o defeito morava.
func TestReadiness_OwnershipCheckSurvivesWiringOrder(t *testing.T) {
	// Estado no momento em que o roteador é montado: ainda não há manager.
	var installed *leaseManager
	probe := buildReadinessProbe(fakePinger{}, func() *leaseManager { return installed })

	if _, present := probe(context.Background()).Checks["session_ownership"]; present {
		t.Fatal("checagem de posse apareceu antes de o manager ser instalado")
	}

	// setupSessionOwnership roda, quatro linhas depois.
	installed = newLeaseManager(newFakeLeaseStore(), "pod-A", 15*time.Second, 5*time.Second, nil)
	installed.lastTick = time.Now().Add(-time.Minute) // parado

	report := probe(context.Background())

	if _, present := report.Checks["session_ownership"]; !present {
		t.Fatal("a sonda não enxergou o manager instalado depois dela; em multi a checagem de posse nunca apareceria")
	}
	if report.Ready() {
		t.Error("heartbeat parado não reprovou a sonda")
	}
}
