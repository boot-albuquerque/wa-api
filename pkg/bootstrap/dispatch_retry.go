package bootstrap

import (
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

// Reagendamento de webhook (F88).
//
// A waitFull entre tentativas NÃO pode acontecer dentro de um worker do pool de
// despacho. Com os padrões que estavam em produção — 5 tentativas, base de 30s,
// backoff exponencial — as esperas somavam 30+60+120+240 = 450s: um único
// evento para um destino morto segurava um worker por 7,5 MINUTOS.
//
// Como os quatro canais de entrega dividem o mesmo pool de 256, 256 eventos
// para um webhook morto o saturavam por inteiro — e o pareamento medido em
// produção são 129 eventos, então DOIS pareamentos simultâneos bastavam para
// parar também WebSocket, webhook global e RabbitMQ, que nada tinham a ver
// com o destino quebrado.
//
// Isso era regressão introduzida pelo próprio pool (F86): antes dele as mesmas
// esperas eram goroutines soltas dormindo, feio e inofensivo.
//
// Aqui a waitFull vira TIMER: o worker devolve o slot imediatamente e o trabalho
// volta ao pool quando o prazo vence. Um timer pendente não custa goroutine.

const (
	// envRetryMaxPendingBytes limita quanto payload pode estar esperando
	// timer. Sem ele, trocar "goroutine dormindo" por "timer pendente" só
	// mudaria ONDE a memória cresce sem limite — o mesmo defeito com outra
	// roupa. Bytes, e não contagem, pelo motivo do pool: os payloads medidos
	// vão de 1,5KB a 120KB.
	envRetryMaxPendingBytes = "WA_API_WEBHOOK_RETRY_MAX_PENDING_BYTES"

	// Mesmo orçamento do pool: a rajada de pareamento medida (129 eventos por
	// sessão) cabe com folga, e um destino morto durante uma tempestade de QR
	// tem teto conhecido em vez de crescer até o OOM.
	retryDefaultMaxPendingBytes = 32 * 1024 * 1024

	// retryJitterPercent espalha as tentativas. Sem jitter, N sessões que
	// falham juntas — exatamente a tempestade de QR — repetem em uníssono nos
	// mesmos instantes, martelando um destino que já está com problema. O
	// backoff exponencial sozinho não resolve isso: ele espaça as tentativas
	// de UM cliente, não separa clientes entre si.
	retryJitterPercent = 25
)

var (
	retryBytesPendentes atomic.Int64
	retryDescartados    atomic.Int64
	retryAvisouTeto     atomic.Bool
)

// atrasoDaTentativa devolve quanto esperar ANTES da tentativa `n` (n >= 1),
// com backoff exponencial e jitter.
//
// A fórmula exponencial é a mesma de antes; o que mudou foi o padrão da base
// (ver config.go) e o jitter. Manter a fórmula preserva o significado da
// variável de configuração para quem já a ajustou.
func atrasoDaTentativa(n int) time.Duration {
	if n < 1 {
		return 0
	}
	base := time.Duration(appCtx.WebhookRetryDelaySeconds) * time.Second
	fator := time.Duration(1) << uint(n-1)
	atraso := base * fator

	// Jitter simétrico: ±retryJitterPercent do valor nominal.
	amplitude := int64(atraso) * retryJitterPercent / 100
	if amplitude > 0 {
		atraso += time.Duration(rand.Int63n(2*amplitude) - amplitude)
	}
	if atraso < 0 {
		atraso = 0
	}
	return atraso
}

// tamanhoDoPayload estima o que o reagendamento mantém vivo. É o mesmo
// `jsonData` que a fila do pool contabiliza, então as duas contas falam da
// mesma coisa.
func tamanhoDoPayload(payload map[string]string) int {
	return len(payload["jsonData"])
}

// agendarProximaTentativa devolve true quando conseguiu agendar. False
// significa "acabou" — por ter esgotado as tentativas, por retry desligado ou
// por estouro do orçamento de pendentes —, e quem chama segue para o caminho
// terminal.
func agendarProximaTentativa(myurl string, payload map[string]string, userID string, encryptedHmacKey []byte, proxima int) bool {
	if !appCtx.WebhookRetryEnabled || proxima >= appCtx.WebhookRetryCount {
		return false
	}

	tamanho := tamanhoDoPayload(payload)
	teto := int64(readIntFromEnv(envRetryMaxPendingBytes, retryDefaultMaxPendingBytes))
	if retryBytesPendentes.Add(int64(tamanho)) > teto {
		retryBytesPendentes.Add(-int64(tamanho))
		retryDescartados.Add(1)
		// Aviso único: logar por ocorrência transformaria a saturação numa
		// segunda rajada, agora de log (ver F85).
		if retryAvisouTeto.CompareAndSwap(false, true) {
			log.Warn().
				Str("url", myurl).
				Int64("teto_bytes", teto).
				Msg("orcamento de retries pendentes estourado; entregas vao direto para o caminho terminal")
		}
		return false
	}

	atraso := atrasoDaTentativa(proxima)
	log.Warn().
		Int("attempt", proxima+1).
		Str("url", myurl).
		Dur("delay", atraso).
		Msg("Retrying webhook request with exponential backoff...")

	// AfterFunc e não `go func(){ Sleep }`: o timer não custa goroutine
	// enquanto não dispara, que é o ponto todo desta mudança.
	time.AfterFunc(atraso, func() {
		retryBytesPendentes.Add(-int64(tamanho))
		// Volta pelo pool, e não direto: a tentativa reagendada é uma entrega
		// como qualquer outra e tem de respeitar o mesmo teto.
		dispatchGo("callHookWithHmac-retry", tamanho, func() {
			tentarWebhook(myurl, payload, userID, encryptedHmacKey, proxima)
		})
	})
	return true
}

// MetricasRetry expõe o estado do reagendamento para teste e diagnóstico.
func MetricasRetry() (bytesPendentes, descartados int64) {
	return retryBytesPendentes.Load(), retryDescartados.Load()
}
