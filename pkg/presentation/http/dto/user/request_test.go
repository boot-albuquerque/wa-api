package user_test

import (
	"strings"
	"testing"

	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
	dtouser "wa-api/pkg/presentation/http/dto/user"
)

// O alias `chat` era testado em pkg/domain/chat_target_test.go enquanto os
// tipos de domínio ERAM o corpo HTTP. Depois da migração de DTO, quem o lê é
// este pacote, e é aqui que a cobertura tem de viver — senão a tabela de lá
// encolheria e ninguém veria que duas rotas ficaram sem teste do alias.

// decodePorRota imita o que decodeRequest faz na fronteira: descodifica e,
// quando o destino implementa ChatResolver, resolve o alias.
//
// A regra imitada é a REAL — domain.DecodeRequest, em
// pkg/domain/chat_target.go:32-40. Um dublê que só descodificasse seria mais
// simples que a produção e abençoaria o campo `chat` a nunca chegar a lado
// nenhum (ARMADILHAS #1).
func decodePorRota(t *testing.T, body string, dst any) {
	t.Helper()
	if err := domain.DecodeRequest(strings.NewReader(body), dst); err != nil {
		t.Fatalf("descodificação falhou: %v", err)
	}
}

func TestBlockUserRequest_AliasChat(t *testing.T) {
	casos := []struct {
		nome  string
		corpo string
		quero string
	}{
		{"só o alias", `{"chat":"5511999999999@s.whatsapp.net"}`, "5511999999999@s.whatsapp.net"},
		{"só o campo", `{"phone":"5511999999999@s.whatsapp.net"}`, "5511999999999@s.whatsapp.net"},
		// O campo VENCE quando os dois vêm: quem manda ambos de propósito está
		// a pedir ambiguidade, não a exprimir intenção.
		{"os dois", `{"phone":"campo@s.whatsapp.net","chat":"alias@s.whatsapp.net"}`, "campo@s.whatsapp.net"},
		{"nenhum", `{}`, ""},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			var req dtouser.BlockUserRequest
			decodePorRota(t, c.corpo, &req)
			if req.Phone != c.quero {
				t.Errorf("Phone = %q, quero %q", req.Phone, c.quero)
			}
		})
	}
}

func TestGetAvatarRequest_AliasChat(t *testing.T) {
	var req dtouser.GetAvatarRequest
	decodePorRota(t, `{"chat":"5511999999999@s.whatsapp.net"}`, &req)
	if req.Phone != "5511999999999@s.whatsapp.net" {
		t.Errorf("Phone = %q: o alias `chat` não foi resolvido", req.Phone)
	}
}

// TestNomesDePedidoAceitamCaixaDiferente é uma MEDIÇÃO, não um desejo.
//
// Escrevi primeiro o teste contrário — "as chaves antigas `Phone`/`JID`
// deixaram de ser lidas" — e ele FALHOU: o `encoding/json` do Go casa nomes de
// campo SEM distinguir maiúsculas, então a etiqueta `json:"phone"` continua a
// aceitar `{"Phone": …}`. Não há corte a seco possível quando o nome antigo e
// o novo diferem só na caixa.
//
// O que isto significa para o contrato, escrito com todas as letras: a chave
// CANÓNICA e documentada é a minúscula, e é a única que a especificação
// OpenAPI descreve; a antiga continua a funcionar por uma propriedade do
// descodificador, não por decisão nossa. Um cliente que dependa disso está a
// depender do Go, não da API.
//
// Fica travado aqui para que a próxima pessoa que tente "remover a chave
// antiga" saiba, em dez segundos, que o caminho é `DisallowUnknownFields` com
// casamento estrito — não uma mudança de etiqueta.
func TestNomesDePedidoAceitamCaixaDiferente(t *testing.T) {
	t.Run("a canónica é lida", func(t *testing.T) {
		var req dtouser.BlockUserRequest
		decodePorRota(t, `{"phone":"5511","jid":"5511@lid"}`, &req)
		if req.Phone != "5511" || req.JID != "5511@lid" {
			t.Errorf("a chave canónica não foi lida: %+v", req)
		}
	})
	t.Run("a antiga também, e isso é do encoding/json", func(t *testing.T) {
		var req dtouser.BlockUserRequest
		decodePorRota(t, `{"Phone":"5511","JID":"5511@lid"}`, &req)
		if req.Phone != "5511" || req.JID != "5511@lid" {
			t.Errorf("o casamento sem distinção de caixa deixou de valer: %+v", req)
		}
	})
	t.Run("preview em minúscula", func(t *testing.T) {
		var req dtouser.GetAvatarRequest
		decodePorRota(t, `{"phone":"5511","preview":true}`, &req)
		if req.Phone != "5511" || !req.Preview {
			t.Errorf("a chave canónica não foi lida: %+v", req)
		}
	})
}

func TestValidate(t *testing.T) {
	casos := []struct {
		nome     string
		validate func() error
		quero    string // "" significa "sem erro"
	}{
		{"check sem telefone", dtouser.CheckUserRequest{}.Validate, dtouser.CodeMissingPhone},
		{"check com telefone", dtouser.CheckUserRequest{Phone: []string{"5511"}}.Validate, ""},
		{"block sem alvo", dtouser.BlockUserRequest{}.Validate, dtouser.CodeMissingPhoneOrJID},
		{"block por telefone", dtouser.BlockUserRequest{Phone: "5511"}.Validate, ""},
		{"block por jid", dtouser.BlockUserRequest{JID: "5511@lid"}.Validate, ""},
		{"avatar sem telefone", dtouser.GetAvatarRequest{}.Validate, dtouser.CodeMissingPhone},
		{"avatar com telefone", dtouser.GetAvatarRequest{Phone: "5511"}.Validate, ""},
		{"privacidade desconhecida",
			dtouser.SetPrivacySettingRequest{PrivacySetting: "xpto", Value: "all"}.Validate,
			domain.InvalidPrivacySettingCode},
		{"valor não aceite para a configuração",
			dtouser.SetPrivacySettingRequest{PrivacySetting: "readreceipts", Value: "contacts"}.Validate,
			domain.InvalidPrivacyValueCode},
		{"privacidade válida",
			dtouser.SetPrivacySettingRequest{PrivacySetting: "readreceipts", Value: "none"}.Validate, ""},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			err := c.validate()
			if c.quero == "" {
				if err != nil {
					t.Fatalf("Validate = %v, quero nil", err)
				}
				return
			}
			if err == nil {
				t.Fatal("Validate = nil, quero recusa")
			}
			// O código é o que um cliente lê para ramificar; a mensagem pode
			// mudar. Por isso a asserção é sobre ele.
			var appErr *apperr.AppError
			if !asAppError(err, &appErr) {
				t.Fatalf("Validate = %T, quero *apperr.AppError para que a fronteira responda 400", err)
			}
			if appErr.Code != c.quero {
				t.Errorf("error.code = %q, quero %q", appErr.Code, c.quero)
			}
		})
	}
}

// TestToDomainLevaTodoCampo trava o mapeamento de ida: um campo esquecido em
// ToDomain é um pedido aceite e ignorado em silêncio, com 200.
func TestToDomainLevaTodoCampo(t *testing.T) {
	t.Run("check", func(t *testing.T) {
		got := dtouser.CheckUserRequest{Phone: []string{"a", "b"}}.ToDomain()
		if len(got.Phone) != 2 || got.Phone[0] != "a" || got.Phone[1] != "b" {
			t.Errorf("ToDomain = %+v", got)
		}
	})
	t.Run("block", func(t *testing.T) {
		req := dtouser.BlockUserRequest{Phone: "5511", JID: "5511@lid"}
		if got := req.ToBlockDomain(); got.Phone != "5511" || got.JID != "5511@lid" {
			t.Errorf("ToBlockDomain = %+v", got)
		}
		if got := req.ToUnblockDomain(); got.Phone != "5511" || got.JID != "5511@lid" {
			t.Errorf("ToUnblockDomain = %+v", got)
		}
	})
	t.Run("avatar", func(t *testing.T) {
		got := dtouser.GetAvatarRequest{Phone: "5511", Preview: true}.ToDomain()
		if got.Phone != "5511" || !got.Preview {
			t.Errorf("ToDomain = %+v", got)
		}
	})
	t.Run("privacidade", func(t *testing.T) {
		got := dtouser.SetPrivacySettingRequest{PrivacySetting: "last", Value: "all"}.ToDomain()
		if got.PrivacySetting != "last" || got.Value != "all" {
			t.Errorf("ToDomain = %+v", got)
		}
	})
}

func asAppError(err error, target **apperr.AppError) bool {
	e, ok := err.(*apperr.AppError)
	if ok {
		*target = e
	}
	return ok
}
