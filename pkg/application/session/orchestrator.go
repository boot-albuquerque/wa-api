// Package session contém a orquestração de ciclo de vida de sessão WhatsApp
// do wa-api, agnóstica de provider. Nada aqui conhece wa-noise nem
// pkg/bootstrap: tudo que atravessa a fronteira passa pelos ports de
// pkg/application/contracts.
package session

import (
	"context"
	"database/sql"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain/apperr"
	"wa-api/pkg/qrimage"
)

const (
	defaultMaxConnectionRetries = 3
	defaultConnectionRetryWait  = 5 * time.Second

	// O tamanho e o prefixo do data URI do QR viviam aqui (qrCodeImageSize /
	// qrCodeDataURIPrefix) e a codificação estava inline em buildQRPayload.
	// Mudaram-se para pkg/qrimage em 2026-08-29 (F373) porque um SEGUNDO
	// engine — wa_headless — passou a responder a mesma rota e não passava
	// por aqui: ele devolvia a string CRUA onde o contrato promete imagem, e
	// nada no tipo detectava a diferença. Ver o doc de pkg/qrimage.

	// startInFlightTTL é o teto de tempo que uma entrada de startInFlight
	// pode segurar um userID antes de ser considerada ESTAGNADA e cedida a
	// um Start novo.
	//
	// Existe por causa da Regra 4 do CLAUDE.md — o conserto também é um
	// mecanismo e também precisa do seu pior caso examinado. A guarda troca
	// "duas sessões concorrentes para o mesmo utilizador" por "uma de cada
	// vez"; sem teto, um único Start que nunca retornasse (canal de
	// pareamento que o SDK não fecha) tornaria o utilizador PERMANENTEMENTE
	// incapaz de conectar, que é estritamente pior que o defeito original.
	// Com teto, o pior caso é uma janela, não um estado absorvente.
	//
	// O valor cobre o pior caso medido do fluxo de pareamento com folga:
	// qrCodeFirstBatchSize (6) códigos × qrCodeTimeout (20s) = 120s até o
	// timeout do SDK — medido em 2026-08-20, 6 códigos entre 03:24:40 e
	// 03:26:20 e QRTimeout em 03:26:40.
	startInFlightTTL = 3 * time.Minute
)

// userStore é a superfície mínima de banco que o orchestrator usa.
//
// Decisão deliberada desta fase: não existe port de repositório de usuário
// que cubra as três colunas envolvidas aqui (proxy_url, webhook_use_proxy,
// qrcode) — port.UserRepository é orientado a CRUD de usuário via
// domain.UserRecord/UserUpdate e não expõe nem qrcode nem as configurações
// de proxy. Criar essas operações no UserRepository seria alargar um port de
// outro contexto só para acomodar esta fase, cujo foco arquitetural é a
// fronteira de *sessão*, não a de persistência de usuário. Por isso o
// orchestrator recebe a superfície de banco direto (*sqlx.DB a satisfaz),
// declarada como interface estreita para manter o serviço testável sem
// banco real. Se/quando um port de persistência de sessão existir, esta
// interface é o ponto de troca.
type userStore interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

// startInFlight serializa Start POR utilizador.
//
// Motivo, medido em 2026-08-20 contra o servidor real: `GET /session/connect`
// numa sessão que JÁ estava a parear não era um no-op — abria um SEGUNDO
// fluxo de pareamento e órfãos o primeiro. Os dois emissores de QR ficavam
// vivos ao mesmo tempo, ambos a escrever `users.qrcode`, e os códigos
// chegavam intercalados ao painel:
//
//	1.080s HTTP connect(1)                        -> 200 connecting
//	1.838s WS MSG type=QR qrlen=1850
//	6.092s HTTP connect(2, durante pareamento)    -> 200 connecting
//	6.736s WS MSG type=QR qrlen=1846   <- fluxo 2
//	19.681s WS MSG type=QR qrlen=1874  <- fluxo 1
//	21.904s WS MSG type=QR qrlen=1850  <- fluxo 2
//	26.772s WS MSG type=QR qrlen=1838  <- fluxo 1
//
// Do lado do utilizador isso É "gerar novo QR não funciona": o painel pisca
// entre dois códigos de fluxos diferentes e só um deles é escaneável a cada
// instante.
//
// A Evolution API fecha exatamente esta porta em
// instance.controller.ts::connectToWhatsapp — `if (state == 'connecting')
// return instance.qrCode;`, isto é, devolve o QR que já existe em vez de
// criar outro socket. Divergimos na FORMA e não no fundo: o nosso
// `/session/connect` mantém o corpo `{"status":"connecting"}` (é contrato
// com clientes que não o painel) e quem lê o QR que já existe é o
// `GET /session/qr`, que já era a rota autoritativa. Registado no
// HOUSEKEEP.md (F192).
//
// # Inventário de detentores (Regra 1 do CLAUDE.md)
//
// O que passa a disputar uma chave, e o pior caso de cada um:
//
//   - fluxo de pareamento por QR: até 120s (6 códigos × 20s) até o SDK
//     fechar o canal por timeout;
//   - connectWithRetry (sessão já pareada): maxConnectionRetries tentativas
//     com espera linear attempt×connectionRetryWait, ~15s no padrão;
//   - recusa por posse: retorna de imediato, antes de materializar nada.
//
// A chave é POR UTILIZADOR e não é um slot de um pool partilhado: um
// utilizador preso não atrasa nenhum outro, e nada aqui ocupa recurso
// limitado global. É por isso que esta guarda não viola a invariante do
// projeto ("nada que espere por relógio ou por par morto pode ocupar slot
// limitado") mesmo esperando por relógio: o único recurso que ela ocupa é o
// direito de parear a si próprio, que é precisamente o que se quer
// serializar.
type startInFlight struct {
	mu    sync.Mutex
	since map[string]time.Time
}

func newStartInFlight() *startInFlight {
	return &startInFlight{since: make(map[string]time.Time)}
}

// acquire marca userID como em curso e devolve true. Devolve false quando já
// há um Start vivo para esse userID — a não ser que a entrada esteja mais
// velha que ttl, caso em que é considerada estagnada e CEDIDA ao chamador
// novo (ver startInFlightTTL).
//
// Não loga, e não é isenta por anotação: a decisão que ela implementa é
// registada pelo CHAMADOR, em Start, com userid e nível Warn. As duas contam
// para o denominador do gate de log — ver F192 no HOUSEKEEP.md, que traz os
// números medidos e a razão de não os mascarar.
func (f *startInFlight) acquire(userID string, now time.Time, ttl time.Duration) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if since, busy := f.since[userID]; busy && now.Sub(since) < ttl {
		return false
	}
	f.since[userID] = now
	return true
}

// release devolve a chave de userID. Ver acquire quanto ao gate de log.
func (f *startInFlight) release(userID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.since, userID)
}

// busy reporta se userID está com um Start em curso e não estagnado, SEM
// reivindicar a chave (ver acquire). Usado pelo pré-check síncrono do
// handler (F274): ao contrário da posse (ADR-0005 D2, reivindicação
// idempotente para o mesmo dono), esta chave não é reentrante — reivindicá-la
// duas vezes na mesma requisição faria a segunda chamada (dentro da goroutine
// de Start) encontrar-se a si própria como "ocupada" e falhar sempre. Por
// isso o pré-check apenas OLHA, e não marca.
func (f *startInFlight) busy(userID string, now time.Time, ttl time.Duration) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	since, busy := f.since[userID]
	return busy && now.Sub(since) < ttl
}

// Orchestrator conduz o ciclo de vida de uma sessão: resolve a configuração
// no banco, materializa a sessão pelo SessionProvider, registra os handles,
// anexa o handler de domínio e então pareia ou conecta.
//
// Não é um port: é o serviço concreto de aplicação que os ports servem.
//
// O que ele deliberadamente NÃO faz:
//   - não conhece kill-channel (propriedade de SessionAttachHook);
//   - não escreve users.connected em nenhum caminho — quem marca
//     connected=1 é o handler de domínio (evento Connected) e quem marca
//     connected=0 é a goroutine de kill dentro do SessionAttachHook. O
//     orchestrator só observa SessionEvent.
type Orchestrator struct {
	provider   port.SessionProvider
	registry   port.SessionRegistry
	dispatcher port.SessionEventDispatcher
	attach     port.SessionAttachHook
	db         userStore

	// inFlight serializa Start por utilizador. Ver startInFlight.
	inFlight *startInFlight

	// now é a fonte de tempo da guarda de startInFlight, substituível nos
	// testes para exercitar a expiração sem dormir.
	now func() time.Time

	// defaultWebhookUseProxy é o valor global aplicado quando o usuário não
	// tem webhook_use_proxy definido (hoje appCtx.GlobalWebhookUseProxy).
	defaultWebhookUseProxy bool

	// claimOwnership reivindica a posse da sessão antes de iniciá-la
	// (ADR-0005 D2). Injetado como função pelo mesmo motivo de ensureS3:
	// pkg/application não importa a implementação. Nil vira "sempre pode",
	// que é o modo `single`.
	claimOwnership func(userID string) bool

	// releaseOwnership devolve a posse quando Start NÃO completa. Sem ele, a
	// posse reivindicada antes de materializar fica presa para sempre: o
	// heartbeat renova indefinidamente uma sessão que nunca subiu, e nenhuma
	// outra réplica consegue assumir aquele usuário (F96, medido).
	releaseOwnership func(userID string)

	// ensureS3 replica o storage.GetS3Manager().EnsureClientFromDB(userID)
	// de startClient. Injetado como função para que pkg/application não
	// importe pkg/infra/storage. Opcional: nil vira no-op.
	ensureS3 func(userID string)

	maxConnectionRetries int
	connectionRetryWait  time.Duration
	sleep                func(time.Duration)
}

// Option ajusta parâmetros do Orchestrator.
// codeSessionOwnedByAnotherReplica identifica a recusa por posse (ADR-0005 D2).
//
// Constante, e não literal inline como o resto deste pacote faz hoje, porque a
// política de idioma/strings deste repositório pede isso — e porque código de
// erro é contrato com quem consome a API: ele aparece no corpo da resposta e
// clientes passam a depender dele.
//
// CategoryConflict (409): a requisição está correta e autorizada, só chegou na
// réplica errada. Era 400 até a F95 acrescentar a categoria — e 400 dizia ao
// cliente "corrija o payload", que é ativamente enganoso quando não há nada a
// corrigir no payload.
const codeSessionOwnedByAnotherReplica = "session_owned_by_another_replica"

// codeSessionStartAlreadyInFlight é devolvido quando já existe um Start vivo
// para o utilizador. Ver startInFlight para a medição que o motivou.
//
// CategoryConflict, pela mesma razão do código acima: o pedido está correto e
// autorizado, só chegou enquanto outro igual ainda corre. Hoje o
// ConnectHandler dispara Start em goroutine e só REGISTA o erro — o cliente
// continua a receber 200 {"status":"connecting"}, que é o contrato de
// /session/connect e não muda por causa desta guarda. O QR que já existe sai
// pelo GET /session/qr, exatamente como na Evolution API.
const codeSessionStartAlreadyInFlight = "session_start_already_in_flight"

const codeSessionAlreadyConnected = "session_already_connected"

// errSessionStartAlreadyInFlight builds the apperr Start returns when the
// in-flight guard refuses a second pairing flow, and the same value
// CheckStartAvailable's synchronous pre-check (F274) returns — a single
// point so the code and the message can never drift between the two paths.
func errSessionStartAlreadyInFlight() error {
	return apperr.New(
		codeSessionStartAlreadyInFlight,
		apperr.CategoryConflict,
		"a session start is already in flight for this user; read the current QR from GET /session/qr",
		false,
		nil,
	)
}

type Option func(*Orchestrator)

// WithOwnershipCheck instala a verificação de posse do ADR-0005 D2.
//
// Sem ela, o caminho de conexão em TEMPO DE EXECUÇÃO (o /session/connect que o
// painel e todo pareamento novo usam) inicia sessão sem reivindicar posse —
// medido em 2026-08-08, com uma sessão pareada pelo painel rodando SEM LEASE
// enquanto a pareada no arranque tinha o seu. Sob N réplicas isso é o
// desastre da F89: a réplica seguinte encontra o lease livre, toma, conecta a
// mesma sessão, e o WhatsApp mata uma das duas para sempre.
func WithOwnershipCheck(claim func(userID string) bool, release func(userID string)) Option {
	return func(o *Orchestrator) {
		o.claimOwnership = claim
		o.releaseOwnership = release
	}
}

// WithRetryPolicy sobrescreve o número de tentativas e a base do backoff
// linear de conexão.
func WithRetryPolicy(attempts int, baseWait time.Duration) Option {
	return func(o *Orchestrator) {
		o.maxConnectionRetries = attempts
		o.connectionRetryWait = baseWait
	}
}

// WithSleep substitui a espera entre tentativas (usado nos testes).
func WithSleep(sleep func(time.Duration)) Option {
	return func(o *Orchestrator) { o.sleep = sleep }
}

// WithClock substitui a fonte de tempo da guarda de startInFlight (usado nos
// testes para exercitar startInFlightTTL sem dormir três minutos).
func WithClock(now func() time.Time) Option {
	return func(o *Orchestrator) { o.now = now }
}

// WithS3Provisioner registra o provisionamento de cliente S3 por usuário.
func WithS3Provisioner(fn func(userID string)) Option {
	return func(o *Orchestrator) { o.ensureS3 = fn }
}

// WithDefaultWebhookUseProxy define o padrão global de uso de proxy na
// entrega de webhook.
func WithDefaultWebhookUseProxy(v bool) Option {
	return func(o *Orchestrator) { o.defaultWebhookUseProxy = v }
}

// NewOrchestrator monta o serviço com os quatro ports e o acesso a banco.
func NewOrchestrator(
	provider port.SessionProvider,
	registry port.SessionRegistry,
	dispatcher port.SessionEventDispatcher,
	attach port.SessionAttachHook,
	db userStore,
	opts ...Option,
) *Orchestrator {
	o := &Orchestrator{
		provider:             provider,
		registry:             registry,
		dispatcher:           dispatcher,
		attach:               attach,
		db:                   db,
		maxConnectionRetries: defaultMaxConnectionRetries,
		connectionRetryWait:  defaultConnectionRetryWait,
		sleep:                time.Sleep,
		inFlight:             newStartInFlight(),
		now:                  time.Now,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// CheckStartAvailable is the synchronous pre-check ConnectHandler runs
// BEFORE firing Start in a goroutine (F274, same shape as F108's ownership
// pre-check). Without it, the handler always responds 200
// {"status":"connecting"} and only learns Start refused — because a pairing
// flow for this user was already in flight — from a background log line the
// client never sees. It only PEEKS at the guard (see startInFlight.busy):
// mutating it here would make Start's own acquire, moments later inside the
// goroutine, find the key already held and always fail.
func (o *Orchestrator) CheckStartAvailable(userID string) error {
	if o.inFlight.busy(userID, o.now(), startInFlightTTL) {
		return errSessionStartAlreadyInFlight()
	}
	return nil
}

// ReleaseStart clears the in-flight mark for userID without waiting for
// startInFlightTTL. DisconnectUseCase calls it after tearing down the
// transport (F274): disconnecting a session whose pairing flow was still
// active left the guard busy for up to startInFlightTTL, so
// `GET /session/connect` right after `/session/disconnect` found the slot
// occupied by a flow that the disconnect itself just cut off.
func (o *Orchestrator) ReleaseStart(userID string) {
	o.inFlight.release(userID)
}

// Start materializa e põe de pé a sessão de userID, na mesma sequência de
// startClient: configuração de proxy, cliente de webhook, S3, attach do
// handler de domínio e então pareamento (sem credenciais) ou conexão com
// retry (já pareado).
//
// Bloqueia enquanto o fluxo de pareamento estiver ativo (o canal de
// PairingEvent é consumido aqui, como startClient consome o qrChan hoje);
// no caminho já pareado retorna assim que a conexão sobe.
func (o *Orchestrator) Start(ctx context.Context, userID, token string) (err error) {
	// Serialização por utilizador ANTES da posse, e não depois: reivindicar a
	// posse é o primeiro efeito observável de Start, e deixá-la fora da
	// guarda faria dois Starts concorrentes tocarem o lease do mesmo
	// utilizador antes de um deles desistir. A ORDEM é o ponto — inverter
	// estas duas passa em qualquer teste que só olhe para o resultado final.
	if !o.inFlight.acquire(userID, o.now(), startInFlightTTL) {
		log.Warn().Str("userid", userID).Msg("start already in flight for this user; not starting a second pairing flow")
		return errSessionStartAlreadyInFlight()
	}
	defer o.inFlight.release(userID)

	if existing, ok := o.registry.Get(userID); ok && existing.IsConnected() {
		log.Info().Str("userid", userID).Msg("session already connected; nothing to do")
		return nil
	}

	// Posse ANTES de materializar qualquer coisa: criar a sessão e só depois
	// descobrir que ela é de outra réplica deixaria cliente e registries
	// sujos, e o caminho de limpeza teria de desfazer o que nem devia ter
	// começado.
	if o.claimOwnership != nil && !o.claimOwnership(userID) {
		return apperr.New(
			codeSessionOwnedByAnotherReplica,
			apperr.CategoryConflict,
			"this session is owned by another replica; route the request to its owner",
			false,
			nil,
		)
	}

	// Reivindicou e vai falhar? Devolve. O `defer` fica AQUI, depois da
	// reivindicação bem-sucedida, e não no topo: instalado antes, ele
	// liberaria a posse de OUTRA réplica no caminho em que a nossa foi negada.
	//
	// E dispara SÓ com err != nil: no caminho de sucesso a posse tem de
	// sobreviver enquanto a sessão roda, que é o ponto do mecanismo inteiro.
	if o.claimOwnership != nil && o.releaseOwnership != nil {
		defer func() {
			if err != nil {
				o.releaseOwnership(userID)
			}
		}()
	}

	sess, err := o.provider.NewSession(ctx, port.SessionSpec{UserID: userID, Token: token})
	if err != nil {
		return err
	}

	o.registry.Register(userID, sess)
	o.provision(sess, userID)

	// Attach ANTES de Pair/Connect: o handler de domínio precisa estar
	// registrado antes que qualquer evento possa chegar.
	if aerr := o.attach.Attach(ctx, userID, token); aerr != nil {
		o.registry.Unregister(userID)
		return aerr
	}

	// pairingConfirmed is the REAL signal that the session authenticated —
	// not the "success" item of the pairing channel
	// (port.PairingEventKindSuccess).
	//
	// F153 (measured in production, real account): the pairing channel may
	// emit ONLY "timeout" even for a session that did authenticate. The
	// local expiry timer of the last QR code (an independent goroutine in
	// the vendored SDK) and the real PairSuccess arriving from the server
	// race for a single CAS on the SDK side; if the timer wins, the SDK
	// drops the "success" silently and delivers only "timeout" — even when
	// authentication completed seconds earlier. runPairing alone would never
	// see the "success" in that case.
	//
	// The signal that does NOT lie is the session-event bus: the
	// Connected/PairSuccess handler of this Subscribe runs whenever
	// authentication really completes (handleConnectSuccess only fires
	// Connected after isLoggedIn=true), and it runs BEFORE the QR channel's
	// own handler in the same dispatch, because it was registered first.
	// That is why this signal, not the one local to runPairing, guards the
	// timeout teardown.
	var pairingConfirmed atomic.Bool

	unsubscribe, serr := sess.Subscribe(func(evt port.SessionEvent) {
		if evt.Kind == port.SessionEventKindConnected || evt.Kind == port.SessionEventKindPairSuccess {
			pairingConfirmed.Store(true)
		}
		o.handleSessionEvent(ctx, userID, evt)
	})
	if serr != nil {
		log.Error().Err(serr).Str("userid", userID).Msg("failed to subscribe to session events")
	} else {
		defer unsubscribe()
	}

	if !sess.HasCredentials() {
		return o.runPairing(ctx, sess, userID, &pairingConfirmed)
	}
	return o.connectWithRetry(ctx, sess, userID)
}

// provision aplica ao redor da sessão o que startClient monta antes de
// conectar: proxy do transporte, cliente HTTP de webhook e cliente S3.
// Falhas aqui são degradações registradas, não abortam a subida da sessão —
// mesmo comportamento de hoje.
func (o *Orchestrator) provision(sess port.Session, userID string) {
	proxyURL, webhookUseProxy := o.resolveProxySettings(userID)
	if proxyURL != "" {
		if err := sess.SetProxy(proxyConfigFor(proxyURL)); err != nil {
			log.Warn().Err(err).Str("userid", userID).Msg("failed to configure session proxy, continuing without it")
		}
	}

	webhookProxy := ""
	if proxyURL != "" && webhookUseProxy {
		webhookProxy = proxyURL
	}
	if err := o.registry.ProvisionWebhookClient(userID, webhookProxy); err != nil {
		log.Error().Err(err).Str("userid", userID).Msg("failed to provision webhook client")
	}

	if o.ensureS3 != nil {
		o.ensureS3(userID)
	}
}

// resolveProxySettings lê proxy_url e webhook_use_proxy do usuário. Ausência
// de linha ou erro de leitura degrada para "sem proxy", como hoje.
func (o *Orchestrator) resolveProxySettings(userID string) (proxyURL string, webhookUseProxy bool) {
	webhookUseProxy = o.defaultWebhookUseProxy
	err := o.db.QueryRow(
		"SELECT proxy_url, COALESCE(webhook_use_proxy, true) FROM users WHERE id=$1",
		userID,
	).Scan(&proxyURL, &webhookUseProxy)
	if err != nil && err != sql.ErrNoRows {
		log.Error().Err(err).Str("user_id", userID).Msg("Failed to query proxy settings from database")
		return "", o.defaultWebhookUseProxy
	}
	if err != nil {
		return "", o.defaultWebhookUseProxy
	}
	return proxyURL, webhookUseProxy
}

// proxyConfigFor classifica a URL nos dois modos que o transporte distingue.
func proxyConfigFor(proxyURL string) port.ProxyConfig {
	mode := port.ProxyModeHTTP
	if parsed, err := url.Parse(proxyURL); err == nil {
		if scheme := strings.ToLower(parsed.Scheme); scheme == "socks5" || scheme == "socks5h" {
			mode = port.ProxyModeSOCKS5
		}
	}
	return port.ProxyConfig{Mode: mode, URL: proxyURL}
}

// runPairing consome o canal de pareamento, replicando o switch de
// startClient sobre os eventos "code"/"timeout"/"success".
//
// Invariant (F153, measured in production with a real account): once the
// session has really authenticated — signalled by `confirmed`, not by the
// "success" item of this channel — no later event from this channel may
// tear the session down or rewrite the QR; before that, "timeout" must tear
// everything down, as it always has (F98).
//
// Why the guard does not use this channel's own "success": the vendored SDK
// (internal/wa-noise/core/qrchan.go) delivers only ONE terminal item per
// channel — success XOR timeout, never both — through a single CAS. But
// that CAS is raced by two independent goroutines: the local expiry timer
// of the last QR code (emitQRs) and the real PairSuccess arriving from the
// server (handleEvent). If the timer wins, the SDK drops the "success"
// silently ("Got status ..., but channel is already closed") and this
// channel never sees anything but "timeout" — even though authentication
// completed seconds earlier outside this channel. `confirmed` is fed by the
// session-event bus (Connected/PairSuccess in Start), which does not go
// through that CAS and therefore cannot lose the event.
func (o *Orchestrator) runPairing(ctx context.Context, sess port.Session, userID string, confirmed *atomic.Bool) error {
	events, err := sess.Pair(ctx)
	if err != nil {
		return err
	}

	for evt := range events {
		switch evt.Kind {
		case port.PairingEventKindQR:
			if confirmed.Load() {
				log.Debug().Str("userid", userID).Msg("ignoring stale QR event after pairing was confirmed")
				continue
			}
			o.onPairingQR(ctx, userID, evt)
		case port.PairingEventKindTimeout:
			if confirmed.Load() {
				log.Warn().Str("userid", userID).Msg("ignoring stale QR-channel timeout: pairing already confirmed by session events (F153)")
				continue
			}
			o.onPairingTimeout(ctx, userID)
		case port.PairingEventKindSuccess:
			confirmed.Store(true)
			o.onPairingSuccess(userID)
		default:
			log.Info().Str("event", string(evt.Kind)).Str("userid", userID).Msg("Login event")
		}
	}
	return nil
}

func (o *Orchestrator) onPairingQR(ctx context.Context, userID string, evt port.PairingEvent) {
	// MESMO construtor do fluxo de Subscribe (F68). Os dois payloads nasceram
	// divergentes por serem montados em dois lugares, e nada comparava um com o
	// outro; agora divergir exige mudar uma função só.
	payload := buildQRPayload(evt.Code, evt.Timeout)

	// A coluna guarda a IMAGEM, que é o que `GET /session/qr` devolve. Só
	// grava se a codificação deu certo — e a ausência não impede o despacho,
	// ao contrário de antes: sem imagem o evento saía CANCELADO, e o cliente
	// ficava sem o código cru, que ele consegue renderizar sozinho.
	if imagem, ok := payload["qrCodeBase64"].(string); ok {
		if _, err := o.db.Exec(`UPDATE users SET qrcode=$1 WHERE id=$2`, imagem, userID); err != nil {
			log.Error().Err(err).Str("userid", userID).Msg("failed to store qrcode")
		}
	}

	o.dispatch(ctx, userID, "QR", payload)
}

// onPairingTimeout limpa o QR e desmonta a sessão. A remoção dos handles de
// cliente e a marcação de desconexão acontecem dentro do SessionAttachHook,
// acionado por Detach — o orchestrator não toca users.connected.
func (o *Orchestrator) onPairingTimeout(ctx context.Context, userID string) {
	o.dispatch(ctx, userID, "QRTimeout", map[string]any{"event": "timeout"})

	if _, err := o.db.Exec(`UPDATE users SET qrcode='' WHERE id=$1`, userID); err != nil {
		log.Error().Err(err).Str("userid", userID).Msg("failed to clear qrcode on timeout")
	}

	log.Warn().Str("userid", userID).Msg("QR timeout killing channel")
	o.registry.Unregister(userID)
	o.attach.Detach(userID)

	// Devolve a posse: o pareamento acabou em nada e não há mais sessão.
	//
	// O `defer` de Start NÃO cobre este caminho, e é essa a lição da F98
	// (medida em bancada). Start devolve a posse quando RETORNA erro; aqui o
	// pareamento por QR já respondeu 200 {"status":"connecting"} e falha
	// DEPOIS, de forma assíncrona, dentro da goroutine que consome os eventos.
	// runPairing então retorna nil, e o defer nunca dispara.
	//
	// DEPOIS do teardown local, não antes: liberar primeiro abriria uma janela
	// em que outra réplica assume o usuário enquanto este processo ainda
	// segura os handles do cliente.
	//
	// Sem isto, o heartbeat renovava indefinidamente o lease de uma sessão com
	// connected=0 e sem transporte, e sob N pods o usuário ficava preso para
	// sempre à réplica onde desistiu de ler o QR — que é o evento mais banal
	// do fluxo de pareamento.
	if o.releaseOwnership != nil {
		o.releaseOwnership(userID)
	}
}

// onPairingSuccess limpa o QR. users.connected=1 é escrito pelo handler de
// domínio ao receber Connected, não aqui (escritor único por caminho).
func (o *Orchestrator) onPairingSuccess(userID string) {
	log.Info().Str("userid", userID).Msg("QR pairing ok!")
	if _, err := o.db.Exec(`UPDATE users SET qrcode='' WHERE id=$1`, userID); err != nil {
		log.Error().Err(err).Str("userid", userID).Msg("failed to clear qrcode after pairing")
	}
}

// connectWithRetry replica o backoff linear de startClient: até
// maxConnectionRetries tentativas, esperando attempt*connectionRetryWait
// antes de cada retentativa.
func (o *Orchestrator) connectWithRetry(ctx context.Context, sess port.Session, userID string) error {
	var lastErr error

	for attempt := 0; attempt < o.maxConnectionRetries; attempt++ {
		if attempt > 0 {
			wait := time.Duration(attempt) * o.connectionRetryWait
			log.Warn().
				Int("attempt", attempt+1).
				Int("max_retries", o.maxConnectionRetries).
				Dur("wait_time", wait).
				Msg("Retrying connection after delay")
			o.sleep(wait)
		}

		err := sess.Connect(ctx)
		if err == nil {
			log.Info().Int("attempt", attempt+1).Str("userid", userID).Msg("Successfully connected to WhatsApp")
			return nil
		}

		lastErr = err
		log.Warn().Err(err).
			Int("attempt", attempt+1).
			Int("max_retries", o.maxConnectionRetries).
			Msg("Failed to connect to WhatsApp")
	}

	connErr := apperr.New("session_connect_failed", apperr.CategoryInternal, "failed to connect to whatsapp after retry attempts", false, lastErr)
	log.Error().Err(connErr).
		Str("userid", userID).
		Int("attempts", o.maxConnectionRetries).
		Msg("Failed to connect to WhatsApp after all retry attempts")

	if _, err := o.db.Exec(`UPDATE users SET qrcode='' WHERE id=$1`, userID); err != nil {
		log.Error().Err(err).Str("userid", userID).Msg("failed to clear qrcode after connection failure")
	}

	// Webhook antes do Detach: eventhandler.go documenta a ordem
	// "webhook antes do sinal de kill" como intencional; Detach sinaliza o
	// kill-channel e derruba os handles.
	o.dispatch(ctx, userID, "ConnectFailure", map[string]any{
		"event":    "ConnectFailure",
		"error":    connErr.Error(),
		"attempts": o.maxConnectionRetries,
		"reason":   "Failed to connect after retry attempts",
	})

	o.registry.Unregister(userID)
	o.attach.Detach(userID)

	return connErr
}

// Stop desmonta a sessão de userID: remove o handle do registry e sinaliza o
// Detach, que é quem derruba o transporte e marca a desconexão. Idempotente.
func (o *Orchestrator) Stop(userID string) {
	o.registry.Unregister(userID)
	o.attach.Detach(userID)
}

// handleSessionEvent traduz os eventos de status da sessão em despachos.
// Não há escrita em users.connected aqui por decisão de arquitetura: o
// orchestrator observa, o SessionAttachHook e o handler de domínio escrevem.
func (o *Orchestrator) handleSessionEvent(ctx context.Context, userID string, evt port.SessionEvent) {
	eventType, payload := translateStatusEvent(evt)
	if eventType == "" {
		log.Debug().Str("kind", string(evt.Kind)).Str("userid", userID).Msg("unhandled session event")
		return
	}
	o.dispatch(ctx, userID, eventType, payload)
}

// translateStatusEvent mapeia SessionEvent para o par (type, payload) que o
// SessionEventDispatcher entrega. eventType vazio significa evento sem
// despacho correspondente.
func translateStatusEvent(evt port.SessionEvent) (string, map[string]any) {
	switch evt.Kind {
	case port.SessionEventKindConnected:
		return "Connected", map[string]any{"event": "connected"}
	case port.SessionEventKindDisconnected:
		return "Disconnected", disconnectedPayload(evt.Disconnected)
	case port.SessionEventKindLoggedOut:
		return "LoggedOut", loggedOutPayload(evt.LoggedOut)
	case port.SessionEventKindPairSuccess:
		return "PairSuccess", pairSuccessPayload(evt.PairSuccess)
	// F194: o QR NÃO é despachado por aqui, e o vazio é deliberado.
	//
	// Havia DOIS escritores do mesmo evento, e o primeiro código chegava
	// duplicado ao cliente em todos os ciclos medidos — 6ms de intervalo,
	// payload idêntico. Os seguintes não duplicavam porque só existem no
	// canal de pareamento.
	//
	// O escritor que fica é o do canal de pareamento (onPairingQR), e não
	// este, porque é o COMPLETO: a cópia que saía daqui vinha sem Timeout, e
	// portanto sem `expiresAt` — um cliente que se guiasse por ela ficava sem
	// a validade e sem barra de progresso.
	//
	// É seguro calar este caminho: QR só existe durante o pareamento, e o
	// pareamento só corre quando a sessão NÃO tem credenciais
	// (orchestrator.go:411), que é exatamente quando runPairing está de pé
	// para o receber.
	case port.SessionEventKindQR:
		return "", nil
	case port.SessionEventKindStreamReplaced:
		return "StreamReplaced", map[string]any{"event": "stream_replaced"}
	default:
		return "", nil
	}
}

func disconnectedPayload(d *port.SessionDisconnectedEvent) map[string]any {
	payload := map[string]any{"event": "disconnected"}
	if d != nil {
		payload["reason"] = d.Reason
	}
	return payload
}

func loggedOutPayload(l *port.SessionLoggedOutEvent) map[string]any {
	payload := map[string]any{"event": "logged_out"}
	if l != nil {
		payload["reason"] = l.Reason
	}
	return payload
}

func pairSuccessPayload(p *port.SessionPairSuccessEvent) map[string]any {
	payload := map[string]any{"event": "pair_success"}
	if p != nil {
		payload["jid"] = p.JID
		payload["businessName"] = p.BusinessName
		payload["platform"] = p.Platform
	}
	return payload
}

// qrEventName é o valor do campo `event` dentro do payload de QR.
//
// Havia DOIS — "code" no fluxo de pareamento e "qr" no de Subscribe —, os dois
// despachados sob o mesmo `type: "QR"`. Um consumidor tinha de testar campos
// para descobrir qual schema chegou (F68).
//
// Unificado em "code" porque era o valor que acompanhava o payload RENDERIZÁVEL
// (com imagem e validade): é o que um cliente de interface provavelmente usa
// para decidir desenhar o QR, e portanto o mais arriscado de mudar.
//
// O campo é redundante com o `type`, que já diz "QR". Removê-lo é candidato a
// uma próxima versão de contrato, não a esta.
const qrEventName = "code"

// buildQRPayload monta o ÚNICO payload de QR do sistema.
//
// Sempre traz `code` — o texto cru que qualquer cliente consegue renderizar por
// conta própria. `qrCodeBase64` e `expiresAt` entram quando dá: o primeiro
// depende da codificação da imagem funcionar, o segundo de haver validade
// conhecida.
//
// Um construtor só, e não dois que "combinam": dois nasceram divergentes
// exatamente por serem dois, e nada comparava um com o outro.
func buildQRPayload(code string, validade time.Duration) map[string]any {
	payload := map[string]any{
		"event": qrEventName,
		"code":  code,
	}

	if imagem, err := qrimage.Encode(code); err == nil && imagem != "" {
		payload["qrCodeBase64"] = imagem
	} else if err != nil {
		// Degrada para só o `code` em vez de não despachar: o cliente ainda
		// consegue renderizar o QR sozinho, e ficar sem evento nenhum
		// impediria o pareamento.
		log.Error().Err(err).Msg("falha ao codificar a imagem do QR; o evento sai apenas com o codigo")
	}

	// Validade real daquele código específico, em RFC3339, para o cliente
	// repassar como está em vez de assumir uma janela fixa.
	//
	// São 20s para TODOS os códigos, incluindo o primeiro. A F69 corrigiu a
	// afirmação anterior ("20s no primeiro, 60s nos demais") para "60s no
	// primeiro", e depois disso a CONSTANTE mudou e este texto não acompanhou:
	// `qrCodeFirstTimeout = qrCodeTimeout` em
	// internal/wa-noise/core/pair_constants.go:23, com o comentário a dizer
	// que a igualdade é deliberada — um QR de pareamento é uma credencial, e
	// triplicar a janela de exposição do primeiro código não compra nada.
	//
	// Medido em 2026-08-20 (F195): os seis códigos chegaram de 20 em 20
	// segundos, o primeiro inclusive — 03:24:40, 03:25:00, 03:25:20, 03:25:40,
	// 03:26:00, 03:26:20.
	//
	// NENHUM código depende deste número, e é assim que tem de continuar:
	// `expiresAt` vem do Timeout real do evento. O comentário é para quem
	// programa o cliente, e é exatamente por isso que estar errado engana.
	if validade > 0 {
		payload["expiresAt"] = time.Now().Add(validade).Format(time.RFC3339)
	}

	return payload
}

func (o *Orchestrator) dispatch(ctx context.Context, userID, eventType string, payload map[string]any) {
	if err := o.dispatcher.Dispatch(ctx, userID, eventType, payload); err != nil {
		log.Error().Err(err).Str("userid", userID).Str("event", eventType).Msg("failed to dispatch session event")
	}
}
