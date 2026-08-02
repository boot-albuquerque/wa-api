package messaging

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// ---------------------------------------------------------------------------
// Captura de log
// ---------------------------------------------------------------------------

// logCapture troca o logger global do zerolog por um que escreve em memoria e
// baixa o nivel global para Trace, de modo que o teste veja TODOS os niveis —
// inclusive os Debug de "RabbitMQ nao configurado". Restaura no Cleanup.
//
// Nao ha t.Parallel em nenhum teste deste pacote: o estado do RabbitMQ e o
// logger global sao ambos globais de processo.
// A escrita e' serializada por mutex porque a goroutine de safeGo loga em
// paralelo com a leitura do teste — sem isso o -race acusa, com razao.
type logCapture struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *logCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Write(p)
}

func (c *logCapture) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

func captureLogs(t *testing.T) *logCapture {
	t.Helper()
	c := &logCapture{}
	prevLogger := log.Logger
	prevLevel := zerolog.GlobalLevel()
	log.Logger = zerolog.New(c).With().Timestamp().Logger()
	zerolog.SetGlobalLevel(zerolog.TraceLevel)
	t.Cleanup(func() {
		log.Logger = prevLogger
		zerolog.SetGlobalLevel(prevLevel)
	})
	return c
}

type logLine map[string]any

func (l logLine) str(k string) string {
	if v, ok := l[k].(string); ok {
		return v
	}
	return ""
}

func (c *logCapture) lines(t *testing.T) []logLine {
	t.Helper()
	var out []logLine
	for _, raw := range strings.Split(c.String(), "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		var l logLine
		if err := json.Unmarshal([]byte(raw), &l); err != nil {
			t.Fatalf("linha de log nao e' JSON valido (%v): %s", err, raw)
		}
		out = append(out, l)
	}
	return out
}

// requireLog exige um registro no nivel dado cuja mensagem contenha msgPart e
// que carregue todos os campos estruturados listados. E' a assercao que torna
// os logs deste pacote verificados, e nao apenas presentes.
func (c *logCapture) requireLog(t *testing.T, level, msgPart string, fields ...string) logLine {
	t.Helper()
	lines := c.lines(t)
	var levelMatches int
	for _, l := range lines {
		if l.str("level") != level || !strings.Contains(l.str("message"), msgPart) {
			continue
		}
		levelMatches++
		missing := ""
		for _, f := range fields {
			if _, ok := l[f]; !ok {
				missing = f
				break
			}
		}
		if missing == "" {
			return l
		}
		t.Fatalf("registro %q/%q nao carrega o campo estruturado %q: %v", level, msgPart, missing, l)
	}
	t.Fatalf("nenhum registro %q com mensagem contendo %q (%d linhas, %d no nivel): %s",
		level, msgPart, len(lines), levelMatches, c.String())
	return nil
}

func (c *logCapture) requireNoLog(t *testing.T, msgPart string) {
	t.Helper()
	for _, l := range c.lines(t) {
		if strings.Contains(l.str("message"), msgPart) {
			t.Fatalf("registro inesperado contendo %q: %v", msgPart, l)
		}
	}
}

// ---------------------------------------------------------------------------
// Fakes do seam AMQP
// ---------------------------------------------------------------------------

var errFake = errors.New("fake amqp failure")

type fakeChannel struct {
	mu         sync.Mutex
	declareErr error
	publishErr error
	declared   []string
	published  []publishedMsg
}

type publishedMsg struct {
	queue string
	body  []byte
}

func (f *fakeChannel) QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp091.Table) (amqp091.Queue, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.declared = append(f.declared, name)
	if f.declareErr != nil {
		return amqp091.Queue{}, f.declareErr
	}
	return amqp091.Queue{Name: name}, nil
}

func (f *fakeChannel) Publish(exchange, key string, mandatory, immediate bool, msg amqp091.Publishing) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.publishErr != nil {
		return f.publishErr
	}
	f.published = append(f.published, publishedMsg{queue: key, body: msg.Body})
	return nil
}

func (f *fakeChannel) snapshot() ([]string, []publishedMsg) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.declared...), append([]publishedMsg(nil), f.published...)
}

type fakeConn struct {
	ch        *fakeChannel
	chErr     error
	closeErr  error
	notify    chan *amqp091.Error
	mu        sync.Mutex
	closed    int
	notifySet chan struct{}
}

// newFakeConn devolve uma conexao cujo canal de NotifyClose ja' esta' fechado:
// a goroutine de monitoramento entra no range e sai de imediato, sem deixar
// goroutine viva depois do teste.
func newFakeConn(ch *fakeChannel) *fakeConn {
	closed := make(chan *amqp091.Error)
	close(closed)
	return &fakeConn{ch: ch, notify: closed, notifySet: make(chan struct{}, 1)}
}

func (f *fakeConn) openChannel() (amqpChannel, error) {
	if f.chErr != nil {
		return nil, f.chErr
	}
	return f.ch, nil
}

func (f *fakeConn) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed++
	return f.closeErr
}

func (f *fakeConn) closeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func (f *fakeConn) NotifyClose(receiver chan *amqp091.Error) chan *amqp091.Error {
	select {
	case f.notifySet <- struct{}{}:
	default:
	}
	return f.notify
}

// waitNotify espera a goroutine de monitoramento ter chamado NotifyClose. Sem
// isso o teste terminaria com uma goroutine ainda por entrar no range, e o
// Cleanup do logger global correria com ela.
func (f *fakeConn) waitNotify(t *testing.T) {
	t.Helper()
	select {
	case <-f.notifySet:
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine de monitoramento nao chamou NotifyClose")
	}
}

// ---------------------------------------------------------------------------
// Isolamento do estado global
// ---------------------------------------------------------------------------

// resetRabbit salva e restaura todo o estado global do pacote e encurta o
// backoff, para que um teste de "esgotou as tentativas" custe microssegundos e
// nao 30 segundos.
func resetRabbit(t *testing.T) {
	t.Helper()
	prevConn, prevChan, prevEnabled := RabbitConn, RabbitChannel, RabbitEnabled
	prevQueue := RabbitQueue
	prevDial := dialAMQP
	prevMax, prevInterval := MaxRetries, RetryInterval
	prevCache, prevErrQueue := userInfoCache, webhookErrorQueuePtr
	t.Cleanup(func() {
		setRabbitState(prevConn, prevChan, prevEnabled)
		RabbitQueue = prevQueue
		dialAMQP = prevDial
		MaxRetries, RetryInterval = prevMax, prevInterval
		userInfoCache, webhookErrorQueuePtr = prevCache, prevErrQueue
	})
	setRabbitState(nil, nil, false)
	RabbitQueue = ""
	MaxRetries = 2
	RetryInterval = time.Millisecond
}
