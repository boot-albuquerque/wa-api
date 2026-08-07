package handshake

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/protocol/proto/waWa6"
	"wa-api/internal/wa-noise/protocol/socket"
	"wa-api/internal/wa-noise/security/keys"
)

// fullServer roda o lado servidor completo do Noise_XX ate' o ServerHello,
// respondendo com uma cadeia de certificados assinada pela raiz de teste (ver
// useTestRootKey) — ou seja, uma cadeia que VerifyServerCert aceita.
//
// E' o unico caminho por onde da' para exercitar a metade final de Do — tudo
// depois de VerifyServerCert. Contra a WACertPubKey de producao isso seria
// impossivel, e e' exatamente essa a propriedade de seguranca que os outros
// testes travam.
func fullServer(t *testing.T, root *keys.KeyPair) func([]byte) []byte {
	t.Helper()
	return func(clientHello []byte) []byte {
		var msg waWa6.HandshakeMessage
		if err := proto.Unmarshal(clientHello, &msg); err != nil {
			t.Errorf("ClientHello ilegivel: %v", err)
			return nil
		}
		clientEph := msg.GetClientHello().GetEphemeral()
		if len(clientEph) != NoiseKeyLength {
			t.Errorf("ephemeral do cliente com %d bytes", len(clientEph))
			return nil
		}
		clientEphArr := *(*[NoiseKeyLength]byte)(clientEph)

		nh := socket.NewNoiseHandshake()
		nh.Start(socket.NoiseStartPattern, socket.WAConnHeader)
		nh.Authenticate(clientEph)

		serverEph := keys.NewKeyPair()
		nh.Authenticate(serverEph.Pub[:])
		if err := nh.MixSharedSecretIntoKey(*serverEph.Priv, clientEphArr); err != nil {
			t.Errorf("mix do efemero: %v", err)
			return nil
		}
		serverStatic := keys.NewKeyPair()
		staticCiphertext := nh.Encrypt(serverStatic.Pub[:])
		if err := nh.MixSharedSecretIntoKey(*serverStatic.Priv, clientEphArr); err != nil {
			t.Errorf("mix da estatica: %v", err)
			return nil
		}
		chain := buildChain(t, root, validOpts(serverStatic.Pub[:]))
		certCiphertext := nh.Encrypt(chain)

		return mustMarshal(&waWa6.HandshakeMessage{ServerHello: &waWa6.HandshakeMessage_ServerHello{
			Ephemeral: serverEph.Pub[:],
			Static:    staticCiphertext,
			Payload:   certCiphertext,
		}})
	}
}

// Handshake completo contra um servidor que apresenta uma cadeia aceitavel: Do
// tem que devolver um NoiseSocket e nenhum erro. Cobre toda a metade final —
// cifragem da chave estatica do cliente, mix da chave privada Noise, marshal e
// envio do ClientFinish, e nh.Finish.
func TestDoCompletesAgainstAcceptableServer(t *testing.T) {
	root := useTestRootKey(t)
	srv := newFakeServer(t, fullServer(t, root))
	fs := connectedSocket(t, srv)

	cfg := testConfig()
	ns, err := Do(context.Background(), fs, cfg)

	if err != nil {
		t.Fatalf("handshake contra servidor aceitavel falhou: %v", err)
	}
	if ns == nil {
		t.Fatal("Do devolveu socket nil sem erro")
	}
	ns.Stop(false, false)
}

// NoiseKey nil so' e' desreferenciada depois de a cadeia ter sido aceita. Este
// teste documenta essa ordem: contra um servidor aceitavel, a falta de NoiseKey
// vira panic, entao a raiz precisa garantir Store.NoiseKey != nil antes de
// chamar. Em producao NewClient sempre preenche.
func TestDoPanicsWithoutNoiseKeyAfterCertAccepted(t *testing.T) {
	root := useTestRootKey(t)
	srv := newFakeServer(t, fullServer(t, root))
	fs := connectedSocket(t, srv)

	cfg := testConfig()
	cfg.NoiseKey = nil

	defer func() {
		if recover() == nil {
			t.Error("queria panic de nil deref em Config.NoiseKey")
		}
	}()
	_, _ = Do(context.Background(), fs, cfg)
}

// ClientPayload nil e' marshalado como mensagem vazia pelo protobuf — nao e'
// erro. O teste existe para que ninguem "corrija" isso achando que era.
func TestDoAcceptsNilClientPayload(t *testing.T) {
	root := useTestRootKey(t)
	srv := newFakeServer(t, fullServer(t, root))
	fs := connectedSocket(t, srv)

	cfg := testConfig()
	cfg.ClientPayload = func() *waWa6.ClientPayload { return nil }
	ns, err := Do(context.Background(), fs, cfg)

	if err != nil {
		t.Fatalf("payload nil deveria ser aceito, err = %v", err)
	}
	ns.Stop(false, false)
}
