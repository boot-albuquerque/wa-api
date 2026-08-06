package contractsfake

import (
	"context"

	port "wa-api/pkg/application/contracts"
)

// --- CallRecorder ------------------------------------------------------

// CallRecorder registra a ordem em que métodos foram chamados. Um mesmo
// *CallRecorder pode ser compartilhado entre vários fakes (Session e
// SessionAttachHook, por exemplo), o que torna verificável a ordem exigida
// pelo contrato — Attach antes de Pair/Connect.
//
// Zero-value pronto para uso. Assume um único goroutine, como os demais
// fakes deste pacote.
type CallRecorder struct {
	Calls []string
}

// Record anota name na sequência de chamadas. Seguro com receiver nil, para
// que o campo Recorder dos fakes possa ficar vazio quando o teste não se
// importa com ordem.
func (r *CallRecorder) Record(name string) {
	if r == nil {
		return
	}
	r.Calls = append(r.Calls, name)
}

// --- SessionProvider ---------------------------------------------------

// SessionProviderNewSessionCall é uma chamada a NewSession.
type SessionProviderNewSessionCall struct {
	Ctx  context.Context
	Spec port.SessionSpec
}

// SessionProvider é o fake de port.SessionProvider. Zero-value devolve
// sempre a mesma *Session (criada sob demanda no primeiro NewSession), o que
// deixa o caminho feliz utilizável sem configuração.
type SessionProvider struct {
	// NewSessionFunc, quando não nil, decide o retorno. Caso contrário o
	// fake devolve NewSessionResult (criando-o se nil).
	NewSessionFunc  func(ctx context.Context, spec port.SessionSpec) (port.Session, error)
	NewSessionCalls []SessionProviderNewSessionCall

	// NewSessionResult é a sessão devolvida no caminho padrão. Preencha
	// para configurar o comportamento da sessão antes do primeiro uso.
	NewSessionResult *Session

	Recorder *CallRecorder
}

var _ port.SessionProvider = (*SessionProvider)(nil)

// NewSession implementa port.SessionProvider.
func (f *SessionProvider) NewSession(ctx context.Context, spec port.SessionSpec) (port.Session, error) {
	f.NewSessionCalls = append(f.NewSessionCalls, SessionProviderNewSessionCall{Ctx: ctx, Spec: spec})
	f.Recorder.Record("SessionProvider.NewSession")
	if f.NewSessionFunc != nil {
		return f.NewSessionFunc(ctx, spec)
	}
	if f.NewSessionResult == nil {
		f.NewSessionResult = &Session{Recorder: f.Recorder}
	}
	return f.NewSessionResult, nil
}

// --- Session -----------------------------------------------------------

// SessionPairCall é uma chamada a Pair.
type SessionPairCall struct {
	Ctx context.Context
}

// SessionConnectCall é uma chamada a Connect.
type SessionConnectCall struct {
	Ctx context.Context
}

// SessionLogoutCall é uma chamada a Logout.
type SessionLogoutCall struct {
	Ctx context.Context
}

// SessionSetProxyCall é uma chamada a SetProxy.
type SessionSetProxyCall struct {
	Cfg port.ProxyConfig
}

// Session é o fake de port.Session. Zero-value: sem credenciais, sem JID,
// desconectada, deslogada, e todos os métodos com erro nil.
//
// Para dirigir o fluxo de pareamento, preencha PairingEvents com um canal
// controlado pelo teste antes de chamar Pair — Pair devolve exatamente esse
// canal (criando um bufferizado se estiver nil).
type Session struct {
	HasCredentialsFunc  func() bool
	HasCredentialsCalls int

	JIDFunc  func() (string, bool)
	JIDCalls int

	// PairingEvents é o canal devolvido por Pair. O teste escreve nele e o
	// fecha para simular o fim do fluxo.
	PairingEvents chan port.PairingEvent
	PairFunc      func(ctx context.Context) (<-chan port.PairingEvent, error)
	PairCalls     []SessionPairCall

	ConnectFunc  func(ctx context.Context) error
	ConnectCalls []SessionConnectCall

	DisconnectFunc  func()
	DisconnectCalls int

	IsConnectedFunc  func() bool
	IsConnectedCalls int

	LogoutFunc  func(ctx context.Context) error
	LogoutCalls []SessionLogoutCall

	IsLoggedInFunc  func() bool
	IsLoggedInCalls int

	SetProxyFunc  func(cfg port.ProxyConfig) error
	SetProxyCalls []SessionSetProxyCall

	SubscribeFunc  func(fn func(port.SessionEvent)) (func(), error)
	SubscribeCalls int
	// Subscribers guarda os observadores registrados via Subscribe, para
	// que o teste possa emitir eventos com Emit.
	Subscribers []func(port.SessionEvent)

	Recorder *CallRecorder
}

var _ port.Session = (*Session)(nil)

// HasCredentials implementa port.Session.
func (f *Session) HasCredentials() bool {
	f.HasCredentialsCalls++
	f.Recorder.Record("Session.HasCredentials")
	if f.HasCredentialsFunc != nil {
		return f.HasCredentialsFunc()
	}
	return false
}

// JID implementa port.Session.
func (f *Session) JID() (string, bool) {
	f.JIDCalls++
	f.Recorder.Record("Session.JID")
	if f.JIDFunc != nil {
		return f.JIDFunc()
	}
	return "", false
}

// Pair implementa port.Session.
func (f *Session) Pair(ctx context.Context) (<-chan port.PairingEvent, error) {
	f.PairCalls = append(f.PairCalls, SessionPairCall{Ctx: ctx})
	f.Recorder.Record("Session.Pair")
	if f.PairFunc != nil {
		return f.PairFunc(ctx)
	}
	if f.PairingEvents == nil {
		f.PairingEvents = make(chan port.PairingEvent, 8)
	}
	return f.PairingEvents, nil
}

// Connect implementa port.Session.
func (f *Session) Connect(ctx context.Context) error {
	f.ConnectCalls = append(f.ConnectCalls, SessionConnectCall{Ctx: ctx})
	f.Recorder.Record("Session.Connect")
	if f.ConnectFunc != nil {
		return f.ConnectFunc(ctx)
	}
	return nil
}

// Disconnect implementa port.Session.
func (f *Session) Disconnect() {
	f.DisconnectCalls++
	f.Recorder.Record("Session.Disconnect")
	if f.DisconnectFunc != nil {
		f.DisconnectFunc()
	}
}

// IsConnected implementa port.Session.
func (f *Session) IsConnected() bool {
	f.IsConnectedCalls++
	f.Recorder.Record("Session.IsConnected")
	if f.IsConnectedFunc != nil {
		return f.IsConnectedFunc()
	}
	return false
}

// Logout implementa port.Session.
func (f *Session) Logout(ctx context.Context) error {
	f.LogoutCalls = append(f.LogoutCalls, SessionLogoutCall{Ctx: ctx})
	f.Recorder.Record("Session.Logout")
	if f.LogoutFunc != nil {
		return f.LogoutFunc(ctx)
	}
	return nil
}

// IsLoggedIn implementa port.Session.
func (f *Session) IsLoggedIn() bool {
	f.IsLoggedInCalls++
	f.Recorder.Record("Session.IsLoggedIn")
	if f.IsLoggedInFunc != nil {
		return f.IsLoggedInFunc()
	}
	return false
}

// SetProxy implementa port.Session.
func (f *Session) SetProxy(cfg port.ProxyConfig) error {
	f.SetProxyCalls = append(f.SetProxyCalls, SessionSetProxyCall{Cfg: cfg})
	f.Recorder.Record("Session.SetProxy")
	if f.SetProxyFunc != nil {
		return f.SetProxyFunc(cfg)
	}
	return nil
}

// Subscribe implementa port.Session.
func (f *Session) Subscribe(fn func(port.SessionEvent)) (func(), error) {
	f.SubscribeCalls++
	f.Recorder.Record("Session.Subscribe")
	if f.SubscribeFunc != nil {
		return f.SubscribeFunc(fn)
	}
	idx := len(f.Subscribers)
	f.Subscribers = append(f.Subscribers, fn)
	return func() { f.Subscribers[idx] = nil }, nil
}

// Emit entrega evt a todos os observadores ainda inscritos. Só faz sentido
// no caminho padrão de Subscribe (SubscribeFunc nil).
func (f *Session) Emit(evt port.SessionEvent) {
	for _, fn := range f.Subscribers {
		if fn != nil {
			fn(evt)
		}
	}
}

// --- SessionRegistry ---------------------------------------------------

// SessionRegistryRegisterCall é uma chamada a Register.
type SessionRegistryRegisterCall struct {
	UserID  string
	Session port.Session
}

// SessionRegistryGetCall é uma chamada a Get.
type SessionRegistryGetCall struct {
	UserID string
}

// SessionRegistryUnregisterCall é uma chamada a Unregister.
type SessionRegistryUnregisterCall struct {
	UserID string
}

// SessionRegistryProvisionWebhookClientCall é uma chamada a
// ProvisionWebhookClient.
type SessionRegistryProvisionWebhookClientCall struct {
	UserID   string
	ProxyURL string
}

// SessionRegistry é o fake de port.SessionRegistry. Zero-value guarda de
// fato as sessões registradas, de modo que Get devolve o que Register
// recebeu.
type SessionRegistry struct {
	Sessions map[string]port.Session

	RegisterFunc  func(userID string, session port.Session)
	RegisterCalls []SessionRegistryRegisterCall

	GetFunc  func(userID string) (port.Session, bool)
	GetCalls []SessionRegistryGetCall

	UnregisterFunc  func(userID string)
	UnregisterCalls []SessionRegistryUnregisterCall

	ProvisionWebhookClientFunc  func(userID, proxyURL string) error
	ProvisionWebhookClientCalls []SessionRegistryProvisionWebhookClientCall

	Recorder *CallRecorder
}

var _ port.SessionRegistry = (*SessionRegistry)(nil)

// Register implementa port.SessionRegistry.
func (f *SessionRegistry) Register(userID string, session port.Session) {
	f.RegisterCalls = append(f.RegisterCalls, SessionRegistryRegisterCall{UserID: userID, Session: session})
	f.Recorder.Record("SessionRegistry.Register")
	if f.RegisterFunc != nil {
		f.RegisterFunc(userID, session)
		return
	}
	if f.Sessions == nil {
		f.Sessions = make(map[string]port.Session)
	}
	f.Sessions[userID] = session
}

// Get implementa port.SessionRegistry.
func (f *SessionRegistry) Get(userID string) (port.Session, bool) {
	f.GetCalls = append(f.GetCalls, SessionRegistryGetCall{UserID: userID})
	f.Recorder.Record("SessionRegistry.Get")
	if f.GetFunc != nil {
		return f.GetFunc(userID)
	}
	s, ok := f.Sessions[userID]
	return s, ok
}

// Unregister implementa port.SessionRegistry.
func (f *SessionRegistry) Unregister(userID string) {
	f.UnregisterCalls = append(f.UnregisterCalls, SessionRegistryUnregisterCall{UserID: userID})
	f.Recorder.Record("SessionRegistry.Unregister")
	if f.UnregisterFunc != nil {
		f.UnregisterFunc(userID)
		return
	}
	delete(f.Sessions, userID)
}

// ProvisionWebhookClient implementa port.SessionRegistry.
func (f *SessionRegistry) ProvisionWebhookClient(userID string, proxyURL string) error {
	f.ProvisionWebhookClientCalls = append(f.ProvisionWebhookClientCalls,
		SessionRegistryProvisionWebhookClientCall{UserID: userID, ProxyURL: proxyURL})
	f.Recorder.Record("SessionRegistry.ProvisionWebhookClient")
	if f.ProvisionWebhookClientFunc != nil {
		return f.ProvisionWebhookClientFunc(userID, proxyURL)
	}
	return nil
}

// --- SessionEventDispatcher --------------------------------------------

// SessionEventDispatcherDispatchCall é uma chamada a Dispatch.
type SessionEventDispatcherDispatchCall struct {
	Ctx       context.Context
	UserID    string
	EventType string
	Payload   map[string]any
}

// SessionEventDispatcher é o fake de port.SessionEventDispatcher.
// Zero-value aceita tudo e só grava as chamadas — DispatchCalls é a
// superfície de inspeção dos eventos despachados.
type SessionEventDispatcher struct {
	DispatchFunc  func(ctx context.Context, userID, eventType string, payload map[string]any) error
	DispatchCalls []SessionEventDispatcherDispatchCall

	Recorder *CallRecorder
}

var _ port.SessionEventDispatcher = (*SessionEventDispatcher)(nil)

// Dispatch implementa port.SessionEventDispatcher.
func (f *SessionEventDispatcher) Dispatch(ctx context.Context, userID string, eventType string, payload map[string]any) error {
	f.DispatchCalls = append(f.DispatchCalls, SessionEventDispatcherDispatchCall{
		Ctx: ctx, UserID: userID, EventType: eventType, Payload: payload,
	})
	f.Recorder.Record("SessionEventDispatcher.Dispatch")
	if f.DispatchFunc != nil {
		return f.DispatchFunc(ctx, userID, eventType, payload)
	}
	return nil
}

// DispatchedTypes devolve os eventType recebidos, na ordem.
func (f *SessionEventDispatcher) DispatchedTypes() []string {
	types := make([]string, 0, len(f.DispatchCalls))
	for _, c := range f.DispatchCalls {
		types = append(types, c.EventType)
	}
	return types
}

// --- SessionAttachHook -------------------------------------------------

// SessionAttachHookAttachCall é uma chamada a Attach.
type SessionAttachHookAttachCall struct {
	Ctx    context.Context
	UserID string
	Token  string
}

// SessionAttachHookDetachCall é uma chamada a Detach.
type SessionAttachHookDetachCall struct {
	UserID string
}

// SessionAttachHook é o fake de port.SessionAttachHook. Compartilhe o
// Recorder com o fake de Session para verificar a ordem obrigatória
// Attach → Pair/Connect.
type SessionAttachHook struct {
	AttachFunc  func(ctx context.Context, userID, token string) error
	AttachCalls []SessionAttachHookAttachCall

	DetachFunc  func(userID string)
	DetachCalls []SessionAttachHookDetachCall

	Recorder *CallRecorder
}

var _ port.SessionAttachHook = (*SessionAttachHook)(nil)

// Attach implementa port.SessionAttachHook.
func (f *SessionAttachHook) Attach(ctx context.Context, userID, token string) error {
	f.AttachCalls = append(f.AttachCalls, SessionAttachHookAttachCall{Ctx: ctx, UserID: userID, Token: token})
	f.Recorder.Record("SessionAttachHook.Attach")
	if f.AttachFunc != nil {
		return f.AttachFunc(ctx, userID, token)
	}
	return nil
}

// Detach implementa port.SessionAttachHook.
func (f *SessionAttachHook) Detach(userID string) {
	f.DetachCalls = append(f.DetachCalls, SessionAttachHookDetachCall{UserID: userID})
	f.Recorder.Record("SessionAttachHook.Detach")
	if f.DetachFunc != nil {
		f.DetachFunc(userID)
	}
}
