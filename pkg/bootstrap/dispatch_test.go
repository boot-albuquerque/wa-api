package bootstrap

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// F86: o despacho de eventos disparava goroutines sem teto. Estes testes
// travam o TETO, não o caminho feliz — um limitador que nunca limita passa
// em qualquer teste de "a função rodou".

// TestLimitador_RespeitaOTeto é o teste central. Dispara muito mais trabalho
// que a capacidade e verifica que a concorrência OBSERVADA nunca a excede.
//
// A contagem é feita pelo próprio trabalho (não pelas métricas do limitador)
// de propósito: medir com o instrumento que está sob teste provaria apenas
// que ele é consistente consigo mesmo.
func TestLimitador_RespeitaOTeto(t *testing.T) {
	const capacidade = 4
	const trabalhos = 200

	l := newDispatchLimiter(capacidade)

	var emVoo, picoObservado atomic.Int64
	var wg sync.WaitGroup
	wg.Add(trabalhos)

	for i := 0; i < trabalhos; i++ {
		l.Go("teste", func() {
			defer wg.Done()
			n := emVoo.Add(1)
			for {
				p := picoObservado.Load()
				if n <= p || picoObservado.CompareAndSwap(p, n) {
					break
				}
			}
			// Segura o slot tempo suficiente para que os demais se acumulem;
			// sem isso o trabalho termina antes de haver concorrência e o
			// teste passaria mesmo sem teto nenhum.
			time.Sleep(2 * time.Millisecond)
			emVoo.Add(-1)
		})
	}
	wg.Wait()

	if p := picoObservado.Load(); p > capacidade {
		t.Fatalf("concorrencia observada chegou a %d com teto %d", p, capacidade)
	}
	if p := picoObservado.Load(); p < 2 {
		t.Fatalf("pico observado = %d: o teste nao chegou a ter concorrencia, entao nao mede o teto", p)
	}
}

// TestLimitador_TetoZeroNaoLimita: zero DESLIGA o teto e restaura o
// comportamento anterior à F86. É o que torna o A/B possível — e é preciso
// provar que desliga de fato, senão a medição compara duas vezes o mesmo.
func TestLimitador_TetoZeroNaoLimita(t *testing.T) {
	const trabalhos = 200

	l := newDispatchLimiter(0)

	var emVoo, pico atomic.Int64
	liberar := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(trabalhos)

	for i := 0; i < trabalhos; i++ {
		l.Go("teste", func() {
			defer wg.Done()
			n := emVoo.Add(1)
			for {
				p := pico.Load()
				if n <= p || pico.CompareAndSwap(p, n) {
					break
				}
			}
			<-liberar
			emVoo.Add(-1)
		})
	}

	// Espera todos entrarem em voo. Com teto, isto travaria — sem teto, não.
	prazo := time.After(5 * time.Second)
	for pico.Load() < trabalhos {
		select {
		case <-prazo:
			t.Fatalf("so' %d de %d entraram em voo: o teto nao foi desligado", pico.Load(), trabalhos)
		default:
			time.Sleep(time.Millisecond)
		}
	}
	close(liberar)
	wg.Wait()
}

// TestLimitador_PanicoNaoVazaSlot: um pânico dentro do trabalho não pode
// consumir o slot para sempre. Sem o release no defer DENTRO da goroutine, o
// teto encolheria a cada pânico até travar todo o despacho — falha que só
// apareceria em produção, depois de acumular.
func TestLimitador_PanicoNaoVazaSlot(t *testing.T) {
	const capacidade = 2

	l := newDispatchLimiter(capacidade)

	var wg sync.WaitGroup
	wg.Add(capacidade * 3)
	for i := 0; i < capacidade*3; i++ {
		l.Go("panico", func() {
			defer wg.Done()
			panic("estourou")
		})
	}

	pronto := make(chan struct{})
	go func() { wg.Wait(); close(pronto) }()
	select {
	case <-pronto:
	case <-time.After(5 * time.Second):
		emVoo, _, _ := l.Metricas()
		t.Fatalf("o despacho travou apos os panicos; emVoo=%d — slot vazou", emVoo)
	}

	// Depois de tudo, nenhum slot pode continuar ocupado.
	prazo := time.After(2 * time.Second)
	for {
		emVoo, _, _ := l.Metricas()
		if emVoo == 0 {
			break
		}
		select {
		case <-prazo:
			t.Fatalf("emVoo = %d apos o fim; slots vazaram", emVoo)
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

// TestLimitador_ContaSaturacao: a métrica de saturação é o que responde "o
// teto está apertado demais?" com dado em vez de palpite.
func TestLimitador_ContaSaturacao(t *testing.T) {
	l := newDispatchLimiter(1)

	liberar := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(10)

	// Os despachos saem de OUTRA goroutine porque a aquisição BLOQUEIA o
	// chamador (ver TestLimitador_AquisicaoBloqueiaOChamador). Emiti-los no
	// corpo do teste travaria no segundo, e o `close(liberar)` abaixo nunca
	// chegaria — foi exatamente o deadlock da primeira versão deste teste,
	// que rodou 600s antes de o pacote estourar por timeout.
	go func() {
		for i := 0; i < 10; i++ {
			l.Go("teste", func() { defer wg.Done(); <-liberar })
		}
	}()

	// Espera saturar de fato: sem isso o teste poderia terminar antes de
	// haver disputa, e mediria zero por falta de carga, não por defeito.
	prazo := time.After(5 * time.Second)
	for {
		if _, _, sat := l.Metricas(); sat > 0 {
			break
		}
		select {
		case <-prazo:
			t.Fatal("saturacao = 0 com teto 1 e 10 trabalhos; a metrica nao esta contando")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	close(liberar)
	wg.Wait()
}

// TestLimitador_AquisicaoBloqueiaOChamador fixa a propriedade que causou o
// deadlock acima, porque ela tem consequência em PRODUÇÃO: quem chama
// dispatchGo é a goroutine do handler de eventos do SDK, e segurá-la é
// backpressure sobre o processamento da sessão inteira, não sobre uma fila.
//
// O teste existe para que essa escolha seja deliberada e visível. Se um dia
// a aquisição virar não-bloqueante (descartar em vez de esperar), este teste
// falha e obriga a decisão a ser tomada de novo, em vez de mudar por
// acidente.
func TestLimitador_AquisicaoBloqueiaOChamador(t *testing.T) {
	l := newDispatchLimiter(1)

	liberar := make(chan struct{})
	var ocupado sync.WaitGroup
	ocupado.Add(1)
	l.Go("ocupa", func() { ocupado.Done(); <-liberar })
	ocupado.Wait()

	voltou := make(chan struct{})
	go func() {
		l.Go("segundo", func() {})
		close(voltou)
	}()

	select {
	case <-voltou:
		close(liberar)
		t.Fatal("o segundo despacho retornou com o teto ocupado: a aquisicao deixou de bloquear")
	case <-time.After(100 * time.Millisecond):
		// Esperado: o chamador está segurado.
	}

	close(liberar)
	select {
	case <-voltou:
	case <-time.After(5 * time.Second):
		t.Fatal("o chamador nao foi liberado depois que o slot vagou")
	}
}

// TestConfig_InvalidoCaiNoPadrao: valor inválido resulta no padrão, com
// aviso em Warn — nunca num teto arbitrário.
//
// O nome anterior era "NaoDesliga", e ficou FALSO quando o padrão virou 0:
// com padrão desligado, um typo também desliga. A asserção honesta é a
// relação (inválido == padrão), que continua valendo qualquer que seja o
// padrão, e é ela que protege o dia em que ele virar não-zero.
func TestConfig_InvalidoCaiNoPadrao(t *testing.T) {
	for _, valor := range []string{"abc", "-1", "1e3", " "} {
		t.Run("valor="+valor, func(t *testing.T) {
			t.Setenv(envDispatchMaxConcurrency, valor)
			if got := dispatchConcurrencyConfigurada(); got != dispatchDefaultConcurrency {
				t.Errorf("teto = %d para %q, quero o padrao %d", got, valor, dispatchDefaultConcurrency)
			}
		})
	}
}

func TestConfig_TetoLidoDoAmbiente(t *testing.T) {
	t.Setenv(envDispatchMaxConcurrency, "7")
	if got := dispatchConcurrencyConfigurada(); got != 7 {
		t.Errorf("teto = %d, quero 7", got)
	}
	// Zero e' valido e significa "sem teto" — distinto de invalido.
	t.Setenv(envDispatchMaxConcurrency, "0")
	if got := dispatchConcurrencyConfigurada(); got != 0 {
		t.Errorf("teto = %d para \"0\", quero 0 (sem teto)", got)
	}
}

// TestLimitador_NaoVazaGoroutine: o teto limita a concorrência, não pode
// deixar goroutine pendurada. Compara a contagem antes e depois.
func TestLimitador_NaoVazaGoroutine(t *testing.T) {
	l := newDispatchLimiter(8)

	runtime.GC()
	antes := runtime.NumGoroutine()

	var wg sync.WaitGroup
	wg.Add(100)
	for i := 0; i < 100; i++ {
		l.Go("teste", func() { defer wg.Done() })
	}
	wg.Wait()

	// As goroutines de safeGo terminam logo depois do wg.Done; dá margem.
	prazo := time.After(3 * time.Second)
	for {
		runtime.GC()
		if runtime.NumGoroutine() <= antes+2 {
			return
		}
		select {
		case <-prazo:
			t.Fatalf("goroutines: antes=%d depois=%d", antes, runtime.NumGoroutine())
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}
