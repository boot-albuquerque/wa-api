package registry

import (
	"crypto/tls"
	"os"
	"strings"
	"sync"

	"github.com/go-resty/resty/v2"
	"github.com/rs/zerolog/log"
)

// webhookTLSSkipVerify reports whether outgoing webhook deliveries should
// skip TLS certificate verification. Mirrors pkg/bootstrap/lifecycle.go's
// webhookTLSSkipVerify (same env var, same insecure-opt-in semantics) —
// duplicated here rather than imported because pkg/infra/wa-noise must not
// depend on pkg/bootstrap (see SessionAttachHook design in the plan).
var webhookTLSSkipVerify = sync.OnceValue(func() bool {
	v := strings.ToLower(os.Getenv(envWebhookTLSSkipVerify))
	skip := v == "true" || v == "1"
	if skip {
		log.Warn().
			Str("env", envWebhookTLSSkipVerify).
			Msg("INSECURE: webhook TLS certificate verification is DISABLED by explicit configuration. " +
				"Webhook deliveries are vulnerable to man-in-the-middle attacks. Unset this variable in production.")
	}
	return skip
})

// ProvisionWebhookClient monta o cliente HTTP de entrega de webhook para
// userID, seguindo a mesma configuração hoje montada inline em
// lifecycle.go:234-291 (redirect policy, timeout, TLS, proxy opcional) e
// registra o resultado via SetHTTPClient. proxyURL vazio entrega sem proxy.
func (cm *ClientManager) ProvisionWebhookClient(userID string, proxyURL string) error {
	webhookClient := resty.New()
	webhookClient.SetRedirectPolicy(resty.FlexibleRedirectPolicy(15))
	webhookClient.SetTimeout(webhookClientTimeout)
	webhookClient.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: webhookTLSSkipVerify()}) //nolint:gosec // opt-in explícito via env var, ver webhookTLSSkipVerify
	webhookClient.OnError(func(req *resty.Request, err error) {
		if v, ok := err.(*resty.ResponseError); ok {
			log.Debug().Str("response", v.Response.String()).Msg("resty error")
			log.Error().Err(v.Err).Msg("resty error")
		}
	})

	if proxyURL != "" {
		webhookClient.SetProxy(proxyURL)
	}

	cm.SetHTTPClient(userID, webhookClient)
	return nil
}

func (cm *ClientManager) SetHTTPClient(userID string, client *resty.Client) {
	cm.Lock()
	defer cm.Unlock()
	cm.httpClients[userID] = client
}

func (cm *ClientManager) GetHTTPClient(userID string) *resty.Client {
	cm.RLock()
	defer cm.RUnlock()
	return cm.httpClients[userID]
}

func (cm *ClientManager) DeleteHTTPClient(userID string) {
	cm.Lock()
	defer cm.Unlock()
	delete(cm.httpClients, userID)
}
