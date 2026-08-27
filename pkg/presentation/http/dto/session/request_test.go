package session_test

import (
	"encoding/json"
	"testing"

	"wa-api/pkg/presentation/http/contracttest"
	dtosession "wa-api/pkg/presentation/http/dto/session"
	dtostorage "wa-api/pkg/presentation/http/dto/storage"
	dtowebhook "wa-api/pkg/presentation/http/dto/webhook"
)

// validator é a forma que todo DTO de pedido desta migração tem.
type validator interface{ Validate() error }

// requestDTOs enumera os DTO de pedido das três famílias migradas em conjunto,
// NOME POR NOME. Uma afirmação agregada ("todos os DTO validam") não é
// evidência de nada — é a lista que torna visível o DTO acrescentado sem
// Validate.
func requestDTOs() map[string]validator {
	return map[string]validator{
		"session.PairPhoneRequest":          dtosession.PairPhoneRequest{},
		"session.SetStatusMessageRequest":   dtosession.SetStatusMessageRequest{},
		"session.RequestHistorySyncRequest": dtosession.RequestHistorySyncRequest{},
		"session.SyncContactRosterRequest":  dtosession.SyncContactRosterRequest{},
		"session.PublishStatusImageRequest": dtosession.PublishStatusImageRequest{},
		"session.PublishStatusVideoRequest": dtosession.PublishStatusVideoRequest{},
		"session.PublishStatusAudioRequest": dtosession.PublishStatusAudioRequest{},
		"webhook.ConfigRequest":             dtowebhook.ConfigRequest{},
		"webhook.HistoryRequest":            dtowebhook.HistoryRequest{},
		"storage.S3ConfigRequest":           dtostorage.S3ConfigRequest{},
		"storage.HmacConfigRequest":         dtostorage.HmacConfigRequest{},
		"storage.ProxyConfigRequest":        dtostorage.ProxyConfigRequest{},
	}
}

// TestValidateNaoAcrescentaRecusa trava a DECISÃO de o Validate desta migração
// estar vazio, em vez de deixar a decisão implícita num método que ninguém
// chama.
//
// Toda a taxonomia de recusa desta família já vive nos use cases, com código
// de erro público (`missing_phone`, `missing_body`, `missing_proxy_url`,
// `hmac_key_too_short`, …). Duplicá-la na fronteira daria ao mesmo pedido DOIS
// sítios de onde responder 400, e os dois divergem no momento em que um deles
// for editado.
//
// O dia em que uma recusa PRECISAR de viver aqui — porque nenhum use case a
// pode ver —, este teste falha e obriga quem a escreve a ligar a chamada no
// manipulador em vez de a deixar num método morto.
func TestValidateNaoAcrescentaRecusa(t *testing.T) {
	for nome, dto := range requestDTOs() {
		if err := dto.Validate(); err != nil {
			t.Errorf("%s.Validate() = %v; esta família decidiu que a recusa vive no use case. "+
				"Se a decisão mudou, ligue a chamada a Validate no manipulador da rota — "+
				"um Validate que recusa e que ninguém chama é pior que nenhum", nome, err)
		}
	}
}

// TestPedidosUsamNomesCanonicos afirma a regra de nomes sobre os corpos de
// PEDIDO, e não só sobre as respostas: a regra de
// docs/HTTP-DTO-CONVENTIONS.md §8 vale para os dois lados.
//
// Serializa o zero de cada DTO — que emite TODAS as chaves, porque nenhum
// deles tem omitempty — e passa o resultado pelo mesmo helper partilhado que
// os testes de rota usam.
func TestPedidosUsamNomesCanonicos(t *testing.T) {
	for nome, dto := range requestDTOs() {
		corpo, err := json.Marshal(dto)
		if err != nil {
			t.Fatalf("%s: marshal: %v", nome, err)
		}
		t.Run(nome, func(t *testing.T) {
			contracttest.AssertPublicJSONUsesCanonicalNaming(t, corpo)
		})
	}
}
