package messaging

import "testing"

// TestResolveWebhookUseProxy fixa a precedencia: a preferencia por usuario
// vence o padrao global, inclusive quando ela e' `false` — o bug classico
// aqui e' tratar `false` como "nao configurado".
func TestResolveWebhookUseProxy(t *testing.T) {
	yes, no := true, false

	tests := []struct {
		name          string
		perUser       *bool
		globalDefault bool
		want          bool
	}{
		{"sem preferencia herda o global habilitado", nil, true, true},
		{"sem preferencia herda o global desabilitado", nil, false, false},
		{"preferencia true vence global false", &yes, false, true},
		{"preferencia false vence global true", &no, true, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			global := tc.globalDefault
			if got := ResolveWebhookUseProxy(tc.perUser, &global); got != tc.want {
				t.Errorf("ResolveWebhookUseProxy() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestProxyConfigResponse(t *testing.T) {
	tests := []struct {
		name            string
		proxyURL        string
		webhookUseProxy bool
		wantEnabled     bool
	}{
		{"url vazia desabilita", "", false, false},
		{"url preenchida habilita", "http://proxy.local:3128", true, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ProxyConfigResponse(tc.proxyURL, tc.webhookUseProxy)

			if got["enabled"] != tc.wantEnabled {
				t.Errorf("enabled = %v, want %v", got["enabled"], tc.wantEnabled)
			}
			if got["proxy_url"] != tc.proxyURL {
				t.Errorf("proxy_url = %v, want %q", got["proxy_url"], tc.proxyURL)
			}
			if got["webhook_use_proxy"] != tc.webhookUseProxy {
				t.Errorf("webhook_use_proxy = %v, want %v", got["webhook_use_proxy"], tc.webhookUseProxy)
			}
		})
	}
}
