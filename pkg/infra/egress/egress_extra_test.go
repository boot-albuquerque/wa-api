package egress

import (
	"context"
	"strings"
	"testing"
)

// TestMustParseCIDRsPanicsOnInvalidLiteral cobre o único caminho de falha da
// construção da tabela de blocos reservados. A tabela é montada em `var
// reservedBlocks = mustParseCIDRs(...)`, ou seja na inicialização do pacote:
// se um literal inválido entrasse na lista e a função apenas o ignorasse, o
// bloco desapareceria da tabela e o validador passaria a aceitar exatamente
// as faixas que existe para negar. Falhar alto é o comportamento correto, e
// é isto que este teste fixa.
func TestMustParseCIDRsPanicsOnInvalidLiteral(t *testing.T) {
	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatal("mustParseCIDRs did not panic on an invalid CIDR literal")
		}
		msg, ok := rec.(string)
		if !ok {
			t.Fatalf("panic value = %#v, want a string", rec)
		}
		if !strings.Contains(msg, "not-a-cidr") {
			t.Errorf("panic message %q does not name the offending literal", msg)
		}
	}()

	mustParseCIDRs([]string{"10.0.0.0/8", "not-a-cidr"})
}

func TestMustParseCIDRsParsesEveryLiteral(t *testing.T) {
	blocks := mustParseCIDRs([]string{"10.0.0.0/8", "::1/128"})
	if len(blocks) != 2 {
		t.Fatalf("len(blocks) = %d, want 2", len(blocks))
	}
	if blocks[0].String() != "10.0.0.0/8" {
		t.Errorf("blocks[0] = %s, want 10.0.0.0/8", blocks[0])
	}
}

// TestValidateOutboundURLAuthorityWithoutHost cobre o ramo `host == ""`, que
// IsHTTPURL sozinho não pega: "http://:8080/x" tem Host não-vazio (":8080")
// e portanto passa por IsHTTPURL, mas Hostname() é vazio — não há nada para
// resolver, e deixar passar significaria dialar o host local implícito.
func TestValidateOutboundURLAuthorityWithoutHost(t *testing.T) {
	err := ValidateOutboundURL(context.Background(), "http://:8080/webhook")
	if err == nil {
		t.Fatal("ValidateOutboundURL accepted an authority with no host")
	}
	if !strings.Contains(err.Error(), "no host") {
		t.Errorf("error = %q, want it to mention the missing host", err.Error())
	}
}

// TestValidateOutboundURLHostnameResolvingToLoopback é o caminho de SSRF que
// só aparece depois da resolução de nome: o host não é um IP literal, então
// nenhuma checagem sintática o rejeita — apenas o loop sobre os endereços
// resolvidos. "localhost" vem de /etc/hosts, sem rede.
func TestValidateOutboundURLHostnameResolvingToLoopback(t *testing.T) {
	err := ValidateOutboundURL(context.Background(), "http://localhost:9000/webhook")
	if err == nil {
		t.Fatal("ValidateOutboundURL accepted a hostname that resolves to loopback")
	}
	if !strings.Contains(err.Error(), "reserved or loopback") {
		t.Errorf("error = %q, want it to name the reserved/loopback rejection", err.Error())
	}
}

// TestValidateOutboundURLDoesNotEchoCredentials: a URL validada é
// configurada pelo operador e pode carregar userinfo. A mensagem de erro sobe
// para o chamador e daí para a resposta HTTP, então não pode devolvê-la.
func TestValidateOutboundURLDoesNotEchoCredentials(t *testing.T) {
	err := ValidateOutboundURL(context.Background(), "http://user:hunter2@127.0.0.1/webhook")
	if err == nil {
		t.Fatal("ValidateOutboundURL accepted a loopback address")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Errorf("error message leaked the URL credentials: %q", err.Error())
	}
}
