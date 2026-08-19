package contractsfake

import (
	"context"

	port "wa-api/pkg/application/contracts"
)

// --- HistoryConfigStore ------------------------------------------------

// HistoryConfigStoreSaveCall é uma chamada a SaveHistoryLimit.
type HistoryConfigStoreSaveCall struct {
	Ctx     context.Context
	UserID  string
	History int
}

// HistoryConfigStore é o fake de port.HistoryConfigStore. O zero-value grava
// em memória, como o UPDATE de uma coluna só que ele imita
// (pkg/infra/db/session_config_repository.go, historyLimitUpdateQuery).
type HistoryConfigStore struct {
	// Stored é o estado observável do "banco" do dublê.
	Stored map[string]int

	SaveHistoryLimitFunc  func(ctx context.Context, userID string, history int) error
	SaveHistoryLimitCalls []HistoryConfigStoreSaveCall

	LoadHistoryLimitFunc  func(ctx context.Context, userID string) (int, error)
	LoadHistoryLimitCalls []string
}

var _ port.HistoryConfigStore = (*HistoryConfigStore)(nil)

// SaveHistoryLimit implementa port.HistoryConfigStore.
func (f *HistoryConfigStore) SaveHistoryLimit(ctx context.Context, userID string, history int) error {
	f.SaveHistoryLimitCalls = append(f.SaveHistoryLimitCalls, HistoryConfigStoreSaveCall{Ctx: ctx, UserID: userID, History: history})
	if f.SaveHistoryLimitFunc != nil {
		return f.SaveHistoryLimitFunc(ctx, userID, history)
	}
	if f.Stored == nil {
		f.Stored = map[string]int{}
	}
	f.Stored[userID] = history
	return nil
}

// LoadHistoryLimit implementa port.HistoryConfigStore.
//
// Imita a regra REAL do adapter de producao
// (pkg/infra/db/session_config_repository.go, historyLimitSelectQuery com
// `COALESCE(history, 0)`): usuario sem linha le' 0, isto e', historico
// DESLIGADO — nao um erro e nao um default inventado.
func (f *HistoryConfigStore) LoadHistoryLimit(ctx context.Context, userID string) (int, error) {
	f.LoadHistoryLimitCalls = append(f.LoadHistoryLimitCalls, userID)
	if f.LoadHistoryLimitFunc != nil {
		return f.LoadHistoryLimitFunc(ctx, userID)
	}
	return f.Stored[userID], nil
}

// --- ProxyConfigStore --------------------------------------------------

// ProxyConfigRecord é o par de colunas que ProxyConfigStore grava junto.
type ProxyConfigRecord struct {
	ProxyURL        string
	WebhookUseProxy bool
}

// ProxyConfigStoreSaveCall é uma chamada a SaveProxyConfig.
type ProxyConfigStoreSaveCall struct {
	Ctx             context.Context
	UserID          string
	ProxyURL        string
	WebhookUseProxy bool
}

// ProxyConfigStoreLoadCall é uma chamada a LoadWebhookUseProxy.
type ProxyConfigStoreLoadCall struct {
	Ctx    context.Context
	UserID string
}

// ProxyConfigStore é o fake de port.ProxyConfigStore.
//
// Duas regras vêm do adapter de produção
// (pkg/infra/db/session_config_repository.go) e NÃO são simplificações:
//
//  1. gravar escreve as DUAS colunas na mesma operação — proxy_url e
//     webhook_use_proxy nunca são observáveis em desacordo;
//  2. usuário sem linha lê `true` em LoadWebhookUseProxy, que é o default da
//     coluna (`webhook_use_proxy BOOLEAN DEFAULT TRUE`, migrations.go:508) e o
//     que o COALESCE do SELECT devolve. Um dublê que devolvesse `false` aqui
//     inverteria o caminho de preservação sem que nada acusasse.
type ProxyConfigStore struct {
	// Stored é o estado observável do "banco" do dublê.
	Stored map[string]ProxyConfigRecord

	SaveProxyConfigFunc     func(ctx context.Context, userID, proxyURL string, webhookUseProxy bool) error
	LoadWebhookUseProxyFunc func(ctx context.Context, userID string) (bool, error)

	SaveProxyConfigCalls     []ProxyConfigStoreSaveCall
	LoadWebhookUseProxyCalls []ProxyConfigStoreLoadCall
}

var _ port.ProxyConfigStore = (*ProxyConfigStore)(nil)

// DefaultWebhookUseProxy é o valor que um usuário sem linha reporta. Espelha
// db.defaultWebhookUseProxy.
const DefaultWebhookUseProxy = true

// SaveProxyConfig implementa port.ProxyConfigStore.
func (f *ProxyConfigStore) SaveProxyConfig(ctx context.Context, userID, proxyURL string, webhookUseProxy bool) error {
	f.SaveProxyConfigCalls = append(f.SaveProxyConfigCalls, ProxyConfigStoreSaveCall{
		Ctx: ctx, UserID: userID, ProxyURL: proxyURL, WebhookUseProxy: webhookUseProxy,
	})
	if f.SaveProxyConfigFunc != nil {
		return f.SaveProxyConfigFunc(ctx, userID, proxyURL, webhookUseProxy)
	}
	if f.Stored == nil {
		f.Stored = map[string]ProxyConfigRecord{}
	}
	f.Stored[userID] = ProxyConfigRecord{ProxyURL: proxyURL, WebhookUseProxy: webhookUseProxy}
	return nil
}

// LoadWebhookUseProxy implementa port.ProxyConfigStore.
func (f *ProxyConfigStore) LoadWebhookUseProxy(ctx context.Context, userID string) (bool, error) {
	f.LoadWebhookUseProxyCalls = append(f.LoadWebhookUseProxyCalls, ProxyConfigStoreLoadCall{Ctx: ctx, UserID: userID})
	if f.LoadWebhookUseProxyFunc != nil {
		return f.LoadWebhookUseProxyFunc(ctx, userID)
	}
	if rec, ok := f.Stored[userID]; ok {
		return rec.WebhookUseProxy, nil
	}
	return DefaultWebhookUseProxy, nil
}

// --- UserInfoHistoryCache / UserInfoProxyCache -------------------------

// UserInfoSessionCacheCall é uma publicação no cache de userinfo.
type UserInfoSessionCacheCall struct {
	Ctx      context.Context
	UserID   string
	History  int
	ProxyURL string
}

// UserInfoSessionCache é o fake de port.UserInfoHistoryCache e de
// port.UserInfoProxyCache.
//
// Os dois ports vivem num dublê só porque o adapter de produção também é um só
// (pkg/bootstrap/session_config_adapters.go): separar aqui daria a impressão
// de que um use case pode publicar num cache sem publicar no outro.
type UserInfoSessionCache struct {
	SetHistoryFunc func(ctx context.Context, userID string, history int)
	SetProxyFunc   func(ctx context.Context, userID, proxyURL string)

	SetHistoryCalls []UserInfoSessionCacheCall
	SetProxyCalls   []UserInfoSessionCacheCall
}

var (
	_ port.UserInfoHistoryCache = (*UserInfoSessionCache)(nil)
	_ port.UserInfoProxyCache   = (*UserInfoSessionCache)(nil)
)

// SetHistory implementa port.UserInfoHistoryCache.
func (f *UserInfoSessionCache) SetHistory(ctx context.Context, userID string, history int) {
	f.SetHistoryCalls = append(f.SetHistoryCalls, UserInfoSessionCacheCall{Ctx: ctx, UserID: userID, History: history})
	if f.SetHistoryFunc != nil {
		f.SetHistoryFunc(ctx, userID, history)
	}
}

// SetProxy implementa port.UserInfoProxyCache.
func (f *UserInfoSessionCache) SetProxy(ctx context.Context, userID, proxyURL string) {
	f.SetProxyCalls = append(f.SetProxyCalls, UserInfoSessionCacheCall{Ctx: ctx, UserID: userID, ProxyURL: proxyURL})
	if f.SetProxyFunc != nil {
		f.SetProxyFunc(ctx, userID, proxyURL)
	}
}
