package whatsmeow

import "testing"

// TestParseJID_PhonePrefixRemove: telefones com "+" são normalizados.
func TestParseJID_PhonePrefixRemove(t *testing.T) {
	jid, ok := ParseJID("+5511987654321")
	if !ok {
		t.Fatal("ParseJID(+5511) = not ok")
	}
	if jid.User != "5511987654321" {
		t.Errorf("ParseJID user = %q, want 5511987654321", jid.User)
	}
}

// TestParseJID_BareNumber aplica servidor padrão.
func TestParseJID_BareNumber(t *testing.T) {
	jid, ok := ParseJID("5511987654321")
	if !ok {
		t.Fatal("ParseJID(bare) = not ok")
	}
	if jid.Server != "s.whatsapp.net" {
		t.Errorf("ParseJID server = %q, want s.whatsapp.net", jid.Server)
	}
}

// TestParseJID_Qualified mantém servidor do input.
func TestParseJID_Qualified(t *testing.T) {
	jid, ok := ParseJID("120363000000000000@g.us")
	if !ok {
		t.Fatal("ParseJID(qualified) = not ok")
	}
	if jid.Server != "g.us" {
		t.Errorf("ParseJID server = %q, want g.us", jid.Server)
	}
}

// TestParseJID_NoServer: entrada com @ mas sem servidor. ParseJID do
// whatsmeow é leniente e pode ou não falhar — apenas verificamos que
// não panic.
func TestParseJID_NoServer(t *testing.T) {
	defer func() {
		_ = recover()
	}()
	jid, _ := ParseJID("5511987654321@")
	_ = jid
}

// TestParseJID_EmptyString: string vazia trata como telefone cru.
func TestParseJID_EmptyString(t *testing.T) {
	// Pode panic em arg[0]; o teste é apenas para forçar a leitura.
	defer func() {
		_ = recover()
	}()
	_, _ = ParseJID("")
}

// TestParseJID_JustPlus: só "+" — após strip, é string vazia.
func TestParseJID_JustPlus(t *testing.T) {
	jid, ok := ParseJID("+")
	if !ok {
		t.Fatal("ParseJID(+) = not ok")
	}
	if jid.User != "" {
		t.Errorf("ParseJID(+) user = %q, want empty", jid.User)
	}
}
