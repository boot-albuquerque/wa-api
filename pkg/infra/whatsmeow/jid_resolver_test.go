package whatsmeow

import (
	"context"
	"testing"
)

// TestJIDResolver_ResolveQualifiedJID_RejectsBareNumber cobre a regra
// estrita: sem "@" não há servidor explícito e a resolução falha. O caso
// importa porque types.ParseJID NÃO devolve erro para um telefone cru — ela
// coloca a string inteira em Server e deixa User vazio —, e o JID achatado em
// string chegaria ao adapter de blocklist, que o reparseia com o ParseJID
// leniente e aplicaria o servidor padrão em silêncio.
func TestJIDResolver_ResolveQualifiedJID_RejectsBareNumber(t *testing.T) {
	r := NewJIDResolverAdapter()

	for _, raw := range []string{"5511999999999", "+5511999999999", ""} {
		got, err := r.ResolveQualifiedJID(context.Background(), raw)
		if err == nil {
			t.Errorf("ResolveQualifiedJID(%q) = %q, want error", raw, got)
		}
		if got != "" {
			t.Errorf("ResolveQualifiedJID(%q) = %q, want empty JID on error", raw, got)
		}
	}
}

// TestJIDResolver_ResolveQualifiedJID_AcceptsQualified garante que a entrada
// já qualificada continua passando inalterada.
func TestJIDResolver_ResolveQualifiedJID_AcceptsQualified(t *testing.T) {
	r := NewJIDResolverAdapter()

	for _, raw := range []string{
		"5511999999999@s.whatsapp.net",
		"120363000000000000@g.us",
	} {
		got, err := r.ResolveQualifiedJID(context.Background(), raw)
		if err != nil {
			t.Errorf("ResolveQualifiedJID(%q) returned error: %v", raw, err)
			continue
		}
		if string(got) != raw {
			t.Errorf("ResolveQualifiedJID(%q) = %q, want %q", raw, got, raw)
		}
	}
}

// TestJIDResolver_ResolveJID_AppliesDefaultServer fixa o contraste: a porta
// leniente continua aceitando telefone cru e aplicando o servidor padrão.
func TestJIDResolver_ResolveJID_AppliesDefaultServer(t *testing.T) {
	r := NewJIDResolverAdapter()

	got, err := r.ResolveJID(context.Background(), "5511999999999")
	if err != nil {
		t.Fatalf("ResolveJID returned error: %v", err)
	}
	if want := "5511999999999@s.whatsapp.net"; string(got) != want {
		t.Errorf("ResolveJID = %q, want %q", got, want)
	}
}
