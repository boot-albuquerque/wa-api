// Copyright (c) 2023 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package types

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// Os cinco enums de newsletter implementam UnmarshalText so' para NORMALIZAR o
// caixa: o servidor manda "ACTIVE" e "active" indistintamente, e sem a
// normalizacao a comparacao com a constante falharia em metade das respostas.
func TestNewsletterEnumsNormaliseCase(t *testing.T) {
	t.Run("VerificationState", func(t *testing.T) {
		var v NewsletterVerificationState
		for _, input := range []string{"VERIFIED", "Verified", "verified"} {
			if err := v.UnmarshalText([]byte(input)); err != nil {
				t.Fatal(err)
			}
			if v != NewsletterVerificationStateVerified {
				t.Errorf("%q virou %q", input, string(v))
			}
		}
	})
	t.Run("Privacy", func(t *testing.T) {
		var v NewsletterPrivacy
		if err := v.UnmarshalText([]byte("PUBLIC")); err != nil {
			t.Fatal(err)
		}
		if v != NewsletterPrivacyPublic {
			t.Errorf("= %q", string(v))
		}
	})
	t.Run("State", func(t *testing.T) {
		var v NewsletterState
		if err := v.UnmarshalText([]byte("GEOSUSPENDED")); err != nil {
			t.Fatal(err)
		}
		if v != NewsletterStateGeoSuspended {
			t.Errorf("= %q", string(v))
		}
	})
	t.Run("MuteState", func(t *testing.T) {
		var v NewsletterMuteState
		if err := v.UnmarshalText([]byte("ON")); err != nil {
			t.Fatal(err)
		}
		if v != NewsletterMuteOn {
			t.Errorf("= %q", string(v))
		}
	})
	t.Run("Role", func(t *testing.T) {
		var v NewsletterRole
		if err := v.UnmarshalText([]byte("OWNER")); err != nil {
			t.Fatal(err)
		}
		if v != NewsletterRoleOwner {
			t.Errorf("= %q", string(v))
		}
	})
}

// Um valor que nao e' nenhuma das constantes NAO e' rejeitado: passa em
// minusculo. E' deliberado — um estado novo do servidor nao pode quebrar a
// desserializacao da resposta inteira.
func TestNewsletterEnumsAcceptUnknownValues(t *testing.T) {
	var role NewsletterRole
	if err := role.UnmarshalText([]byte("MODERATOR")); err != nil {
		t.Fatalf("valor desconhecido deveria ser aceito: %v", err)
	}
	if string(role) != "moderator" {
		t.Errorf("= %q, esperado normalizado em minusculo", string(role))
	}
}

// O caminho real e' via encoding/json, que chama UnmarshalText por ser
// encoding.TextUnmarshaler. Se a assinatura mudasse, o json passaria a
// atribuir a string crua sem normalizar e nada quebraria em compilacao.
func TestNewsletterEnumsNormaliseThroughJSON(t *testing.T) {
	var payload struct {
		Role  NewsletterRole  `json:"role"`
		State NewsletterState `json:"state"`
	}
	if err := json.Unmarshal([]byte(`{"role":"ADMIN","state":"ACTIVE"}`), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Role != NewsletterRoleAdmin {
		t.Errorf("role = %q", string(payload.Role))
	}
	if payload.State != NewsletterStateActive {
		t.Errorf("state = %q", string(payload.State))
	}
}

func gqlError(code int, message, severity string) GraphQLError {
	return GraphQLError{
		Extensions: GraphQLErrorExtensions{ErrorCode: code, Severity: severity},
		Message:    message,
	}
}

func TestGraphQLErrorRendersCodeMessageAndSeverity(t *testing.T) {
	got := gqlError(404, "nao encontrado", "WARNING").Error()
	for _, want := range []string{"404", "nao encontrado", "WARNING"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q nao contem %q", got, want)
		}
	}
}

// GraphQLErrors implementa Unwrap() []error, entao a arvore de erros expoe
// cada erro individualmente. E' o que importa porque Error() SO' mostra o
// primeiro (ver o teste abaixo).
//
// ACHADO (HOUSEKEEP.md, F28): errors.Is NAO funciona com um GraphQLError como
// alvo. GraphQLError tem um campo `Path []string`, o que o torna nao
// comparavel, e errors.Is so' compara alvos comparaveis — a busca percorre a
// lista inteira e devolve false sempre. Quem quiser casar um erro especifico
// tem que usar errors.As, que e' o que este teste exercita.
func TestGraphQLErrorsUnwrapExposesEveryError(t *testing.T) {
	first := gqlError(400, "primeiro", "ERROR")
	second := gqlError(500, "segundo", "ERROR")
	list := GraphQLErrors{first, second}

	unwrapped := list.Unwrap()
	if len(unwrapped) != 2 {
		t.Fatalf("Unwrap devolveu %d erros, esperado 2", len(unwrapped))
	}
	if unwrapped[0].Error() != first.Error() || unwrapped[1].Error() != second.Error() {
		t.Errorf("Unwrap devolveu os erros errados: %v", unwrapped)
	}

	// errors.As alcanca o primeiro GraphQLError da arvore.
	var target GraphQLError
	if !errors.As(error(list), &target) {
		t.Fatal("errors.As nao alcancou nenhum GraphQLError")
	}
	if target.Extensions.ErrorCode != 400 {
		t.Errorf("errors.As pegou o codigo %d, esperado 400", target.Extensions.ErrorCode)
	}

	// E errors.Is nao alcanca nenhum, pelo motivo do comentario acima.
	if errors.Is(error(list), first) {
		t.Error("errors.Is passou a funcionar — GraphQLError virou comparavel, atualize HOUSEKEEP F28")
	}
}

// Error() tem tres ramos por tamanho da lista, e o de varios erros mostra so'
// o PRIMEIRO mais a contagem dos demais. Quem precisa dos outros tem que usar
// Unwrap — e' a razao de o teste acima existir.
func TestGraphQLErrorsErrorSummarisesByCount(t *testing.T) {
	if got := (GraphQLErrors{}).Error(); got != "" {
		t.Errorf("lista vazia = %q, esperado vazio", got)
	}

	only := gqlError(400, "unico", "ERROR")
	if got := (GraphQLErrors{only}).Error(); got != only.Error() {
		t.Errorf("lista de um = %q, esperado %q", got, only.Error())
	}

	many := GraphQLErrors{
		gqlError(400, "primeiro", "ERROR"),
		gqlError(500, "segundo", "ERROR"),
		gqlError(503, "terceiro", "ERROR"),
	}
	got := many.Error()
	if !strings.Contains(got, "primeiro") {
		t.Errorf("%q nao traz o primeiro erro", got)
	}
	if !strings.Contains(got, "2 other errors") {
		t.Errorf("%q nao traz a contagem dos demais", got)
	}
	if strings.Contains(got, "segundo") || strings.Contains(got, "terceiro") {
		t.Errorf("%q deveria resumir, nao listar", got)
	}
}
