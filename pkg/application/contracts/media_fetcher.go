package port

import "context"

// MediaFetcher busca os bytes de uma URL externa para anexar a uma
// mensagem de mídia.
//
// Porta separada de MediaMessenger de propósito: buscar (rede HTTP
// arbitrária, sujeita a SSRF e a limite de tamanho) e enviar (upload +
// protocolo noise) falham por motivos diferentes e são substituíveis
// independentemente — mesma disciplina de portas estreitas que
// TextMessenger e LinkPreviewFetcher já seguem (CAP-01/CAP-01.1).
type MediaFetcher interface {
	// FetchBytes baixa resourceURL e devolve o corpo da resposta e o
	// Content-Type declarado pelo servidor remoto. limit é aplicado
	// DURANTE a leitura — não só contra um Content-Length declarado — para
	// que uma resposta sem esse cabeçalho (ou que minta sobre ele) não
	// produza leitura sem teto. O chamador NÃO deve tratar contentType
	// como fonte de verdade do tipo de mídia: é o valor que o servidor
	// remoto quis declarar, não o que os bytes realmente são.
	FetchBytes(ctx context.Context, resourceURL string, limit int64) (data []byte, contentType string, err error)
}
