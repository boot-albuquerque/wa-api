package message

import (
	"context"
	"errors"
	"testing"
	"time"

	"wa-api/internal/wa-noise/persistence/store"
	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
)

// --- HistorySyncQueue ---

func TestHistorySyncQueueShape(t *testing.T) {
	q := NewHistorySyncQueue(7)
	if !q.Ready() {
		t.Fatal("Ready = false apos NewHistorySyncQueue")
	}
	if q.Cap() != 7 {
		t.Errorf("Cap = %d, queria 7", q.Cap())
	}
	if q.Len() != 0 {
		t.Errorf("Len = %d, queria 0", q.Len())
	}
	var zero *HistorySyncQueue
	if zero.Ready() {
		t.Error("Ready = true para fila nil")
	}
}

// O flag de loop ativo e' o que garante NO MAXIMO um consumidor. Duas chamadas
// seguidas de EnqueueHistorySync nao podem ligar dois loops.
func TestEnqueueHistorySyncStartsLoopOnce(t *testing.T) {
	f := newFakeTransport()
	f.downloadErr = errors.New("sem rede")

	EnqueueHistorySync(f, &waE2E.HistorySyncNotification{})
	EnqueueHistorySync(f, &waE2E.HistorySyncNotification{})

	if !waitFor(t, f, func() bool { return f.histSync.Len() == 0 }) {
		t.Fatal("a fila nao foi drenada")
	}
	if !f.histSync.handlerActive.Load() {
		t.Fatal("o loop deveria continuar ativo apos drenar a fila")
	}
}

// --- DecryptBufferState ---

// A primeira chamada limpa (o zero de time.Time e' distante o suficiente); a
// segunda, logo em seguida, nao — o intervalo minimo e' de 12h.
func TestDecryptBufferStateShouldClearRespectsInterval(t *testing.T) {
	var s DecryptBufferState
	if !s.ShouldClear() {
		t.Fatal("primeira chamada = false, queria true")
	}
	if s.ShouldClear() {
		t.Fatal("segunda chamada imediata = true, queria false")
	}
	// Voltando o marcador para alem do intervalo, volta a limpar.
	s.lastClear = time.Now().Add(-decryptedBufferClearInterval - time.Minute)
	if !s.ShouldClear() {
		t.Fatal("apos o intervalo = false, queria true")
	}
}

// --- BufferedDecrypt ---

// Com o buffer DESLIGADO, BufferedDecrypt e' um passa-adiante: chama o decrypt,
// nao toca no store e devolve hash zerado.
func TestBufferedDecryptDisabledIsPassthrough(t *testing.T) {
	f := newFakeTransport()
	called := 0
	pt, hash, err := BufferedDecrypt(context.Background(), f, []byte("ct"), time.Now(),
		func(context.Context) ([]byte, error) {
			called++
			return []byte("pt"), nil
		}, "dominio")
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if called != 1 || string(pt) != "pt" {
		t.Fatalf("called=%d pt=%q", called, pt)
	}
	if hash != [32]byte{} {
		t.Errorf("hash = %X, queria zerado com o buffer desligado", hash)
	}
}

// bufferStore permite encenar as tres respostas possiveis do buffer: ausente,
// ja' processado (Plaintext nil) e ja' decifrado (Plaintext preenchido).
type bufferStore struct {
	store.NoopStore

	buffered  *store.BufferedEvent
	getErr    error
	putCalls  int
	txnCalls  int
	putErr    error
	lastStore []byte
}

func (b *bufferStore) GetBufferedEvent(context.Context, [32]byte) (*store.BufferedEvent, error) {
	return b.buffered, b.getErr
}

func (b *bufferStore) PutBufferedEvent(_ context.Context, _ [32]byte, plaintext []byte, _ time.Time) error {
	b.putCalls++
	b.lastStore = plaintext
	return b.putErr
}

func (b *bufferStore) DoDecryptionTxn(ctx context.Context, fn func(context.Context) error) error {
	b.txnCalls++
	return fn(ctx)
}

func bufferedTransport(b *bufferStore) *fakeTransport {
	f := newFakeTransport()
	f.buffer = true
	f.dev.EventBuffer = b
	return f
}

// Buffer ligado, ciphertext inedito: decifra dentro da transacao e grava.
func TestBufferedDecryptStoresNewPlaintext(t *testing.T) {
	b := &bufferStore{}
	f := bufferedTransport(b)

	pt, hash, err := BufferedDecrypt(context.Background(), f, []byte("ct"), time.Now(),
		func(context.Context) ([]byte, error) { return []byte("pt"), nil }, "dominio")
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if string(pt) != "pt" || string(b.lastStore) != "pt" {
		t.Fatalf("pt=%q gravado=%q", pt, b.lastStore)
	}
	if b.txnCalls != 1 || b.putCalls != 1 {
		t.Errorf("txn=%d put=%d", b.txnCalls, b.putCalls)
	}
	if hash == [32]byte{} {
		t.Error("hash zerado com o buffer ligado")
	}
}

// Entrada no buffer com Plaintext preenchido: devolve o plaintext gravado sem
// chamar o decrypt de novo. E' o ponto do buffer.
func TestBufferedDecryptReturnsCachedPlaintext(t *testing.T) {
	b := &bufferStore{buffered: &store.BufferedEvent{Plaintext: []byte("cacheado")}}
	f := bufferedTransport(b)
	called := false

	pt, _, err := BufferedDecrypt(context.Background(), f, []byte("ct"), time.Now(),
		func(context.Context) ([]byte, error) { called = true; return nil, nil })
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if called {
		t.Fatal("chamou o decrypt apesar do plaintext estar em cache")
	}
	if string(pt) != "cacheado" {
		t.Fatalf("pt = %q", pt)
	}
}

// Entrada com Plaintext nil = ja' foi entregue aos handlers. Tem que devolver
// ErrEventAlreadyProcessed, que o chamador IGNORA em silencio — se virasse
// falha de decifragem, mandaria retry receipt e o telefone reenviaria uma
// mensagem ja' processada.
func TestBufferedDecryptAlreadyProcessed(t *testing.T) {
	b := &bufferStore{buffered: &store.BufferedEvent{InsertTime: time.Now()}}
	f := bufferedTransport(b)

	_, _, err := BufferedDecrypt(context.Background(), f, []byte("ct"), time.Now(),
		func(context.Context) ([]byte, error) { return []byte("pt"), nil })
	if !errors.Is(err, ErrEventAlreadyProcessed) {
		t.Fatalf("err = %v", err)
	}
}

func TestBufferedDecryptGetError(t *testing.T) {
	sentinel := errors.New("banco caiu")
	f := bufferedTransport(&bufferStore{getErr: sentinel})
	if _, _, err := BufferedDecrypt(context.Background(), f, []byte("ct"), time.Now(),
		func(context.Context) ([]byte, error) { return nil, nil }); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
}

// Erro do decrypt sobe pela transacao sem gravar nada.
func TestBufferedDecryptDecryptErrorSkipsPut(t *testing.T) {
	b := &bufferStore{}
	f := bufferedTransport(b)
	sentinel := errors.New("sem sessao")
	if _, _, err := BufferedDecrypt(context.Background(), f, []byte("ct"), time.Now(),
		func(context.Context) ([]byte, error) { return nil, sentinel }); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
	if b.putCalls != 0 {
		t.Fatalf("gravou %d vezes apos falha de decifragem", b.putCalls)
	}
}

// O hash e' a chave persistida do buffer. Partes extras diferentes tem que dar
// hashes diferentes — e' a separacao de dominio entre prekey/normal/senderkey.
func TestBufferedDecryptHashIsDomainSeparated(t *testing.T) {
	hashFor := func(extra ...string) [32]byte {
		f := bufferedTransport(&bufferStore{})
		_, h, err := BufferedDecrypt(context.Background(), f, []byte("ct"), time.Now(),
			func(context.Context) ([]byte, error) { return []byte("pt"), nil }, extra...)
		if err != nil {
			t.Fatalf("erro: %v", err)
		}
		return h
	}
	base := hashFor(ciphertextHashDomainNormal, "a@s.whatsapp.net")
	variants := map[string][32]byte{
		"outro dominio":   hashFor(ciphertextHashDomainPreKey, "a@s.whatsapp.net"),
		"outro remetente": hashFor(ciphertextHashDomainNormal, "b@s.whatsapp.net"),
		"parte a mais":    hashFor(ciphertextHashDomainSenderKey, "a@s.whatsapp.net", "grupo"),
		"sem partes":      hashFor(),
	}
	for name, got := range variants {
		if got == base {
			t.Errorf("%s: mesmo hash que o base", name)
		}
	}
}

// --- DecryptDM / DecryptGroupMsg: guardas de tipo ---

// Conteudo que nao e' []byte vira erro, e nao panico — mesma classe de bug que
// a regressao do <enc type="msmsg">.
func TestDecryptDMAndGroupRejectNonByteContent(t *testing.T) {
	f := newFakeTransport()
	child := &waBinary.Node{Tag: "enc", Content: []waBinary.Node{{Tag: "x"}}}

	if _, _, err := DecryptDM(context.Background(), f, child, testOtherJID, false, time.Now()); err == nil {
		t.Error("DecryptDM: esperava erro")
	}
	if _, _, err := DecryptGroupMsg(context.Background(), f, child, testOtherJID, testGroupJID, time.Now()); err == nil {
		t.Error("DecryptGroupMsg: esperava erro")
	}
}

// Bytes que nao formam um SignalMessage/PreKeySignalMessage/SenderKeyMessage
// valido param no parse, antes de tocar na sessao.
func TestDecryptDMAndGroupRejectMalformedCiphertext(t *testing.T) {
	f := newFakeTransport()
	child := &waBinary.Node{Tag: "enc", Content: []byte{0xFF, 0xFF, 0xFF, 0xFF}}

	for _, isPreKey := range []bool{true, false} {
		if _, _, err := DecryptDM(context.Background(), f, child, testOtherJID, isPreKey, time.Now()); err == nil {
			t.Errorf("DecryptDM(isPreKey=%v): esperava erro", isPreKey)
		}
	}
	if _, _, err := DecryptGroupMsg(context.Background(), f, child, testOtherJID, testGroupJID, time.Now()); err == nil {
		t.Error("DecryptGroupMsg: esperava erro")
	}
}
