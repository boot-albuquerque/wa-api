package bootstrap

import (
	"testing"
	"time"
)

// F88: a espera entre tentativas de webhook dormia DENTRO de um worker do pool
// de despacho. Estes testes travam as propriedades que impedem isso de voltar.

// prepararRetry ajusta a configuração de retry e restaura ao fim. `appCtx` é
// var de pacote: sem restaurar, um teste contamina os seguintes.
func prepararRetry(t *testing.T, ligado bool, tentativas, baseSegundos int) {
	t.Helper()
	antesLigado := appCtx.WebhookRetryEnabled
	antesCount := appCtx.WebhookRetryCount
	antesDelay := appCtx.WebhookRetryDelaySeconds
	t.Cleanup(func() {
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
	// Base de 60s: se a implementação esperar, o teste leva um minuto.
	prepararRetry(t, true, 5, 60)

	inicio := time.Now()
	agendou := agendarProximaTentativa("http://127.0.0.1:1/morto", payloadDeTeste(1024), "u", nil, 1)
	decorrido := time.Since(inicio)

	if !agendou {
		t.Fatal("nao agendou a segunda tentativa com retry ligado e tentativas sobrando")
	}
	if decorrido > 100*time.Millisecond {
		t.Errorf("agendar segurou o chamador por %s: a espera voltou para dentro do worker", decorrido)
	}
	// E o payload tem de estar contabilizado enquanto espera.
	if pend, _ := MetricasRetry(); pend != 1024 {
		t.Errorf("bytes pendentes = %d, quero 1024: o reagendamento nao esta' sendo contabilizado", pend)
	}
}

// TestRetry_OrcamentoDePendentesLimita: trocar "goroutine dormindo" por "timer
// pendente" só muda ONDE a memória cresce, se não houver teto. O teto existe e
// é por bytes, igual ao do pool.
func TestRetry_OrcamentoDePendentesLimita(t *testing.T) {
	prepararRetry(t, true, 5, 60)
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
// correção — a espera não segura mais worker —, mas o custo de RELEVÂNCIA não:
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
