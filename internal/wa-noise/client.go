// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package whatsmeow implements a client for interacting with the WhatsApp web multidevice API.
package whatsmeow

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"go.mau.fi/util/exsync"
	"go.mau.fi/util/ptr"
	"go.mau.fi/util/random"

	"wa-api/internal/wa-noise/appstate"
	"wa-api/internal/wa-noise/appstatesync"
	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/group"
	"wa-api/internal/wa-noise/media"
	"wa-api/internal/wa-noise/pairing"
	"wa-api/internal/wa-noise/prekeys"
	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/proto/waWa6"
	"wa-api/internal/wa-noise/retry"
	"wa-api/internal/wa-noise/socket"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/tctoken"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
	"wa-api/internal/wa-noise/user"
	waLog "wa-api/internal/wa-noise/util/log"
)

// Client contains everything necessary to connect to and interact with the WhatsApp web API.
type Client struct {
	Store   *store.Device
	Log     waLog.Logger
	recvLog waLog.Logger
	sendLog waLog.Logger

	socket     *socket.NoiseSocket
	socketLock sync.RWMutex
	socketWait chan struct{}

	isLoggedIn            atomic.Bool
	expectedDisconnect    *exsync.Event
	forceAutoReconnect    atomic.Bool
	EnableAutoReconnect   bool
	InitialAutoReconnect  bool
	LastSuccessfulConnect time.Time
	AutoReconnectErrors   int
	// AutoReconnectHook is called when auto-reconnection fails. If the function returns false,
	// the client will not attempt to reconnect. The number of retries can be read from AutoReconnectErrors.
	AutoReconnectHook func(error) bool
	// If SynchronousAck is set, acks for messages will only be sent after all event handlers return.
	SynchronousAck             bool
	EnableDecryptedEventBuffer bool
	lastDecryptedBufferClear   time.Time

	DisableLoginAutoReconnect bool

	sendActiveReceipts atomic.Uint32

	// EmitAppStateEventsOnFullSync can be set to true if you want to get app state events emitted
	// even when re-syncing the whole state.
	EmitAppStateEventsOnFullSync bool
	AppStateDebugLogs            bool

	AutomaticMessageRerequestFromPhone bool

	appStateProc *appstate.Processor
	// appStateSync reune os antigos appStateSyncLock, appStateKeyRequests e
	// appStateKeyRequestsLock; os dois locks de dentro continuam sendo dois,
	// com os mesmos pontos de aquisicao.
	appStateSync appstatesync.State

	historySyncNotifications        chan *waE2E.HistorySyncNotification
	historySyncHandlerStarted       atomic.Bool
	ManualHistorySyncDownload       bool
	DisableManualHistorySyncReceipt bool

	preKeyState prekeys.State // lock de upload e horario do ultimo upload

	// mediaConn cacheia a media connection; o lock vive dentro dele.
	mediaConn media.ConnCache

	responseWaiters     map[string]chan<- *waBinary.Node
	responseWaitersLock sync.Mutex

	nodeHandlers      map[string]nodeHandler
	handlerQueue      chan *waBinary.Node
	eventHandlers     []wrappedEventHandler
	eventHandlersLock sync.RWMutex

	// retryState reune os antigos messageRetries/messageRetriesLock,
	// retrySema, incomingRetryRequestCounter (+lock), o buffer circular de
	// mensagens recentes (+lock), lastRetryStoreClear, sessionRecreateHistory
	// (+lock) e pendingPhoneRerequests (+lock). Os cinco locks de dentro
	// continuam sendo cinco, com os mesmos pontos de aquisicao. F36 (os dois
	// contadores sem despejo) viajou junto e segue em aberto.
	retryState retry.State

	messageSendLock sync.Mutex

	tcToken tctoken.State // cache de emissao de tctoken e os dois locks dele

	privacySettingsCache atomic.Value

	// groupCache reune os antigos groupCache/groupCacheLock. O lock de dentro
	// continua sendo um so', com os mesmos pontos de aquisicao — inclusive o
	// que atravessa a consulta ao servidor em group.GetOrFetch.
	groupCache group.Cache
	// userDevicesCache reune os antigos userDevicesCache/userDevicesCacheLock.
	// O lock de dentro continua sendo um so', com os mesmos pontos de
	// aquisicao — inclusive o que atravessa a consulta ao servidor em
	// user.GetDevices.
	userDevicesCache user.DeviceCache

	// GetMessageForRetry is used to find the source message for handling retry receipts
	// when the message is not found in the recently sent message cache.
	// Note: in DMs, the "to" field may be different from what you originally sent to (LID vs phone number),
	// make sure to check both if necessary.
	GetMessageForRetry func(requester, to types.JID, id types.MessageID) *waE2E.Message
	// PreRetryCallback is called before a retry receipt is accepted.
	// If it returns false, the accepting will be cancelled and the retry receipt will be ignored.
	PreRetryCallback func(receipt *events.Receipt, id types.MessageID, retryCount int, msg *waE2E.Message) bool
	// Should whatsmeow store recently sent messages in the database so that retry receipts can be accepted
	// even if the process is restarted? If false, only the in-memory cache and GetMessageForRetry will be used.
	UseRetryMessageStore bool

	// PrePairCallback is called before pairing is completed. If it returns false, the pairing will be cancelled and
	// the client will disconnect.
	PrePairCallback func(jid types.JID, platform, businessName string) bool

	// GetClientPayload is called to get the client payload for connecting to the server.
	// This should NOT be used for WhatsApp (to change the OS name, update fields in store.BaseClientPayload directly).
	GetClientPayload func() *waWa6.ClientPayload
	QRClientType     PairClientType

	// Should untrusted identity errors be handled automatically? If true, the stored identity and existing signal
	// sessions will be removed on untrusted identity errors, and an events.IdentityChange will be dispatched.
	// If false, decrypting a message from untrusted devices will fail.
	AutoTrustIdentity bool

	// Should SubscribePresence return an error if no privacy token is stored for the user?
	ErrorOnSubscribePresenceWithoutToken bool

	SendReportingTokens bool

	BackgroundEventCtx context.Context

	pairState pairing.State // sessao de pareamento por codigo pendente
	uniqueID  string
	idCounter atomic.Uint64

	serverTimeOffset atomic.Int64

	mediaHTTP     *http.Client
	websocketHTTP *http.Client
	preLoginHTTP  *http.Client

	// This field changes the client to act like a Messenger client instead of a WhatsApp one.
	//
	// Note that you cannot use a Messenger account just by setting this field, you must use a
	// separate library for all the non-e2ee-related stuff like logging in.
	// The library is currently embedded in mautrix-meta (https://github.com/mautrix/meta), but may be separated later.
	MessengerConfig *MessengerConfig
	RefreshCAT      func(context.Context) error
}

type MessengerConfig struct {
	UserAgent    string
	BaseURL      string
	WebsocketURL string
}

// NewClient initializes a new WhatsApp web client.
//
// The logger can be nil, it will default to a no-op logger.
//
// The device store must be set. A default SQL-backed implementation is available in the store/sqlstore package.
//
//	container, err := sqlstore.New("sqlite3", "file:yoursqlitefile.db?_foreign_keys=on", nil)
//	if err != nil {
//		panic(err)
//	}
//	// If you want multiple sessions, remember their JIDs and use .GetDevice(jid) or .GetAllDevices() instead.
//	deviceStore, err := container.GetFirstDevice()
//	if err != nil {
//		panic(err)
//	}
//	client := whatsmeow.NewClient(deviceStore, nil)
func NewClient(deviceStore *store.Device, log waLog.Logger) *Client {
	if log == nil {
		log = waLog.Noop
	}
	uniqueIDPrefix := random.Bytes(uniqueIDPrefixLength)
	baseHTTPClient := &http.Client{
		Transport: (http.DefaultTransport.(*http.Transport)).Clone(),
	}
	cli := &Client{
		mediaHTTP:          ptr.Clone(baseHTTPClient),
		websocketHTTP:      ptr.Clone(baseHTTPClient),
		preLoginHTTP:       ptr.Clone(baseHTTPClient),
		Store:              deviceStore,
		Log:                log,
		recvLog:            log.Sub("Recv"),
		sendLog:            log.Sub("Send"),
		uniqueID:           fmt.Sprintf("%d.%d-", uniqueIDPrefix[0], uniqueIDPrefix[1]),
		responseWaiters:    make(map[string]chan<- *waBinary.Node),
		eventHandlers:      make([]wrappedEventHandler, 0, initialEventHandlerCapacity),
		handlerQueue:       make(chan *waBinary.Node, handlerQueueSize),
		appStateProc:       appstate.NewProcessor(deviceStore, log.Sub("AppState")),
		socketWait:         make(chan struct{}),
		expectedDisconnect: exsync.NewEvent(),

		historySyncNotifications: make(chan *waE2E.HistorySyncNotification, historySyncNotificationBufferSize),

		GetMessageForRetry: func(requester, to types.JID, id types.MessageID) *waE2E.Message { return nil },

		EnableAutoReconnect: true,
		AutoTrustIdentity:   true,

		BackgroundEventCtx: context.Background(),
	}
	cli.nodeHandlers = map[string]nodeHandler{
		"message":      cli.handleEncryptedMessage,
		"appdata":      cli.handleEncryptedMessage,
		"receipt":      cli.handleReceipt,
		"call":         cli.handleCallEvent,
		"chatstate":    cli.handleChatState,
		"presence":     cli.handlePresence,
		"notification": cli.handleNotification,
		"success":      cli.handleConnectSuccess,
		"failure":      cli.handleConnectFailure,
		"stream:error": cli.handleStreamError,
		"iq":           cli.handleIQ,
		"ib":           cli.handleIB,
		// Apparently there's also an <error> node which can have a code=479 and means "Invalid stanza sent (smax-invalid)"
	}
	return cli
}

func (cli *Client) getOwnID() types.JID {
	if cli == nil {
		return types.EmptyJID
	}
	return cli.Store.GetJID()
}

func (cli *Client) getOwnLID() types.JID {
	if cli == nil {
		return types.EmptyJID
	}
	return cli.Store.GetLID()
}
