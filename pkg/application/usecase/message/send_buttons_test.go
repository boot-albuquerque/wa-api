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

// Este arquivo recebe os eixos que send_message_test.go cobria para
// SendButtons enquanto ele era um use case de port.MessageComposer que só
// validava e devolvia "validated" sem enviar nada (CAP-21 migrou-o para
// port.InteractiveMessenger, com envio de verdade). Mesma estrutura de
// send_template_test.go.
//
// O que este bloco tem de próprio está na HOUSEKEEP F147/F148: o DTO tinha
// ficado com {Phone, Body, Id} e perdido TUDO que dá sentido à capability, e
// a normalização dos botões — a cadeia de fallbacks, o truncamento do título
// e o DESCARTE SILENCIOSO do tipo desconhecido — é regra de contrato
// público, não detalhe de wire. Qual `Name` e quais parâmetros cada tipo
// vira está medido no adapter
// (pkg/infra/wa-noise/adapters/chat/messenger_buttons_test.go).

const buttonsPhone = "5511987654321"

// buttonsInput são os botões de referência desta suite: um de cada um dos
// QUATRO tipos, na ordem, para que a ordem preservada seja observável.
func buttonsInput() []domain.InteractiveButton {
	return []domain.InteractiveButton{
		{Title: "Sim", ID: "btn-sim", Type: domain.ButtonTypeReply},
		{Title: "Site", URL: "https://example.invalid/a", Type: domain.ButtonTypeCTAURL},
		{Title: "Ligar", PhoneNumber: "+5511987654321", Type: domain.ButtonTypeCTACall},
		{Title: "Copiar", CopyCode: "PROMO10", Type: domain.ButtonTypeCopy},
	}
}

// buttonsJIDResolver é o dublê de port.JIDResolver que imita a regra REAL de
// pkg/infra/wa-noise/mapping/jid/parse.go:12 — número sem "@" recebe o
// servidor padrão; com "@", passa intacto. O dublê padrão de contractsfake
// devolve `raw` sem tocar, o que é MAIS PERMISSIVO que a produção e
// esconderia exatamente o defeito de destinatário que este teste existe para
// pegar (ARMADILHA 1).
func buttonsJIDResolver() *contractsfake.JIDResolver {
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

func validButtonsRequest() domain.SendButtonsRequest {
	return domain.SendButtonsRequest{
		Phone:   buttonsPhone,
		Body:    "Escolha",
		Buttons: buttonsInput(),
	}
}

func newSendButtons(im *contractsfake.InteractiveMessenger, jr *contractsfake.JIDResolver, mf *contractsfake.MediaFetcher, l *contractsfake.Logger) *message.SendButtonsUseCase {
	return message.NewSendButtonsUseCase(im, jr, mf, l)
}

// TestSendButtons_MissingRequiredField: Phone, Body ou Buttons ausente é
// recusado antes de qualquer porta ser tocada. A regra e a MENSAGEM são as
// HISTÓRICAS: UMA recusa cobrindo os três ("missing Phone, Body or Buttons",
// `git show 41bc8e2^:handlers.go`, na função SendButtons), e não uma por
// campo — é assim que a rota sempre respondeu.
func TestSendButtons_MissingRequiredField(t *testing.T) {
	cases := []struct {
		name string
		req  domain.SendButtonsRequest
	}{
		{"Phone", domain.SendButtonsRequest{Body: "Escolha", Buttons: buttonsInput()}},
		{"Body", domain.SendButtonsRequest{Phone: buttonsPhone, Buttons: buttonsInput()}},
		{"Buttons_ausente", domain.SendButtonsRequest{Phone: buttonsPhone, Body: "Escolha"}},
		{"Buttons_vazio", domain.SendButtonsRequest{Phone: buttonsPhone, Body: "Escolha", Buttons: []domain.InteractiveButton{}}},
		// Body só de espaço em branco é o mesmo que ausente: o histórico
		// aplicava TrimSpace ANTES de comparar com "".
		{"Body_so_espaco", domain.SendButtonsRequest{Phone: buttonsPhone, Body: "   ", Buttons: buttonsInput()}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			im := &contractsfake.InteractiveMessenger{}
			logger := &contractsfake.Logger{}

			_, err := newSendButtons(im, &contractsfake.JIDResolver{}, &contractsfake.MediaFetcher{}, logger).
				Execute(context.Background(), userID, tc.req)

			if err == nil {
				t.Fatalf("request invalido (%s) foi aceito", tc.name)
			}
			if got := err.Error(); !strings.Contains(got, "missing Phone, Body or Buttons") {
				t.Errorf("causa: got %q, want a mensagem historica unica", got)
			}
			if n := len(im.EnsureSessionCalls); n != 0 {
				t.Errorf("validacao falhou mas a sessao foi consultada %d vez(es)", n)
			}
			if n := len(im.SendButtonsCalls); n != 0 {
				t.Errorf("validacao falhou mas SendButtons foi chamado %d vez(es)", n)
			}
		})
	}
}

// TestSendButtons_BodyFallsBackToText trava o fallback `Body <- text` do
// histórico. Sem ele, um cliente que sempre mandou `text` passaria a receber
// 400 numa requisição que a rota sempre aceitou.
func TestSendButtons_BodyFallsBackToText(t *testing.T) {
	im := &contractsfake.InteractiveMessenger{}
	logger := &contractsfake.Logger{}

	req := validButtonsRequest()
	req.Body = ""
	req.Text = "  Escolha pelo text  "

	_, err := newSendButtons(im, buttonsJIDResolver(), &contractsfake.MediaFetcher{}, logger).
		Execute(context.Background(), userID, req)

	if err != nil {
		t.Fatalf("Body vazio com text preenchido foi recusado: %v", err)
	}
	if n := len(im.SendButtonsCalls); n != 1 {
		t.Fatalf("SendButtons chamado %d vez(es), quero 1", n)
	}
	// TrimSpace também no fallback, como no histórico.
	if got := im.SendButtonsCalls[0].Payload.Body; got != "Escolha pelo text" {
		t.Errorf("Body: got %q, want o text com TrimSpace", got)
	}
}

// TestSendButtons_AllButtonsDiscardedIsRejected trava a segunda recusa do
// histórico: se a normalização descartar TODOS os botões, a resposta é 400
// "no valid buttons parsed" e nada é enviado. É a rede PARCIAL que existe
// sob o descarte silencioso.
func TestSendButtons_AllButtonsDiscardedIsRejected(t *testing.T) {
	cases := map[string][]domain.InteractiveButton{
		"tipo desconhecido": {{Title: "Sim", Type: "tipo-que-nao-existe"}},
		"titulo vazio":      {{Type: domain.ButtonTypeReply}},
		"titulo so espaco":  {{Title: "   ", Type: domain.ButtonTypeReply}},
	}
	for name, buttons := range cases {
		t.Run(name, func(t *testing.T) {
			im := &contractsfake.InteractiveMessenger{}
			logger := &contractsfake.Logger{}

			req := validButtonsRequest()
			req.Buttons = buttons

			_, err := newSendButtons(im, &contractsfake.JIDResolver{}, &contractsfake.MediaFetcher{}, logger).
				Execute(context.Background(), userID, req)

			if err == nil {
				t.Fatal("todos os botoes descartados, mas o request foi aceito")
			}
			if got := err.Error(); !strings.Contains(got, "no valid buttons parsed") {
				t.Errorf("causa: got %q, want %q", got, "no valid buttons parsed")
			}
			if n := len(im.EnsureSessionCalls); n != 0 {
				t.Errorf("recusa por botoes, mas a sessao foi consultada %d vez(es)", n)
			}
			if n := len(im.SendButtonsCalls); n != 0 {
				t.Errorf("recusa por botoes, mas SendButtons foi chamado %d vez(es)", n)
			}
		})
	}
}

// TestSendButtons_UnknownTypeIsSilentlyDiscarded é a TRAVA do comportamento
// preservado por decisão explícita (HOUSEKEEP F148, mesma disciplina da F121
// e da F135).
//
// O cliente manda TRÊS botões, um deles com erro de digitação no `type`;
// recebe 200, e a mensagem sai com DOIS. Nada avisa. Isso diverge do
// send_template, onde o tipo desconhecido cai em quickreply, e é o histórico
// (`default: continue`). Quem quiser "consertar" — recusar o payload, ou
// tratar como reply — está mudando contrato público e tem de decidir isso,
// não descobrir depois: este teste falha nos dois casos.
func TestSendButtons_UnknownTypeIsSilentlyDiscarded(t *testing.T) {
	im := &contractsfake.InteractiveMessenger{}
	logger := &contractsfake.Logger{}

	req := validButtonsRequest()
	req.Buttons = []domain.InteractiveButton{
		{Title: "Sim", Type: domain.ButtonTypeReply},
		{Title: "Erro de digitacao", Type: "cta_urll"},
		{Title: "Nao", Type: domain.ButtonTypeReply},
	}

	result, err := newSendButtons(im, buttonsJIDResolver(), &contractsfake.MediaFetcher{}, logger).
		Execute(context.Background(), userID, req)

	if err != nil {
		t.Fatalf("tipo desconhecido produziu recusa: %v (o historico o DESCARTA e envia o resto)", err)
	}
	if result == nil {
		t.Fatal("tipo desconhecido nao produziu resultado")
	}
	if n := len(im.SendButtonsCalls); n != 1 {
		t.Fatalf("SendButtons chamado %d vez(es), quero 1", n)
	}

	got := im.SendButtonsCalls[0].Payload.Buttons
	if len(got) != 2 {
		t.Fatalf("chegaram %d botao(oes) a porta, quero 2: o de tipo desconhecido SOME", len(got))
	}
	if got[0].Title != "Sim" || got[1].Title != "Nao" {
		t.Errorf("os botoes que sobraram sao %q e %q, quero \"Sim\" e \"Nao\" — o descarte tem de ser o do MEIO",
			got[0].Title, got[1].Title)
	}
	for _, b := range got {
		if b.Type != domain.ButtonTypeReply {
			t.Errorf("botao %q chegou com tipo %q; o desconhecido NAO pode ter virado reply", b.Title, b.Type)
		}
	}
}

// TestSendButtons_TitleFallbackChain trava a ORDEM exata da cadeia de
// fallback do título: Title <- Text <- ButtonText. Uma etapa que deixe de
// existir troca o rótulo do botão no aparelho de quem recebe, e nenhuma
// outra asserção desta suite notaria.
func TestSendButtons_TitleFallbackChain(t *testing.T) {
	cases := []struct {
		name  string
		in    domain.InteractiveButton
		want  string
		wantI string
	}{
		{"Title vence todos",
			domain.InteractiveButton{Title: "A", Text: "B", ButtonText: "C", Type: domain.ButtonTypeReply}, "A", "A"},
		{"Text quando Title vazio",
			domain.InteractiveButton{Text: "B", ButtonText: "C", Type: domain.ButtonTypeReply}, "B", "B"},
		{"ButtonText quando os dois vazios",
			domain.InteractiveButton{ButtonText: "C", Type: domain.ButtonTypeReply}, "C", "C"},
		{"espaco em branco nao conta como preenchido",
			domain.InteractiveButton{Title: "   ", Text: "B", Type: domain.ButtonTypeReply}, "B", "B"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			im := &contractsfake.InteractiveMessenger{}
			logger := &contractsfake.Logger{}

			req := validButtonsRequest()
			req.Buttons = []domain.InteractiveButton{tc.in}

			if _, err := newSendButtons(im, buttonsJIDResolver(), &contractsfake.MediaFetcher{}, logger).
				Execute(context.Background(), userID, req); err != nil {
				t.Fatalf("caminho feliz falhou: %v", err)
			}
			got := im.SendButtonsCalls[0].Payload.Buttons[0]
			if got.Title != tc.want {
				t.Errorf("Title: got %q, want %q", got.Title, tc.want)
			}
			if got.ID != tc.wantI {
				t.Errorf("ID: got %q, want %q (o identificador cai no titulo resolvido)", got.ID, tc.wantI)
			}
		})
	}
}

// TestSendButtons_IDFallbackChain trava a segunda cadeia: ID <- ButtonId <-
// título JÁ TRUNCADO. O último elo importa mais do que parece: é o valor que
// volta no clique de quem recebeu a mensagem.
func TestSendButtons_IDFallbackChain(t *testing.T) {
	cases := []struct {
		name string
		in   domain.InteractiveButton
		want string
	}{
		{"ID vence", domain.InteractiveButton{Title: "T", ID: "a", ButtonID: "b", Type: domain.ButtonTypeReply}, "a"},
		{"ButtonId quando ID vazio", domain.InteractiveButton{Title: "T", ButtonID: "b", Type: domain.ButtonTypeReply}, "b"},
		{"titulo quando os dois vazios", domain.InteractiveButton{Title: "T", Type: domain.ButtonTypeReply}, "T"},
		// 24 runas: o título é truncado em 20 ANTES de virar ID, então o ID
		// tem de ser o truncado. Truncar depois (ou não truncar) daria
		// "aaaaaaaaaaaaaaaaaaaaaaaa" aqui.
		{"titulo TRUNCADO quando os dois vazios",
			domain.InteractiveButton{Title: strings.Repeat("a", 24), Type: domain.ButtonTypeReply},
			strings.Repeat("a", 20)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			im := &contractsfake.InteractiveMessenger{}
			logger := &contractsfake.Logger{}

			req := validButtonsRequest()
			req.Buttons = []domain.InteractiveButton{tc.in}

			if _, err := newSendButtons(im, buttonsJIDResolver(), &contractsfake.MediaFetcher{}, logger).
				Execute(context.Background(), userID, req); err != nil {
				t.Fatalf("caminho feliz falhou: %v", err)
			}
			if got := im.SendButtonsCalls[0].Payload.Buttons[0].ID; got != tc.want {
				t.Errorf("ID: got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSendButtons_TitleTruncatesByRuneNotByte: o corte é em RUNAS. Com
// bytes, um título de 20 acentos (40 bytes) sairia partido no meio de um
// caractere — e o teste com título ASCII passaria mesmo assim.
func TestSendButtons_TitleTruncatesByRuneNotByte(t *testing.T) {
	im := &contractsfake.InteractiveMessenger{}
	logger := &contractsfake.Logger{}

	req := validButtonsRequest()
	req.Buttons = []domain.InteractiveButton{{Title: strings.Repeat("ç", 25), Type: domain.ButtonTypeReply}}

	if _, err := newSendButtons(im, buttonsJIDResolver(), &contractsfake.MediaFetcher{}, logger).
		Execute(context.Background(), userID, req); err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	got := im.SendButtonsCalls[0].Payload.Buttons[0].Title
	if n := len([]rune(got)); n != 20 {
		t.Errorf("Title tem %d runa(s), quero 20 — o corte e' por RUNA", n)
	}
	if got != strings.Repeat("ç", 20) {
		t.Errorf("Title: got %q; corte por byte partiria o caractere", got)
	}
}

// TestSendButtons_TypeIsNormalized trava o ToLower(TrimSpace(...)) do
// histórico: " CTA_URL " é o mesmo que "cta_url", e Type vazio é "reply".
func TestSendButtons_TypeIsNormalized(t *testing.T) {
	cases := map[string]string{
		" CTA_URL ": domain.ButtonTypeCTAURL,
		"Cta_Call":  domain.ButtonTypeCTACall,
		"COPY":      domain.ButtonTypeCopy,
		"":          domain.ButtonTypeReply,
		"  ":        domain.ButtonTypeReply,
	}
	for in, want := range cases {
		t.Run("type="+in, func(t *testing.T) {
			im := &contractsfake.InteractiveMessenger{}
			logger := &contractsfake.Logger{}

			req := validButtonsRequest()
			req.Buttons = []domain.InteractiveButton{{Title: "T", Type: in}}

			if _, err := newSendButtons(im, buttonsJIDResolver(), &contractsfake.MediaFetcher{}, logger).
				Execute(context.Background(), userID, req); err != nil {
				t.Fatalf("type %q foi recusado: %v", in, err)
			}
			if got := im.SendButtonsCalls[0].Payload.Buttons[0].Type; got != want {
				t.Errorf("Type: got %q, want %q", got, want)
			}
		})
	}
}

// TestSendButtons_SessionFailurePropagates: sem sessão, o erro da porta
// chega ao chamador por identidade e SendButtons nunca é tocado.
func TestSendButtons_SessionFailurePropagates(t *testing.T) {
	im := &contractsfake.InteractiveMessenger{SessionGuard: contractsfake.FailSession(errSession)}
	logger := &contractsfake.Logger{}

	_, err := newSendButtons(im, &contractsfake.JIDResolver{}, &contractsfake.MediaFetcher{}, logger).
		Execute(context.Background(), userID, validButtonsRequest())

	if !errors.Is(err, errSession) {
		t.Fatalf("erro da porta nao chegou ao chamador por identidade: got %#v", err)
	}
	if n := len(im.SendButtonsCalls); n != 0 {
		t.Fatalf("sessao invalida, mas SendButtons foi chamado %d vez(es)", n)
	}
}

// TestSendButtons_InvalidPhoneNeverReachesSendButtons: JID que não parseia é
// recusa, e a recusa acontece ANTES do envio.
func TestSendButtons_InvalidPhoneNeverReachesSendButtons(t *testing.T) {
	im := &contractsfake.InteractiveMessenger{}
	logger := &contractsfake.Logger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errJID },
	}

	req := validButtonsRequest()
	req.Phone = "lixo"

	_, err := newSendButtons(im, jr, &contractsfake.MediaFetcher{}, logger).
		Execute(context.Background(), userID, req)

	if err == nil {
		t.Fatal("telefone invalido foi aceito")
	}
	if n := len(im.SendButtonsCalls); n != 0 {
		t.Fatalf("JID invalido, mas SendButtons foi chamado %d vez(es)", n)
	}
}

// TestSendButtons_PhoneResolvedWithDefaultServerRule trava a REGRA de
// resolução: o histórico passava Phone por validateMessageFields ->
// parseJID, que é o ResolveJID de hoje — não o ResolveQualifiedJID, mais
// estrito. Trocar um pelo outro passaria a recusar entradas que a rota
// aceita, e nenhum outro teste deste arquivo notaria: o dublê responde aos
// dois.
func TestSendButtons_PhoneResolvedWithDefaultServerRule(t *testing.T) {
	im := &contractsfake.InteractiveMessenger{}
	logger := &contractsfake.Logger{}
	jr := &contractsfake.JIDResolver{}

	if _, err := newSendButtons(im, jr, &contractsfake.MediaFetcher{}, logger).
		Execute(context.Background(), userID, validButtonsRequest()); err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(jr.ResolveJIDCalls); n != 1 {
		t.Fatalf("ResolveJID chamado %d vez(es), quero 1", n)
	}
	if got := jr.ResolveJIDCalls[0].Raw; got != buttonsPhone {
		t.Errorf("ResolveJID recebeu %q, quero o Phone cru", got)
	}
	if n := len(jr.ResolveQualifiedJIDCalls); n != 0 {
		t.Errorf("ResolveQualifiedJID chamado %d vez(es); a regra historica e' ResolveJID", n)
	}
}

// TestSendButtons_CausalSuccess é o teste da causa: Body/Title/Footer viram
// ButtonsPayload, os QUATRO botões chegam normalizados e na ORDEM, e Status
// só vale StatusSent depois que SendButtons devolve sucesso.
func TestSendButtons_CausalSuccess(t *testing.T) {
	sentAt := time.Date(2026, 8, 19, 10, 30, 0, 0, time.UTC)
	im := &contractsfake.InteractiveMessenger{
		SendButtonsFunc: func(context.Context, string, domain.JID, domain.ButtonsPayload, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{Timestamp: sentAt, ID: "wire-id-buttons-123"}, nil
		},
	}
	logger := &contractsfake.Logger{}

	req := validButtonsRequest()
	req.Title = "Cabecalho"
	req.Footer = "Equipe wa-api"

	result, err := newSendButtons(im, buttonsJIDResolver(), &contractsfake.MediaFetcher{}, logger).
		Execute(context.Background(), userID, req)

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(im.SendButtonsCalls); n != 1 {
		t.Fatalf("SendButtons chamado %d vez(es), quero exatamente 1", n)
	}
	call := im.SendButtonsCalls[0]
	if call.Target != domain.JID(buttonsPhone+"@s.whatsapp.net") {
		t.Errorf("destinatario: got %q, want %q", call.Target, buttonsPhone+"@s.whatsapp.net")
	}
	if call.Payload.Body != "Escolha" {
		t.Errorf("Body: got %q, want %q", call.Payload.Body, "Escolha")
	}
	if call.Payload.Title != "Cabecalho" {
		t.Errorf("Title: got %q, want %q", call.Payload.Title, "Cabecalho")
	}
	if call.Payload.Footer != "Equipe wa-api" {
		t.Errorf("Footer: got %q, want %q", call.Payload.Footer, "Equipe wa-api")
	}
	if len(call.Payload.HeaderImage) != 0 {
		t.Errorf("sem Image no request, HeaderImage tem de ser vazio; got %d byte(s)", len(call.Payload.HeaderImage))
	}

	want := []domain.InteractiveButton{
		{Title: "Sim", ID: "btn-sim", Type: domain.ButtonTypeReply},
		{Title: "Site", ID: "Site", URL: "https://example.invalid/a", Type: domain.ButtonTypeCTAURL},
		{Title: "Ligar", ID: "Ligar", PhoneNumber: "+5511987654321", Type: domain.ButtonTypeCTACall},
		{Title: "Copiar", ID: "Copiar", CopyCode: "PROMO10", Type: domain.ButtonTypeCopy},
	}
	if len(call.Payload.Buttons) != len(want) {
		t.Fatalf("Buttons: got %d botao(oes), want %d", len(call.Payload.Buttons), len(want))
	}
	for i := range want {
		if call.Payload.Buttons[i] != want[i] {
			t.Errorf("Buttons[%d]: got %+v, want %+v (a ORDEM e' a que aparece no aparelho)",
				i, call.Payload.Buttons[i], want[i])
		}
	}

	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
	if result.MessageID != "wire-id-buttons-123" {
		t.Errorf("MessageID: got %q, want o ID devolvido pela porta", result.MessageID)
	}
	if result.Timestamp != sentAt.Unix() {
		t.Errorf("Timestamp: got %v, want %v", result.Timestamp, sentAt.Unix())
	}
}

// TestSendButtons_SendFailureNeverReportsSent é o teste da ORDEM: quando o
// envio falha, o use case não pode ter produzido um resultado de sucesso. É
// o eixo que o defeito original violava — devolvia "validated" sem nunca
// enviar.
func TestSendButtons_SendFailureNeverReportsSent(t *testing.T) {
	sendErr := errors.New("porta: envio de botoes recusado pelo servidor")
	im := &contractsfake.InteractiveMessenger{
		SendButtonsFunc: func(context.Context, string, domain.JID, domain.ButtonsPayload, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, sendErr
		},
	}
	logger := &contractsfake.Logger{}

	result, err := newSendButtons(im, &contractsfake.JIDResolver{}, &contractsfake.MediaFetcher{}, logger).
		Execute(context.Background(), userID, validButtonsRequest())

	if !errors.Is(err, sendErr) {
		t.Fatalf("erro do envio nao chegou ao chamador: got %#v", err)
	}
	if result != nil {
		t.Fatalf("envio falhou mas o use case devolveu resultado: %+v", result)
	}
}

// TestSendButtons_MessageIDIsTheOneActuallySent: o MessageID publicado é o
// que a porta devolveu, mesmo quando diverge do id de entrada — e o id de
// entrada é repassado à porta para que o SDK possa usá-lo.
func TestSendButtons_MessageIDIsTheOneActuallySent(t *testing.T) {
	im := &contractsfake.InteractiveMessenger{
		SendButtonsFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.ButtonsPayload, id string) (domain.MessageSendResult, error) {
			if id != callerID {
				t.Errorf("id repassado a porta: got %q, want %q", id, callerID)
			}
			return domain.MessageSendResult{ID: "id-que-o-sdk-usou"}, nil
		},
	}
	logger := &contractsfake.Logger{}

	req := validButtonsRequest()
	req.ID = callerID

	result, err := newSendButtons(im, buttonsJIDResolver(), &contractsfake.MediaFetcher{}, logger).
		Execute(context.Background(), userID, req)

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if result.MessageID != "id-que-o-sdk-usou" {
		t.Errorf("MessageID: got %q, want o que a porta devolveu", result.MessageID)
	}
}

// TestSendButtons_HeaderImageFromDataURI: o header por data URI é decodado
// LOCALMENTE — sem tocar MediaFetcher —, e os bytes decodificados chegam à
// porta com o MIME resolvido por sniffing (o histórico usava
// http.DetectContentType sobre os bytes, nunca o rótulo da data URI).
func TestSendButtons_HeaderImageFromDataURI(t *testing.T) {
	im := &contractsfake.InteractiveMessenger{}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	req := validButtonsRequest()
	req.Image = "data:image/png;base64," + onePixelPNGBase64

	if _, err := newSendButtons(im, buttonsJIDResolver(), mf, logger).
		Execute(context.Background(), userID, req); err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Errorf("data URI tocou a rede %d vez(es); o decode e' LOCAL", n)
	}
	payload := im.SendButtonsCalls[0].Payload
	if len(payload.HeaderImage) == 0 {
		t.Fatal("HeaderImage vazio; a data URI nao chegou a porta")
	}
	if payload.HeaderImageMimeType != "image/png" {
		t.Errorf("HeaderImageMimeType: got %q, want image/png (sniffing dos BYTES)", payload.HeaderImageMimeType)
	}
}

// TestSendButtons_HeaderImageFromURL: o header por URL http(s) passa pelo
// MediaFetcher (infra SSRF-safe), com o teto histórico.
func TestSendButtons_HeaderImageFromURL(t *testing.T) {
	im := &contractsfake.InteractiveMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return []byte("bytes-do-header"), "image/png", nil
		},
	}
	logger := &contractsfake.Logger{}

	req := validButtonsRequest()
	req.Image = "https://example.invalid/header.png"

	if _, err := newSendButtons(im, buttonsJIDResolver(), mf, logger).
		Execute(context.Background(), userID, req); err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(mf.FetchBytesCalls); n != 1 {
		t.Fatalf("FetchBytes chamado %d vez(es), quero 1", n)
	}
	if got := mf.FetchBytesCalls[0].Limit; got != 10*1024*1024 {
		t.Errorf("limite: got %d, want 10MB (o openGraphImageMaxBytes historico)", got)
	}
	if got := string(im.SendButtonsCalls[0].Payload.HeaderImage); got != "bytes-do-header" {
		t.Errorf("HeaderImage: got %q, want os bytes buscados", got)
	}
}

// TestSendButtons_HeaderImageFailureIsSilentlyDropped é a TRAVA do segundo
// comportamento preservado por decisão (HOUSEKEEP F148): qualquer falha em
// obter a imagem do header NÃO vira erro para o cliente — a mensagem sai sem
// header, como no histórico. Transformar isso em 400 rejeitaria requisições
// que a rota sempre aceitou.
func TestSendButtons_HeaderImageFailureIsSilentlyDropped(t *testing.T) {
	cases := map[string]struct {
		image   string
		fetcher *contractsfake.MediaFetcher
	}{
		"fetch falha": {"https://example.invalid/x.png", &contractsfake.MediaFetcher{
			FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
				return nil, "", errors.New("rede caiu")
			},
		}},
		"data uri quebrada":    {"data:image/png;base64,%%%nao-e-base64%%%", &contractsfake.MediaFetcher{}},
		"nem data uri nem url": {"/caminho/local.png", &contractsfake.MediaFetcher{}},
		"corpo vazio": {"https://example.invalid/x.png", &contractsfake.MediaFetcher{
			FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
				return nil, "image/png", nil
			},
		}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			im := &contractsfake.InteractiveMessenger{}
			logger := &contractsfake.Logger{}

			req := validButtonsRequest()
			req.Image = tc.image

			result, err := newSendButtons(im, buttonsJIDResolver(), tc.fetcher, logger).
				Execute(context.Background(), userID, req)

			if err != nil {
				t.Fatalf("falha ao obter o header virou erro para o cliente: %v", err)
			}
			if result.Status != domain.StatusSent {
				t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
			}
			if n := len(im.SendButtonsCalls); n != 1 {
				t.Fatalf("SendButtons chamado %d vez(es), quero 1", n)
			}
			if n := len(im.SendButtonsCalls[0].Payload.HeaderImage); n != 0 {
				t.Errorf("HeaderImage tem %d byte(s); o descarte tem de deixar o header VAZIO", n)
			}
		})
	}
}

// onePixelPNGBase64 é um PNG 1x1 real, e não bytes arbitrários: o MIME do
// header vem de http.DetectContentType sobre os bytes DECODIFICADOS, então
// um payload que não seja PNG de verdade não produziria "image/png" e a
// asserção mediria outra coisa.
const onePixelPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
