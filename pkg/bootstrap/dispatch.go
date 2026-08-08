package bootstrap

import (
	"os"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog/log"
)

// Teto de concorrência do despacho de eventos (F86).
//
// Um evento de domínio dispara até quatro entregas — webhook do usuário,
// WebSocket, webhook global e RabbitMQ — e cada uma faz E/S de rede. Sem
// teto, uma rajada de HistorySync as multiplica sem limite: `SafeGo` é
// `go func()` com `recover`, que protege contra pânico e não contra volume.
//
// O teto NÃO muda o contrato: o despacho continua fire-and-forget, e quem
// chama não espera. O que muda é que a rajada passa a ser absorvida em ondas
// do tamanho do teto, em vez de virar N goroutines simultâneas em E/S.

// envDispatchMaxConcurrency é lido do .env. Zero desliga o teto — e hoje é o
// PADRÃO, enquanto a forma de aquisição não está decidida. Ver
// dispatchDefaultConcurrency.
const envDispatchMaxConcurrency = "WA_API_DISPATCH_MAX_CONCURRENCY"

// dispatchDefaultConcurrency é o teto quando a variável não está definida.
//
// ZERO — desligado — por decisão deliberada, e não por omissão.
//
// A aquisição deste limitador BLOQUEIA o chamador, e em produção o chamador é
// a goroutine do handler de eventos do SDK. Com o teto saturado, um webhook
// lento passaria a atrasar o processamento de TODOS os eventos da sessão,
// inclusive os que nem vão para webhook. Trocar "goroutines demais" por
// "handler de eventos parado" pode ser pior, e ainda não foi medido.
//
// Enquanto a forma de aquisição não estiver decidida (bloquear, descartar ou
// enfileirar — ver F86), o padrão não muda comportamento nenhum. Ligar é
// escolha explícita via WA_API_DISPATCH_MAX_CONCURRENCY, e é assim que a
// medição A/B roda.
const dispatchDefaultConcurrency = 0

// dispatchLimiter limita quantas entregas correm ao mesmo tempo.
//
// Um canal com buffer, e não um WaitGroup: o que se quer é um teto de
// ocupação, não esperar o fim. Aquisição bloqueante é deliberada — quem
// despacha é a goroutine do handler de eventos, e segurá-la por um instante
// é exatamente o backpressure que falta hoje.
type dispatchLimiter struct {
	slots chan struct{}

	// emVoo e pico são observabilidade, não controle. Sem eles não há como
	// responder "o teto foi atingido?" senão por especulação — que é
	// justamente o que esta medição existe para evitar.
	emVoo      atomic.Int64
	pico       atomic.Int64
	saturacoes atomic.Int64
}

func newDispatchLimiter(capacidade int) *dispatchLimiter {
	if capacidade <= 0 {
		return &dispatchLimiter{} // slots nil: sem teto
	}
	return &dispatchLimiter{slots: make(chan struct{}, capacidade)}
}

// Go executa fn em goroutine própria, respeitando o teto.
//
// Com slots nil (teto desligado) delega direto a safeGo, e o comportamento é
// idêntico ao anterior à F86 — é o que torna o A/B possível.
func (l *dispatchLimiter) Go(nome string, fn func()) {
	if l == nil || l.slots == nil {
		safeGo(nome, fn)
		return
	}

	// Tenta sem bloquear primeiro, só para saber se o teto foi atingido: a
	// contagem de saturação é o dado que diz se o teto está apertado demais.
	select {
	case l.slots <- struct{}{}:
	default:
		l.saturacoes.Add(1)
		l.slots <- struct{}{} // agora sim, bloqueando
	}

	atual := l.emVoo.Add(1)
	for {
		pico := l.pico.Load()
		if atual <= pico || l.pico.CompareAndSwap(pico, atual) {
			break
		}
	}

	// A liberação vai no defer DENTRO da goroutine, não fora: safeGo já tem
	// recover, mas um pânico antes do release vazaria o slot para sempre e
	// o teto encolheria a cada pânico até travar tudo.
	safeGo(nome, func() {
		defer func() {
			l.emVoo.Add(-1)
			<-l.slots
		}()
		fn()
	})
}

// Metricas devolve o estado observado. Usada pelos testes e pela medição.
func (l *dispatchLimiter) Metricas() (emVoo, pico, saturacoes int64) {
	if l == nil {
		return 0, 0, 0
	}
	return l.emVoo.Load(), l.pico.Load(), l.saturacoes.Load()
}

var (
	dispatchOnce sync.Once
	dispatch     *dispatchLimiter
)

// dispatchGo é o ponto único por onde as quatro entregas passam.
func dispatchGo(nome string, fn func()) {
	dispatchOnce.Do(func() { dispatch = newDispatchLimiter(dispatchConcurrencyConfigurada()) })
	dispatch.Go(nome, fn)
}

// dispatchConcurrencyConfigurada lê o teto do ambiente.
//
// Valor ausente ou inválido usa dispatchDefaultConcurrency, sempre com aviso
// no caso inválido — um typo não pode mudar a proteção em silêncio.
//
// ATENÇÃO ao mexer no padrão: enquanto ele for 0, "inválido" resulta em
// DESLIGADO, o que é inofensivo porque desligado já é o estado pretendido.
// No dia em que o padrão virar não-zero, esse mesmo caminho passa a ser a
// diferença entre proteger e não proteger, e o aviso em Warn é a única
// pista. TestConfig_InvalidoCaiNoPadrao trava a relação.
func dispatchConcurrencyConfigurada() int {
	bruto := os.Getenv(envDispatchMaxConcurrency)
	if bruto == "" {
		return dispatchDefaultConcurrency
	}
	n, err := strconv.Atoi(bruto)
	if err != nil {
		log.Warn().Str("valor", bruto).Str("var", envDispatchMaxConcurrency).
			Int("usando", dispatchDefaultConcurrency).
			Msg("teto de despacho invalido; usando o padrao")
		return dispatchDefaultConcurrency
	}
	if n < 0 {
		log.Warn().Int("valor", n).Str("var", envDispatchMaxConcurrency).
			Int("usando", dispatchDefaultConcurrency).
			Msg("teto de despacho negativo; usando o padrao")
		return dispatchDefaultConcurrency
	}
	return n
}
