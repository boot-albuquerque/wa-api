package binary

import (
	"bytes"
	"testing"

	"wa-api/internal/wa-noise/binary/token"
	"wa-api/internal/wa-noise/types"
)

// A tag "0" nao e' um no': o encoder a substitui por uma lista de um item
// vazio. E' o marcador que o protocolo usa para "nada aqui".
func TestEmptyNodeTagBecomesEmptyList(t *testing.T) {
	got := mustMarshal(t, Node{Tag: emptyNodeTag, Attrs: Attrs{"ignorado": "x"}, Content: []byte("tambem ignorado")})
	want := []byte{0, token.List8, token.ListEmpty}
	if !bytes.Equal(got, want) {
		t.Errorf("= %x, esperado %x", got, want)
	}
}

// O tamanho da lista que writeNode escreve tem que bater exatamente com o que
// writeAttributes de fato escreve. Um par a mais ou a menos desalinha todo o
// resto do frame, e o erro so' aparece no servidor.
func TestNodeListSizeMatchesWhatIsWritten(t *testing.T) {
	tests := []struct {
		name string
		node Node
		size int
	}{
		{"so tag", Node{Tag: "x"}, tagSize},
		{"tag e conteudo", Node{Tag: "x", Content: []byte{1}}, tagSize + 1},
		{"tag e 1 atributo", Node{Tag: "x", Attrs: Attrs{"a": "1"}}, tagSize + attrEntrySize},
		{"tag, 2 atributos e conteudo", Node{Tag: "x", Attrs: Attrs{"a": "1", "b": "2"}, Content: []byte{1}}, tagSize + 2*attrEntrySize + 1},
		{"atributo vazio nao conta", Node{Tag: "x", Attrs: Attrs{"a": "1", "b": ""}}, tagSize + attrEntrySize},
		{"atributo nil nao conta", Node{Tag: "x", Attrs: Attrs{"a": "1", "b": nil}}, tagSize + attrEntrySize},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := mustMarshal(t, tc.node)[1:]
			if raw[0] != token.List8 {
				t.Fatalf("esperado List8, obtido %d", raw[0])
			}
			if int(raw[1]) != tc.size {
				t.Errorf("tamanho de lista = %d, esperado %d", raw[1], tc.size)
			}
			// E o frame tem que ser lido de volta inteiro, sem sobra.
			if _, err := Unmarshal(raw); err != nil {
				t.Errorf("frame nao releu limpo: %v", err)
			}
		})
	}
}

// writeAttributes e countAttributes tem que filtrar exatamente os mesmos
// atributos. Este teste amarra os dois: se so' um deles mudar, o Unmarshal
// acima falha e a contagem abaixo tambem.
func TestEmptyAndNilAttributesAreDropped(t *testing.T) {
	back := mustRoundTrip(t, Node{Tag: "x", Attrs: Attrs{
		"mantido": "valor",
		"vazio":   "",
		"nulo":    nil,
	}})
	if len(back.Attrs) != 1 {
		t.Errorf("atributos = %v, esperado so' 'mantido'", back.Attrs)
	}
	if back.Attrs["mantido"] != "valor" {
		t.Errorf("atributo mantido = %v", back.Attrs["mantido"])
	}
}

// O zero de cada tipo numerico NAO e' filtrado — so' string vazia e nil sao.
// E' uma distincao facil de quebrar ao mexer no filtro, e um t=0 sumindo do
// frame mudaria o significado da mensagem.
func TestZeroValuedNumericAttributesSurvive(t *testing.T) {
	back := mustRoundTrip(t, Node{Tag: "x", Attrs: Attrs{"n": 0}})
	if got := back.Attrs["n"]; got != "0" {
		t.Errorf("atributo 0 = %v (%T), esperado \"0\"", got, got)
	}
	back = mustRoundTrip(t, Node{Tag: "x", Attrs: Attrs{"b": false}})
	if got := back.Attrs["b"]; got != "false" {
		t.Errorf("atributo false = %v, esperado \"false\"", got)
	}
}

// Todo tipo numerico vira a MESMA string decimal. O decoder devolve string, e
// e' o AttrUtility que reconverte — por isso o encoder nao precisa preservar o
// tipo, mas precisa preservar o valor exato, inclusive nos extremos.
func TestWriteConvertsEveryNumericTypeToDecimalString(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		want  string
	}{
		{"int", 42, "42"},
		{"int negativo", -42, "-42"},
		{"int32 minimo", int32(-2147483648), "-2147483648"},
		{"int64 maximo", int64(9223372036854775807), "9223372036854775807"},
		{"int64 minimo", int64(-9223372036854775808), "-9223372036854775808"},
		{"uint", uint(7), "7"},
		{"uint32 maximo", uint32(4294967295), "4294967295"},
		{"uint64 maximo", uint64(18446744073709551615), "18446744073709551615"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"string", "texto", "texto"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			back := mustRoundTrip(t, Node{Tag: "x", Attrs: Attrs{"v": tc.value}})
			if got := back.Attrs["v"]; got != tc.want {
				t.Errorf("= %v (%T), esperado %q", got, got, tc.want)
			}
		})
	}
}

func TestWriteNilBecomesEmptyList(t *testing.T) {
	w := newEncoder()
	w.write(nil)
	if got := w.getData()[1]; got != token.ListEmpty {
		t.Errorf("= %d, esperado ListEmpty (%d)", got, token.ListEmpty)
	}
}

func TestWritePanicsOnUnsupportedType(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("tipo nao suportado deveria entrar em panic")
		}
		if err, ok := r.(error); !ok || !bytes.Contains([]byte(err.Error()), []byte(ErrInvalidType.Error())) {
			t.Errorf("panic = %v, esperado envolver ErrInvalidType", r)
		}
	}()
	w := newEncoder()
	w.write(3.14)
}

// writeString escolhe a representacao mais curta. A ordem importa: um token
// conhecido nunca pode virar bloco empacotado, senao o frame cresce e deixa de
// bater com o que o servidor espera.
func TestWriteStringPrefersTheShortestRepresentation(t *testing.T) {
	tests := []struct {
		name  string
		value string
		first byte
		size  int
	}{
		{"token de 1 byte", "type", 4, 1},
		{"token de 2 bytes", "reaction", token.Dictionary0, 2},
		{"numerico vira nibble8", "5511999", token.Nibble8, 6},
		{"hex vira hex8", "ABCDEF", token.Hex8, 5},
		{"resto vira bloco cru", "texto qualquer", token.Binary8, 16},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := newEncoder()
			w.writeString(tc.value)
			raw := w.getData()[1:]
			if raw[0] != tc.first {
				t.Errorf("primeiro byte = %d, esperado %d", raw[0], tc.first)
			}
			if len(raw) != tc.size {
				t.Errorf("escreveu %d bytes, esperado %d", len(raw), tc.size)
			}
		})
	}
}

// "0" e' ao mesmo tempo um token conhecido e uma string nibble8 valida. O
// token ganha — e' o caso que prova a ordem das clausulas de writeString.
func TestSingleByteTokenWinsOverPacking(t *testing.T) {
	w := newEncoder()
	w.writeString("0")
	if got := len(w.getData()[1:]); got != 1 {
		t.Errorf("\"0\" produziu %d bytes, esperado 1 (token)", got)
	}
}

func TestWriteNodeListSerializesEveryChild(t *testing.T) {
	back := mustRoundTrip(t, Node{Tag: "root", Content: []Node{
		{Tag: "a", Attrs: Attrs{"i": 1}},
		{Tag: "b", Content: []byte{0xFF}},
		{Tag: "c", Content: []Node{{Tag: "d"}}},
	}})
	children := back.GetChildren()
	if len(children) != 3 {
		t.Fatalf("filhos = %d, esperado 3", len(children))
	}
	if children[0].Attrs["i"] != "1" {
		t.Errorf("filho a: atributo = %v", children[0].Attrs["i"])
	}
	if content, ok := children[1].Content.([]byte); !ok || !bytes.Equal(content, []byte{0xFF}) {
		t.Errorf("filho b: conteudo = %v", children[1].Content)
	}
	if got := children[2].GetChildByTag("d").Tag; got != "d" {
		t.Errorf("filho c: neto = %q", got)
	}
}

func TestWriteJIDGoesThroughTheTypeSwitch(t *testing.T) {
	// types.JID tem que ser reconhecido pelo write, senao cairia no panic
	// de tipo nao suportado.
	w := newEncoder()
	w.write(types.ServerJID)
	if got := w.getData()[1]; got != token.JIDPair {
		t.Errorf("primeiro byte = %d, esperado JIDPair (%d)", got, token.JIDPair)
	}
}

func TestNilAttrsMapProducesNodeWithoutAttributes(t *testing.T) {
	back := mustRoundTrip(t, Node{Tag: "x", Attrs: nil})
	if len(back.Attrs) != 0 {
		t.Errorf("atributos = %v, esperado nenhum", back.Attrs)
	}
}
