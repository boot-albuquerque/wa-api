package storage

import (
	"context"
	"fmt"
	"net/url"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// Result and error messages of this use case, as constants (ADR-0004). Every
// string is the historical text: `41bc8e2^:handlers.go:6193`, `:6138`,
// `:6100`, `:6150`, `:6156`, `:6162`, `:6179` and `:6125`.
const (
	proxyConfiguredDetails = "Proxy configured successfully"
	proxyDisabledDetails   = "Proxy disabled successfully"

	proxyWhileConnectedMsg = "cannot set proxy while connected. Please disconnect first"
	proxyMissingURLMsg     = "missing proxy_url in payload"
	proxyInvalidURLMsg     = "invalid proxy URL format"
	proxyUnsupportedMsg    = "only HTTP and SOCKS5 proxies are supported"
	proxySaveFailedMsg     = "failed to save proxy configuration"
	proxyRemoveFailedMsg   = "failed to remove proxy configuration"

	proxyWhileConnectedCode = "proxy_while_connected"
	proxyMissingURLCode     = "missing_proxy_url"
	proxyInvalidURLCode     = "invalid_proxy_url"
	proxyUnsupportedCode    = "unsupported_proxy_scheme"
)

// SetProxyUseCase writes the per-user proxy configuration.
type SetProxyUseCase struct {
	status appport.SessionStatusReader
	store  appport.ProxyConfigStore
	cache  appport.UserInfoProxyCache
	logger appport.Logger

	// defaultWebhookUseProxy is the process-wide value applied when the
	// request omits `webhook_use_proxy` AND the stored one cannot be read.
	// Injected as a value, like Orchestrator.defaultWebhookUseProxy, because
	// pkg/application does not read flags.
	defaultWebhookUseProxy bool
}

// NewSetProxyUseCase creates the use case.
func NewSetProxyUseCase(
	status appport.SessionStatusReader,
	store appport.ProxyConfigStore,
	cache appport.UserInfoProxyCache,
	defaultWebhookUseProxy bool,
	l appport.Logger,
) *SetProxyUseCase {
	return &SetProxyUseCase{
		status:                 status,
		store:                  store,
		cache:                  cache,
		logger:                 l,
		defaultWebhookUseProxy: defaultWebhookUseProxy,
	}
}

// Execute refuses a connected session, then validates, writes and publishes.
//
// # The guard is FIRST, and that is the contract
//
// A live session already has its transport dialled; swapping the proxy under
// it changes nothing about the connection that exists and leaves the stored
// configuration disagreeing with the socket in use. The historical handler
// refused before doing anything else (`41bc8e2^:handlers.go:6099`), and moving
// this check below the write would still answer 400 at the end — with the
// proxy ALREADY STORED. Every test that only looks at the status code would
// keep passing. TestSetProxy_ClienteConectado_NaoChamaORepositorio asserts the
// repository was never reached, which is the part a status code cannot show.
//
// This use case takes SessionStatusReader and NOT SessionGuard on purpose: the
// two ask opposite questions. SessionGuard refuses when there is NO session;
// here the refusal is for a session that IS connected, and a user with no
// session at all must be allowed to configure a proxy — that is precisely the
// state you are in when you are about to connect through one.
//
// # Divergence from the historical order, stated because it is observable
//
// The historical handler ran this guard before DECODING the body; in this fork
// the body is decoded by the handler, at the wire boundary, like every other
// route here. So a connected client sending a malformed body gets 400
// "could not decode payload" where the historical answered 400 "cannot set
// proxy while connected". Both refuse, both are 400, and neither writes — the
// difference is the error code in the envelope.
func (uc *SetProxyUseCase) Execute(ctx context.Context, txtID string, req domain.ProxyConfigRequest) (*domain.ProxyConfigResult, error) {
	if connected, _ := uc.status.SessionStatus(ctx, txtID); connected {
		uc.logger.Warn(ctx, proxyWhileConnectedMsg, "txtID", txtID)
		return nil, apperr.New(proxyWhileConnectedCode, apperr.CategoryValidation, proxyWhileConnectedMsg, false, nil)
	}

	if !req.Enable {
		return uc.disable(ctx, txtID, req)
	}
	return uc.enable(ctx, txtID, req)
}

// disable clears users.proxy_url and keeps webhook_use_proxy resolved.
//
// The URL in the request is deliberately NOT validated here: the historical
// disable branch never looked at it, and refusing a disable because the URL
// that is about to be thrown away is malformed would make a broken
// configuration impossible to remove.
func (uc *SetProxyUseCase) disable(ctx context.Context, txtID string, req domain.ProxyConfigRequest) (*domain.ProxyConfigResult, error) {
	webhookUseProxy := uc.resolveWebhookUseProxy(ctx, txtID, req.WebhookUseProxy)

	if err := uc.store.SaveProxyConfig(ctx, txtID, "", webhookUseProxy); err != nil {
		uc.logger.Error(ctx, proxyRemoveFailedMsg, "txtID", txtID, "error", err)
		return nil, fmt.Errorf("%s: %w", proxyRemoveFailedMsg, err)
	}

	uc.cache.SetProxy(ctx, txtID, "")

	uc.logger.Info(ctx, proxyDisabledDetails, "txtID", txtID)
	return &domain.ProxyConfigResult{Details: proxyDisabledDetails}, nil
}

// enable validates the URL and stores it.
func (uc *SetProxyUseCase) enable(ctx context.Context, txtID string, req domain.ProxyConfigRequest) (*domain.ProxyConfigResult, error) {
	if req.ProxyURL == "" {
		uc.logger.Warn(ctx, proxyMissingURLMsg, "txtID", txtID)
		return nil, apperr.New(proxyMissingURLCode, apperr.CategoryValidation, proxyMissingURLMsg, false, nil)
	}

	// The raw URL never enters a log or an error message: a proxy URL carries
	// credentials in its userinfo ("socks5://user:pass@host:port" is the
	// documented shape, `41bc8e2^:handlers.go:6087`). Only the SCHEME is
	// observable, and only after parsing succeeded.
	parsed, err := url.Parse(req.ProxyURL)
	if err != nil {
		uc.logger.Warn(ctx, proxyInvalidURLMsg, "txtID", txtID)
		return nil, apperr.New(proxyInvalidURLCode, apperr.CategoryValidation, proxyInvalidURLMsg, false, nil)
	}

	if !domain.IsSupportedProxyScheme(parsed.Scheme) {
		uc.logger.Warn(ctx, proxyUnsupportedMsg, "txtID", txtID, "scheme", parsed.Scheme)
		return nil, apperr.New(proxyUnsupportedCode, apperr.CategoryValidation, proxyUnsupportedMsg, false, nil)
	}

	webhookUseProxy := uc.resolveWebhookUseProxy(ctx, txtID, req.WebhookUseProxy)

	if err := uc.store.SaveProxyConfig(ctx, txtID, req.ProxyURL, webhookUseProxy); err != nil {
		uc.logger.Error(ctx, proxySaveFailedMsg, "txtID", txtID, "error", err)
		return nil, fmt.Errorf("%s: %w", proxySaveFailedMsg, err)
	}

	uc.cache.SetProxy(ctx, txtID, req.ProxyURL)

	uc.logger.Info(ctx, proxyConfiguredDetails, "txtID", txtID, "scheme", parsed.Scheme)
	return &domain.ProxyConfigResult{
		Details:         proxyConfiguredDetails,
		Set:             true,
		ProxyURL:        req.ProxyURL,
		WebhookUseProxy: &webhookUseProxy,
	}, nil
}

// resolveWebhookUseProxy reproduces the historical three-step resolution
// (`41bc8e2^:handlers.go:6118` and `:6167`), in this order:
//
//  1. the value the request sent, when it sent one;
//  2. otherwise the value STORED for this user, so that a proxy write does not
//     silently reset a flag the caller never mentioned;
//  3. otherwise the process default — which is what the historical code fell
//     back to, because its Scan error was ignored and left the variable
//     holding resolveWebhookUseProxy(nil).
//
// A read failure is logged and swallowed for that last reason: it is the
// historical behaviour, and failing the whole proxy write because an auxiliary
// preference could not be read would be a worse outcome than defaulting it.
func (uc *SetProxyUseCase) resolveWebhookUseProxy(ctx context.Context, txtID string, requested *bool) bool {
	if requested != nil {
		return *requested
	}
	stored, err := uc.store.LoadWebhookUseProxy(ctx, txtID)
	if err != nil {
		uc.logger.Warn(ctx, "could not read the stored webhook_use_proxy; falling back to the process default",
			"txtID", txtID, "error", err, "default", uc.defaultWebhookUseProxy)
		return uc.defaultWebhookUseProxy
	}
	return stored
}
