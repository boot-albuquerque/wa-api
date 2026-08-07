package handshake

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/protocol/proto/waCert"
	"wa-api/internal/wa-noise/protocol/proto/waWa6"
	"wa-api/internal/wa-noise/protocol/socket"
	"wa-api/internal/wa-noise/security/keys"
)

// serverHello roda o **lado servidor** do Noise_XX ate' o ServerHello, espelhando
// exatamente a sequencia que Do executa do lado cliente: Authenticate(efemero
// do servidor), mix com o efemero do cliente, Encrypt(chave estatica), mix com
// a estatica, Encrypt(cadeia de certificados).
//
// Isso permite testar o unico ponto de defesa que sobra depois de o Noise ter
// funcionado: a verificacao da cadeia. Um MITM consegue fazer tudo isto — o que
// ele nao consegue e' assinar a cadeia com a chave da WhatsApp. Este teste
// prova que e' exatamente ai' que ele para.
func serverHello(t *testing.T, clientHello []byte, certChain []byte) []byte {
	t.Helper()
	var msg waWa6.HandshakeMessage
	if err := proto.Unmarshal(clientHello, &msg); err != nil {
		t.Errorf("ClientHello ilegivel: %v", err)
		return nil
	}
	clientEph := msg.GetClientHello().GetEphemeral()
	if len(clientEph) != NoiseKeyLength {
		t.Errorf("ephemeral do cliente com %d bytes, queria %d", len(clientEph), NoiseKeyLength)
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
	certCiphertext := nh.Encrypt(certChain)

	return mustMarshal(&waWa6.HandshakeMessage{ServerHello: &waWa6.HandshakeMessage_ServerHello{
		Ephemeral: serverEph.Pub[:],
		Static:    staticCiphertext,
		Payload:   certCiphertext,
	}})
}

// Servidor que executa o Noise corretamente mas apresenta uma cadeia de
// certificados que nao e' da WhatsApp: o handshake tem que parar na verificacao
// da cadeia. Sem isso, qualquer servidor conseguiria se passar pela WhatsApp.
func TestDoRejectsServerWithForgedCertChain(t *testing.T) {
	details := mustMarshal(&waCert.CertChain_NoiseCertificate_Details{
		Serial: proto.Uint32(1),
		Key:    make([]byte, NoiseKeyLength),
	})
	forged := mustMarshal(&waCert.CertChain{
		Intermediate: &waCert.CertChain_NoiseCertificate{
			Details:   details,
			Signature: make([]byte, CertSignatureLength),
		},
		Leaf: &waCert.CertChain_NoiseCertificate{
			Details:   details,
			Signature: make([]byte, CertSignatureLength),
		},
	})

	srv := newFakeServer(t, func(clientHello []byte) []byte {
		return serverHello(t, clientHello, forged)
	})
	fs := connectedSocket(t, srv)

	ns, err := Do(context.Background(), fs, testConfig())

	if ns != nil {
		t.Error("um servidor com cadeia forjada nao pode terminar o handshake")
	}
	if err == nil {
		t.Fatal("cadeia forjada deveria ser rejeitada")
	}
	if !strings.Contains(err.Error(), "failed to verify server cert") {
		t.Errorf("err = %v, queria \"failed to verify server cert\"", err)
	}
}

// Mesma coisa, mas o payload cifrado nem sequer e' uma CertChain valida: tem
// que virar erro de verificacao, nao panic.
func TestDoRejectsServerWithUnparseableCertChain(t *testing.T) {
	srv := newFakeServer(t, func(clientHello []byte) []byte {
		return serverHello(t, clientHello, []byte{0xff, 0xff, 0xff, 0xff})
	})
	fs := connectedSocket(t, srv)

	_, err := Do(context.Background(), fs, testConfig())

	if err == nil {
		t.Fatal("cadeia ilegivel deveria ser rejeitada")
	}
	if !strings.Contains(err.Error(), "failed to verify server cert") {
		t.Errorf("err = %v, queria \"failed to verify server cert\"", err)
	}
}

// Config sem NoiseKey e' erro de programacao da raiz, nao dado de rede. O teste
// existe para documentar que o campo e' obrigatorio: Do so' chega a le-lo depois
// de a cadeia ter sido aceita, entao a falha aparece tarde.
func TestConfigNoiseKeyIsRequiredOnlyAfterCertCheck(t *testing.T) {
	cfg := testConfig()
	cfg.NoiseKey = nil
	srv := newFakeServer(t, func([]byte) []byte {
		return mustMarshal(&waWa6.HandshakeMessage{})
	})
	fs := connectedSocket(t, srv)

	_, err := Do(context.Background(), fs, cfg)

	if err == nil || !strings.Contains(err.Error(), "missing parts of handshake response") {
		t.Errorf("err = %v, queria a falha de resposta e nao um nil deref de NoiseKey", err)
	}
}
