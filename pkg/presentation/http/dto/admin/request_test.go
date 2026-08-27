package admin_test

import (
	"encoding/json"
	"testing"

	"wa-api/pkg/domain/apperr"
	dtoadmin "wa-api/pkg/presentation/http/dto/admin"
)

// Estes testes substituem pkg/domain/user_config_alias_test.go, que morreu com
// o alias que ele protegia.
//
// A F210 era esta: a API LIA `s3Config`/`proxyConfig` em camelCase e
// DEVOLVIA `s3_config`/`proxy_config` em snake_case, então o ciclo mais
// natural que existe — ler o utilizador, mudar um campo, reenviar — era
// ignorado em SILÊNCIO com 200. A correcção de então foi aceitar OS DOIS
// nomes na leitura. A migração para DTO alinhou os dois lados em snake_case,
// que resolve a mesma falha sem manter duas grafias vivas — e a grafia dupla
// é justamente o que o corte a seco proíbe (HTTP-DTO-CONVENTIONS §11).
//
// O que estes testes travam é que o alinhamento REALMENTE aconteceu: o corpo
// com os nomes canónicos tem de chegar aos campos, e o antigo não.

func TestEditUserRequest_LeOsNomesCanonicos(t *testing.T) {
	corpo := `{"name":"x","s3_config":{"bucket":"b"},"proxy_config":{"proxy_url":"http://p"}}`

	var req dtoadmin.EditUserRequest
	if err := json.Unmarshal([]byte(corpo), &req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if req.S3Config == nil {
		t.Fatal("S3Config chegou nil: o campo seria ignorado em silêncio e o PUT devolveria 200 sem fazer nada (F210)")
	}
	if req.S3Config.Bucket != "b" {
		t.Errorf("bucket = %q", req.S3Config.Bucket)
	}
	if req.ProxyConfig == nil {
		t.Fatal("ProxyConfig chegou nil: mesmo defeito, mesmo campo")
	}
	if req.ProxyConfig.ProxyURL != "http://p" {
		t.Errorf("proxyURL = %q", req.ProxyConfig.ProxyURL)
	}
}

// TestAddUserRequest_LeOsNomesCanonicos: aplicar só à edição criaria a
// assimetria que a F210 já tinha corrigido — PUT a ler um nome e POST outro.
func TestAddUserRequest_LeOsNomesCanonicos(t *testing.T) {
	var req dtoadmin.AddUserRequest
	corpo := `{"name":"x","token":"t","hmac_key":"k","s3_config":{"bucket":"b","access_key":"ak","secret_key":"sk","path_style":true}}`
	if err := json.Unmarshal([]byte(corpo), &req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if req.HmacKey != "k" {
		t.Errorf("HmacKey = %q, queria ler `hmac_key`", req.HmacKey)
	}
	if req.S3Config == nil {
		t.Fatal("S3Config nil: `s3_config` não foi lido")
	}
	if req.S3Config.Bucket != "b" || req.S3Config.AccessKey != "ak" ||
		req.S3Config.SecretKey != "sk" || !req.S3Config.PathStyle {
		t.Errorf("s3 = %+v", *req.S3Config)
	}
}

// TestEditUserRequest_NomeAntigoNaoEhLido é a metade que o teste acima não
// prova: o corte é a seco, e o camelCase de antes NÃO tem de continuar a
// funcionar. Sem esta asserção, um `UnmarshalJSON` de alias reintroduzido
// passaria em tudo o resto.
func TestEditUserRequest_NomeAntigoNaoEhLido(t *testing.T) {
	var req dtoadmin.EditUserRequest
	corpo := `{"s3Config":{"bucket":"b"},"proxyConfig":{"proxyUrl":"http://p"}}`
	if err := json.Unmarshal([]byte(corpo), &req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if req.S3Config != nil || req.ProxyConfig != nil {
		t.Errorf("o camelCase antigo ainda é lido: s3=%+v proxy=%+v — o corte é a seco, "+
			"e duas grafias vivas é exactamente o que ele proíbe", req.S3Config, req.ProxyConfig)
	}
}

// TestEditUserRequest_SemConfigContinuaNil é o limite que impede a correcção
// de virar outro defeito: um corpo que não traz os campos não pode passar a
// trazê-los vazios, senão APAGA a configuração de quem só queria mudar o nome.
func TestEditUserRequest_SemConfigContinuaNil(t *testing.T) {
	var req dtoadmin.EditUserRequest
	if err := json.Unmarshal([]byte(`{"name":"so-o-nome"}`), &req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if req.S3Config != nil {
		t.Errorf("S3Config = %+v num corpo que não o traz: isto apagaria a configuração existente", *req.S3Config)
	}
	if req.ProxyConfig != nil {
		t.Errorf("ProxyConfig = %+v num corpo que não o traz", *req.ProxyConfig)
	}
	if req.ToDomain("u1").S3Config != nil || req.ToDomain("u1").ProxyConfig != nil {
		t.Error("ToDomain materializou uma configuração que o corpo não trazia")
	}
}

// --- F218: history distingue ausente de zero ------------------------------

func TestEditUserRequest_History(t *testing.T) {
	casos := []struct {
		nome   string
		corpo  string
		quero  int
		wantNi bool // quero nil
	}{
		{"zero é um valor, não ausência", `{"history":0}`, 0, false},
		{"ausente fica nil", `{"name":"x"}`, 0, true},
		{"positivo chega como valor", `{"history":9999}`, 9999, false},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			var req dtoadmin.EditUserRequest
			if err := json.Unmarshal([]byte(c.corpo), &req); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if c.wantNi {
				if req.History != nil {
					t.Errorf("History = %d para um corpo que não o menciona — isto sobrescreveria o banco", *req.History)
				}
				return
			}
			if req.History == nil {
				t.Fatalf("History = nil para %s — o zero foi tratado como ausente (F218)", c.corpo)
			}
			if *req.History != c.quero {
				t.Errorf("History = %d, quero %d", *req.History, c.quero)
			}
			if got := req.ToDomain("u1").History; got == nil || *got != c.quero {
				t.Errorf("ToDomain().History = %v, quero %d — o ponteiro tem de atravessar", got, c.quero)
			}
		})
	}
}

// --- webhook_use_proxy: mesmo problema, outro campo -----------------------

// TestProxyConfigRequest_WebhookUseProxyDistingueAusenteDeFalso: ausente
// significa "mantém o padrão true", e false significa "pára de mandar o
// webhook pelo proxy". Com um bool simples os dois seriam o mesmo estado.
func TestProxyConfigRequest_WebhookUseProxyDistingueAusenteDeFalso(t *testing.T) {
	casos := []struct {
		corpo string
		quero *bool
	}{
		{`{"proxy_config":{"enabled":true}}`, nil},
		{`{"proxy_config":{"enabled":true,"webhook_use_proxy":false}}`, boolPtr(false)},
		{`{"proxy_config":{"enabled":true,"webhook_use_proxy":true}}`, boolPtr(true)},
	}
	for _, c := range casos {
		var req dtoadmin.AddUserRequest
		if err := json.Unmarshal([]byte(c.corpo), &req); err != nil {
			t.Fatalf("decode %s: %v", c.corpo, err)
		}
		got := req.ToDomain().ProxyConfig.WebhookUseProxy
		switch {
		case c.quero == nil && got != nil:
			t.Errorf("%s: WebhookUseProxy = %v, quero nil (não informado)", c.corpo, *got)
		case c.quero != nil && got == nil:
			t.Errorf("%s: WebhookUseProxy = nil, quero %v", c.corpo, *c.quero)
		case c.quero != nil && *got != *c.quero:
			t.Errorf("%s: WebhookUseProxy = %v, quero %v", c.corpo, *got, *c.quero)
		}
	}
}

func boolPtr(b bool) *bool { return &b }

// --- Validate -------------------------------------------------------------

// TestAddUserRequest_Validate: a recusa tem de trazer o MESMO código que o
// use case devolve para a mesma condição. Rejeitar mais cedo não pode mudar
// aquilo em que o cliente ramifica.
func TestAddUserRequest_Validate(t *testing.T) {
	casos := []struct {
		nome     string
		req      dtoadmin.AddUserRequest
		wantCode string
	}{
		{"sem nome", dtoadmin.AddUserRequest{Token: "t"}, "missing_name_or_token"},
		{"sem token", dtoadmin.AddUserRequest{Name: "n"}, "missing_name_or_token"},
		{"completo", dtoadmin.AddUserRequest{Name: "n", Token: "t"}, ""},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			err := c.req.Validate()
			if c.wantCode == "" {
				if err != nil {
					t.Fatalf("Validate = %v, queria nil", err)
				}
				return
			}
			var appErr *apperr.AppError
			if !asAppError(err, &appErr) {
				t.Fatalf("Validate = %v, queria *apperr.AppError (senão a fronteira devolve 500 genérico)", err)
			}
			if appErr.Code != c.wantCode {
				t.Errorf("code = %q, quero %q", appErr.Code, c.wantCode)
			}
			if appErr.Category != apperr.CategoryValidation {
				t.Errorf("category = %v, quero validation (é ela que faz o status ser 400)", appErr.Category)
			}
		})
	}
}

// asAppError é errors.As com a assinatura estreita que este ficheiro usa.
func asAppError(err error, target **apperr.AppError) bool {
	if err == nil {
		return false
	}
	e, ok := err.(*apperr.AppError)
	if ok {
		*target = e
	}
	return ok
}
