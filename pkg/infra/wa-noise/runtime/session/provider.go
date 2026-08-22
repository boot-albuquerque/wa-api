package session

import (
	"context"
	wajid "wa-api/pkg/infra/wa-noise/mapping/jid"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain/apperr"

	wanoise "wa-api/internal/wa-noise"
	waLog "wa-api/internal/wa-noise/observability/log"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"

	"golang.org/x/net/proxy"
)

// deviceContainer é a superfície do sqlstore.Container que o provider usa
// para materializar o device de uma sessão. Estreita por testabilidade,
// pelo mesmo motivo de waclient.Client em wa_client_seam.go.
type deviceContainer interface {
	GetDevice(ctx context.Context, jid types.JID) (*store.Device, error)
	NewDevice() *store.Device
}

// sessionClient é a superfície de *wanoise.Client que uma Session exercita.
// Store.ID não aparece aqui: o adapter guarda o *store.Device com que criou o
// cliente (é o mesmo ponteiro de client.Store) e lê as credenciais de lá.
type sessionClient interface {
	GetQRChannel(ctx context.Context) (<-chan wanoise.QRChannelItem, error)
	Connect() error
	Disconnect()
	IsConnected() bool
	IsLoggedIn() bool
	Logout(ctx context.Context) error
	AddEventHandler(handler wanoise.EventHandler) uint32
	RemoveEventHandler(id uint32) bool
	SetSOCKSProxy(px proxy.Dialer, opts ...wanoise.SetProxyOptions)
	SetProxyAddress(addr string, opts ...wanoise.SetProxyOptions) error
}

// DeviceJIDLookup resolve o JID do device já persistido para um userID (hoje
// a coluna users.jid). Devolve string vazia quando ainda não houve
// pareamento — nesse caso o provider cria um device novo.
type DeviceJIDLookup func(ctx context.Context, userID string) (string, error)

// SessionProviderAdapter implementa appport.SessionProvider sobre o wa-noise,
// encapsulando NewClient/GetQRChannel/Connect/AddEventHandler.
type SessionProviderAdapter struct {
	container deviceContainer
	lookupJID DeviceJIDLookup
	newClient func(*store.Device) sessionClient
}

// NewSessionProviderAdapter cria o provider. newClient permite injetar a
// construção do cliente; quando nil, usa wa-noise.NewClient sem logger.
func NewSessionProviderAdapter(
	container deviceContainer,
	lookupJID DeviceJIDLookup,
	newClient func(*store.Device) sessionClient,
) *SessionProviderAdapter {
	if newClient == nil {
		newClient = func(dev *store.Device) sessionClient {
			cli := wanoise.NewClient(dev, nil)
			cli.UseRetryMessageStore = true
			return cli
		}
	}
	return &SessionProviderAdapter{container: container, lookupJID: lookupJID, newClient: newClient}
}

// NewSessionProviderWithLogger monta o provider aplicando logger do SDK aos
// clientes criados. Existe porque o parâmetro newClient de
// NewSessionProviderAdapter usa o tipo não-exportado sessionClient e por isso
// não pode ser construído fora deste pacote. logger nil equivale a cliente sem
// log, como wa-noise.NewClient(dev, nil).
func NewSessionProviderWithLogger(container deviceContainer, lookupJID DeviceJIDLookup, logger waLog.Logger) *SessionProviderAdapter {
	return NewSessionProviderAdapter(container, lookupJID, func(dev *store.Device) sessionClient {
		cli := wanoise.NewClient(dev, logger)
		cli.UseRetryMessageStore = true
		return cli
	})
}

// NewSession materializa a sessão de spec.UserID sem conectar nem parear.
func (p *SessionProviderAdapter) NewSession(ctx context.Context, spec appport.SessionSpec) (appport.Session, error) {
	device, err := p.resolveDevice(ctx, spec.UserID)
	if err != nil {
		return nil, err
	}
	return &wanoiseSession{
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
		jid, ok := wajid.ParseJID(textJID)
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
