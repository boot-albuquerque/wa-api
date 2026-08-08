package bootstrap

import (
	"os"
	"runtime/debug"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

// Despacho de eventos com pool e fila limitada por BYTES (F86).
//
// Um evento de domínio dispara até quatro entregas — WebSocket, webhook do
// usuário, webhook global e RabbitMQ — e cada uma faz E/S de rede. Antes disto
// cada entrega era uma goroutine solta: `SafeGo` é `go func()` com `recover`,
// que protege contra pânico e não contra volume.
//
// O desenho é uma ESCADA de degradação, medida e não suposta. Ver F86 no
// HOUSEKEEP.md para as tabelas completas:
//
//	rajada <= pool       — indistinguível de não ter mecanismo nenhum
//	rajada  > pool       — enfileira: entrega mais lenta, COMPLETA
//	rajada  > orçamento  — o handler absorve backpressure, ainda SEM PERDA
//
// Em nenhum degrau se descarta entrega. Isso é escolha, e é a razão de o
// caminho de saturação bloquear em vez de jogar fora: descartar com teto 64
// perdeu 93,6% das entregas na medição, e 70,5% mesmo com fila de 8MB.
//
// Não há detector de rajada nem troca de modo. Ele seria um limiar a calibrar,
// histerese a acertar e oscilação na fronteira a depurar — para produzir o
// comportamento que a escada acima já produz sozinha.

const (
	// envDispatchWorkers substitui WA_API_DISPATCH_MAX_CONCURRENCY, que era o
	// teto bloqueante da primeira tentativa. O nome mudou porque a coisa
	// mudou: não é mais um teto sobre o chamador, é o tamanho do pool.
	envDispatchWorkers    = "WA_API_DISPATCH_WORKERS"
	envDispatchQueueBytes = "WA_API_DISPATCH_QUEUE_BYTES"

	// dispatchDefaultWorkers vem da rajada medida em produção: o pico de
	// pareamento foi de 23 eventos em 100ms, que são 92 entregas. Com 256
	// workers esse pico passa direto, sem enfileirar nada — o mecanismo só
	// aparece a partir de ~3 pareamentos simultâneos.
	dispatchDefaultWorkers = 256

	// dispatchDefaultQueueBytes vem da marca d'água medida: uma rajada de
	// 8.000 entregas (~87 pareamentos simultâneos) segurou 26,8MB de pico.
	// Com 32MB o handler não é tocado até essa escala.
	//
	// A unidade é BYTE e não item de propósito: os payloads medidos nesta
	// instalação vão de 1,5KB a 120KB — o máximo é 40× a mediana, e uma fila
	// de N itens teria teto de memória 40× diferente conforme a sorte do lote.
	dispatchDefaultQueueBytes = 32 * 1024 * 1024

	// dispatchQueueCapItems é guarda secundária, não o limite principal. Ela
	// existe para o caso degenerado de payloads minúsculos, em que o
	// orçamento de bytes sozinho deixaria a fila crescer em NÚMERO sem
	// crescer em bytes — cada item ainda custa um descritor e uma closure.
	dispatchQueueCapItems = 65536

	// dispatchAvisoEsperaAcumulada é quanto de espera acumulada no handler
	// justifica um aviso. Abaixo disso, esperar é o mecanismo funcionando
	// como projetado e logar seria ruído.
	dispatchAvisoEsperaAcumulada = 500 * time.Millisecond
)

// dispatchJob é uma entrega enfileirada. `bytes` é o tamanho do payload que a
// closure mantém vivo, e é o que a contabilidade do orçamento usa.
type dispatchJob struct {
	nome  string
	bytes int
	fn    func()
}

// dispatchPool é o pool de entrega com fila limitada por bytes.
type dispatchPool struct {
	jobs chan dispatchJob

	// mu protege bytesAtu/picoBytes E é o Locker de cond. A contabilidade de
	// bytes não pode ser atômica solta: quem espera precisa ser acordado
	// quando um worker devolve espaço, e isso pede condição, não contador.
	mu        sync.Mutex
	cond      *sync.Cond
	bytesAtu  int64
	bytesMax  int64
	picoBytes int64

	// Observabilidade. Sem ela não há como responder "a fila encostou no
	// teto?" senão por especulação — que é o que esta implementação inteira
	// existe para evitar.
	emVoo        atomic.Int64
	pico         atomic.Int64
	esperas      atomic.Int64
	esperaNanos  atomic.Int64
	avisouEspera atomic.Bool
}

func newDispatchPool(workers int, bytesMax int64) *dispatchPool {
	if workers <= 0 {
		return &dispatchPool{} // jobs nil: mecanismo desligado
	}
	p := &dispatchPool{
		jobs:     make(chan dispatchJob, dispatchQueueCapItems),
		bytesMax: bytesMax,
	}
	p.cond = sync.NewCond(&p.mu)
	for i := 0; i < workers; i++ {
		go p.trabalhar()
	}
	return p
}

func (p *dispatchPool) trabalhar() {
	for j := range p.jobs {
		p.executar(j)
	}
}

// executar roda uma entrega e SEMPRE devolve o espaço que ela ocupava.
//
// O recover é POR TRABALHO, e não por goroutine como em SafeGo. A diferença
// importa num pool permanente: SafeGo recupera o pânico e a goroutine morre em
// seguida, o que aqui encolheria o pool a cada pânico até não sobrar worker.
func (p *dispatchPool) executar(j dispatchJob) {
	defer func() {
		p.emVoo.Add(-1)
		p.mu.Lock()
		p.bytesAtu -= int64(j.bytes)
		p.mu.Unlock()
		p.cond.Broadcast()
		if r := recover(); r != nil {
			log.Error().
				Str("goroutine", j.nome).
				Interface("panic", r).
				Str("stack", string(debug.Stack())).
				Msg("panic recovered in dispatch worker")
		}
	}()

	atual := p.emVoo.Add(1)
	for {
		pico := p.pico.Load()
		if atual <= pico || p.pico.CompareAndSwap(pico, atual) {
			break
		}
	}
	j.fn()
}

// Go enfileira uma entrega. `bytes` é o tamanho do payload que fn mantém vivo.
//
// Com o pool desligado delega a safeGo, e o comportamento é idêntico ao
// anterior à F86 — é o que mantém o A/B possível e o rollback a uma variável
// de ambiente.
func (p *dispatchPool) Go(nome string, bytes int, fn func()) {
	if p == nil || p.jobs == nil {
		safeGo(nome, fn)
		return
	}

	inicio := time.Now()
	p.mu.Lock()
	// Um item maior que o orçamento inteiro passa mesmo assim. Sem esta
	// guarda, o evento de 120KB medido nesta instalação ficaria preso para
	// sempre num orçamento pequeno, esperando espaço que nunca chega — o
	// mecanismo de proteção viraria um deadlock.
	if int64(bytes) <= p.bytesMax {
		for p.bytesAtu+int64(bytes) > p.bytesMax {
			p.esperas.Add(1)
			// sync.Cond, e não N fichas num canal: com fichas e MAIS DE UM
			// produtor — em produção há um handler por sessão — dois
			// chamadores podem ficar cada um com metade e nenhum completar.
			p.cond.Wait()
		}
	}
	p.bytesAtu += int64(bytes)
	if p.bytesAtu > p.picoBytes {
		p.picoBytes = p.bytesAtu
	}
	p.mu.Unlock()

	p.jobs <- dispatchJob{nome: nome, bytes: bytes, fn: fn}

	if espera := time.Since(inicio); espera > 0 {
		total := p.esperaNanos.Add(int64(espera))
		// O aviso sai UMA vez: quem chama é a goroutine do handler de eventos,
		// e logar por ocorrência transformaria a saturação numa segunda
		// rajada, agora de log. Ver F85.
		if total > int64(dispatchAvisoEsperaAcumulada) && p.avisouEspera.CompareAndSwap(false, true) {
			log.Warn().
				Dur("espera_acumulada", time.Duration(total)).
				Int64("orcamento_bytes", p.bytesMax).
				Msg("fila de despacho saturada: o handler de eventos esta sendo segurado")
		}
	}
}

// Metricas devolve o estado observado. Usada pelos testes e pela medição.
func (p *dispatchPool) Metricas() (emVoo, pico, esperas, picoBytes int64) {
	if p == nil {
		return 0, 0, 0, 0
	}
	p.mu.Lock()
	pb := p.picoBytes
	p.mu.Unlock()
	return p.emVoo.Load(), p.pico.Load(), p.esperas.Load(), pb
}

var (
	dispatchOnce sync.Once
	dispatch     *dispatchPool
)

// dispatchGo é o ponto único por onde as quatro entregas passam.
func dispatchGo(nome string, bytes int, fn func()) {
	dispatchOnce.Do(func() {
		dispatch = newDispatchPool(dispatchWorkersConfigurados(), dispatchQueueBytesConfigurado())
	})
	dispatch.Go(nome, bytes, fn)
}

// dispatchWorkersConfigurados lê o tamanho do pool do ambiente. Zero DESLIGA o
// mecanismo — é o rollback, e é distinto de valor inválido.
func dispatchWorkersConfigurados() int {
	return lerInteiroDoAmbiente(envDispatchWorkers, dispatchDefaultWorkers)
}

// dispatchQueueBytesConfigurado lê o orçamento da fila do ambiente.
func dispatchQueueBytesConfigurado() int64 {
	return int64(lerInteiroDoAmbiente(envDispatchQueueBytes, dispatchDefaultQueueBytes))
}

// lerInteiroDoAmbiente devolve o padrão para valor ausente, inválido ou
// negativo, sempre com aviso em Warn nos dois últimos casos: um typo não pode
// mudar a proteção em silêncio, e com padrões NÃO-ZERO esse caminho é a
// diferença entre proteger e não proteger.
func lerInteiroDoAmbiente(nome string, padrao int) int {
	bruto := os.Getenv(nome)
	if bruto == "" {
		return padrao
	}
	n, err := strconv.Atoi(bruto)
	if err != nil {
		log.Warn().Str("valor", bruto).Str("var", nome).Int("usando", padrao).
			Msg("valor invalido; usando o padrao")
		return padrao
	}
	if n < 0 {
		log.Warn().Int("valor", n).Str("var", nome).Int("usando", padrao).
			Msg("valor negativo; usando o padrao")
		return padrao
	}
	return n
}
