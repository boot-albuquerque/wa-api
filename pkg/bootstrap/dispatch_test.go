package bootstrap

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// F86: o despacho de eventos disparava uma goroutine por entrega, sem teto.
// Estes testes travam as PROPRIEDADES da escada de degradação — não o caminho
// feliz. Um pool que nunca enfileira e uma fila que nunca segura passariam em
// qualquer teste de "a função rodou".

// esperarPor waitFull uma condição virar verdadeira, ou falha com a mensagem.
// Sondar é preciso porque as entregas rodam noutras goroutines; dormir um
// tempo fixo esconderia lentidão atrás de folga.
func esperarPor(t *testing.T, prazo time.Duration, cond func() bool, formato string, args ...any) {
	t.Helper()
	limite := time.After(prazo)
	for {
		if cond() {
			return
		}
		select {
		case <-limite:
			t.Fatalf(formato, args...)
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

// esperarGrupo waitFull um WaitGroup COM PRAZO, reportando o progresso quando
// estoura. `sync.WaitGroup` só oferece waitFull infinita, e num teste que
// persegue "o mecanismo travou" a waitFull infinita é o próprio defeito
// silenciando o teste.
func esperarGrupo(t *testing.T, wg *sync.WaitGroup, prazo time.Duration, formato string, progresso *atomic.Int64, total int64) {
	t.Helper()
	pronto := make(chan struct{})
	go func() { wg.Wait(); close(pronto) }()
	select {
	case <-pronto:
	case <-time.After(prazo):
		t.Fatalf(formato, progresso.Load(), total)
	}
}

// TestPool_RajadaMenorQueOPoolNaoEspera trava o PRIMEIRO degrau: enquanto a
// rajada cabe no pool, o mecanismo tem de ser indistinguível de não existir.
//
// É o degrau que a produção desta instalação vive 100% do tempo (peak medido:
// 23 eventos em 100ms, contra pool de 256). Se ele custar, o mecanismo cobra
// de todo mundo por um problema que quase ninguém tem.
func TestPool_RajadaMenorQueOPoolNaoEspera(t *testing.T) {
	const workers = 64
	const trabalhos = 32 // metade do pool: nunca deveria haver disputa

	p := newDispatchPool(workers, 32*1024*1024)

	var wg sync.WaitGroup
	wg.Add(trabalhos)
	liberar := make(chan struct{})
	for i := 0; i < trabalhos; i++ {
		p.Go("teste", 1024, func() { defer wg.Done(); <-liberar })
	}

	// Todos têm de estar EM VOO ao mesmo tempo — nada enfileirado esperando.
	esperarPor(t, 5*time.Second, func() bool {
		inFlight, _, _, _ := p.Metrics()
		return inFlight == trabalhos
	}, "nem todos entraram em voo com rajada menor que o pool: o pool esta serializando")

	close(liberar)
	wg.Wait()

	if _, _, esperas, _ := p.Metrics(); esperas != 0 {
		t.Errorf("esperas = %d com rajada (%d) menor que o pool (%d): o handler foi segurado sem necessidade",
			esperas, trabalhos, workers)
	}
}

// TestPool_RespeitaOOrcamentoDeBytes trava o TERCEIRO degrau: os bytes em voo
// nunca podem passar do orçamento.
//
// A contagem é feita pelo próprio trabalho, e não pelas métricas do pool:
// medir com o instrumento sob teste provaria só que ele é consistente consigo
// mesmo.
func TestPool_RespeitaOOrcamentoDeBytes(t *testing.T) {
	const tamanho = 1024
	const orcamento = 8 * tamanho // cabem 8 de cada vez
	const trabalhos = 200

	// Pool grande de propósito: quem tem de limitar aqui é o BYTE, não o
	// número de workers. Com pool pequeno o teste passaria pelo motivo errado.
	p := newDispatchPool(128, orcamento)

	var bytesEmVoo, peakBytes atomic.Int64
	var wg sync.WaitGroup
	wg.Add(trabalhos)

	go func() {
		for i := 0; i < trabalhos; i++ {
			p.Go("teste", tamanho, func() {
				defer wg.Done()
				n := bytesEmVoo.Add(tamanho)
				for {
					peak := peakBytes.Load()
					if n <= peak || peakBytes.CompareAndSwap(peak, n) {
						break
					}
				}
				time.Sleep(time.Millisecond)
				bytesEmVoo.Add(-tamanho)
			})
		}
	}()
	wg.Wait()

	if p := peakBytes.Load(); p > orcamento {
		t.Fatalf("bytes em voo chegaram a %d com orcamento %d", p, orcamento)
	}
	if p := peakBytes.Load(); p < 2*tamanho {
		t.Fatalf("peak de bytes = %d: o teste nao chegou a ter concorrencia, entao nao mede o orcamento", p)
	}
}

// TestPool_NuncaDescarta é a propriedade que distingue este desenho da
// alternativa que foi medida e rejeitada.
//
// Descartar na saturação perdeu 93,6% das entregas com teto 64, e 70,5% mesmo
// com fila de 8MB. A escolha aqui é degradar latência, nunca correção — e é um
// teste, e não um comentário, que impede alguém de "otimizar" isso de volta.
func TestPool_NuncaDescarta(t *testing.T) {
	const trabalhos = 500
	const tamanho = 4096

	// Orçamento apertadíssimo: cabe UM de cada vez. Saturação garantida.
	p := newDispatchPool(8, tamanho)

	var executados atomic.Int64
	var wg sync.WaitGroup
	wg.Add(trabalhos)
	go func() {
		for i := 0; i < trabalhos; i++ {
			p.Go("teste", tamanho, func() { defer wg.Done(); executados.Add(1) })
		}
	}()

	pronto := make(chan struct{})
	go func() { wg.Wait(); close(pronto) }()
	select {
	case <-pronto:
	case <-time.After(30 * time.Second):
		t.Fatalf("so' %d de %d executaram: o despacho travou", executados.Load(), trabalhos)
	}

	if n := executados.Load(); n != trabalhos {
		t.Errorf("executados = %d, quero %d: houve descarte", n, trabalhos)
	}
	if _, _, esperas, _ := p.Metrics(); esperas == 0 {
		t.Error("esperas = 0 com orcamento de 1 item e 500 trabalhos: o teste nao saturou, entao nao prova que nao descarta sob saturacao")
	}
}

// TestPool_PanicoNaoMataOWorker é o teste que justifica o recover ser POR
// TRABALHO e não por goroutine.
//
// `SafeGo` recupera o pânico e a goroutine termina logo depois — comportamento
// certo para uma goroutine descartável, e errado para um worker permanente:
// aqui o pool encolheria a cada pânico até não sobrar nenhum, e o despacho
// pararia de vez. É falha que só apareceria em produção, depois de acumular.
func TestPool_PanicoNaoMataOWorker(t *testing.T) {
	const workers = 4

	p := newDispatchPool(workers, 32*1024*1024)

	// Mais pânicos que workers: se cada um matasse um worker, sobrariam zero.
	var panicos sync.WaitGroup
	panicos.Add(workers * 3)
	var executados atomic.Int64
	for i := 0; i < workers*3; i++ {
		p.Go("panico", 128, func() {
			defer panicos.Done()
			executados.Add(1)
			panic("estourou")
		})
	}
	// COM PRAZO. Um `panicos.Wait()` nu parece mais simples e é pior: sob o
	// defeito que este teste persegue, os workers morrem, os pânicos restantes
	// nunca rodam e a waitFull fica para sempre. Foi o que aconteceu ao rodar o
	// controle negativo — o teste pendurou por 300s em vez de acusar. Teste
	// que trava não avisa ninguém; teste que falha, sim.
	esperarGrupo(t, &panicos, 10*time.Second,
		"so' %d de %d panicos rodaram: os workers morreram e o pool encolheu",
		&executados, int64(workers*3))

	// O pool tem de continuar entregando depois.
	var depois sync.WaitGroup
	const seguintes = 50
	depois.Add(seguintes)
	var ok atomic.Int64
	for i := 0; i < seguintes; i++ {
		p.Go("depois", 128, func() { defer depois.Done(); ok.Add(1) })
	}

	pronto := make(chan struct{})
	go func() { depois.Wait(); close(pronto) }()
	select {
	case <-pronto:
	case <-time.After(10 * time.Second):
		t.Fatalf("apos %d panicos o pool entregou so' %d de %d: os workers morreram", workers*3, ok.Load(), seguintes)
	}

	// E o orçamento tem de ter sido devolvido: o defer que solta os bytes roda
	// ANTES do recover, senão um pânico vazaria orçamento para sempre.
	esperarPor(t, 5*time.Second, func() bool {
		inFlight, _, _, _ := p.Metrics()
		return inFlight == 0
	}, "inFlight != 0 apos os panicos: o orcamento vazou")
}

// TestPool_ItemMaiorQueOOrcamentoPassa: sem esta guarda, o mecanismo de
// proteção vira deadlock. O evento de 120KB medido nesta instalação, com um
// orçamento menor que ele, esperaria para sempre por espaço que nunca chega.
func TestPool_ItemMaiorQueOOrcamentoPassa(t *testing.T) {
	p := newDispatchPool(4, 1024)

	feito := make(chan struct{})
	go func() {
		p.Go("gigante", 120*1024, func() { close(feito) })
	}()

	select {
	case <-feito:
	case <-time.After(5 * time.Second):
		t.Fatal("item maior que o orcamento inteiro nunca executou: a guarda virou deadlock")
	}
}

// TestPool_DesligadoDelegaDireto: workers=0 restaura o comportamento anterior
// à F86. É o rollback por variável de ambiente, e precisa ser provado — senão
// o "desligado" pode estar tão quebrado quanto o ligado.
func TestPool_DesligadoDelegaDireto(t *testing.T) {
	p := newDispatchPool(0, 32*1024*1024)

	const trabalhos = 200
	var inFlight, peak atomic.Int64
	liberar := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(trabalhos)

	for i := 0; i < trabalhos; i++ {
		p.Go("teste", 1024, func() {
			defer wg.Done()
			n := inFlight.Add(1)
			for {
				pi := peak.Load()
				if n <= pi || peak.CompareAndSwap(pi, n) {
					break
				}
			}
			<-liberar
			inFlight.Add(-1)
		})
	}

	// Sem pool, todos entram em voo ao mesmo tempo. Com pool, isto travaria.
	esperarPor(t, 5*time.Second, func() bool { return peak.Load() >= trabalhos },
		"so' %d de %d entraram em voo: o mecanismo nao foi desligado", peak.Load(), trabalhos)

	close(liberar)
	wg.Wait()
}

// TestConfig_InvalidoCaiNoPadrao: valor inválido resulta no padrão, com aviso
// em Warn — nunca num pool arbitrário. Com padrões NÃO-ZERO este caminho é a
// diferença entre proteger e não proteger.
func TestConfig_InvalidoCaiNoPadrao(t *testing.T) {
	for _, valor := range []string{"abc", "-1", "1e3", " ", "256MB"} {
		t.Run("valor="+valor, func(t *testing.T) {
			t.Setenv(envDispatchWorkers, valor)
			if got := configuredDispatchWorkers(); got != dispatchDefaultWorkers {
				t.Errorf("workers = %d para %q, quero o padrao %d", got, valor, dispatchDefaultWorkers)
			}
			t.Setenv(envDispatchQueueBytes, valor)
			if got := configuredDispatchQueueBytes(); got != dispatchDefaultQueueBytes {
				t.Errorf("orcamento = %d para %q, quero o padrao %d", got, valor, int64(dispatchDefaultQueueBytes))
			}
		})
	}
}

func TestConfig_LidoDoAmbiente(t *testing.T) {
	t.Setenv(envDispatchWorkers, "7")
	if got := configuredDispatchWorkers(); got != 7 {
		t.Errorf("workers = %d, quero 7", got)
	}
	// Zero e' VALIDO e significa "desligado" — distinto de invalido, que cai
	// no padrao. E' o rollback, entao nao pode ser tratado como erro.
	t.Setenv(envDispatchWorkers, "0")
	if got := configuredDispatchWorkers(); got != 0 {
		t.Errorf("workers = %d para \"0\", quero 0 (desligado)", got)
	}

	t.Setenv(envDispatchQueueBytes, "1048576")
	if got := configuredDispatchQueueBytes(); got != 1048576 {
		t.Errorf("orcamento = %d, quero 1048576", got)
	}
}

// TestPool_PadroesCobremARajadaMedida trava os padrões contra a medição que os
// escolheu. Se alguém mexer nos números sem refazer a medição, este teste
// falha e obriga a decisão a ser tomada de novo.
//
// Pico de pareamento medido em produção: 23 eventos em 100ms, 4 entregas por
// evento. Ver F86 no HOUSEKEEP.md.
func TestPool_PadroesCobremARajadaMedida(t *testing.T) {
	const picoEventos = 23
	const entregasPorEvento = 4
	const entregasNoPico = picoEventos * entregasPorEvento

	if dispatchDefaultWorkers < entregasNoPico {
		t.Errorf("pool padrao %d < %d entregas do peak de pareamento medido: o degrau 1 nao cobre a producao",
			dispatchDefaultWorkers, entregasNoPico)
	}
	// Payload maximo medido: 120.302B. O orcamento tem de comportar o peak
	// inteiro no pior caso de tamanho, senao o handler e' segurado na rajada
	// que a medicao diz ser rotineira.
	const payloadMax = 120302
	if int64(dispatchDefaultQueueBytes) < entregasNoPico*payloadMax {
		t.Errorf("orcamento padrao %d < %d bytes (peak x payload maximo medido)",
			int64(dispatchDefaultQueueBytes), int64(entregasNoPico*payloadMax))
	}
}

// TestPool_NaoVazaGoroutine: o pool tem tamanho fixo. Depois da rajada, a
// contagem tem de voltar ao pool, e não crescer com o número de entregas.
func TestPool_NaoVazaGoroutine(t *testing.T) {
	runtime.GC()
	antes := runtime.NumGoroutine()

	const workers = 8
	p := newDispatchPool(workers, 32*1024*1024)

	var wg sync.WaitGroup
	wg.Add(500)
	for i := 0; i < 500; i++ {
		p.Go("teste", 512, func() { defer wg.Done() })
	}
	wg.Wait()

	// Os workers são permanentes: o esperado é antes+workers, e não antes.
	// Uma margem pequena cobre a goroutine de teste e o runtime.
	esperarPor(t, 3*time.Second, func() bool {
		runtime.GC()
		return runtime.NumGoroutine() <= antes+workers+2
	}, "goroutines: antes=%d workers=%d depois=%d — 500 entregas viraram goroutines",
		antes, workers, runtime.NumGoroutine())
}
