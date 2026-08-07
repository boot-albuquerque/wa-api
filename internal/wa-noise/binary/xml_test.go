package binary

import (
	"strings"
	"testing"

	"wa-api/internal/wa-noise/types"
)

// IndentXML e MaxBytesToPrintAsHex sao variaveis GLOBAIS de pacote. Os testes
// que as mexem tem que restaurar o valor, senao contaminam os outros.
func withXMLOptions(t *testing.T, indent bool, maxHex int) {
	t.Helper()
	oldIndent, oldMax := IndentXML, MaxBytesToPrintAsHex
	IndentXML, MaxBytesToPrintAsHex = indent, maxHex
	t.Cleanup(func() { IndentXML, MaxBytesToPrintAsHex = oldIndent, oldMax })
}

func TestXMLStringOfEmptyNodeIsSelfClosing(t *testing.T) {
	withXMLOptions(t, false, 128)
	if got := (&Node{Tag: "ping"}).XMLString(); got != "<ping/>" {
		t.Errorf("= %q", got)
	}
}

// Os atributos sao ordenados antes de imprimir. Sem isso, o mesmo no'
// produziria XML diferente a cada chamada (Attrs e' um map) e o log ficaria
// impossivel de comparar entre execucoes.
func TestXMLStringSortsAttributesDeterministically(t *testing.T) {
	withXMLOptions(t, false, 128)
	n := &Node{Tag: "iq", Attrs: Attrs{"z": "3", "a": "1", "m": "2"}}
	first := n.XMLString()
	for i := 0; i < 20; i++ {
		if got := n.XMLString(); got != first {
			t.Fatalf("saida instavel:\n  %q\n  %q", first, got)
		}
	}
	if !strings.Contains(first, `a="1"`) || !strings.Contains(first, `z="3"`) {
		t.Errorf("= %q", first)
	}
	if strings.Index(first, `a="1"`) > strings.Index(first, `z="3"`) {
		t.Errorf("atributos fora de ordem: %q", first)
	}
}

func TestXMLStringRendersJIDAttributes(t *testing.T) {
	withXMLOptions(t, false, 128)
	n := &Node{Tag: "iq", Attrs: Attrs{"to": types.NewJID("5511999999999", types.DefaultUserServer)}}
	if got := n.XMLString(); !strings.Contains(got, "5511999999999@s.whatsapp.net") {
		t.Errorf("= %q", got)
	}
}

func TestXMLStringNestsChildren(t *testing.T) {
	withXMLOptions(t, false, 128)
	n := &Node{Tag: "iq", Content: []Node{{Tag: "a"}, {Tag: "b"}}}
	got := n.XMLString()
	if !strings.Contains(got, "<a/>") || !strings.Contains(got, "<b/>") {
		t.Errorf("= %q", got)
	}
	if !strings.HasPrefix(got, "<iq>") || !strings.HasSuffix(got, "</iq>") {
		t.Errorf("envelope errado: %q", got)
	}
}

// Conteudo binario tem tres representacoes, e a escolha entre elas e' o que
// torna o log legivel: texto imprimivel sai como texto, binario curto sai em
// hex, binario longo vira so' a contagem de bytes.
func TestContentRenderingDependsOnPrintabilityAndSize(t *testing.T) {
	withXMLOptions(t, false, 8)

	if got := (&Node{Tag: "x", Content: []byte("texto")}).XMLString(); !strings.Contains(got, "texto") {
		t.Errorf("texto imprimivel = %q", got)
	}
	if got := (&Node{Tag: "x", Content: []byte{0x00, 0x01}}).XMLString(); !strings.Contains(got, "0001") {
		t.Errorf("binario curto = %q, esperado hex", got)
	}
	long := make([]byte, MaxBytesToPrintAsHex+1)
	if got := (&Node{Tag: "x", Content: long}).XMLString(); !strings.Contains(got, "9 bytes") {
		t.Errorf("binario longo = %q, esperado a contagem", got)
	}
	// UTF-8 invalido nao e' imprimivel e cai no ramo de hex.
	if got := (&Node{Tag: "x", Content: []byte{0xFF, 0xFE}}).XMLString(); !strings.Contains(got, "fffe") {
		t.Errorf("utf8 invalido = %q", got)
	}
}

// Sem indentacao, uma quebra de linha no conteudo vira "\\n" literal para nao
// arrebentar o alinhamento de uma linha de log.
//
// Note QUAL ramo faz isso: para Content []byte, um texto com \n nao chega la',
// porque printable() rejeita \n (nao passa em unicode.IsPrint) e o conteudo cai
// no ramo de hex. O escape so' e' alcancavel pelo ramo default, de conteudo que
// nao e' nem lista nem []byte. O ReplaceAll do ramo []byte e' inalcancavel —
// registrado em HOUSEKEEP.md, F27.
func TestNewlinesAreEscapedWhenNotIndenting(t *testing.T) {
	withXMLOptions(t, false, 128)

	got := (&Node{Tag: "x", Content: "linha1\nlinha2"}).XMLString()
	if strings.Count(got, "\n") != 0 {
		t.Errorf("= %q, esperado sem quebra de linha real", got)
	}
	if !strings.Contains(got, `linha1\nlinha2`) {
		t.Errorf("= %q", got)
	}

	// O mesmo texto como []byte vai para hex, nao para o escape.
	asBytes := (&Node{Tag: "x", Content: []byte("linha1\nlinha2")}).XMLString()
	if !strings.Contains(asBytes, "6c696e") {
		t.Errorf("[]byte com \\n = %q, esperado hex", asBytes)
	}
}

func TestIndentedOutputBreaksIntoLines(t *testing.T) {
	withXMLOptions(t, true, 128)
	n := &Node{Tag: "iq", Content: []Node{{Tag: "a"}, {Tag: "b"}}}
	got := n.XMLString()
	if !strings.Contains(got, "\n") {
		t.Errorf("modo indentado nao quebrou linha: %q", got)
	}
	if !strings.Contains(got, "  <a/>") {
		t.Errorf("modo indentado nao recuou os filhos: %q", got)
	}
}

func TestIndentedHexIsWrappedInFixedWidthLines(t *testing.T) {
	withXMLOptions(t, true, 1024)
	// 100 bytes = 200 caracteres hex, que passam de uma linha de 80.
	got := (&Node{Tag: "x", Content: make([]byte, 100)}).XMLString()
	for _, line := range strings.Split(got, "\n") {
		if len(strings.TrimSpace(line)) > 80 {
			t.Errorf("linha de %d caracteres: %q", len(line), line)
		}
	}
}

// Conteudo que nao e' nem lista nem []byte cai no ramo default e e' impresso
// com %s. Node.Content e' interface{}, entao esse caso e' alcancavel.
func TestUnexpectedContentTypeIsPrintedAsIs(t *testing.T) {
	withXMLOptions(t, false, 128)
	if got := (&Node{Tag: "x", Content: "string crua"}).XMLString(); !strings.Contains(got, "string crua") {
		t.Errorf("= %q", got)
	}
	withXMLOptions(t, true, 128)
	if got := (&Node{Tag: "x", Content: "a\nb"}).XMLString(); !strings.Contains(got, "a") {
		t.Errorf("= %q", got)
	}
}

func TestPrintableRejectsControlAndInvalidBytes(t *testing.T) {
	if got := printable([]byte("ok")); got != "ok" {
		t.Errorf("= %q", got)
	}
	if got := printable([]byte{0x00}); got != "" {
		t.Errorf("byte de controle = %q, esperado vazio", got)
	}
	if got := printable([]byte{0xFF, 0xFE}); got != "" {
		t.Errorf("utf8 invalido = %q, esperado vazio", got)
	}
}

// O caminho completo: o no' vem do fio e e' impresso. E' assim que o XML
// aparece no log de debug do wa-api.
func TestXMLStringOfDecodedFrame(t *testing.T) {
	withXMLOptions(t, false, 128)
	back := mustRoundTrip(t, Node{
		Tag:     "message",
		Attrs:   Attrs{"id": "3EB0ABCDEF"},
		Content: []Node{{Tag: "enc", Content: []byte{0x01, 0x02}}},
	})
	got := back.XMLString()
	if !strings.Contains(got, `id="3EB0ABCDEF"`) || !strings.Contains(got, "<enc>") {
		t.Errorf("= %q", got)
	}
}
