// Package webhook guarda, por userID, o cliente HTTP de entrega de webhook.
//
// O diretório não se chama http: o pacote passaria a se chamar http e
// sombrearia net/http em qualquer arquivo que importasse os dois. E o
// conteúdo é mais específico que "HTTP" — tudo aqui é sobre a entrega de
// webhook ao endpoint do usuário.
//
// Único mapa, nenhum acesso cruzado a outro sub-registry: este pacote saiu
// inteiro de ClientManager sem nenhuma ressalva de atomicidade.
package webhook

import (
	"crypto/tls"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/rs/zerolog/log"
)

const (
	// clientTimeout é o teto de uma entrega de webhook para o endpoint do
	// usuário. Semântica distinta do teto de uma requisição ao servidor do
	// WhatsApp (waclient.RequestTimeout) — o alvo é infraestrutura de
	// terceiro — e por isso constante própria, ainda que hoje coincidam em
	// valor.
	clientTimeout = 30 * time.Second

	// maxRedirects limita a cadeia de redirecionamentos que uma entrega
	// aceita seguir antes de desistir.
	maxRedirects = 15

	// EnvTLSSkipVerify é lido e reportado no log de aviso pelo mesmo
	// sync.OnceValue; o nome aparecia duas vezes, e divergir os dois é um
	// erro silencioso (o log passaria a citar uma variável inexistente).
	//
	// Exportado porque é contrato com o operador e com
	// pkg/bootstrap/lifecycle.go, que lê a mesma variável.
	EnvTLSSkipVerify = "WA_API_WEBHOOK_TLS_SKIP_VERIFY"
)

// tlsSkipVerify informa se as entregas de webhook devem pular a verificação
// do certificado TLS. Espelha o webhookTLSSkipVerify de
// pkg/bootstrap/lifecycle.go (mesma env var, mesma semântica de opt-in
// inseguro) — duplicado aqui em vez de importado porque pkg/infra/wa-noise
// não pode depender de pkg/bootstrap.
var tlsSkipVerify = sync.OnceValue(func() bool {
	v := strings.ToLower(os.Getenv(EnvTLSSkipVerify))
	skip := v == "true" || v == "1"
	if skip {
		log.Warn().
			Str("env", EnvTLSSkipVerify).
			Msg("INSECURE: webhook TLS certificate verification is DISABLED by explicit configuration. " +
				"Webhook deliveries are vulnerable to man-in-the-middle attacks. Unset this variable in production.")
	}
	return skip
})

// Registry é o registro de clientes HTTP de webhook por userID.
type Registry struct {
	mu      sync.RWMutex
	clients map[string]*resty.Client
}

// New devolve um Registry vazio e pronto para uso.
func New() *Registry {
	return &Registry{clients: make(map[string]*resty.Client)}
}

// Provision monta o cliente HTTP de entrega de webhook para userID
// (política de redirect, timeout, TLS, proxy opcional) e o registra.
// proxyURL vazio entrega sem proxy.
//
// A construção do cliente acontece FORA do lock: montar um resty.Client não
// depende do estado do registro, e segurá-lo durante a construção
// bloquearia leitores de todos os outros usuários sem necessidade.
func (r *Registry) Provision(userID string, proxyURL string) error {
	c := resty.New()
	c.SetRedirectPolicy(resty.FlexibleRedirectPolicy(maxRedirects))
	c.SetTimeout(clientTimeout)
	c.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: tlsSkipVerify()}) //nolint:gosec // opt-in explícito via env var, ver tlsSkipVerify
	// Asserção de tipo, e não errors.As: errors.As pegaria também o caso
	// embrulhado e é o que o lint prefere, mas seria mudança de
	// comportamento numa quebra que se propõe a não ter nenhuma. Fica como
	// estava; trocar é decisão separada.
	c.OnError(func(_ *resty.Request, err error) {
		if v, ok := err.(*resty.ResponseError); ok { //nolint:errorlint // ver comentário acima
			log.Debug().Str("response", v.Response.String()).Msg("resty error")
			log.Error().Err(v.Err).Msg("resty error")
		}
	})

	if proxyURL != "" {
		c.SetProxy(proxyURL)
	}

	r.Set(userID, c)
	return nil
}

// Set guarda o cliente HTTP de userID.
func (r *Registry) Set(userID string, c *resty.Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[userID] = c
}

// Get devolve o cliente HTTP de userID, ou nil.
func (r *Registry) Get(userID string) *resty.Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.clients[userID]
}

// Delete remove o cliente HTTP de userID.
func (r *Registry) Delete(userID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.clients, userID)
}
