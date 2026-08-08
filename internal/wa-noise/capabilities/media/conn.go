package media

import (
	"context"
	"fmt"
	"sync"
	"time"

	waLog "wa-api/internal/wa-noise/observability/log"
	waBinary "wa-api/internal/wa-noise/protocol/binary"
)

// ConnHost representa um host de onde a midia pode ser baixada.
type ConnHost struct {
	Hostname string
}

// Conn e' a lista de servidores do WhatsApp de onde anexos podem ser baixados,
// junto do token de autenticacao e do TTL devolvidos pelo IQ <media_conn>.
type Conn struct {
	Auth       string
	AuthTTL    int
	TTL        int
	MaxBuckets int
	FetchedAt  time.Time
	Hosts      []ConnHost
}

// Expiry devolve o instante em que a Conn expira.
func (mc *Conn) Expiry() time.Time {
	return mc.FetchedAt.Add(time.Duration(mc.TTL) * time.Second)
}

// ConnCache guarda a media connection corrente e serializa as renovacoes.
// O zero value e' utilizavel; o lock que antes era o campo mediaConnLock do
// *wanoise.Client vive aqui, com os mesmos pontos de aquisicao e liberacao.
type ConnCache struct {
	lock  sync.Mutex
	cache *Conn
}

// Set substitui a Conn em cache sem consultar o servidor. Serve para prepopular
// o cache (testes, ou um cliente que ja' tenha uma mediaConn valida em maos).
//
// ATENCAO — restricao de reentrancia: Set e Get travam o MESMO mutex nao
// reentrante que Refresh segura durante toda a consulta ao servidor. Portanto
// nenhuma implementacao de [Transport] pode chamar Set ou Get de dentro de
// SendMediaConnIQ: seria autodeadlock, e `go test -race` nao acusaria isso
// (deadlock nao e' corrida). Hoje nao ha' nenhum chamador de producao — so'
// testes, sempre fora de Refresh.
func (cc *ConnCache) Set(mc *Conn) {
	cc.lock.Lock()
	defer cc.lock.Unlock()
	cc.cache = mc
}

// Get devolve a Conn em cache sem renovar nem validar a expiracao.
//
// Vale a mesma restricao de reentrancia documentada em [ConnCache.Set].
func (cc *ConnCache) Get() *Conn {
	cc.lock.Lock()
	defer cc.lock.Unlock()
	return cc.cache
}

// Refresh devolve a media connection em cache, renovando-a se ela nao existir,
// se force for verdadeiro ou se ja' tiver expirado.
//
// Fidelidade ao codigo de origem: em caso de erro na consulta o cache e'
// zerado (o codigo original fazia `cli.mediaConnCache, err = ...`, atribuindo
// nil antes de checar o erro). Esse comportamento foi preservado de proposito;
// ver PATCHES.md, Fase F/G lote 1.
func (cc *ConnCache) Refresh(ctx context.Context, t Transport, force bool) (*Conn, error) {
	cc.lock.Lock()
	defer cc.lock.Unlock()
	if cc.cache == nil || force || time.Now().After(cc.cache.Expiry()) {
		var err error
		cc.cache, err = QueryConn(ctx, t)
		if err != nil {
			return nil, err
		}
	}
	return cc.cache, nil
}

// RefreshConn e' o atalho para renovar a media connection do Transport dado.
func RefreshConn(ctx context.Context, t Transport, force bool) (*Conn, error) {
	return t.MediaConnCache().Refresh(ctx, t, force)
}

// QueryConn pergunta ao servidor a lista de hosts de midia, sem passar pelo
// cache.
func QueryConn(ctx context.Context, t Transport) (*Conn, error) {
	resp, err := t.SendMediaConnIQ(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query media connections: %w", err)
	}
	return ParseConnNode(t.Log(), resp)
}

// ParseConnNode interpreta a resposta do IQ <media_conn>. E' puro: nao toca em
// rede nem em estado compartilhado, so' no logger (para hosts inesperados).
func ParseConnNode(log waLog.Logger, resp *waBinary.Node) (*Conn, error) {
	if len(resp.GetChildren()) == 0 || resp.GetChildren()[0].Tag != "media_conn" {
		return nil, fmt.Errorf("failed to query media connections: unexpected child tag")
	}
	respMC := resp.GetChildren()[0]
	var mc Conn
	ag := respMC.AttrGetter()
	mc.FetchedAt = time.Now()
	mc.Auth = ag.String("auth")
	mc.TTL = ag.Int("ttl")
	mc.AuthTTL = ag.Int("auth_ttl")
	mc.MaxBuckets = ag.Int("max_buckets")
	if !ag.OK() {
		return nil, fmt.Errorf("failed to parse media connections: %+v", ag.Errors)
	}
	for _, child := range respMC.GetChildren() {
		if child.Tag != "host" {
			log.Warnf("Unexpected child in media_conn element: %s", child.XMLString())
			continue
		}
		cag := child.AttrGetter()
		mc.Hosts = append(mc.Hosts, ConnHost{
			Hostname: cag.String("hostname"),
		})
		if !cag.OK() {
			return nil, fmt.Errorf("failed to parse media connection host: %+v", ag.Errors)
		}
	}
	return &mc, nil
}
