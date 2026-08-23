package message

import (
	"context"
	"net/http"
	"strings"

	"github.com/vincent-petithory/dataurl"

	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// buttonsHeaderImageMaxBytes é o teto de bytes aceito ao obter a imagem do
// header — paridade com o limite histórico do wuzapi, que usava
// openGraphImageMaxBytes nesta rota (`git show 41bc8e2^:handlers.go`, linha
// 2142; a constante está em `41bc8e2^:helpers.go`, linha 52: 10MB).
//
// CAP-21 (divergência deliberada, mesma da D1 do CAP-03): historicamente o
// ramo data URI não tinha teto nenhum — o limite só valia para o ramo URL.
// Aqui vale para os dois, senão o limite do ramo URL é um bypass trivial
// (basta mandar a imagem como data URI).
const buttonsHeaderImageMaxBytes int64 = 10 * 1024 * 1024

// buttonTitleMaxRunes é o limite de RUNAS do título de um botão nativo — o
// histórico truncava em 20 com o comentário "WhatsApp limita o título a 20
// caracteres nos botões" (`git show 41bc8e2^:handlers.go`, linha 2069).
//
// É um limite de WIRE, e mesmo assim vive AQUI, junto da normalização, e não
// no adapter: a cadeia de fallback do identificador usa o título JÁ TRUNCADO
// (`id = title` acontece DEPOIS do corte, mesma função histórica). Truncar
// no adapter mudaria o `id` que volta no clique de quem recebeu a mensagem.
const buttonTitleMaxRunes = 20

// SendButtonsUseCase envia uma mensagem interativa de verdade: normaliza os
// botões do payload, obtém os bytes do header opcional (por URL externa via
// infra SSRF-safe, ou por decode local de data URI, como SendImageUseCase) e
// envia pela porta de verdade. Só devolve domain.StatusSent depois que o
// envio retorna sucesso — nunca antes (mesma disciplina de
// SendMessageUseCase, CAP-01, e de SendTemplateUseCase, CAP-15).
//
// Este use case não conhece protobuf: domain.InteractiveButton é DTO de
// domínio, e qual `Name` e quais parâmetros cada tipo vira no NativeFlow é
// tradução de port.InteractiveMessenger.
type SendButtonsUseCase struct {
	messages appport.InteractiveMessenger
	jids     appport.JIDResolver
	fetcher  appport.MediaFetcher
	logger   appport.Logger
}

// NewSendButtonsUseCase cria uma nova instância do usecase.
func NewSendButtonsUseCase(im appport.InteractiveMessenger, jr appport.JIDResolver, mf appport.MediaFetcher, l appport.Logger) *SendButtonsUseCase {
	return &SendButtonsUseCase{
		messages: im,
		jids:     jr,
		fetcher:  mf,
		logger:   l,
	}
}

// Execute normaliza e valida o payload, resolve o destinatário e envia a
// mensagem interativa pela porta de verdade.
//
// A validação é a HISTÓRICA, e com a mesma FORMA (`git show
// 41bc8e2^:handlers.go`, função SendButtons): UMA recusa cobrindo os três
// campos obrigatórios ("missing Phone, Body or Buttons") e não uma por
// campo, porque é assim que a rota sempre respondeu; e uma segunda recusa
// para o caso em que todos os botões são descartados na normalização ("no
// valid buttons parsed"). O corpo aceita `Body` OU `text` — nesta ordem.
//
// Phone é resolvido com ResolveJID, e não com ResolveQualifiedJID, pelo
// mesmo motivo de SendTemplateUseCase: o histórico o passava por
// validateMessageFields -> parseJID, que aplica o servidor padrão a um
// número sem servidor. Trocar por ResolveQualifiedJID passaria a rejeitar
// entradas que a rota hoje aceita.
//
// O `ContextInfo` que o histórico entregava a validateMessageFields (os
// ponteiros StanzaID e Participant) NÃO tem equivalente aqui: reply-to está
// fora do escopo do CAP-21 por decisão registrada (HOUSEKEEP F148), o DTO
// não tem esses campos, e no histórico eles eram sempre nil quando o cliente
// não pedia citação — caso em que validateMessageFields se reduzia a
// parseJID(Phone), que é exatamente o que ResolveJID faz. Separar as duas
// coisas foi possível SEM implementar reply-to porque a citação e' um
// parâmetro da MENSAGEM, não do destinatário.
func (uc *SendButtonsUseCase) Execute(ctx context.Context, txtID string, req domain.SendButtonsRequest) (*domain.SendButtonsResult, error) {
	body := strings.TrimSpace(req.Body)
	if body == "" {
		body = strings.TrimSpace(req.Text)
	}

	if req.Phone == "" || body == "" || len(req.Buttons) == 0 {
		return nil, apperr.New("missing_buttons_fields", apperr.CategoryValidation,
			"missing Phone, Body or Buttons", false, nil)
	}

	buttons, dropped := normalizeInteractiveButtons(req.Buttons)
	// DISCARD RECORD (HOUSEKEEP F185). The discard itself is preserved
	// behaviour (F148); what changes here is that it stops being invisible.
	// The same file already Warns on every reason the header image can be
	// dropped (headerImageBytes below) — the button discard was the odd one
	// out, and that divergence is internal, not historical.
	for _, d := range dropped {
		uc.logger.Warn(ctx, "buttons button dropped: unknown type",
			"txtID", txtID,
			// clientMsgID, not the server message ID: the discard happens
			// BEFORE the send, so no server ID exists yet. req.ID is the only
			// identifier the caller can correlate on, and it is empty when the
			// caller did not supply one.
			"clientMsgID", req.ID,
			"receivedType", d.ReceivedType,
			"title", d.Title,
			"reason", buttonDropReasonUnknownType,
			"acceptedTypes", acceptedButtonTypes())
	}
	if len(buttons) == 0 {
		return nil, apperr.New("no_valid_buttons", apperr.CategoryValidation,
			"no valid buttons parsed, accepted types: "+acceptedButtonTypes(), false, nil)
	}

	if err := uc.messages.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	recipient, err := uc.jids.ResolveJID(ctx, req.Phone)
	if err != nil {
		uc.logger.Warn(ctx, "invalid phone in send buttons payload", "txtID", txtID, "error", err)
		return nil, apperr.New("invalid_phone", apperr.CategoryValidation, "could not parse Phone", false, nil)
	}

	headerImage := uc.headerImageBytes(ctx, txtID, req.Image)

	payload := domain.ButtonsPayload{
		Body:        body,
		Title:       req.Title,
		Footer:      req.Footer,
		Buttons:     buttons,
		HeaderImage: headerImage,
	}
	if len(headerImage) > 0 {
		payload.HeaderImageMimeType = http.DetectContentType(headerImage)
	}

	sent, err := uc.messages.SendButtons(ctx, txtID, recipient, payload, req.ReplyTo, req.ID)
	if err != nil {
		uc.logger.Error(ctx, "failed to send buttons message", "txtID", txtID, "error", err)
		return nil, err
	}

	result := &domain.SendButtonsResult{
		MessageID: sent.ID,
		Timestamp: sent.Timestamp.Unix(),
		Status:    domain.StatusSent,
	}

	uc.logger.Info(ctx, "buttons sent", "msgID", result.MessageID)
	return result, nil
}

// normalizeInteractiveButtons resolve os fallbacks de cada botão, trunca o
// título ao limite do wire, normaliza o tipo e DESCARTA o que o histórico
// descartava. A ordem das operações é a do histórico e é observável:
//
//  1. título: Title <- Text <- ButtonText; ainda vazio => descarta o botão
//  2. trunca o título em buttonTitleMaxRunes RUNAS (não bytes)
//  3. identificador: ID <- ButtonID <- título JÁ TRUNCADO
//  4. tipo: ToLower(TrimSpace(Type)); vazio => "reply"
//  5. tipo fora dos quatro conhecidos => DESCARTA o botão, em silêncio
//
// O passo 5 é COMPORTAMENTO PRESERVADO POR DECISÃO EXPLÍCITA, não descuido
// (HOUSEKEEP F148, mesma disciplina da F121 e da F135). Ele diverge do
// send_template, onde tipo desconhecido cai em quickreply: aqui um erro de
// digitação em `type` faz o botão SUMIR, e o cliente que manda três recebe
// 200 com dois. A rede que existe é parcial e também é histórica — se TODOS
// forem descartados, o Execute recusa com "no valid buttons parsed".
// "Consertar" isto é mudança de contrato público e precisa de decisão, não
// de um `default` reescrito de passagem; travado por
// TestSendButtons_UnknownTypeIsSilentlyDiscarded.
func normalizeInteractiveButtons(in []domain.InteractiveButton) ([]domain.InteractiveButton, []droppedButton) {
	out := make([]domain.InteractiveButton, 0, len(in))
	var dropped []droppedButton

	for _, btn := range in {
		title := strings.TrimSpace(btn.Title)
		if title == "" {
			title = strings.TrimSpace(btn.Text)
		}
		if title == "" {
			title = strings.TrimSpace(btn.ButtonText)
		}
		if title == "" {
			continue
		}

		if runes := []rune(title); len(runes) > buttonTitleMaxRunes {
			title = string(runes[:buttonTitleMaxRunes])
		}

		id := strings.TrimSpace(btn.ID)
		if id == "" {
			id = strings.TrimSpace(btn.ButtonID)
		}
		if id == "" {
			id = title
		}

		buttonType := strings.ToLower(strings.TrimSpace(btn.Type))
		if buttonType == "" {
			buttonType = domain.ButtonTypeReply
		}
		switch buttonType {
		case domain.ButtonTypeReply, domain.ButtonTypeCTAURL,
			domain.ButtonTypeCTACall, domain.ButtonTypeCopy:
		default:
			// Reporting the discard instead of swallowing it. The function
			// stays pure — it returns what it dropped and lets the caller do
			// the logging, so the discard is assertable in a test without a
			// log double.
			dropped = append(dropped, droppedButton{ReceivedType: btn.Type, Title: title})
			continue
		}

		out = append(out, domain.InteractiveButton{
			Type:        buttonType,
			Title:       title,
			ID:          id,
			URL:         btn.URL,
			PhoneNumber: btn.PhoneNumber,
			CopyCode:    btn.CopyCode,
		})
	}

	return out, dropped
}

// droppedButton is one button that normalizeInteractiveButtons refused, kept
// so the caller can say WHICH button went and why.
//
// ReceivedType is the type exactly as the caller wrote it, NOT the lowercased
// and trimmed form: the whole point of the record is to show the operator the
// typo they made, and normalising it first would hide a stray space or a
// capital letter — the two mistakes most likely to produce a silent discard.
type droppedButton struct {
	ReceivedType string
	Title        string
}

// buttonDropReasonUnknownType is the reason recorded when a button is refused
// for its type. Named for the same reason as discardReasonUnclassified in the
// bootstrap package: it is what an operator greps for (ADR-0004).
const buttonDropReasonUnknownType = "unknown_button_type"

// acceptedButtonTypes lists the four types normalizeInteractiveButtons keeps.
//
// It exists because "no valid buttons parsed" without the list is a dead end
// for the caller: `quickreply` is valid at /chat/send/template and discarded
// here, so a caller who knows one route has every reason to expect the other
// to agree. That was measured in the field (F185) on the first real use.
func acceptedButtonTypes() string {
	return strings.Join([]string{
		domain.ButtonTypeReply,
		domain.ButtonTypeCTAURL,
		domain.ButtonTypeCTACall,
		domain.ButtonTypeCopy,
	}, ", ")
}

// headerImageBytes obtém os bytes da imagem de header, ou devolve nil.
//
// DESCARTE SILENCIOSO PRESERVADO, com divergência de OBSERVABILIDADE: o
// histórico ignorava qualquer falha aqui (data URI que não decodifica, fetch
// que falha, string que não é nem data URI nem URL) e seguia enviando a
// mensagem SEM header, sem avisar ninguém (`git show 41bc8e2^:handlers.go`,
// linhas 2136-2148: os erros caem em `if ... == nil` e o `imgMsg` fica nil).
// O contrato HTTP é preservado — nenhuma dessas falhas vira erro para o
// cliente, porque isso rejeitaria requisições que a rota sempre aceitou —,
// mas cada descarte passa a emitir um Warn: silêncio total tornava o defeito
// impossível de diagnosticar em produção.
func (uc *SendButtonsUseCase) headerImageBytes(ctx context.Context, txtID, image string) []byte {
	if image == "" {
		return nil
	}

	var data []byte

	switch {
	case isDataURIImage(image):
		decoded, err := dataurl.DecodeString(image)
		if err != nil {
			uc.logger.Warn(ctx, "buttons header image dropped: data uri did not decode", "txtID", txtID, "error", err)
			return nil
		}
		data = decoded.Data

	case isHTTPImageURL(image):
		fetched, _, err := uc.fetcher.FetchBytes(ctx, image, buttonsHeaderImageMaxBytes)
		if err != nil {
			uc.logger.Warn(ctx, "buttons header image dropped: fetch failed", "txtID", txtID, "error", err)
			return nil
		}
		data = fetched

	default:
		uc.logger.Warn(ctx, "buttons header image dropped: unsupported source", "txtID", txtID)
		return nil
	}

	if len(data) == 0 {
		uc.logger.Warn(ctx, "buttons header image dropped: empty body", "txtID", txtID)
		return nil
	}
	if int64(len(data)) > buttonsHeaderImageMaxBytes {
		uc.logger.Warn(ctx, "buttons header image dropped: exceeds size limit",
			"txtID", txtID, "bytes", len(data), "limit", buttonsHeaderImageMaxBytes)
		return nil
	}

	return data
}
