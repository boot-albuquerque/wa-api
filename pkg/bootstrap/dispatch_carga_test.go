package bootstrap

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// limitadorBloqueante é a PRIMEIRA tentativa da F86: um teto que, ao saturar,
// SEGURA o chamador até vagar slot. Ele saiu de produção — o pool com fila
// tomou o lugar — e sobrevive aqui porque os números dele são a evidência que
// levou ao pool, e sem eles a decisão vira "confie em mim".
//
// Medido: handler 26× mais lento (3,165s contra 121ms), p95 de 51ms de atraso
// por evento. E o dreno IDÊNTICO ao do pool de mesmo tamanho (3,232s contra
// 3,234s), provando que o custo de entrega é o mesmo e a única diferença entre
// as duas formas é quem espera.
type limitadorBloqueante struct {
	slots      chan struct{}
	emVoo      atomic.Int64
	pico       atomic.Int64
	saturacoes atomic.Int64
}

func novoLimitadorBloqueante(capacidade int) *limitadorBloqueante {
	if capacidade <= 0 {
		return &limitadorBloqueante{}
	}
	return &limitadorBloqueante{slots: make(chan struct{}, capacidade)}
}

func (l *limitadorBloqueante) Go(nome string, fn func()) {
	if l == nil || l.slots == nil {
		safeGo(nome, fn)
		return
	}
	select {
	case l.slots <- struct{}{}:
	default:
		l.saturacoes.Add(1)
		l.slots <- struct{}{} // bloqueia o CHAMADOR: e' o ponto todo
	}
	atual := l.emVoo.Add(1)
	for {
		pico := l.pico.Load()
		if atual <= pico || l.pico.CompareAndSwap(pico, atual) {
			break
		}
	}
	safeGo(nome, func() {
		defer func() { l.emVoo.Add(-1); <-l.slots }()
		fn()
	})
}

func (l *limitadorBloqueante) Metricas() (emVoo, pico, saturacoes int64) {
	if l == nil {
		return 0, 0, 0
	}
	return l.emVoo.Load(), l.pico.Load(), l.saturacoes.Load()
}

// Medição A/B do teto de despacho (F86).
//
// Este arquivo NÃO é asserção de comportamento — é instrumento de medida. Ele
// existe porque a escolha entre "teto", "teto + breaker" e "as três camadas"
// tinha de sair de dado, não de intuição.
//
// As duas pernas rodam na MESMA execução, na mesma máquina e no mesmo
// instante, alternando só o teto. Comparar execuções separadas mediria também
// o estado da máquina.
//
// Rode com:
//   go test ./pkg/bootstrap/ -run TestMedicaoCargaDespacho -v
//
// Sem -race: o detector de corrida altera o escalonamento e multiplica o
// custo de cada goroutine, o que é exatamente a variável sob medida. Os
// testes de CORRETUDE do limitador (dispatch_test.go) é que rodam com -race.

// cenarioCarga descreve uma rajada.
type cenarioCarga struct {
	nome      string
	eventos   int
	porEvento int           // entregas por evento; 4 em produção
	latencia  time.Duration // quanto cada entrega demora (webhook lento)
}

// medida é o que se observa de uma perna.
type medida struct {
	picoGoroutines int
	duracao        time.Duration
	picoHeapMB     float64
	saturacoes     int64
}

// medirRajada dispara `eventos × porEvento` entregas pelo limitador e observa
// pico de goroutines, tempo e heap.
//
// O pico é amostrado por uma goroutine própria a cada 1ms: `NumGoroutine` no
// fim mediria o repouso, não o pico, que é justamente o dano da rajada.
func medirRajada(t *testing.T, teto int, c cenarioCarga) medida {
	t.Helper()

	l := novoLimitadorBloqueante(teto)

	pararAmostra := make(chan struct{})
	var picoGo atomic.Int64
	var picoHeap atomic.Uint64
	var amostrador sync.WaitGroup
	amostrador.Add(1)
	go func() {
		defer amostrador.Done()
		var ms runtime.MemStats
		for {
			select {
			case <-pararAmostra:
				return
			default:
			}
			if n := int64(runtime.NumGoroutine()); n > picoGo.Load() {
				picoGo.Store(n)
			}
			runtime.ReadMemStats(&ms)
			if ms.HeapAlloc > picoHeap.Load() {
				picoHeap.Store(ms.HeapAlloc)
			}
			time.Sleep(time.Millisecond)
		}
	}()

	var wg sync.WaitGroup
	inicio := time.Now()
	for i := 0; i < c.eventos; i++ {
		for j := 0; j < c.porEvento; j++ {
			wg.Add(1)
			l.Go("carga", func() {
				defer wg.Done()
				// Simula E/S de rede: a goroutine fica VIVA e bloqueada, que é
				// o custo real de um webhook lento. Trabalho de CPU mediria
				// outra coisa.
				time.Sleep(c.latencia)
			})
		}
	}
	wg.Wait()
	duracao := time.Since(inicio)

	close(pararAmostra)
	amostrador.Wait()

	_, _, sat := l.Metricas()
	return medida{
		picoGoroutines: int(picoGo.Load()),
		duracao:        duracao,
		picoHeapMB:     float64(picoHeap.Load()) / (1024 * 1024),
		saturacoes:     sat,
	}
}

// TestMedicaoCargaDespacho compara SEM teto contra tetos diferentes.
//
// Não falha por número: são medidas, e fixar limiar aqui viraria teste
// instável. A única asserção é a que não depende de máquina — sem teto, o
// pico tem de ser MAIOR que com teto. Se isso deixar de valer, o limitador
// parou de limitar, e aí é defeito e não variação.
func TestMedicaoCargaDespacho(t *testing.T) {
	if testing.Short() {
		t.Skip("medicao de carga: pulada em -short")
	}

	cenarios := []cenarioCarga{
		// Espelha a rajada medida em produção: o HistorySync das quatro
		// sessões restauradas produziu 13 lotes em ~32s, e cada evento
		// entregue dispara 4 entregas.
		{nome: "historysync", eventos: 500, porEvento: 4, latencia: 20 * time.Millisecond},
		// Webhook lento: o caso que hoje multiplica por 5 via retry.
		{nome: "webhook-lento", eventos: 200, porEvento: 4, latencia: 200 * time.Millisecond},
	}
	tetos := []int{0, 16, 64, 256} // 0 = sem teto (comportamento pré-F86)

	for _, c := range cenarios {
		t.Run(c.nome, func(t *testing.T) {
			resultados := map[int]medida{}
			for _, teto := range tetos {
				m := medirRajada(t, teto, c)
				resultados[teto] = m
				rotulo := fmt.Sprintf("teto=%d", teto)
				if teto == 0 {
					rotulo = "SEM TETO"
				}
				t.Logf("%-10s pico_goroutines=%-6d duracao=%-10s heap_pico=%.1fMB saturacoes=%d",
					rotulo, m.picoGoroutines, m.duracao.Round(time.Millisecond), m.picoHeapMB, m.saturacoes)
			}

			semTeto := resultados[0]
			comTeto := resultados[16]
			if comTeto.picoGoroutines >= semTeto.picoGoroutines {
				t.Errorf("pico com teto=16 (%d) nao ficou abaixo do pico sem teto (%d): o limitador nao limitou",
					comTeto.picoGoroutines, semTeto.picoGoroutines)
			}
		})
	}
}

// --- medição com E/S real ------------------------------------------------

// A medição acima usa time.Sleep, que NÃO ALOCA. Isso subestima o custo de
// memória de uma rajada, e memória é justamente o que decide se goroutine sem
// teto é problema — uma entrega real segura http.Request, buffers de resposta
// e estado TLS.
//
// Esta segunda medição faz requisição HTTP de verdade contra um servidor
// deliberadamente lento, que é o que um webhook mal-comportado parece.
func TestMedicaoCargaDespachoHTTPReal(t *testing.T) {
	if testing.Short() {
		t.Skip("medicao de carga: pulada em -short")
	}

	const latenciaServidor = 100 * time.Millisecond
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		time.Sleep(latenciaServidor)
		w.WriteHeader(http.StatusOK)
		// Corpo de resposta com tamanho realista: um webhook devolve algo.
		_, _ = w.Write(make([]byte, 4096))
	}))
	defer srv.Close()

	// Payload do tamanho de um evento Message real, que e' o que a rajada
	// carrega. Medido nesta sessao: os dumps passam de 1KB com folga.
	payload := make([]byte, 8*1024)

	medir := func(teto, entregas int) medida {
		l := novoLimitadorBloqueante(teto)
		parar := make(chan struct{})
		var picoGo atomic.Int64
		var picoHeap atomic.Uint64
		var am sync.WaitGroup
		am.Add(1)
		go func() {
			defer am.Done()
			var ms runtime.MemStats
			for {
				select {
				case <-parar:
					return
				default:
				}
				if n := int64(runtime.NumGoroutine()); n > picoGo.Load() {
					picoGo.Store(n)
				}
				runtime.ReadMemStats(&ms)
				if ms.HeapAlloc > picoHeap.Load() {
					picoHeap.Store(ms.HeapAlloc)
				}
				time.Sleep(time.Millisecond)
			}
		}()

		var wg sync.WaitGroup
		inicio := time.Now()
		for i := 0; i < entregas; i++ {
			wg.Add(1)
			l.Go("carga-http", func() {
				defer wg.Done()
				resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(payload))
				if err != nil {
					return
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			})
		}
		wg.Wait()
		d := time.Since(inicio)
		close(parar)
		am.Wait()
		_, _, sat := l.Metricas()
		return medida{int(picoGo.Load()), d, float64(picoHeap.Load()) / (1024 * 1024), sat}
	}

	const entregas = 800
	for _, teto := range []int{0, 16, 64, 256} {
		runtime.GC()
		m := medir(teto, entregas)
		rotulo := fmt.Sprintf("teto=%d", teto)
		if teto == 0 {
			rotulo = "SEM TETO"
		}
		t.Logf("%-10s pico_goroutines=%-6d duracao=%-10s heap_pico=%.1fMB saturacoes=%d",
			rotulo, m.picoGoroutines, m.duracao.Round(time.Millisecond), m.picoHeapMB, m.saturacoes)
	}
}
