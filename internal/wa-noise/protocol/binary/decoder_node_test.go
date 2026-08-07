package binary

import (
	"errors"
	"strings"
	"testing"

	"wa-api/internal/wa-noise/protocol/binary/token"
	"wa-api/internal/wa-noise/protocol/types"
)

func TestReadDispatchesByTag(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want interface{}
	}{
		{"lista vazia vira nil", []byte{token.ListEmpty}, nil},
		{"token de 1 byte", []byte{1}, token.SingleByteTokens[1]},
		{"binary8", []byte{token.Binary8, 0x02, 'o', 'i'}, "oi"},
		{"binary20", []byte{token.Binary20, 0x00, 0x00, 0x02, 'o', 'i'}, "oi"},
		{"binary32", []byte{token.Binary32, 0x00, 0x00, 0x00, 0x02, 'o', 'i'}, "oi"},
		{"nibble8", []byte{token.Nibble8, 0x01, 0x12}, "12"},
		{"hex8", []byte{token.Hex8, 0x01, 0xAB}, "AB"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := newDecoder(tc.data).read(true)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if got != tc.want {
				t.Errorf("= %v (%T), esperado %v", got, got, tc.want)
			}
		})
	}
}

// Os quatro dicionarios de dois bytes sao lidos pelo deslocamento em relacao a
// Dictionary0. Trocar o indice devolve a string errada silenciosamente, entao
// os quatro tem teste.
func TestReadResolvesEveryDoubleByteDictionary(t *testing.T) {
	for dict, tag := range []int{token.Dictionary0, token.Dictionary1, token.Dictionary2, token.Dictionary3} {
		got, err := newDecoder([]byte{byte(tag), 0x00}).read(true)
		if err != nil {
			t.Fatalf("dicionario %d: %v", dict, err)
		}
		want := token.DoubleByteTokens[dict][0]
		if got != want {
			t.Errorf("dicionario %d, indice 0 = %q, esperado %q", dict, got, want)
		}
	}
}

// A faixa 240-244 e' o buraco entre o fim dos dicionarios de dois bytes (239)
// e InteropJID (245): nenhuma tag do protocolo cai ali. E' a unica entrada que
// alcanca ErrInvalidToken, porque len(SingleByteTokens) e' exatamente 236, o
// mesmo valor de Dictionary0 — todo indice abaixo disso e' token valido e todo
// indice acima e' tratado por um case explicito.
func TestReadRejectsUnknownToken(t *testing.T) {
	for tag := byte(240); tag <= 244; tag++ {
		_, err := newDecoder([]byte{tag}).read(true)
		if !errors.Is(err, ErrInvalidToken) {
			t.Errorf("tag %d = %v, esperado ErrInvalidToken", tag, err)
		}
	}
}

func TestReadAttributesPairsKeysWithValues(t *testing.T) {
	back := mustRoundTrip(t, Node{Tag: "iq", Attrs: Attrs{"id": "abc", "type": "get", "xmlns": "w:p"}})
	for key, want := range map[string]string{"id": "abc", "type": "get", "xmlns": "w:p"} {
		if got := back.Attrs[key]; got != want {
			t.Errorf("atributo %q = %v, esperado %q", key, got, want)
		}
	}
	if len(back.Attrs) != 3 {
		t.Errorf("numero de atributos = %d, esperado 3", len(back.Attrs))
	}
}

func TestReadAttributesOfZeroReturnsNilMap(t *testing.T) {
	attrs, err := newDecoder(nil).readAttributes(0)
	if err != nil || attrs != nil {
		t.Errorf("= (%v, %v), esperado (nil, nil)", attrs, err)
	}
}

// Uma chave de atributo tem que ser string. Se o frame trouxer um JID ou uma
// lista na posicao da chave, o mapa de atributos nao pode ser montado.
func TestReadAttributesRejectsNonStringKey(t *testing.T) {
	// Lista vazia na posicao da chave: read devolve nil, que nao e' string.
	_, err := newDecoder([]byte{token.ListEmpty, token.ListEmpty}).readAttributes(1)
	if !errors.Is(err, ErrNonStringKey) {
		t.Errorf("= %v, esperado ErrNonStringKey", err)
	}
}

func TestReadAttributesPropagatesTruncation(t *testing.T) {
	if _, err := newDecoder(nil).readAttributes(1); err == nil {
		t.Error("chave truncada deveria falhar")
	}
	if _, err := newDecoder([]byte{1}).readAttributes(1); err == nil {
		t.Error("valor truncado deveria falhar")
	}
}

func TestReadListReadsEveryChild(t *testing.T) {
	children := []Node{{Tag: "a"}, {Tag: "b"}, {Tag: "c"}}
	back := mustRoundTrip(t, Node{Tag: "list", Content: children})
	got := back.GetChildren()
	if len(got) != len(children) {
		t.Fatalf("numero de filhos = %d, esperado %d", len(got), len(children))
	}
	for i, child := range got {
		if child.Tag != children[i].Tag {
			t.Errorf("filho %d = %q, esperado %q", i, child.Tag, children[i].Tag)
		}
	}
}

// Uma lista com mais de 255 itens usa List16. E' a fronteira entre as duas
// tags de lista e a unica que exercita readInt16 no caminho de lista.
func TestReadListCrossesTheList8ToList16Boundary(t *testing.T) {
	for _, size := range []int{1, token.SingleByteMax - 1, token.SingleByteMax, token.SingleByteMax + 1} {
		children := make([]Node, size)
		for i := range children {
			children[i] = Node{Tag: "item"}
		}
		back := mustRoundTrip(t, Node{Tag: "list", Content: children})
		if got := len(back.GetChildren()); got != size {
			t.Errorf("lista de %d itens voltou com %d", size, got)
		}
	}
}

func TestReadListPropagatesChildError(t *testing.T) {
	// List8 anunciando 1 filho, mas sem bytes para ele.
	if _, err := newDecoder([]byte{0x01}).readList(token.List8); err == nil {
		t.Error("filho truncado deveria falhar")
	}
	if _, err := newDecoder(nil).readList(token.List8); err == nil {
		t.Error("tamanho truncado deveria falhar")
	}
}

// O tamanho da lista de um no' e' impar quando ele nao tem conteudo e par
// quando tem. E' o unico sinal do formato para "tem filho ou nao".
func TestNodeListSizeParityDecidesContent(t *testing.T) {
	withContent := mustRoundTrip(t, Node{Tag: "x", Attrs: Attrs{"a": "1"}, Content: []byte{0x01}})
	if withContent.Content == nil {
		t.Error("no' com conteudo voltou sem conteudo")
	}
	withoutContent := mustRoundTrip(t, Node{Tag: "x", Attrs: Attrs{"a": "1"}})
	if withoutContent.Content != nil {
		t.Errorf("no' sem conteudo voltou com %v", withoutContent.Content)
	}
}

func TestReadNodeRejectsEmptyListAndEmptyTag(t *testing.T) {
	// Tamanho de lista 0 seguido de uma tag valida: o no' nao tem nem a
	// posicao da propria tag, entao nao e' um no'.
	if _, err := newDecoder([]byte{token.ListEmpty, 0x01}).readNode(); !errors.Is(err, ErrInvalidNode) {
		t.Errorf("lista vazia = %v, esperado ErrInvalidNode", err)
	}
	// Lista de 1 item cuja tag e' um bloco binary8 de tamanho zero: a
	// posicao da tag existe, mas a string e' vazia.
	//
	// E' o UNICO jeito de alcancar o ramo `ret.Tag == ""`. O token de
	// indice 0 de SingleByteTokens tambem e' a string vazia, mas o byte 0
	// e' ListEmpty e read() o intercepta antes, devolvendo nil — que nao
	// chega a virar string, entra no panic do achado F24 abaixo.
	if _, err := newDecoder([]byte{token.List8, 0x01, token.Binary8, 0x00}).readNode(); !errors.Is(err, ErrInvalidNode) {
		t.Error("tag vazia deveria devolver ErrInvalidNode")
	}
}

// ACHADO (registrado em HOUSEKEEP.md, F24): readNode faz `rawDesc.(string)`
// sem checar o ok. Quando o frame traz na posicao da tag algo que nao e'
// string — uma lista vazia (nil), um JID ou uma lista de nos — o decoder entra
// em PANIC em vez de devolver erro, com bytes vindos da rede.
//
// Este teste NAO conserta: trava o comportamento atual para que a assimetria
// fique explicita e para que consertar seja uma mudanca deliberada, com este
// teste virando a prova do fix. Mesmo tratamento que a Fase B deu ao panic
// latente de record.Session.
func TestReadNodePanicsOnNonStringTag(t *testing.T) {
	nonStringTags := map[string][]byte{
		"lista vazia (nil)": {token.List8, 0x01, token.ListEmpty},
		"JID":               {token.List8, 0x01, token.JIDPair, 0x01, 0x01},
	}
	for name, data := range nonStringTags {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("readNode nao entrou em panic — o comportamento mudou, atualize HOUSEKEEP F24")
				}
			}()
			_, _ = newDecoder(data).readNode()
		})
	}
}

// A mesma falta de checagem existe nos leitores de JID: readADJID, readFBJID e
// readInteropJID fazem `user.(string)`. Mesmo achado F24.
func TestJIDReadersPanicOnNonStringUser(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("readADJID nao entrou em panic — atualize HOUSEKEEP F24")
		}
	}()
	// agente, device e, na posicao do user, uma lista vazia.
	_, _ = newDecoder([]byte{0x00, 0x00, token.ListEmpty}).readADJID()
}

func TestReadNodePropagatesTruncationAtEveryStage(t *testing.T) {
	truncated := [][]byte{
		{},                              // sem o byte de tag de lista
		{token.List8},                   // sem o tamanho
		{token.List8, 0x02},             // sem a tag
		{token.List8, 0x04, 0x01},       // sem os atributos
		{token.List8, 0x02, 0x01},       // sem o conteudo
		{token.List8, 0x02, 0x01, 0xF8}, // conteudo com lista truncada
	}
	for i, data := range truncated {
		if _, err := newDecoder(data).readNode(); err == nil {
			t.Errorf("caso %d (%x) deveria falhar", i, data)
		}
	}
}

// Unmarshal exige que o frame termine exatamente no fim do no'. Bytes
// sobrando sao sinal de frame corrompido ou de dois frames colados.
func TestUnmarshalRejectsLeftoverBytes(t *testing.T) {
	data := mustMarshal(t, Node{Tag: "x"})[1:]
	n, err := Unmarshal(append(data, 0x00))
	if err == nil || !strings.Contains(err.Error(), "leftover") {
		t.Errorf("= %v, esperado erro de bytes sobrando", err)
	}
	if n == nil {
		t.Error("Unmarshal deveria devolver o no' lido junto com o erro")
	}
}

func TestUnmarshalReadsNestedStructure(t *testing.T) {
	original := Node{
		Tag:   "iq",
		Attrs: Attrs{"id": "1", "to": types.ServerJID},
		Content: []Node{
			{Tag: "list", Content: []Node{
				{Tag: "item", Attrs: Attrs{"jid": types.NewJID("5511999999999", types.DefaultUserServer)}},
			}},
		},
	}
	back := mustRoundTrip(t, original)
	item := back.GetChildByTag("list", "item")
	if item.Tag != "item" {
		t.Fatalf("no' aninhado nao foi encontrado: %+v", back)
	}
	if got := item.Attrs["jid"].(types.JID).User; got != "5511999999999" {
		t.Errorf("jid do item = %q", got)
	}
}
