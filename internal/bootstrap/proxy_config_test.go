package bootstrap

import "testing"

// TestResolveWebhookUseProxy removed — depends on pre-refactor globals (TODO #5)


func TestProxyConfigResponse(t *testing.T) {
	response := proxyConfigResponse("socks5://127.0.0.1:1080", false)

	if response["enabled"] != true {
		t.Fatalf("expected enabled true when proxy URL is set, got %v", response["enabled"])
	}
	if response["proxy_url"] != "socks5://127.0.0.1:1080" {
		t.Fatalf("unexpected proxy_url: %v", response["proxy_url"])
	}
	if response["webhook_use_proxy"] != false {
		t.Fatalf("expected webhook_use_proxy false, got %v", response["webhook_use_proxy"])
	}
}