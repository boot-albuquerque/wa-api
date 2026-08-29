package user

import (
	"sync"
	"time"

	"wa-api/internal/noise/protocol/types"
)

// DeviceEntry e' a lista de dispositivos cacheada de um usuario, com o dhash
// que o servidor usa para anunciar mudancas. Era o `deviceCache` do pacote
// raiz, onde hoje ha' um apelido de tipo com esse nome.
//
// Os campos eram nao exportados (`devices`, `dhash`); ficaram exportados porque
// notification_device.go, na raiz, monta e le entradas diretamente — e' o
// dominio de NOTIFICACAO de dispositivo, nao deste, e continua na raiz.
type DeviceEntry struct {
	Devices []types.JID
	DHash   string
}

// DeviceCache e' o cache de listas de dispositivo da sessao. Reune os dois
// campos que viviam soltos em *Client antes desta extracao:
//
//	userDevicesCache     map[types.JID]deviceCache -> entries
//	userDevicesCacheLock sync.Mutex                -> lock
//
// A estrutura de aquisicao e liberacao do lock NAO mudou. Em particular:
//
//   - GetDevices segura o lock atraves da consulta usync ao servidor,
//     exatamente como o GetUserDevices original fazia. E' deliberado: e' o que
//     impede duas goroutines de dispararem a mesma query de dispositivos em
//     paralelo. O custo (uma ida a' rede sob mutex) e' o mesmo de antes.
//   - Por isso GetFBIDDevices e' *Locked por contrato: e' chamado de dentro de
//     GetDevices, com o lock **ja'** segurado, e um segundo Lock() num
//     sync.Mutex nao reentrante seria deadlock imediato. Era assim antes;
//     continua sendo.
//
// O zero value e' usavel: o mapa e' criado preguicosamente em SetLocked, sempre
// sob o lock (mesmo racional do lote 3, do tctoken do lote 4, do retry do lote
// 5 e do group do lote 6). Uma leitura antes de qualquer gravacao enxerga mapa
// nil, o que em Go devolve o zero — mesmo resultado que um mapa vazio; e
// `delete` em mapa nil e' no-op.
//
// Por conter um mutex, DeviceCache NUNCA pode ser copiado por valor depois de
// usado — sempre passe *DeviceCache.
type DeviceCache struct {
	lock      sync.Mutex
	entries   map[types.JID]cachedDevices
	lastSweep time.Time
}

// cachedDevices e' a entrada com o carimbo de quando foi gravada.
type cachedDevices struct {
	entry DeviceEntry
	seen  time.Time
}

// Politica de expiracao do cache de dispositivos.
//
// A invalidacao PRIMARIA e' push: o servidor manda notificacao de dispositivo
// (`handleDeviceNotification`) quando o usuario adiciona ou remove um aparelho
// vinculado, e o cache e' apagado ali. O TTL abaixo NAO substitui isso — ele
// existe como rede para dois casos que o push nao cobre:
//
//  1. notificacao perdida (desconexao no momento errado), que deixaria a lista
//     de dispositivos errada ate' o processo reiniciar;
//  2. chave orfa. GetUserDevices grava o resultado sob o JID que o SERVIDOR
//     devolveu, que pode nao ser o consultado — se isso acontecer, a entrada
//     nunca mais e' lida nem apagada e o mapa so' cresce (F37 em HOUSEKEEP.md).
//
// 24h e' longo de proposito. GetDevices segura o lock do cache ATRAVESSANDO a
// consulta usync ao servidor, entao expirar agressivamente troca memoria por
// idas a' rede com mutex segurado — que e' o recurso mais caro aqui. Para
// referencia, o evolution-api usa NodeCache com stdTTL de 300000s (~3,5 dias)
// no mesmo cache.
const (
	deviceCacheTTL           = 24 * time.Hour
	deviceCacheSweepInterval = time.Hour
)

// Lock/Unlock expoem o mutex do cache. Substituem o par
// `cli.userDevicesCacheLock.Lock()` / `.Unlock()` da raiz.
//
// Sao exportados porque as secoes criticas originais atravessam varias funcoes
// (GetDevices -> GetFBIDDevices) e varios acessos ao mapa numa unica secao
// (handleDeviceNotification, na raiz, le e escreve dezenas de vezes sob um so'
// lock). Encapsular cada acesso com o seu proprio lock mudaria o desenho:
// transformaria uma secao critica em muitas, abrindo janelas que nao existiam.
func (c *DeviceCache) Lock() { c.lock.Lock() }

// Unlock libera o lock tomado por Lock.
func (c *DeviceCache) Unlock() { c.lock.Unlock() }

// GetLocked devolve a entrada de jid, tratando entrada expirada como ausente.
//
// So' pode ser chamado com o lock segurado.
func (c *DeviceCache) GetLocked(jid types.JID) (DeviceEntry, bool) {
	e, ok := c.entries[jid]
	if !ok || time.Since(e.seen) > deviceCacheTTL {
		return DeviceEntry{}, false
	}
	return e.entry, true
}

// SetLocked grava a entrada de jid, criando o mapa se ainda nao existir.
//
// So' pode ser chamado com o lock segurado.
func (c *DeviceCache) SetLocked(jid types.JID, entry DeviceEntry) {
	now := time.Now()
	c.sweepLocked(now)
	if c.entries == nil {
		c.entries = make(map[types.JID]cachedDevices)
	}
	c.entries[jid] = cachedDevices{entry: entry, seen: now}
}

// sweepLocked remove entradas expiradas, no maximo uma vez por
// deviceCacheSweepInterval.
//
// GetLocked ja' trata expirada como ausente, entao a varredura nao muda
// resultado nenhum — ela existe para o caso 2 do comentario da politica: chave
// orfa, que ninguem consulta e portanto nunca seria expirada pela leitura.
func (c *DeviceCache) sweepLocked(now time.Time) {
	if now.Sub(c.lastSweep) < deviceCacheSweepInterval {
		return
	}
	c.lastSweep = now
	for jid, e := range c.entries {
		if now.Sub(e.seen) > deviceCacheTTL {
			delete(c.entries, jid)
		}
	}
}

// DeleteLocked remove a entrada de jid.
//
// So' pode ser chamado com o lock segurado. E' o que
// handleDeviceNotification/handleOwnDevicesNotification usam, porque o delete
// deles esta' no meio de uma secao critica maior.
func (c *DeviceCache) DeleteLocked(jid types.JID) {
	delete(c.entries, jid)
}

// Delete remove a entrada de jid, tomando o lock por conta propria.
//
// E' o unico metodo que sincroniza sozinho, porque o unico chamador —
// invalidateParticipantCache, em send_ack.go — tomava o lock so' para o
// `delete`, sem nada mais na secao critica.
func (c *DeviceCache) Delete(jid types.JID) {
	c.lock.Lock()
	defer c.lock.Unlock()
	delete(c.entries, jid)
}

// Len devolve o numero de entradas, tomando o lock por conta propria. Existe
// para os testes da raiz, que antes mediam `len(cli.userDevicesCache)` no mapa
// nu.
func (c *DeviceCache) Len() int {
	c.lock.Lock()
	defer c.lock.Unlock()
	return len(c.entries)
}
