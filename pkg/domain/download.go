// Package domain contém as entidades centrais do domínio disparazaap-wa-api.
package domain

// MediaKind nomeia qual sub-mensagem protobuf o descritor de download
// representa. Existe porque o download de mídia é UMA primitive
// (`Client.Download(ctx, DownloadableMessage)`) que aceita cinco tipos
// diferentes de sub-mensagem: ImageMessage, VideoMessage, AudioMessage,
// DocumentMessage e StickerMessage. Sem o kind explícito o adapter não teria
// como escolher qual delas montar, e a alternativa — cinco portas — duplicaria
// a mesma chamada cinco vezes.
type MediaKind string

// Os cinco kinds, um por rota /chat/download*. Constantes nomeadas, não
// literais soltos (ADR-0004): o mapeamento kind -> protobuf vive no adapter e
// é verificado por tabela nos testes.
const (
	MediaKindImage    MediaKind = "image"
	MediaKindVideo    MediaKind = "video"
	MediaKindAudio    MediaKind = "audio"
	MediaKindDocument MediaKind = "document"
	MediaKindSticker  MediaKind = "sticker"
)

// DownloadRequest representa o payload de download de mídia.
type DownloadRequest struct {
	URL           string `json:"Url"`
	DirectPath    string `json:"DirectPath"`
	MediaKey      []byte `json:"MediaKey"`
	Mimetype      string `json:"Mimetype"`
	FileEncSHA256 []byte `json:"FileEncSHA256"`
	FileSHA256    []byte `json:"FileSHA256"`
	FileLength    uint64 `json:"FileLength"`
}

// MediaDescriptor é o que a porta de download atravessa: os campos de
// cifragem/localização que `Client.Download` realmente consome, mais o kind.
// Não é um DTO de HTTP (não tem envelope, não tem base64) e não é um DTO
// gigante: cada campo aqui alimenta um campo homônimo da sub-mensagem
// protobuf montada pelo adapter, exatamente como o handler histórico fazia
// (`git show 41bc8e2^:handlers.go`, DownloadImage na linha 3836).
type MediaDescriptor struct {
	Kind          MediaKind
	URL           string
	DirectPath    string
	MediaKey      []byte
	Mimetype      string
	FileEncSHA256 []byte
	FileSHA256    []byte
	FileLength    uint64
}

// Descriptor converte o payload HTTP no descritor da porta, para o kind dado.
//
// É o ÚNICO ponto de conversão dos sete campos de DownloadRequest, e existe
// por isso: com cinco use cases copiando a conversão à mão, um campo esquecido
// numa das cópias atravessaria o HTTP sem efeito e sem ninguém notar — que era
// exatamente o estado anterior a CAP-09B, em que SEIS dos sete campos eram
// aceitos e ignorados.
func (r DownloadRequest) Descriptor(kind MediaKind) MediaDescriptor {
	return MediaDescriptor{
		Kind:          kind,
		URL:           r.URL,
		DirectPath:    r.DirectPath,
		MediaKey:      r.MediaKey,
		Mimetype:      r.Mimetype,
		FileEncSHA256: r.FileEncSHA256,
		FileSHA256:    r.FileSHA256,
		FileLength:    r.FileLength,
	}
}

// DownloadResult representa o resultado do download de mídia.
type DownloadResult struct {
	Mimetype string `json:"Mimetype"`
	Data     string `json:"Data"` // base64 data URL
}
