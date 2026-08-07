package binary

import (
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-noise/protocol/types"
)

// AttrUtility acumula erros em vez de devolve-los. O contrato inteiro depende
// disso: quem le' 10 atributos checa au.Error() uma vez no fim, e um atributo
// obrigatorio faltando tem que aparecer la'.
func TestAttrUtilityAccumulatesErrorsInsteadOfReturning(t *testing.T) {
	n := &Node{Attrs: Attrs{"presente": "1"}}
	au := n.AttrGetter()

	au.String("ausente1")
	au.Int("ausente2")
	au.JID("ausente3")

	if au.OK() {
		t.Error("OK() = true apesar dos atributos obrigatorios faltando")
	}
	if len(au.Errors) != 3 {
		t.Errorf("erros acumulados = %d, esperado 3", len(au.Errors))
	}
	err := au.Error()
	if err == nil {
		t.Fatal("Error() = nil apesar de OK() ser false")
	}
	for _, key := range []string{"ausente1", "ausente2", "ausente3"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("erro nao menciona %q: %v", key, err)
		}
	}
}

func TestAttrUtilityWithoutErrorsIsClean(t *testing.T) {
	au := (&Node{Attrs: Attrs{"a": "1"}}).AttrGetter()
	au.String("a")
	if !au.OK() {
		t.Errorf("OK() = false com %v", au.Errors)
	}
	if err := au.Error(); err != nil {
		t.Errorf("Error() = %v, esperado nil", err)
	}
}

// A distincao entre Optional* e o getter obrigatorio e' o coracao do tipo:
// ausente e' erro num, silencio no outro. Um TIPO errado, porem, e' erro nos
// dois — um atributo presente com o tipo errado e' frame malformado.
func TestOptionalGettersIgnoreAbsenceButNotWrongType(t *testing.T) {
	au := (&Node{Attrs: Attrs{"jid": "nao e um JID", "num": "abc", "bool": "talvez"}}).AttrGetter()

	if got := au.OptionalString("ausente"); got != "" {
		t.Errorf("OptionalString de ausente = %q", got)
	}
	if got := au.OptionalInt("ausente"); got != 0 {
		t.Errorf("OptionalInt de ausente = %d", got)
	}
	if got := au.OptionalBool("ausente"); got != false {
		t.Errorf("OptionalBool de ausente = %v", got)
	}
	if got := au.OptionalJID("ausente"); got != nil {
		t.Errorf("OptionalJID de ausente = %v", got)
	}
	if got := au.OptionalJIDOrEmpty("ausente"); !got.IsEmpty() {
		t.Errorf("OptionalJIDOrEmpty de ausente = %v", got)
	}
	if got := au.OptionalUnixTime("ausente"); !got.IsZero() {
		t.Errorf("OptionalUnixTime de ausente = %v", got)
	}
	if got := au.OptionalUnixMilli("ausente"); !got.IsZero() {
		t.Errorf("OptionalUnixMilli de ausente = %v", got)
	}
	if !au.OK() {
		t.Errorf("atributos ausentes nao deveriam gerar erro: %v", au.Errors)
	}

	// Presentes com o tipo errado: cada um acrescenta um erro.
	au.OptionalJID("jid")
	au.OptionalInt("num")
	au.OptionalBool("bool")
	if len(au.Errors) != 3 {
		t.Errorf("erros = %d, esperado 3: %v", len(au.Errors), au.Errors)
	}
}

func TestAttrUtilityParsesEveryType(t *testing.T) {
	jid := types.NewJID("5511999999999", types.DefaultUserServer)
	au := (&Node{Attrs: Attrs{
		"jid":    jid,
		"str":    "texto",
		"int":    "42",
		"neg":    "-42",
		"uint":   "18446744073709551615",
		"bool":   "true",
		"t":      "1700000000",
		"tms":    "1700000000123",
		"zero":   "0",
		"zeroms": "0",
	}}).AttrGetter()

	if got := au.JID("jid"); got.String() != jid.String() {
		t.Errorf("JID = %v", got)
	}
	if got := au.String("str"); got != "texto" {
		t.Errorf("String = %q", got)
	}
	if got := au.Int("int"); got != 42 {
		t.Errorf("Int = %d", got)
	}
	if got := au.Int64("neg"); got != -42 {
		t.Errorf("Int64 = %d", got)
	}
	if got := au.Uint64("uint"); got != 18446744073709551615 {
		t.Errorf("Uint64 = %d", got)
	}
	if got := au.Bool("bool"); !got {
		t.Errorf("Bool = %v", got)
	}
	if got := au.UnixTime("t"); !got.Equal(time.Unix(1700000000, 0)) {
		t.Errorf("UnixTime = %v", got)
	}
	if got := au.UnixMilli("tms"); !got.Equal(time.UnixMilli(1700000000123)) {
		t.Errorf("UnixMilli = %v", got)
	}
	if got := au.OptionalInt("int"); got != 42 {
		t.Errorf("OptionalInt = %d", got)
	}
	if got := au.OptionalString("str"); got != "texto" {
		t.Errorf("OptionalString = %q", got)
	}
	if got := au.OptionalJID("jid"); got == nil || got.String() != jid.String() {
		t.Errorf("OptionalJID = %v", got)
	}
	if got := au.OptionalJIDOrEmpty("jid"); got.String() != jid.String() {
		t.Errorf("OptionalJIDOrEmpty = %v", got)
	}
	if !au.OK() {
		t.Errorf("erros inesperados: %v", au.Errors)
	}
}

// Timestamp 0 vira time zero, nao 1970-01-01. E' a diferenca entre "sem
// timestamp" e "no comeco da epoca", e o codigo que consome checa IsZero().
func TestZeroTimestampBecomesZeroTime(t *testing.T) {
	au := (&Node{Attrs: Attrs{"t": "0", "ms": "0"}}).AttrGetter()
	if got := au.UnixTime("t"); !got.IsZero() {
		t.Errorf("UnixTime de 0 = %v, esperado time zero", got)
	}
	if got := au.UnixMilli("ms"); !got.IsZero() {
		t.Errorf("UnixMilli de 0 = %v, esperado time zero", got)
	}
	if !au.OK() {
		t.Errorf("erros inesperados: %v", au.Errors)
	}
}

func TestAttrUtilityReportsUnparseableValues(t *testing.T) {
	tests := []struct {
		name string
		read func(*AttrUtility)
	}{
		{"int nao numerico", func(au *AttrUtility) { au.Int("bad") }},
		{"uint negativo", func(au *AttrUtility) { au.Uint64("neg") }},
		{"bool invalido", func(au *AttrUtility) { au.Bool("bad") }},
		{"unix time nao numerico", func(au *AttrUtility) { au.UnixTime("bad") }},
		{"unix milli nao numerico", func(au *AttrUtility) { au.UnixMilli("bad") }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			au := (&Node{Attrs: Attrs{"bad": "abc", "neg": "-1"}}).AttrGetter()
			tc.read(au)
			if au.OK() {
				t.Error("valor invalido nao registrou erro")
			}
		})
	}
}

// Um atributo que veio como JID mas foi pedido como string (e vice-versa) e' o
// erro de tipo mais comum: o decoder devolve types.JID para uns e string para
// outros dependendo do formato de fio.
func TestAttrUtilityReportsWrongType(t *testing.T) {
	au := (&Node{Attrs: Attrs{"jid": types.ServerJID, "str": "texto"}}).AttrGetter()
	au.String("jid")
	if au.OK() {
		t.Error("ler JID como string deveria registrar erro")
	}
	before := len(au.Errors)
	au.JID("str")
	if len(au.Errors) != before+1 {
		t.Error("ler string como JID deveria registrar erro")
	}
}

func TestErrorListRendersEveryError(t *testing.T) {
	au := (&Node{Attrs: Attrs{}}).AttrGetter()
	au.String("a")
	au.String("b")
	list, ok := au.Error().(ErrorList)
	if !ok {
		t.Fatalf("Error() devolveu %T, esperado ErrorList", au.Error())
	}
	if len(list) != 2 {
		t.Errorf("ErrorList tem %d itens, esperado 2", len(list))
	}
	rendered := list.Error()
	if !strings.Contains(rendered, "'a'") || !strings.Contains(rendered, "'b'") {
		t.Errorf("Error() = %q, esperado mencionar 'a' e 'b'", rendered)
	}
}

// AttrGetter tem que funcionar sobre um no' sem atributos: e' o caso de todo
// no' folha que chega da rede.
func TestAttrGetterOnNodeWithoutAttributes(t *testing.T) {
	au := (&Node{Tag: "x"}).AttrGetter()
	if got := au.OptionalString("qualquer"); got != "" {
		t.Errorf("= %q", got)
	}
	if !au.OK() {
		t.Errorf("erros inesperados: %v", au.Errors)
	}
}

// O caminho real: o no' vem de Unmarshal, entao os valores sao os que o
// decoder produz, nao os que o teste montou a mao.
func TestAttrUtilityReadsWhatTheDecoderProduces(t *testing.T) {
	jid := types.NewJID("5511999999999", types.DefaultUserServer)
	back := mustRoundTrip(t, Node{Tag: "message", Attrs: Attrs{
		"from": jid,
		"t":    int64(1700000000),
		"id":   "3EB0ABCDEF",
	}})
	au := back.AttrGetter()
	if got := au.JID("from"); got.String() != jid.String() {
		t.Errorf("from = %v", got)
	}
	if got := au.UnixTime("t"); !got.Equal(time.Unix(1700000000, 0)) {
		t.Errorf("t = %v", got)
	}
	if got := au.String("id"); got != "3EB0ABCDEF" {
		t.Errorf("id = %q", got)
	}
	if err := au.Error(); err != nil {
		t.Errorf("erros: %v", err)
	}
}
