package headless

import (
	"testing"

	"wa-api/internal/headless"
	"wa-api/pkg/domain"
)

// TestOJIDDoSocketEConvertidoENaoRepassado é o defeito que esta conversão
// existe para impedir, e é o único teste aqui que não pode faltar.
//
// Um domain.JID produzido pelo adaptador do socket nomeia a pessoa certa no
// namespace errado para esta página. Repassá-lo faria a página responder "não
// há tal conversa" para um chat presente — bem-formado e errado, que é a pior
// forma de errar.
func TestOJIDDoSocketEConvertidoENaoRepassado(t *testing.T) {
	doSocket := domain.JID("5511999999999@s.whatsapp.net")

	got, err := ToPageJID(doSocket)
	if err != nil {
		t.Fatalf("ToPageJID: %v", err)
	}
	if got == string(doSocket) {
		t.Fatal("o JID do socket foi REPASSADO sem conversão")
	}
	want := "5511999999999" + headless.ServerPhone
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestToPageJIDPorNamespace(t *testing.T) {
	casos := map[domain.JID]string{
		"5511999999999@s.whatsapp.net": "5511999999999" + headless.ServerPhone,
		"5511999999999@c.us":           "5511999999999" + headless.ServerPhone,
		"5511999999999@lid":            "5511999999999" + headless.ServerLID,
		"120363000000000@g.us":         "120363000000000" + headless.ServerGroup,
		// Número nu: o padrão pertence ao ADAPTADOR, e o adaptador é este.
		"5511999999999":   "5511999999999" + headless.ServerPhone,
		"+5511999999999":  "5511999999999" + headless.ServerPhone,
		" 5511999999999 ": "5511999999999" + headless.ServerPhone,
	}
	for in, want := range casos {
		got, err := ToPageJID(in)
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

// Namespaces que esta conversão não serve são RECUSADOS, e não adivinhados.
// Inventar um sufixo aqui produziria um JID bem-formado que nomeia coisa
// nenhuma — exatamente a classe de erro que a decisão 66 recusou.
func TestToPageJIDRecusaOQueNaoSabeConverter(t *testing.T) {
	for _, in := range []domain.JID{
		"", "   ",
		"status@broadcast",
		"0000000000@newsletter",
		"5511999999999@naoexiste.example",
	} {
		if got, err := ToPageJID(in); err == nil {
			t.Errorf("%q foi aceito e virou %q; devia ser recusado", in, got)
		}
	}
}
