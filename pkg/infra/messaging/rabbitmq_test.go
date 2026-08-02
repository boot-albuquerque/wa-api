package messaging

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	mwpkg "wa-api/pkg/presentation/http/middleware"

	"github.com/patrickmn/go-cache"
	"github.com/rabbitmq/amqp091-go"
)

// ---------------------------------------------------------------------------
// safeGo e acessores protegidos por mutex
// ---------------------------------------------------------------------------

// TestSafeGo_RecoversPanic fixa o contrato do unico ponto de concorrencia do
// pacote: um panic dentro da goroutine de monitoramento NAO derruba o
// processo, e deixa rastro identificado pelo nome da goroutine.
func TestSafeGo_RecoversPanic(t *testing.T) {
	logs := captureLogs(t)

	var wg sync.WaitGroup
	wg.Add(1)
	safeGo("panicking-goroutine", func() {
		defer wg.Done()
		panic("boom")
	})
	wg.Wait()
	// O recover roda no defer, depois do wg.Done: espera ativa curta ate' o
	// registro aparecer.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(logs.String()) == 0 {
		time.Sleep(time.Millisecond)
	}

	rec := logs.requireLog(t, "error", "panic recovered in messaging goroutine", "goroutine", "panic")
	if rec.str("goroutine") != "panicking-goroutine" {
		t.Errorf("goroutine = %q, want %q", rec.str("goroutine"), "panicking-goroutine")
	}
}

func TestSafeGo_RunsFunctionWithoutPanic(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	ran := make(chan struct{})
	safeGo("ok-goroutine", func() {
		defer wg.Done()
		close(ran)
	})
	wg.Wait()
	select {
	case <-ran:
	default:
		t.Fatal("safeGo nao executou a funcao")
	}
}

// TestRabbitStateAccessors cobre o par set/get sob mutex, incluindo a
// transicao de desabilitacao usada pela deteccao de queda de conexao.
func TestRabbitStateAccessors(t *testing.T) {
	resetRabbit(t)

	if getRabbitEnabled() {
		t.Fatal("estado inicial deveria estar desabilitado")
	}
	if getRabbitChannel() != nil {
		t.Fatal("canal inicial deveria ser nil")
	}

	ch := &fakeChannel{}
	conn := newFakeConn(ch)
	setRabbitState(conn, ch, true)

	if !getRabbitEnabled() {
		t.Error("getRabbitEnabled() = false apos setRabbitState(..., true)")
	}
	if getRabbitChannel() != amqpChannel(ch) {
		t.Error("getRabbitChannel() nao devolveu o canal armazenado")
	}
	if RabbitConn != amqpConnection(conn) {
		t.Error("RabbitConn nao recebeu a conexao armazenada")
	}

	setRabbitDisabled()
	if getRabbitEnabled() {
		t.Error("getRabbitEnabled() = true apos setRabbitDisabled()")
	}
	if getRabbitChannel() != amqpChannel(ch) {
		t.Error("setRabbitDisabled() nao deveria limpar o canal")
	}
}

func TestSetupDependencies(t *testing.T) {
	resetRabbit(t)

	c := cache.New(time.Minute, time.Minute)
	queue := "errors-queue"
	SetupDependencies(c, &queue)

	if userInfoCache != c {
		t.Error("SetupDependencies nao guardou o cache de usuarios")
	}
	if webhookErrorQueuePtr == nil || *webhookErrorQueuePtr != queue {
		t.Error("SetupDependencies nao guardou o ponteiro da fila de erro")
	}
}

// ---------------------------------------------------------------------------
// InitRabbitMQ
// ---------------------------------------------------------------------------

func TestInitRabbitMQ_NoURLDisablesPublishing(t *testing.T) {
	resetRabbit(t)
	logs := captureLogs(t)
	t.Setenv("RABBITMQ_URL", "")
	t.Setenv("RABBITMQ_QUEUE", "")

	dialAMQP = func(string) (amqpConnection, error) {
		t.Fatal("dialAMQP nao deveria ser chamado sem RABBITMQ_URL")
		return nil, nil
	}

	InitRabbitMQ()

	if getRabbitEnabled() {
		t.Error("RabbitMQ deveria ficar desabilitado sem RABBITMQ_URL")
	}
	if RabbitQueue != "whatsapp_events" {
		t.Errorf("RabbitQueue = %q, want a fila padrao %q", RabbitQueue, "whatsapp_events")
	}
	logs.requireLog(t, "info", "RABBITMQ_URL is not set", "queue")
}

func TestInitRabbitMQ_HonorsQueueFromEnv(t *testing.T) {
	resetRabbit(t)
	captureLogs(t)
	t.Setenv("RABBITMQ_URL", "")
	t.Setenv("RABBITMQ_QUEUE", "custom_queue")

	InitRabbitMQ()

	if RabbitQueue != "custom_queue" {
		t.Errorf("RabbitQueue = %q, want %q", RabbitQueue, "custom_queue")
	}
}

// TestInitRabbitMQ_DialFailsAllRetries cobre o backoff completo: a primeira
// falha registra Warn e reagenda, a ultima registra Error e desabilita.
func TestInitRabbitMQ_DialFailsAllRetries(t *testing.T) {
	resetRabbit(t)
	logs := captureLogs(t)
	t.Setenv("RABBITMQ_URL", "amqp://fake")
	t.Setenv("RABBITMQ_QUEUE", "q1")

	var attempts int
	dialAMQP = func(url string) (amqpConnection, error) {
		attempts++
		if url != "amqp://fake" {
			t.Errorf("dialAMQP recebeu url %q", url)
		}
		return nil, errFake
	}

	InitRabbitMQ()

	if attempts != MaxRetries {
		t.Errorf("tentativas = %d, want %d", attempts, MaxRetries)
	}
	if getRabbitEnabled() {
		t.Error("RabbitMQ deveria ficar desabilitado apos esgotar as tentativas")
	}
	logs.requireLog(t, "warn", "Failed to connect to RabbitMQ", "error", "attempt", "max_retries", "queue")
	logs.requireLog(t, "info", "Retrying RabbitMQ connection", "retry_in")
	logs.requireLog(t, "error", "Could not connect to RabbitMQ after all retries", "error", "queue")
}

// TestInitRabbitMQ_ChannelFailsAllRetries cobre o caminho em que a conexao
// abre mas o canal nao: a conexao TEM de ser fechada em toda tentativa, senao
// o processo vaza sockets a cada retry.
func TestInitRabbitMQ_ChannelFailsAllRetries(t *testing.T) {
	resetRabbit(t)
	logs := captureLogs(t)
	t.Setenv("RABBITMQ_URL", "amqp://fake")
	t.Setenv("RABBITMQ_QUEUE", "q2")

	conn := newFakeConn(nil)
	conn.chErr = errFake
	conn.closeErr = errors.New("close falhou")
	dialAMQP = func(string) (amqpConnection, error) { return conn, nil }

	InitRabbitMQ()

	if got := conn.closeCount(); got != MaxRetries {
		t.Errorf("Close() chamado %d vezes, want %d (uma por tentativa)", got, MaxRetries)
	}
	if getRabbitEnabled() {
		t.Error("RabbitMQ deveria ficar desabilitado apos esgotar as tentativas de canal")
	}
	logs.requireLog(t, "warn", "Failed to close RabbitMQ connection", "error", "queue")
	logs.requireLog(t, "warn", "Failed to open RabbitMQ channel", "error", "attempt", "queue")
	logs.requireLog(t, "error", "Could not open RabbitMQ channel after all retries", "error", "queue")
}

// TestInitRabbitMQ_SucceedsOnSecondAttempt cobre o caminho feliz depois de uma
// falha, incluindo o start da goroutine de monitoramento.
func TestInitRabbitMQ_SucceedsOnSecondAttempt(t *testing.T) {
	resetRabbit(t)
	logs := captureLogs(t)
	t.Setenv("RABBITMQ_URL", "amqp://fake")
	t.Setenv("RABBITMQ_QUEUE", "q3")

	ch := &fakeChannel{}
	conn := newFakeConn(ch)
	var attempts int
	dialAMQP = func(string) (amqpConnection, error) {
		attempts++
		if attempts == 1 {
			return nil, errFake
		}
		return conn, nil
	}

	InitRabbitMQ()
	conn.waitNotify(t)

	if !getRabbitEnabled() {
		t.Error("RabbitMQ deveria ficar habilitado apos conexao bem sucedida")
	}
	if getRabbitChannel() != amqpChannel(ch) {
		t.Error("o canal aberto nao foi publicado no estado global")
	}
	logs.requireLog(t, "info", "RabbitMQ connection established successfully", "queue", "attempt")
}

// ---------------------------------------------------------------------------
// HandleConnectionErrors
// ---------------------------------------------------------------------------

// notifyConn devolve uma conexao cujo canal de NotifyClose entrega os erros
// dados e depois fecha — e' o que faz o loop de monitoramento terminar.
func notifyConn(t *testing.T, ch *fakeChannel, errs ...*amqp091.Error) *fakeConn {
	t.Helper()
	c := newFakeConn(ch)
	notify := make(chan *amqp091.Error, len(errs))
	for _, e := range errs {
		notify <- e
	}
	close(notify)
	c.notify = notify
	return c
}

func TestHandleConnectionErrors_ReturnsWhenChannelCloses(t *testing.T) {
	resetRabbit(t)
	logs := captureLogs(t)

	HandleConnectionErrors(notifyConn(t, nil))

	logs.requireNoLog(t, "RabbitMQ connection closed unexpectedly")
}

// TestHandleConnectionErrors_ReconnectFailsAllRetries cobre o pior caso: a
// conexao caiu e nenhuma tentativa de reconexao pega.
func TestHandleConnectionErrors_ReconnectFailsAllRetries(t *testing.T) {
	resetRabbit(t)
	logs := captureLogs(t)
	RabbitQueue = "q4"
	t.Setenv("RABBITMQ_URL", "amqp://fake")

	var attempts int
	dialAMQP = func(string) (amqpConnection, error) {
		attempts++
		return nil, errFake
	}

	HandleConnectionErrors(notifyConn(t, nil, &amqp091.Error{Code: 320, Reason: "CONNECTION_FORCED"}))

	if attempts != MaxRetries {
		t.Errorf("tentativas de reconexao = %d, want %d", attempts, MaxRetries)
	}
	if getRabbitEnabled() {
		t.Error("a queda de conexao deveria ter desabilitado o publish")
	}
	logs.requireLog(t, "error", "RabbitMQ connection closed unexpectedly", "error", "queue")
	logs.requireLog(t, "info", "Reconnecting to RabbitMQ", "attempt")
	logs.requireLog(t, "warn", "Reconnection failed", "error", "attempt", "queue")
	logs.requireLog(t, "error", "Failed to reconnect to RabbitMQ after all retries", "queue", "max_retries")
}

// TestHandleConnectionErrors_ChannelFailureThenSuccess cobre as duas metades
// que faltam: falha ao abrir canal na reconexao (com Close da conexao nova) e,
// na tentativa seguinte, reconexao bem sucedida com remonitoramento.
func TestHandleConnectionErrors_ChannelFailureThenSuccess(t *testing.T) {
	resetRabbit(t)
	logs := captureLogs(t)
	RabbitQueue = "q5"
	t.Setenv("RABBITMQ_URL", "amqp://fake")
	MaxRetries = 3

	badConn := newFakeConn(nil)
	badConn.chErr = errFake
	badConn.closeErr = errors.New("close falhou")

	goodChan := &fakeChannel{}
	goodConn := newFakeConn(goodChan)

	var attempts int
	dialAMQP = func(string) (amqpConnection, error) {
		attempts++
		if attempts == 1 {
			return badConn, nil
		}
		return goodConn, nil
	}

	HandleConnectionErrors(notifyConn(t, nil, &amqp091.Error{Code: 501, Reason: "FRAME"}))
	goodConn.waitNotify(t)

	if got := badConn.closeCount(); got != 1 {
		t.Errorf("Close() na conexao com canal ruim = %d, want 1", got)
	}
	if !getRabbitEnabled() {
		t.Error("a reconexao bem sucedida deveria reabilitar o publish")
	}
	if getRabbitChannel() != amqpChannel(goodChan) {
		t.Error("o canal da reconexao nao foi publicado no estado global")
	}
	logs.requireLog(t, "warn", "Failed to open channel on reconnection", "error", "attempt", "queue")
	logs.requireLog(t, "warn", "Failed to close RabbitMQ connection", "error", "queue")
	logs.requireLog(t, "info", "RabbitMQ reconnected successfully", "queue", "attempt")
}

// ---------------------------------------------------------------------------
// PublishToRabbit
// ---------------------------------------------------------------------------

func TestPublishToRabbit_NoopWhenDisabled(t *testing.T) {
	resetRabbit(t)
	captureLogs(t)

	if err := PublishToRabbit([]byte(`{}`)); err != nil {
		t.Errorf("PublishToRabbit com RabbitMQ desabilitado = %v, want nil", err)
	}
}

func TestPublishToRabbit_NoopWhenChannelIsNil(t *testing.T) {
	resetRabbit(t)
	captureLogs(t)
	setRabbitState(nil, nil, true)

	if err := PublishToRabbit([]byte(`{}`)); err != nil {
		t.Errorf("PublishToRabbit sem canal = %v, want nil", err)
	}
}

func TestPublishToRabbit_PublishesToDefaultQueue(t *testing.T) {
	resetRabbit(t)
	logs := captureLogs(t)
	ch := &fakeChannel{}
	RabbitQueue = "default_queue"
	setRabbitState(nil, ch, true)

	if err := PublishToRabbit([]byte(`{"a":1}`)); err != nil {
		t.Fatalf("PublishToRabbit() = %v, want nil", err)
	}

	declared, published := ch.snapshot()
	if len(declared) != 1 || declared[0] != "default_queue" {
		t.Errorf("filas declaradas = %v, want [default_queue]", declared)
	}
	if len(published) != 1 || published[0].queue != "default_queue" || string(published[0].body) != `{"a":1}` {
		t.Errorf("mensagens publicadas = %+v", published)
	}
	logs.requireLog(t, "debug", "Published message to RabbitMQ", "queue")
}

func TestPublishToRabbit_QueueOverride(t *testing.T) {
	resetRabbit(t)
	captureLogs(t)
	ch := &fakeChannel{}
	RabbitQueue = "default_queue"
	setRabbitState(nil, ch, true)

	// Override vazio cai de volta na fila padrao; override preenchido vence.
	if err := PublishToRabbit([]byte(`{}`), ""); err != nil {
		t.Fatalf("PublishToRabbit(override vazio) = %v", err)
	}
	if err := PublishToRabbit([]byte(`{}`), "other_queue"); err != nil {
		t.Fatalf("PublishToRabbit(override) = %v", err)
	}

	declared, _ := ch.snapshot()
	if len(declared) != 2 || declared[0] != "default_queue" || declared[1] != "other_queue" {
		t.Errorf("filas declaradas = %v, want [default_queue other_queue]", declared)
	}
}

func TestPublishToRabbit_QueueDeclareError(t *testing.T) {
	resetRabbit(t)
	logs := captureLogs(t)
	ch := &fakeChannel{declareErr: errFake}
	RabbitQueue = "broken_queue"
	setRabbitState(nil, ch, true)

	if err := PublishToRabbit([]byte(`{}`)); !errors.Is(err, errFake) {
		t.Errorf("PublishToRabbit() = %v, want %v", err, errFake)
	}

	_, published := ch.snapshot()
	if len(published) != 0 {
		t.Errorf("nada deveria ser publicado apos falha de declare: %+v", published)
	}
	logs.requireLog(t, "error", "Could not declare RabbitMQ queue", "error", "queue")
}

func TestPublishToRabbit_PublishError(t *testing.T) {
	resetRabbit(t)
	logs := captureLogs(t)
	ch := &fakeChannel{publishErr: errFake}
	RabbitQueue = "broken_queue"
	setRabbitState(nil, ch, true)

	if err := PublishToRabbit([]byte(`{}`)); !errors.Is(err, errFake) {
		t.Errorf("PublishToRabbit() = %v, want %v", err, errFake)
	}
	logs.requireLog(t, "error", "Could not publish to RabbitMQ", "error", "queue")
}

// ---------------------------------------------------------------------------
// SendToGlobalRabbit
// ---------------------------------------------------------------------------

func TestSendToGlobalRabbit_DisabledButConfiguredLogsError(t *testing.T) {
	resetRabbit(t)
	logs := captureLogs(t)
	t.Setenv("RABBITMQ_URL", "amqp://fake")
	t.Setenv("RABBITMQ_QUEUE", "")

	SendToGlobalRabbit([]byte(`{}`), "tok", "user-1")

	rec := logs.requireLog(t, "error", "RabbitMQ is configured but disabled",
		"rabbitmq_url_set", "rabbitmq_queue_set")
	if rec.str("rabbitmq_url_set") != "yes" || rec.str("rabbitmq_queue_set") != "no" {
		t.Errorf("flags de configuracao = %v", rec)
	}
}

func TestSendToGlobalRabbit_DisabledWithQueueOnly(t *testing.T) {
	resetRabbit(t)
	logs := captureLogs(t)
	t.Setenv("RABBITMQ_URL", "")
	t.Setenv("RABBITMQ_QUEUE", "q")

	SendToGlobalRabbit([]byte(`{}`), "tok", "user-1")

	rec := logs.requireLog(t, "error", "RabbitMQ is configured but disabled",
		"rabbitmq_url_set", "rabbitmq_queue_set")
	if rec.str("rabbitmq_url_set") != "no" || rec.str("rabbitmq_queue_set") != "yes" {
		t.Errorf("flags de configuracao = %v", rec)
	}
}

func TestSendToGlobalRabbit_NotConfigured(t *testing.T) {
	resetRabbit(t)
	logs := captureLogs(t)
	t.Setenv("RABBITMQ_URL", "")
	t.Setenv("RABBITMQ_QUEUE", "")

	SendToGlobalRabbit([]byte(`{}`), "tok", "user-1")

	logs.requireLog(t, "debug", "RabbitMQ not configured", "queue")
}

func TestSendToGlobalRabbit_InvalidJSON(t *testing.T) {
	resetRabbit(t)
	logs := captureLogs(t)
	setRabbitState(nil, &fakeChannel{}, true)

	SendToGlobalRabbit([]byte(`nao e json`), "tok", "user-1")

	logs.requireLog(t, "error", "Failed to unmarshal original JSON data",
		"error", "queue", "user_id", "payload_bytes")
}

// TestSendToGlobalRabbit_EnrichesPayload prova o contrato de dados: userID e
// instanceName entram no evento, o segundo vindo do cache de instancia.
func TestSendToGlobalRabbit_EnrichesPayload(t *testing.T) {
	resetRabbit(t)
	captureLogs(t)
	ch := &fakeChannel{}
	RabbitQueue = "events"
	setRabbitState(nil, ch, true)

	c := cache.New(time.Minute, time.Minute)
	c.Set("tok", mwpkg.Values{M: map[string]string{"Name": "instancia-a"}}, cache.DefaultExpiration)
	SetupDependencies(c, nil)

	SendToGlobalRabbit([]byte(`{"event":"x"}`), "tok", "user-1")

	_, published := ch.snapshot()
	if len(published) != 1 {
		t.Fatalf("mensagens publicadas = %d, want 1", len(published))
	}
	var got map[string]any
	if err := json.Unmarshal(published[0].body, &got); err != nil {
		t.Fatalf("payload publicado nao e' JSON: %v", err)
	}
	if got["userID"] != "user-1" || got["instanceName"] != "instancia-a" || got["event"] != "x" {
		t.Errorf("payload enriquecido = %v", got)
	}
}

// TestSendToGlobalRabbit_CacheMissAndWrongType cobre os dois desvios do lookup
// de instancia: token ausente e valor de tipo inesperado no cache. Em ambos o
// evento sai com instanceName vazio, e nao com panic.
func TestSendToGlobalRabbit_CacheMissAndWrongType(t *testing.T) {
	resetRabbit(t)
	captureLogs(t)
	ch := &fakeChannel{}
	RabbitQueue = "events"
	setRabbitState(nil, ch, true)

	c := cache.New(time.Minute, time.Minute)
	c.Set("tipo-errado", 42, cache.DefaultExpiration)
	SetupDependencies(c, nil)

	SendToGlobalRabbit([]byte(`{"event":"a"}`), "ausente", "user-1")
	SendToGlobalRabbit([]byte(`{"event":"b"}`), "tipo-errado", "user-1")

	_, published := ch.snapshot()
	if len(published) != 2 {
		t.Fatalf("mensagens publicadas = %d, want 2", len(published))
	}
	for i, m := range published {
		var got map[string]any
		if err := json.Unmarshal(m.body, &got); err != nil {
			t.Fatalf("payload %d nao e' JSON: %v", i, err)
		}
		if got["instanceName"] != "" {
			t.Errorf("payload %d: instanceName = %v, want vazio", i, got["instanceName"])
		}
	}
}

// TestSendToGlobalRabbit_NilCache garante que o pacote nao explode quando
// SetupDependencies nunca foi chamado.
func TestSendToGlobalRabbit_NilCache(t *testing.T) {
	resetRabbit(t)
	captureLogs(t)
	ch := &fakeChannel{}
	RabbitQueue = "events"
	setRabbitState(nil, ch, true)
	userInfoCache = nil

	SendToGlobalRabbit([]byte(`{"event":"x"}`), "tok", "user-1")

	_, published := ch.snapshot()
	if len(published) != 1 {
		t.Fatalf("mensagens publicadas = %d, want 1", len(published))
	}
}

func TestSendToGlobalRabbit_PublishError(t *testing.T) {
	resetRabbit(t)
	logs := captureLogs(t)
	RabbitQueue = "events"
	setRabbitState(nil, &fakeChannel{publishErr: errFake}, true)
	userInfoCache = nil

	SendToGlobalRabbit([]byte(`{"event":"x"}`), "tok", "user-1", "override")

	logs.requireLog(t, "error", "Failed to publish to RabbitMQ", "error", "queue", "user_id")
}

// ---------------------------------------------------------------------------
// PublishFileErrorToQueue / PublishDataErrorToQueue
// ---------------------------------------------------------------------------

func TestPublishFileErrorToQueue_QueueNotConfigured(t *testing.T) {
	resetRabbit(t)
	logs := captureLogs(t)
	webhookErrorQueuePtr = nil

	PublishFileErrorToQueue(WebhookFileErrorPayload{UserID: "user-1", FilePath: "/tmp/a.jpg"})

	logs.requireLog(t, "error", "Webhook error queue not configured", "user_id", "file_path")
}

func TestPublishFileErrorToQueue_MarshalError(t *testing.T) {
	resetRabbit(t)
	logs := captureLogs(t)
	queue := "errors"
	webhookErrorQueuePtr = &queue

	// Um canal nao e' serializavel em JSON: e' o unico jeito honesto de
	// alcancar o caminho de erro de Marshal.
	PublishFileErrorToQueue(WebhookFileErrorPayload{
		UserID:   "user-1",
		FilePath: "/tmp/a.jpg",
		Payload:  map[string]interface{}{"ch": make(chan int)},
	})

	logs.requireLog(t, "error", "Failed to marshal file error payload",
		"error", "queue", "user_id", "file_path")
}

func TestPublishFileErrorToQueue_PublishErrorAndSuccess(t *testing.T) {
	resetRabbit(t)
	logs := captureLogs(t)
	queue := "errors"
	webhookErrorQueuePtr = &queue
	ch := &fakeChannel{publishErr: errFake}
	setRabbitState(nil, ch, true)

	PublishFileErrorToQueue(WebhookFileErrorPayload{UserID: "user-1", FilePath: "/tmp/a.jpg"})
	logs.requireLog(t, "error", "Failed to publish file error payload",
		"error", "queue", "user_id", "file_path")

	ch.publishErr = nil
	PublishFileErrorToQueue(WebhookFileErrorPayload{UserID: "user-1", FilePath: "/tmp/a.jpg"})
	logs.requireLog(t, "info", "File error payload successfully published", "queue")

	_, published := ch.snapshot()
	if len(published) != 1 || published[0].queue != "errors" {
		t.Errorf("mensagens publicadas = %+v", published)
	}
}

func TestPublishDataErrorToQueue_QueueNotConfigured(t *testing.T) {
	resetRabbit(t)
	logs := captureLogs(t)
	webhookErrorQueuePtr = nil

	PublishDataErrorToQueue(WebhookErrorPayload{UserID: "user-1", URL: "https://hook.example/x"})

	logs.requireLog(t, "error", "Webhook error queue not configured", "user_id", "webhook_url")
}

func TestPublishDataErrorToQueue_MarshalError(t *testing.T) {
	resetRabbit(t)
	logs := captureLogs(t)
	queue := "errors"
	webhookErrorQueuePtr = &queue

	PublishDataErrorToQueue(WebhookErrorPayload{
		UserID:  "user-1",
		URL:     "https://hook.example/x",
		Payload: map[string]interface{}{"ch": make(chan int)},
	})

	logs.requireLog(t, "error", "Failed to marshal data error payload",
		"error", "queue", "user_id", "webhook_url")
}

func TestPublishDataErrorToQueue_PublishErrorAndSuccess(t *testing.T) {
	resetRabbit(t)
	logs := captureLogs(t)
	queue := "errors"
	webhookErrorQueuePtr = &queue
	ch := &fakeChannel{publishErr: errFake}
	setRabbitState(nil, ch, true)

	PublishDataErrorToQueue(WebhookErrorPayload{UserID: "user-1", URL: "https://hook.example/x"})
	logs.requireLog(t, "error", "Failed to publish data error payload",
		"error", "queue", "user_id", "webhook_url")

	ch.publishErr = nil
	PublishDataErrorToQueue(WebhookErrorPayload{UserID: "user-1", URL: "https://hook.example/x"})
	logs.requireLog(t, "info", "Data error payload successfully published", "queue")
}

// ---------------------------------------------------------------------------
// Seam sobre o SDK
// ---------------------------------------------------------------------------

// TestDialAMQP_PropagatesDialError exercita o seam real contra uma URL
// invalida: nenhum broker e' necessario, o parse falha antes de qualquer I/O.
func TestDialAMQP_PropagatesDialError(t *testing.T) {
	conn, err := dialAMQP("nao-e-uma-url-amqp")
	if err == nil {
		t.Fatal("dialAMQP com URL invalida deveria falhar")
	}
	if conn != nil {
		t.Errorf("dialAMQP devolveu conexao %v junto com erro", conn)
	}
}
