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

// SendImageRequest representa o payload de envio de imagem.
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
	Status    string `json:"status"`
}

// SendDocumentRequest representa o payload de envio de documento.
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
	Status    string `json:"status"`
}

// SendAudioRequest representa o payload de envio de áudio.
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
	Status    string `json:"status"`
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
	Status    string `json:"status"`
}

// SendVideoRequest representa o payload de envio de vídeo.
type SendVideoRequest struct {
	Phone   string `json:"Phone"`
	Video   string `json:"Video"`
	Caption string `json:"Caption,omitempty"`
	ID      string `json:"Id,omitempty"`
}

// SendVideoResult representa o resultado do envio de vídeo.
type SendVideoResult struct {
	MessageID string `json:"message_id"`
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
	Status    string `json:"status"`
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
type DeleteMessageResult struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}

// SendEditMessageRequest representa o payload de edição de mensagem.
type SendEditMessageRequest struct {
	Phone string `json:"Phone"`
	Body  string `json:"Body"`
	ID    string `json:"Id"`
}

// SendEditMessageResult representa o resultado da edição de mensagem.
type SendEditMessageResult struct {
	MessageID string `json:"message_id"`
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
