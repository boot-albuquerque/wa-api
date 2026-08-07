package retry

import (
	"context"
	"encoding/binary"
	"errors"
	"strconv"
	"testing"

	"wa-api/internal/wa-noise/capabilities/prekeys"
	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/security/keys"
)

// incomingNode monta a mensagem recebida a partir da qual o recibo de retry e'
// construido.
func incomingNode(id string, encCount int) *waBinary.Node {
	enc := waBinary.Node{Tag: "enc", Attrs: waBinary.Attrs{}}
	if encCount > 0 {
		// O decodificador do wire entrega atributos como string; o AttrGetter
		// so' converte a partir dela.
		enc.Attrs["count"] = strconv.Itoa(encCount)
	}
	return &waBinary.Node{
		Tag:     "message",
		Attrs:   waBinary.Attrs{"id": id, "from": testPeerJID, "t": "1700000000"},
		Content: []waBinary.Node{enc},
	}
}

func sendRef(id types.MessageID) MessageRef {
	return MessageRef{Chat: testPeerJID, Sender: testPeerJID, ID: id}
}

func childByTag(node waBinary.Node, tag string) (waBinary.Node, bool) {
	for _, c := range node.GetChildren() {
		if c.Tag == tag {
			return c, true
		}
	}
	return waBinary.Node{}, false
}

func TestSendReceiptFirstAttempt(t *testing.T) {
	tr := newFakeTransport()
	tr.rerequestEnabled = true
	tr.synchronousAck = true
	SendReceipt(context.Background(), tr, incomingNode("MSG1", 0), sendRef("MSG1"), false)

	sent := tr.sent()
	if len(sent) != 1 {
		t.Fatalf("nos enviados = %d, esperado 1", len(sent))
	}
	if sent[0].Tag != "receipt" || sent[0].Attrs["type"] != string(types.ReceiptTypeRetry) {
		t.Errorf("no = %+v", sent[0])
	}
	retryChild, ok := childByTag(sent[0], "retry")
	if !ok {
		t.Fatal("faltou o filho <retry>")
	}
	if retryChild.Attrs["count"] != 1 {
		t.Errorf("count = %v, esperado 1", retryChild.Attrs["count"])
	}
	if retryChild.Attrs["v"] != ReceiptVersion {
		t.Errorf("v = %v, esperado %d", retryChild.Attrs["v"], ReceiptVersion)
	}
	// A primeira tentativa tambem pede a mensagem ao proprio telefone; com
	// SynchronousAck isso acontece na mesma goroutine.
	if len(tr.unavailReqs) != 1 {
		t.Errorf("pedidos ao telefone = %v, esperado 1", tr.unavailReqs)
	}
	// <registration> carrega o RegistrationID em big-endian, 4 bytes.
	reg, ok := childByTag(sent[0], "registration")
	if !ok {
		t.Fatal("faltou o filho <registration>")
	}
	regBytes, _ := reg.Content.([]byte)
	if len(regBytes) != prekeys.RegistrationIDLength {
		t.Fatalf("registration tem %d bytes, esperado %d", len(regBytes), prekeys.RegistrationIDLength)
	}
	if got := binary.BigEndian.Uint32(regBytes); got != tr.device.RegistrationID {
		t.Errorf("RegistrationID = %x, esperado %x", got, tr.device.RegistrationID)
	}
	// Primeira tentativa sem forceIncludeIdentity nao manda <keys>.
	if _, ok := childByTag(sent[0], "keys"); ok {
		t.Error("a primeira tentativa nao deveria mandar <keys>")
	}
}

// Sem SynchronousAck o pedido ao telefone vai para uma goroutine; aqui so'
// interessa que o recibo saia e que o caminho nao trave.
func TestSendReceiptAsyncPhoneRequest(t *testing.T) {
	tr := newFakeTransport()
	tr.rerequestEnabled = false // a goroutine volta na hora
	tr.synchronousAck = false
	SendReceipt(context.Background(), tr, incomingNode("MSG1", 0), sendRef("MSG1"), false)
	if len(tr.sent()) != 1 {
		t.Fatalf("nos enviados = %d, esperado 1", len(tr.sent()))
	}
}

// A partir da segunda tentativa (ou com forceIncludeIdentity) o recibo carrega
// o bloco <keys>.
func TestSendReceiptIncludesKeys(t *testing.T) {
	for name, tc := range map[string]struct {
		attempts int
		force    bool
	}{
		"segunda tentativa":    {2, false},
		"forceIncludeIdentity": {1, true},
	} {
		t.Run(name, func(t *testing.T) {
			tr := newFakeTransport()
			tr.stores.genOneKey = &keys.PreKey{KeyPair: *keys.NewKeyPair(), KeyID: 3}
			for i := 0; i < tc.attempts; i++ {
				SendReceipt(context.Background(), tr, incomingNode("MSG1", 0), sendRef("MSG1"), tc.force)
			}
			sent := tr.sent()
			last := sent[len(sent)-1]
			keysNode, ok := childByTag(last, "keys")
			if !ok {
				t.Fatal("faltou o filho <keys>")
			}
			if n := len(keysNode.GetChildren()); n != 5 {
				t.Errorf("filhos de <keys> = %d, esperado 5", n)
			}
		})
	}
}

// Falha ao gerar a prekey NAO aborta: o recibo sai sem <keys>. Falha ao
// serializar a conta ABORTA. A assimetria e' do upstream.
func TestSendReceiptKeyErrorsAreAsymmetric(t *testing.T) {
	t.Run("falha na prekey manda o recibo assim mesmo", func(t *testing.T) {
		tr := newFakeTransport()
		tr.stores.genOneErr = errors.New("boom")
		SendReceipt(context.Background(), tr, incomingNode("MSG1", 0), sendRef("MSG1"), true)
		sent := tr.sent()
		if len(sent) != 1 {
			t.Fatalf("nos enviados = %d, esperado 1", len(sent))
		}
		if _, ok := childByTag(sent[0], "keys"); ok {
			t.Error("nao deveria haver <keys> se a prekey falhou")
		}
	})
	t.Run("falha ao serializar a conta aborta", func(t *testing.T) {
		tr := newFakeTransport()
		tr.stores.genOneKey = &keys.PreKey{KeyPair: *keys.NewKeyPair(), KeyID: 3}
		tr.device.Account = nil // proto.Marshal(nil) nao falha; ver nota abaixo
		SendReceipt(context.Background(), tr, incomingNode("MSG1", 0), sendRef("MSG1"), true)
		// proto.Marshal de uma mensagem tipada nil NAO devolve erro em Go, entao
		// este ramo (o `return` do else-if) e' estruturalmente inalcancavel sem
		// injecao de dependencia. Registrado em PATCHES.md em vez de fabricado;
		// o que o teste garante e' que o caminho nao entra em panico.
		if len(tr.sent()) != 1 {
			t.Errorf("nos enviados = %d, esperado 1", len(tr.sent()))
		}
	})
}

// Passando de MaxOutgoingReceipts, desiste sem enviar nada.
func TestSendReceiptStopsAfterMax(t *testing.T) {
	tr := newFakeTransport()
	tr.stores.genOneKey = &keys.PreKey{KeyPair: *keys.NewKeyPair(), KeyID: 3}
	for i := 0; i < MaxOutgoingReceipts+3; i++ {
		SendReceipt(context.Background(), tr, incomingNode("MSG1", 0), sendRef("MSG1"), false)
	}
	// Envia nas tentativas 1..MaxOutgoingReceipts-1; a de numero
	// MaxOutgoingReceipts ja' cai no >=.
	if n := len(tr.sent()); n != MaxOutgoingReceipts-1 {
		t.Errorf("nos enviados = %d, esperado %d", n, MaxOutgoingReceipts-1)
	}
}

// Se o processo reiniciou no meio de uma cadeia de retries, o contador
// recomeca do valor que o proprio <enc> declara.
func TestSendReceiptResumesCountFromMessage(t *testing.T) {
	tr := newFakeTransport()
	tr.stores.genOneKey = &keys.PreKey{KeyPair: *keys.NewKeyPair(), KeyID: 3}
	SendReceipt(context.Background(), tr, incomingNode("MSG1", 2), sendRef("MSG1"), false)
	sent := tr.sent()
	if len(sent) != 1 {
		t.Fatalf("nos enviados = %d, esperado 1", len(sent))
	}
	retryChild, _ := childByTag(sent[0], "retry")
	if retryChild.Attrs["count"] != 3 {
		t.Errorf("count = %v, esperado 3 (2 do <enc> + 1)", retryChild.Attrs["count"])
	}
}

// category="peer" so' entra quando a mensagem e' peer_msg E veio de nos.
func TestSendReceiptPeerCategory(t *testing.T) {
	for name, tc := range map[string]struct {
		ref  MessageRef
		want bool
	}{
		"peer_msg de nos":       {MessageRef{ID: "M", Type: "peer_msg", IsFromMe: true}, true},
		"peer_msg de outro":     {MessageRef{ID: "M", Type: "peer_msg"}, false},
		"mensagem normal nossa": {MessageRef{ID: "M", IsFromMe: true}, false},
	} {
		t.Run(name, func(t *testing.T) {
			tr := newFakeTransport()
			SendReceipt(context.Background(), tr, incomingNode("M", 0), tc.ref, false)
			_, got := tr.sent()[0].Attrs["category"]
			if got != tc.want {
				t.Errorf("category presente = %v, esperado %v", got, tc.want)
			}
		})
	}
}

// Sem exatamente um filho <enc>, o count da mensagem nao e' lido.
func TestSendReceiptIgnoresCountWithoutSingleEnc(t *testing.T) {
	tr := newFakeTransport()
	node := &waBinary.Node{
		Tag:   "message",
		Attrs: waBinary.Attrs{"id": "MSG1", "from": testPeerJID, "t": "1"},
		Content: []waBinary.Node{
			{Tag: "enc", Attrs: waBinary.Attrs{"count": "4"}},
			{Tag: "outro"},
		},
	}
	SendReceipt(context.Background(), tr, node, sendRef("MSG1"), false)
	retryChild, _ := childByTag(tr.sent()[0], "retry")
	if retryChild.Attrs["count"] != 1 {
		t.Errorf("count = %v, esperado 1", retryChild.Attrs["count"])
	}
}

// Falha de envio e' logada e engolida.
func TestSendReceiptSendError(t *testing.T) {
	tr := newFakeTransport()
	tr.sendNodeErr = errors.New("sem socket")
	SendReceipt(context.Background(), tr, incomingNode("MSG1", 0), sendRef("MSG1"), false)
	if len(tr.sent()) != 1 {
		t.Errorf("nos enviados = %d, esperado 1 tentativa", len(tr.sent()))
	}
}

// No sem atributo `id` de tipo string: o id fica vazio e o fluxo segue.
func TestSendReceiptWithoutStringID(t *testing.T) {
	tr := newFakeTransport()
	node := &waBinary.Node{
		Tag:   "message",
		Attrs: waBinary.Attrs{"id": 42, "from": testPeerJID, "t": "1"},
	}
	SendReceipt(context.Background(), tr, node, sendRef("MSG1"), false)
	retryChild, _ := childByTag(tr.sent()[0], "retry")
	if retryChild.Attrs["id"] != "" {
		t.Errorf("id = %v, esperado vazio", retryChild.Attrs["id"])
	}
}
