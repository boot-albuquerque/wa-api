package whatsmeow

import (
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
)

// TestClientManager_HTTPLifecycle: Set → Get → Delete para HTTP clients.
func TestClientManager_HTTPLifecycle(t *testing.T) {
	cm := NewClientManager()
	hc := resty.New()
	cm.SetHTTPClient("u1", hc)
	if got := cm.GetHTTPClient("u1"); got != hc {
		t.Error("GetHTTPClient returned different client")
	}
	cm.DeleteHTTPClient("u1")
	if got := cm.GetHTTPClient("u1"); got != nil {
		t.Error("GetHTTPClient after delete returned non-nil")
	}
}

// TestClientManager_ProvisionWebhookClient registra um cliente configurado.
func TestClientManager_ProvisionWebhookClient(t *testing.T) {
	cm := NewClientManager()
	if err := cm.ProvisionWebhookClient("u1", ""); err != nil {
		t.Fatalf("ProvisionWebhookClient = %v, want nil", err)
	}
	got := cm.GetHTTPClient("u1")
	if got == nil {
		t.Fatal("ProvisionWebhookClient did not register an HTTP client")
	}
	if got.GetClient().Timeout != 30*time.Second {
		t.Errorf("timeout = %v, want %v", got.GetClient().Timeout, 30*time.Second)
	}
}

// TestClientManager_ProvisionWebhookClient_WithProxy cobre o ramo de proxy.
func TestClientManager_ProvisionWebhookClient_WithProxy(t *testing.T) {
	cm := NewClientManager()
	if err := cm.ProvisionWebhookClient("u1", "http://127.0.0.1:3128"); err != nil {
		t.Fatalf("ProvisionWebhookClient(proxy) = %v, want nil", err)
	}
	if cm.GetHTTPClient("u1") == nil {
		t.Fatal("ProvisionWebhookClient(proxy) did not register an HTTP client")
	}
}

// TestWebhookTLSSkipVerify_DefaultFalse: sem env var, verificação TLS ativa.
func TestWebhookTLSSkipVerify_DefaultFalse(t *testing.T) {
	if webhookTLSSkipVerify() {
		t.Error("webhookTLSSkipVerify() = true without env var, want false")
	}
}
