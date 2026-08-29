package webhook

import (
	"testing"

	"github.com/go-resty/resty/v2"
)

// TestClientManager_HTTPLifecycle: Set → Get → Delete para HTTP clients.
func TestRegistry_HTTPLifecycle(t *testing.T) {
	r := New()
	hc := resty.New()
	r.Set("u1", hc)
	if got := r.Get("u1"); got != hc {
		t.Error("GetHTTPClient returned different client")
	}
	r.Delete("u1")
	if got := r.Get("u1"); got != nil {
		t.Error("GetHTTPClient after delete returned non-nil")
	}
}

// TestClientManager_ProvisionWebhookClient registra um cliente configurado.
func TestRegistry_ProvisionWebhookClient(t *testing.T) {
	r := New()
	if err := r.Provision("u1", ""); err != nil {
		t.Fatalf("ProvisionWebhookClient = %v, want nil", err)
	}
	got := r.Get("u1")
	if got == nil {
		t.Fatal("ProvisionWebhookClient did not register an HTTP client")
	}
	if got.GetClient().Timeout != clientTimeout {
		t.Errorf("timeout = %v, want %v", got.GetClient().Timeout, clientTimeout)
	}
}

// TestClientManager_ProvisionWebhookClient_WithProxy cobre o ramo de proxy.
func TestRegistry_ProvisionWebhookClient_WithProxy(t *testing.T) {
	r := New()
	if err := r.Provision("u1", "http://127.0.0.1:3128"); err != nil {
		t.Fatalf("ProvisionWebhookClient(proxy) = %v, want nil", err)
	}
	if r.Get("u1") == nil {
		t.Fatal("ProvisionWebhookClient(proxy) did not register an HTTP client")
	}
}

// TestWebhookTLSSkipVerify_DefaultFalse: sem env var, verificação TLS ativa.
func TestWebhookTLSSkipVerify_DefaultFalse(t *testing.T) {
	if tlsSkipVerify() {
		t.Error("tlsSkipVerify() = true without env var, want false")
	}
}
