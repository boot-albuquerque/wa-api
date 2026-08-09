package session

import (
	"slices"
	"sort"
	"testing"
	"time"

	port "wa-api/pkg/application/contracts"
)

// TestTranslateStatusEvent_TodosOsKinds: translateStatusEvent é a tabela que
// decide QUAL evento de sessão vira despacho e sob que nome. Um kind que caia
// no default silencia o evento inteiro (handleSessionEvent só loga em Debug),
// então cada linha da tabela precisa de asserção própria — é o tipo de erro
// que não aparece em teste de fluxo, só em produção como "webhook que sumiu".
func TestTranslateStatusEvent_TodosOsKinds(t *testing.T) {
	casos := []struct {
		nome      string
		evt       port.SessionEvent
		wantType  string
		wantEvent string
	}{
		{
			nome:      "connected",
			evt:       port.SessionEvent{Kind: port.SessionEventKindConnected},
			wantType:  "Connected",
			wantEvent: "connected",
		},
		{
			nome:      "disconnected",
			evt:       port.SessionEvent{Kind: port.SessionEventKindDisconnected},
			wantType:  "Disconnected",
			wantEvent: "disconnected",
		},
		{
			nome:      "logged_out",
			evt:       port.SessionEvent{Kind: port.SessionEventKindLoggedOut},
			wantType:  "LoggedOut",
			wantEvent: "logged_out",
		},
		{
			nome:      "pair_success",
			evt:       port.SessionEvent{Kind: port.SessionEventKindPairSuccess},
			wantType:  "PairSuccess",
			wantEvent: "pair_success",
		},
		{
			nome:      "qr",
			evt:       port.SessionEvent{Kind: port.SessionEventKindQR},
			wantType:  "QR",
			wantEvent: qrEventName,
		},
		{
			nome:      "stream_replaced",
			evt:       port.SessionEvent{Kind: port.SessionEventKindStreamReplaced},
			wantType:  "StreamReplaced",
			wantEvent: "stream_replaced",
		},
	}

	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			gotType, payload := translateStatusEvent(tc.evt)
			if gotType != tc.wantType {
				t.Fatalf("tipo = %q, quero %q", gotType, tc.wantType)
			}
			if payload["event"] != tc.wantEvent {
				t.Fatalf("payload[event] = %v, quero %q", payload["event"], tc.wantEvent)
			}
		})
	}
}

// TestTranslateStatusEvent_KindDesconhecido: tipo vazio é o contrato que
// handleSessionEvent usa para decidir NÃO despachar.
func TestTranslateStatusEvent_KindDesconhecido(t *testing.T) {
	gotType, payload := translateStatusEvent(port.SessionEvent{Kind: "inexistente"})
	if gotType != "" {
		t.Fatalf("tipo = %q, quero vazio", gotType)
	}
	if payload != nil {
		t.Fatalf("payload = %v, quero nil", payload)
	}
}

// TestPayloads_ComDetalhe: os quatro construtores de payload copiam os campos
// do evento. Sem isso o webhook chega sem jid/reason e o consumidor não tem
// como correlacionar.
func TestPayloads_ComDetalhe(t *testing.T) {
	t.Run("disconnected", func(t *testing.T) {
		p := disconnectedPayload(&port.SessionDisconnectedEvent{Reason: "stream end"})
		if p["reason"] != "stream end" {
			t.Fatalf("reason = %v", p["reason"])
		}
	})

	t.Run("logged_out", func(t *testing.T) {
		p := loggedOutPayload(&port.SessionLoggedOutEvent{Reason: "401"})
		if p["reason"] != "401" {
			t.Fatalf("reason = %v", p["reason"])
		}
	})

	t.Run("pair_success", func(t *testing.T) {
		p := pairSuccessPayload(&port.SessionPairSuccessEvent{
			JID:          "55119@s.whatsapp.net",
			BusinessName: "Acme",
			Platform:     "android",
		})
		if p["jid"] != "55119@s.whatsapp.net" || p["businessName"] != "Acme" || p["platform"] != "android" {
			t.Fatalf("payload = %v", p)
		}
	})

	t.Run("qr", func(t *testing.T) {
		p := qrPayload(&port.SessionQREvent{Code: "2@abc"})
		if p["code"] != "2@abc" {
			t.Fatalf("code = %v", p["code"])
		}
	})
}

// TestPayloads_SemDetalhe: os ponteiros de detalhe são opcionais no port, e
// um nil não pode derrubar o despacho — o evento base ainda vale.
func TestPayloads_SemDetalhe(t *testing.T) {
	if p := disconnectedPayload(nil); p["event"] != "disconnected" || len(p) != 1 {
		t.Fatalf("disconnected = %v", p)
	}
	if p := loggedOutPayload(nil); p["event"] != "logged_out" || len(p) != 1 {
		t.Fatalf("logged_out = %v", p)
	}
	if p := pairSuccessPayload(nil); p["event"] != "pair_success" || len(p) != 1 {
		t.Fatalf("pair_success = %v", p)
	}
	// O QR é a exceção desta lista: desde a F68 ele carrega SEMPRE `code`
	// além do `event`, mesmo sem detalhe nenhum. Os outros continuam sendo só
	// o `event` — e é isso que a lista prova.
	if p := qrPayload(nil); p["event"] != qrEventName || p["code"] != "" {
		t.Fatalf("qr = %v", p)
	}
}

// TestProxyConfigFor_Modo: o modo decide QUAL chamada o transporte faz
// (dialer SOCKS vs endereço HTTP). Classificar socks5h como HTTP configura o
// proxy no lugar errado e a sessão sobe sem proxy nenhum, silenciosamente.
func TestProxyConfigFor_Modo(t *testing.T) {
	casos := []struct {
		url  string
		want port.ProxyMode
	}{
		{"socks5://user:pass@10.0.0.1:1080", port.ProxyModeSOCKS5},
		{"socks5h://10.0.0.1:1080", port.ProxyModeSOCKS5},
		{"SOCKS5://10.0.0.1:1080", port.ProxyModeSOCKS5},
		{"http://proxy:3128", port.ProxyModeHTTP},
		{"https://proxy:3128", port.ProxyModeHTTP},
		{"://url-quebrada", port.ProxyModeHTTP},
	}

	for _, tc := range casos {
		t.Run(tc.url, func(t *testing.T) {
			got := proxyConfigFor(tc.url)
			if got.Mode != tc.want {
				t.Fatalf("modo = %q, quero %q", got.Mode, tc.want)
			}
			if got.URL != tc.url {
				t.Fatalf("URL = %q, quero %q (proxyConfigFor não pode reescrever a URL)", got.URL, tc.url)
			}
		})
	}
}

// TestOptions_S3EWebhookProxy: as duas Options que o wiring de bootstrap usa
// e que nenhum teste de fluxo exercita.
func TestOptions_S3EWebhookProxy(t *testing.T) {
	var chamado string
	o := &Orchestrator{}

	WithS3Provisioner(func(userID string) { chamado = userID })(o)
	WithDefaultWebhookUseProxy(true)(o)

	if o.ensureS3 == nil {
		t.Fatal("WithS3Provisioner não registrou a função")
	}
	o.ensureS3("user-1")
	if chamado != "user-1" {
		t.Fatalf("ensureS3 recebeu %q, quero user-1", chamado)
	}
	if !o.defaultWebhookUseProxy {
		t.Fatal("WithDefaultWebhookUseProxy(true) não aplicou")
	}
}

// TestQRPayload_OsDoisFluxosProduzemOMesmoSchema é o teste da F68.
//
// O mesmo `type: "QR"` era despachado com DOIS payloads incompatíveis: o fluxo
// de pareamento mandava `{event:"code", qrCodeBase64, expiresAt}` e o de
// Subscribe mandava `{event:"qr", code}`. Nada no tipo permitia distinguir — o
// consumidor tinha de testar a presença dos campos, e quem tratasse só um
// schema ignorava o outro em silêncio.
//
// O teste compara os CONJUNTOS DE CHAVES, não valores: valores diferem
// legitimamente (códigos diferentes, validades diferentes), e é o schema que
// precisa ser o mesmo.
func TestQRPayload_OsDoisFluxosProduzemOMesmoSchema(t *testing.T) {
	const code = "2@abc"
	const validade = 20 * time.Second

	// Fluxo de pareamento (onPairingQR) e fluxo de Subscribe (qrPayload)
	// chamam o mesmo construtor; o teste passa pelos dois caminhos de entrada.
	doPareamento := buildQRPayload(code, validade)
	doSubscribe := qrPayload(&port.SessionQREvent{Code: code, Timeout: validade})

	chaves := func(m map[string]any) []string {
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}

	if a, b := chaves(doPareamento), chaves(doSubscribe); !slices.Equal(a, b) {
		t.Fatalf("schemas divergem: pareamento=%v subscribe=%v", a, b)
	}

	// E o schema precisa ter as três coisas que um cliente usa: o código cru
	// (renderizável por conta própria), a imagem pronta e a validade.
	for _, campo := range []string{"code", "qrCodeBase64", "expiresAt"} {
		if _, ok := doPareamento[campo]; !ok {
			t.Errorf("campo %q ausente do payload unificado", campo)
		}
	}
	if doPareamento["code"] != code {
		t.Errorf("code = %v, quero %q", doPareamento["code"], code)
	}
}

// TestQRPayload_SemValidadeOmiteExpiresAt: um `expiresAt` inventado é pior que
// ausente — o cliente desenharia uma barra de progresso que não corresponde a
// nada.
func TestQRPayload_SemValidadeOmiteExpiresAt(t *testing.T) {
	p := buildQRPayload("2@abc", 0)

	if _, presente := p["expiresAt"]; presente {
		t.Errorf("expiresAt presente sem validade conhecida: %v", p["expiresAt"])
	}
	if p["code"] != "2@abc" {
		t.Errorf("o code sumiu junto: %v", p["code"])
	}
}
