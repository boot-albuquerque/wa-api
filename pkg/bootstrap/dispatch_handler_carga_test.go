package bootstrap

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Medição do ATRASO INDUZIDO NO HANDLER (F86, segunda rodada).
//
// A primeira medição (dispatch_carga_test.go) respondeu "quanto custa não ter
// teto": ~4.000 goroutines e ~32MB para 800 entregas. Ela NÃO responde a
// pergunta que decide o desenho, que é o custo do remédio:
//
//	quanto tempo o teto SEGURA quem despacha?
//
// Isso importa porque em produção quem despacha não é um laço de teste — é a
// goroutine do handler de eventos do SDK, que processa os eventos da sessão em
// SEQUÊNCIA. Segurá-la não atrasa "uma entrega": atrasa TODOS os eventos
// seguintes daquela sessão, inclusive os que nem vão para webhook (marcar como
// lido, presença, recibo). Trocar "goroutines demais" por "handler parado"
// pode ser troca ruim, e até aqui isso era especulação minha.
//
// Este arquivo é INSTRUMENTO, não asserção de comportamento. As quatro formas
// de aquisição são implementadas AQUI, e não em produção, de propósito: só
// entra em pkg/ a que ganhar a medição.
//
// Rode com:
//
//	go test ./pkg/bootstrap/ -run TestMedicaoAtrasoNoHandler -v -timeout 20m
//
// Sem -race: o detector muda o escalonamento, que é a variável sob medida.

// --- as quatro formas de aquisição ---------------------------------------

// despachante é o contrato que o handler enxerga. `tamanho` é o tamanho do
// payload em bytes: a forma "fila" precisa dele para se limitar por BYTES, e
// não por contagem — a medição de payload real desta sessão achou máximo de
// 120KB contra mediana de 2KB, então uma fila de N itens tem teto de memória
// 40× diferente conforme a sorte do lote.
type despachante interface {
	Despachar(tamanho int, fn func())
	Fechar()
	Perdidas() int64
}

// formaSemTeto reproduz o comportamento anterior à F86: uma goroutine por
// entrega, sem limite. É a linha de base — sem ela os números das outras não
// significam nada.
type formaSemTeto struct{ wg sync.WaitGroup }

func (f *formaSemTeto) Despachar(_ int, fn func()) {
	f.wg.Add(1)
	go func() { defer f.wg.Done(); fn() }()
}
func (f *formaSemTeto) Fechar()         { f.wg.Wait() }
func (f *formaSemTeto) Perdidas() int64 { return 0 }

// formaBloqueante é o limitador da primeira tentativa da F86
// (limitadorBloqueante, em dispatch_carga_test.go):
// quando satura, a aquisição segura o chamador até vagar slot. É a forma sob
// suspeita — a medição existe para dizer quanto ela segura.
type formaBloqueante struct {
	l  *limitadorBloqueante
	wg sync.WaitGroup
}

func novaFormaBloqueante(teto int) *formaBloqueante {
	return &formaBloqueante{l: novoLimitadorBloqueante(teto)}
}
func (f *formaBloqueante) Despachar(_ int, fn func()) {
	f.wg.Add(1)
	f.l.Go("carga", func() { defer f.wg.Done(); fn() })
}
func (f *formaBloqueante) Fechar() { f.wg.Wait() }
func (f *formaBloqueante) Perdidas() int64 {
	_, _, sat := f.l.Metrics()
	// Saturações NÃO são perdas nesta forma — ela não perde, ela waitFull. O
	// número é reportado à parte para não ser confundido com descarte.
	_ = sat
	return 0
}

// formaDescarte troca latência por perda: se não há slot, a entrega é jogada
// fora e o handler segue. Nunca atrasa o handler, e é a única forma em que o
// usuário perde webhook — o que a medição precisa quantificar, porque "perdeu
// 3%" e "perdeu 60%" são decisões diferentes.
type formaDescarte struct {
	slots    chan struct{}
	perdidas atomic.Int64
	wg       sync.WaitGroup
}

func novaFormaDescarte(teto int) *formaDescarte {
	return &formaDescarte{slots: make(chan struct{}, teto)}
}
func (f *formaDescarte) Despachar(_ int, fn func()) {
	select {
	case f.slots <- struct{}{}:
	default:
		f.perdidas.Add(1)
		return
	}
	f.wg.Add(1)
	go func() {
		defer func() { f.wg.Done(); <-f.slots }()
		fn()
	}()
}
func (f *formaDescarte) Fechar()         { f.wg.Wait() }
func (f *formaDescarte) Perdidas() int64 { return f.perdidas.Load() }

// formaFila é o meio-termo: um pool fixo de workers consome de uma fila
// limitada POR BYTES. O handler só é segurado se a fila encher, e a fila
// absorve rajada curta sem multiplicar goroutine.
//
// O limite é em bytes porque item não é unidade de memória: ver o comentário
// do contrato acima.
// Na BORDA (fila cheia) há duas políticas possíveis, e a medição da rajada
// pequena não distingue as duas porque a fila nunca encheu. `waitFull` escolhe:
//
//	waitFull=false — descarta o excedente. O handler nunca para; o usuário perde.
//	waitFull=true  — segura o chamador SÓ quando cheia. Não perde; volta a ter
//	               backpressure sobre o handler, mas apenas na borda.
type formaFila struct {
	jobs          chan trabalho
	mu            sync.Mutex
	cond          *sync.Cond
	bytesInFlight int64
	bytesBudget   int64
	// peakBytes é a marca d'água: quanto a fila REALMENTE precisou. Sem ela
	// só dá para dizer "com 32MB não segurou", que não calibra nada — o que
	// calibra é saber que a rajada pediu 17MB e não 31.
	peakBytes int64
	waitFull  bool
	perdidas  atomic.Int64
	workers   sync.WaitGroup
	fechaUma  sync.Once
}

type trabalho struct {
	tamanho int
	fn      func()
}

func novaFormaFila(workers, capItens int, bytesBudget int64, waitFull bool) *formaFila {
	f := &formaFila{
		jobs:        make(chan trabalho, capItens),
		bytesBudget: bytesBudget,
		waitFull:    waitFull,
	}
	f.cond = sync.NewCond(&f.mu)
	for i := 0; i < workers; i++ {
		f.workers.Add(1)
		go func() {
			defer f.workers.Done()
			for t := range f.jobs {
				t.fn()
				f.mu.Lock()
				f.bytesInFlight -= int64(t.tamanho)
				f.mu.Unlock()
				f.cond.Broadcast()
			}
		}()
	}
	return f
}

func (f *formaFila) Despachar(tamanho int, fn func()) {
	// Duas guardas: bytes em voo e capacidade do canal. A de bytes é a que
	// protege a memória; a do canal é o que impede a fila de crescer sem fim
	// quando os payloads são pequenos.
	f.mu.Lock()
	// Um item maior que o orçamento inteiro passa mesmo assim: senão o
	// evento de 120KB medido nesta instalação ficaria preso para sempre com
	// orçamento pequeno, esperando por espaço que nunca chega.
	if int64(tamanho) <= f.bytesBudget {
		for f.bytesInFlight+int64(tamanho) > f.bytesBudget {
			if !f.waitFull {
				f.mu.Unlock()
				f.perdidas.Add(1)
				return
			}
			// sync.Cond e não aquisição de N fichas num canal: com N fichas e
			// MAIS DE UM produtor (em produção há um handler por sessão), dois
			// chamadores podem ficar cada um com metade e nenhum completar.
			f.cond.Wait()
		}
	}
	f.bytesInFlight += int64(tamanho)
	if f.bytesInFlight > f.peakBytes {
		f.peakBytes = f.bytesInFlight
	}
	f.mu.Unlock()

	if f.waitFull {
		f.jobs <- trabalho{tamanho: tamanho, fn: fn}
		return
	}
	select {
	case f.jobs <- trabalho{tamanho: tamanho, fn: fn}:
	default:
		f.mu.Lock()
		f.bytesInFlight -= int64(tamanho)
		f.mu.Unlock()
		f.cond.Broadcast()
		f.perdidas.Add(1)
	}
}

func (f *formaFila) Fechar() {
	f.fechaUma.Do(func() { close(f.jobs) })
	f.workers.Wait()
}
func (f *formaFila) Perdidas() int64 { return f.perdidas.Load() }

// PicoBytes é a marca d'água de bytes em voo. Só a fila tem — as outras
// formas não têm orçamento para calibrar.
func (f *formaFila) PicoBytes() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.peakBytes
}

// --- distribuição de payload medida em produção ---------------------------

// tamanhosReais reproduz a distribuição observada em 20.000 eventos desta
// instalação: mediana 2.089B, média 2.899B, p90 3.998B, p99 10.094B, máx
// 120.302B.
//
// Payload de tamanho FIXO mediria outra coisa: com 8KB constante a fila por
// bytes e a fila por itens se comportam igual, e a diferença entre as duas é
// justamente o que precisa aparecer.
func tamanhosReais() []int {
	r := rand.New(rand.NewSource(1)) // semente fixa: medição tem de ser repetível
	var t []int
	for i := 0; i < 50; i++ {
		t = append(t, 1500+r.Intn(600)) // até a mediana
	}
	for i := 0; i < 40; i++ {
		t = append(t, 2100+r.Intn(1900)) // mediana → p90
	}
	for i := 0; i < 9; i++ {
		t = append(t, 4000+r.Intn(6100)) // p90 → p99
	}
	t = append(t, 120302) // a cauda: 40× a mediana, e ela existe
	r.Shuffle(len(t), func(i, j int) { t[i], t[j] = t[j], t[i] })
	return t
}

// --- a medição ------------------------------------------------------------

type medidaHandler struct {
	forma          string
	handlerTotal   time.Duration // quanto o handler levou para processar tudo
	atrasoP50      time.Duration // atraso por evento, mediana
	atrasoP95      time.Duration
	atrasoMax      time.Duration
	drenoTotal     time.Duration // quanto levou até a última entrega terminar
	picoGoroutines int
	picoHeapMB     float64
	perdidas       int64
	entregas       int
}

// medirHandler simula o handler de eventos: UMA goroutine, eventos em
// sequência, cada evento com um pouco de trabalho próprio antes de despachar.
//
// O trabalho próprio (`custoProprio`) não é enfeite: sem ele o handler é só um
// laço de despacho, e o atraso induzido pareceria dominar 100% do tempo. Em
// produção o handler decodifica protobuf, grava no banco e só então despacha.
func medirHandler(nome string, d despachante, eventos, porEvento int, tamanhos []int, custoProprio time.Duration, entregar func(payload []byte)) medidaHandler {
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

	atrasos := make([]time.Duration, 0, eventos)
	inicio := time.Now()

	// O HANDLER: uma única goroutine, sequencial. É este o ponto todo.
	for i := 0; i < eventos; i++ {
		// trabalho próprio do handler antes de despachar
		fimProprio := time.Now().Add(custoProprio)
		for time.Now().Before(fimProprio) {
		}

		antes := time.Now()
		for j := 0; j < porEvento; j++ {
			tam := tamanhos[(i*porEvento+j)%len(tamanhos)]
			// O payload é alocado AQUI, pelo handler, e mantido vivo pela
			// closure até a entrega — como em produção, onde o postmap é
			// construído no handler e a closure de despacho o segura.
			//
			// A primeira versão fatiava um array compartilhado
			// (`corpo[:tam]`). Com isso os itens na fila não seguravam
			// memória própria, e o heap medido ficou CONSTANTE em ~25MB de
			// 1MB a 32MB de orçamento — número que parecia dado e dizia
			// apenas que o instrumento não alocava. Ver ARMADILHAS.md 14.
			payload := make([]byte, tam)
			d.Despachar(tam, func() { entregar(payload) })
		}
		atrasos = append(atrasos, time.Since(antes))
	}
	handlerTotal := time.Since(inicio)

	d.Fechar()
	dreno := time.Since(inicio)

	close(parar)
	am.Wait()

	sort.Slice(atrasos, func(i, j int) bool { return atrasos[i] < atrasos[j] })
	pct := func(p float64) time.Duration {
		if len(atrasos) == 0 {
			return 0
		}
		idx := int(float64(len(atrasos)-1) * p)
		return atrasos[idx]
	}

	return medidaHandler{
		forma:          nome,
		handlerTotal:   handlerTotal,
		atrasoP50:      pct(0.50),
		atrasoP95:      pct(0.95),
		atrasoMax:      atrasos[len(atrasos)-1],
		drenoTotal:     dreno,
		picoGoroutines: int(picoGo.Load()),
		picoHeapMB:     float64(picoHeap.Load()) / (1024 * 1024),
		perdidas:       d.Perdidas(),
		entregas:       eventos * porEvento,
	}
}

// TestMedicaoAtrasoNoHandler compara as quatro formas de aquisição sob a mesma
// rajada, na mesma execução.
//
// A única asserção é a que não depende de máquina: a forma que descarta NÃO
// pode atrasar o handler mais que a que bloqueia. Se isso deixar de valer, o
// harness está medindo ruído e nenhum número dele vale.
func TestMedicaoAtrasoNoHandler(t *testing.T) {
	exigirModoMedicao(t)

	const latenciaServidor = 100 * time.Millisecond
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		time.Sleep(latenciaServidor)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(make([]byte, 4096))
	}))
	defer srv.Close()

	tamanhos := tamanhosReais()
	entregar := func(payload []byte) {
		resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(payload))
		if err != nil {
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}

	const (
		eventos      = 500
		porEvento    = 4 // webhook do usuário, WS, webhook global, RabbitMQ
		teto         = 64
		custoProprio = 200 * time.Microsecond // decodificar + gravar
	)

	formas := []struct {
		nome string
		novo func() despachante
	}{
		{"sem-teto", func() despachante { return &formaSemTeto{} }},
		{"bloqueia-64", func() despachante { return novaFormaBloqueante(teto) }},
		{"descarta-64", func() despachante { return novaFormaDescarte(teto) }},
		{"fila-64w-8MB", func() despachante { return novaFormaFila(teto, 2048, 8*1024*1024, false) }},
	}

	const repeticoes = 3
	acumulado := map[string][]medidaHandler{}

	for r := 0; r < repeticoes; r++ {
		for _, f := range formas {
			runtime.GC()
			m := medirHandler(f.nome, f.novo(), eventos, porEvento, tamanhos, custoProprio, entregar)
			acumulado[f.nome] = append(acumulado[f.nome], m)
			t.Logf("rodada %d | %-13s handler=%-9s atraso_p50=%-9s atraso_p95=%-9s atraso_max=%-9s dreno=%-9s goroutines=%-6d heap=%.1fMB perdidas=%d/%d",
				r+1, m.forma,
				m.handlerTotal.Round(time.Millisecond),
				m.atrasoP50.Round(time.Microsecond),
				m.atrasoP95.Round(time.Microsecond),
				m.atrasoMax.Round(time.Microsecond),
				m.drenoTotal.Round(time.Millisecond),
				m.picoGoroutines, m.picoHeapMB, m.perdidas, m.entregas)
		}
	}

	t.Log("")
	t.Log("=== mediana das 3 rodadas ===")
	for _, f := range formas {
		ms := acumulado[f.nome]
		mediana := func(sel func(medidaHandler) time.Duration) time.Duration {
			v := make([]time.Duration, len(ms))
			for i, m := range ms {
				v[i] = sel(m)
			}
			sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
			return v[len(v)/2]
		}
		var perdidas int64
		var heap float64
		var goroutines int
		for _, m := range ms {
			perdidas += m.perdidas
			heap += m.picoHeapMB
			goroutines += m.picoGoroutines
		}
		t.Logf("%-13s handler=%-9s atraso_p50=%-9s atraso_p95=%-9s dreno=%-9s goroutines~%d heap~%.1fMB perdidas=%d/%d",
			f.nome,
			mediana(func(m medidaHandler) time.Duration { return m.handlerTotal }).Round(time.Millisecond),
			mediana(func(m medidaHandler) time.Duration { return m.atrasoP50 }).Round(time.Microsecond),
			mediana(func(m medidaHandler) time.Duration { return m.atrasoP95 }).Round(time.Microsecond),
			mediana(func(m medidaHandler) time.Duration { return m.drenoTotal }).Round(time.Millisecond),
			goroutines/len(ms), heap/float64(len(ms)),
			perdidas, int64(eventos*porEvento)*int64(len(ms)))
	}

	// Asserção de sanidade do INSTRUMENTO, não do desenho.
	medianaHandler := func(nome string) time.Duration {
		ms := acumulado[nome]
		v := make([]time.Duration, len(ms))
		for i, m := range ms {
			v[i] = m.handlerTotal
		}
		sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
		return v[len(v)/2]
	}
	if medianaHandler("descarta-64") >= medianaHandler("bloqueia-64") {
		t.Errorf("descartar (%s) nao ficou mais rapido que bloquear (%s) no handler: o harness nao esta medindo o atraso induzido",
			medianaHandler("descarta-64"), medianaHandler("bloqueia-64"))
	}
}

// TestMedicaoFilaNaSaturacao mede a BORDA, que a medição anterior não tocou.
//
// Em TestMedicaoAtrasoNoHandler a fila teve zero perdas — mas a rajada inteira
// (2.000 entregas, ~4,9MB) COUBE no orçamento de 8MB e nos 2.048 itens de
// capacidade. "Não perdeu" ali significa "não encheu", e não diz nada sobre o
// que acontece quando enche. Aqui a rajada é deliberadamente maior que o
// orçamento, que é a única condição em que a política de borda importa.
//
// Rode com:
//
//	go test ./pkg/bootstrap/ -run TestMedicaoFilaNaSaturacao -v -timeout 20m
func TestMedicaoFilaNaSaturacao(t *testing.T) {
	exigirModoMedicao(t)

	const latenciaServidor = 100 * time.Millisecond
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		time.Sleep(latenciaServidor)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(make([]byte, 4096))
	}))
	defer srv.Close()

	tamanhos := tamanhosReais()
	entregar := func(payload []byte) {
		resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(payload))
		if err != nil {
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}

	const (
		eventos      = 2000
		porEvento    = 4
		teto         = 64
		custoProprio = 200 * time.Microsecond
	)
	// 8.000 entregas × ~2,4KB ≈ 19MB de payload: mais que o dobro do
	// orçamento de 8MB, então a fila enche de fato.
	formas := []struct {
		nome string
		novo func() despachante
	}{
		{"sem-teto", func() despachante { return &formaSemTeto{} }},
		{"fila-8MB-descarta", func() despachante { return novaFormaFila(teto, 2048, 8*1024*1024, false) }},
		{"fila-8MB-waitFull", func() despachante { return novaFormaFila(teto, 2048, 8*1024*1024, true) }},
		{"fila-32MB-waitFull", func() despachante { return novaFormaFila(teto, 8192, 32*1024*1024, true) }},
	}

	const repeticoes = 3
	acumulado := map[string][]medidaHandler{}
	for r := 0; r < repeticoes; r++ {
		for _, f := range formas {
			runtime.GC()
			m := medirHandler(f.nome, f.novo(), eventos, porEvento, tamanhos, custoProprio, entregar)
			acumulado[f.nome] = append(acumulado[f.nome], m)
			t.Logf("rodada %d | %-18s handler=%-9s atraso_p50=%-9s atraso_p95=%-9s atraso_max=%-9s dreno=%-9s goroutines=%-6d heap=%.1fMB perdidas=%d/%d",
				r+1, m.forma,
				m.handlerTotal.Round(time.Millisecond),
				m.atrasoP50.Round(time.Microsecond),
				m.atrasoP95.Round(time.Microsecond),
				m.atrasoMax.Round(time.Microsecond),
				m.drenoTotal.Round(time.Millisecond),
				m.picoGoroutines, m.picoHeapMB, m.perdidas, m.entregas)
		}
	}

	t.Log("")
	t.Log("=== mediana das 3 rodadas (fila SATURADA) ===")
	for _, f := range formas {
		ms := acumulado[f.nome]
		mediana := func(sel func(medidaHandler) time.Duration) time.Duration {
			v := make([]time.Duration, len(ms))
			for i, m := range ms {
				v[i] = sel(m)
			}
			sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
			return v[len(v)/2]
		}
		var perdidas int64
		var heap float64
		var goroutines int
		for _, m := range ms {
			perdidas += m.perdidas
			heap += m.picoHeapMB
			goroutines += m.picoGoroutines
		}
		t.Logf("%-18s handler=%-9s atraso_p50=%-9s atraso_p95=%-9s dreno=%-9s goroutines~%d heap~%.1fMB perdidas=%d/%d (%.1f%%)",
			f.nome,
			mediana(func(m medidaHandler) time.Duration { return m.handlerTotal }).Round(time.Millisecond),
			mediana(func(m medidaHandler) time.Duration { return m.atrasoP50 }).Round(time.Microsecond),
			mediana(func(m medidaHandler) time.Duration { return m.atrasoP95 }).Round(time.Microsecond),
			mediana(func(m medidaHandler) time.Duration { return m.drenoTotal }).Round(time.Millisecond),
			goroutines/len(ms), heap/float64(len(ms)),
			perdidas, int64(eventos*porEvento)*int64(len(ms)),
			100*float64(perdidas)/float64(int64(eventos*porEvento)*int64(len(ms))))
	}

	// Asserção do INSTRUMENTO: a rajada precisa ter ESTOURADO a fila, senão
	// este teste mede a mesma coisa que o anterior e a comparação é vazia.
	var perdaDescarte int64
	for _, m := range acumulado["fila-8MB-descarta"] {
		perdaDescarte += m.perdidas
	}
	if perdaDescarte == 0 {
		t.Fatal("fila-8MB-descarta nao perdeu nada: a rajada nao saturou a fila, entao esta medicao nao mede a borda")
	}
	// A variante que waitFull não pode perder: se perdeu, a política de borda
	// não está implementada como descrita.
	for _, m := range acumulado["fila-8MB-waitFull"] {
		if m.perdidas != 0 {
			t.Errorf("fila-8MB-waitFull perdeu %d entregas; a politica de waitFull nao pode descartar", m.perdidas)
		}
	}
}

// TestMedicaoWorkersVsDreno separa os dois botões que eu vinha tratando como
// um só.
//
// TestMedicaoFilaNaSaturacao mostrou dreno de 12,5s contra 557ms sem teto, e é
// tentador atribuir isso "à fila". Não é da fila: é do número de WORKERS. A
// fila decide quanta rajada cabe antes de o handler ser segurado; o pool
// decide quão rápido a rajada escoa. Com orçamento folgado (32MB, acima dos
// ~19MB da rajada) o handler não é segurado em nenhuma das pernas, e o que
// sobra medido é só o efeito do pool.
//
// bloqueia-N entra como referência direta: se o pool de N workers e o teto
// bloqueante de N derem o mesmo dreno, fica provado que o custo de entrega é o
// mesmo e a única diferença entre as duas formas é quem waitFull.
//
// Rode com:
//
//	go test ./pkg/bootstrap/ -run TestMedicaoWorkersVsDreno -v -timeout 25m
func TestMedicaoWorkersVsDreno(t *testing.T) {
	exigirModoMedicao(t)

	const latenciaServidor = 100 * time.Millisecond
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		time.Sleep(latenciaServidor)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(make([]byte, 4096))
	}))
	defer srv.Close()

	tamanhos := tamanhosReais()
	entregar := func(payload []byte) {
		resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(payload))
		if err != nil {
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}

	const (
		eventos      = 2000
		porEvento    = 4
		custoProprio = 200 * time.Microsecond
		orcamento    = 32 * 1024 * 1024 // folgado de proposito: isola o pool
	)

	formas := []struct {
		nome string
		novo func() despachante
	}{
		{"sem-teto", func() despachante { return &formaSemTeto{} }},
		{"fila-64w", func() despachante { return novaFormaFila(64, 8192, orcamento, true) }},
		{"fila-128w", func() despachante { return novaFormaFila(128, 8192, orcamento, true) }},
		{"fila-256w", func() despachante { return novaFormaFila(256, 8192, orcamento, true) }},
		{"fila-512w", func() despachante { return novaFormaFila(512, 8192, orcamento, true) }},
		{"bloqueia-256", func() despachante { return novaFormaBloqueante(256) }},
	}

	const repeticoes = 3
	acumulado := map[string][]medidaHandler{}
	for r := 0; r < repeticoes; r++ {
		for _, f := range formas {
			runtime.GC()
			m := medirHandler(f.nome, f.novo(), eventos, porEvento, tamanhos, custoProprio, entregar)
			acumulado[f.nome] = append(acumulado[f.nome], m)
			t.Logf("rodada %d | %-13s handler=%-9s atraso_p95=%-9s dreno=%-9s goroutines=%-6d heap=%.1fMB",
				r+1, m.forma,
				m.handlerTotal.Round(time.Millisecond),
				m.atrasoP95.Round(time.Microsecond),
				m.drenoTotal.Round(time.Millisecond),
				m.picoGoroutines, m.picoHeapMB)
		}
	}

	t.Log("")
	t.Log("=== mediana das 3 rodadas (orcamento folgado: so o pool varia) ===")
	for _, f := range formas {
		ms := acumulado[f.nome]
		mediana := func(sel func(medidaHandler) time.Duration) time.Duration {
			v := make([]time.Duration, len(ms))
			for i, m := range ms {
				v[i] = sel(m)
			}
			sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
			return v[len(v)/2]
		}
		var heap float64
		var goroutines int
		for _, m := range ms {
			heap += m.picoHeapMB
			goroutines += m.picoGoroutines
		}
		t.Logf("%-13s handler=%-9s atraso_p95=%-9s dreno=%-9s goroutines~%d heap~%.1fMB",
			f.nome,
			mediana(func(m medidaHandler) time.Duration { return m.handlerTotal }).Round(time.Millisecond),
			mediana(func(m medidaHandler) time.Duration { return m.atrasoP95 }).Round(time.Microsecond),
			mediana(func(m medidaHandler) time.Duration { return m.drenoTotal }).Round(time.Millisecond),
			goroutines/len(ms), heap/float64(len(ms)))
	}

	// Asserção do INSTRUMENTO: mais workers têm de drenar mais rápido. Se não
	// drenarem, o gargalo está no harness (servidor, transporte) e não no
	// pool — e aí nenhum número desta tabela fala sobre o pool.
	dreno := func(nome string) time.Duration {
		ms := acumulado[nome]
		v := make([]time.Duration, len(ms))
		for i, m := range ms {
			v[i] = m.drenoTotal
		}
		sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
		return v[len(v)/2]
	}
	if dreno("fila-256w") >= dreno("fila-64w") {
		t.Errorf("256 workers (%s) nao drenaram mais rapido que 64 (%s): o gargalo nao e' o pool, e esta tabela nao mede o que diz medir",
			dreno("fila-256w"), dreno("fila-64w"))
	}
}

// TestMedicaoCalibracaoOrcamento calibra o TAMANHO da fila.
//
// As medições anteriores usaram dois orçamentos extremos de propósito: 8MB
// (apertado, handler segurado por 9,2s) e 32MB (folgado, handler intacto).
// Isso prova que o orçamento importa, e não diz onde fica o joelho da curva —
// que é a única coisa que permite escolher um número.
//
// Duas varreduras, com o pool FIXO em 256 workers para não misturar os botões:
//
//	A) orçamento variável, rajada fixa — onde o handler começa a ser segurado
//	B) rajada variável, orçamento fixo — se um orçamento "suficiente" existe
//
// A varredura B é a que decide se dá para escolher um número e esquecer, ou se
// o orçamento tem de acompanhar o tamanho da rajada. Se o handler produz muito
// mais rápido do que o pool escoa, a fila tende a precisar da rajada INTEIRA,
// e aí não existe número que sempre baste — só a escolha de quanto atraso se
// aceita. A marca d'água (PicoBytes) responde isso com dado.
//
// Rode com:
//
//	go test ./pkg/bootstrap/ -run TestMedicaoCalibracaoOrcamento -v -timeout 25m
func TestMedicaoCalibracaoOrcamento(t *testing.T) {
	exigirModoMedicao(t)

	const latenciaServidor = 100 * time.Millisecond
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		time.Sleep(latenciaServidor)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(make([]byte, 4096))
	}))
	defer srv.Close()

	tamanhos := tamanhosReais()
	entregar := func(payload []byte) {
		resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(payload))
		if err != nil {
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}

	const (
		porEvento    = 4
		workers      = 256 // fixo: o botao sob medida agora e' o orcamento
		custoProprio = 200 * time.Microsecond
		capItens     = 65536 // alto de proposito: quem limita tem de ser o BYTE
		repeticoes   = 3
	)

	const mb = 1024 * 1024

	// Uma perna: roda `repeticoes` vezes e devolve as medianas + marca d'água.
	rodar := func(eventos int, orcamento int64) (medidaHandler, int64) {
		var ms []medidaHandler
		var peakBytes int64
		for r := 0; r < repeticoes; r++ {
			runtime.GC()
			f := novaFormaFila(workers, capItens, orcamento, true)
			m := medirHandler("", f, eventos, porEvento, tamanhos, custoProprio, entregar)
			if p := f.PicoBytes(); p > peakBytes {
				peakBytes = p
			}
			ms = append(ms, m)
		}
		med := func(sel func(medidaHandler) time.Duration) time.Duration {
			v := make([]time.Duration, len(ms))
			for i, m := range ms {
				v[i] = sel(m)
			}
			sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
			return v[len(v)/2]
		}
		var heap float64
		var perdidas int64
		for _, m := range ms {
			heap += m.picoHeapMB
			perdidas += m.perdidas
		}
		return medidaHandler{
			handlerTotal: med(func(m medidaHandler) time.Duration { return m.handlerTotal }),
			atrasoP95:    med(func(m medidaHandler) time.Duration { return m.atrasoP95 }),
			atrasoMax:    med(func(m medidaHandler) time.Duration { return m.atrasoMax }),
			drenoTotal:   med(func(m medidaHandler) time.Duration { return m.drenoTotal }),
			picoHeapMB:   heap / float64(len(ms)),
			perdidas:     perdidas,
			entregas:     eventos * porEvento * repeticoes,
		}, peakBytes
	}

	// --- A) orçamento variável, rajada fixa ---
	const eventosFixos = 2000 // 8.000 entregas, ~19MB de payload
	t.Logf("=== A) orcamento variavel | rajada fixa: %d entregas | pool %dw ===", eventosFixos*porEvento, workers)
	t.Logf("%-10s %-10s %-10s %-10s %-10s %-10s %s", "orcamento", "handler", "atraso_p95", "atraso_max", "dreno", "heap", "pico_fila")
	for _, orc := range []int64{1 * mb, 2 * mb, 4 * mb, 8 * mb, 16 * mb, 24 * mb, 32 * mb} {
		m, peak := rodar(eventosFixos, orc)
		t.Logf("%-10s %-10s %-10s %-10s %-10s %-10s %.1fMB",
			fmt.Sprintf("%dMB", orc/mb),
			m.handlerTotal.Round(time.Millisecond),
			m.atrasoP95.Round(time.Microsecond),
			m.atrasoMax.Round(time.Microsecond),
			m.drenoTotal.Round(time.Millisecond),
			fmt.Sprintf("%.1fMB", m.picoHeapMB),
			float64(peak)/mb)
	}

	// --- B) rajada variável, orçamento fixo ---
	const orcamentoFixo = int64(16 * mb)
	t.Log("")
	t.Logf("=== B) rajada variavel | orcamento fixo: %dMB | pool %dw ===", orcamentoFixo/mb, workers)
	t.Logf("%-10s %-10s %-10s %-10s %-10s %s", "entregas", "handler", "atraso_p95", "dreno", "heap", "pico_fila")
	for _, ev := range []int{500, 1000, 2000, 4000} {
		m, peak := rodar(ev, orcamentoFixo)
		t.Logf("%-10d %-10s %-10s %-10s %-10s %.1fMB",
			ev*porEvento,
			m.handlerTotal.Round(time.Millisecond),
			m.atrasoP95.Round(time.Microsecond),
			m.drenoTotal.Round(time.Millisecond),
			fmt.Sprintf("%.1fMB", m.picoHeapMB),
			float64(peak)/mb)
	}

	// Asserção do INSTRUMENTO: com 1MB o handler TEM de ser segurado, senão a
	// varredura A nao exercitou a borda e a curva nao existe.
	apertado, _ := rodar(eventosFixos, 1*mb)
	folgado, _ := rodar(eventosFixos, 32*mb)
	if apertado.handlerTotal <= folgado.handlerTotal {
		t.Errorf("handler com 1MB (%s) nao ficou mais lento que com 32MB (%s): o orcamento nao esta limitando nada",
			apertado.handlerTotal, folgado.handlerTotal)
	}
}
