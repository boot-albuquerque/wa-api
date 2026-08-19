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

// Os quatro tipos de botão interativo que o histórico reconhecia
// (`git show 41bc8e2^:handlers.go`, função SendButtons). São valores do
// CONTRATO PÚBLICO — chegam no campo `type` do JSON do cliente —, e por isso
// vivem no domínio e não no adapter: o adapter traduz cada um para o `Name`
// do NativeFlowButton, mas quem os define é a API.
//
// O mapa tipo público -> `Name` do wire NÃO é a identidade, e por isso os
// dois conjuntos de constantes são SEPARADOS (os `Name` vivem no adapter):
// `copy` vira `cta_copy` no wire. Colapsar os dois conjuntos num só faria a
// rota passar a aceitar `cta_copy` como tipo de entrada, que nunca foi
// contrato.
const (
	ButtonTypeReply   = "reply"
	ButtonTypeCTAURL  = "cta_url"
	ButtonTypeCTACall = "cta_call"
	ButtonTypeCopy    = "copy"
)

// InteractiveButton é UM botão de uma mensagem interativa (NativeFlow), com
// os nove campos do buttonStruct histórico (`git show 41bc8e2^:handlers.go`,
// linha 2006).
//
// É um DTO de DOMÍNIO, não de protobuf: qual `Name` e quais parâmetros cada
// um vira é tradução do adapter. Os TRÊS pares de campo redundantes existem
// porque o histórico os aceitava como fallback encadeado, nesta ordem exata:
//
//	Title <- Text <- ButtonText
//	ID    <- ButtonID <- (o Title já resolvido e já truncado)
//
// Estreitar isso agora recusaria payloads que a rota sempre aceitou.
type InteractiveButton struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	Text        string `json:"text"`
	ButtonText  string `json:"buttonText"`
	ID          string `json:"id"`
	ButtonID    string `json:"buttonId"`
	URL         string `json:"url"`
	PhoneNumber string `json:"phone_number"`
	CopyCode    string `json:"copy_code"`
}

// SendButtonsRequest representa o payload de envio de botões.
//
// Text/Title/Footer/Image/Buttons entraram no CAP-21 (HOUSEKEEP F147/F148):
// o DTO tinha ficado com {Phone, Body, Id} e sem `Buttons` a capability não
// tem sentido — uma mensagem interativa sem botão é uma mensagem de texto.
// O acréscimo é aditivo, mas é mudança de contrato, e não só reconexão de
// fiação.
//
// ContextInfo e QuotedMessage do payload histórico NÃO entram: são reply-to,
// nenhuma das dez capabilities de envio entregues os suporta, e a F134 já
// registra a dívida equivalente no edit. Decisão do Orchestrator, registrada
// em HOUSEKEEP F148 junto com o que se fez com a validação que dependia
// deles.
//
// Text é o fallback de Body (`body <- Body <- Text`, na ordem do histórico),
// e não um campo com significado próprio.
type SendButtonsRequest struct {
	Phone   string              `json:"Phone"`
	Body    string              `json:"Body"`
	Text    string              `json:"text"`
	Title   string              `json:"Title"`
	Footer  string              `json:"Footer"`
	Image   string              `json:"Image"`
	Buttons []InteractiveButton `json:"Buttons"`
	ID      string              `json:"Id,omitempty"`
}

// SendButtonsResult representa o resultado do envio de botões.
//
// Timestamp entrou no CAP-21 pela mesma razão que em SendStickerResult
// (F137), SendPollResult (CAP-14) e SendTemplateResult (CAP-15): as
// capabilities de envio têm a forma {message_id, timestamp, status}, travada
// em send_wire_contract_test.go. Decisão F131 (manter a forma ATUAL, e não a
// histórica {Details, Timestamp, Id}) não se reabre aqui.
type SendButtonsResult struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Status    string `json:"status"`
}

// ButtonsPayload é a metadata de protocolo que
// port.InteractiveMessenger.SendButtons repassa para InteractiveMessage
// (CAP-21).
//
// Buttons chega aqui JÁ NORMALIZADO pelo use case: Title resolvido pela
// cadeia de fallback e truncado ao limite do wire, ID resolvido, Type em
// caixa baixa e garantidamente um dos quatro de domínio, e os botões
// descartados já fora da lista. O adapter só traduz tipo -> `Name` e monta o
// JSON de parâmetros; ele NÃO reaplica fallback nenhum, porque a ordem
// "trunca o título e SÓ ENTÃO usa-o como ID" é observável no id que volta no
// clique de quem recebeu a mensagem.
//
// HeaderImage são os bytes JÁ OBTIDOS do header opcional (o use case decide
// entre data URI e URL externa, como em SendImageUseCase); vazio significa
// "sem imagem no header", e nesse caso o header carrega Title, se houver. O
// upload é do adapter — é ele que tem o cliente do wa-noise.
type ButtonsPayload struct {
	Body                string
	Title               string
	Footer              string
	Buttons             []InteractiveButton
	HeaderImage         []byte
	HeaderImageMimeType string
}

// ListRow é UMA linha de uma seção de lista, com os SEIS campos do
// listItem histórico (`git show 41bc8e2^:handlers.go`, função SendList).
//
// É um DTO de DOMÍNIO, não de protobuf. RowId/RowID/Rowid/Rowid2 existem
// porque o histórico os aceitava como fallback encadeado nesta ordem exata:
//
//	RowId <- RowID <- Rowid <- Rowid2 <- (o Title já resolvido e já trimado)
//
// O último nível NÃO é um campo do payload — é o próprio título, usado como
// identificador quando os quatro campos de ID vêm vazios. Depois de
// normalizado pelo use case, só Title/Description/RowId carregam valor: os
// outros três ficam vazios, mesma disciplina de InteractiveButton.ID/ButtonID.
type ListRow struct {
	Title       string `json:"title"`
	Description string `json:"desc"`
	RowId       string `json:"RowId"`
	RowID       string `json:"RowID"`
	Rowid       string `json:"rowId"`
	Rowid2      string `json:"rowID"`
}

// ListSection é UMA seção de `Sections`, com Rows já filtradas de linhas
// sem título quando normalizada pelo use case (ver
// SendListUseCase.normalizeSections).
type ListSection struct {
	Title string    `json:"title"`
	Rows  []ListRow `json:"rows"`
}

// SendListRequest representa o payload de envio de lista, com o contrato
// histórico completo levantado em HOUSEKEEP F149
// (`git show 41bc8e2^:handlers.go`, função SendList — 290 linhas, o maior
// handler dos stubs recuperados nesta sessão).
//
// DUAS formas de entrada: Sections (preferida, multi-seção) e List (legado,
// lista plana embrulhada numa seção única no use case). O corpo aceita
// QUATRO chaves JSON como fallback encadeado, nesta ordem:
//
//	Desc <- Body <- body <- text
//
// ContextInfo e QuotedMessage do payload histórico NÃO entram: reply-to é
// dívida separada (F134/F148), decisão do Orchestrator não reaberta aqui
// (HOUSEKEEP F149).
type SendListRequest struct {
	Phone      string        `json:"Phone"`
	ButtonText string        `json:"ButtonText"` // rótulo do botão que abre a lista; default "Select"
	Desc       string        `json:"Desc"`       // corpo principal. Fallback: Body, body, text
	Body       string        `json:"Body"`
	Body2      string        `json:"body"`
	Text       string        `json:"text"`
	TopText    string        `json:"TopText"`    // cabeçalho opcional; também default do título da seção legada
	FooterText string        `json:"FooterText"` // rodapé opcional
	Sections   []ListSection `json:"Sections"`   // preferida: multi-seção
	List       []ListRow     `json:"List"`       // legado: lista plana
	ID         string        `json:"Id,omitempty"`
}

// SendListResult representa o resultado do envio de lista.
//
// Timestamp entrou no CAP-22 pela mesma razão que em SendButtonsResult
// (CAP-21) e nas outras nove capabilities de envio: a forma
// {message_id, timestamp, status} é travada em send_wire_contract_test.go
// como a DÉCIMA SEGUNDA entrada. Decisão F131 (forma ATUAL, não a histórica
// {Details, Timestamp, Id}) não se reabre aqui.
type SendListResult struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Status    string `json:"status"`
}

// ListPayload é a metadata de protocolo que port.SimpleMessenger.SendList
// repassa para ListMessage (CAP-22).
//
// Sections chega aqui JÁ NORMALIZADO pelo use case: linha sem título
// descartada, seção que ficou sem linhas descartada, RowId resolvido pela
// cadeia de fallback (título já trimado é o último nível). O adapter NÃO
// reaplica fallback nenhum nem descarte nenhum — só traduz para
// waE2E.ListMessage_Section/Row.
type ListPayload struct {
	Body       string
	ButtonText string
	Title      string // TopText; vazio significa "sem cabeçalho" (ListMessage.Title fica nil)
	Footer     string // FooterText; vazio significa "sem rodapé" (ListMessage.FooterText fica nil)
	Sections   []ListSection
}

// SendPollRequest representa o payload de envio de enquete.
type SendPollRequest struct {
	Group   string   `json:"Group"`
	Header  string   `json:"Header"`
	Options []string `json:"Options"`
	ID      string   `json:"Id,omitempty"`
}

// SendPollResult representa o resultado do envio de enquete.
//
// Timestamp entrou no CAP-14 pela mesma razão que em SendStickerResult
// (F137): as capabilities de envio têm a forma {message_id, timestamp,
// status}, travada em send_wire_contract_test.go. Sem ele, /chat/send/poll
// seria a única a devolver o instante do envio como nada.
type SendPollResult struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Status    string `json:"status"`
}

// PollPayload é a metadata de protocolo pura que
// port.SimpleMessenger.SendPoll repassa para PollCreationMessage — sem
// upload, sem fetch, sem conversão (CAP-14). Name (o cabeçalho da enquete)
// e Options (o texto em claro de cada opção) são os únicos dois campos que
// o histórico passava a BuildPollCreation (ver `git show 41bc8e2^:handlers.go`,
// linha 2796).
//
// Não há campo para o número de opções selecionáveis DE PROPÓSITO: o
// histórico sempre passou 1 (escolha ÚNICA) e nunca expôs isso no payload
// público. Acrescentar aqui seria mudança de contrato, não recuperação da
// capability — a constante vive no adapter, junto da montagem do protobuf.
type PollPayload struct {
	Name    string
	Options []string
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

// Os três tipos de botão de template que o histórico reconhecia
// (`git show 41bc8e2^:handlers.go`, função SendTemplate). São valores do
// CONTRATO PÚBLICO — chegam no campo Type do JSON do cliente —, e por isso
// vivem no domínio e não no adapter: o adapter traduz cada um para o seu
// protobuf, mas quem os define é a API.
//
// Um Type desconhecido NÃO é recusado: o histórico o tratava como
// quickreply (ramo `default` do switch), e recusá-lo agora rejeitaria
// payloads que a rota sempre aceitou.
const (
	TemplateButtonQuickReply = "quickreply"
	TemplateButtonURL        = "url"
	TemplateButtonCall       = "call"
)

// TemplateButton é UM botão de um template hidratado, com os cinco campos do
// buttonStruct histórico (`git show 41bc8e2^:handlers.go`, linha 3099).
//
// É um DTO de DOMÍNIO, não de protobuf: qual dos três `Hydrated*Button` cada
// um vira é tradução do adapter. Os campos são deliberadamente NÃO
// exclusivos entre si — o histórico aceitava um botão com Url e PhoneNumber
// preenchidos ao mesmo tempo e usava só o que o Type pedia —, e estreitar
// isso agora recusaria payloads que a rota sempre aceitou.
//
// ID é o identificador do botão de resposta rápida. Vazio significa
// "numere automaticamente", e a numeração é do adapter, junto da montagem do
// protobuf: é o wire que exige um id decimal por botão.
type TemplateButton struct {
	DisplayText string `json:"DisplayText"`
	ID          string `json:"Id,omitempty"`
	URL         string `json:"Url,omitempty"`
	PhoneNumber string `json:"PhoneNumber,omitempty"`
	Type        string `json:"Type"`
}

// SendTemplateRequest representa o payload de envio de template.
//
// Buttons entrou no CAP-15 (HOUSEKEEP F139): sem ele o DTO tinha perdido o
// campo que dá SENTIDO à capability — um template hidratado sem botão é uma
// mensagem de texto com rodapé. O acréscimo é aditivo, mas é mudança de
// contrato, e não só reconexão de fiação como nos blocos anteriores.
type SendTemplateRequest struct {
	Phone   string           `json:"Phone"`
	Content string           `json:"Content"`
	Footer  string           `json:"Footer"`
	ID      string           `json:"Id,omitempty"`
	Buttons []TemplateButton `json:"Buttons"`
}

// SendTemplateResult representa o resultado do envio de template.
//
// Timestamp entrou no CAP-15 pela mesma razão que em SendStickerResult
// (F137) e SendPollResult (CAP-14): as capabilities de envio têm a forma
// {message_id, timestamp, status}, travada em send_wire_contract_test.go.
type SendTemplateResult struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Status    string `json:"status"`
}

// TemplatePayload é a metadata de protocolo pura que
// port.SimpleMessenger.SendTemplate repassa para TemplateMessage — sem
// upload, sem fetch, sem conversão (CAP-15). Content/Footer/Buttons são os
// únicos três campos que o histórico preenchia em HydratedFourRowTemplate
// (`git show 41bc8e2^:handlers.go`, linha 3226); o quarto, TemplateId, era o
// literal "1" e vive no adapter, junto da montagem do protobuf, porque a API
// pública nunca o expôs.
type TemplatePayload struct {
	Content string
	Footer  string
	Buttons []TemplateButton
}
