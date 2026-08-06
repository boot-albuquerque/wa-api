// Package session contém a orquestração de ciclo de vida de sessão WhatsApp
// do wa-api, agnóstica de provider. Nada aqui conhece whatsmeow nem
// pkg/bootstrap: tudo que atravessa a fronteira passa pelos ports de
// pkg/application/contracts.
package session

import (
	"context"
	"database/sql"
	"encoding/base64"
	"net/url"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/skip2/go-qrcode"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain/apperr"
)

const (
	defaultMaxConnectionRetries = 3
	defaultConnectionRetryWait  = 5 * time.Second
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

	// defaultWebhookUseProxy é o valor global aplicado quando o usuário não
	// tem webhook_use_proxy definido (hoje appCtx.GlobalWebhookUseProxy).
	defaultWebhookUseProxy bool

	// ensureS3 replica o storage.GetS3Manager().EnsureClientFromDB(userID)
	// de startClient. Injetado como função para que pkg/application não
	// importe pkg/infra/storage. Opcional: nil vira no-op.
	ensureS3 func(userID string)

	maxConnectionRetries int
	connectionRetryWait  time.Duration
	sleep                func(time.Duration)
}

// Option ajusta parâmetros do Orchestrator.
type Option func(*Orchestrator)

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
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Start materializa e põe de pé a sessão de userID, na mesma sequência de
// startClient: configuração de proxy, cliente de webhook, S3, attach do
// handler de domínio e então pareamento (sem credenciais) ou conexão com
// retry (já pareado).
//
// Bloqueia enquanto o fluxo de pareamento estiver ativo (o canal de
// PairingEvent é consumido aqui, como startClient consome o qrChan hoje);
// no caminho já pareado retorna assim que a conexão sobe.
func (o *Orchestrator) Start(ctx context.Context, userID, token string) error {
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

	unsubscribe, serr := sess.Subscribe(func(evt port.SessionEvent) {
		o.handleSessionEvent(ctx, userID, evt)
	})
	if serr != nil {
		log.Error().Err(serr).Str("userid", userID).Msg("failed to subscribe to session events")
	} else {
		defer unsubscribe()
	}

	if !sess.HasCredentials() {
		return o.runPairing(ctx, sess, userID)
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
func (o *Orchestrator) runPairing(ctx context.Context, sess port.Session, userID string) error {
	events, err := sess.Pair(ctx)
	if err != nil {
		return err
	}

	for evt := range events {
		switch evt.Kind {
		case port.PairingEventKindQR:
			o.onPairingQR(ctx, userID, evt)
		case port.PairingEventKindTimeout:
			o.onPairingTimeout(ctx, userID)
		case port.PairingEventKindSuccess:
			o.onPairingSuccess(userID)
		default:
			log.Info().Str("event", string(evt.Kind)).Str("userid", userID).Msg("Login event")
		}
	}
	return nil
}

func (o *Orchestrator) onPairingQR(ctx context.Context, userID string, evt port.PairingEvent) {
	image, err := qrcode.Encode(evt.Code, qrcode.Medium, 256)
	if err != nil {
		log.Error().Err(err).Str("userid", userID).Msg("failed to encode QR code")
		return
	}
	base64qrcode := "data:image/png;base64," + base64.StdEncoding.EncodeToString(image)

	if _, err := o.db.Exec(`UPDATE users SET qrcode=$1 WHERE id=$2`, base64qrcode, userID); err != nil {
		log.Error().Err(err).Str("userid", userID).Msg("failed to store qrcode")
	}

	o.dispatch(ctx, userID, "QR", map[string]any{
		"event":        "code",
		"qrCodeBase64": base64qrcode,
		// Validade real deste código específico (whatsmeow emite 20s para os
		// 5 primeiros e 60s para o último), em RFC3339 para o wa-worker
		// repassar como está em vez de assumir uma janela fixa.
		"expiresAt": time.Now().Add(evt.Timeout).Format(time.RFC3339),
	})
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
	case port.SessionEventKindQR:
		return "QR", qrPayload(evt.QR)
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

func qrPayload(q *port.SessionQREvent) map[string]any {
	payload := map[string]any{"event": "qr"}
	if q != nil {
		payload["code"] = q.Code
	}
	return payload
}

func (o *Orchestrator) dispatch(ctx context.Context, userID, eventType string, payload map[string]any) {
	if err := o.dispatcher.Dispatch(ctx, userID, eventType, payload); err != nil {
		log.Error().Err(err).Str("userid", userID).Str("event", eventType).Msg("failed to dispatch session event")
	}
}
