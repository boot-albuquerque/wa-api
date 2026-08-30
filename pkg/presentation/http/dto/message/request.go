package message

import (
	"time"

	"wa-api/pkg/domain"
)

// The REQUEST side of the message family: the types the client's JSON is
// decoded into, and the ToDomain conversions that build the use-case input.
//
// # Why the domain types stopped being the wire
//
// Every Send*Request in pkg/domain carried Go-idiomatic `json` tags — "Phone",
// "Body", "MentionedJid", "RowId" — so renaming an internal field broke every
// client in silence and no test saw it. The tags are gone from the domain now;
// the names live here, where changing one is visible as a contract change.
//
// # Validation is NOT here yet, and that is deliberate
//
// docs/HTTP-DTO-CONVENTIONS.md §6 puts Validate on the request DTO. This
// family's validation currently lives in the use cases and produces STABLE
// apperr codes ("missing Phone in Payload", "could not parse Phone", …) that a
// large suite asserts by code. Moving it here in the same commit that renames
// every wire key would put a rename and a behaviour change in one diff, which
// is exactly the mix a review cannot separate. It is a named follow-up, not an
// omission.
//
// # Optional vs zero
//
// A pointer field is one where "did not send" and "sent the zero" are
// different facts. Latitude/Longitude are the measured case (F121): 0 is a
// real coordinate, the point where the equator crosses the prime meridian, and
// with float64 a legitimate send there was refused as "missing Latitude".

// ChatTarget is embedded in every request whose destination has a
// route-specific name, so that `chat` works as a universal alias for it.
//
// Precedence is unchanged from domain.ChatTarget: the route-specific field
// wins when non-empty, because it is the explicit name the caller chose; the
// alias only fills an ABSENT field.
type ChatTarget struct {
	ChatAlias string `json:"chat"`
}

func resolveChatField(dst *string, alias string) {
	if *dst == "" {
		*dst = alias
	}
}

// ReplyContextRequest quotes an existing message.
//
// quoted_text is the text preview embedded as ContextInfo.QuotedMessage. It is
// OPTIONAL, and the claim that it is required was REFUTED BY MEASUREMENT
// (F222): two replies differing only in this field both rendered the quote
// bubble on a real iOS device.
type ReplyContextRequest struct {
	StanzaID    string `json:"stanza_id"`
	Participant string `json:"participant"`
	QuotedText  string `json:"quoted_text"`
}

// ToDomain builds the domain value. A nil DTO converts to a nil domain
// pointer, because "no reply_to key" and "reply to nothing" are the same fact
// here and the use cases already branch on nil.
func (r *ReplyContextRequest) ToDomain() *domain.ReplyContext {
	if r == nil {
		return nil
	}
	return &domain.ReplyContext{
		StanzaID:    r.StanzaID,
		Participant: r.Participant,
		QuotedText:  r.QuotedText,
	}
}

// InteractiveButtonRequest is ONE button of an interactive (NativeFlow)
// message.
//
// The THREE redundant pairs are kept because the historical handler accepted
// them as a chained fallback, in this exact order:
//
//	title <- text <- button_text
//	id    <- button_id <- (the already-resolved, already-truncated title)
//
// Narrowing that now would refuse payloads the route always accepted. What
// DID change is spelling: `buttonText` and `buttonId` were camelCase, which
// the canonical key rule rejects.
type InteractiveButtonRequest struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	Text        string `json:"text"`
	ButtonText  string `json:"button_text"`
	ID          string `json:"id"`
	ButtonID    string `json:"button_id"`
	URL         string `json:"url"`
	PhoneNumber string `json:"phone_number"`
	CopyCode    string `json:"copy_code"`
}

// ToDomain maps one button.
func (b InteractiveButtonRequest) ToDomain() domain.InteractiveButton {
	return domain.InteractiveButton{
		Type:        b.Type,
		Title:       b.Title,
		Text:        b.Text,
		ButtonText:  b.ButtonText,
		ID:          b.ID,
		ButtonID:    b.ButtonID,
		URL:         b.URL,
		PhoneNumber: b.PhoneNumber,
		CopyCode:    b.CopyCode,
	}
}

func interactiveButtonsToDomain(src []InteractiveButtonRequest) []domain.InteractiveButton {
	if src == nil {
		return nil
	}
	out := make([]domain.InteractiveButton, 0, len(src))
	for _, b := range src {
		out = append(out, b.ToDomain())
	}
	return out
}

// SendTextRequest is the body of POST /chat/send/text.
type SendTextRequest struct {
	ChatTarget
	Phone        string               `json:"phone"`
	Body         string               `json:"body"`
	LinkPreview  bool                 `json:"link_preview"`
	ID           string               `json:"id"`
	ReplyTo      *ReplyContextRequest `json:"reply_to"`
	MentionedJID []string             `json:"mentioned_jid"`
}

func (r *SendTextRequest) ResolveChat() { resolveChatField(&r.Phone, r.ChatAlias) }

// ToDomain produces the use-case input.
func (r SendTextRequest) ToDomain() domain.SendMessageRequest {
	return domain.SendMessageRequest{
		Phone:        r.Phone,
		Body:         r.Body,
		LinkPreview:  r.LinkPreview,
		ID:           r.ID,
		ReplyTo:      r.ReplyTo.ToDomain(),
		MentionedJID: r.MentionedJID,
	}
}

// SendImageRequest is the body of POST /chat/send/image.
//
// image is a UNION of two ways of OBTAINING the bytes — a data URI
// ("data:image/…;base64,…") and an http(s) URL — that converge on the same
// send protocol.
type SendImageRequest struct {
	ChatTarget
	Phone         string               `json:"phone"`
	Image         string               `json:"image"`
	Caption       string               `json:"caption"`
	ID            string               `json:"id"`
	MimeType      string               `json:"mime_type"`
	JPEGThumbnail []byte               `json:"jpeg_thumbnail"`
	ReplyTo       *ReplyContextRequest `json:"reply_to"`
	MentionedJID  []string             `json:"mentioned_jid"`
}

func (r *SendImageRequest) ResolveChat() { resolveChatField(&r.Phone, r.ChatAlias) }

// ToDomain produces the use-case input.
func (r SendImageRequest) ToDomain() domain.SendImageRequest {
	return domain.SendImageRequest{
		Phone:         r.Phone,
		Image:         r.Image,
		Caption:       r.Caption,
		ID:            r.ID,
		MimeType:      r.MimeType,
		JPEGThumbnail: r.JPEGThumbnail,
		ReplyTo:       r.ReplyTo.ToDomain(),
		MentionedJID:  r.MentionedJID,
	}
}

// SendDocumentRequest is the body of POST /chat/send/document.
type SendDocumentRequest struct {
	ChatTarget
	Phone        string               `json:"phone"`
	Document     string               `json:"document"`
	FileName     string               `json:"file_name"`
	Caption      string               `json:"caption"`
	ID           string               `json:"id"`
	MimeType     string               `json:"mime_type"`
	ReplyTo      *ReplyContextRequest `json:"reply_to"`
	MentionedJID []string             `json:"mentioned_jid"`
}

func (r *SendDocumentRequest) ResolveChat() { resolveChatField(&r.Phone, r.ChatAlias) }

// ToDomain produces the use-case input.
func (r SendDocumentRequest) ToDomain() domain.SendDocumentRequest {
	return domain.SendDocumentRequest{
		Phone:        r.Phone,
		Document:     r.Document,
		FileName:     r.FileName,
		Caption:      r.Caption,
		ID:           r.ID,
		MimeType:     r.MimeType,
		ReplyTo:      r.ReplyTo.ToDomain(),
		MentionedJID: r.MentionedJID,
	}
}

// SendAudioRequest is the body of POST /chat/send/audio.
//
// ptt is a POINTER on purpose: nil means "the client did not say", and the
// default for that is TRUE (voice note), not false.
//
// caption exists in the public contract but is INERT for audio: waE2E
// .AudioMessage has no caption field, so the use case sends it as a separate
// message (F116).
type SendAudioRequest struct {
	ChatTarget
	Phone    string               `json:"phone"`
	Audio    string               `json:"audio"`
	Caption  string               `json:"caption"`
	ID       string               `json:"id"`
	PTT      *bool                `json:"ptt"`
	MimeType string               `json:"mime_type"`
	Seconds  uint32               `json:"seconds"`
	Waveform []byte               `json:"waveform"`
	ReplyTo  *ReplyContextRequest `json:"reply_to"`
}

func (r *SendAudioRequest) ResolveChat() { resolveChatField(&r.Phone, r.ChatAlias) }

// ToDomain produces the use-case input.
func (r SendAudioRequest) ToDomain() domain.SendAudioRequest {
	return domain.SendAudioRequest{
		Phone:    r.Phone,
		Audio:    r.Audio,
		Caption:  r.Caption,
		ID:       r.ID,
		PTT:      r.PTT,
		MimeType: r.MimeType,
		Seconds:  r.Seconds,
		Waveform: r.Waveform,
		ReplyTo:  r.ReplyTo.ToDomain(),
	}
}

// SendStickerRequest is the body of POST /chat/send/sticker.
type SendStickerRequest struct {
	ChatTarget
	Phone         string               `json:"phone"`
	Sticker       string               `json:"sticker"`
	ID            string               `json:"id"`
	MimeType      string               `json:"mime_type"`
	PngThumbnail  []byte               `json:"png_thumbnail"`
	PackID        string               `json:"pack_id"`
	PackName      string               `json:"pack_name"`
	PackPublisher string               `json:"pack_publisher"`
	Emojis        []string             `json:"emojis"`
	ReplyTo       *ReplyContextRequest `json:"reply_to"`
}

func (r *SendStickerRequest) ResolveChat() { resolveChatField(&r.Phone, r.ChatAlias) }

// ToDomain produces the use-case input.
func (r SendStickerRequest) ToDomain() domain.SendStickerRequest {
	return domain.SendStickerRequest{
		Phone:         r.Phone,
		Sticker:       r.Sticker,
		ID:            r.ID,
		MimeType:      r.MimeType,
		PngThumbnail:  r.PngThumbnail,
		PackID:        r.PackID,
		PackName:      r.PackName,
		PackPublisher: r.PackPublisher,
		Emojis:        r.Emojis,
		ReplyTo:       r.ReplyTo.ToDomain(),
	}
}

// SendVideoRequest is the body of POST /chat/send/video.
type SendVideoRequest struct {
	ChatTarget
	Phone         string               `json:"phone"`
	Video         string               `json:"video"`
	Caption       string               `json:"caption"`
	ID            string               `json:"id"`
	MimeType      string               `json:"mime_type"`
	JPEGThumbnail []byte               `json:"jpeg_thumbnail"`
	ReplyTo       *ReplyContextRequest `json:"reply_to"`
	MentionedJID  []string             `json:"mentioned_jid"`
}

func (r *SendVideoRequest) ResolveChat() { resolveChatField(&r.Phone, r.ChatAlias) }

// ToDomain produces the use-case input.
func (r SendVideoRequest) ToDomain() domain.SendVideoRequest {
	return domain.SendVideoRequest{
		Phone:         r.Phone,
		Video:         r.Video,
		Caption:       r.Caption,
		ID:            r.ID,
		MimeType:      r.MimeType,
		JPEGThumbnail: r.JPEGThumbnail,
		ReplyTo:       r.ReplyTo.ToDomain(),
		MentionedJID:  r.MentionedJID,
	}
}

// SendContactRequest is the body of POST /chat/send/contact.
//
// vcard is passed through as a RAW string, without parsing or format
// validation — the same discipline the historical handler had.
type SendContactRequest struct {
	ChatTarget
	Phone   string               `json:"phone"`
	Name    string               `json:"name"`
	Vcard   string               `json:"vcard"`
	ID      string               `json:"id"`
	ReplyTo *ReplyContextRequest `json:"reply_to"`
}

func (r *SendContactRequest) ResolveChat() { resolveChatField(&r.Phone, r.ChatAlias) }

// ToDomain produces the use-case input.
func (r SendContactRequest) ToDomain() domain.SendContactRequest {
	return domain.SendContactRequest{
		Phone:   r.Phone,
		Name:    r.Name,
		Vcard:   r.Vcard,
		ID:      r.ID,
		ReplyTo: r.ReplyTo.ToDomain(),
	}
}

// SendLocationRequest is the body of POST /chat/send/location.
type SendLocationRequest struct {
	ChatTarget
	Phone string `json:"phone"`
	Name  string `json:"name"`
	// POINTER, not float64, to separate "not informed" from "zero" (F121).
	Latitude  *float64             `json:"latitude"`
	Longitude *float64             `json:"longitude"`
	ID        string               `json:"id"`
	ReplyTo   *ReplyContextRequest `json:"reply_to"`
}

func (r *SendLocationRequest) ResolveChat() { resolveChatField(&r.Phone, r.ChatAlias) }

// ToDomain produces the use-case input.
func (r SendLocationRequest) ToDomain() domain.SendLocationRequest {
	return domain.SendLocationRequest{
		Phone:     r.Phone,
		Name:      r.Name,
		Latitude:  r.Latitude,
		Longitude: r.Longitude,
		ID:        r.ID,
		ReplyTo:   r.ReplyTo.ToDomain(),
	}
}

// SendButtonsRequest is the body of POST /chat/send/buttons.
//
// text is the fallback of body (`body <- body <- text`, in the historical
// order) and not a field with a meaning of its own.
type SendButtonsRequest struct {
	ChatTarget
	Phone        string                     `json:"phone"`
	Body         string                     `json:"body"`
	Text         string                     `json:"text"`
	Title        string                     `json:"title"`
	Footer       string                     `json:"footer"`
	Image        string                     `json:"image"`
	Buttons      []InteractiveButtonRequest `json:"buttons"`
	ID           string                     `json:"id"`
	ReplyTo      *ReplyContextRequest       `json:"reply_to"`
	MentionedJID []string                   `json:"mentioned_jid"`
}

func (r *SendButtonsRequest) ResolveChat() { resolveChatField(&r.Phone, r.ChatAlias) }

// ToDomain produces the use-case input.
func (r SendButtonsRequest) ToDomain() domain.SendButtonsRequest {
	return domain.SendButtonsRequest{
		Phone:        r.Phone,
		Body:         r.Body,
		Text:         r.Text,
		Title:        r.Title,
		Footer:       r.Footer,
		Image:        r.Image,
		Buttons:      interactiveButtonsToDomain(r.Buttons),
		ID:           r.ID,
		ReplyTo:      r.ReplyTo.ToDomain(),
		MentionedJID: r.MentionedJID,
	}
}

// ListRowRequest is ONE row of a list section.
//
// It has ONE id field, `row_id`, where the domain type had FOUR — RowId,
// RowID, rowId and rowID — all four accepted as a chained fallback by the
// historical handler. Under the canonical key rule the four spellings are the
// SAME key, so the chain collapses to `row_id <- (the already-trimmed title)`.
// This is not a silent pick: only RowId was ever WRITTEN by the use case after
// normalisation and only RowId is READ by the adapter
// (pkg/infra/noise/adapters/chat/messenger_list.go), so the other three
// never carried a value past the boundary.
type ListRowRequest struct {
	Title       string `json:"title"`
	Description string `json:"desc"`
	RowID       string `json:"row_id"`
}

// ToDomain maps one row.
func (r ListRowRequest) ToDomain() domain.ListRow {
	return domain.ListRow{
		Title:       r.Title,
		Description: r.Description,
		RowID:       r.RowID,
	}
}

// ListSectionRequest is ONE section of a list.
type ListSectionRequest struct {
	Title string           `json:"title"`
	Rows  []ListRowRequest `json:"rows"`
}

// ToDomain maps one section.
func (s ListSectionRequest) ToDomain() domain.ListSection {
	out := domain.ListSection{Title: s.Title}
	if s.Rows == nil {
		return out
	}
	out.Rows = make([]domain.ListRow, 0, len(s.Rows))
	for _, row := range s.Rows {
		out.Rows = append(out.Rows, row.ToDomain())
	}
	return out
}

// SendListRequest is the body of POST /chat/send/list.
//
// TWO input forms: sections (preferred, multi-section) and list (legacy, a
// flat list the use case wraps in a single section).
//
// The historical body accepted FOUR keys as a chained fallback for the main
// text — Desc <- Body <- body <- text. Under the canonical key rule `Body` and
// `body` are the same key, and so are `Text` and `text`, so the chain is now
// THREE: desc <- body <- text. Nothing is lost: a caller could never send
// `Body` and `body` meaning two different things and expect a defined result.
type SendListRequest struct {
	ChatTarget
	Phone        string               `json:"phone"`
	ButtonText   string               `json:"button_text"`
	Desc         string               `json:"desc"`
	Body         string               `json:"body"`
	Text         string               `json:"text"`
	TopText      string               `json:"top_text"`
	FooterText   string               `json:"footer_text"`
	Sections     []ListSectionRequest `json:"sections"`
	List         []ListRowRequest     `json:"list"`
	ID           string               `json:"id"`
	ReplyTo      *ReplyContextRequest `json:"reply_to"`
	MentionedJID []string             `json:"mentioned_jid"`
}

func (r *SendListRequest) ResolveChat() { resolveChatField(&r.Phone, r.ChatAlias) }

// ToDomain produces the use-case input.
//
// Body2 in the domain type is the old `body` key of the four-key chain. It is
// left empty here because that key no longer exists separately; the use case
// still reads it, and leaving it empty is what makes the collapsed chain
// behave as desc <- body <- text.
func (r SendListRequest) ToDomain() domain.SendListRequest {
	out := domain.SendListRequest{
		Phone:        r.Phone,
		ButtonText:   r.ButtonText,
		Desc:         r.Desc,
		Body:         r.Body,
		Text:         r.Text,
		TopText:      r.TopText,
		FooterText:   r.FooterText,
		ID:           r.ID,
		ReplyTo:      r.ReplyTo.ToDomain(),
		MentionedJID: r.MentionedJID,
	}
	if r.Sections != nil {
		out.Sections = make([]domain.ListSection, 0, len(r.Sections))
		for _, s := range r.Sections {
			out.Sections = append(out.Sections, s.ToDomain())
		}
	}
	if r.List != nil {
		out.List = make([]domain.ListRow, 0, len(r.List))
		for _, row := range r.List {
			out.List = append(out.List, row.ToDomain())
		}
	}
	return out
}

// SendPollRequest is the body of POST /chat/send/poll.
//
// The destination field is `group` and not `phone` — the route was born for
// group polls and the name is the one the historical handler read.
type SendPollRequest struct {
	ChatTarget
	Group   string               `json:"group"`
	Header  string               `json:"header"`
	Options []string             `json:"options"`
	ID      string               `json:"id"`
	ReplyTo *ReplyContextRequest `json:"reply_to"`
}

func (r *SendPollRequest) ResolveChat() { resolveChatField(&r.Group, r.ChatAlias) }

// ToDomain produces the use-case input.
func (r SendPollRequest) ToDomain() domain.SendPollRequest {
	return domain.SendPollRequest{
		Group:   r.Group,
		Header:  r.Header,
		Options: r.Options,
		ID:      r.ID,
		ReplyTo: r.ReplyTo.ToDomain(),
	}
}

// SendPollVoteRequest is the body of POST /chat/send/pollvote.
//
// The four identifying fields are explicit and stateless by design: the vote
// is encrypted with a secret derived from the ORIGINAL poll message, so the
// adapter needs a full types.MessageInfo and not just an id. Looking the poll
// up from message_history instead would make the route depend on retention and
// fail silently once history was pruned.
type SendPollVoteRequest struct {
	ChatTarget
	Phone                string   `json:"phone"`
	Sender               string   `json:"sender"`
	PollMessageID        string   `json:"poll_message_id"`
	PollMessageTimestamp int64    `json:"poll_message_timestamp"`
	Options              []string `json:"options"`
	ID                   string   `json:"id"`
}

func (r *SendPollVoteRequest) ResolveChat() { resolveChatField(&r.Phone, r.ChatAlias) }

// ToDomain produces the use-case input.
func (r SendPollVoteRequest) ToDomain() domain.SendPollVoteRequest {
	return domain.SendPollVoteRequest{
		Phone:                r.Phone,
		Sender:               r.Sender,
		PollMessageID:        r.PollMessageID,
		PollMessageTimestamp: r.PollMessageTimestamp,
		Options:              r.Options,
		ID:                   r.ID,
	}
}

// TemplateButtonRequest is ONE button of a hydrated template.
//
// The fields are deliberately NOT mutually exclusive: the historical handler
// accepted a button carrying both url and phone_number and used only what the
// type asked for. Narrowing that now would refuse payloads the route always
// accepted.
type TemplateButtonRequest struct {
	DisplayText string `json:"display_text"`
	ID          string `json:"id"`
	URL         string `json:"url"`
	PhoneNumber string `json:"phone_number"`
	Type        string `json:"type"`
}

// ToDomain maps one template button.
func (b TemplateButtonRequest) ToDomain() domain.TemplateButton {
	return domain.TemplateButton{
		DisplayText: b.DisplayText,
		ID:          b.ID,
		URL:         b.URL,
		PhoneNumber: b.PhoneNumber,
		Type:        b.Type,
	}
}

// SendTemplateRequest is the body of POST /chat/send/template.
type SendTemplateRequest struct {
	ChatTarget
	Phone        string                  `json:"phone"`
	Content      string                  `json:"content"`
	Footer       string                  `json:"footer"`
	ID           string                  `json:"id"`
	Buttons      []TemplateButtonRequest `json:"buttons"`
	ReplyTo      *ReplyContextRequest    `json:"reply_to"`
	MentionedJID []string                `json:"mentioned_jid"`
}

func (r *SendTemplateRequest) ResolveChat() { resolveChatField(&r.Phone, r.ChatAlias) }

// ToDomain produces the use-case input.
func (r SendTemplateRequest) ToDomain() domain.SendTemplateRequest {
	out := domain.SendTemplateRequest{
		Phone:        r.Phone,
		Content:      r.Content,
		Footer:       r.Footer,
		ID:           r.ID,
		ReplyTo:      r.ReplyTo.ToDomain(),
		MentionedJID: r.MentionedJID,
	}
	if r.Buttons != nil {
		out.Buttons = make([]domain.TemplateButton, 0, len(r.Buttons))
		for _, b := range r.Buttons {
			out.Buttons = append(out.Buttons, b.ToDomain())
		}
	}
	return out
}

// SendCarouselCardRequest is ONE card of a carousel.
//
// title is decorative: iOS does not render it, only Android shows it (F217).
// Required information belongs in body.
type SendCarouselCardRequest struct {
	Title   string                     `json:"title"`
	Body    string                     `json:"body"`
	Footer  string                     `json:"footer"`
	Image   string                     `json:"image"`
	Buttons []InteractiveButtonRequest `json:"buttons"`
}

// ToDomain maps one card.
func (c SendCarouselCardRequest) ToDomain() domain.SendCarouselCardRequest {
	return domain.SendCarouselCardRequest{
		Title:   c.Title,
		Body:    c.Body,
		Footer:  c.Footer,
		Image:   c.Image,
		Buttons: interactiveButtonsToDomain(c.Buttons),
	}
}

// SendCarouselRequest is the body of POST /chat/send/carousel.
type SendCarouselRequest struct {
	ChatTarget
	Phone        string                    `json:"phone"`
	Body         string                    `json:"body"`
	Footer       string                    `json:"footer"`
	Cards        []SendCarouselCardRequest `json:"cards"`
	ID           string                    `json:"id"`
	ReplyTo      *ReplyContextRequest      `json:"reply_to"`
	MentionedJID []string                  `json:"mentioned_jid"`
}

func (r *SendCarouselRequest) ResolveChat() { resolveChatField(&r.Phone, r.ChatAlias) }

// ToDomain produces the use-case input.
func (r SendCarouselRequest) ToDomain() domain.SendCarouselRequest {
	out := domain.SendCarouselRequest{
		Phone:        r.Phone,
		Body:         r.Body,
		Footer:       r.Footer,
		ID:           r.ID,
		ReplyTo:      r.ReplyTo.ToDomain(),
		MentionedJID: r.MentionedJID,
	}
	if r.Cards != nil {
		out.Cards = make([]domain.SendCarouselCardRequest, 0, len(r.Cards))
		for _, c := range r.Cards {
			out.Cards = append(out.Cards, c.ToDomain())
		}
	}
	return out
}

// SendForwardRequest is the body of POST /chat/send/forward.
//
// Two forms. BY CONTENT (CAP-49): phone + body, and the API creates a new text
// message marked as forwarded with the caller's forwarding_score. BY KEY
// (CAP-55): phone + message_id + chat, and the API re-sends the stored
// original — media included, without re-upload — with the score DERIVED from
// it. When message_id is present, body and forwarding_score are ignored.
type SendForwardRequest struct {
	ChatTarget
	Phone           string               `json:"phone"`
	Body            string               `json:"body"`
	ForwardingScore *uint32              `json:"forwarding_score"`
	ID              string               `json:"id"`
	ReplyTo         *ReplyContextRequest `json:"reply_to"`
	MentionedJID    []string             `json:"mentioned_jid"`
	MessageID       string               `json:"message_id"`
	Chat            string               `json:"chat_jid"`
}

func (r *SendForwardRequest) ResolveChat() { resolveChatField(&r.Phone, r.ChatAlias) }

// ToDomain produces the use-case input.
func (r SendForwardRequest) ToDomain() domain.SendForwardRequest {
	return domain.SendForwardRequest{
		Phone:           r.Phone,
		Body:            r.Body,
		ForwardingScore: r.ForwardingScore,
		ID:              r.ID,
		ReplyTo:         r.ReplyTo.ToDomain(),
		MentionedJID:    r.MentionedJID,
		MessageID:       r.MessageID,
		Chat:            r.Chat,
	}
}

// DeleteMessageRequest is the body of POST /chat/delete.
type DeleteMessageRequest struct {
	ChatTarget
	Phone string `json:"phone"`
	ID    string `json:"id"`
}

func (r *DeleteMessageRequest) ResolveChat() { resolveChatField(&r.Phone, r.ChatAlias) }

// ToDomain produces the use-case input.
func (r DeleteMessageRequest) ToDomain() domain.DeleteMessageRequest {
	return domain.DeleteMessageRequest{Phone: r.Phone, ID: r.ID}
}

// SendEditMessageRequest is the body of POST /chat/send/edit.
//
// stanza_id and participant are pointers because an edit can legitimately
// carry an EMPTY context field, and "" is not the same as "not sent".
type SendEditMessageRequest struct {
	ChatTarget
	Phone        string   `json:"phone"`
	Body         string   `json:"body"`
	ID           string   `json:"id"`
	StanzaID     *string  `json:"stanza_id"`
	Participant  *string  `json:"participant"`
	MentionedJID []string `json:"mentioned_jid"`
}

func (r *SendEditMessageRequest) ResolveChat() { resolveChatField(&r.Phone, r.ChatAlias) }

// ToDomain produces the use-case input.
func (r SendEditMessageRequest) ToDomain() domain.SendEditMessageRequest {
	return domain.SendEditMessageRequest{
		Phone:        r.Phone,
		Body:         r.Body,
		ID:           r.ID,
		StanzaID:     r.StanzaID,
		Participant:  r.Participant,
		MentionedJID: r.MentionedJID,
	}
}

// ReactRequest is the body of POST /chat/react.
//
// body carries the emoji, or the literal "remove" to take a reaction back.
type ReactRequest struct {
	ChatTarget
	Phone       string `json:"phone"`
	Body        string `json:"body"`
	ID          string `json:"id"`
	Participant string `json:"participant"`
}

func (r *ReactRequest) ResolveChat() { resolveChatField(&r.Phone, r.ChatAlias) }

// ToDomain produces the use-case input.
func (r ReactRequest) ToDomain() domain.ReactRequest {
	return domain.ReactRequest{
		Phone:       r.Phone,
		Body:        r.Body,
		Id:          r.ID,
		Participant: r.Participant,
	}
}

// SendPresenceRequest is the body of POST /user/presence.
type SendPresenceRequest struct {
	Type string `json:"type"`
}

// ToDomain produces the use-case input.
func (r SendPresenceRequest) ToDomain() domain.SendPresenceRequest {
	return domain.SendPresenceRequest{Type: r.Type}
}

// SubscribePresenceRequest is the body of POST /user/presence/subscribe.
type SubscribePresenceRequest struct {
	ChatTarget
	Phone string `json:"phone"`
}

func (r *SubscribePresenceRequest) ResolveChat() { resolveChatField(&r.Phone, r.ChatAlias) }

// ToDomain produces the use-case input.
func (r SubscribePresenceRequest) ToDomain() domain.SubscribePresenceRequest {
	return domain.SubscribePresenceRequest{Phone: r.Phone}
}

// ChatPresenceRequest is the body of POST /chat/presence.
type ChatPresenceRequest struct {
	ChatTarget
	Phone string `json:"phone"`
	State string `json:"state"`
	Media string `json:"media"`
}

func (r *ChatPresenceRequest) ResolveChat() { resolveChatField(&r.Phone, r.ChatAlias) }

// ToDomain produces the use-case input.
func (r ChatPresenceRequest) ToDomain() domain.ChatPresenceRequest {
	return domain.ChatPresenceRequest{Phone: r.Phone, State: r.State, Media: r.Media}
}

// MarkReadRequest is the body of POST /chat/markread.
//
// chat_phone/sender_phone are the only spellings accepted — see
// HOUSEKEEP.md F302 for why domain.MarkReadRequest's "Chat"/"Sender" legacy
// fields do NOT get a wire name here: they resolve to an empty JID
// (mark_read.go never ported that parsing) and are kept only because a test
// locks that as inherited upstream behaviour, not because a client should be
// able to reach them.
type MarkReadRequest struct {
	ID          []string `json:"id"`
	ChatPhone   string   `json:"chat_phone"`
	SenderPhone string   `json:"sender_phone"`
}

// ToDomain produces the use-case input.
func (r MarkReadRequest) ToDomain() domain.MarkReadRequest {
	return domain.MarkReadRequest{
		Id:          r.ID,
		ChatPhone:   r.ChatPhone,
		SenderPhone: r.SenderPhone,
	}
}

// MuteChatRequest is the body of POST /chat/mute (HOUSEKEEP F323).
//
// The wire keys already matched domain.MuteChatRequest's Go names lowercased
// — this DTO buys nothing new on the wire. What it buys is a COMPILE ERROR:
// rename a field in domain.MuteChatRequest and this file's ToDomain stops
// building, instead of silently changing the published contract.
type MuteChatRequest struct {
	ChatTarget
	Jid          string         `json:"jid"`
	Mute         bool           `json:"mute"`
	MuteDuration *time.Duration `json:"mute_duration,omitempty"`
}

func (r *MuteChatRequest) ResolveChat() { resolveChatField(&r.Jid, r.ChatAlias) }

// ToDomain produces the use-case input.
func (r MuteChatRequest) ToDomain() domain.MuteChatRequest {
	return domain.MuteChatRequest{
		Jid:          r.Jid,
		Mute:         r.Mute,
		MuteDuration: r.MuteDuration,
	}
}

// ArchiveChatRequest is the body of POST /chat/archive (HOUSEKEEP F323). See
// MuteChatRequest's doc comment for why this DTO exists despite the wire
// keys already matching.
type ArchiveChatRequest struct {
	ChatTarget
	Jid     string `json:"jid"`
	Archive bool   `json:"archive"`
}

func (r *ArchiveChatRequest) ResolveChat() { resolveChatField(&r.Jid, r.ChatAlias) }

// ToDomain produces the use-case input.
func (r ArchiveChatRequest) ToDomain() domain.ArchiveChatRequest {
	return domain.ArchiveChatRequest{Jid: r.Jid, Archive: r.Archive}
}

// PinChatRequest is the body of POST /chat/pin (HOUSEKEEP F323). See
// MuteChatRequest's doc comment for why this DTO exists despite the wire
// keys already matching.
type PinChatRequest struct {
	ChatTarget
	Jid string `json:"jid"`
	Pin bool   `json:"pin"`
}

func (r *PinChatRequest) ResolveChat() { resolveChatField(&r.Jid, r.ChatAlias) }

// ToDomain produces the use-case input.
func (r PinChatRequest) ToDomain() domain.PinChatRequest {
	return domain.PinChatRequest{Jid: r.Jid, Pin: r.Pin}
}

// RequestUnavailableMessageRequest is the body of POST
// /chat/request-unavailable-message (HOUSEKEEP F323). See MuteChatRequest's
// doc comment for why this DTO exists despite the wire keys already
// matching.
//
// No ChatTarget/`chat` alias here: unlike Mute/Archive/Pin, the domain type
// has no ResolveChat and the route never accepted the alias.
type RequestUnavailableMessageRequest struct {
	Chat   string `json:"chat"`
	Sender string `json:"sender"`
	ID     string `json:"id"`
}

// ToDomain produces the use-case input.
func (r RequestUnavailableMessageRequest) ToDomain() domain.RequestUnavailableMessageRequest {
	return domain.RequestUnavailableMessageRequest{
		Chat:   r.Chat,
		Sender: r.Sender,
		ID:     r.ID,
	}
}
