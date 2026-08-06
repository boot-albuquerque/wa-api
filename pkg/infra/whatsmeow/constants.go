package whatsmeow

import "time"

// Constantes de fronteira do pacote: nenhum timeout literal deve aparecer
// solto nos adapters. Timeouts específicos de uma operação só (o pull de
// app-state, por exemplo) continuam declarados junto da operação, porque
// dar-lhes um nome global sugeriria um reuso que não existe.

const (
	// waRequestTimeout é o teto de espera de uma requisição ao servidor do
	// WhatsApp feita por um adapter (blocklist, privacidade, contatos). O
	// SDK não impõe teto próprio: sem isto, um servidor que aceita a
	// conexão e não responde prende o handler HTTP até o cliente desistir.
	waRequestTimeout = 30 * time.Second

	// webhookClientTimeout é o teto de uma entrega de webhook para o
	// endpoint do usuário. Semântica distinta de waRequestTimeout — o alvo
	// é infraestrutura de terceiro, não o servidor do WhatsApp — e por isso
	// constante própria, ainda que hoje coincidam em valor.
	webhookClientTimeout = 30 * time.Second

	// wsBroadcastTimeout limita quanto BroadcastToUser espera por um cliente
	// WS lento antes de desistir dele — um leitor travado do outro lado não
	// pode atrasar a entrega para todas as outras conexões.
	wsBroadcastTimeout = 5 * time.Second
)

const (
	// envWebhookTLSSkipVerify é lido e reportado no log de aviso pelo mesmo
	// sync.OnceValue; o nome aparecia duas vezes, e divergir os dois é um
	// erro silencioso (o log passaria a citar uma variável inexistente).
	envWebhookTLSSkipVerify = "WA_API_WEBHOOK_TLS_SKIP_VERIFY"

	// codeSessionAlreadyPaired é devolvido por dois caminhos distintos de
	// Pair (checagem local de credenciais e ErrQRStoreContainsID do SDK) que
	// o caller precisa tratar da mesma forma.
	codeSessionAlreadyPaired = "session_already_paired"

	// codeUserSessionUnavailable e devolvido pelos tres metodos de
	// ContactDirectory quando nao ha sessao ativa; o caller distingue este
	// caso de uma falha do SDK pelo codigo, nao pela mensagem.
	codeUserSessionUnavailable = "user_session_unavailable"
)

// Auditoria de literais (plano §4, Fase 4): os demais literais de string do
// pacote são códigos e mensagens de apperr usados uma vez cada — nomeá-los
// só acrescentaria indireção —, valores de protocolo do WhatsApp reproduzidos
// literalmente em switch (platform.go, session_pairing.go, misc_adapters.go)
// e chaves de campo de log do zerolog. Nenhum deles se repete entre arquivos.
