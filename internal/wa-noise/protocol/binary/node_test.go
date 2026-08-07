package binary

import (
	"bytes"
	"encoding/json"
	"testing"

	"wa-api/internal/wa-noise/protocol/types"
)

func TestGetChildrenOnlyReturnsNodeLists(t *testing.T) {
	if got := (&Node{Tag: "x"}).GetChildren(); got != nil {
		t.Errorf("conteudo nil = %v, esperado nil", got)
	}
	if got := (&Node{Tag: "x", Content: []byte{1, 2}}).GetChildren(); got != nil {
		t.Errorf("conteudo binario = %v, esperado nil", got)
	}
	n := &Node{Tag: "x", Content: []Node{{Tag: "a"}}}
	if got := n.GetChildren(); len(got) != 1 || got[0].Tag != "a" {
		t.Errorf("= %v", got)
	}
}

func TestGetChildrenByTagFilters(t *testing.T) {
	n := &Node{Content: []Node{{Tag: "a"}, {Tag: "b"}, {Tag: "a"}}}
	if got := n.GetChildrenByTag("a"); len(got) != 2 {
		t.Errorf("filhos 'a' = %d, esperado 2", len(got))
	}
	if got := n.GetChildrenByTag("z"); got != nil {
		t.Errorf("tag inexistente = %v, esperado nil", got)
	}
}

// GetOptionalChildByTag desce um nivel por tag. E' como todo o resto do
// whatsmeow navega a arvore, e o ok=false tem que distinguir "nao achei" de
// "achei um no' vazio".
func TestGetOptionalChildByTagRecursesPerTag(t *testing.T) {
	root := &Node{Tag: "iq", Content: []Node{
		{Tag: "list", Content: []Node{
			{Tag: "item", Attrs: Attrs{"n": "1"}},
		}},
	}}

	item, ok := root.GetOptionalChildByTag("list", "item")
	if !ok || item.Attrs["n"] != "1" {
		t.Errorf("= (%+v, %v)", item, ok)
	}

	if _, ok := root.GetOptionalChildByTag("list", "ausente"); ok {
		t.Error("tag ausente no ultimo nivel devolveu ok=true")
	}
	if _, ok := root.GetOptionalChildByTag("ausente", "item"); ok {
		t.Error("tag ausente no primeiro nivel devolveu ok=true")
	}

	// Sem tags nenhuma, devolve o proprio no'.
	self, ok := root.GetOptionalChildByTag()
	if !ok || self.Tag != "iq" {
		t.Errorf("sem tags = (%+v, %v), esperado o proprio no'", self, ok)
	}
}

// ARMADILHA (registrada em HOUSEKEEP.md, F26): quando nao acha, GetChildByTag
// NAO devolve um Node zero — devolve o ULTIMO no' que conseguiu alcancar, que
// para uma unica tag e' o proprio no' de partida. GetOptionalChildByTag usa
// retorno nomeado e faz `return` nu' com val ja' atribuido.
//
// Consequencia pratica: `n.GetChildByTag("ausente").Tag` devolve a tag de n, e
// nao "". Quem quiser saber se achou TEM que usar GetOptionalChildByTag e olhar
// o ok. Este teste trava o comportamento atual para que a armadilha fique
// escrita em algum lugar.
func TestGetChildByTagReturnsTheStartNodeWhenMissing(t *testing.T) {
	root := &Node{Tag: "iq", Attrs: Attrs{"id": "1"}}
	got := root.GetChildByTag("ausente")
	if got.Tag != "iq" {
		t.Errorf("tag = %q, esperado \"iq\" (o proprio no' de partida)", got.Tag)
	}

	// Com duas tags, devolve o no' do ultimo nivel que existiu.
	nested := &Node{Tag: "iq", Content: []Node{{Tag: "list"}}}
	if got := nested.GetChildByTag("list", "ausente"); got.Tag != "list" {
		t.Errorf("tag = %q, esperado \"list\"", got.Tag)
	}
}

func TestUnmarshalJSONParsesNodeTree(t *testing.T) {
	raw := `{"Tag":"iq","Attrs":{"id":"1"},"Content":[{"Tag":"item","Attrs":{}}]}`
	var n Node
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		t.Fatal(err)
	}
	if n.Tag != "iq" || n.Attrs["id"] != "1" {
		t.Errorf("= %+v", n)
	}
	if children := n.GetChildren(); len(children) != 1 || children[0].Tag != "item" {
		t.Errorf("filhos = %v", n.Content)
	}
}

// Um atributo que se parece com um JID de servidor conhecido e' convertido
// para types.JID na desserializacao. E' o que faz um Node vindo de JSON se
// comportar como um vindo do fio.
func TestUnmarshalJSONPromotesKnownServersToJID(t *testing.T) {
	raw := `{"Tag":"x","Attrs":{
		"user":"5511999999999@s.whatsapp.net",
		"grupo":"120363@g.us",
		"news":"123@newsletter",
		"bcast":"status@broadcast",
		"texto":"nao e jid"
	}}`
	var n Node
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"user", "grupo", "news", "bcast"} {
		if _, ok := n.Attrs[key].(types.JID); !ok {
			t.Errorf("atributo %q voltou como %T, esperado types.JID", key, n.Attrs[key])
		}
	}
	if got := n.Attrs["texto"]; got != "nao e jid" {
		t.Errorf("string comum virou %v (%T)", got, got)
	}
}

// JSON so' tem float64. Um timestamp que passasse por float perderia precisao
// acima de 2^53, entao a desserializacao o converte para int64.
func TestUnmarshalJSONConvertsNumbersToInt64(t *testing.T) {
	var n Node
	if err := json.Unmarshal([]byte(`{"Tag":"x","Attrs":{"t":1700000000}}`), &n); err != nil {
		t.Fatal(err)
	}
	got, ok := n.Attrs["t"].(int64)
	if !ok {
		t.Fatalf("t voltou como %T, esperado int64", n.Attrs["t"])
	}
	if got != 1700000000 {
		t.Errorf("t = %d", got)
	}
}

func TestUnmarshalJSONReadsBase64Content(t *testing.T) {
	var n Node
	if err := json.Unmarshal([]byte(`{"Tag":"x","Content":"AAEC"}`), &n); err != nil {
		t.Fatal(err)
	}
	got, ok := n.Content.([]byte)
	if !ok {
		t.Fatalf("conteudo voltou como %T, esperado []byte", n.Content)
	}
	if !bytes.Equal(got, []byte{0x00, 0x01, 0x02}) {
		t.Errorf("= %x", got)
	}
}

func TestUnmarshalJSONRejectsMalformedInput(t *testing.T) {
	tests := map[string]string{
		"json invalido":                       `{`,
		"conteudo que nao e lista nem base64": `{"Tag":"x","Content":123}`,
		"lista com item invalido":             `{"Tag":"x","Content":[{"Tag":1}]}`,
		"base64 invalido":                     `{"Tag":"x","Content":"!!!"}`,
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			var n Node
			if err := json.Unmarshal([]byte(raw), &n); err == nil {
				t.Errorf("nao falhou: %+v", n)
			}
		})
	}
}

func TestUnmarshalJSONAcceptsNodeWithoutContent(t *testing.T) {
	var n Node
	if err := json.Unmarshal([]byte(`{"Tag":"x"}`), &n); err != nil {
		t.Fatal(err)
	}
	if n.Content != nil {
		t.Errorf("conteudo = %v, esperado nil", n.Content)
	}
}

func TestMarshalNeverFails(t *testing.T) {
	// Marshal devolve erro na assinatura mas nunca o preenche: os casos
	// invalidos entram em panic dentro do encoder. Trava o contrato para
	// quem le' a assinatura e acha que da' para tratar o erro.
	for _, n := range corpus() {
		if _, err := Marshal(n); err != nil {
			t.Errorf("Marshal(%+v) = %v", n, err)
		}
	}
}
