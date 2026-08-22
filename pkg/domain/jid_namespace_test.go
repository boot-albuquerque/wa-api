package domain

import "testing"

// TestOsDoisSufixosDeTelefoneCaemNoMesmoNamespace é a razão de o arquivo
// existir. Se algum dia os dois deixarem de concordar aqui, um JID produzido
// por um adaptador passa a estar silenciosamente errado no outro.
func TestOsDoisSufixosDeTelefoneCaemNoMesmoNamespace(t *testing.T) {
	socket := JID("5511999999999@s.whatsapp.net").Namespace()
	pagina := JID("5511999999999@c.us").Namespace()
	if socket != NamespacePhone || pagina != NamespacePhone {
		t.Fatalf("socket=%q pagina=%q, quero ambos %q", socket, pagina, NamespacePhone)
	}
	if socket != pagina {
		t.Fatalf("os dois transportes discordam do namespace do MESMO telefone: %q vs %q", socket, pagina)
	}
}

func TestNamespacePorSufixo(t *testing.T) {
	casos := map[JID]Namespace{
		"5511999999999@lid":     NamespaceLID,
		"120363000000000@g.us":  NamespaceGroup,
		"status@broadcast":      NamespaceBroadcast,
		"0000000000@newsletter": NamespaceNewsletter,
	}
	for in, want := range casos {
		if got := in.Namespace(); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

// TestSemServidorNaoAdivinha trava a decisão, não só o comportamento: o padrão
// pertence ao adaptador, que sabe qual é o SEU transporte. Adivinhar aqui
// colocaria a forma canônica de um transporte na camada compartilhada, que é
// exatamente o erro silencioso que este vocabulário existe para impedir.
func TestSemServidorNaoAdivinha(t *testing.T) {
	if got := JID("5511999999999").Namespace(); got != NamespaceUnknown {
		t.Fatalf("número nu virou %q, quero %q — o padrão é do adaptador", got, NamespaceUnknown)
	}
	if got := JID("5511999999999@naoexiste.example").Namespace(); got != NamespaceUnknown {
		t.Fatalf("servidor desconhecido virou %q, quero %q", got, NamespaceUnknown)
	}
}

// TestServidorUsaOUltimoArroba documenta uma forma que CHEGA aqui: o parser do
// socket divide em "@" e fica com parts[1], descartando o resto em silêncio,
// então "a@b@c" não é rejeitado antes.
func TestServidorUsaOUltimoArroba(t *testing.T) {
	if got := JID("a@b@c.us").Server(); got != "c.us" {
		t.Fatalf("got %q, want %q", got, "c.us")
	}
	if got := JID("semarroba").Server(); got != "" {
		t.Fatalf("got %q, want vazio", got)
	}
}

func TestNormalizeIdentityInput(t *testing.T) {
	casos := map[string]string{
		"  +5511999999999 ": "5511999999999",
		"+5511999999999":    "5511999999999",
		"5511999999999":     "5511999999999",
		"":                  "",
	}
	for in, want := range casos {
		if got := NormalizeIdentityInput(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}
