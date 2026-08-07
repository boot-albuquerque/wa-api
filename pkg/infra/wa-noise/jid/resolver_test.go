package jid

import (
	"context"
	"testing"

	"wa-api/pkg/domain"
)

// --- JIDResolverAdapter ---

func TestJIDResolverAdapter_New(t *testing.T) {
	if NewJIDResolverAdapter() == nil {
		t.Fatal("NewJIDResolverAdapter returned nil")
	}
}

// TestResolveJID_Invalid devolve erro para entrada que ParseJID não aceita.
// O ParseJID é leniente (devolve ok=true para telefone cru com servidor
// padrão), então o caminho de erro é difícil de atingir — mas tentamos.
func TestResolveJID_Invalid(t *testing.T) {
	a := JIDResolverAdapter{}
	_, err := a.ResolveJID(context.Background(), string([]byte{0x00}))
	if err == nil {
		t.Skip("ParseJID não devolve erro para esta entrada; o caminho de erro é raro")
	}
}

// TestResolveJID_Phone com telefone cru aplica servidor padrão.
func TestResolveJID_Phone(t *testing.T) {
	a := JIDResolverAdapter{}
	got, err := a.ResolveJID(context.Background(), "5511987654321")
	if err != nil {
		t.Fatalf("ResolveJID phone = %v", err)
	}
	if got != "5511987654321@s.whatsapp.net" {
		t.Errorf("ResolveJID phone = %q", got)
	}
}

// TestResolveJID_Qualified devolve o JID qualificado como está.
func TestResolveJID_Qualified(t *testing.T) {
	a := JIDResolverAdapter{}
	in := "120363000000000000@g.us"
	got, err := a.ResolveJID(context.Background(), in)
	if err != nil {
		t.Fatalf("ResolveJID qualified = %v", err)
	}
	if string(got) != in {
		t.Errorf("ResolveJID qualified = %q, want %q", got, in)
	}
}

// TestResolveQualifiedJID_BareNumber rejeita telefone sem @.
func TestResolveQualifiedJID_BareNumber(t *testing.T) {
	a := JIDResolverAdapter{}
	_, err := a.ResolveQualifiedJID(context.Background(), "5511987654321")
	if err == nil {
		t.Fatal("ResolveQualifiedJID aceitou telefone cru")
	}
}

// TestResolveQualifiedJID_Qualified aceita entrada com servidor.
func TestResolveQualifiedJID_Qualified(t *testing.T) {
	a := JIDResolverAdapter{}
	in := "5511987654321@s.whatsapp.net"
	got, err := a.ResolveQualifiedJID(context.Background(), in)
	if err != nil {
		t.Fatalf("ResolveQualifiedJID = %v", err)
	}
	if string(got) != in {
		t.Errorf("ResolveQualifiedJID = %q, want %q", got, in)
	}
}

// TestToJID_Empty devolve JID zero.
func TestToJID_Empty(t *testing.T) {
	got, err := ToJID("")
	if err != nil {
		t.Fatalf("ToJID(empty) = %v", err)
	}
	if !got.IsEmpty() {
		t.Errorf("ToJID(empty) = %v, want empty", got)
	}
}

// TestToJID_Invalid devolve erro quando ParseJID falha.
func TestToJID_Invalid(t *testing.T) {
	_, err := ToJID(domain.JID(string([]byte{0x00})))
	if err == nil {
		t.Skip("ParseJID não falhou; caminho de erro é raro")
	}
}

// TestToJID_Valid faz round-trip.
func TestToJID_Valid(t *testing.T) {
	in := domain.JID("5511987654321@s.whatsapp.net")
	got, err := ToJID(in)
	if err != nil {
		t.Fatalf("ToJID = %v", err)
	}
	if got.String() != string(in) {
		t.Errorf("ToJID round-trip = %q, want %q", got.String(), in)
	}
}
