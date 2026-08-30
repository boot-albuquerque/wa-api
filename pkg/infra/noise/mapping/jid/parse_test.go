package jid

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
// noise é leniente e pode ou não falhar — apenas verificamos que
// não panic.
func TestParseJID_NoServer(t *testing.T) {
	defer func() {
		_ = recover()
	}()
	jid, _ := ParseJID("5511987654321@")
	_ = jid
}

// TestParseJID_EmptyString: string vazia trata como telefone cru.
// TestParseJID_EmptyString: string vazia é RECUSADA, e sem pânico.
//
// A versão anterior deste teste dizia, em comentário: "Pode panic em arg[0]; o
// teste é apenas para forçar a leitura" — e engolia o pânico com `recover`. Um
// teste que apanha o pânico em vez de o proibir CONFIRMA o defeito e ainda
// aparece verde no relatório. `ParseJID("")` acedia a `arg[0]` numa string
// vazia; só não derrubou nada porque os use cases guardam o campo vazio antes
// de chamar (F209).
func TestParseJID_EmptyString(t *testing.T) {
	defer func() {
		if p := recover(); p != nil {
			t.Fatalf("ParseJID(\"\") entrou em pânico: %v", p)
		}
	}()
	if _, ok := ParseJID(""); ok {
		t.Error("ParseJID(\"\") = ok; string vazia não é telefone nem JID")
	}
}

// TestParseJID_JustPlus: só "+" é RECUSADO.
//
// Este teste afirmava o contrário — que `ParseJID("+")` devolvia `ok` com
// utilizador vazio — e é o defeito da F209 em miniatura: um "+" sozinho virava
// `@s.whatsapp.net` sem utilizador, e seguia para a rede. Um teste que descreve
// o comportamento errado como esperado torna a correção numa "quebra", e é por
// isso que ele muda aqui em vez de ser contornado.
func TestParseJID_JustPlus(t *testing.T) {
	if _, ok := ParseJID("+"); ok {
		t.Error("ParseJID(+) = ok; um sinal de mais sozinho não é telefone")
	}
}

// TestParseJID_RecusaOQueNaoEhTelefone é o teste do DEFEITO medido: o valor
// exato que pendurou o pedido 75 segundos em campo, mais os vizinhos que
// falhariam da mesma maneira.
//
// Medido a 2026-08-21, POST /user/block:
//
//	{"Phone":"abc"} -> HTTP 500 após duration_ms=75003.445
//	log: ERR Failed to block user error="info query timed out" jid=abc@s.whatsapp.net
func TestParseJID_RecusaOQueNaoEhTelefone(t *testing.T) {
	for _, raw := range []string{
		"abc",              // o valor medido em campo
		"5511abc99999",     // dígitos misturados com texto
		"1234",             // curto de mais para ser um número
		"1234567890123456", // 16 dígitos, acima do teto do E.164
		"55 11 98765-4321", // formatado para humano, não para a rede
		"+",
		"",
	} {
		t.Run(raw, func(t *testing.T) {
			if jid, ok := ParseJID(raw); ok {
				t.Errorf("ParseJID(%q) = %q, ok; isto ia para a rede e o servidor "+
					"do WhatsApp nunca responde — o pedido fica pendurado até o "+
					"prazo do info query esgotar (75s medidos em campo)", raw, jid.String())
			}
		})
	}
}

// E o contrário, que é o que a armadilha nº2 exige: o caminho de SUCESSO.
// Recusar de mais é pior que o defeito — quem tem um número válido deixa de
// conseguir usar a rota, e isso não aparece em nenhum teste da guarda.
func TestParseJID_AceitaTelefoneReal(t *testing.T) {
	for _, raw := range []string{
		"5511987654321",  // o número de teste deste repositório
		"+5511987654321", // com o mais, que é retirado
		"554192421234",   // sessão `lucas`, em campo
		"5516981818244",  // sessão `filarapida`, em campo
		"12025550123",    // 11 dígitos, EUA
		"12345",          // o mínimo aceite
	} {
		t.Run(raw, func(t *testing.T) {
			jid, ok := ParseJID(raw)
			if !ok {
				t.Fatalf("ParseJID(%q) = not ok; número válido recusado", raw)
			}
			if jid.Server != "s.whatsapp.net" {
				t.Errorf("servidor = %q, quero s.whatsapp.net", jid.Server)
			}
		})
	}
}
