// session_config_test.go — os dois use cases de ESCRITA de configuração de
// sessão: SetHistory (POST /session/history) e SetProxy (POST /session/proxy).
//
// O que estes testes existem para travar, e que um teste de status code não
// pega:
//
//   - a ORDEM. Cache só DEPOIS do banco confirmar em SetHistory; guarda de
//     sessão conectada ANTES de qualquer escrita em SetProxy. Inverter
//     qualquer uma das duas continua respondendo o mesmo código HTTP.
//   - a PUBLICAÇÃO no cache do SetHistory, que é o que fecha a F128: sem ela o
//     gate de leitura de histórico continua revalidando no banco a cada
//     requisição.
package storage_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/storage"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/egress"
)

// errStoreDown é a falha de gravação injetada nos testes de ORDEM. Vem do
// dublê do store, e não de um erro fabricado no meio do use case, para que o
// caminho testado seja o mesmo que o driver real percorre.
var errStoreDown = errors.New("store: gravacao recusada")

// defaultWebhookUseProxyForTest é o valor global que o use case aplica quando
// o pedido omite `webhook_use_proxy` E a leitura do gravado falha. `true` é o
// default do processo (flag `-webhookuseproxy`, config.go:55).
const defaultWebhookUseProxyForTest = true

func newSetHistory(
	sg port.SessionGuard,
	log port.Logger,
	store *contractsfake.HistoryConfigStore,
	cache *contractsfake.UserInfoSessionCache,
) *storage.SetHistoryUseCase {
	return storage.NewSetHistoryUseCase(sg, store, cache, log)
}

func newSetProxy(
	status port.SessionStatusReader,
	log port.Logger,
	store *contractsfake.ProxyConfigStore,
	cache *contractsfake.UserInfoSessionCache,
) *storage.SetProxyUseCase {
	return storage.NewSetProxyUseCase(status, store, cache, defaultWebhookUseProxyForTest, rfc6761Resolver{}, log)
}

// newSetProxyComDefault é newSetProxy com o default de processo escolhido pelo
// teste. Existe por causa do TESTE 9: o que ele prova só é observável quando o
// default é FALSE, porque com `true` a recusa aconteceria de qualquer jeito e o
// teste passaria com a guarda lendo o default em vez do modo estrito.
func newSetProxyComDefault(
	status port.SessionStatusReader,
	log port.Logger,
	store *contractsfake.ProxyConfigStore,
	cache *contractsfake.UserInfoSessionCache,
	padrao bool,
) *storage.SetProxyUseCase {
	return storage.NewSetProxyUseCase(status, store, cache, padrao, rfc6761Resolver{}, log)
}

// --- endereços usados nos testes de proxy ---------------------------------
//
// Os hosts são IP LITERAL sempre que o teste não estiver medindo o ramo de
// NOME, e por dois motivos, os mesmos que fixaram s3TestEndpoint
// (pkg/bootstrap/s3_config_route_test.go:66):
//
//   - literal curto-circuita a resolução em egress.go:197, então o teste não
//     depende de DNS nem de rede;
//   - 203.0.113.0/24 é TEST-NET-3 (RFC 5737) e NÃO está em reservedCIDRs
//     (egress.go:50), então passa a guarda de endereço reservado — que é o que
//     o caminho de sucesso precisa.
//
// Antes do CAP-31 estes testes usavam `proxy.invalid`, que era inofensivo
// porque nada resolvia nada; com a guarda no lugar, um nome `.invalid` é
// NXDOMAIN por norma (RFC 6761 §6.4) e viraria recusa em todo caminho de
// sucesso. A troca é o que mantém os testes medindo o que mediam.
const (
	proxyHostPublico = "203.0.113.10"

	proxyHTTPPublico   = "http://" + proxyHostPublico + ":3128"
	proxySOCKS5Publico = "socks5://user:pass@" + proxyHostPublico + ":1080"

	// Loopback e RFC1918: os dois endereços que a guarda do CAP-31 recusa
	// quando a entrega de webhook sai pelo proxy.
	proxyHTTPLoopback    = "http://127.0.0.1:3128"
	proxySOCKS5Reservado = "socks5://10.0.0.1:1080"

	// Por NOME, para exercitar o ramo que resolve. `localhost` é o único nome
	// cuja resposta é fixada por norma (RFC 6761 §6.3: todo resolvedor o mapeia
	// para o endereço de loopback), então é o único que um dublê pode responder
	// sem ficar mais permissivo que a produção.
	proxyHTTPLocalhost = "http://localhost:3128"
)

// rfc6761Resolver é o dublê de DNS da guarda de endereço reservado. Ele
// responde APENAS nomes cuja resposta é fixada por norma, e é isso que o
// impede de ser mais permissivo que a produção (ARMADILHA 1 deste repo):
//
//   - "localhost" → 127.0.0.1. RFC 6761 §6.3 obriga todo resolvedor a mapear
//     `localhost` para o loopback; é o que o *net.Resolver de produção
//     (egress.go:176, usado em egress.go:213) devolve em qualquer máquina.
//   - "*.invalid" → NXDOMAIN. RFC 6761 §6.4 reserva `.invalid` exatamente para
//     que nunca resolva; a produção também erra aqui.
//
// Qualquer outro nome faz o dublê ERRAR de propósito, com a mensagem dizendo
// por quê: inventar uma resposta é como um dublê passa a medir outra coisa.
type rfc6761Resolver struct{}

var _ egress.HostResolver = rfc6761Resolver{}

func (rfc6761Resolver) LookupIP(_ context.Context, _, host string) ([]net.IP, error) {
	switch {
	case host == "localhost":
		return []net.IP{net.IPv4(127, 0, 0, 1)}, nil
	case strings.HasSuffix(host, ".invalid"):
		return nil, fmt.Errorf("no such host %q (RFC 6761 §6.4)", host)
	default:
		return nil, fmt.Errorf("rfc6761Resolver: nome %q nao tem resposta fixada por norma; "+
			"use IP literal ou acrescente o nome com a citacao da regra real", host)
	}
}

// disconnectedSession é o estado em que o proxy PODE ser configurado: sem
// sessão conectada. É o zero-value do dublê, nomeado aqui para que o caso de
// sucesso diga qual estado está exercitando.
func disconnectedSession() *contractsfake.SessionStatusReader {
	return &contractsfake.SessionStatusReader{}
}

// connectedSession imita a regra REAL do adapter de produção
// (pkg/infra/noise/runtime/session/guard.go:72): SessionStatus devolve
// `client.IsConnected()`, e é esse primeiro valor — e não o segundo, que é
// IsLoggedIn — que a guarda histórica consultava
// (`41bc8e2^:handlers.go:6099`). Um dublê que devolvesse "conectado" no
// segundo campo não mediria a guarda.
func connectedSession() *contractsfake.SessionStatusReader {
	return &contractsfake.SessionStatusReader{
		SessionStatusFunc: func(context.Context, string) (bool, bool) { return true, true },
	}
}

// --- SetHistory ---------------------------------------------------------

// TESTE 1 — grava no banco E publica no cache, nesta ordem, com o valor do
// pedido nos dois lugares. É a prova de que a F128 fecha pelo lado da escrita:
// o gate de leitura semeia-se do valor cacheado, e ele passa a ser o novo.
func TestSetHistory_GravaNoBancoEPublicaNoCache(t *testing.T) {
	store := &contractsfake.HistoryConfigStore{}
	cache := &contractsfake.UserInfoSessionCache{}
	log := &contractsfake.Logger{}

	r, err := newSetHistory(&contractsfake.SessionGuard{}, log, store, cache).
		Execute(context.Background(), txtID, domain.WebhookHistoryRequest{History: 50})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if r.History != 50 {
		t.Errorf("History = %d, quero 50", r.History)
	}

	if len(store.SaveHistoryLimitCalls) != 1 {
		t.Fatalf("SaveHistoryLimit chamado %d vez(es), quero 1 — o stub da F151 voltou?", len(store.SaveHistoryLimitCalls))
	}
	if got := store.SaveHistoryLimitCalls[0]; got.UserID != txtID || got.History != 50 {
		t.Errorf("SaveHistoryLimit(%q, %d), quero (%q, 50)", got.UserID, got.History, txtID)
	}
	if got := store.Stored[txtID]; got != 50 {
		t.Errorf("banco guardou %d, quero 50", got)
	}

	if len(cache.SetHistoryCalls) != 1 {
		t.Fatalf("o cache NAO foi atualizado (%d chamadas): o gate de leitura da F128 vai continuar "+
			"revalidando no banco uma vez por requisicao ate' o TTL expirar", len(cache.SetHistoryCalls))
	}
	if got := cache.SetHistoryCalls[0]; got.UserID != txtID || got.History != 50 {
		t.Errorf("SetHistory(%q, %d), quero (%q, 50)", got.UserID, got.History, txtID)
	}
}

// TESTE 2 — history < 0: recusa, NADA gravado e cache intocado.
func TestSetHistory_Negativo_NaoGravaNemPublica(t *testing.T) {
	store := &contractsfake.HistoryConfigStore{}
	cache := &contractsfake.UserInfoSessionCache{}
	log := &contractsfake.Logger{}

	r, err := newSetHistory(&contractsfake.SessionGuard{}, log, store, cache).
		Execute(context.Background(), txtID, domain.WebhookHistoryRequest{History: -1})
	if err == nil {
		t.Fatal("history negativo devia ser recusado")
	}
	if r != nil {
		t.Error("resultado devia ser nil na recusa")
	}
	if len(store.SaveHistoryLimitCalls) != 0 {
		t.Errorf("a recusa gravou no banco: %+v", store.SaveHistoryLimitCalls)
	}
	if len(cache.SetHistoryCalls) != 0 {
		t.Errorf("a recusa publicou no cache: %+v", cache.SetHistoryCalls)
	}
	if len(log.ByLevel(contractsfake.LevelInfo)) != 0 {
		t.Errorf("a recusa logou sucesso: %v", log.Messages())
	}
}

// TESTE 3 — teste de ORDEM. A gravação falha, então o cache NÃO pode ser
// tocado: publicar assim mesmo mostraria ao usuário um limite que o banco não
// tem, e o gate de leitura da F128 confiaria nele.
func TestSetHistory_FalhaDeGravacao_NaoTocaOCache(t *testing.T) {
	store := &contractsfake.HistoryConfigStore{
		SaveHistoryLimitFunc: func(context.Context, string, int) error { return errStoreDown },
	}
	cache := &contractsfake.UserInfoSessionCache{}
	log := &contractsfake.Logger{}

	r, err := newSetHistory(&contractsfake.SessionGuard{}, log, store, cache).
		Execute(context.Background(), txtID, domain.WebhookHistoryRequest{History: 50})
	if err == nil {
		t.Fatal("falha de gravacao devia produzir erro")
	}
	if !errors.Is(err, errStoreDown) {
		t.Errorf("a causa do store se perdeu: %v", err)
	}
	if r != nil {
		t.Error("resultado devia ser nil na falha")
	}
	if len(cache.SetHistoryCalls) != 0 {
		t.Fatalf("o cache foi publicado DEPOIS de o banco falhar (%d chamadas): o usuario passa a ver "+
			"um limite que o banco nao tem", len(cache.SetHistoryCalls))
	}
	if len(log.ByLevel(contractsfake.LevelError)) == 0 {
		t.Errorf("falha de dependencia nao foi logada em error: %v", log.Messages())
	}
}

// --- SetProxy -----------------------------------------------------------

// TESTE 4 — teste de ORDEM. Com a sessão CONECTADA a recusa é 400, e o
// repositório NÃO é alcançado. Mover a guarda para depois do UPDATE devolveria
// 400 do mesmo jeito, com o proxy já gravado — nenhum teste de status code
// veria a diferença.
func TestSetProxy_ClienteConectado_NaoChamaORepositorio(t *testing.T) {
	// Os dois corpos que importam: um válido e um que o ramo de habilitação
	// recusaria por conta própria. Nos DOIS a guarda é quem responde, e nos
	// dois o repositório fica intocado.
	corpos := []struct {
		name string
		req  domain.ProxyConfigRequest
	}{
		{"corpo valido", domain.ProxyConfigRequest{Enable: true, ProxyURL: "http://203.0.113.10:3128"}},
		{"corpo que seria recusado adiante", domain.ProxyConfigRequest{Enable: true}},
		{"pedido de desabilitar", domain.ProxyConfigRequest{Enable: false}},
	}
	for _, c := range corpos {
		t.Run(c.name, func(t *testing.T) {
			store := &contractsfake.ProxyConfigStore{}
			cache := &contractsfake.UserInfoSessionCache{}
			log := &contractsfake.Logger{}

			r, err := newSetProxy(connectedSession(), log, store, cache).
				Execute(context.Background(), txtID, c.req)

			if err == nil {
				t.Fatal("proxy com sessao conectada devia ser recusado")
			}
			if r != nil {
				t.Error("resultado devia ser nil na recusa")
			}
			if len(store.SaveProxyConfigCalls) != 0 {
				t.Fatalf("a guarda de sessao conectada roda DEPOIS da escrita: o repositorio foi chamado "+
					"%d vez(es) e o proxy ja' esta' gravado", len(store.SaveProxyConfigCalls))
			}
			if len(store.LoadWebhookUseProxyCalls) != 0 {
				t.Errorf("a recusa leu o banco %d vez(es)", len(store.LoadWebhookUseProxyCalls))
			}
			if len(cache.SetProxyCalls) != 0 {
				t.Errorf("a recusa publicou no cache: %+v", cache.SetProxyCalls)
			}
			if len(log.ByLevel(contractsfake.LevelInfo)) != 0 {
				t.Errorf("a recusa logou sucesso: %v", log.Messages())
			}
		})
	}
}

// TESTES 5, 6, 7 e 9 — os esquemas. `http` e `socks5` passam; qualquer outro
// é recusado, `https` inclusive — que é o que parece obviamente aceitável e
// não é.
//
// # O subteste do loopback MUDOU DE SENTIDO no CAP-31, e a mudança é a decisão
//
// Até o CAP-30 havia aqui um caso "http em loopback passa — proxy interno e' o
// caso normal", com `http://127.0.0.1:3128` e SEM dizer em que modo. Ele
// documentava a ausência da guarda de endereço reservado, que saiu junto com
// `egress.ValidateOutboundURL` quando a rota passou a funcionar de verdade.
//
// Ele não passou a estar errado: o que ele afirmava continua verdadeiro no modo
// em que foi escrito — com `webhook_use_proxy` DESLIGADO, loopback passa, e é o
// TestSetProxy_ReservadoDependeDoModo/"desligado ..." que agora o trava. O que
// ele não dizia é que o modo importa, e omitir isso é o que o tornava uma
// afirmação forte demais depois de a rota ficar viva: com o modo LIGADO a
// entrega de webhook sai por esse proxy, e loopback vira contorno do validador
// de saída. Por isso o caso saiu daqui, onde o eixo é o ESQUEMA e o modo não é
// escolhido, e virou uma tabela própria com o modo explícito nos dois valores.
//
// Todas as URLs aqui são de host público (proxyHostPublico) para que este teste
// meça só o esquema: um host reservado faria a recusa vir da outra guarda.
func TestSetProxy_Esquemas(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "http passa", url: proxyHTTPPublico},
		{name: "socks5 passa", url: proxySOCKS5Publico},
		{name: "https e' recusado", url: "https://" + proxyHostPublico + ":3128", wantErr: true},
		{name: "ftp e' recusado", url: "ftp://" + proxyHostPublico + ":21", wantErr: true},
		{name: "sem esquema e' recusado", url: proxyHostPublico + ":3128", wantErr: true},
		{name: "malformada e' recusada", url: "://x", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &contractsfake.ProxyConfigStore{}
			cache := &contractsfake.UserInfoSessionCache{}

			r, err := newSetProxy(disconnectedSession(), &contractsfake.Logger{}, store, cache).
				Execute(context.Background(), txtID, domain.ProxyConfigRequest{Enable: true, ProxyURL: tc.url})

			if tc.wantErr {
				if err == nil {
					t.Fatalf("URL %q devia ser recusada", tc.url)
				}
				if r != nil {
					t.Error("resultado devia ser nil na recusa")
				}
				if len(store.SaveProxyConfigCalls) != 0 {
					t.Errorf("a recusa gravou %+v", store.SaveProxyConfigCalls)
				}
				return
			}
			if err != nil {
				t.Fatalf("URL %q devia ser aceita: %v", tc.url, err)
			}
			if !r.Set {
				t.Error("Set = false no caminho de sucesso")
			}
			if r.ProxyURL != tc.url {
				t.Errorf("ProxyURL = %q, quero %q", r.ProxyURL, tc.url)
			}
			if got := store.Stored[txtID].ProxyURL; got != tc.url {
				t.Errorf("banco guardou %q, quero %q", got, tc.url)
			}
			if len(cache.SetProxyCalls) != 1 || cache.SetProxyCalls[0].ProxyURL != tc.url {
				t.Errorf("cache = %+v, quero uma publicacao de %q", cache.SetProxyCalls, tc.url)
			}
		})
	}
}

// TESTE 8 — habilitar sem `proxy_url`: recusa, nada gravado.
func TestSetProxy_HabilitarSemURL_NaoGrava(t *testing.T) {
	store := &contractsfake.ProxyConfigStore{}
	cache := &contractsfake.UserInfoSessionCache{}

	r, err := newSetProxy(disconnectedSession(), &contractsfake.Logger{}, store, cache).
		Execute(context.Background(), txtID, domain.ProxyConfigRequest{Enable: true})
	if err == nil {
		t.Fatal("habilitar sem proxy_url devia ser recusado")
	}
	if r != nil {
		t.Error("resultado devia ser nil na recusa")
	}
	if len(store.SaveProxyConfigCalls) != 0 {
		t.Errorf("a recusa gravou %+v", store.SaveProxyConfigCalls)
	}
	if len(cache.SetProxyCalls) != 0 {
		t.Errorf("a recusa publicou no cache: %+v", cache.SetProxyCalls)
	}
}

// TESTE 10 — desabilitar zera proxy_url no banco e publica "" no cache. A URL
// do pedido é ignorada de propósito: o ramo histórico de desabilitação nunca
// a olhou, e recusar uma remoção porque a URL que vai ser jogada fora está
// quebrada tornaria uma configuração ruim impossível de tirar.
func TestSetProxy_Desabilitar_ZeraBancoECache(t *testing.T) {
	store := &contractsfake.ProxyConfigStore{Stored: map[string]contractsfake.ProxyConfigRecord{
		txtID: {ProxyURL: "socks5://203.0.113.10:1080", WebhookUseProxy: true},
	}}
	cache := &contractsfake.UserInfoSessionCache{}

	r, err := newSetProxy(disconnectedSession(), &contractsfake.Logger{}, store, cache).
		Execute(context.Background(), txtID, domain.ProxyConfigRequest{Enable: false, ProxyURL: "nao-e-uma-url"})
	if err != nil {
		t.Fatalf("desabilitar devia passar: %v", err)
	}
	if r.Set {
		t.Error("Set = true no ramo de desabilitacao")
	}
	if got := store.Stored[txtID].ProxyURL; got != "" {
		t.Fatalf("proxy_url = %q depois de desabilitar, quero vazio", got)
	}
	if len(cache.SetProxyCalls) != 1 || cache.SetProxyCalls[0].ProxyURL != "" {
		t.Fatalf("cache = %+v, quero uma publicacao de \"\"", cache.SetProxyCalls)
	}
}

// TESTE 11 — teste de ORDEM. A gravação falha e o cache NÃO é tocado, nos dois
// ramos: publicar um proxy que o banco não tem faria a entrega de webhook
// apontar para um destino que some no próximo restart.
func TestSetProxy_FalhaDeGravacao_NaoTocaOCache(t *testing.T) {
	ramos := []struct {
		name string
		req  domain.ProxyConfigRequest
	}{
		{"habilitar", domain.ProxyConfigRequest{Enable: true, ProxyURL: "http://203.0.113.10:3128"}},
		{"desabilitar", domain.ProxyConfigRequest{Enable: false}},
	}
	for _, tc := range ramos {
		t.Run(tc.name, func(t *testing.T) {
			store := &contractsfake.ProxyConfigStore{
				SaveProxyConfigFunc: func(context.Context, string, string, bool) error { return errStoreDown },
			}
			cache := &contractsfake.UserInfoSessionCache{}
			log := &contractsfake.Logger{}

			r, err := newSetProxy(disconnectedSession(), log, store, cache).
				Execute(context.Background(), txtID, tc.req)
			if err == nil {
				t.Fatal("falha de gravacao devia produzir erro")
			}
			if !errors.Is(err, errStoreDown) {
				t.Errorf("a causa do store se perdeu: %v", err)
			}
			if r != nil {
				t.Error("resultado devia ser nil na falha")
			}
			if len(cache.SetProxyCalls) != 0 {
				t.Fatalf("o cache foi publicado DEPOIS de o banco falhar: %+v", cache.SetProxyCalls)
			}
			if len(log.ByLevel(contractsfake.LevelError)) == 0 {
				t.Errorf("falha de dependencia nao foi logada em error: %v", log.Messages())
			}
		})
	}
}

// TestSetProxy_WebhookUseProxy trava a resolução de três passos do
// `webhook_use_proxy` (`41bc8e2^:handlers.go:6118`). Sem ela, toda escrita de
// proxy zeraria a preferência de quem não a mandou no corpo.
func TestSetProxy_WebhookUseProxy(t *testing.T) {
	verdadeiro, falso := true, false

	cases := []struct {
		name      string
		requested *bool
		store     *contractsfake.ProxyConfigStore
		want      bool
	}{
		{
			name:      "o pedido manda e vence o gravado",
			requested: &falso,
			store: &contractsfake.ProxyConfigStore{Stored: map[string]contractsfake.ProxyConfigRecord{
				txtID: {WebhookUseProxy: true},
			}},
			want: false,
		},
		{
			name:      "o pedido omite e o gravado e' PRESERVADO",
			requested: nil,
			store: &contractsfake.ProxyConfigStore{Stored: map[string]contractsfake.ProxyConfigRecord{
				txtID: {WebhookUseProxy: false},
			}},
			want: false,
		},
		{
			name:      "o pedido omite e a leitura falha: cai no default do processo",
			requested: nil,
			store: &contractsfake.ProxyConfigStore{
				LoadWebhookUseProxyFunc: func(context.Context, string) (bool, error) { return false, errStoreDown },
			},
			want: defaultWebhookUseProxyForTest,
		},
		{
			name:      "o pedido manda true e nem le' o banco",
			requested: &verdadeiro,
			store:     &contractsfake.ProxyConfigStore{},
			want:      true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := newSetProxy(disconnectedSession(), &contractsfake.Logger{}, tc.store, &contractsfake.UserInfoSessionCache{}).
				Execute(context.Background(), txtID, domain.ProxyConfigRequest{
					Enable: true, ProxyURL: "http://203.0.113.10:3128", WebhookUseProxy: tc.requested,
				})
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if len(tc.store.SaveProxyConfigCalls) != 1 {
				t.Fatalf("SaveProxyConfig chamado %d vez(es), quero 1", len(tc.store.SaveProxyConfigCalls))
			}
			if got := tc.store.SaveProxyConfigCalls[0].WebhookUseProxy; got != tc.want {
				t.Errorf("gravou webhook_use_proxy = %v, quero %v", got, tc.want)
			}
			if r.WebhookUseProxy == nil {
				t.Fatal("a resposta do ramo de habilitacao nao ecoou webhook_use_proxy")
			}
			if *r.WebhookUseProxy != tc.want {
				t.Errorf("a resposta ecoou %v, quero %v", *r.WebhookUseProxy, tc.want)
			}
			if tc.requested != nil && len(tc.store.LoadWebhookUseProxyCalls) != 0 {
				t.Errorf("o pedido trouxe o valor e o banco foi lido assim mesmo: %d vez(es)",
					len(tc.store.LoadWebhookUseProxyCalls))
			}
		})
	}
}

// TestSetProxy_SemSessao_E_Permitido é o caminho de SUCESSO que a ARMADILHA 2
// deste repo diz para não deixar de fora: quem NÃO tem sessão nenhuma precisa
// conseguir configurar o proxy — é exatamente o estado de quem vai conectar
// através dele. Trocar SessionStatusReader por SessionGuard aqui inverteria a
// guarda e este é o teste que morde.
func TestSetProxy_SemSessao_E_Permitido(t *testing.T) {
	store := &contractsfake.ProxyConfigStore{}
	status := disconnectedSession()

	r, err := newSetProxy(status, &contractsfake.Logger{}, store, &contractsfake.UserInfoSessionCache{}).
		Execute(context.Background(), txtID, domain.ProxyConfigRequest{Enable: true, ProxyURL: "socks5://203.0.113.10:1080"})
	if err != nil {
		t.Fatalf("usuario sem sessao nao conseguiu configurar o proxy: %v", err)
	}
	if !r.Set {
		t.Error("Set = false no caminho de sucesso")
	}
	if len(status.SessionStatusCalls) != 1 {
		t.Fatalf("SessionStatus consultado %d vez(es), quero 1", len(status.SessionStatusCalls))
	}
	if status.SessionStatusCalls[0].UserID != txtID {
		t.Errorf("SessionStatus recebeu %q, quero %q", status.SessionStatusCalls[0].UserID, txtID)
	}
}

// --- CAP-31: endereço reservado quando o WEBHOOK sai pelo proxy -----------

// verdadeiroP e falsoP são endereços de bool para o campo `webhook_use_proxy`
// do pedido, que é ponteiro porque "ausente" e "false explícito" precisam
// continuar distinguíveis (domain/storage.go).
var verdadeiroP, falsoP = func() (*bool, *bool) {
	v, f := true, false
	return &v, &f
}()

// TestSetProxy_ReservadoDependeDoModo é a tabela da decisão do CAP-31, e o eixo
// que ela varia é o MODO, não o endereço.
//
// A regra: com `webhook_use_proxy` LIGADO a entrega de webhook sai por este
// proxy, então um proxy em loopback ou em faixa reservada controlado pelo
// tenant vira rota para contornar o validador de saída que a fase sec/F24
// instalou (commit 8d9c040) — o tenant escolhe o endereço que o processo passa
// a discar por ele. DESLIGADO, o proxy só carrega o tráfego do próprio servidor
// para o WhatsApp, e proxy interno é o jeito normal de rodar isso.
//
// Os dois lados são obrigatórios. Só as recusas provariam que a guarda existe,
// não que o GATILHO dela é o modo — aplicá-la sempre passaria por metade da
// tabela (ARMADILHA 2: teste o caminho de SUCESSO).
func TestSetProxy_ReservadoDependeDoModo(t *testing.T) {
	cases := []struct {
		name    string
		modo    *bool
		url     string
		wantErr bool
	}{
		// TESTE 1 do pacote.
		{name: "ligado + http em loopback e' RECUSADO", modo: verdadeiroP, url: proxyHTTPLoopback, wantErr: true},
		// TESTE 2 do pacote: RFC1918 é reservado tanto quanto loopback.
		{name: "ligado + socks5 em 10.0.0.0/8 e' RECUSADO", modo: verdadeiroP, url: proxySOCKS5Reservado, wantErr: true},
		// TESTE 3 do pacote: o caminho de sucesso do modo ligado.
		{name: "ligado + IP publico grava", modo: verdadeiroP, url: proxyHTTPPublico},
		// TESTE 4 do pacote — o caso que a decisão PROTEGE.
		{name: "desligado + http em loopback GRAVA", modo: falsoP, url: proxyHTTPLoopback},
		{name: "desligado + socks5 reservado GRAVA", modo: falsoP, url: proxySOCKS5Reservado},
		{name: "desligado + IP publico grava", modo: falsoP, url: proxyHTTPPublico},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &contractsfake.ProxyConfigStore{}
			cache := &contractsfake.UserInfoSessionCache{}

			r, err := newSetProxy(disconnectedSession(), &contractsfake.Logger{}, store, cache).
				Execute(context.Background(), txtID, domain.ProxyConfigRequest{
					Enable: true, ProxyURL: tc.url, WebhookUseProxy: tc.modo,
				})

			if tc.wantErr {
				if err == nil {
					t.Fatalf("proxy %q com webhook_use_proxy=%v devia ser recusado: a entrega de webhook "+
						"sairia por um endereco reservado escolhido pelo tenant", tc.url, *tc.modo)
				}
				if r != nil {
					t.Error("resultado devia ser nil na recusa")
				}
				// TESTE 8 do pacote — a ORDEM: recusa NÃO grava e NÃO publica.
				if len(store.SaveProxyConfigCalls) != 0 {
					t.Errorf("a guarda roda DEPOIS da escrita: o repositorio guardou %+v", store.SaveProxyConfigCalls)
				}
				if len(cache.SetProxyCalls) != 0 {
					t.Errorf("a recusa publicou no cache: %+v", cache.SetProxyCalls)
				}
				return
			}

			if err != nil {
				t.Fatalf("proxy %q com webhook_use_proxy=%v devia ser aceito: %v", tc.url, *tc.modo, err)
			}
			if !r.Set {
				t.Error("Set = false no caminho de sucesso")
			}
			if got := store.Stored[txtID].ProxyURL; got != tc.url {
				t.Errorf("banco guardou %q, quero %q", got, tc.url)
			}
			if len(cache.SetProxyCalls) != 1 || cache.SetProxyCalls[0].ProxyURL != tc.url {
				t.Errorf("cache = %+v, quero uma publicacao de %q", cache.SetProxyCalls, tc.url)
			}
		})
	}
}

// TESTE 5 do pacote — regressão do CAP-30: `socks5` continua aceito nos DOIS
// modos. A guarda nova é de ENDEREÇO; se ela tivesse encostado no esquema, o
// esquema que a rota existe para aceitar teria ido junto.
func TestSetProxy_Socks5_ContinuaAceitoNosDoisModos(t *testing.T) {
	for _, modo := range []*bool{verdadeiroP, falsoP} {
		t.Run(fmt.Sprintf("webhook_use_proxy=%v", *modo), func(t *testing.T) {
			store := &contractsfake.ProxyConfigStore{}

			r, err := newSetProxy(disconnectedSession(), &contractsfake.Logger{}, store, &contractsfake.UserInfoSessionCache{}).
				Execute(context.Background(), txtID, domain.ProxyConfigRequest{
					Enable: true, ProxyURL: proxySOCKS5Publico, WebhookUseProxy: modo,
				})
			if err != nil {
				t.Fatalf("socks5 devia ser aceito com webhook_use_proxy=%v: %v", *modo, err)
			}
			if got := store.Stored[txtID].ProxyURL; got != proxySOCKS5Publico {
				t.Errorf("banco guardou %q, quero %q", got, proxySOCKS5Publico)
			}
			if r.WebhookUseProxy == nil || *r.WebhookUseProxy != *modo {
				t.Errorf("a resposta ecoou %v, quero %v", r.WebhookUseProxy, *modo)
			}
		})
	}
}

// TESTE 6 do pacote — fidelidade histórica: `https` continua RECUSADO nos dois
// modos, e pelo código de esquema, não pelo de endereço. Um proxy HTTPS exige
// um transporte que este processo não monta.
func TestSetProxy_HTTPS_ContinuaRecusadoNosDoisModos(t *testing.T) {
	for _, modo := range []*bool{verdadeiroP, falsoP} {
		t.Run(fmt.Sprintf("webhook_use_proxy=%v", *modo), func(t *testing.T) {
			store := &contractsfake.ProxyConfigStore{}

			_, err := newSetProxy(disconnectedSession(), &contractsfake.Logger{}, store, &contractsfake.UserInfoSessionCache{}).
				Execute(context.Background(), txtID, domain.ProxyConfigRequest{
					Enable: true, ProxyURL: "https://" + proxyHostPublico + ":3128", WebhookUseProxy: modo,
				})
			if err == nil {
				t.Fatalf("https devia ser recusado com webhook_use_proxy=%v", *modo)
			}
			if !strings.Contains(err.Error(), "only HTTP and SOCKS5 proxies are supported") {
				t.Errorf("https recusado pelo motivo errado: %v", err)
			}
			if len(store.SaveProxyConfigCalls) != 0 {
				t.Errorf("a recusa gravou %+v", store.SaveProxyConfigCalls)
			}
		})
	}
}

// TESTE 7 do pacote — de ONDE vem o modo que arma a guarda. É a MESMA regra de
// resolveWebhookUseProxy que o CAP-30 fixou: o valor do pedido quando vier, o
// do banco quando o campo for omitido. Sem isto, omitir `webhook_use_proxy`
// deixaria a guarda decidir por um modo que ninguém configurou.
//
// A divergência proposital está no terceiro caso e é a do TESTE 9, logo abaixo.
func TestSetProxy_ModoDaGuarda_VemDoPedidoOuDoBanco(t *testing.T) {
	cases := []struct {
		name      string
		requested *bool
		store     *contractsfake.ProxyConfigStore
		wantErr   bool
	}{
		{
			name:      "o pedido manda true e o banco diz false: a guarda usa o PEDIDO e recusa",
			requested: verdadeiroP,
			store: &contractsfake.ProxyConfigStore{Stored: map[string]contractsfake.ProxyConfigRecord{
				txtID: {WebhookUseProxy: false},
			}},
			wantErr: true,
		},
		{
			name:      "o pedido manda false e o banco diz true: a guarda usa o PEDIDO e aceita",
			requested: falsoP,
			store: &contractsfake.ProxyConfigStore{Stored: map[string]contractsfake.ProxyConfigRecord{
				txtID: {WebhookUseProxy: true},
			}},
		},
		{
			name:      "o pedido OMITE e o banco diz true: a guarda usa o BANCO e recusa",
			requested: nil,
			store: &contractsfake.ProxyConfigStore{Stored: map[string]contractsfake.ProxyConfigRecord{
				txtID: {WebhookUseProxy: true},
			}},
			wantErr: true,
		},
		{
			name:      "o pedido OMITE e o banco diz false: a guarda usa o BANCO e aceita",
			requested: nil,
			store: &contractsfake.ProxyConfigStore{Stored: map[string]contractsfake.ProxyConfigRecord{
				txtID: {WebhookUseProxy: false},
			}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cache := &contractsfake.UserInfoSessionCache{}

			_, err := newSetProxy(disconnectedSession(), &contractsfake.Logger{}, tc.store, cache).
				Execute(context.Background(), txtID, domain.ProxyConfigRequest{
					Enable: true, ProxyURL: proxyHTTPLoopback, WebhookUseProxy: tc.requested,
				})

			if tc.wantErr {
				if err == nil {
					t.Fatal("loopback devia ser recusado: o modo resolvido liga a guarda")
				}
				if len(tc.store.SaveProxyConfigCalls) != 0 {
					t.Errorf("a recusa gravou %+v", tc.store.SaveProxyConfigCalls)
				}
				if len(cache.SetProxyCalls) != 0 {
					t.Errorf("a recusa publicou no cache: %+v", cache.SetProxyCalls)
				}
				return
			}
			if err != nil {
				t.Fatalf("loopback devia ser aceito com o modo desligado: %v", err)
			}
			if got := tc.store.Stored[txtID].ProxyURL; got != proxyHTTPLoopback {
				t.Errorf("banco guardou %q, quero %q", got, proxyHTTPLoopback)
			}
		})
	}
}

// TESTE 9 (emenda da coordenação) — FAIL-CLOSED: quando o modo NÃO pode ser
// determinado, a guarda assume o modo mais estrito e recusa.
//
// # Por que este teste existe
//
// resolveWebhookUseProxy tem três passos, e o terceiro é um chute: se a leitura
// do banco FALHA, ele cai em `uc.defaultWebhookUseProxy`. Esse default vem de
// `appCtx.GlobalWebhookUseProxy` (wiring_handlers.go), que é configuração de
// instalação e PODE ser false. Se a guarda de segurança lesse esse valor, uma
// falha de leitura do banco numa instalação com o default em false abriria o
// caminho de loopback — falha ABERTA numa decisão de segurança.
//
// Por isso a guarda consulta se o modo foi DETERMINADO, e não só qual ele é.
//
// # A divergência é proposital
//
// O valor PERSISTIDO continua sendo o de resolveWebhookUseProxy (o default do
// processo), porque essa parte é comportamento histórico e não é uma decisão de
// segurança. O valor que DECIDE A GUARDA é o mais estrito. Os dois divergem só
// neste caminho de erro, e divergem de propósito: persiste-se o default e
// recusa-se por precaução.
//
// O default deste teste é FALSE de propósito. Com `true` a recusa aconteceria
// de qualquer jeito e o teste passaria com a guarda lendo o default — que é
// exatamente o defeito que ele existe para pegar.
func TestSetProxy_ModoIndeterminado_RecusaReservadoPorPrecaucao(t *testing.T) {
	const defaultPermissivo = false

	store := &contractsfake.ProxyConfigStore{
		LoadWebhookUseProxyFunc: func(context.Context, string) (bool, error) { return false, errStoreDown },
	}
	cache := &contractsfake.UserInfoSessionCache{}

	r, err := newSetProxyComDefault(disconnectedSession(), &contractsfake.Logger{}, store, cache, defaultPermissivo).
		Execute(context.Background(), txtID, domain.ProxyConfigRequest{
			Enable: true, ProxyURL: proxyHTTPLoopback, // `webhook_use_proxy` OMITIDO: força o passo 3.
		})

	if err == nil {
		t.Fatalf("loopback foi ACEITO com o modo indeterminado: a guarda leu o default do processo (%v) "+
			"em vez de assumir o modo estrito, e uma falha de leitura do banco abre o caminho de loopback",
			defaultPermissivo)
	}
	if r != nil {
		t.Error("resultado devia ser nil na recusa")
	}
	if len(store.SaveProxyConfigCalls) != 0 {
		t.Errorf("a recusa gravou %+v", store.SaveProxyConfigCalls)
	}
	if len(cache.SetProxyCalls) != 0 {
		t.Errorf("a recusa publicou no cache: %+v", cache.SetProxyCalls)
	}

	// O contraste que prova que o gatilho é a INDETERMINAÇÃO, e não o endereço:
	// com a leitura funcionando e devolvendo o MESMO false, o mesmo loopback é
	// aceito. Sem este par, tornar a guarda incondicional passaria acima.
	storeOK := &contractsfake.ProxyConfigStore{Stored: map[string]contractsfake.ProxyConfigRecord{
		txtID: {WebhookUseProxy: false},
	}}
	if _, err := newSetProxyComDefault(disconnectedSession(), &contractsfake.Logger{}, storeOK,
		&contractsfake.UserInfoSessionCache{}, defaultPermissivo).
		Execute(context.Background(), txtID, domain.ProxyConfigRequest{
			Enable: true, ProxyURL: proxyHTTPLoopback,
		}); err != nil {
		t.Fatalf("com a leitura OK devolvendo false, loopback devia passar: %v", err)
	}
}

// A guarda também vale para HOST POR NOME, não só para IP literal: `localhost`
// é o contorno óbvio de uma guarda que só olhasse literais, e RFC 6761 §6.3
// obriga todo resolvedor a mapeá-lo para loopback.
//
// O ramo de nome é o que a produção resolve por DNS (egress.go:213); aqui o
// resolvedor é o dublê rfc6761Resolver, cuja permissividade está amarrada à
// norma — ver o comentário dele.
func TestSetProxy_NomeQueResolveParaLoopback_E_Recusado(t *testing.T) {
	store := &contractsfake.ProxyConfigStore{}
	cache := &contractsfake.UserInfoSessionCache{}

	_, err := newSetProxy(disconnectedSession(), &contractsfake.Logger{}, store, cache).
		Execute(context.Background(), txtID, domain.ProxyConfigRequest{
			Enable: true, ProxyURL: proxyHTTPLocalhost, WebhookUseProxy: verdadeiroP,
		})
	if err == nil {
		t.Fatal("`localhost` foi aceito: a guarda so' olha IP literal e o nome e' o contorno de uma linha")
	}
	if len(store.SaveProxyConfigCalls) != 0 {
		t.Errorf("a recusa gravou %+v", store.SaveProxyConfigCalls)
	}
	if len(cache.SetProxyCalls) != 0 {
		t.Errorf("a recusa publicou no cache: %+v", cache.SetProxyCalls)
	}

	// E com o modo desligado o mesmo nome passa — o gatilho continua sendo o
	// modo, no ramo de nome tanto quanto no de literal.
	storeOff := &contractsfake.ProxyConfigStore{}
	if _, err := newSetProxy(disconnectedSession(), &contractsfake.Logger{}, storeOff,
		&contractsfake.UserInfoSessionCache{}).
		Execute(context.Background(), txtID, domain.ProxyConfigRequest{
			Enable: true, ProxyURL: proxyHTTPLocalhost, WebhookUseProxy: falsoP,
		}); err != nil {
		t.Fatalf("`localhost` devia passar com webhook_use_proxy desligado: %v", err)
	}
}

// Desabilitar NUNCA passa pela guarda de endereço: a URL do pedido é jogada
// fora, e recusar uma remoção por causa dela tornaria uma configuração ruim
// impossível de tirar — que é o mesmo motivo pelo qual o ramo de desabilitação
// já não valida esquema.
func TestSetProxy_Desabilitar_NaoAplicaAGuardaDeEndereco(t *testing.T) {
	store := &contractsfake.ProxyConfigStore{Stored: map[string]contractsfake.ProxyConfigRecord{
		txtID: {ProxyURL: proxyHTTPLoopback, WebhookUseProxy: true},
	}}
	cache := &contractsfake.UserInfoSessionCache{}

	if _, err := newSetProxy(disconnectedSession(), &contractsfake.Logger{}, store, cache).
		Execute(context.Background(), txtID, domain.ProxyConfigRequest{
			Enable: false, ProxyURL: proxyHTTPLoopback, WebhookUseProxy: verdadeiroP,
		}); err != nil {
		t.Fatalf("desabilitar um proxy em loopback devia passar: %v", err)
	}
	if got := store.Stored[txtID].ProxyURL; got != "" {
		t.Fatalf("proxy_url = %q depois de desabilitar, quero vazio", got)
	}
}
