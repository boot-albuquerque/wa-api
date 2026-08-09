package bootstrap

import (
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Fila serial de eventos por sessão (F87, opção A).
//
// # O defeito
//
// O `handleEvent` roda DENTRO do laço de nós do SDK, que é sequencial por
// sessão: ele só consome o próximo nó quando o corrente termina. E o nosso
// handler baixa mídia de forma síncrona (handleMessage -> processMessageMedia
// -> Download), com prazos de 1 a 10 minutos.
//
// Resultado medido: uma mensagem de mídia trava a fila de nós daquela sessão
// pelo tempo do download, e recibo, presença e marcação de leitura ficam atrás.
// Os 5,7s que apareceram em produção eram um download pequeno de
// `status@broadcast` — e status é alto volume, porque cada contato que publica
// um story enfileira um download.
//
// # Por que fila POR SESSÃO e não o pool compartilhado
//
// O pool da F86 já existe e seria mais barato. Mas ele não preserva ORDEM: um
// texto enviado depois de um vídeo poderia chegar ao webhook antes dele. Hoje
// o cliente tem essa garantia sem saber que tem — ela vem de graça do laço
// sequencial do SDK —, e tirá-la é o tipo de mudança que ninguém percebe até
// depender dela.
//
// Uma fila por sessão devolve o laço do SDK imediatamente E mantém a ordem.
//
// # Por que limitar por ITENS e não por bytes
//
// O contrário do que a F86 fez, e por um motivo concreto: aqui a fila guarda o
// evento ANTES do download. A mídia não está no item — o que está é uma
// referência à mensagem, com as chaves para baixar depois. Os itens são
// pequenos e de tamanho parecido, então contá-los mede o que interessa.
//
// # O que acontece quando a fila enche
//
// O envio BLOQUEIA. É deliberado: bloquear devolve o sistema ao comportamento
// de hoje (laço de nós parado) em vez de descartar evento. É a mesma escada da
// F86 — mais lento, nunca com perda.

const (
	// sessionQueueCapacity é quantos eventos cabem por sessão.
	//
	// O pareamento medido em produção são 129 eventos numa rajada; 1024 dá
	// quase 8x de folga sobre o pior caso conhecido. Como os itens são
	// pequenos (a mídia ainda não foi baixada), o custo de errar para mais é
	// baixo, e o de errar para menos é bloquear o laço de nós — que é
	// exatamente o que esta fila existe para evitar.
	sessionQueueCapacity = 1024

	// sessionEventSlowThreshold é a partir de quanto tempo o processamento de
	// um evento vira aviso.
	//
	// Isto NÃO é enfeite. O aviso `Node handling took` do SDK era a métrica que
	// media o nosso handler de graça, e tirar o trabalho do laço de nós cega
	// justamente o instrumento que provaria que esta mudança funcionou. O
	// limiar espelha o do SDK para que as duas séries continuem comparáveis.
	sessionEventSlowThreshold = 5 * time.Second
)

// sessionEventQueue é a fila de uma sessão.
type sessionEventQueue struct {
	fila chan func()
	// parada fecha quando a sessão é desmontada. O envio a observa: sem isso,
	// um produtor bloqueado numa fila cheia de sessão morta ficaria preso para
	// sempre — segurando o laço de nós do SDK, que é o oposto do objetivo.
	parada chan struct{}
	// uma vez garante que fechar duas vezes não entre em pânico. Detach é
	// idempotente por contrato, então o desligamento também tem de ser.
	uma sync.Once
}

var (
	sessionQueuesMu sync.Mutex
	sessionQueues   = map[string]*sessionEventQueue{}
)

// startSessionEventQueue cria a fila de userID e sobe seu worker.
//
// Idempotente: chamar de novo para uma sessão que já tem fila devolve a
// existente, sem criar worker órfão. Attach pode ser chamado mais de uma vez
// para o mesmo usuário (reconexão), e cada chamada extra criando um worker novo
// significaria dois consumidores da mesma sessão — e ordem perdida, que é
// justamente o que esta fila protege.
func startSessionEventQueue(userID string) *sessionEventQueue {
	sessionQueuesMu.Lock()
	defer sessionQueuesMu.Unlock()

	if q, existe := sessionQueues[userID]; existe {
		return q
	}

	q := &sessionEventQueue{
		fila:   make(chan func(), sessionQueueCapacity),
		parada: make(chan struct{}),
	}
	sessionQueues[userID] = q

	safeGo("session-event-queue-"+userID, func() { q.consumir(userID) })
	return q
}

// consumir processa os eventos em ordem, até a sessão ser desmontada.
func (q *sessionEventQueue) consumir(userID string) {
	for {
		select {
		case <-q.parada:
			return
		case trabalho := <-q.fila:
			q.executar(userID, trabalho)
		}
	}
}

// executar roda UM evento e mede quanto levou.
//
// O recover é POR EVENTO, não por goroutine: um pânico num evento não pode
// matar o worker da sessão, senão a sessão inteira para de processar eventos e
// o sintoma aparece como "parou de receber mensagem", sem relação aparente com
// o evento que estourou. Mesma lição do pool da F86.
func (q *sessionEventQueue) executar(userID string, trabalho func()) {
	defer func() {
		if r := recover(); r != nil {
			log.Error().Str("userid", userID).Interface("panic", r).
				Msg("panic recuperado ao processar evento de sessao; a fila continua")
		}
	}()

	inicio := time.Now()
	trabalho()

	if levou := time.Since(inicio); levou >= sessionEventSlowThreshold {
		// Substitui o `Node handling took` do SDK, que deixa de medir o nosso
		// handler quando ele sai do laço de nós. Sem esta linha, a correção da
		// F87 apagaria a própria evidência de estar funcionando.
		log.Warn().Str("userid", userID).Dur("took", levou).
			Int("queued", len(q.fila)).
			Msg("evento de sessao demorou; a fila desta sessao esta atrasada")
	}
}

// enqueueSessionEvent põe trabalho na fila de userID.
//
// Devolve false quando a sessão já foi desmontada — e nesse caso o evento é
// DESCARTADO de propósito: ele é de uma sessão que não existe mais, e
// processá-lo tocaria em cliente e registries já liberados.
//
// Sem fila registrada, executa em linha. É o caminho de quem nunca passou pelo
// Attach (testes, e qualquer chamador futuro), e manter o comportamento antigo
// ali é mais seguro que criar fila implícita que ninguém desmonta.
func enqueueSessionEvent(userID string, trabalho func()) bool {
	sessionQueuesMu.Lock()
	q, existe := sessionQueues[userID]
	sessionQueuesMu.Unlock()

	if !existe {
		trabalho()
		return true
	}

	select {
	case <-q.parada:
		return false
	default:
	}

	select {
	case q.fila <- trabalho:
		return true
	case <-q.parada:
		return false
	}
}

// stopSessionEventQueue desmonta a fila de userID.
//
// Não drena o que restou: os eventos pendentes são de uma sessão que está sendo
// derrubada, e processá-los depois do teardown tocaria em handles já liberados.
// Quem quiser saber o que se perdeu tem o `queued` do aviso de lentidão.
func stopSessionEventQueue(userID string) {
	sessionQueuesMu.Lock()
	q, existe := sessionQueues[userID]
	delete(sessionQueues, userID)
	sessionQueuesMu.Unlock()

	if !existe {
		return
	}
	q.uma.Do(func() { close(q.parada) })

	if pendentes := len(q.fila); pendentes > 0 {
		log.Warn().Str("userid", userID).Int("descartados", pendentes).
			Msg("sessao desmontada com eventos ainda na fila; eles nao serao processados")
	}
}

// sessionQueueDepth expõe o tamanho da fila de userID, para diagnóstico e
// teste. Devolve -1 quando não há fila, que é distinto de fila vazia.
func sessionQueueDepth(userID string) int {
	sessionQueuesMu.Lock()
	defer sessionQueuesMu.Unlock()

	q, existe := sessionQueues[userID]
	if !existe {
		return -1
	}
	return len(q.fila)
}
