package port

import (
	"context"

	"wa-api/pkg/domain"
)

// LinkPreviewFetcher resolve a metadata de Open Graph (título, descrição,
// miniatura) da primeira URL encontrada em um texto, para compor o link
// preview que TextMessenger.SendText embute no ExtendedTextMessage.
//
// Porta separada de TextMessenger de propósito: a resolução do preview é
// uma chamada de rede independente do envio em si — resolver e enviar são
// operações que falham por motivos diferentes e são substituíveis
// separadamente (CAP-01.1 segue a mesma disciplina de portas estreitas que
// TextMessenger e JIDResolver já seguem).
type LinkPreviewFetcher interface {
	// FetchLinkPreview procura a primeira URL em text. Se não achar
	// nenhuma, found é false e data é o zero value — o chamador não deve
	// montar preview algum. Se achar, found é true e data.MatchedURL vem
	// preenchido; Title, Description e ThumbnailJPEG podem vir vazios
	// quando a busca de Open Graph falha ou a página não expõe essa
	// metadata — isso NÃO é erro, é um preview sem enriquecimento.
	FetchLinkPreview(ctx context.Context, text string) (data domain.LinkPreviewData, found bool)
}
