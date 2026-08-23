package message_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
)

// Este arquivo recebe os eixos que send_message_test.go cobria para SendList
// enquanto ele era um use case de port.MessageComposer que só validava e
// devolvia "validated" sem enviar nada (CAP-22 migrou-o para
// port.SimpleMessenger, com envio de verdade). Mesma estrutura de
// send_buttons_test.go/send_template_test.go.
//
// O que este bloco tem de próprio está na HOUSEKEEP F149: DUAS formas de
// entrada (Sections/List), DUAS cadeias de fallback (corpo: 4 níveis; id da
// linha: 5 níveis) e TRÊS descartes silenciosos. Uma cadeia de 5 níveis passa
// num teste que exercite só o primeiro — por isso cada cadeia aqui é
// percorrida NÍVEL POR NÍVEL. A tradução para waE2E.ListMessage está medida
// no adapter (pkg/infra/wa-noise/adapters/chat/messenger_list_test.go).

const listPhone = "5511987654321"

func listJIDResolver() *contractsfake.JIDResolver {
	return &contractsfake.JIDResolver{
		ResolveJIDFunc: func(_ context.Context, raw string) (domain.JID, error) {
			raw = strings.TrimPrefix(raw, "+")
			if strings.ContainsRune(raw, '@') {
				return domain.JID(raw), nil
			}
			return domain.JID(raw + "@s.whatsapp.net"), nil
		},
	}
}

func validListRequest() domain.SendListRequest {
	return domain.SendListRequest{
		Phone: listPhone,
		Desc:  "Escolha um item",
		Sections: []domain.ListSection{
			{Title: "Cardápio", Rows: []domain.ListRow{{Title: "Item 1"}}},
		},
	}
}

func newSendList(sm *contractsfake.SimpleMessenger, jr *contractsfake.JIDResolver, l *contractsfake.Logger) *message.SendListUseCase {
	return message.NewSendListUseCase(sm, jr, l)
}

// TestSendList_MissingRequiredField cobre as três recusas do contrato
// histórico, cada uma com sua mensagem PRÓPRIA (diferente de SendButtons, que
// tinha uma recusa só para os três campos).
func TestSendList_MissingRequiredField(t *testing.T) {
	cases := []struct {
		name  string
		req   domain.SendListRequest
		cause string
	}{
		{"Phone", domain.SendListRequest{Desc: "x", Sections: validListRequest().Sections}, "missing Phone in payload"},
		{"Desc_ausente", domain.SendListRequest{Phone: listPhone, Sections: validListRequest().Sections}, "missing Desc/Body in payload"},
		{"Desc_so_espaco", domain.SendListRequest{Phone: listPhone, Desc: "   ", Sections: validListRequest().Sections}, "missing Desc/Body in payload"},
		{"Sections_e_List_ausentes", domain.SendListRequest{Phone: listPhone, Desc: "x"}, "missing Sections (or List) in payload"},
		{"Sections_e_List_vazios", domain.SendListRequest{Phone: listPhone, Desc: "x", Sections: []domain.ListSection{}, List: []domain.ListRow{}}, "missing Sections (or List) in payload"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sm := &contractsfake.SimpleMessenger{}
			logger := &contractsfake.Logger{}

			_, err := newSendList(sm, &contractsfake.JIDResolver{}, logger).Execute(context.Background(), userID, tc.req)

			if err == nil {
				t.Fatalf("request invalido (%s) foi aceito", tc.name)
			}
			if got := err.Error(); !strings.Contains(got, tc.cause) {
				t.Errorf("causa: got %q, want conter %q", got, tc.cause)
			}
			if n := len(sm.EnsureSessionCalls); n != 0 {
				t.Errorf("validacao falhou mas a sessao foi consultada %d vez(es)", n)
			}
			if n := len(sm.SendListCalls); n != 0 {
				t.Errorf("validacao falhou mas SendList foi chamado %d vez(es)", n)
			}
		})
	}
}

// TestSendList_BodyFallbackChain percorre os QUATRO níveis da cadeia do
// corpo, um por vez: cada nível só é usado quando TODOS os anteriores vêm
// vazios. Um teste que só exercitasse o primeiro nível não pegaria uma
// inversão de ordem entre os quatro (HOUSEKEEP F149).
func TestSendList_BodyFallbackChain(t *testing.T) {
	cases := []struct {
		name string
		req  domain.SendListRequest
		want string
	}{
		{"Desc vence todos", domain.SendListRequest{Desc: "A", Body: "B", Body2: "C", Text: "D"}, "A"},
		{"Body quando Desc vazio", domain.SendListRequest{Body: "B", Body2: "C", Text: "D"}, "B"},
		{"body quando Desc e Body vazios", domain.SendListRequest{Body2: "C", Text: "D"}, "C"},
		{"text quando os tres primeiros vazios", domain.SendListRequest{Text: "D"}, "D"},
		{"espaco em branco nao conta como preenchido", domain.SendListRequest{Desc: "   ", Body: "B"}, "B"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sm := &contractsfake.SimpleMessenger{}
			logger := &contractsfake.Logger{}

			req := tc.req
			req.Phone = listPhone
			req.Sections = validListRequest().Sections

			if _, err := newSendList(sm, listJIDResolver(), logger).Execute(context.Background(), userID, req); err != nil {
				t.Fatalf("caminho feliz falhou: %v", err)
			}
			if got := sm.SendListCalls[0].Payload.Body; got != tc.want {
				t.Errorf("Body: got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSendList_RowIDFallbackChain percorre os CINCO níveis da cadeia do
// identificador de linha, nível por nível — o último não é um campo, é o
// título já resolvido e já trimado.
func TestSendList_RowIDFallbackChain(t *testing.T) {
	cases := []struct {
		name string
		row  domain.ListRow
		want string
	}{
		{"RowId vence todos", domain.ListRow{Title: "T", RowId: "a", RowID: "b", Rowid: "c", Rowid2: "d"}, "a"},
		{"RowID quando RowId vazio", domain.ListRow{Title: "T", RowID: "b", Rowid: "c", Rowid2: "d"}, "b"},
		{"rowId quando RowId e RowID vazios", domain.ListRow{Title: "T", Rowid: "c", Rowid2: "d"}, "c"},
		{"rowID quando os tres primeiros vazios", domain.ListRow{Title: "T", Rowid2: "d"}, "d"},
		{"titulo quando os quatro vazios", domain.ListRow{Title: "T"}, "T"},
		{"espaco em branco nao conta como preenchido", domain.ListRow{Title: "T", RowId: "  ", RowID: "b"}, "b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sm := &contractsfake.SimpleMessenger{}
			logger := &contractsfake.Logger{}

			req := validListRequest()
			req.Sections = []domain.ListSection{{Title: "Sec", Rows: []domain.ListRow{tc.row}}}

			if _, err := newSendList(sm, listJIDResolver(), logger).Execute(context.Background(), userID, req); err != nil {
				t.Fatalf("caminho feliz falhou: %v", err)
			}
			got := sm.SendListCalls[0].Payload.Sections[0].Rows[0]
			if got.RowId != tc.want {
				t.Errorf("RowId: got %q, want %q", got.RowId, tc.want)
			}
			if got.Title != "T" {
				t.Errorf("Title: got %q, want %q", got.Title, "T")
			}
		})
	}
}

// TestSendList_RowWithoutTitleIsSilentlyDiscarded trava o PRIMEIRO descarte
// silencioso do contrato histórico: linha sem título some, sem 400 e sem
// aviso. Uma seção com três linhas, duas sem título, devolve sucesso com
// UMA linha.
func TestSendList_RowWithoutTitleIsSilentlyDiscarded(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	logger := &contractsfake.Logger{}

	req := validListRequest()
	req.Sections = []domain.ListSection{{
		Title: "Sec",
		Rows: []domain.ListRow{
			{Title: "Sobrevive 1"},
			{Title: "   "},
			{},
			{Title: "Sobrevive 2"},
		},
	}}

	if _, err := newSendList(sm, listJIDResolver(), logger).Execute(context.Background(), userID, req); err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	rows := sm.SendListCalls[0].Payload.Sections[0].Rows
	if len(rows) != 2 {
		t.Fatalf("chegaram %d linha(s) a porta, quero 2 — a sem titulo SOME em silencio", len(rows))
	}
	if rows[0].Title != "Sobrevive 1" || rows[1].Title != "Sobrevive 2" {
		t.Errorf("sobraram %q e %q, quero \"Sobrevive 1\" e \"Sobrevive 2\"", rows[0].Title, rows[1].Title)
	}
}

// TestSendList_SectionWithoutRowsIsSilentlyDiscarded trava o SEGUNDO
// descarte silencioso: uma seção que ficou sem linha nenhuma (todas sem
// título) some, e as outras seções sobrevivem.
func TestSendList_SectionWithoutRowsIsSilentlyDiscarded(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	logger := &contractsfake.Logger{}

	req := validListRequest()
	req.Sections = []domain.ListSection{
		{Title: "Vazia", Rows: []domain.ListRow{{Title: "  "}, {}}},
		{Title: "Sobrevive", Rows: []domain.ListRow{{Title: "Item"}}},
	}

	if _, err := newSendList(sm, listJIDResolver(), logger).Execute(context.Background(), userID, req); err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	sections := sm.SendListCalls[0].Payload.Sections
	if len(sections) != 1 {
		t.Fatalf("chegaram %d secao(oes) a porta, quero 1 — a sem linha SOME em silencio", len(sections))
	}
	if sections[0].Title != "Sobrevive" {
		t.Errorf("sobrou %q, quero %q", sections[0].Title, "Sobrevive")
	}
}

// TestSendList_AllSectionsDiscardedRejects é a REDE do TERCEIRO descarte: só
// quando TODAS as seções somem é que a rota recusa — os dois descartes
// anteriores são mudos, este vira 400.
func TestSendList_AllSectionsDiscardedRejects(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	logger := &contractsfake.Logger{}

	req := validListRequest()
	req.Sections = []domain.ListSection{
		{Title: "Vazia 1", Rows: []domain.ListRow{{Title: "  "}}},
		{Title: "Vazia 2", Rows: nil},
	}

	_, err := newSendList(sm, listJIDResolver(), logger).Execute(context.Background(), userID, req)

	if err == nil {
		t.Fatal("todas as secoes descartadas produziu sucesso")
	}
	if got := err.Error(); !strings.Contains(got, "no valid sections/rows found in payload") {
		t.Errorf("causa: got %q, want conter a mensagem de rede", got)
	}
	if n := len(sm.EnsureSessionCalls); n != 0 {
		t.Errorf("rede acionada mas a sessao foi consultada %d vez(es)", n)
	}
	if n := len(sm.SendListCalls); n != 0 {
		t.Errorf("rede acionada mas SendList foi chamado %d vez(es)", n)
	}
}

// TestSendList_LegacyListWrappedInSingleSection trava o modo legado: `List`
// vira UMA seção só, com o título vindo de TopText (ou "Menu" quando
// TopText vem vazio).
func TestSendList_LegacyListWrappedInSingleSection(t *testing.T) {
	cases := []struct {
		name        string
		topText     string
		wantSection string
	}{
		{"TopText vira titulo da secao", "Cabecalho", "Cabecalho"},
		{"TopText vazio cai em Menu", "", "Menu"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sm := &contractsfake.SimpleMessenger{}
			logger := &contractsfake.Logger{}

			req := domain.SendListRequest{
				Phone:   listPhone,
				Desc:    "Escolha",
				TopText: tc.topText,
				List:    []domain.ListRow{{Title: "Item 1"}, {Title: "Item 2"}},
			}

			if _, err := newSendList(sm, listJIDResolver(), logger).Execute(context.Background(), userID, req); err != nil {
				t.Fatalf("caminho feliz falhou: %v", err)
			}
			sections := sm.SendListCalls[0].Payload.Sections
			if len(sections) != 1 {
				t.Fatalf("modo legado produziu %d secao(oes), quero 1", len(sections))
			}
			if sections[0].Title != tc.wantSection {
				t.Errorf("titulo da secao: got %q, want %q", sections[0].Title, tc.wantSection)
			}
			if len(sections[0].Rows) != 2 {
				t.Fatalf("secao legada tem %d linha(s), quero 2", len(sections[0].Rows))
			}
		})
	}
}

// TestSendList_SectionsTakesPrecedenceOverList: quando os dois vêm no
// payload, `Sections` (preferida) é usada e `List` é ignorado — a mesma
// regra do histórico (`if len(req.Sections) > 0`).
func TestSendList_SectionsTakesPrecedenceOverList(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	logger := &contractsfake.Logger{}

	req := domain.SendListRequest{
		Phone:    listPhone,
		Desc:     "Escolha",
		Sections: []domain.ListSection{{Title: "Preferida", Rows: []domain.ListRow{{Title: "Sections"}}}},
		List:     []domain.ListRow{{Title: "Legado"}},
	}

	if _, err := newSendList(sm, listJIDResolver(), logger).Execute(context.Background(), userID, req); err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	sections := sm.SendListCalls[0].Payload.Sections
	if len(sections) != 1 || sections[0].Rows[0].Title != "Sections" {
		t.Fatalf("List vazou para a porta: %+v", sections)
	}
}

// TestSendList_ButtonTextDefaultsToSelect trava o literal histórico
// "Select" como padrão, e que ButtonText explícito o sobrescreve.
func TestSendList_ButtonTextDefaultsToSelect(t *testing.T) {
	cases := map[string]string{"": "Select", "  ": "Select", "Ver opcoes": "Ver opcoes"}
	for in, want := range cases {
		t.Run("ButtonText="+in, func(t *testing.T) {
			sm := &contractsfake.SimpleMessenger{}
			logger := &contractsfake.Logger{}

			req := validListRequest()
			req.ButtonText = in

			if _, err := newSendList(sm, listJIDResolver(), logger).Execute(context.Background(), userID, req); err != nil {
				t.Fatalf("caminho feliz falhou: %v", err)
			}
			if got := sm.SendListCalls[0].Payload.ButtonText; got != want {
				t.Errorf("ButtonText: got %q, want %q", got, want)
			}
		})
	}
}

// TestSendList_SessionFailurePropagates: sem sessão, o erro da porta chega
// ao chamador por identidade e SendList nunca é tocado.
func TestSendList_SessionFailurePropagates(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{SessionGuard: contractsfake.FailSession(errSession)}
	logger := &contractsfake.Logger{}

	_, err := newSendList(sm, &contractsfake.JIDResolver{}, logger).Execute(context.Background(), userID, validListRequest())

	if !errors.Is(err, errSession) {
		t.Fatalf("erro da porta nao chegou ao chamador por identidade: got %#v", err)
	}
	if n := len(sm.SendListCalls); n != 0 {
		t.Fatalf("sessao invalida, mas SendList foi chamado %d vez(es)", n)
	}
	assertSessionLog(t, logger, "txtID", userID)
}

// TestSendList_InvalidPhoneNeverSends: JID que não resolve nunca alcança
// SendList.
func TestSendList_InvalidPhoneNeverSends(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errors.New("jid invalido") },
	}
	logger := &contractsfake.Logger{}

	_, err := newSendList(sm, jr, logger).Execute(context.Background(), userID, validListRequest())

	if err == nil {
		t.Fatal("JID invalido foi aceito")
	}
	if n := len(sm.SendListCalls); n != 0 {
		t.Fatalf("JID invalido, mas SendList foi chamado %d vez(es)", n)
	}
}

// TestSendList_CausalSuccess prova o caminho inteiro: destinatário
// resolvido, payload com Body/ButtonText/Title/Footer/Sections corretos,
// Status=StatusSent SÓ depois do envio retornar sucesso, e MessageID/
// Timestamp vindos da porta.
func TestSendList_CausalSuccess(t *testing.T) {
	sentAt := time.Unix(1755500777, 0)
	sm := &contractsfake.SimpleMessenger{
		SendListFunc: func(_ context.Context, _ string, target domain.JID, payload domain.ListPayload, _ *domain.ReplyContext, _ []string, _ string) (domain.MessageSendResult, error) {
			if target != domain.JID(listPhone+"@s.whatsapp.net") {
				t.Errorf("target: got %q, want resolvido com o servidor padrao", target)
			}
			if payload.Body != "Escolha um prato" {
				t.Errorf("Body: got %q", payload.Body)
			}
			if payload.ButtonText != "Ver cardapio" {
				t.Errorf("ButtonText: got %q", payload.ButtonText)
			}
			if payload.Title != "Cabecalho" {
				t.Errorf("Title: got %q", payload.Title)
			}
			if payload.Footer != "Rodape" {
				t.Errorf("Footer: got %q", payload.Footer)
			}
			return domain.MessageSendResult{ID: "wire-list-1", Timestamp: sentAt}, nil
		},
	}
	logger := &contractsfake.Logger{}

	req := domain.SendListRequest{
		Phone:      listPhone,
		Desc:       "Escolha um prato",
		ButtonText: "Ver cardapio",
		TopText:    "Cabecalho",
		FooterText: "Rodape",
		Sections:   []domain.ListSection{{Title: "Pratos", Rows: []domain.ListRow{{Title: "Feijoada"}}}},
	}

	result, err := newSendList(sm, listJIDResolver(), logger).Execute(context.Background(), userID, req)

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
	if result.MessageID != "wire-list-1" {
		t.Errorf("MessageID: got %q, want %q", result.MessageID, "wire-list-1")
	}
	if result.Timestamp != sentAt.Unix() {
		t.Errorf("Timestamp: got %d, want %d", result.Timestamp, sentAt.Unix())
	}
}

// TestSendList_SendFailureNeverReportsSent é o eixo do defeito original: a
// rota devolvia 200 sem enviar nada. Aqui o envio FALHA, e Status=StatusSent
// nunca pode aparecer — prova que o resultado só é montado DEPOIS do envio
// retornar sucesso (ORDEM).
func TestSendList_SendFailureNeverReportsSent(t *testing.T) {
	sendErr := errors.New("send-list-downstream-boom")
	sm := &contractsfake.SimpleMessenger{
		SendListFunc: func(context.Context, string, domain.JID, domain.ListPayload, *domain.ReplyContext, []string, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, sendErr
		},
	}
	logger := &contractsfake.Logger{}

	result, err := newSendList(sm, listJIDResolver(), logger).Execute(context.Background(), userID, validListRequest())

	if !errors.Is(err, sendErr) {
		t.Fatalf("erro do envio nao chegou ao chamador por identidade: got %#v", err)
	}
	if result != nil {
		t.Fatalf("envio falho produziu resultado nao-nil: %+v", result)
	}
}

// TestSendList_MessageIDIsTheOneActuallySent: o Id do chamador é repassado à
// porta, mas o MessageID publicado é o que a sessão REALMENTE usou.
func TestSendList_MessageIDIsTheOneActuallySent(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{
		SendListFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.ListPayload, _ *domain.ReplyContext, _ []string, id string) (domain.MessageSendResult, error) {
			if id != "id-do-cliente" {
				t.Errorf("id repassado a porta: got %q, want %q", id, "id-do-cliente")
			}
			return domain.MessageSendResult{ID: "id-que-o-sdk-usou"}, nil
		},
	}
	logger := &contractsfake.Logger{}

	req := validListRequest()
	req.ID = "id-do-cliente"

	result, err := newSendList(sm, listJIDResolver(), logger).Execute(context.Background(), userID, req)

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if result.MessageID != "id-que-o-sdk-usou" {
		t.Errorf("MessageID: got %q, want %q", result.MessageID, "id-que-o-sdk-usou")
	}
}

// --- Discard logging (F149, same form as F186 / send_buttons) ---

// TestSendList_DroppedRowIsRecorded trava o registo do PRIMEIRO descarte:
// linha sem título some E deixa rastro. O comportamento continua idêntico
// (duas linhas enviadas, não três), e agora o log diz QUAL foi e PORQUE.
func TestSendList_DroppedRowIsRecorded(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	logger := &contractsfake.Logger{}

	req := validListRequest()
	req.ID = "lista-abc"
	req.Sections = []domain.ListSection{{
		Title: "Cardapio",
		Rows: []domain.ListRow{
			{Title: "Feijoada"},
			{Title: "   "},
			{Title: "Moqueca"},
		},
	}}

	result, err := newSendList(sm, listJIDResolver(), logger).Execute(context.Background(), userID, req)
	if err != nil {
		t.Fatalf("envio recusado: %v (o descarte é parcial, o resto tem de ir)", err)
	}
	if result == nil {
		t.Fatal("sem resultado")
	}

	if n := len(sm.SendListCalls); n != 1 {
		t.Fatalf("SendList chamado %d vez(es), quero 1", n)
	}
	if got := len(sm.SendListCalls[0].Payload.Sections[0].Rows); got != 2 {
		t.Fatalf("linhas enviadas = %d, quero 2 (o registro não pode alterar o descarte)", got)
	}

	var rec *contractsfake.LogRecord
	for i, r := range logger.Records() {
		if r.Level == contractsfake.LevelWarn && strings.Contains(r.Msg, "row dropped") {
			rec = &logger.Records()[i]
			break
		}
	}
	if rec == nil {
		t.Fatalf("nenhum Warn de linha descartada; registros = %+v", logger.Records())
	}

	for _, tc := range []struct {
		key  string
		want any
	}{
		{"clientMsgID", "lista-abc"},
		{"reason", "empty_title"},
		{"sectionIndex", 0},
		{"rowIndex", 1},
	} {
		got, ok := rec.Keyval(tc.key)
		if !ok {
			t.Errorf("campo %q ausente do registro de descarte", tc.key)
			continue
		}
		if got != tc.want {
			t.Errorf("campo %q = %v, quero %v", tc.key, got, tc.want)
		}
	}
}

// TestSendList_DroppedSectionIsRecorded trava o registo do SEGUNDO descarte:
// seção que ficou sem linhas some E deixa rastro.
func TestSendList_DroppedSectionIsRecorded(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	logger := &contractsfake.Logger{}

	req := validListRequest()
	req.ID = "lista-sec"
	req.Sections = []domain.ListSection{
		{Title: "Vazia", Rows: []domain.ListRow{{Title: "  "}, {}}},
		{Title: "Sobrevive", Rows: []domain.ListRow{{Title: "Item"}}},
	}

	result, err := newSendList(sm, listJIDResolver(), logger).Execute(context.Background(), userID, req)
	if err != nil {
		t.Fatalf("envio recusado: %v", err)
	}
	if result == nil {
		t.Fatal("sem resultado")
	}

	sections := sm.SendListCalls[0].Payload.Sections
	if len(sections) != 1 {
		t.Fatalf("secoes enviadas = %d, quero 1", len(sections))
	}

	var secRec *contractsfake.LogRecord
	for i, r := range logger.Records() {
		if r.Level == contractsfake.LevelWarn && strings.Contains(r.Msg, "section dropped") {
			secRec = &logger.Records()[i]
			break
		}
	}
	if secRec == nil {
		t.Fatalf("nenhum Warn de secao descartada; registros = %+v", logger.Records())
	}

	for _, tc := range []struct {
		key  string
		want any
	}{
		{"clientMsgID", "lista-sec"},
		{"reason", "no_surviving_rows"},
		{"sectionIndex", 0},
		{"sectionTitle", "Vazia"},
	} {
		got, ok := secRec.Keyval(tc.key)
		if !ok {
			t.Errorf("campo %q ausente do registro de descarte de secao", tc.key)
			continue
		}
		if got != tc.want {
			t.Errorf("campo %q = %v, quero %v", tc.key, got, tc.want)
		}
	}
}

// TestSendList_ValidPayloadProducesNoDropRecord é o controle POSITIVO: um
// envio sem descarte nenhum não emite Warn de descarte. Sem este par, um
// Warn emitido em TODO envio passaria nas asserções acima e ninguém notaria
// até o log encher (mesma lição de ValidTypesProduceNoDropRecord em
// send_buttons_test.go).
func TestSendList_ValidPayloadProducesNoDropRecord(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	logger := &contractsfake.Logger{}

	req := validListRequest()

	if _, err := newSendList(sm, listJIDResolver(), logger).Execute(context.Background(), userID, req); err != nil {
		t.Fatalf("envio valido recusado: %v", err)
	}

	for _, r := range logger.Records() {
		if strings.Contains(r.Msg, "dropped") {
			t.Fatalf("aviso de descarte emitido sem descarte: %+v", r)
		}
	}
}
