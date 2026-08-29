package send

import (
	"context"
	"errors"
	"strings"
	"testing"

	"wa-api/internal/noise/persistence/store"
	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/proto/waMsgTransport"
	"wa-api/internal/noise/protocol/types"
)

// O que este arquivo NAO cobre, e por que:
//
// A cifragem propriamente dita (session.NewCipher(...).Encrypt) exige uma
// sessao Signal estabelecida — o que so' existe depois de um handshake real com
// o servidor ou de um bundle de prekey valido de outro dispositivo. Nao da'
// para simular isso em teste de unidade sem reimplementar metade do libsignal,
// e reimplementar seria pior que nao testar: um duble que "cifra" errado
// passaria verde enquanto o fio recebe lixo.
//
// O que DA' para travar sem sessao viva, e esta' travado aqui:
//   - a recusa quando nao ha sessao (o caminho de erro mais importante);
//   - a propagacao de erro do store;
//   - o curto-circuito de lista vazia, que e' o que permite testar toda a
//     montagem de no nos outros arquivos;
//   - copyAttrs, cuja ordem de sobrescrita decide o `type` que vai no <enc>.

// --- EncryptForDevice: sem sessao ---

func TestEncryptForDeviceWithoutSessionFails(t *testing.T) {
	tr := loggedIn()
	_, _, err := EncryptForDevice(
		context.Background(), tr, []byte("oi"), sendTestUserJID, nil, nil, nil,
	)
	if !errors.Is(err, ErrNoSession) {
		t.Fatalf("err = %v, want ErrNoSession", err)
	}
	// O endereco Signal entra no texto do erro: e' o que permite descobrir
	// QUAL dispositivo ficou sem sessao num grupo de dezenas.
	if !strings.Contains(err.Error(), sendTestUserJID.SignalAddress().String()) {
		t.Errorf("err = %q, deveria citar o endereco Signal", err)
	}
}

// O mapa de sessoes prefetchadas evita uma consulta por dispositivo. Um `false`
// explicito la' e' resposta final: nao pode cair no ContainsSession.
func TestEncryptForDeviceUsesPrefetchedSessionMap(t *testing.T) {
	tr := loggedIn()
	tr.store.Sessions = explodingSessions{&store.NoopStore{}}
	existing := map[string]bool{sendTestUserJID.SignalAddress().String(): false}
	_, _, err := EncryptForDevice(
		context.Background(), tr, []byte("oi"), sendTestUserJID, nil, nil, existing,
	)
	if !errors.Is(err, ErrNoSession) {
		t.Fatalf("err = %v, want ErrNoSession", err)
	}
}

func TestEncryptForDevicePropagatesSessionStoreError(t *testing.T) {
	tr := loggedIn()
	tr.store.Sessions = explodingSessions{&store.NoopStore{}}
	_, _, err := EncryptForDevice(
		context.Background(), tr, []byte("oi"), sendTestUserJID, nil, nil, nil,
	)
	if !errors.Is(err, errSessionBoom) {
		t.Fatalf("err = %v, want %v", err, errSessionBoom)
	}
}

// A versao V3 devolve ErrNoSession NU (sem o endereco no texto), ao contrario
// da waE2E. A diferenca e' do upstream e foi preservada na extracao; o teste
// existe para que uma "harmonizacao" futura seja deliberada.
func TestEncryptForDeviceV3WithoutSessionFailsBare(t *testing.T) {
	tr := loggedIn()
	_, err := EncryptForDeviceV3(
		context.Background(), tr, &waMsgTransport.MessageTransport_Payload{}, nil, nil,
		sendTestUserJID, nil, nil,
	)
	if err != ErrNoSession {
		t.Fatalf("err = %v, queria exatamente ErrNoSession", err)
	}
}

// --- lista vazia ---

func TestEncryptForDevicesEmptyListIsNoop(t *testing.T) {
	tr := loggedIn()
	nodes, includeIdentity, err := EncryptForDevices(
		context.Background(), tr, nil, "MSG1", []byte("oi"), nil, waBinary.Attrs{},
	)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(nodes) != 0 || includeIdentity {
		t.Errorf("nodes = %v, includeIdentity = %v", nodes, includeIdentity)
	}
}

func TestEncryptForDevicesV3EmptyListIsNoop(t *testing.T) {
	tr := loggedIn()
	nodes, err := EncryptForDevicesV3(
		context.Background(), tr, nil, tr.ownID, "MSG1", nil, nil, nil, waBinary.Attrs{},
	)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("nodes = %v", nodes)
	}
}

// O mapeamento LID/PN e' buscado antes de qualquer cifragem; se falhar, nada
// e' cifrado (cifrar para o PN quando existe LID gravaria a sessao errada).
func TestEncryptForDevicesPropagatesLIDMappingError(t *testing.T) {
	tr := loggedIn()
	sentinel := errors.New("boom")
	tr.store.LIDs = stubManyLIDStore{err: sentinel}
	_, _, err := EncryptForDevices(
		context.Background(), tr, []types.JID{sendTestUserJID}, "MSG1", []byte("oi"), nil, waBinary.Attrs{},
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}

// Um dispositivo sem sessao nao derruba a mensagem inteira: e' logado e
// pulado, e os outros seguem.
//
// O CONTRATO MUDOU AQUI (F62 em HOUSEKEEP.md). A versao anterior deste teste
// afirmava que, com TODOS os dispositivos falhando, a lista saia vazia "mas
// sem erro, que e' o contrato". Esse contrato estava errado: node_build.go
// monta o <participants> com a lista vazia sem checar, o stanza vai para o fio
// sem destinatario nenhum, e SendMessage devolve sucesso com ID de mensagem.
// Ninguem recebe e nada no retorno diz isso.
//
// A tolerancia a falha PARCIAL continua intacta — e' o caso do teste abaixo.
func TestEncryptForDevicesTodosSemSessaoEhErro(t *testing.T) {
	tr := loggedIn()
	_, _, err := EncryptForDevices(
		context.Background(), tr,
		[]types.JID{{User: "1", Server: types.HiddenUserServer, Device: 1}},
		"MSG1", []byte("oi"), nil, waBinary.Attrs{},
	)
	if !errors.Is(err, ErrAllDevicesFailedEncryption) {
		t.Fatalf("err = %v, queria %v", err, ErrAllDevicesFailedEncryption)
	}
	// A causa por dispositivo tem de sobreviver ao embrulho: sem ela, quem
	// investigar so' sabe que "tudo falhou", nao por que.
	if !errors.Is(err, ErrNoSession) {
		t.Errorf("err = %v, queria preservar %v como causa", err, ErrNoSession)
	}
}

// O proprio device do usuario e' pulado ANTES da cifragem, entao uma lista que
// so' tem ele nao conta como "todos falharam" — sai vazia e sem erro. Sem esta
// distincao, o contador de F62 acusaria falha total onde nao houve tentativa
// nenhuma.
func TestEncryptForDevicesSoODispositivoProprioNaoEhFalhaTotal(t *testing.T) {
	tr := loggedIn()
	nodes, includeIdentity, err := EncryptForDevices(
		context.Background(), tr, []types.JID{tr.ownID},
		"MSG1", []byte("oi"), []byte("dsm"), waBinary.Attrs{},
	)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(nodes) != 0 || includeIdentity {
		t.Errorf("nodes = %v, includeIdentity = %v", nodes, includeIdentity)
	}
}

// Contexto ja' cancelado transforma a falha por dispositivo em erro fatal — o
// contrario faria o laco varrer centenas de dispositivos depois do shutdown.
func TestEncryptForDevicesFailsFastOnCancelledContext(t *testing.T) {
	tr := loggedIn()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := EncryptForDevices(
		ctx, tr, []types.JID{{User: "1", Server: types.HiddenUserServer, Device: 1}},
		"MSG1", []byte("oi"), nil, waBinary.Attrs{},
	)
	if err == nil {
		t.Fatal("queria erro com contexto cancelado")
	}
}
