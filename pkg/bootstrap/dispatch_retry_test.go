package bootstrap

import (
	"testing"
	"time"
)

// F88: a waitFull entre tentativas de webhook dormia DENTRO de um worker do pool
// de despacho. Estes testes travam as propriedades que impedem isso de voltar.

// retryBaseQueNaoDisparaSegundos é a base de backoff dos testes que ARMAM um
// timer em memória sem esperá-lo. Ela existe por causa da F136.
//
// `retryBytesPendentes` é global e `prepararRetry` o zera; o timer armado por um
// teste, porém, continua em voo depois que ele termina e, ao disparar, subtrai o
// seu payload do contador JÁ ZERADO por um teste seguinte. Medido: com base de
// 60s o timer dispara 53,9s a 66,4s depois de armado, e o pacote inteiro leva
// ~143s sob `-race -count=20` — ou seja, cada timer cai em cheio no meio de
// outro teste.
//
// A cura é não deixar timer em voo, e não tolerar o efeito dele: com uma base de
// 24h o disparo fica muito além da vida do processo de teste (o `go test` estoura
// em 10min por padrão). Todo teste que arma sem esperar usa esta base.
const retryBaseQueNaoDisparaSegundos = 24 * 60 * 60

// timerCanOutliveTest is the invariant of F136 written as a predicate instead of
// a convention: a test may only end with a retry reservation outstanding when the
// configured base puts the timer's deadline past the life of the test binary.
//
// It is a predicate, and not an inline `if`, so that it can be exercised directly
// by TestRetry_GuardaDeTimerEmVooNaoEVacua — a guard nobody can see failing is
// the same class of defect it exists to prevent.
func timerCanOutliveTest(outstandingBytes int64, baseSeconds int) bool {
	return outstandingBytes > 0 && baseSeconds < retryBaseQueNaoDisparaSegundos
}

// requireNoRetryTimerCanFire is where the base of 24h stops being documentation
// and becomes a check.
//
// EVAL-12 measured that reverting the base at ONE arming site produced 0/0/0
// failures over three `-race -count=20` runs: what carried the weight was the
// delta assertion, and the base was defence in depth that no test could see being
// removed. This guard closes that gap, and it fails on the test that ARMED —
// not on whichever test happens to read the shared counter a minute later.
//
// It runs from prepararRetry's cleanup, so it covers every arming site that
// exists today and every one added later, without anyone remembering to opt in.
func requireNoRetryTimerCanFire(t *testing.T, baseSegundos int) {
	t.Helper()
	// A negative value is somebody ELSE's leak reaching this test; it is the
	// delta assertions that diagnose that, and reporting it here would blame the
	// wrong test. Only an outstanding reservation is this test's own doing.
	pendentes := retryBytesPendentes.Load()
	if !timerCanOutliveTest(pendentes, baseSegundos) {
		return
	}
	t.Errorf("o teste terminou com %d bytes reservados e base de %ds: o timer vence em ~%ds, "+
		"dentro da vida do binario de teste, e vai subtrair de um contador ja' zerado por outro teste (F136). "+
		"Use retryBaseQueNaoDisparaSegundos, ou espere o timer drenar antes de sair",
		pendentes, baseSegundos, baseSegundos)
}

// prepararRetry ajusta a configuração de retry e restaura ao fim. `appCtx` é
// var de pacote: sem restaurar, um teste contamina os seguintes.
func prepararRetry(t *testing.T, ligado bool, tentativas, baseSegundos int) {
	t.Helper()
	antesLigado := appCtx.WebhookRetryEnabled
	antesCount := appCtx.WebhookRetryCount
	antesDelay := appCtx.WebhookRetryDelaySeconds
	t.Cleanup(func() {
		// ANTES de restaurar e de zerar: depois do Store(0) a evidencia some.
		requireNoRetryTimerCanFire(t, baseSegundos)
		appCtx.WebhookRetryEnabled = antesLigado
		appCtx.WebhookRetryCount = antesCount
		appCtx.WebhookRetryDelaySeconds = antesDelay
		retryBytesPendentes.Store(0)
		retryDescartados.Store(0)
		retryAvisouTeto.Store(false)
	})
	appCtx.WebhookRetryEnabled = ligado
	appCtx.WebhookRetryCount = tentativas
	appCtx.WebhookRetryDelaySeconds = baseSegundos
	retryBytesPendentes.Store(0)
	retryDescartados.Store(0)
	retryAvisouTeto.Store(false)
}

func payloadDeTeste(bytes int) map[string]string {
	return map[string]string{"jsonData": string(make([]byte, bytes))}
}

// TestRetry_AgendarNaoBloqueiaOChamador é O teste desta correção.
//
// Quem chama é um worker do pool de despacho. Se agendar bloquear, o worker
// fica preso pela janela inteira do backoff — que com os padrões anteriores
// eram 7,5 minutos, e dois pareamentos simultâneos contra um destino morto
// saturavam o pool de 256 e paravam TAMBÉM WebSocket, webhook global e
// RabbitMQ.
func TestRetry_AgendarNaoBloqueiaOChamador(t *testing.T) {
	// A base não pode mais ser curta (ver retryBaseQueNaoDisparaSegundos), então
	// o prazo do teste não pode depender dela: a chamada vai para uma goroutine e
	// quem falha é o select. Assim uma regressão que volte a esperar é detectada
	// em 100ms, e não depois de uma base inteira de backoff — diagnóstico melhor
	// que o de antes, além de não deixar timer em voo.
	prepararRetry(t, true, 5, retryBaseQueNaoDisparaSegundos)

	antes, _ := MetricasRetry()

	agendou := make(chan bool, 1)
	go func() {
		agendou <- agendarProximaTentativa("http://127.0.0.1:1/morto", payloadDeTeste(1024), "u", nil, 1)
	}()

	select {
	case ok := <-agendou:
		if !ok {
			t.Fatal("nao agendou a segunda tentativa com retry ligado e tentativas sobrando")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("agendar segurou o chamador por mais de 100ms: a waitFull voltou para dentro do worker")
	}

	// E o payload tem de estar contabilizado enquanto waitFull. A pergunta e'
	// "houve reserva NOVA de 1024 bytes?", e quem responde e' o DELTA — nao o
	// valor absoluto. `retryBytesPendentes` e' global: um timer alheio que vença
	// entre o `Store(0)` de prepararRetry e esta leitura deixa o absoluto em
	// -1024, e a assercao antiga reportaria isso como "nao esta' contabilizando",
	// que e' diagnostico errado. Mesmo motivo da correcao do outbox (F136).
	depois, _ := MetricasRetry()
	if delta := depois - antes; delta != 1024 {
		if delta < 0 {
			t.Errorf("retryBytesPendentes caiu %d bytes durante este teste: algum teste anterior deixou timer de retry em voo (ver retryBaseQueNaoDisparaSegundos)", -delta)
		} else {
			t.Errorf("reserva nova de %d bytes, quero 1024: o reagendamento nao esta' sendo contabilizado", delta)
		}
	}
}

// TestRetry_GuardaDeTimerEmVooNaoEVacua exercita requireNoRetryTimerCanFire pelo
// seu predicado.
//
// A guarda mora num `t.Cleanup` e reprova o proprio teste que a acionou, entao
// nao da' para vê-la falhar sem um `*testing.T` falso. Travar o PREDICADO e' o
// que impede que ela vire enfeite: as tres primeiras linhas sao os sitios reais
// que armam timer, com as bases medidas na F136 antes da correcao.
func TestRetry_GuardaDeTimerEmVooNaoEVacua(t *testing.T) {
	casos := []struct {
		nome       string
		pendentes  int64
		base       int
		querAcusar bool
	}{
		{"AgendarNaoBloqueiaOChamador com a base curta de antes", 1024, 60, true},
		{"OrcamentoDePendentesLimita com a base curta de antes", 4096, 60, true},
		{"SemOutboxCaiParaAMemoria com a base curta de antes", 128, 30, true},
		{"base de um segundo ainda vence dentro da corrida", 16, 1, true},
		{"base de 24h: o timer nao vence antes do processo morrer", 1024, retryBaseQueNaoDisparaSegundos, false},
		{"nada reservado: nao ha' timer para vencer", 0, 30, false},
		{"LiberaOrcamentoAoDisparar drena o proprio timer", 0, 0, false},
		{"valor negativo e' vazamento alheio, nao deste teste", -1024, 30, false},
	}
	for _, c := range casos {
		if got := timerCanOutliveTest(c.pendentes, c.base); got != c.querAcusar {
			t.Errorf("%s: timerCanOutliveTest(%d, %d) = %v, quero %v",
				c.nome, c.pendentes, c.base, got, c.querAcusar)
		}
	}
}

// TestRetry_OrcamentoDePendentesLimita: trocar "goroutine dormindo" por "timer
// pendente" só muda ONDE a memória cresce, se não houver teto. O teto existe e
// é por bytes, igual ao do pool.
func TestRetry_OrcamentoDePendentesLimita(t *testing.T) {
	prepararRetry(t, true, 5, retryBaseQueNaoDisparaSegundos)
	t.Setenv(envRetryMaxPendingBytes, "4096")

	const tamanho = 1024
	var agendados int
	for i := 0; i < 20; i++ {
		if agendarProximaTentativa("http://127.0.0.1:1/morto", payloadDeTeste(tamanho), "u", nil, 1) {
			agendados++
		}
	}

	if agendados == 20 {
		t.Fatal("agendou todos os 20 com teto de 4KB: o orcamento nao esta' limitando")
	}
	if agendados == 0 {
		t.Fatal("nao agendou nenhum: o teto esta' rejeitando ate' o que cabe")
	}
	if pend, _ := MetricasRetry(); pend > 4096 {
		t.Errorf("bytes pendentes = %d, acima do teto de 4096", pend)
	}
	if _, desc := MetricasRetry(); desc == 0 {
		t.Error("descartados = 0 apesar de o teto ter sido atingido: a metrica nao conta")
	}
}

// TestRetry_LiberaOrcamentoAoDisparar: o orçamento devolvido é o que impede o
// teto de virar um vazamento que só cresce.
func TestRetry_LiberaOrcamentoAoDisparar(t *testing.T) {
	// Base zero: o timer dispara de imediato. O usuario "u" nao tem cliente
	// HTTP registrado, entao tentarWebhook sai com Warn sem tocar a rede.
	prepararRetry(t, true, 5, 0)

	if !agendarProximaTentativa("http://127.0.0.1:1/morto", payloadDeTeste(2048), "u", nil, 1) {
		t.Fatal("nao agendou")
	}

	prazo := time.After(5 * time.Second)
	for {
		if pend, _ := MetricasRetry(); pend == 0 {
			return
		}
		select {
		case <-prazo:
			pend, _ := MetricasRetry()
			t.Fatalf("bytes pendentes = %d apos o timer disparar: o orcamento vazou", pend)
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

// TestRetry_NaoAgendaQuandoNaoDeve cobre os dois fins de linha.
func TestRetry_NaoAgendaQuandoNaoDeve(t *testing.T) {
	t.Run("tentativas esgotadas", func(t *testing.T) {
		prepararRetry(t, true, 5, 60)
		if agendarProximaTentativa("http://x", payloadDeTeste(64), "u", nil, 5) {
			t.Error("agendou a 6a tentativa com o teto em 5")
		}
	})
	t.Run("retry desligado", func(t *testing.T) {
		prepararRetry(t, false, 5, 60)
		if agendarProximaTentativa("http://x", payloadDeTeste(64), "u", nil, 1) {
			t.Error("agendou com retry desligado")
		}
	})
}

// TestRetry_JitterEspalha: sem jitter, N sessões que falham juntas repetem em
// uníssono — exatamente a tempestade de QR martelando um destino que já está
// com problema. O backoff exponencial sozinho espaça as tentativas de UM
// cliente; não separa clientes entre si.
func TestRetry_JitterEspalha(t *testing.T) {
	prepararRetry(t, true, 5, 10)

	nominal := 10 * time.Second // tentativa 1: base × 2^0
	vistos := map[time.Duration]bool{}
	for i := 0; i < 50; i++ {
		d := atrasoDaTentativa(1)
		vistos[d] = true

		menor := nominal - nominal*retryJitterPercent/100
		maior := nominal + nominal*retryJitterPercent/100
		if d < menor || d > maior {
			t.Fatalf("atraso %s fora da faixa [%s, %s] do jitter de %d%%", d, menor, maior, retryJitterPercent)
		}
	}
	if len(vistos) < 10 {
		t.Errorf("so' %d valores distintos em 50 sorteios: o jitter nao esta' espalhando", len(vistos))
	}
}

// TestRetry_BackoffCresceExponencialmente: a fórmula não mudou nesta correção,
// só a base e o jitter. Travar a forma impede que um ajuste de base vire, sem
// querer, uma mudança de curva.
func TestRetry_BackoffCresceExponencialmente(t *testing.T) {
	prepararRetry(t, true, 5, 8)

	// Compara nominais, tolerando o jitter: cada passo tem de ser ~2× o
	// anterior, e com ±25% em ambos a razão fica entre 1,2 e 3,4.
	anterior := atrasoDaTentativa(1)
	for n := 2; n <= 4; n++ {
		atual := atrasoDaTentativa(n)
		razao := float64(atual) / float64(anterior)
		if razao < 1.2 || razao > 3.4 {
			t.Errorf("tentativa %d: razao %.2f (%s apos %s), esperava ~2x", n, razao, atual, anterior)
		}
		anterior = atual
	}
}

// TestRetry_JanelaTotalCabeNoOrcamentoDeRelevancia trava o padrão contra a
// decisão que o escolheu, no molde de TestPool_PadroesCobremARajadaMedida.
//
// A janela era de 450s (base 30). O custo de RECURSO dela sumiu com esta
// correção — a waitFull não segura mais worker —, mas o custo de RELEVÂNCIA não:
// entregar um evento de mensagem sete minutos atrasado já não serve para boa
// parte dos usos. Se alguém subir a base, este teste obriga a decisão a ser
// consciente.
func TestRetry_JanelaTotalCabeNoOrcamentoDeRelevancia(t *testing.T) {
	const janelaMaxima = 150 * time.Second

	base := time.Duration(*webhookRetryDelaySeconds) * time.Second
	tentativas := *webhookRetryCount

	var total time.Duration
	for n := 1; n < tentativas; n++ {
		total += base * (time.Duration(1) << uint(n-1))
	}

	if total > janelaMaxima {
		t.Errorf("janela total de retry = %s (base %s, %d tentativas), acima do teto de %s",
			total, base, tentativas, janelaMaxima)
	}
}
