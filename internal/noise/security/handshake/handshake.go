package handshake

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/noise/protocol/proto/waWa6"
	"wa-api/internal/noise/protocol/socket"
	"wa-api/internal/noise/security/keys"
)

// Config e' todo o material que Do precisa do cliente. E' deliberadamente um
// struct de valores, e nao uma interface "Transport" como nos outros
// subpacotes: o handshake nao consulta o cliente ao longo do caminho, ele
// recebe tudo de uma vez no comeco.
//
// Em particular Config **nao** carrega o *Client nem o socket atual. O
// *socket.NoiseSocket resultante e' devolvido por Do; guardar esse ponteiro em
// Client.socket continua sendo responsabilidade da raiz, que faz isso sob
// socketLock (ver client_connection.go, unlockedConnect).
type Config struct {
	// NoiseKey e' o par de chaves estatico do device (Store.NoiseKey).
	NoiseKey *keys.KeyPair
	// EphemeralKP e' o par efemero sorteado para esta conexao.
	EphemeralKP keys.KeyPair
	// ClientPayload devolve o payload de login. E' uma **funcao**, e nao o
	// valor ja' resolvido, de proposito: o codigo original lia
	// Client.GetClientPayload no meio do handshake, depois da verificacao do
	// certificado — ou seja, depois de um round trip de rede inteiro (ate' 20s,
	// ver ResponseTimeout). Passar o valor pronto adiantaria essa leitura para
	// antes do ClientHello, e GetClientPayload e' um campo de funcao publico,
	// trocavel em tempo de execucao. Como funcao, Do a chama no mesmo ponto da
	// sequencia em que o original lia o campo, e a extracao fica literal.
	ClientPayload func() *waWa6.ClientPayload
	// FrameHandler e DisconnectHandler sao repassados intactos ao NoiseSocket.
	FrameHandler      socket.FrameHandler
	DisconnectHandler socket.DisconnectHandler
}

// Do implementa o handshake Noise_XX_25519_AESGCM_SHA256 sobre um FrameSocket
// ja' conectado e devolve o NoiseSocket resultante.
//
// Em caso de erro o FrameSocket **nao** e' fechado: quem abriu fecha. Era assim
// antes da extracao (unlockedConnect chama fs.Close(0) nos dois ramos de erro) e
// continua sendo.
func Do(ctx context.Context, fs *socket.FrameSocket, cfg Config) (*socket.NoiseSocket, error) {
	nh := socket.NewNoiseHandshake()
	nh.Start(socket.NoiseStartPattern, fs.Header)
	nh.Authenticate(cfg.EphemeralKP.Pub[:])
	data, err := proto.Marshal(&waWa6.HandshakeMessage{
		ClientHello: &waWa6.HandshakeMessage_ClientHello{
			Ephemeral: cfg.EphemeralKP.Pub[:],
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal handshake message: %w", err)
	}
	err = fs.SendFrame(data)
	if err != nil {
		return nil, fmt.Errorf("failed to send handshake message: %w", err)
	}
	var resp []byte
	select {
	case resp = <-fs.Frames:
	case <-time.After(ResponseTimeout):
		return nil, fmt.Errorf("timed out waiting for handshake response")
	}
	var handshakeResponse waWa6.HandshakeMessage
	err = proto.Unmarshal(resp, &handshakeResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal handshake response: %w", err)
	}
	serverEphemeral := handshakeResponse.GetServerHello().GetEphemeral()
	serverStaticCiphertext := handshakeResponse.GetServerHello().GetStatic()
	certificateCiphertext := handshakeResponse.GetServerHello().GetPayload()
	if len(serverEphemeral) != NoiseKeyLength || serverStaticCiphertext == nil || certificateCiphertext == nil {
		return nil, fmt.Errorf("missing parts of handshake response")
	}
	serverEphemeralArr := *(*[NoiseKeyLength]byte)(serverEphemeral)

	nh.Authenticate(serverEphemeral)
	err = nh.MixSharedSecretIntoKey(*cfg.EphemeralKP.Priv, serverEphemeralArr)
	if err != nil {
		return nil, fmt.Errorf("failed to mix server ephemeral key in: %w", err)
	}

	staticDecrypted, err := nh.Decrypt(serverStaticCiphertext)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt server static ciphertext: %w", err)
	} else if len(staticDecrypted) != NoiseKeyLength {
		return nil, fmt.Errorf("unexpected length of server static plaintext %d (expected %d)", len(staticDecrypted), NoiseKeyLength)
	}
	err = nh.MixSharedSecretIntoKey(*cfg.EphemeralKP.Priv, *(*[NoiseKeyLength]byte)(staticDecrypted))
	if err != nil {
		return nil, fmt.Errorf("failed to mix server static key in: %w", err)
	}

	certDecrypted, err := nh.Decrypt(certificateCiphertext)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt noise certificate ciphertext: %w", err)
	} else if err = VerifyServerCert(certDecrypted, staticDecrypted); err != nil {
		return nil, fmt.Errorf("failed to verify server cert: %w", err)
	}

	encryptedPubkey := nh.Encrypt(cfg.NoiseKey.Pub[:])
	err = nh.MixSharedSecretIntoKey(*cfg.NoiseKey.Priv, serverEphemeralArr)
	if err != nil {
		return nil, fmt.Errorf("failed to mix noise private key in: %w", err)
	}

	// Este e' o ponto exato em que o codigo original lia cli.GetClientPayload.
	clientFinishPayloadBytes, err := proto.Marshal(cfg.ClientPayload())
	if err != nil {
		return nil, fmt.Errorf("failed to marshal client finish payload: %w", err)
	}
	encryptedClientFinishPayload := nh.Encrypt(clientFinishPayloadBytes)
	data, err = proto.Marshal(&waWa6.HandshakeMessage{
		ClientFinish: &waWa6.HandshakeMessage_ClientFinish{
			Static:  encryptedPubkey,
			Payload: encryptedClientFinishPayload,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal handshake finish message: %w", err)
	}
	err = fs.SendFrame(data)
	if err != nil {
		return nil, fmt.Errorf("failed to send handshake finish message: %w", err)
	}

	ns, err := nh.Finish(ctx, fs, cfg.FrameHandler, cfg.DisconnectHandler)
	if err != nil {
		return nil, fmt.Errorf("failed to create noise socket: %w", err)
	}
	return ns, nil
}
