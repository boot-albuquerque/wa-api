// Package domain contém as entidades centrais do domínio disparazaap-wa-api.
package domain

// ReplyContext carries the fields needed to quote (reply-to) an existing
// message. Separate from EditContextInfo on purpose: edit carries
// MentionedJID, which is orthogonal to quoting; and reply-to carries
// QuotedText, which edit does not need. Collapsing them into one type would
// force every consumer to carry fields it never uses, and the name would be
// wrong for at least one of the two callers.
//
// QuotedText is the text preview of the quoted message, embedded as
// ContextInfo.QuotedMessage. Baileys always sends it.
//
// It is OPTIONAL, and the claim that it is required has been REFUTED BY
// MEASUREMENT (F222). Two replies to the same original, sent in the same
// instant and differing only in QuotedText, BOTH rendered the quote bubble on
// a real iOS device. The earlier note here — taken from mautrix/whatsapp#904 —
// said mobile would not render without it; field measurement says otherwise.
//
// The measurement has a limit worth stating: the quoted message had been sent
// minutes earlier, so it was in the recipient's LOCAL history. Quoting a
// message the recipient does NOT hold locally (very old, or after a reinstall)
// was not tested, and QuotedMessage may well matter there. Send it when you
// have it; do not require it.
type ReplyContext struct {
	StanzaID    string `json:"StanzaId"`
	Participant string `json:"Participant"`
	QuotedText  string `json:"QuotedText,omitempty"`
}

// SendMessageRequest representa o payload de envio de mensagem de texto.
// Corresponde ao struct textStruct em handlers.go:SendMessage().
type SendMessageRequest struct {
	ChatTarget
	Phone        string        `json:"Phone"`
	Body         string        `json:"Body"`
	LinkPreview  bool          `json:"LinkPreview,omitempty"`
	ID           string        `json:"Id,omitempty"`
	ReplyTo      *ReplyContext `json:"ReplyTo,omitempty"`
	MentionedJID []string      `json:"MentionedJid,omitempty"`
}

func (r *SendMessageRequest) ResolveChat() { ResolveChatField(&r.Phone, r.ChatAlias) }

// SendMessageResult representa o resultado do envio de mensagem.
type SendMessageResult struct {
	MessageID string
	Timestamp int64
	Status    string
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
	HQImageData   []byte
	HQWidth       uint32
	HQHeight      uint32
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
	ChatTarget
	Phone         string        `json:"Phone"`
	Image         string        `json:"Image"`
	Caption       string        `json:"Caption,omitempty"`
	ID            string        `json:"Id,omitempty"`
	MimeType      string        `json:"MimeType,omitempty"`
	JPEGThumbnail []byte        `json:"JPEGThumbnail,omitempty"`
	ReplyTo       *ReplyContext `json:"ReplyTo,omitempty"`
	MentionedJID  []string      `json:"MentionedJid,omitempty"`
}

func (r *SendImageRequest) ResolveChat() { ResolveChatField(&r.Phone, r.ChatAlias) }

// SendImageResult representa o resultado do envio de imagem.
type SendImageResult struct {
	MessageID string
	Timestamp int64
	Status    string
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
	Bytes         []byte
	MimeType      string
	Caption       string
	FileName      string
	JPEGThumbnail []byte
	PngThumbnail  []byte
}

// SendDocumentRequest representa o payload de envio de documento. Document é
// uma união de dois transportes de OBTENÇÃO dos bytes — data URI (qualquer
// MIME, não só "data:image" como em SendImageRequest.Image) e URL http(s)
// externa — que convergem no mesmo protocolo de envio (appport.MediaMessenger),
// mesmo racional de SendImageRequest (CAP-02/CAP-03) — ver
// `git show 41bc8e2^:handlers.go`, em torno da linha 900.
type SendDocumentRequest struct {
	ChatTarget
	Phone        string        `json:"Phone"`
	Document     string        `json:"Document"`
	FileName     string        `json:"FileName"`
	Caption      string        `json:"Caption,omitempty"`
	ID           string        `json:"Id,omitempty"`
	MimeType     string        `json:"MimeType,omitempty"`
	ReplyTo      *ReplyContext `json:"ReplyTo,omitempty"`
	MentionedJID []string      `json:"MentionedJid,omitempty"`
}

func (r *SendDocumentRequest) ResolveChat() { ResolveChatField(&r.Phone, r.ChatAlias) }

// SendDocumentResult representa o resultado do envio de documento.
type SendDocumentResult struct {
	MessageID string
	Timestamp int64
	Status    string
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
	ChatTarget
	Phone    string        `json:"Phone"`
	Audio    string        `json:"Audio"`
	Caption  string        `json:"Caption,omitempty"`
	ID       string        `json:"Id,omitempty"`
	PTT      *bool         `json:"ptt,omitempty"`
	MimeType string        `json:"mimetype,omitempty"`
	Seconds  uint32        `json:"Seconds,omitempty"`
	Waveform []byte        `json:"Waveform,omitempty"`
	ReplyTo  *ReplyContext `json:"ReplyTo,omitempty"`
}

func (r *SendAudioRequest) ResolveChat() { ResolveChatField(&r.Phone, r.ChatAlias) }

// SendAudioResult representa o resultado do envio de áudio.
type SendAudioResult struct {
	MessageID string
	Timestamp int64
	Status    string

	// CaptionMessageID e CaptionStatus descrevem a legenda, que vai como
	// mensagem SEPARADA porque o protocolo não tem campo de legenda em áudio
	// (F116). Ambos ausentes quando não houve legenda a enviar.
	//
	// São dois campos e não um porque o envio NÃO é atómico: o áudio pode sair
	// e a legenda falhar. Sem `CaptionStatus`, o cliente veria um 200 e teria
	// de adivinhar se a legenda chegou.
	CaptionMessageID string
	CaptionStatus    string
}

// Estados possíveis de CaptionStatus.
const (
	// CaptionSent: a legenda saiu como mensagem própria, e o id dela está em
	// CaptionMessageID.
	CaptionSent = "sent"
	// CaptionFailed: o ÁUDIO saiu, a legenda não. O pedido continua
	// bem-sucedido de propósito — devolver erro faria o cliente reenviar e
	// duplicar o áudio, que já está entregue e não se desfaz.
	CaptionFailed = "failed"
)

// AudioPayload é o anexo de áudio já resolvido (bytes em mãos, MIME
// decidido, PTT e Seconds resolvidos) que SendAudioUseCase passa a
// port.MediaMessenger.SendAudio. Tipo próprio, e não domain.MediaPayload —
// ver o comentário de SendAudio em port.MediaMessenger para o porquê.
type AudioPayload struct {
	Bytes    []byte
	MimeType string
	PTT      bool
	Seconds  uint32
	Waveform []byte
}

// SendStickerRequest representa o payload de envio de sticker.
type SendStickerRequest struct {
	ChatTarget
	Phone         string        `json:"Phone"`
	Sticker       string        `json:"Sticker"`
	ID            string        `json:"Id,omitempty"`
	MimeType      string        `json:"MimeType,omitempty"`
	PngThumbnail  []byte        `json:"PngThumbnail,omitempty"`
	PackID        string        `json:"PackId,omitempty"`
	PackName      string        `json:"PackName,omitempty"`
	PackPublisher string        `json:"PackPublisher,omitempty"`
	Emojis        []string      `json:"Emojis,omitempty"`
	ReplyTo       *ReplyContext `json:"ReplyTo,omitempty"`
}

func (r *SendStickerRequest) ResolveChat() { ResolveChatField(&r.Phone, r.ChatAlias) }

// SendStickerResult representa o resultado do envio de sticker.
type SendStickerResult struct {
	MessageID string
	Timestamp int64
	Status    string
}

// SendVideoRequest representa o payload de envio de vídeo. Video é uma
// união de dois transportes de OBTENÇÃO dos bytes — data URI e URL http(s)
// externa — mesmo racional de SendImageRequest/SendDocumentRequest, mas com
// a discriminação MAIS FROUXA das quatro: os 4 primeiros caracteres têm de
// ser "data" (sem os dois-pontos), não "data:video/" (estreito como Audio)
// nem "data:" (como Document) — ver `git show 41bc8e2^:handlers.go`, em
// torno da linha 1583, e isDataVideo em send_video.go.
type SendVideoRequest struct {
	ChatTarget
	Phone         string        `json:"Phone"`
	Video         string        `json:"Video"`
	Caption       string        `json:"Caption,omitempty"`
	ID            string        `json:"Id,omitempty"`
	MimeType      string        `json:"MimeType,omitempty"`
	JPEGThumbnail []byte        `json:"JPEGThumbnail,omitempty"`
	ReplyTo       *ReplyContext `json:"ReplyTo,omitempty"`
	MentionedJID  []string      `json:"MentionedJid,omitempty"`
}

func (r *SendVideoRequest) ResolveChat() { ResolveChatField(&r.Phone, r.ChatAlias) }

// SendVideoResult representa o resultado do envio de vídeo.
type SendVideoResult struct {
	MessageID string
	Timestamp int64
	Status    string
}

// SendContactRequest representa o payload de envio de contato.
type SendContactRequest struct {
	ChatTarget
	Phone   string        `json:"Phone"`
	Name    string        `json:"Name"`
	Vcard   string        `json:"Vcard"`
	ID      string        `json:"Id,omitempty"`
	ReplyTo *ReplyContext `json:"ReplyTo,omitempty"`
}

func (r *SendContactRequest) ResolveChat() { ResolveChatField(&r.Phone, r.ChatAlias) }

// SendContactResult representa o resultado do envio de contato.
type SendContactResult struct {
	MessageID string
	Timestamp int64
	Status    string
}

// SendLocationRequest representa o payload de envio de localização.
type SendLocationRequest struct {
	ChatTarget
	Phone string `json:"Phone"`
	Name  string `json:"Name,omitempty"`
	// PONTEIRO, e não float64, para separar "não informado" de "zero" (F121).
	//
	// Zero é coordenada VÁLIDA — o ponto onde o equador cruza o meridiano de
	// Greenwich, no golfo da Guiné. Com float64 as duas situações colidiam e
	// um envio legítimo para lá era recusado com "missing Latitude".
	//
	// A mudança só AMPLIA o que é aceite: quem omite o campo continua a receber
	// 400, e quem manda 0 passa a ser aceite em vez de recusado. Nenhum cliente
	// existente perde comportamento.
	Latitude  *float64      `json:"Latitude"`
	Longitude *float64      `json:"Longitude"`
	ID        string        `json:"Id,omitempty"`
	ReplyTo   *ReplyContext `json:"ReplyTo,omitempty"`
}

func (r *SendLocationRequest) ResolveChat() { ResolveChatField(&r.Phone, r.ChatAlias) }

// SendLocationResult representa o resultado do envio de localização.
type SendLocationResult struct {
	MessageID string
	Timestamp int64
	Status    string
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
	ChatTarget
	Phone        string              `json:"Phone"`
	Body         string              `json:"Body"`
	Text         string              `json:"text"`
	Title        string              `json:"Title"`
	Footer       string              `json:"Footer"`
	Image        string              `json:"Image"`
	Buttons      []InteractiveButton `json:"Buttons"`
	ID           string              `json:"Id,omitempty"`
	ReplyTo      *ReplyContext       `json:"ReplyTo,omitempty"`
	MentionedJID []string            `json:"MentionedJid,omitempty"`
}

func (r *SendButtonsRequest) ResolveChat() { ResolveChatField(&r.Phone, r.ChatAlias) }

// SendButtonsResult representa o resultado do envio de botões.
//
// Timestamp entrou no CAP-21 pela mesma razão que em SendStickerResult
// (F137), SendPollResult (CAP-14) e SendTemplateResult (CAP-15): as
// capabilities de envio têm a forma {message_id, timestamp, status}, travada
// em send_wire_contract_test.go. Decisão F131 (manter a forma ATUAL, e não a
// histórica {Details, Timestamp, Id}) não se reabre aqui.
type SendButtonsResult struct {
	MessageID string
	Timestamp int64
	Status    string
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
	ChatTarget
	Phone        string        `json:"Phone"`
	ButtonText   string        `json:"ButtonText"` // rótulo do botão que abre a lista; default "Select"
	Desc         string        `json:"Desc"`       // corpo principal. Fallback: Body, body, text
	Body         string        `json:"Body"`
	Body2        string        `json:"body"`
	Text         string        `json:"text"`
	TopText      string        `json:"TopText"`    // cabeçalho opcional; também default do título da seção legada
	FooterText   string        `json:"FooterText"` // rodapé opcional
	Sections     []ListSection `json:"Sections"`   // preferida: multi-seção
	List         []ListRow     `json:"List"`       // legado: lista plana
	ID           string        `json:"Id,omitempty"`
	ReplyTo      *ReplyContext `json:"ReplyTo,omitempty"`
	MentionedJID []string      `json:"MentionedJid,omitempty"`
}

func (r *SendListRequest) ResolveChat() { ResolveChatField(&r.Phone, r.ChatAlias) }

// SendListResult representa o resultado do envio de lista.
//
// Timestamp entrou no CAP-22 pela mesma razão que em SendButtonsResult
// (CAP-21) e nas outras nove capabilities de envio: a forma
// {message_id, timestamp, status} é travada em send_wire_contract_test.go
// como a DÉCIMA SEGUNDA entrada. Decisão F131 (forma ATUAL, não a histórica
// {Details, Timestamp, Id}) não se reabre aqui.
type SendListResult struct {
	MessageID string
	Timestamp int64
	Status    string
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
	ChatTarget
	Group   string        `json:"Group"`
	Header  string        `json:"Header"`
	Options []string      `json:"Options"`
	ID      string        `json:"Id,omitempty"`
	ReplyTo *ReplyContext `json:"ReplyTo,omitempty"`
}

func (r *SendPollRequest) ResolveChat() { ResolveChatField(&r.Group, r.ChatAlias) }

// SendPollResult representa o resultado do envio de enquete.
//
// Timestamp entrou no CAP-14 pela mesma razão que em SendStickerResult
// (F137): as capabilities de envio têm a forma {message_id, timestamp,
// status}, travada em send_wire_contract_test.go. Sem ele, /chat/send/poll
// seria a única a devolver o instante do envio como nada.
type SendPollResult struct {
	MessageID string
	Timestamp int64
	Status    string
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

// SendPollVoteRequest represents the HTTP payload for POST /chat/send/pollvote.
//
// The caller provides the four fields that identify the original poll message:
// Phone (the chat), Sender (who created the poll), PollMessageId (the poll's
// message ID), and PollMessageTimestamp (the poll's Unix timestamp). These are
// needed because the vote is encrypted with a secret derived from the original
// poll message — BuildPollVote requires a *types.MessageInfo, not just an ID.
//
// Design choice (a): stateless, explicit. The alternative (b) — looking up the
// poll from message_history — creates a dependency on retention, and fails
// silently when the history was pruned. With (a), the caller always knows
// exactly what it passed, and the error is always about what it passed.
type SendPollVoteRequest struct {
	ChatTarget
	Phone                string   `json:"Phone"`
	Sender               string   `json:"Sender"`
	PollMessageID        string   `json:"PollMessageId"`
	PollMessageTimestamp int64    `json:"PollMessageTimestamp"`
	Options              []string `json:"Options"`
	ID                   string   `json:"Id,omitempty"`
}

func (r *SendPollVoteRequest) ResolveChat() { ResolveChatField(&r.Phone, r.ChatAlias) }

// SendPollVoteResult represents the response for POST /chat/send/pollvote.
// Same shape as every other send capability: {message_id, timestamp, status}.
type SendPollVoteResult struct {
	MessageID string
	Timestamp int64
	Status    string
}

// PollVotePayload is the protocol-pure metadata that
// port.ChatMessenger.SendPollVote passes to the adapter. It carries the four
// fields needed to reconstruct types.MessageInfo for the original poll, plus
// the option names the caller is voting for.
type PollVotePayload struct {
	PollChat      JID
	PollSender    JID
	PollMessageID string
	PollTimestamp int64
	OptionNames   []string
}

// ForwardContext marks a message as forwarded and carries the forwarding score.
// Analogous to ReplyContext for reply-to: a non-nil pointer means "this is a
// forwarded message", nil means "not forwarded". The score tracks how many
// times the message has been forwarded — Baileys increments by 1 on each hop,
// starting at 1 for the first forward.
//
// When the caller does not provide a score (ForwardingScore == 0), the use case
// defaults to 1 — the same value Baileys produces for a first-time forward.
type ForwardContext struct {
	ForwardingScore uint32
}

// SendForwardRequest represents the HTTP payload for POST /chat/send/forward.
//
// Two forms:
//
//	(a) By content (CAP-49): Phone + Body are required, the API creates a new
//	    text message marked as forwarded. ForwardingScore is caller-supplied.
//
//	(b) By key (CAP-55): Phone + MessageID + Chat are required. The API looks
//	    up the original message from message_history and re-sends it with
//	    forwarding context applied — including media, without re-upload.
//	    ForwardingScore is DERIVED from the stored message (incremented by 1)
//	    and any caller-supplied value is IGNORED.
//
// When MessageID is present, Body is ignored and ForwardingScore is ignored.
// When MessageID is absent, Body is required (backward compat with CAP-49).
type SendForwardRequest struct {
	ChatTarget
	Phone           string        `json:"Phone"`
	Body            string        `json:"Body"`
	ForwardingScore *uint32       `json:"ForwardingScore,omitempty"`
	ID              string        `json:"Id,omitempty"`
	ReplyTo         *ReplyContext `json:"ReplyTo,omitempty"`
	MentionedJID    []string      `json:"MentionedJid,omitempty"`
	MessageID       string        `json:"MessageID,omitempty"`
	Chat            string        `json:"Chat,omitempty"`
}

func (r *SendForwardRequest) ResolveChat() { ResolveChatField(&r.Phone, r.ChatAlias) }

// StoredMessageData is the application-boundary representation of a message
// retrieved from message_history for forwarding (CAP-55). It carries the raw
// datajson blob (which the adapter deserializes into the wire proto) and the
// chat where the message was originally received. The forwarding score is
// extracted by the adapter at send time, not here — it requires proto
// deserialization that belongs in the wa-noise layer.
type StoredMessageData struct {
	DataJSON string
	ChatJID  string
}

// SendForwardResult represents the response for POST /chat/send/forward.
// Same shape as every other send capability: {message_id, timestamp, status}.
type SendForwardResult struct {
	MessageID string
	Timestamp int64
	Status    string
}

// DeleteMessageRequest representa o payload de exclusão de mensagem.
type DeleteMessageRequest struct {
	ChatTarget
	Phone string `json:"Phone"`
	ID    string `json:"Id"`
}

func (r *DeleteMessageRequest) ResolveChat() { ResolveChatField(&r.Phone, r.ChatAlias) }

// DeleteMessageResult representa o resultado da exclusão de mensagem.
//
// Timestamp entra aqui no CAP-10 (decisão HOUSEKEEP F131: manter a forma
// ATUAL {message_id, timestamp, status}, a das oito capabilities de envio
// já entregues, e não a histórica {Details, Timestamp, Id}). O acréscimo é
// aditivo — nenhum campo sai — e alinha estes dois DTOs com
// SendMessageResult/SendLocationResult, dos quais só divergiam porque
// nunca tinham chegado a enviar nada.
type DeleteMessageResult struct {
	MessageID string
	Timestamp int64
	Status    string
}

// SendEditMessageRequest representa o payload de edição de mensagem.
type SendEditMessageRequest struct {
	ChatTarget
	Phone        string   `json:"Phone"`
	Body         string   `json:"Body"`
	ID           string   `json:"Id"`
	StanzaID     *string  `json:"StanzaId,omitempty"`
	Participant  *string  `json:"Participant,omitempty"`
	MentionedJID []string `json:"MentionedJid,omitempty"`
}

func (r *SendEditMessageRequest) ResolveChat() { ResolveChatField(&r.Phone, r.ChatAlias) }

// EditContextInfo carries the optional ContextInfo fields for EditMessage,
// restoring the contract the historical handlers.go accepted (F134).
type EditContextInfo struct {
	StanzaID     string
	Participant  string
	MentionedJID []string
}

// SendEditMessageResult representa o resultado da edição de mensagem.
// Mesma disciplina de DeleteMessageResult quanto a Timestamp (CAP-10,
// HOUSEKEEP F131).
type SendEditMessageResult struct {
	MessageID string
	Timestamp int64
	Status    string
}

// SendReactionResult é o resultado de POST /chat/react.
//
// Nasceu na F190, e o motivo de não ter nascido antes é o achado que a entrada
// regista: a rota devolvia um `map[string]interface{}` literal, montado dentro
// do use case. Sem tipo, não havia onde pendurar uma tag JSON — e portanto não
// havia nada que uma revisão de DTO apanhasse. Foi por isso que a `react` ficou
// com a forma histórica `{Details, Timestamp, Id}` enquanto as catorze irmãs
// migraram para esta.
//
// Tipar o resultado não é cerimónia: é o que põe a rota debaixo da mesma
// disciplina das outras, incluindo a trava de nomes de wire.
type SendReactionResult struct {
	MessageID string
	Timestamp int64
	Status    string
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
	ChatTarget
	Phone        string           `json:"Phone"`
	Content      string           `json:"Content"`
	Footer       string           `json:"Footer"`
	ID           string           `json:"Id,omitempty"`
	Buttons      []TemplateButton `json:"Buttons"`
	ReplyTo      *ReplyContext    `json:"ReplyTo,omitempty"`
	MentionedJID []string         `json:"MentionedJid,omitempty"`
}

func (r *SendTemplateRequest) ResolveChat() { ResolveChatField(&r.Phone, r.ChatAlias) }

// SendTemplateResult representa o resultado do envio de template.
//
// Timestamp entrou no CAP-15 pela mesma razão que em SendStickerResult
// (F137) e SendPollResult (CAP-14): as capabilities de envio têm a forma
// {message_id, timestamp, status}, travada em send_wire_contract_test.go.
type SendTemplateResult struct {
	MessageID string
	Timestamp int64
	Status    string
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

// CarouselCardType escolhe a APRESENTAÇÃO do carrossel no cliente do
// WhatsApp. Mapeia InteractiveMessage.CarouselMessage.CarouselCardType do
// protobuf (`internal/wa-noise/protocol/proto/waE2E/WAWebProtobufsE2E.proto`).
//
// O tipo é do CARROSSEL, não de cada cartão: um envio é inteiro HSCROLL ou
// inteiro ÁLBUM, e não há mistura.
type CarouselCardType string

const (
	// CarouselHScrollCards é o carrossel clássico: os cartões deslizam na
	// horizontal, cada um com a sua imagem, texto e botões. É o padrão.
	CarouselHScrollCards CarouselCardType = "hscroll_cards"

	// CarouselAlbumImage agrupa as imagens dos cartões como álbum.
	CarouselAlbumImage CarouselCardType = "album_image"
)

// CarouselCard é UM cartão do carrossel.
//
// No wire cada cartão é um InteractiveMessage COMPLETO — com o seu próprio
// header, corpo, rodapé e botões de fluxo nativo. Por isso os campos aqui
// espelham os de ButtonsPayload: um cartão é, literalmente, uma mensagem de
// botões aninhada dentro da moldura do carrossel.
//
// Image são os bytes JÁ OBTIDOS (o use case decide entre data URI e URL
// externa, como em SendImageUseCase e em ButtonsPayload.HeaderImage); vazio
// significa cartão sem imagem.
type CarouselCard struct {
	Title         string
	Body          string
	Footer        string
	Image         []byte
	ImageMimeType string
	Buttons       []InteractiveButton
}

// CarouselPayload é a metadata de protocolo que o use case repassa ao adapter
// para montar um InteractiveMessage com CarouselMessage.
//
// Buttons de cada cartão chegam aqui JÁ NORMALIZADOS pelo use case, com a
// mesma disciplina de ButtonsPayload: Title resolvido e truncado, ID
// resolvido, Type em caixa baixa. O adapter só traduz e monta.
type CarouselPayload struct {
	Body     string
	Footer   string
	CardType CarouselCardType
	Cards    []CarouselCard
}

// SendCarouselRequest represents the HTTP payload for POST /chat/send/carousel.
//
// Cards carry the same button vocabulary as SendButtonsRequest.Buttons: each
// card has its own Title, Body, Footer, Image and Buttons list. The use case
// normalises every card's buttons with the same chain as SendButtonsUseCase
// (title fallback -> truncate -> id fallback -> type lower -> discard unknown).
//
// Only HSCROLL_CARDS is exposed publicly; ALBUM_IMAGE does not render on
// current WhatsApp clients and is intentionally excluded from this surface
// (HOUSEKEEP F211).
type SendCarouselRequest struct {
	ChatTarget
	Phone string `json:"Phone"`
	Body  string `json:"Body"`
	// Footer is the carousel-level footer, below all cards.
	Footer       string                    `json:"Footer,omitempty"`
	Cards        []SendCarouselCardRequest `json:"Cards"`
	ID           string                    `json:"Id,omitempty"`
	ReplyTo      *ReplyContext             `json:"ReplyTo,omitempty"`
	MentionedJID []string                  `json:"MentionedJid,omitempty"`
}

func (r *SendCarouselRequest) ResolveChat() { ResolveChatField(&r.Phone, r.ChatAlias) }

// SendCarouselCardRequest is one card in a SendCarouselRequest.
type SendCarouselCardRequest struct {
	// Title is decorative: iOS does not render it; only Android shows it (F217).
	// Put required information in Body, not here.
	Title   string              `json:"Title,omitempty"`
	Body    string              `json:"Body"`
	Footer  string              `json:"Footer,omitempty"`
	Image   string              `json:"Image,omitempty"`
	Buttons []InteractiveButton `json:"Buttons"`
}

// SendCarouselResult is the response for POST /chat/send/carousel.
// Same shape as every other send capability: {message_id, timestamp, status}.
type SendCarouselResult struct {
	MessageID string
	Timestamp int64
	Status    string
}
