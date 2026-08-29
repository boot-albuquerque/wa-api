package retry

import (
	"slices"
	"time"
)

// Politica de despejo dos dois contadores de retry (F36 em HOUSEKEEP.md).
//
// Os dois mapas sao chaveados por dado que vem do SERVIDOR — o remetente e o ID
// da mensagem. Sem despejo, uma sessao longa acumula uma entrada por mensagem
// que precisou de retry, para sempre, e um par malicioso acumula uma entrada
// por ID inventado. Nao e' panic: e' crescimento monotonico proporcional ao que
// o outro lado mandar.
const (
	// counterTTL e' quanto tempo uma entrada sobrevive sem ser tocada.
	//
	// Uma hora, o mesmo teto de recreateSessionTimeout, pelo mesmo motivo: e' a
	// janela em que o protocolo ainda considera um retry relacionado ao envio
	// original. Recibos de retry legitimos chegam em segundos ou minutos, entao
	// a janela nunca e' alcancada por trafego normal — o que ela remove sao
	// entradas que o par abandonou.
	//
	// Consequencia aceita: um retry que chegue depois de uma hora volta a ser
	// contado como primeiro. E' o comportamento que ja' se tem hoje ao
	// reiniciar o processo, e melhor do que vazar memoria indefinidamente.
	counterTTL = time.Hour

	// counterSweepInterval limita o custo da varredura. A varredura e' O(n) e
	// roda dentro do lock do contador; sem este intervalo ela rodaria a cada
	// incremento.
	counterSweepInterval = time.Minute

	// counterMaxEntries e' o teto rigido, para o caso que o TTL sozinho nao
	// cobre: um par que inunde com IDs distintos DENTRO da janela de uma hora.
	// Ao ultrapassar, a varredura roda fora do intervalo e, se ainda assim
	// sobrar gente, as entradas mais antigas saem ate' caber.
	//
	// 8192 e' folgado o suficiente para nunca ser alcancado por uma sessao
	// real (o buffer de mensagens recentes, no mesmo dominio, tem 256) e baixo
	// o suficiente para o mapa nao passar de alguns megabytes.
	counterMaxEntries = 8192

	// counterEvictTarget e' ate' onde o despejo por teto desce: 75% do teto,
	// nao o teto exato.
	//
	// Isto NAO e' detalhe de ajuste fino, e' o que evita transformar a defesa
	// em amplificacao. Descendo so' ate' o teto, a proxima insercao estoura de
	// novo e dispara outra ordenacao — ou seja, um par inundando pagaria
	// O(n log n) POR MENSAGEM, e o teto que existe para conter abuso viraria o
	// vetor. Com a marca d'agua, a ordenacao amortiza em uma a cada ~2048
	// insercoes.
	counterEvictTarget = counterMaxEntries * 3 / 4
)

type counterEntry struct {
	count int
	seen  time.Time
}

// counterMap e' um mapa de contadores com despejo por idade e teto de tamanho.
//
// NAO tem lock proprio: os dois usos vivem dentro de State, cada um sob o mutex
// que ja' protegia o mapa cru que este tipo substituiu. Manter o lock fora e' o
// que preserva as secoes criticas exatamente como estavam.
type counterMap[K comparable] struct {
	entries   map[K]counterEntry
	lastSweep time.Time
}

// increment soma um na chave e devolve o valor resultante, varrendo quando for
// a hora.
func (m *counterMap[K]) increment(key K, now time.Time) int {
	m.maybeSweep(now)
	if m.entries == nil {
		m.entries = make(map[K]counterEntry)
	}
	e := m.entries[key]
	e.count++
	e.seen = now
	m.entries[key] = e
	return e.count
}

// set sobrescreve o contador da chave. Usado por BumpMessageRetries, que
// reinicia a contagem a partir do valor que o servidor mandou.
func (m *counterMap[K]) set(key K, count int, now time.Time) {
	if m.entries == nil {
		m.entries = make(map[K]counterEntry)
	}
	m.entries[key] = counterEntry{count: count, seen: now}
}

func (m *counterMap[K]) len() int { return len(m.entries) }

// maybeSweep roda a varredura quando o intervalo passou OU quando o mapa
// ultrapassou o teto — o teto tem prioridade justamente porque o caso que ele
// cobre e' uma inundacao, que acontece rapido demais para esperar o intervalo.
func (m *counterMap[K]) maybeSweep(now time.Time) {
	// >= e nao >: a varredura roda ANTES da insercao, entao parar em "maior
	// que" deixaria o mapa estacionar em teto+1.
	over := len(m.entries) >= counterMaxEntries
	if !over && now.Sub(m.lastSweep) < counterSweepInterval {
		return
	}
	m.lastSweep = now

	for k, e := range m.entries {
		if now.Sub(e.seen) > counterTTL {
			delete(m.entries, k)
		}
	}
	if len(m.entries) < counterMaxEntries {
		return
	}

	// Sobrou gente depois do TTL: alguem esta' inundando dentro da janela. Sai
	// o mais antigo primeiro, que e' o menos provavel de ainda estar em uso.
	type aged struct {
		key  K
		seen time.Time
	}
	all := make([]aged, 0, len(m.entries))
	for k, e := range m.entries {
		all = append(all, aged{k, e.seen})
	}
	slices.SortFunc(all, func(a, b aged) int { return a.seen.Compare(b.seen) })
	for _, a := range all[:len(all)-counterEvictTarget] {
		delete(m.entries, a.key)
	}
}
