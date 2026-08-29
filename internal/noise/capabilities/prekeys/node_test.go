package prekeys

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/security/keys"
)

func childContent(t *testing.T, node waBinary.Node, tag string) []byte {
	t.Helper()
	content, ok := node.GetChildByTag(tag).Content.([]byte)
	if !ok {
		t.Fatalf("filho %q ausente ou com conteudo inesperado em %s", tag, node.XMLString())
	}
	return content
}

// O key ID vai para o wire truncado em 24 bits: o byte mais significativo do
// uint32 e' descartado. Se isso mudar, o servidor deixa de reconhecer as chaves.
func TestPreKeyToNodeWritesA24BitKeyID(t *testing.T) {
	node := ToNode(testPreKey(t, 0x00ABCDEF, false))

	if node.Tag != "key" {
		t.Errorf("tag = %q, esperado \"key\"", node.Tag)
	}
	idBytes := childContent(t, node, "id")
	if len(idBytes) != idLength {
		t.Fatalf("len(id) = %d, esperado %d", len(idBytes), idLength)
	}
	if want := []byte{0xAB, 0xCD, 0xEF}; !bytes.Equal(idBytes, want) {
		t.Errorf("id = %x, esperado %x (big-endian, sem o byte mais alto)", idBytes, want)
	}
	if got := childContent(t, node, "value"); len(got) != pubLength {
		t.Errorf("len(value) = %d, esperado %d", got, pubLength)
	}
	if _, ok := node.GetOptionalChildByTag("signature"); ok {
		t.Error("prekey sem assinatura nao deveria ter no <signature>")
	}
}

// Uma prekey assinada vira <skey> com o no de assinatura; uma comum vira <key>.
// Os dois casos sao lidos de volta por nodeToPreKey, que decide pela tag.
func TestPreKeyToNodeSignedUsesSkeyTag(t *testing.T) {
	node := ToNode(testPreKey(t, 1, true))

	if node.Tag != "skey" {
		t.Errorf("tag = %q, esperado \"skey\"", node.Tag)
	}
	if got := childContent(t, node, "signature"); len(got) != signatureLength {
		t.Errorf("len(signature) = %d, esperado %d", len(got), signatureLength)
	}
}

// Round trip: o que preKeyToNode escreve, nodeToPreKey le de volta identico.
// E' a garantia de que os dois lados do corte de 24 bits concordam.
func TestPreKeyNodeRoundTrip(t *testing.T) {
	for name, tc := range map[string]struct {
		keyID  uint32
		signed bool
	}{
		"comum":             {keyID: 42, signed: false},
		"assinada":          {keyID: 7, signed: true},
		"id maximo 24 bits": {keyID: 0xFFFFFF, signed: false},
		"id zero":           {keyID: 0, signed: true},
	} {
		t.Run(name, func(t *testing.T) {
			original := testPreKey(t, tc.keyID, tc.signed)

			decoded, err := NodeToPreKey(ToNode(original))
			if err != nil {
				t.Fatalf("nodeToPreKey: %v", err)
			}
			if decoded.KeyID != tc.keyID {
				t.Errorf("KeyID = %d, esperado %d", decoded.KeyID, tc.keyID)
			}
			if *decoded.Pub != *original.Pub {
				t.Error("chave publica nao sobreviveu ao round trip")
			}
			if tc.signed {
				if decoded.Signature == nil || *decoded.Signature != *original.Signature {
					t.Error("assinatura nao sobreviveu ao round trip")
				}
			} else if decoded.Signature != nil {
				t.Error("prekey comum voltou com assinatura")
			}
		})
	}
}

// Um key ID acima de 24 bits perde silenciosamente o byte alto no wire. Nao e'
// bug alcancavel (o store nunca gera IDs tao altos), mas o comportamento fica
// travado para que uma mudanca em idLength seja consciente.
func TestPreKeyIDAbove24BitsIsTruncated(t *testing.T) {
	decoded, err := NodeToPreKey(ToNode(testPreKey(t, 0xFF000001, false)))
	if err != nil {
		t.Fatalf("nodeToPreKey: %v", err)
	}
	if decoded.KeyID != 0x000001 {
		t.Errorf("KeyID = %#x, esperado %#x", decoded.KeyID, 0x000001)
	}
}

// Todo campo de tamanho fixo e' validado antes de virar array. Sem essas
// guardas, uma conversao *(*[N]byte) sobre slice curto entraria em panico —
// e este parser roda sobre dado vindo do servidor.
func TestNodeToPreKeyRejectsMalformedNodes(t *testing.T) {
	valid := func() waBinary.Node { return ToNode(testPreKey(t, 1, true)) }
	replaceChild := func(node waBinary.Node, tag string, content any) waBinary.Node {
		children := append([]waBinary.Node(nil), node.GetChildren()...)
		for i := range children {
			if children[i].Tag == tag {
				children[i].Content = content
			}
		}
		node.Content = children
		return node
	}
	dropChild := func(node waBinary.Node, tag string) waBinary.Node {
		var children []waBinary.Node
		for _, child := range node.GetChildren() {
			if child.Tag != tag {
				children = append(children, child)
			}
		}
		node.Content = children
		return node
	}

	for name, tc := range map[string]struct {
		node    waBinary.Node
		wantErr string
	}{
		"sem no de id":          {dropChild(valid(), "id"), "doesn't contain ID tag"},
		"id nao e bytes":        {replaceChild(valid(), "id", "abc"), "unexpected content"},
		"id curto":              {replaceChild(valid(), "id", []byte{1, 2}), "unexpected number of bytes"},
		"id longo":              {replaceChild(valid(), "id", []byte{1, 2, 3, 4}), "unexpected number of bytes"},
		"sem no de value":       {dropChild(valid(), "value"), "doesn't contain value tag"},
		"value nao e bytes":     {replaceChild(valid(), "value", 5), "unexpected content"},
		"value curto":           {replaceChild(valid(), "value", []byte{1}), "unexpected number of bytes"},
		"sem no de signature":   {dropChild(valid(), "signature"), "doesn't contain signature tag"},
		"signature nao e bytes": {replaceChild(valid(), "signature", "x"), "unexpected content"},
		"signature curta":       {replaceChild(valid(), "signature", []byte{1, 2, 3}), "unexpected number of bytes"},
	} {
		t.Run(name, func(t *testing.T) {
			key, err := NodeToPreKey(tc.node)
			if err == nil {
				t.Fatalf("aceitou no malformado, devolveu %+v", key)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("erro = %q, esperado conter %q", err, tc.wantErr)
			}
		})
	}
}

// Uma prekey comum (tag "key") sem no <signature> e' valida: o parser so' exige
// assinatura quando a tag e' "skey".
func TestNodeToPreKeyPlainKeyDoesNotRequireSignature(t *testing.T) {
	key, err := NodeToPreKey(ToNode(testPreKey(t, 9, false)))
	if err != nil {
		t.Fatalf("nodeToPreKey: %v", err)
	}
	if key.Signature != nil {
		t.Error("assinatura deveria ser nil")
	}
}

func TestPreKeysToNodesPreservesOrder(t *testing.T) {
	input := []*keys.PreKey{testPreKey(t, 1, false), testPreKey(t, 2, false), testPreKey(t, 3, false)}

	nodes := ToNodes(input)

	if len(nodes) != len(input) {
		t.Fatalf("len = %d, esperado %d", len(nodes), len(input))
	}
	for i, node := range nodes {
		idBytes := childContent(t, node, "id")
		got := binary.BigEndian.Uint32(append(make([]byte, idPadLength), idBytes...))
		if got != input[i].KeyID {
			t.Errorf("nodes[%d].id = %d, esperado %d", i, got, input[i].KeyID)
		}
	}
}

func TestPreKeysToNodesEmptyIsNonNil(t *testing.T) {
	if nodes := ToNodes(nil); nodes == nil || len(nodes) != 0 {
		t.Errorf("= %v, esperado slice vazio nao-nil", nodes)
	}
}

// nodeToPreKeyBundle e' o parser da resposta de <iq> de prekeys de outro
// dispositivo. Monta um no completo e confere o bundle resultante.
func preKeyBundleNode(t *testing.T, registrationID uint32, withPreKey bool) waBinary.Node {
	t.Helper()
	var regBytes [RegistrationIDLength]byte
	binary.BigEndian.PutUint32(regBytes[:], registrationID)
	children := []waBinary.Node{
		{Tag: "registration", Content: regBytes[:]},
		{Tag: "identity", Content: bytes.Repeat([]byte{0x11}, pubLength)},
		ToNode(testPreKey(t, 77, true)),
	}
	if withPreKey {
		children = append(children, ToNode(testPreKey(t, 33, false)))
	}
	return waBinary.Node{Tag: "user", Content: children}
}

func TestNodeToPreKeyBundle(t *testing.T) {
	const deviceID = 3

	t.Run("com prekey opcional", func(t *testing.T) {
		bundle, err := NodeToBundle(deviceID, preKeyBundleNode(t, 12345, true))
		if err != nil {
			t.Fatalf("nodeToPreKeyBundle: %v", err)
		}
		if bundle.RegistrationID() != 12345 {
			t.Errorf("RegistrationID = %d, esperado 12345", bundle.RegistrationID())
		}
		if bundle.DeviceID() != deviceID {
			t.Errorf("DeviceID = %d, esperado %d", bundle.DeviceID(), deviceID)
		}
		if !bundle.PreKeyID().IsEmpty && bundle.PreKeyID().Value != 33 {
			t.Errorf("PreKeyID = %d, esperado 33", bundle.PreKeyID().Value)
		}
		if bundle.SignedPreKeyID() != 77 {
			t.Errorf("SignedPreKeyID = %d, esperado 77", bundle.SignedPreKeyID())
		}
		if bundle.PreKey() == nil {
			t.Error("PreKey deveria estar presente")
		}
	})

	t.Run("sem prekey opcional", func(t *testing.T) {
		bundle, err := NodeToBundle(deviceID, preKeyBundleNode(t, 1, false))
		if err != nil {
			t.Fatalf("nodeToPreKeyBundle: %v", err)
		}
		if !bundle.PreKeyID().IsEmpty {
			t.Error("PreKeyID deveria ser vazio quando nao ha <key>")
		}
		if bundle.PreKey() != nil {
			t.Error("PreKey deveria ser nil")
		}
	})
}

func TestNodeToPreKeyBundleRejectsMalformedResponses(t *testing.T) {
	withoutChild := func(tag string) waBinary.Node {
		node := preKeyBundleNode(t, 1, true)
		var children []waBinary.Node
		for _, child := range node.GetChildren() {
			if child.Tag != tag {
				children = append(children, child)
			}
		}
		node.Content = children
		return node
	}

	for name, tc := range map[string]struct {
		node    waBinary.Node
		wantErr string
	}{
		"no de erro do servidor": {
			waBinary.Node{Tag: "user", Content: []waBinary.Node{{Tag: "error", Attrs: waBinary.Attrs{"code": "404"}}}},
			"got error getting prekeys",
		},
		"prekey opcional malformada": {
			func() waBinary.Node {
				node := preKeyBundleNode(t, 1, true)
				children := append([]waBinary.Node(nil), node.GetChildren()...)
				for i := range children {
					if children[i].Tag == "key" {
						children[i].Content = []waBinary.Node{{Tag: "id", Content: []byte{1}}}
					}
				}
				node.Content = children
				return node
			}(),
			"invalid prekey in prekey response",
		},
		"sem registration":  {withoutChild("registration"), "invalid registration ID"},
		"sem identity":      {withoutChild("identity"), "invalid identity key"},
		"sem signed prekey": {withoutChild("skey"), "invalid signed prekey"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NodeToBundle(1, tc.node); err == nil {
				t.Fatal("aceitou resposta malformada")
			} else if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("erro = %q, esperado conter %q", err, tc.wantErr)
			}
		})
	}
}

// O servidor pode aninhar as chaves num <keys>; sem ele, os filhos ficam no
// proprio no do usuario. Os dois formatos precisam funcionar.
func TestNodeToPreKeyBundleAcceptsNestedKeysNode(t *testing.T) {
	flat := preKeyBundleNode(t, 999, true)
	var registration waBinary.Node
	var rest []waBinary.Node
	for _, child := range flat.GetChildren() {
		if child.Tag == "registration" {
			registration = child
		} else {
			rest = append(rest, child)
		}
	}
	nested := waBinary.Node{Tag: "user", Content: []waBinary.Node{
		registration,
		{Tag: "keys", Content: rest},
	}}

	bundle, err := NodeToBundle(1, nested)
	if err != nil {
		t.Fatalf("nodeToPreKeyBundle: %v", err)
	}
	if bundle.RegistrationID() != 999 {
		t.Errorf("RegistrationID = %d, esperado 999", bundle.RegistrationID())
	}
}

// As duas constantes de politica sao lidas juntas em uploadPreKeys: o upload
// inicial precisa ser muito maior que o lote regular, e o lote regular maior
// que o limiar que dispara um novo upload.
func TestPreKeyCountPolicyIsCoherent(t *testing.T) {
	if MinCount >= WantedCount {
		t.Errorf("MinCount (%d) deveria ser menor que WantedCount (%d)", MinCount, WantedCount)
	}
	if InitialCount <= WantedCount {
		t.Errorf("InitialCount (%d) deveria ser maior que WantedCount (%d)", InitialCount, WantedCount)
	}
}

func TestFetchNoErrorWithNoDevicesDoesNotTouchTheSocket(t *testing.T) {
	// Transport nil de proposito: se a guarda de lista vazia sumir, o teste
	// entra em panico em vez de passar silenciosamente.
	if got := FetchNoError(t.Context(), nil, nil); got != nil {
		t.Errorf("= %v, esperado nil", got)
	}
}
