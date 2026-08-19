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
	"testing"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/storage"
	"wa-api/pkg/domain"
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
	return storage.NewSetProxyUseCase(status, store, cache, defaultWebhookUseProxyForTest, log)
}

// disconnectedSession é o estado em que o proxy PODE ser configurado: sem
// sessão conectada. É o zero-value do dublê, nomeado aqui para que o caso de
// sucesso diga qual estado está exercitando.
func disconnectedSession() *contractsfake.SessionStatusReader {
	return &contractsfake.SessionStatusReader{}
}

// connectedSession imita a regra REAL do adapter de produção
// (pkg/infra/wa-noise/runtime/session/guard.go:72): SessionStatus devolve
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
		{"corpo valido", domain.ProxyConfigRequest{Enable: true, ProxyURL: "http://proxy.invalid:3128"}},
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
func TestSetProxy_Esquemas(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "http passa", url: "http://proxy.invalid:3128"},
		{name: "socks5 passa", url: "socks5://user:pass@proxy.invalid:1080"},
		{name: "http em loopback passa — proxy interno e' o caso normal", url: "http://127.0.0.1:3128"},
		{name: "https e' recusado", url: "https://proxy.invalid:3128", wantErr: true},
		{name: "ftp e' recusado", url: "ftp://proxy.invalid:21", wantErr: true},
		{name: "sem esquema e' recusado", url: "proxy.invalid:3128", wantErr: true},
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
		txtID: {ProxyURL: "socks5://proxy.invalid:1080", WebhookUseProxy: true},
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
		{"habilitar", domain.ProxyConfigRequest{Enable: true, ProxyURL: "http://proxy.invalid:3128"}},
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
					Enable: true, ProxyURL: "http://proxy.invalid:3128", WebhookUseProxy: tc.requested,
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
		Execute(context.Background(), txtID, domain.ProxyConfigRequest{Enable: true, ProxyURL: "socks5://proxy.invalid:1080"})
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
