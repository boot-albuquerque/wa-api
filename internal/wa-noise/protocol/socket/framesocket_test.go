package socket

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	waLog "wa-api/internal/wa-noise/observability/log"
)

// collectFrames roda processData com as mensagens dadas e devolve os frames
// completos entregues no canal Frames. Nao ha rede envolvida: processData e'
// logica pura de remontagem sobre o buffer interno do FrameSocket.
func collectFrames(t *testing.T, messages ...[]byte) [][]byte {
	t.Helper()
	fs := NewFrameSocket(waLog.Noop, nil)
	got := make([][]byte, 0, len(messages))
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, msg := range messages {
			fs.processData(msg)
		}
	}()
	for {
		select {
		case frame := <-fs.Frames:
			got = append(got, frame)
		case <-done:
			// Drena o que ficou pendente sem bloquear.
			for {
				select {
				case frame := <-fs.Frames:
					got = append(got, frame)
				default:
					return got
				}
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timeout esperando frames de processData")
		}
	}
}

func mustFrame(payload []byte) []byte {
	frame := make([]byte, FrameLengthSize+len(payload))
	encodeFrameLength(frame, len(payload))
	copy(frame[FrameLengthSize:], payload)
	return frame
}

func assertFrames(t *testing.T, got [][]byte, want ...[]byte) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("recebidos %d frames, esperados %d (%q)", len(got), len(want), got)
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Errorf("frame %d = %q, esperado %q", i, got[i], want[i])
		}
	}
}

// TestProcessDataSingleFrame: o caso feliz, um websocket message contendo
// exatamente um frame.
func TestProcessDataSingleFrame(t *testing.T) {
	payload := []byte("hello whatsapp")
	assertFrames(t, collectFrames(t, mustFrame(payload)), payload)
}

// TestProcessDataMultipleFramesInOneMessage: o servidor pode empacotar varios
// frames num unico websocket message; o loop tem que emitir todos.
func TestProcessDataMultipleFramesInOneMessage(t *testing.T) {
	a, b := []byte("primeiro"), []byte("segundo")
	msg := append(mustFrame(a), mustFrame(b)...)
	assertFrames(t, collectFrames(t, msg), a, b)
}

// TestProcessDataSplitPayload: um frame cujo payload chega picado em varios
// websocket messages tem que ser remontado antes de ser emitido.
//
// Ficou `t.Skip`ado ate' a correcao da F18: receivedLength era contado antes de
// o cabecalho ser descartado, entao o segundo pedaco era escrito
// FrameLengthSize bytes adiante do lugar certo. O frame saia como
// "pay\x00\x00\x00load longo dividido em peda" — bytes zerados no meio e o
// fim truncado.
func TestProcessDataSplitPayload(t *testing.T) {
	payload := []byte("payload longo dividido em pedacos")
	full := mustFrame(payload)
	assertFrames(t, collectFrames(t, full[:6], full[6:14], full[14:]), payload)
}

// A F18 so' aparecia com o payload cortado em pontos especificos, entao vale
// varrer todos os cortes possiveis em vez de confiar num so'. Qualquer
// aritmetica de offset errada em processData cai em pelo menos um deles.
func TestProcessDataSplitPayloadEmTodosOsPontosDeCorte(t *testing.T) {
	payload := []byte("payload longo dividido em pedacos")
	full := mustFrame(payload)
	for corte := 1; corte < len(full); corte++ {
		got := collectFrames(t, full[:corte], full[corte:])
		if len(got) != 1 {
			t.Errorf("corte em %d: %d frames, esperava 1", corte, len(got))
			continue
		}
		if string(got[0]) != string(payload) {
			t.Errorf("corte em %d: frame = %q, esperava %q", corte, got[0], payload)
		}
	}
}

// TestProcessDataPartialHeader: menos de FrameLengthSize bytes nao dao para
// decodificar o comprimento; o resto do cabecalho vem no proximo message.
func TestProcessDataPartialHeader(t *testing.T) {
	payload := []byte("cabecalho picado")
	full := mustFrame(payload)
	assertFrames(t, collectFrames(t, full[:2], full[2:]), payload)
}

// TestProcessDataEmptyFrame: comprimento zero e' representavel e nao pode
// travar o loop de remontagem.
func TestProcessDataEmptyFrame(t *testing.T) {
	got := collectFrames(t, mustFrame(nil))
	if len(got) != 1 || len(got[0]) != 0 {
		t.Fatalf("esperado um frame vazio, recebido %q", got)
	}
}

// TestSendFrameWithoutConnection: sem conexao, SendFrame devolve
// ErrSocketClosed em vez de derefenciar conn nil.
func TestSendFrameWithoutConnection(t *testing.T) {
	fs := NewFrameSocket(waLog.Noop, nil)
	if err := fs.SendFrame([]byte("x")); !errors.Is(err, ErrSocketClosed) {
		t.Fatalf("SendFrame sem conexao devolveu %v, esperado ErrSocketClosed", err)
	}
}

// TestNewFrameSocketDefaults trava os defaults que o handshake depende: o
// header WA que abre a conexao, a URL e o header Origin.
func TestNewFrameSocketDefaults(t *testing.T) {
	fs := NewFrameSocket(waLog.Noop, nil)
	if !bytes.Equal(fs.Header, WAConnHeader) {
		t.Errorf("Header = %v, esperado %v", fs.Header, WAConnHeader)
	}
	if fs.URL != URL {
		t.Errorf("URL = %q, esperado %q", fs.URL, URL)
	}
	if got := fs.HTTPHeaders.Get(originHeaderName); got != Origin {
		t.Errorf("header %s = %q, esperado %q", originHeaderName, got, Origin)
	}
	if fs.IsConnected() {
		t.Error("FrameSocket recem-criado nao deveria estar conectado")
	}
}

// TestWAConnHeaderShape: o servidor rejeita a conexao se o prefixo mudar de
// forma ou de magic value.
func TestWAConnHeaderShape(t *testing.T) {
	if len(WAConnHeader) != 4 {
		t.Fatalf("WAConnHeader tem %d bytes, esperado 4", len(WAConnHeader))
	}
	if WAConnHeader[0] != 'W' || WAConnHeader[1] != 'A' || WAConnHeader[2] != WAMagicValue {
		t.Errorf("WAConnHeader = %v, esperado prefixo 'W','A',%d", WAConnHeader, WAMagicValue)
	}
}

// O read pump (que chama Close no defer) e o handshake (que registra o
// callback) rodam em goroutines diferentes, e a janela entre Connect e
// SetOnDisconnect cobre um round trip de rede inteiro. Como campo exportado sem
// lock isso era data race (F56); com SetOnDisconnect sob fs.lock, nao e' mais.
func TestSetOnDisconnectEConcorrenteComClose(t *testing.T) {
	fs := NewFrameSocket(waLog.Noop, nil)
	const rodadas = 200

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < rodadas; i++ {
			fs.SetOnDisconnect(func(context.Context, bool) {})
			fs.SetOnDisconnect(nil)
		}
	}()
	for i := 0; i < rodadas; i++ {
		// Close com conn nil sai cedo, mas so' depois de tomar fs.lock — que e'
		// exatamente o lock que precisa sincronizar com o setter.
		fs.Close(statusForceClose)
	}
	<-done
}
