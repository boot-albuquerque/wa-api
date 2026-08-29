package message

import (
	"context"
	"strings"

	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// defaultListButtonText é o rótulo do botão que abre a lista quando
// ButtonText vem vazio ou ausente. O histórico escrevia o literal "Select"
// (`git show 41bc8e2^:handlers.go`, função SendList) e nunca o expôs no
// payload público — constante nomeada aqui, e não literal solto (ADR-0004).
const defaultListButtonText = "Select"

// defaultLegacySectionTitle é o título da seção única em que o modo legado
// (`List`) embrulha suas linhas quando TopText vem vazio. O histórico
// escrevia o literal "Menu".
const defaultLegacySectionTitle = "Menu"

// SendListUseCase envia uma mensagem de lista de verdade: normaliza seções
// e linhas do payload (as duas formas de entrada, as duas cadeias de
// fallback e os três descartes silenciosos do contrato histórico — HOUSEKEEP
// F149) e envia pela porta de verdade. Só devolve domain.StatusSent depois
// que o envio retornar sucesso — nunca antes (mesma disciplina de
// SendButtonsUseCase, CAP-21).
//
// Este use case não conhece protobuf: domain.ListSection/ListRow são DTO de
// domínio; a tradução para waE2E.ListMessage é do adapter.
type SendListUseCase struct {
	messages appport.SimpleMessenger
	jids     appport.JIDResolver
	logger   appport.Logger
}

// NewSendListUseCase cria uma nova instância do usecase.
func NewSendListUseCase(sm appport.SimpleMessenger, jr appport.JIDResolver, l appport.Logger) *SendListUseCase {
	return &SendListUseCase{
		messages: sm,
		jids:     jr,
		logger:   l,
	}
}

// Execute normaliza e valida o payload, resolve o destinatário e envia a
// mensagem de lista pela porta de verdade.
//
// A validação é a HISTÓRICA (`git show 41bc8e2^:handlers.go`, função
// SendList): Phone ausente => 400; corpo ausente DEPOIS da cadeia de
// fallback (Desc <- Body <- body <- text) => 400 "missing Desc/Body in
// payload"; Sections E List ambos vazios => 400 "missing Sections (or List)
// in payload"; e, depois de normalizar, se NENHUMA seção sobrou (a rede
// final dos três descartes silenciosos) => 400 "no valid sections/rows
// found in payload".
//
// Phone é resolvido com ResolveJID, e não ResolveQualifiedJID, mesmo
// motivo de SendButtonsUseCase/SendTemplateUseCase: o histórico o passava
// por validateMessageFields -> parseJID.
//
// ContextInfo/QuotedMessage do payload histórico NÃO têm equivalente aqui:
// reply-to está fora do escopo (HOUSEKEEP F148/F149), e no histórico eram
// sempre nil quando o cliente não pedia citação — caso em que
// validateMessageFields se reduzia a parseJID(Phone), exatamente o que
// ResolveJID faz.
func (uc *SendListUseCase) Execute(ctx context.Context, txtID string, req domain.SendListRequest) (*domain.SendListResult, error) {
	body := strings.TrimSpace(req.Desc)
	if body == "" {
		body = strings.TrimSpace(req.Body)
	}
	if body == "" {
		body = strings.TrimSpace(req.Text)
	}

	if req.Phone == "" {
		return nil, apperr.New("missing_phone", apperr.CategoryValidation, "missing Phone in payload", false, nil)
	}
	if body == "" {
		return nil, apperr.New("missing_desc", apperr.CategoryValidation, "missing Desc/Body in payload", false, nil)
	}
	if len(req.Sections) == 0 && len(req.List) == 0 {
		return nil, apperr.New("missing_sections", apperr.CategoryValidation, "missing Sections (or List) in payload", false, nil)
	}

	sections, droppedRows, droppedSecs := normalizeListSections(req.Sections, req.List, req.TopText)
	for _, d := range droppedRows {
		uc.logger.Warn(ctx, "list row dropped: empty title",
			"txtID", txtID,
			"clientMsgID", req.ID,
			"sectionIndex", d.SectionIndex,
			"rowIndex", d.RowIndex,
			"reason", d.Reason)
	}
	for _, d := range droppedSecs {
		uc.logger.Warn(ctx, "list section dropped: no surviving rows",
			"txtID", txtID,
			"clientMsgID", req.ID,
			"sectionIndex", d.SectionIndex,
			"sectionTitle", d.Title,
			"reason", d.Reason)
	}
	if len(sections) == 0 {
		return nil, apperr.New("no_valid_sections", apperr.CategoryValidation, "no valid sections/rows found in payload", false, nil)
	}

	if err := uc.messages.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no noise session", "txtID", txtID, "error", err)
		return nil, err
	}

	recipient, err := uc.jids.ResolveJID(ctx, req.Phone)
	if err != nil {
		uc.logger.Warn(ctx, "invalid phone in send list payload", "txtID", txtID, "error", err)
		return nil, apperr.New("invalid_phone", apperr.CategoryValidation, "could not parse Phone", false, nil)
	}

	buttonText := strings.TrimSpace(req.ButtonText)
	if buttonText == "" {
		buttonText = defaultListButtonText
	}

	payload := domain.ListPayload{
		Body:       body,
		ButtonText: buttonText,
		Title:      strings.TrimSpace(req.TopText),
		Footer:     strings.TrimSpace(req.FooterText),
		Sections:   sections,
	}

	sent, err := uc.messages.SendList(ctx, txtID, recipient, payload, req.ReplyTo, req.MentionedJID, req.ID)
	if err != nil {
		uc.logger.Error(ctx, "failed to send list message", "txtID", txtID, "error", err)
		return nil, err
	}

	result := &domain.SendListResult{
		MessageID: sent.ID,
		Timestamp: sent.Timestamp.Unix(),
		Status:    domain.StatusSent,
	}

	uc.logger.Info(ctx, "list sent", "msgID", result.MessageID)
	return result, nil
}

// resolveRowID escolhe o identificador da linha: row_id quando vem, e o
// título JÁ TRIMADO como último recurso quando os quatro vêm vazios.
func resolveRowID(row domain.ListRow, trimmedTitle string) string {
	for _, candidate := range []string{row.RowID} {
		if v := strings.TrimSpace(candidate); v != "" {
			return v
		}
	}
	return trimmedTitle
}

// normalizeRows resolve título e RowId de cada linha e DESCARTA a linha sem
// título — PRIMEIRO descarte silencioso do contrato histórico
// (`if rowTitle == "" { continue }`), preservado por decisão explícita
// (HOUSEKEEP F149, mesma disciplina de F121/F135/F147/F148). Mandar uma
// seção com três linhas, duas sem título, faz DUAS sumirem sem aviso;
// travado por TestSendList_RowWithoutTitleIsSilentlyDiscarded.
//
// A função devolve o que descartou para que o chamador registe (mesma forma
// de normalizeInteractiveButtons em send_buttons.go / F186).
func normalizeRows(in []domain.ListRow, sectionIndex int) ([]domain.ListRow, []droppedRow) {
	out := make([]domain.ListRow, 0, len(in))
	var dropped []droppedRow
	for i, row := range in {
		title := strings.TrimSpace(row.Title)
		if title == "" {
			dropped = append(dropped, droppedRow{
				SectionIndex: sectionIndex,
				RowIndex:     i,
				Reason:       rowDropReasonEmptyTitle,
			})
			continue
		}
		out = append(out, domain.ListRow{
			Title:       title,
			Description: strings.TrimSpace(row.Description),
			RowID:       resolveRowID(row, title),
		})
	}
	return out, dropped
}

// normalizeListSections resolve as DUAS formas de entrada do contrato
// histórico e os TRÊS descartes silenciosos (HOUSEKEEP F149):
//
//  1. linha sem título: descartada dentro de normalizeRows.
//  2. seção que ficou sem linhas (todas descartadas, ou já vazia):
//     descartada aqui — SEGUNDO descarte silencioso
//     (`if len(rows) == 0 { continue }`), travado por
//     TestSendList_SectionWithoutRowsIsSilentlyDiscarded.
//  3. rede: se NENHUMA seção sobreviveu, a lista volta vazia e o Execute
//     recusa com "no valid sections/rows found in payload" — TERCEIRO
//     descarte, que é o único que vira erro (`if len(protoSections) == 0`
//     no histórico). Travado por
//     TestSendList_AllSectionsDiscardedRejects.
//
// `Sections` é a forma PREFERIDA (multi-seção); na ausência dela, `List`
// (legado) é embrulhado numa seção única cujo título vem de topText,
// caindo em defaultLegacySectionTitle quando topText vem vazio.
//
// Devolve os descartes de linha e de seção para que o chamador registe
// (mesma forma de normalizeInteractiveButtons / F186).
func normalizeListSections(sections []domain.ListSection, legacy []domain.ListRow, topText string) ([]domain.ListSection, []droppedRow, []droppedSection) {
	var allDroppedRows []droppedRow
	var droppedSections []droppedSection

	if len(sections) > 0 {
		out := make([]domain.ListSection, 0, len(sections))
		for i, sec := range sections {
			rows, dr := normalizeRows(sec.Rows, i)
			allDroppedRows = append(allDroppedRows, dr...)
			if len(rows) == 0 {
				droppedSections = append(droppedSections, droppedSection{
					SectionIndex: i,
					Title:        strings.TrimSpace(sec.Title),
					Reason:       sectionDropReasonNoRows,
				})
				continue
			}
			out = append(out, domain.ListSection{Title: strings.TrimSpace(sec.Title), Rows: rows})
		}
		return out, allDroppedRows, droppedSections
	}

	rows, dr := normalizeRows(legacy, 0)
	allDroppedRows = append(allDroppedRows, dr...)
	if len(rows) == 0 {
		return nil, allDroppedRows, nil
	}

	sectionTitle := strings.TrimSpace(topText)
	if sectionTitle == "" {
		sectionTitle = defaultLegacySectionTitle
	}
	return []domain.ListSection{{Title: sectionTitle, Rows: rows}}, allDroppedRows, nil
}

// droppedRow is one list row that normalizeRows refused, kept so the caller
// can say WHICH row went and WHY — same pattern as droppedButton (F186).
type droppedRow struct {
	SectionIndex int
	RowIndex     int
	Reason       string
}

// droppedSection is one section that normalizeListSections refused because
// all of its rows were discarded.
type droppedSection struct {
	SectionIndex int
	Title        string
	Reason       string
}

const (
	rowDropReasonEmptyTitle = "empty_title"
	sectionDropReasonNoRows = "no_surviving_rows"
)
