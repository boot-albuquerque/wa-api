package registry

import "time"

// Constantes de fronteira do registro de clientes: nenhum timeout literal
// deve aparecer solto nos métodos abaixo.

const (
	// webhookClientTimeout é o teto de uma entrega de webhook para o
	// endpoint do usuário. Semântica distinta do teto de uma requisição ao
	// servidor do WhatsApp (waclient.RequestTimeout) — o alvo é
	// infraestrutura de terceiro — e por isso constante própria, ainda que
	// hoje coincidam em valor.
	webhookClientTimeout = 30 * time.Second

	// wsBroadcastTimeout limita quanto BroadcastToUser espera por um cliente
	// WS lento antes de desistir dele — um leitor travado do outro lado não
	// pode atrasar a entrega para todas as outras conexões.
	wsBroadcastTimeout = 5 * time.Second
)

// envWebhookTLSSkipVerify é lido e reportado no log de aviso pelo mesmo
// sync.OnceValue; o nome aparecia duas vezes, e divergir os dois é um
// erro silencioso (o log passaria a citar uma variável inexistente).
const envWebhookTLSSkipVerify = "WA_API_WEBHOOK_TLS_SKIP_VERIFY"
