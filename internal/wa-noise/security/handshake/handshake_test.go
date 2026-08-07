package handshake

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/protocol/proto/waWa6"
	"wa-api/internal/wa-noise/protocol/socket"
	"wa-api/internal/wa-noise/security/keys"
	waLog "wa-api/internal/wa-noise/observability/log"
)

// Estes testes exercitam Do contra um servidor websocket de mentira. O caminho
// de **aceitacao** nao e' testavel aqui: terminar o handshake exigiria a chave
// privada com que a WhatsApp assina a cadeia de certificados, que nao existe
// fora dos servidores deles. O que da' para travar — e o que importa para
// seguranca — e' que toda resposta que nao seja a esperada vire erro, nunca
// panic e nunca um NoiseSocket valido.

// frame embrulha payload no formato de fio do FrameSocket: prefixo de
// comprimento de 3 bytes big-endian seguido do payload.
func frame(payload []byte) []byte {
	out := make([]byte, socket.FrameLengthSize+len(payload))
	out[0] = byte(len(payload) >> 16)
	out[1] = byte(len(payload) >> 8)
	out[2] = byte(len(payload))
	copy(out[socket.FrameLengthSize:], payload)
	return out
}

// newFakeServer sobe um websocket que le o ClientHello e responde com o que
// reply devolver. reply recebe o ClientHello ja' sem o cabecalho WA de 4 bytes
// e sem o prefixo de comprimento. Devolver nil faz o servidor nao responder.
func newFakeServer(t *testing.T, reply func(clientHello []byte) []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
		if err != nil {
			return
		}
		defer conn.CloseNow()
		_, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		// A primeira mensagem do cliente carrega o cabecalho WA (4 bytes) antes
		// do prefixo de comprimento.
		const waHeaderLen = 4
		if len(data) < waHeaderLen+socket.FrameLengthSize {
			return
		}
		body := data[waHeaderLen+socket.FrameLengthSize:]
		resp := reply(body)
		if resp == nil {
			<-r.Context().Done()
			return
		}
		_ = conn.Write(r.Context(), websocket.MessageBinary, frame(resp))
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	return srv
}

func connectedSocket(t *testing.T, srv *httptest.Server) *socket.FrameSocket {
	t.Helper()
	fs := socket.NewFrameSocket(waLog.Noop, srv.Client())
	fs.URL = "ws" + strings.TrimPrefix(srv.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	if err := fs.Connect(ctx); err != nil {
		t.Fatalf("conectar no servidor de mentira: %v", err)
	}
	t.Cleanup(func() { fs.Close(0) })
	return fs
}

func testConfig() Config {
	return Config{
		NoiseKey:      keys.NewKeyPair(),
		EphemeralKP:   *keys.NewKeyPair(),
		ClientPayload: func() *waWa6.ClientPayload { return &waWa6.ClientPayload{} },
		FrameHandler:  func(context.Context, []byte) {},
		DisconnectHandler: func(context.Context, *socket.NoiseSocket, bool) {
		},
	}
}

// Socket nao conectado: o primeiro SendFrame falha e Do tem que devolver erro
// embrulhado, sem tocar em nada mais.
func TestDoFailsWhenSocketNotConnected(t *testing.T) {
	fs := socket.NewFrameSocket(waLog.Noop, http.DefaultClient)
	ns, err := Do(context.Background(), fs, testConfig())
	if ns != nil {
		t.Error("nenhum NoiseSocket pode sair de um handshake que falhou")
	}
	if err == nil {
		t.Fatal("handshake sobre socket fechado deveria falhar")
	}
	if !strings.Contains(err.Error(), "failed to send handshake message") {
		t.Errorf("err = %v, queria \"failed to send handshake message\"", err)
	}
}

func TestDoRejectsBadServerHello(t *testing.T) {
	// Chave publica de verdade: um ephemeral todo zero e' low-order point e o
	// X25519 rejeita antes de chegar no ramo que se quer exercitar.
	valid := keys.NewKeyPair().Pub[:]
	cases := []struct {
		name         string
		reply        []byte
		wantContains string
	}{
		{
			name:         "resposta que nao e' protobuf",
			reply:        []byte{0xff, 0xff, 0xff, 0xff},
			wantContains: "failed to unmarshal handshake response",
		},
		{
			name:         "ServerHello ausente",
			reply:        mustMarshal(&waWa6.HandshakeMessage{}),
			wantContains: "missing parts of handshake response",
		},
		{
			name: "ephemeral com tamanho errado",
			reply: mustMarshal(&waWa6.HandshakeMessage{ServerHello: &waWa6.HandshakeMessage_ServerHello{
				Ephemeral: make([]byte, NoiseKeyLength-1),
				Static:    []byte{1},
				Payload:   []byte{1},
			}}),
			wantContains: "missing parts of handshake response",
		},
		{
			name: "static ausente",
			reply: mustMarshal(&waWa6.HandshakeMessage{ServerHello: &waWa6.HandshakeMessage_ServerHello{
				Ephemeral: valid,
				Payload:   []byte{1},
			}}),
			wantContains: "missing parts of handshake response",
		},
		{
			name: "payload ausente",
			reply: mustMarshal(&waWa6.HandshakeMessage{ServerHello: &waWa6.HandshakeMessage_ServerHello{
				Ephemeral: valid,
				Static:    []byte{1},
			}}),
			wantContains: "missing parts of handshake response",
		},
		{
			name: "static que nao decifra",
			reply: mustMarshal(&waWa6.HandshakeMessage{ServerHello: &waWa6.HandshakeMessage_ServerHello{
				Ephemeral: valid,
				Static:    make([]byte, 48),
				Payload:   make([]byte, 48),
			}}),
			wantContains: "failed to decrypt server static ciphertext",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reply := tc.reply
			srv := newFakeServer(t, func([]byte) []byte { return reply })
			fs := connectedSocket(t, srv)
			ns, err := Do(context.Background(), fs, testConfig())
			if ns != nil {
				t.Error("nenhum NoiseSocket pode sair de um handshake que falhou")
			}
			if err == nil {
				t.Fatal("deveria falhar")
			}
			if !strings.Contains(err.Error(), tc.wantContains) {
				t.Errorf("err = %v, queria conter %q", err, tc.wantContains)
			}
		})
	}
}

// O ephemeral que o cliente manda tem que ser exatamente a chave publica
// efemera passada em Config — e' o que o servidor usa para derivar o segredo
// compartilhado. Se Do mandasse outra coisa, o handshake so' falharia contra o
// servidor real.
func TestDoSendsConfiguredEphemeralKey(t *testing.T) {
	cfg := testConfig()
	got := make(chan []byte, 1)
	srv := newFakeServer(t, func(clientHello []byte) []byte {
		var msg waWa6.HandshakeMessage
		if err := proto.Unmarshal(clientHello, &msg); err == nil {
			got <- msg.GetClientHello().GetEphemeral()
		} else {
			got <- nil
		}
		return []byte{0xff}
	})
	fs := connectedSocket(t, srv)
	_, _ = Do(context.Background(), fs, cfg)

	select {
	case sent := <-got:
		if string(sent) != string(cfg.EphemeralKP.Pub[:]) {
			t.Errorf("ephemeral enviado = %x, queria %x", sent, cfg.EphemeralKP.Pub[:])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("servidor nao recebeu o ClientHello")
	}
}

func mustMarshal(m proto.Message) []byte {
	raw, err := proto.Marshal(m)
	if err != nil {
		panic(err)
	}
	return raw
}
