package domain_test

import (
	"encoding/json"
	"testing"

	"wa-api/pkg/domain"
)

// Testes da F210: a API LIA `s3Config`/`proxyConfig` e DEVOLVIA
// `s3_config`/`proxy_config`, então o ciclo mais natural que existe — ler o
// utilizador, mudar um campo, reenviar — era ignorado em SILÊNCIO com 200.
//
// Medido em campo a 2026-08-22, antes da correção:
//
//	PUT {"name":"lucas","s3_config":{"bucket":"snake-case"}} -> 200, bucket=""
//	PUT {"name":"lucas","s3Config":{"bucket":"camel-case"}}  -> 200, bucket="camel-case"
//
// Pior que um 500: o 200 afirma sucesso. Quem configura S3 por esta rota fica
// convencido de que ficou, e só descobre ao tentar enviar média — ou nunca.

// TestEditUserRequest_AceitaOsDoisNomes é o teste do defeito, com os dois
// nomes que a API mistura.
func TestEditUserRequest_AceitaOsDoisNomes(t *testing.T) {
	casos := []struct {
		nome  string
		corpo string
	}{
		{"snake_case, o que a RESPOSTA devolve", `{"name":"x","s3_config":{"bucket":"b"},"proxy_config":{"proxyUrl":"http://p"}}`},
		{"camelCase, o que o README documenta", `{"name":"x","s3Config":{"bucket":"b"},"proxyConfig":{"proxyUrl":"http://p"}}`},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			var req domain.EditUserRequest
			if err := json.Unmarshal([]byte(c.corpo), &req); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if req.S3Config == nil {
				t.Fatal("S3Config chegou nil: o campo é ignorado em silêncio e o " +
					"PUT devolve 200 sem fazer nada (F210)")
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
		})
	}
}

// TestAddUserRequest_AceitaOsDoisNomes: aplicar só à edição criaria uma
// assimetria nova — PUT a aceitar dois nomes e POST a aceitar um.
func TestAddUserRequest_AceitaOsDoisNomes(t *testing.T) {
	var req domain.AddUserRequest
	if err := json.Unmarshal([]byte(`{"name":"x","s3_config":{"bucket":"b"}}`), &req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if req.S3Config == nil || req.S3Config.Bucket != "b" {
		t.Errorf("POST não aceita snake_case: %+v", req.S3Config)
	}
}

// TestEditUserRequest_CamelVenceQuandoAmbosVem: quem envia os dois nomes no
// mesmo corpo está a pedir ambiguidade. O camelCase ganha porque é o
// documentado (README.md:289-311).
func TestEditUserRequest_CamelVenceQuandoAmbosVem(t *testing.T) {
	var req domain.EditUserRequest
	corpo := `{"s3Config":{"bucket":"camel"},"s3_config":{"bucket":"snake"}}`
	if err := json.Unmarshal([]byte(corpo), &req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if req.S3Config == nil || req.S3Config.Bucket != "camel" {
		t.Errorf("bucket = %+v, quero o camelCase (o documentado)", req.S3Config)
	}
}

// TestEditUserRequest_SemConfigContinuaNil é o limite, e é o que impede a
// correção de virar outro defeito: um corpo que não traz os campos não pode
// passar a trazê-los vazios, senão APAGA a configuração de quem só queria
// mudar o nome.
func TestEditUserRequest_SemConfigContinuaNil(t *testing.T) {
	var req domain.EditUserRequest
	if err := json.Unmarshal([]byte(`{"name":"so-o-nome"}`), &req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if req.S3Config != nil {
		t.Errorf("S3Config = %+v num corpo que não o traz: isto apagaria a "+
			"configuração existente", *req.S3Config)
	}
	if req.ProxyConfig != nil {
		t.Errorf("ProxyConfig = %+v num corpo que não o traz", *req.ProxyConfig)
	}
}

// TestEditUserRequest_RoundTrip é o formato que a entrada do HOUSEKEEP pedia:
// ler a forma que a API DEVOLVE e reenviá-la sem lhe tocar.
//
// É o único que apanha assimetria de nome. Asserções sobre cada lado em
// separado passam com os dois nomes divergentes — foi por isso que o defeito
// sobreviveu.
func TestEditUserRequest_RoundTrip(t *testing.T) {
	// A forma da RESPOSTA, tal como session.go:66-67 a emite.
	daResposta := `{"name":"lucas","s3_config":{"enabled":true,"bucket":"meu-balde"},"proxy_config":{"enabled":true,"proxyUrl":"http://p"}}`

	var req domain.EditUserRequest
	if err := json.Unmarshal([]byte(daResposta), &req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if req.S3Config == nil || !req.S3Config.Enabled || req.S3Config.Bucket != "meu-balde" {
		t.Errorf("o corpo devolvido pela API não volta a entrar: %+v", req.S3Config)
	}
	if req.ProxyConfig == nil || req.ProxyConfig.ProxyURL != "http://p" {
		t.Errorf("o proxy devolvido pela API não volta a entrar: %+v", req.ProxyConfig)
	}
}
