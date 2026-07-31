package messaging

// ResolveWebhookUseProxy determines whether to use proxy for webhook delivery.
// If perUser is not nil, returns its value; otherwise returns the global default.
func ResolveWebhookUseProxy(perUser *bool, globalDefault *bool) bool {
	if perUser != nil {
		return *perUser
	}
	return *globalDefault
}

// ProxyConfigResponse returns a formatted proxy configuration response.
func ProxyConfigResponse(proxyURL string, webhookUseProxy bool) map[string]interface{} {
	return map[string]interface{}{
		"enabled":           proxyURL != "",
		"proxy_url":         proxyURL,
		"webhook_use_proxy": webhookUseProxy,
	}
}
