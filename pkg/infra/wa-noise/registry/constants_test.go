package registry

import (
	"testing"
	"time"

	waclient "wa-api/pkg/infra/wa-noise/client"
)

// TestTimeouts_SaoPositivos: um timeout zero em context.WithTimeout expira
// imediatamente — seria uma quebra silenciosa de todo caminho que o usa.
func TestTimeouts_SaoPositivos(t *testing.T) {
	for name, d := range map[string]time.Duration{
		"webhookClientTimeout": webhookClientTimeout,
		"wsBroadcastTimeout":   wsBroadcastTimeout,
	} {
		if d <= 0 {
			t.Errorf("%s = %v, want > 0", name, d)
		}
	}
}

// TestWSBroadcastTimeout_MenorQueRequest: o fan-out WS percorre as conexões
// em série, então seu teto por conexão tem de ser bem menor que o de uma
// requisição ao servidor do WhatsApp.
func TestWSBroadcastTimeout_MenorQueRequest(t *testing.T) {
	if wsBroadcastTimeout >= waclient.RequestTimeout {
		t.Errorf("wsBroadcastTimeout (%v) >= waclient.RequestTimeout (%v)", wsBroadcastTimeout, waclient.RequestTimeout)
	}
}

// TestEnvWebhookTLSSkipVerify_Nome trava o nome da variável de ambiente: ele
// é contrato com o operador e com pkg/bootstrap/lifecycle.go.
func TestEnvWebhookTLSSkipVerify_Nome(t *testing.T) {
	if envWebhookTLSSkipVerify != "WA_API_WEBHOOK_TLS_SKIP_VERIFY" {
		t.Errorf("envWebhookTLSSkipVerify = %q", envWebhookTLSSkipVerify)
	}
}
