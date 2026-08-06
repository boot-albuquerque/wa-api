package whatsmeow

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain/apperr"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"golang.org/x/net/proxy"
)

// deviceContainer é a superfície do sqlstore.Container que o provider usa
// para materializar o device de uma sessão. Estreita por testabilidade,
// pelo mesmo motivo de waClient em wa_client_seam.go.
type deviceContainer interface {
	GetDevice(ctx context.Context, jid types.JID) (*store.Device, error)
	NewDevice() *store.Device
}

// sessionClient é a superfície de *whatsmeow.Client que uma Session exercita.
// Store.ID não aparece aqui: o adapter guarda o *store.Device com que criou o
// cliente (é o mesmo ponteiro de client.Store) e lê as credenciais de lá.
type sessionClient interface {
	GetQRChannel(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error)
	Connect() error
	Disconnect()
	IsConnected() bool
	IsLoggedIn() bool
	Logout(ctx context.Context) error
	AddEventHandler(handler whatsmeow.EventHandler) uint32
	RemoveEventHandler(id uint32) bool
	SetSOCKSProxy(px proxy.Dialer, opts ...whatsmeow.SetProxyOptions)
	SetProxyAddress(addr string, opts ...whatsmeow.SetProxyOptions) error
}

// DeviceJIDLookup resolve o JID do device já persistido para um userID (hoje
// a coluna users.jid). Devolve string vazia quando ainda não houve
// pareamento — nesse caso o provider cria um device novo.
type DeviceJIDLookup func(ctx context.Context, userID string) (string, error)

// SessionProviderAdapter implementa appport.SessionProvider sobre o whatsmeow,
// encapsulando NewClient/GetQRChannel/Connect/AddEventHandler.
type SessionProviderAdapter struct {
	container deviceContainer
	lookupJID DeviceJIDLookup
	newClient func(*store.Device) sessionClient
}

// NewSessionProviderAdapter cria o provider. newClient permite injetar a
// construção do cliente; quando nil, usa whatsmeow.NewClient sem logger.
func NewSessionProviderAdapter(
	container deviceContainer,
	lookupJID DeviceJIDLookup,
	newClient func(*store.Device) sessionClient,
) *SessionProviderAdapter {
	if newClient == nil {
		newClient = func(dev *store.Device) sessionClient {
			return whatsmeow.NewClient(dev, nil)
		}
	}
	return &SessionProviderAdapter{container: container, lookupJID: lookupJID, newClient: newClient}
}

// NewSessionProviderWithLogger monta o provider aplicando logger do SDK aos
// clientes criados. Existe porque o parâmetro newClient de
// NewSessionProviderAdapter usa o tipo não-exportado sessionClient e por isso
// não pode ser construído fora deste pacote. logger nil equivale a cliente sem
// log, como whatsmeow.NewClient(dev, nil).
func NewSessionProviderWithLogger(container deviceContainer, lookupJID DeviceJIDLookup, logger waLog.Logger) *SessionProviderAdapter {
	return NewSessionProviderAdapter(container, lookupJID, func(dev *store.Device) sessionClient {
		return whatsmeow.NewClient(dev, logger)
	})
}

// NewSession materializa a sessão de spec.UserID sem conectar nem parear.
func (p *SessionProviderAdapter) NewSession(ctx context.Context, spec appport.SessionSpec) (appport.Session, error) {
	device, err := p.resolveDevice(ctx, spec.UserID)
	if err != nil {
		return nil, err
	}
	return &whatsmeowSession{
		userID: spec.UserID,
		token:  spec.Token,
		device: device,
		client: p.newClient(device),
	}, nil
}

// resolveDevice reaproveita o device persistido quando há JID conhecido e
// recuperável; caso contrário cria um novo, como startClient hoje faz.
func (p *SessionProviderAdapter) resolveDevice(ctx context.Context, userID string) (*store.Device, error) {
	textJID := ""
	if p.lookupJID != nil {
		var err error
		textJID, err = p.lookupJID(ctx, userID)
		if err != nil {
			return nil, apperr.New("session_device_lookup_failed", apperr.CategoryInternal, "failed to resolve device jid", true, err)
		}
	}

	if textJID != "" {
		jid, ok := ParseJID(textJID)
		if ok {
			if device, derr := p.container.GetDevice(ctx, jid); derr == nil && device != nil {
				return device, nil
			}
		}
	}

	device := p.container.NewDevice()
	if device == nil {
		return nil, apperr.New("session_device_create_failed", apperr.CategoryInternal, "failed to create device store", true, nil)
	}
	return device, nil
}

var _ appport.SessionProvider = (*SessionProviderAdapter)(nil)

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

// Pair inicia o fluxo de QR e conecta o transporte (a conexão é o que faz o
// whatsmeow emitir os códigos). O canal devolvido é fechado quando o canal
// do SDK fecha.
func (s *whatsmeowSession) Pair(ctx context.Context) (<-chan appport.PairingEvent, error) {
	if s.HasCredentials() {
		return nil, apperr.New("session_already_paired", apperr.CategoryValidation, "session already has credentials", false, nil)
	}

	qrChan, err := s.client.GetQRChannel(ctx)
	if err != nil {
		if errors.Is(err, whatsmeow.ErrQRStoreContainsID) {
			return nil, apperr.New("session_already_paired", apperr.CategoryValidation, "session already has credentials", false, err)
		}
		return nil, apperr.New("qr_channel_failed", apperr.CategoryInternal, "failed to get QR channel", true, err)
	}

	if err := s.client.Connect(); err != nil {
		return nil, apperr.New("qr_connect_failed", apperr.CategoryInternal, "failed to connect client for QR pairing", true, err)
	}

	out := make(chan appport.PairingEvent)
	go func() {
		defer close(out)
		for item := range qrChan {
			evt, ok := s.translatePairingItem(item)
			if !ok {
				continue
			}
			select {
			case out <- evt:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (s *whatsmeowSession) translatePairingItem(item whatsmeow.QRChannelItem) (appport.PairingEvent, bool) {
	switch item.Event {
	case whatsmeow.QRChannelEventCode:
		return appport.PairingEvent{
			Kind:    appport.PairingEventKindQR,
			Code:    item.Code,
			Timeout: item.Timeout,
		}, true
	case "timeout":
		return appport.PairingEvent{Kind: appport.PairingEventKindTimeout}, true
	case "success":
		jid, _ := s.JID()
		return appport.PairingEvent{Kind: appport.PairingEventKindSuccess, JID: jid}, true
	default:
		return appport.PairingEvent{}, false
	}
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

// Subscribe traduz os 6 eventos de sessão/transporte do whatsmeow para
// appport.SessionEvent. Eventos de domínio (mensagem, presença, grupo) não
// passam por aqui: seguem no handler registrado via SessionAttachHook.
func (s *whatsmeowSession) Subscribe(fn func(appport.SessionEvent)) (func(), error) {
	if fn == nil {
		return nil, apperr.New("session_subscribe_nil_handler", apperr.CategoryValidation, "subscriber must not be nil", false, nil)
	}

	id := s.client.AddEventHandler(func(raw any) {
		if evt, ok := translateSessionEvent(raw); ok {
			fn(evt)
		}
	})

	var once sync.Once
	return func() { once.Do(func() { s.client.RemoveEventHandler(id) }) }, nil
}

func translateSessionEvent(raw any) (appport.SessionEvent, bool) {
	switch evt := raw.(type) {
	case *events.Connected:
		return appport.SessionEvent{Kind: appport.SessionEventKindConnected}, true
	case *events.Disconnected:
		return appport.SessionEvent{
			Kind:         appport.SessionEventKindDisconnected,
			Disconnected: &appport.SessionDisconnectedEvent{Reason: fmt.Sprintf("%+v", evt)},
		}, true
	case *events.LoggedOut:
		return appport.SessionEvent{
			Kind:      appport.SessionEventKindLoggedOut,
			LoggedOut: &appport.SessionLoggedOutEvent{Reason: evt.Reason.String()},
		}, true
	case *events.PairSuccess:
		return appport.SessionEvent{
			Kind: appport.SessionEventKindPairSuccess,
			PairSuccess: &appport.SessionPairSuccessEvent{
				JID:          evt.ID.String(),
				BusinessName: evt.BusinessName,
				Platform:     evt.Platform,
			},
		}, true
	case *events.QR:
		qr := &appport.SessionQREvent{}
		if len(evt.Codes) > 0 {
			qr.Code = evt.Codes[0]
		}
		return appport.SessionEvent{Kind: appport.SessionEventKindQR, QR: qr}, true
	case *events.StreamReplaced:
		return appport.SessionEvent{Kind: appport.SessionEventKindStreamReplaced}, true
	default:
		return appport.SessionEvent{}, false
	}
}

var _ appport.Session = (*whatsmeowSession)(nil)
