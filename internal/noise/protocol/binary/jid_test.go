package binary

import (
	"testing"

	"wa-api/internal/noise/protocol/binary/token"
	"wa-api/internal/noise/protocol/types"
)

// encodeJID serializa um JID isolado e devolve os bytes, sem o envelope de
// Node. A primeira posicao e' a tag de formato que writeJID escolheu.
func encodeJID(jid types.JID) []byte {
	w := newEncoder()
	w.writeJID(jid)
	return w.getData()[1:]
}

// TestWriteJIDPicksFormatByServerAndDevice trava a tabela de decisao de
// writeJID. E' a parte mais fragil do arquivo: escolher a tag errada nao
// quebra a serializacao, produz um JID que o servidor le' como outro usuario.
func TestWriteJIDPicksFormatByServerAndDevice(t *testing.T) {
	tests := []struct {
		name string
		jid  types.JID
		tag  byte
	}{
		{"usuario comum sem device", types.NewJID("5511999999999", types.DefaultUserServer), token.JIDPair},
		{"usuario comum com device", types.JID{User: "5511999999999", Device: 4, Server: types.DefaultUserServer}, token.ADJID},
		{"lid sem device", types.NewJID("123", types.HiddenUserServer), token.JIDPair},
		{"lid com device", types.JID{User: "123", Device: 4, Server: types.HiddenUserServer}, token.ADJID},
		{"hosted sempre AD", types.NewJID("123", types.HostedServer), token.ADJID},
		{"hosted lid sempre AD", types.NewJID("123", types.HostedLIDServer), token.ADJID},
		{"grupo", types.NewJID("123", types.GroupServer), token.JIDPair},
		{"broadcast", types.NewJID("status", types.BroadcastServer), token.JIDPair},
		{"newsletter", types.NewJID("123", types.NewsletterServer), token.JIDPair},
		{"messenger", types.JID{User: "9", Device: 1, Server: types.MessengerServer}, token.FBJID},
		{"interop", types.JID{User: "9", Device: 1, Integrator: 7, Server: types.InteropServer}, token.InteropJID},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := encodeJID(tc.jid)[0]; got != tc.tag {
				t.Errorf("tag de formato = %d, esperado %d", got, tc.tag)
			}
		})
	}
}

// TestJIDRoundTripsThroughEveryFormat prova que cada formato volta com todos
// os campos que o formato carrega.
func TestJIDRoundTripsThroughEveryFormat(t *testing.T) {
	tests := []struct {
		name string
		jid  types.JID
	}{
		{"par simples", types.NewJID("5511999999999", types.DefaultUserServer)},
		{"par com user vazio", types.NewJID("", types.BroadcastServer)},
		{"grupo", types.NewJID("120363000000000000", types.GroupServer)},
		{"AD em s.whatsapp.net", types.JID{User: "5511999999999", Device: 12, Server: types.DefaultUserServer}},
		{"AD em lid", types.JID{User: "123456", Device: 3, Server: types.HiddenUserServer}},
		{"AD em hosted", types.JID{User: "123456", Server: types.HostedServer}},
		{"AD em hosted.lid", types.JID{User: "123456", Server: types.HostedLIDServer}},
		{"messenger", types.JID{User: "999", Device: 3, Server: types.MessengerServer}},
		{"interop", types.JID{User: "999", Device: 3, Integrator: 42, Server: types.InteropServer}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			back := mustRoundTrip(t, Node{Tag: "x", Attrs: Attrs{"jid": tc.jid}})
			got, ok := back.Attrs["jid"].(types.JID)
			if !ok {
				t.Fatalf("atributo voltou como %T, esperado types.JID: %+v", back.Attrs["jid"], back.Attrs["jid"])
			}
			if got.String() != tc.jid.String() {
				t.Errorf("round trip: esperado %s, obtido %s", tc.jid, got)
			}
			if got.Integrator != tc.jid.Integrator {
				t.Errorf("Integrator: esperado %d, obtido %d", tc.jid.Integrator, got.Integrator)
			}
		})
	}
}

// O formato AD carrega o dominio no lugar do agente, e o decoder o traduz de
// volta para o servidor. E' por isso que um JID lid com device volta como lid
// e nao como s.whatsapp.net.
func TestADJIDCarriesDomainNotServerString(t *testing.T) {
	jid := types.JID{User: "123456", Device: 3, Server: types.HiddenUserServer}
	raw := encodeJID(jid)
	if raw[1] != types.LIDDomain {
		t.Errorf("byte de dominio = %d, esperado LIDDomain (%d)", raw[1], types.LIDDomain)
	}
	if raw[2] != byte(jid.Device) {
		t.Errorf("byte de device = %d, esperado %d", raw[2], jid.Device)
	}
	back := mustRoundTrip(t, Node{Tag: "x", Attrs: Attrs{"jid": jid}})
	if got := back.Attrs["jid"].(types.JID); got.Server != types.HiddenUserServer {
		t.Errorf("servidor apos round trip = %q, esperado %q", got.Server, types.HiddenUserServer)
	}
}

// Um par sem user vira token.ListEmpty no lugar do user, nao uma string
// vazia. Sao bytes diferentes e o decoder distingue os dois.
func TestJIDPairWithoutUserWritesEmptyList(t *testing.T) {
	raw := encodeJID(types.NewJID("", types.GroupServer))
	if raw[0] != token.JIDPair {
		t.Fatalf("esperado JIDPair, obtido %d", raw[0])
	}
	if raw[1] != token.ListEmpty {
		t.Errorf("user vazio deveria virar ListEmpty (%d), obtido %d", token.ListEmpty, raw[1])
	}
}

func TestReadJIDPairRejectsMissingServer(t *testing.T) {
	// Par cujo servidor veio como lista vazia: sem servidor nao ha JID.
	r := newDecoder([]byte{token.ListEmpty, token.ListEmpty})
	if _, err := r.readJIDPair(); err != ErrInvalidJIDType {
		t.Errorf("esperado ErrInvalidJIDType, obtido %v", err)
	}
}

// Os formatos FBJID e InteropJID carregam o servidor no fim do frame e o
// decoder o confere. Um servidor divergente e' frame corrompido, nao um JID de
// outro servidor: aceitar produziria um JID silenciosamente errado.
func TestFBAndInteropRejectMismatchedServer(t *testing.T) {
	w := newEncoder()
	w.write("999")
	w.pushInt16(1)
	w.write(types.GroupServer)
	payload := w.getData()[1:]

	if _, err := newDecoder(payload).readFBJID(); err == nil {
		t.Error("readFBJID aceitou servidor que nao e' msgr")
	}

	w = newEncoder()
	w.write("999")
	w.pushInt16(1)
	w.pushInt16(2)
	w.write(types.GroupServer)
	if _, err := newDecoder(w.getData()[1:]).readInteropJID(); err == nil {
		t.Error("readInteropJID aceitou servidor que nao e' interop")
	}
}

func TestJIDReadersPropagateTruncation(t *testing.T) {
	readers := map[string]func(*binaryDecoder) (interface{}, error){
		"JIDPair":    (*binaryDecoder).readJIDPair,
		"ADJID":      (*binaryDecoder).readADJID,
		"FBJID":      (*binaryDecoder).readFBJID,
		"InteropJID": (*binaryDecoder).readInteropJID,
	}
	for name, read := range readers {
		t.Run(name, func(t *testing.T) {
			if _, err := read(newDecoder(nil)); err == nil {
				t.Error("leitura de buffer vazio deveria falhar")
			}
		})
	}
}
