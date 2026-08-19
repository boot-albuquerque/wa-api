// Package domain contém as entidades centrais do domínio disparazaap-wa-api.
package domain

// SendMessageRequest representa o payload de envio de mensagem de texto.
// Corresponde ao struct textStruct em handlers.go:SendMessage().
type SendMessageRequest struct {
	Phone       string `json:"Phone"`
	Body        string `json:"Body"`
	LinkPreview bool   `json:"LinkPreview,omitempty"`
	ID          string `json:"Id,omitempty"`
}

// SendMessageResult representa o resultado do envio de mensagem.
type SendMessageResult struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Status    string `json:"status"`
}

// LinkPreviewData é a metadata de Open Graph resolvida para a primeira URL
// encontrada no corpo de uma mensagem de texto quando
// SendMessageRequest.LinkPreview é true. Compõe o ExtendedTextMessage que
// o wa-noise envia no lugar do Conversation simples — CAP-01.1 recuperou a
// semântica original do campo LinkPreview no histórico do wuzapi (commit
// 542e707: "Add LinkPreview support to SendMessage and improve Open Graph
// data fetching").
//
// MatchedURL nunca fica vazio quando o ponteiro para este tipo não é nil —
// é a condição usada para decidir que uma URL foi encontrada no corpo (ver
// port.LinkPreviewFetcher). Title, Description e ThumbnailJPEG podem vir
// vazios se a busca de Open Graph falhar ou a página não expuser essa
// metadata; o preview ainda assim é enviado, só sem esses campos.
type LinkPreviewData struct {
	MatchedURL    string
	Title         string
	Description   string
	ThumbnailJPEG []byte
}

// StatusSent é o valor de SendMessageResult.Status para uma mensagem de
// texto que o wa-noise efetivamente entregou ao transporte — só aparece
// DEPOIS que client.SendMessage retorna sucesso (CAP-01).
const StatusSent = "sent"

// StatusDeleted é o valor de DeleteMessageResult.Status para uma mensagem
// que o wa-noise efetivamente revogou — só aparece DEPOIS que o envio da
// revogação retorna sucesso (CAP-10).
//
// É um valor próprio, e não StatusSent, porque o histórico distinguia os
// dois no mesmo campo: DeleteMessage devolvia Details="Deleted" e
// SendEditMessage devolvia Details="Sent"
// (`git show 41bc8e2^:handlers.go`, linhas 2877 e 2969). A decisão
// HOUSEKEEP F131 fixou a FORMA do envelope, não os valores; apagar essa
// distinção seria perder informação que o cliente histórico recebia.
const StatusDeleted = "deleted"

// SendImageRequest representa o payload de envio de imagem. Image é uma
// união: aceita tanto data URI ("data:image/...;base64,...", CAP-03) quanto
// URL http(s) externa (CAP-02) — o mesmo campo e a mesma rota servem os
// dois casos, herdado de handlers.go pré-refactor (ver
// `git show 41bc8e2^:handlers.go`).
type SendImageRequest struct {
	Phone    string `json:"Phone"`
	Image    string `json:"Image"`
	Caption  string `json:"Caption,omitempty"`
	ID       string `json:"Id,omitempty"`
	MimeType string `json:"MimeType,omitempty"`
}

// SendImageResult representa o resultado do envio de imagem.
type SendImageResult struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Status    string `json:"status"`
}

// MediaPayload é o anexo já resolvido (bytes em mãos, MIME decidido) que um
// use case de envio de mídia passa a port.MediaMessenger. Fica em domain,
// não em port, porque é dado — não comportamento — compartilhado entre a
// camada de aplicação e o adapter (mesmo racional de LinkPreviewData).
//
// FileName só é usado por SendDocument (CAP-04); SendImage o deixa vazio.
// É metadata pura — nunca vira operação de sistema de arquivos em nenhum
// consumidor deste tipo.
type MediaPayload struct {
	Bytes    []byte
	MimeType string
	Caption  string
	FileName string
}

// SendDocumentRequest representa o payload de envio de documento. Document é
// uma união de dois transportes de OBTENÇÃO dos bytes — data URI (qualquer
// MIME, não só "data:image" como em SendImageRequest.Image) e URL http(s)
// externa — que convergem no mesmo protocolo de envio (appport.MediaMessenger),
// mesmo racional de SendImageRequest (CAP-02/CAP-03) — ver
// `git show 41bc8e2^:handlers.go`, em torno da linha 900.
type SendDocumentRequest struct {
	Phone    string `json:"Phone"`
	Document string `json:"Document"`
	FileName string `json:"FileName"`
	Caption  string `json:"Caption,omitempty"`
	ID       string `json:"Id,omitempty"`
	MimeType string `json:"MimeType,omitempty"`
}

// SendDocumentResult representa o resultado do envio de documento.
type SendDocumentResult struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Status    string `json:"status"`
}

// SendAudioRequest representa o payload de envio de áudio. Audio é uma
// união de dois transportes de OBTENÇÃO dos bytes — data URI
// ("data:audio/...;base64,...") e URL http(s) externa — mesmo racional de
// SendImageRequest/SendDocumentRequest, mas com discriminação ESTREITA
// como SendImageRequest.Image (só "data:audio/", não "data:" genérico como
// SendDocumentRequest.Document) — ver `git show 41bc8e2^:handlers.go`, em
// torno da linha 1088.
//
// PTT é ponteiro de propósito: nil significa "cliente não declarou",
// TRUE por default (voice note) — não false. Ver resolveAudioPTT em
// send_audio.go.
//
// Caption existe no contrato público mas é INERTE para áudio: o histórico
// (`git show 41bc8e2^:handlers.go`, em torno da linha 1148) nunca monta um
// campo Caption em AudioMessage, e o protobuf waE2E.AudioMessage não tem
// esse campo (confirmado: nenhum campo Caption em
// internal/wa-noise/protocol/proto/waE2E, struct AudioMessage). Achado do
// CAP-05, reportado — não implementado por conta própria (decisão de
// contrato não é do executor).
type SendAudioRequest struct {
	Phone    string `json:"Phone"`
	Audio    string `json:"Audio"`
	Caption  string `json:"Caption,omitempty"`
	ID       string `json:"Id,omitempty"`
	PTT      *bool  `json:"ptt,omitempty"`
	MimeType string `json:"mimetype,omitempty"`
	Seconds  uint32 `json:"Seconds,omitempty"`
}

// SendAudioResult representa o resultado do envio de áudio.
type SendAudioResult struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Status    string `json:"status"`
}

// AudioPayload é o anexo de áudio já resolvido (bytes em mãos, MIME
// decidido, PTT e Seconds resolvidos) que SendAudioUseCase passa a
// port.MediaMessenger.SendAudio. Tipo próprio, e não domain.MediaPayload —
// ver o comentário de SendAudio em port.MediaMessenger para o porquê.
type AudioPayload struct {
	Bytes    []byte
	MimeType string
	PTT      bool
	Seconds  uint32
}

// SendStickerRequest representa o payload de envio de sticker.
type SendStickerRequest struct {
	Phone    string `json:"Phone"`
	Sticker  string `json:"Sticker"`
	ID       string `json:"Id,omitempty"`
	MimeType string `json:"MimeType,omitempty"`
}

// SendStickerResult representa o resultado do envio de sticker.
type SendStickerResult struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Status    string `json:"status"`
}

// SendVideoRequest representa o payload de envio de vídeo. Video é uma
// união de dois transportes de OBTENÇÃO dos bytes — data URI e URL http(s)
// externa — mesmo racional de SendImageRequest/SendDocumentRequest, mas com
// a discriminação MAIS FROUXA das quatro: os 4 primeiros caracteres têm de
// ser "data" (sem os dois-pontos), não "data:video/" (estreito como Audio)
// nem "data:" (como Document) — ver `git show 41bc8e2^:handlers.go`, em
// torno da linha 1583, e isDataVideo em send_video.go.
//
// MimeType e JPEGThumbnail existiam no DTO histórico (imageStruct de
// SendVideo) e NÃO estão aqui — achado do CAP-06, reportado, não
// implementado por conta própria (decisão de contrato não é do executor).
type SendVideoRequest struct {
	Phone   string `json:"Phone"`
	Video   string `json:"Video"`
	Caption string `json:"Caption,omitempty"`
	ID      string `json:"Id,omitempty"`
}

// SendVideoResult representa o resultado do envio de vídeo.
type SendVideoResult struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Status    string `json:"status"`
}

// SendContactRequest representa o payload de envio de contato.
type SendContactRequest struct {
	Phone string `json:"Phone"`
	Name  string `json:"Name"`
	Vcard string `json:"Vcard"`
	ID    string `json:"Id,omitempty"`
}

// SendContactResult representa o resultado do envio de contato.
type SendContactResult struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Status    string `json:"status"`
}

// SendLocationRequest representa o payload de envio de localização.
type SendLocationRequest struct {
	Phone     string  `json:"Phone"`
	Name      string  `json:"Name,omitempty"`
	Latitude  float64 `json:"Latitude"`
	Longitude float64 `json:"Longitude"`
	ID        string  `json:"Id,omitempty"`
}

// SendLocationResult representa o resultado do envio de localização.
type SendLocationResult struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Status    string `json:"status"`
}

// LocationPayload é a metadata de protocolo pura que
// port.SimpleMessenger.SendLocation repassa para LocationMessage — sem
// upload, sem fetch, sem conversão (CAP-08A). Latitude/Longitude/Name são
// os únicos três campos que o histórico preenchia em LocationMessage (ver
// `git show 41bc8e2^:handlers.go`, em torno da linha 1935).
type LocationPayload struct {
	Latitude  float64
	Longitude float64
	Name      string
}

// ContactPayload é a metadata de protocolo pura que
// port.SimpleMessenger.SendContact repassa para ContactMessage — sem
// upload, sem fetch, sem conversão (CAP-08B). Name/Vcard são os únicos dois
// campos que o histórico preenchia em ContactMessage (ver
// `git show 41bc8e2^:handlers.go`, em torno da linha 1810). Vcard é
// repassado como STRING crua, sem parse nem validação de formato — mesma
// disciplina do histórico.
type ContactPayload struct {
	Name  string
	Vcard string
}

// SendButtonsRequest representa o payload de envio de botões.
type SendButtonsRequest struct {
	Phone string `json:"Phone"`
	Body  string `json:"Body"`
	ID    string `json:"Id,omitempty"`
}

// SendButtonsResult representa o resultado do envio de botões.
type SendButtonsResult struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}

// SendListRequest representa o payload de envio de lista.
type SendListRequest struct {
	Phone string `json:"Phone"`
	Desc  string `json:"Desc"`
	ID    string `json:"Id,omitempty"`
}

// SendListResult representa o resultado do envio de lista.
type SendListResult struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}

// SendPollRequest representa o payload de envio de enquete.
type SendPollRequest struct {
	Group   string   `json:"Group"`
	Header  string   `json:"Header"`
	Options []string `json:"Options"`
	ID      string   `json:"Id,omitempty"`
}

// SendPollResult representa o resultado do envio de enquete.
type SendPollResult struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}

// DeleteMessageRequest representa o payload de exclusão de mensagem.
type DeleteMessageRequest struct {
	Phone string `json:"Phone"`
	ID    string `json:"Id"`
}

// DeleteMessageResult representa o resultado da exclusão de mensagem.
//
// Timestamp entra aqui no CAP-10 (decisão HOUSEKEEP F131: manter a forma
// ATUAL {message_id, timestamp, status}, a das oito capabilities de envio
// já entregues, e não a histórica {Details, Timestamp, Id}). O acréscimo é
// aditivo — nenhum campo sai — e alinha estes dois DTOs com
// SendMessageResult/SendLocationResult, dos quais só divergiam porque
// nunca tinham chegado a enviar nada.
type DeleteMessageResult struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Status    string `json:"status"`
}

// SendEditMessageRequest representa o payload de edição de mensagem.
type SendEditMessageRequest struct {
	Phone string `json:"Phone"`
	Body  string `json:"Body"`
	ID    string `json:"Id"`
}

// SendEditMessageResult representa o resultado da edição de mensagem.
// Mesma disciplina de DeleteMessageResult quanto a Timestamp (CAP-10,
// HOUSEKEEP F131).
type SendEditMessageResult struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Status    string `json:"status"`
}

// SendTemplateRequest representa o payload de envio de template.
type SendTemplateRequest struct {
	Phone   string `json:"Phone"`
	Content string `json:"Content"`
	Footer  string `json:"Footer"`
	ID      string `json:"Id,omitempty"`
}

// SendTemplateResult representa o resultado do envio de template.
type SendTemplateResult struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}
