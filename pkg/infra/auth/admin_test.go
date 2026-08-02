package auth_test

import (
	"strings"
	"testing"

	"wa-api/pkg/infra/auth"
)

// O teste de mutação que motivou este arquivo: trocar o
// subtle.ConstantTimeCompare de admin.go por `return true` — ou por qualquer
// comparação de prefixo — não era detectado por nada. Cada caso abaixo existe
// para matar uma mutação nomeada, não para exercitar a função.

func TestAdminValidator_AcceptsExactToken(t *testing.T) {
	v := auth.NewAdminValidator("s3cr3t-admin")

	if !v.Validate("s3cr3t-admin") {
		t.Fatal("o token admin correto foi rejeitado")
	}
}

func TestAdminValidator_RejectsWrongTokens(t *testing.T) {
	// Mata: `return true`, comparação de prefixo, comparação de sufixo,
	// comparação case-insensitive, comparação por comprimento.
	cases := map[string]string{
		"token diferente":         "outro-token",
		"prefixo do token":        "s3cr3t",
		"sufixo do token":         "admin",
		"token com sufixo extra":  "s3cr3t-adminX",
		"token com prefixo extra": "Xs3cr3t-admin",
		"diferenca de caixa":      "S3CR3T-ADMIN",
		"mesmo comprimento":       "aaaaaaaaaaaa",
		"vazio":                   "",
		"espaco a mais":           "s3cr3t-admin ",
	}

	v := auth.NewAdminValidator("s3cr3t-admin")
	for name, candidate := range cases {
		t.Run(name, func(t *testing.T) {
			if v.Validate(candidate) {
				t.Fatalf("candidato %q foi aceito como admin", candidate)
			}
		})
	}
}

// TestAdminValidator_LongTokenDoesNotTruncate mata a mutação em que o hash é
// substituído por uma comparação sobre os N primeiros bytes: dois tokens que
// só divergem depois de 4 KiB precisam ser distinguidos.
func TestAdminValidator_LongTokenDoesNotTruncate(t *testing.T) {
	base := strings.Repeat("a", 4096)
	v := auth.NewAdminValidator(base + "X")

	if v.Validate(base + "Y") {
		t.Fatal("dois tokens longos que divergem no ultimo byte foram tratados como iguais")
	}
	if !v.Validate(base + "X") {
		t.Fatal("o token longo correto foi rejeitado")
	}
}

// TestAdminValidator_EmptyAdminTokenAcceptsEmptyCandidate documenta uma
// PRE-CONDICAO, não um comportamento desejado: construído com token vazio, o
// validador autentica um header vazio — ou seja, qualquer requisição sem
// Authorization vira admin.
//
// Hoje isso não é explorável porque pkg/bootstrap/main.go:201-212 gera um
// token aleatório quando nenhum é configurado. O teste existe para que, se
// alguém remover essa geração, a falha apareça AQUI, com o motivo escrito,
// em vez de virar bypass silencioso em produção.
func TestAdminValidator_EmptyAdminTokenAcceptsEmptyCandidate(t *testing.T) {
	v := auth.NewAdminValidator("")

	if !v.Validate("") {
		t.Fatal("comportamento mudou: um token admin vazio deixou de aceitar candidato vazio; " +
			"se a mudanca foi deliberada (rejeitar sempre quando nao ha token configurado), " +
			"este teste deve ser invertido e a garantia de main.go documentada aqui")
	}
}

func TestValidateAdminToken_MatchesValidatorSemantics(t *testing.T) {
	// ValidateAdminToken e AdminValidator.Validate são duas implementações da
	// mesma regra. Se uma divergir da outra, este teste quebra.
	const admin = "s3cr3t-admin"
	v := auth.NewAdminValidator(admin)

	for _, candidate := range []string{admin, "", "s3cr3t", "outro", admin + "x"} {
		if got, want := auth.ValidateAdminToken(candidate, admin), v.Validate(candidate); got != want {
			t.Fatalf("candidato %q: ValidateAdminToken=%v, AdminValidator.Validate=%v — as duas implementacoes divergiram",
				candidate, got, want)
		}
	}
}
