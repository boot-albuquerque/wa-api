package media

import (
	"context"
	"net/http"

	waLog "wa-api/internal/wa-noise/observability/log"
	waBinary "wa-api/internal/wa-noise/protocol/binary"
)

// HTTPTransport e' a fatia do cliente de que o caminho HTTP puro de midia
// precisa: o http.Client dedicado, o logger e as duas decisoes que dependem da
// configuracao do cliente (Messenger x WhatsApp, avisos de validacao).
//
// Deliberadamente nao expoe nada do *wanoise.Client alem disso — e' o que
// permite que este pacote nao importe o pacote raiz e que os testes usem um
// duble em vez de um cliente real.
type HTTPTransport interface {
	// HTTPClient e' o http.Client usado para baixar e subir midia.
	HTTPClient() *http.Client
	// Log e' o logger do cliente.
	Log() waLog.Logger
	// IsMessenger diz se o cliente esta' configurado como cliente do
	// Messenger (MessengerConfig != nil), o que muda o User-Agent do
	// download, o prefixo e o host do upload.
	IsMessenger() bool
	// MessengerUserAgent e' o User-Agent a mandar nos downloads quando
	// IsMessenger() e' verdadeiro. Nao e' consultado caso contrario.
	MessengerUserAgent() string
	// ReturnDownloadWarnings diz se as validacoes nao-fatais de tamanho e de
	// hash do plaintext devem virar erro no fim do download.
	ReturnDownloadWarnings() bool
}

// Transport acrescenta a HTTPTransport a resolucao da lista de hosts de midia
// (a "media connection"), necessaria para todo download por directPath e para
// qualquer upload.
type Transport interface {
	HTTPTransport
	// MediaConnCache devolve o cache de media connection do cliente. O
	// ponteiro precisa ser estavel entre chamadas (e' o dono do lock).
	MediaConnCache() *ConnCache
	// SendMediaConnIQ manda o IQ <media_conn> em w:m e devolve a resposta
	// crua; a interpretacao fica em ParseConnNode.
	SendMediaConnIQ(ctx context.Context) (*waBinary.Node, error)
}
