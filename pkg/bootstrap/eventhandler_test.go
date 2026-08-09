package bootstrap

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// F91: o ramo `default` de handleEvent despejava a struct inteira com `%+v`.
//
// A F76 pegou o caso mais grave — o evento QR carrega `Codes`, que são códigos
// de PAREAMENTO — e o corrigiu com um `case` dedicado. Isso tratou a
// instância, não a classe: qualquer outro tipo caindo no `default` vaza igual,
// e o log de produção já mostrou evento de chamada com CallID e CallCreator.

type eventoComSegredo struct {
	Codes     []string
	CallID    string
	Timestamp int64
	FromSync  bool
}

func TestUnhandledEventFields_DevolveNomesNuncaValores(t *testing.T) {
	segredo := "https://wa.me/settings/linked_devices#2@B+o1E1qrtMni2Ojv"
	evt := &eventoComSegredo{
		Codes:  []string{segredo},
		CallID: "0008D39B2799E377FA7D4E02FDA0C07B",
	}

	campos := unhandledEventFields(evt)

	querAlgum := map[string]bool{"Codes": false, "CallID": false, "Timestamp": false, "FromSync": false}
	for _, c := range campos {
		if _, ok := querAlgum[c]; ok {
			querAlgum[c] = true
		}
	}
	for nome, achou := range querAlgum {
		if !achou {
			t.Errorf("campo %q ausente: quem for implementar o case precisa da FORMA do evento", nome)
		}
	}

	// A asserção que importa: nenhum VALOR aparece.
	for _, c := range campos {
		if strings.Contains(c, segredo) || strings.Contains(c, evt.CallID) {
			t.Fatalf("valor vazou na lista de campos: %q", c)
		}
	}
}

func TestUnhandledEventFields_NaoQuebraComEntradaEstranha(t *testing.T) {
	casos := []struct {
		nome string
		evt  any
	}{
		{"nil", nil},
		{"ponteiro nil", (*eventoComSegredo)(nil)},
		{"nao-struct", "apenas uma string"},
		{"struct vazia", struct{}{}},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			// Não pode entrar em pânico: isto roda no caminho de TODO evento
			// não tratado, e um pânico ali derruba o handler da sessão.
			if got := unhandledEventFields(c.evt); len(got) != 0 {
				t.Errorf("campos = %v, quero vazio", got)
			}
		})
	}
}

// TestHandleEvent_DefaultNaoLogaValores é o teste que importa, e o que faltava.
//
// Os dois acima cobrem `unhandledEventFields` — a FUNÇÃO. Mas o vazamento da
// F91 acontece no CALL SITE: bastava alguém trocar o `Strs(...)` de volta por
// `%+v` que a função continuaria correta e o segredo voltaria ao log. O
// controle negativo provou isso: mutando o call site, os dois testes anteriores
// seguiram verdes.
//
// Aqui a asserção é sobre o que SAI no log.
func TestHandleEvent_DefaultNaoLogaValores(t *testing.T) {
	const segredo = "https://wa.me/settings/linked_devices#2@SEGREDO-DE-PAREAMENTO"

	var buf bytes.Buffer
	original := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = original })

	evh := &UserEventHandler{UserID: "u1"}
	evh.handleEvent(&eventoComSegredo{
		Codes:  []string{segredo},
		CallID: "0008D39B2799E377FA7D4E02FDA0C07B",
	})

	saida := buf.String()
	if saida == "" {
		t.Fatal("nada foi logado: o aviso de evento nao tratado sumiu")
	}
	if strings.Contains(saida, segredo) {
		t.Fatalf("o log contem o VALOR do campo Codes — e' credencial de pareamento:\n%s", saida)
	}
	if strings.Contains(saida, "0008D39B2799E377FA7D4E02FDA0C07B") {
		t.Fatalf("o log contem o CallID:\n%s", saida)
	}
	// O aviso tem de continuar útil: sem tipo e sem campos ele vira ruído.
	if !strings.Contains(saida, "eventoComSegredo") {
		t.Errorf("o log nao identifica o TIPO do evento:\n%s", saida)
	}
	if !strings.Contains(saida, "Codes") {
		t.Errorf("o log nao lista os nomes dos campos:\n%s", saida)
	}
}
