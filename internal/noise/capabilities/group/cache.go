package group

import (
	"slices"
	"sync"

	"wa-api/internal/noise/protocol/types"
)

// Meta e' o metadado de grupo que o caminho de envio consulta. Era o
// groupMetaCache do pacote raiz; la' hoje ha' um apelido de tipo com esse nome,
// porque internals.go (gerado, fora de escopo) cita o nome antigo na assinatura
// de DangerousInternalClient.GetCachedGroupData.
type Meta struct {
	AddressingMode             types.AddressingMode
	CommunityAnnouncementGroup bool
	Members                    []types.JID
}

// Clone devolve uma copia independente, com Members tambem copiado.
//
// Existe para que GetOrFetch nao entregue o *Meta VIVO do mapa. O caminho de
// envio lia Members DEPOIS de GetOrFetch retornar, ou seja, fora do lock,
// enquanto UpdateParticipantCache (handler de w:gp2) mutava a mesma fatia no
// lugar sob o lock — os dois lados nunca seguravam o lock ao mesmo tempo, entao
// o mutex nao protegia esse par. Um envio concorrente com uma entrada/saida de
// participante podia ler um slice header rasgado (F53 em HOUSEKEEP.md).
//
// Custo medido em cache_bench_test.go, no teto de 1024 membros do WhatsApp:
// ~5.5us e UMA alocacao de 41KB por envio de grupo com cache quente. O mesmo
// envio faz criptografia Signal por dispositivo para os mesmos 1024 membros,
// que custa milissegundos — a copia e' ruido perto disso.
func (m *Meta) Clone() *Meta {
	if m == nil {
		return nil
	}
	return &Meta{
		AddressingMode:             m.AddressingMode,
		CommunityAnnouncementGroup: m.CommunityAnnouncementGroup,
		Members:                    slices.Clone(m.Members),
	}
}

// Cache e' o cache de metadados de grupo da sessao. Reune os dois campos que
// viviam soltos em *Client antes desta extracao:
//
//	groupCache     map[types.JID]*groupMetaCache -> entries
//	groupCacheLock sync.Mutex                    -> lock
//
// A estrutura de aquisicao e liberacao do lock NAO mudou. Em particular:
//
//   - GetOrFetch segura o lock durante a consulta ao servidor, exatamente como
//     o getCachedGroupData original fazia. E' deliberado: e' o que impede duas
//     goroutines de dispararem a mesma query de metadados em paralelo. O custo
//     (uma ida a' rede sob mutex) e' o mesmo de antes da extracao.
//   - Por isso CacheInfo mantem o parametro `lock`: quando chamado de dentro
//     de GetOrFetch o lock **ja'** esta' segurado, e um segundo Lock() seria
//     deadlock. Era assim antes; continua sendo.
//
// O zero value e' usavel: o mapa e' criado preguicosamente em SetLocked, sempre
// sob o lock (mesmo racional do lote 3 e do tctoken do lote 4). Uma leitura
// antes de qualquer gravacao enxerga mapa nil, o que em Go devolve o zero —
// mesmo resultado que um mapa vazio.
//
// Por conter um mutex, Cache NUNCA pode ser copiado por valor depois de usado —
// sempre passe *Cache.
type Cache struct {
	lock    sync.Mutex
	entries map[types.JID]*Meta
}

// Lock/Unlock expoem o mutex do cache. Substituem o par
// `cli.groupCacheLock.Lock()` / `cli.groupCacheLock.Unlock()` da raiz.
//
// Sao exportados porque a secao critica original atravessa varias funcoes
// (GetOrFetch -> GetInfo -> CacheInfo), e encapsular cada uma com seu proprio
// lock mudaria o desenho: transformaria uma secao critica em tres, abrindo
// janelas entre elas que nao existiam.
func (c *Cache) Lock() { c.lock.Lock() }

// Unlock libera o lock tomado por Lock.
func (c *Cache) Unlock() { c.lock.Unlock() }

// GetLocked devolve a entrada de jid.
//
// So' pode ser chamado com o lock segurado.
func (c *Cache) GetLocked(jid types.JID) (*Meta, bool) {
	m, ok := c.entries[jid]
	return m, ok
}

// SetLocked grava a entrada de jid, criando o mapa se ainda nao existir.
//
// So' pode ser chamado com o lock segurado.
func (c *Cache) SetLocked(jid types.JID, meta *Meta) {
	if c.entries == nil {
		c.entries = make(map[types.JID]*Meta)
	}
	c.entries[jid] = meta
}

// Delete remove a entrada de jid, tomando o lock por conta propria.
//
// E' o unico metodo que sincroniza sozinho, porque o unico chamador —
// invalidateParticipantCache, em send_ack.go — tomava o lock so' para o
// `delete`, sem nada mais na secao critica.
func (c *Cache) Delete(jid types.JID) {
	c.lock.Lock()
	defer c.lock.Unlock()
	delete(c.entries, jid)
}
