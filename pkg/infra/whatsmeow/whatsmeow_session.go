package whatsmeow

import (
	"context"
	"net/url"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain/apperr"

	whatsmeow "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/store"

	"golang.org/x/net/proxy"
)

// whatsmeowSession implementa appport.Session sobre um cliente whatsmeow.
type whatsmeowSession struct {
	userID string
	token  string
	device *store.Device
	client sessionClient
}

func (s *whatsmeowSession) HasCredentials() bool { return s.device.ID != nil }

// WhatsmeowClient expõe o cliente do SDK por trás da sessão. É o que permite
// ao ClientManager.Register manter whatsmeowClients em dia sem que o
// orchestrator (que só conhece port.Session) conheça o SDK. Devolve nil quando
// a sessão foi criada com um cliente falso (testes).
func (s *whatsmeowSession) WhatsmeowClient() *whatsmeow.Client {
	client, _ := s.client.(*whatsmeow.Client)
	return client
}

func (s *whatsmeowSession) JID() (string, bool) {
	if s.device.ID == nil {
		return "", false
	}
	return s.device.ID.String(), true
}

// Connect estabelece o transporte de uma sessão já pareada.
func (s *whatsmeowSession) Connect(_ context.Context) error {
	if !s.HasCredentials() {
		return apperr.New("session_not_paired", apperr.CategoryValidation, "session has no credentials", false, nil)
	}
	if err := s.client.Connect(); err != nil {
		return apperr.New("session_connect_failed", apperr.CategoryInternal, "failed to connect session", true, err)
	}
	return nil
}

func (s *whatsmeowSession) Disconnect()       { s.client.Disconnect() }
func (s *whatsmeowSession) IsConnected() bool { return s.client.IsConnected() }
func (s *whatsmeowSession) IsLoggedIn() bool  { return s.client.IsLoggedIn() }

func (s *whatsmeowSession) Logout(ctx context.Context) error {
	if err := s.client.Logout(ctx); err != nil {
		return apperr.New("session_logout_failed", apperr.CategoryInternal, "failed to logout session", true, err)
	}
	return nil
}

// SetProxy aplica o proxy no transporte. Invariante do port: só vale antes de
// Connect/Pair — com o transporte de pé devolve erro em vez de aplicar
// silenciosamente na próxima reconexão.
func (s *whatsmeowSession) SetProxy(cfg appport.ProxyConfig) error {
	if s.client.IsConnected() {
		return apperr.New("session_proxy_after_connect", apperr.CategoryValidation, "proxy must be set before connecting", false, nil)
	}

	parsed, err := url.Parse(cfg.URL)
	if err != nil {
		return apperr.New("session_proxy_invalid_url", apperr.CategoryValidation, "invalid proxy URL", false, err)
	}

	switch cfg.Mode {
	case appport.ProxyModeSOCKS5:
		dialer, derr := proxy.FromURL(parsed, nil)
		if derr != nil {
			return apperr.New("session_proxy_dialer_failed", apperr.CategoryInternal, "failed to build SOCKS proxy dialer", true, derr)
		}
		s.client.SetSOCKSProxy(dialer, whatsmeow.SetProxyOptions{})
		return nil
	case appport.ProxyModeHTTP:
		if perr := s.client.SetProxyAddress(parsed.String(), whatsmeow.SetProxyOptions{}); perr != nil {
			return apperr.New("session_proxy_address_failed", apperr.CategoryInternal, "failed to set HTTP proxy address", true, perr)
		}
		return nil
	default:
		return apperr.New("session_proxy_unknown_mode", apperr.CategoryValidation, "unknown proxy mode", false, nil)
	}
}

var _ appport.Session = (*whatsmeowSession)(nil)
