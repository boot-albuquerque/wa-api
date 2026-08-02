package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
)

// Este arquivo cobre os QUATRO handlers de mensagem de forma exaustiva:
// send/text, send/edit, delete/message e send/template.
//
// Os quatro tem a MESMA forma — mesma guarda de autenticacao, mesma guarda de
// sessao, mesmo decode, mesma traducao de erro do use case — e e' por isso que
// valem uma unica tabela: qualquer divergencia entre eles aparece como uma
// linha vermelha, e nao como um teste que ninguem escreveu.
//
// Cada caso de saida >=400 passa pelo co-gate D (logassert.OutcomeLogged):
// o caminho tem de logar, com causa, com req_id, em warn ou error, e sem
// nenhum segredo da F9.4 na saida inteira.

// msgSentinelErr e' o erro que as portas devolvem nos casos de falha. O texto
// e' um token proprio do teste, nao uma mensagem de producao: assertar que ELE
// chega ao log prova que a CAUSA foi propagada, sem travar o teste no texto de
// nenhuma mensagem (Principio 5).
const msgSentinelToken = "msg-sentinel-cause-1a2b3c"

var msgSentinelErr = errors.New(msgSentinelToken)

// msgHandlerCase descreve um dos quatro handlers de mensagem.
type msgHandlerCase struct {
	name string
	path string
	// build liga o handler ao fake. MessageComposer embute SessionGuard, entao
	// o mesmo fake serve aos quatro use cases (dois consomem MessageComposer,
	// dois consomem apenas SessionGuard).
	build func(*contractsfake.MessageComposer) http.Handler
	// validBody e' o menor payload que o use case aceita.
	validBody string
	// wantMessageID e' o message_id do envelope de sucesso.
	wantMessageID string
	// generatesID indica se o use case chama NewMessageID quando Id vem vazio.
	// So' send/text e send/template o fazem — e' o unico eixo em que os quatro
	// handlers divergem.
	generatesID bool
	// missingField mapeia nome do campo obrigatorio ausente -> payload que o
	// omite. E' a matriz de rejeicao do use case vista da fronteira HTTP.
	missingField map[string]string
}

func msgHandlerCases() []msgHandlerCase {
	log := silentLogger{}
	return []msgHandlerCase{
		{
			name: "SendMessage",
			path: "/chat/send/text",
			build: func(f *contractsfake.MessageComposer) http.Handler {
				return NewSendMessageHandler(message.NewSendMessageUseCase(f, log))
			},
			validBody:     `{"Phone":"5511999999999","Body":"ola"}`,
			wantMessageID: contractsfake.DefaultMessageID,
			generatesID:   true,
			missingField: map[string]string{
				"Phone": `{"Body":"ola"}`,
				"Body":  `{"Phone":"5511999999999"}`,
			},
		},
		{
			name: "SendEditMessage",
			path: "/chat/send/edit",
			build: func(f *contractsfake.MessageComposer) http.Handler {
				return NewSendEditMessageHandler(message.NewSendEditMessageUseCase(f, log))
			},
			validBody:     `{"Phone":"5511999999999","Body":"corrigido","Id":"msg-1"}`,
			wantMessageID: "msg-1",
			missingField: map[string]string{
				"Phone": `{"Body":"corrigido","Id":"msg-1"}`,
				"Body":  `{"Phone":"5511999999999","Id":"msg-1"}`,
				"Id":    `{"Phone":"5511999999999","Body":"corrigido"}`,
			},
		},
		{
			name: "DeleteMessage",
			path: "/chat/delete/message",
			build: func(f *contractsfake.MessageComposer) http.Handler {
				return NewDeleteMessageHandler(message.NewDeleteMessageUseCase(f, log))
			},
			validBody:     `{"Phone":"5511999999999","Id":"msg-1"}`,
			wantMessageID: "msg-1",
			missingField: map[string]string{
				"Phone": `{"Id":"msg-1"}`,
				"Id":    `{"Phone":"5511999999999"}`,
			},
		},
		{
			name: "SendTemplate",
			path: "/chat/send/template",
			build: func(f *contractsfake.MessageComposer) http.Handler {
				return NewSendTemplateHandler(message.NewSendTemplateUseCase(f, log))
			},
			validBody:     `{"Phone":"5511999999999","Content":"corpo","Footer":"rodape"}`,
			wantMessageID: contractsfake.DefaultMessageID,
			generatesID:   true,
			missingField: map[string]string{
				"Phone":   `{"Content":"corpo","Footer":"rodape"}`,
				"Content": `{"Phone":"5511999999999","Footer":"rodape"}`,
				"Footer":  `{"Phone":"5511999999999","Content":"corpo"}`,
			},
		},
	}
}

// msgServe roda o handler com a cadeia hlog de producao instalada e devolve a
// resposta junto com a captura de log. E' o unico ponto do arquivo que monta
// requisicao — nenhum caso reimplementa isso.
func msgServe(t *testing.T, tc msgHandlerCase, fake *contractsfake.MessageComposer, body string, mut func(*http.Request) *http.Request) (*httptest.ResponseRecorder, []logLine) {
	t.Helper()

	h, capture := logassert.Wrap(tc.build(fake))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(body))
	h.ServeHTTP(rec, mut(req))

	return rec, capture.Records(t)
}

// msgAuthed e' a mutacao padrao: requisicao autenticada com sessao valida.
func msgAuthed(r *http.Request) *http.Request { return withUser(r, "user-1") }

// TestMessageHandlers_Success: o caminho feliz dos quatro. E' o unico teste do
// arquivo que exige AUSENCIA de log — o caminho de sucesso ja e' registrado
// pelo middleware de fronteira, e um log aqui seria o Cenario 2 do plano
// (ruido redundante inflando a metrica).
func TestMessageHandlers_Success(t *testing.T) {
	for _, tc := range msgHandlerCases() {
		t.Run(tc.name, func(t *testing.T) {
			fake := &contractsfake.MessageComposer{}

			rec, recs := msgServe(t, tc, fake, tc.validBody, msgAuthed)

			if rec.Code != http.StatusOK {
				t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
			}
			env := decodeEnvelope(t, rec)
			if !env.Success {
				t.Fatalf("envelope.success=false num 200: %s", rec.Body.String())
			}
			if len(env.Error) != 0 {
				t.Fatalf("resposta de sucesso carregou error: %s", env.Error)
			}

			var data struct {
				MessageID string `json:"message_id"`
				Status    string `json:"status"`
			}
			if err := json.Unmarshal(env.Data, &data); err != nil {
				t.Fatalf("envelope.data nao e' o resultado do use case: %v (%s)", err, env.Data)
			}
			if data.MessageID != tc.wantMessageID {
				t.Errorf("message_id: got %q, want %q", data.MessageID, tc.wantMessageID)
			}

			// A sessao foi checada exatamente uma vez, com o txtID do contexto.
			if n := len(fake.EnsureSessionCalls); n != 1 {
				t.Fatalf("EnsureSession chamado %d vez(es), quero 1", n)
			}
			if got := fake.EnsureSessionCalls[0].TxtID; got != "user-1" {
				t.Errorf("EnsureSession recebeu txtID %q, quero %q", got, "user-1")
			}

			// So' os dois use cases que geram ID tocam NewMessageID.
			wantIDCalls := 0
			if tc.generatesID {
				wantIDCalls = 1
			}
			if n := len(fake.NewMessageIDCalls); n != wantIDCalls {
				t.Errorf("NewMessageID chamado %d vez(es), quero %d", n, wantIDCalls)
			}

			for _, r := range recs {
				if lvl := r.str("level"); lvl == "warn" || lvl == "error" {
					t.Fatalf("caminho de sucesso emitiu registro %s: %s", lvl, r.Raw)
				}
			}
		})
	}
}

// TestMessageHandlers_RejectUnauthenticated: sem (ou com o tipo errado de)
// userInfo no contexto e' 401, a porta nao e' tocada, e o co-gate D exige que
// a recusa tenha sido registrada com a causa.
func TestMessageHandlers_RejectUnauthenticated(t *testing.T) {
	mutations := []struct {
		name string
		mut  func(*http.Request) *http.Request
	}{
		{"contexto sem userinfo", func(r *http.Request) *http.Request { return r }},
		{"contexto com tipo errado", func(r *http.Request) *http.Request {
			return r.WithContext(context.WithValue(r.Context(), appport.UserInfoKey, 42))
		}},
	}

	for _, tc := range msgHandlerCases() {
		for _, m := range mutations {
			t.Run(tc.name+"/"+m.name, func(t *testing.T) {
				fake := &contractsfake.MessageComposer{}

				rec, recs := msgServe(t, tc, fake, tc.validBody, m.mut)

				assertErrorEnvelope(t, rec, http.StatusUnauthorized)
				msgAssertPortUntouched(t, fake)
				logassert.OutcomeLogged(t, recs)
			})
		}
	}
}

// TestMessageHandlers_RejectEmptySessionID: autenticado porem sem Id e' 400 —
// "sei quem voce e', mas nao ha sessao a que essa chamada se refira" — com log.
func TestMessageHandlers_RejectEmptySessionID(t *testing.T) {
	for _, tc := range msgHandlerCases() {
		t.Run(tc.name, func(t *testing.T) {
			fake := &contractsfake.MessageComposer{}

			rec, recs := msgServe(t, tc, fake, tc.validBody, func(r *http.Request) *http.Request {
				return withUser(r, "")
			})

			assertErrorEnvelope(t, rec, http.StatusBadRequest)
			msgAssertPortUntouched(t, fake)
			logassert.OutcomeLogged(t, recs)
		})
	}
}

// TestMessageHandlers_RejectMalformedBody: corpo ilegivel e' 400 do cliente, a
// porta nao e' tocada, e o log carrega o erro do decoder — a causa real, nao o
// erro-sentinela generico que vai para o cliente.
func TestMessageHandlers_RejectMalformedBody(t *testing.T) {
	bodies := []struct {
		name string
		body string
	}{
		{"json truncado", `{"Phone":"5511999`},
		{"json invalido", `nao sou json`},
		{"corpo vazio", ``},
		{"tipo errado no campo", `{"Phone":42}`},
	}

	for _, tc := range msgHandlerCases() {
		for _, b := range bodies {
			t.Run(tc.name+"/"+b.name, func(t *testing.T) {
				fake := &contractsfake.MessageComposer{}

				rec, recs := msgServe(t, tc, fake, b.body, msgAuthed)

				assertErrorEnvelope(t, rec, http.StatusBadRequest)
				msgAssertPortUntouched(t, fake)
				logassert.OutcomeLogged(t, recs)
			})
		}
	}
}

// TestMessageHandlers_RejectMissingRequiredField percorre a matriz de campos
// obrigatorios de cada use case a partir da fronteira HTTP. E' a paridade com
// os testes de use case da F11 medida uma camada acima: o que la' e' um erro
// de retorno, aqui tem de virar resposta >=400 COM log.
func TestMessageHandlers_RejectMissingRequiredField(t *testing.T) {
	for _, tc := range msgHandlerCases() {
		for field, body := range tc.missingField {
			t.Run(tc.name+"/sem "+field, func(t *testing.T) {
				fake := &contractsfake.MessageComposer{}

				rec, recs := msgServe(t, tc, fake, body, msgAuthed)

				if rec.Code < 400 {
					t.Fatalf("payload sem %s produziu status de sucesso %d", field, rec.Code)
				}
				msgAssertPortUntouched(t, fake)
				logassert.OutcomeLogged(t, recs)
			})
		}
	}
}

// TestMessageHandlers_SessionFailure: a porta de sessao recusa. O handler
// devolve erro ao cliente e o co-gate D exige que a CAUSA da porta apareca no
// log — e' a diferenca entre "algo deu 500" e um log operavel.
func TestMessageHandlers_SessionFailure(t *testing.T) {
	for _, tc := range msgHandlerCases() {
		t.Run(tc.name, func(t *testing.T) {
			fake := &contractsfake.MessageComposer{SessionGuard: contractsfake.FailSession(msgSentinelErr)}

			rec, recs := msgServe(t, tc, fake, tc.validBody, msgAuthed)

			if rec.Code < 400 {
				t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
			}
			if n := len(fake.EnsureSessionCalls); n != 1 {
				t.Fatalf("EnsureSession chamado %d vez(es), quero 1", n)
			}
			if n := len(fake.NewMessageIDCalls); n != 0 {
				t.Fatalf("sessao invalida ainda gerou message ID (%d chamada(s))", n)
			}
			logassert.OutcomeLogged(t, recs, msgSentinelToken)
		})
	}
}

// TestMessageHandlers_MessageIDFailure cobre o unico caminho de erro exclusivo
// de send/text e send/template: a sessao esta' viva, mas a geracao do message
// ID falha. Os outros dois handlers nunca chegam aqui — e o teste assere isso,
// para que mover a geracao de ID para eles nao passe despercebido.
func TestMessageHandlers_MessageIDFailure(t *testing.T) {
	for _, tc := range msgHandlerCases() {
		t.Run(tc.name, func(t *testing.T) {
			fake := &contractsfake.MessageComposer{
				NewMessageIDFunc: func(context.Context, string) (string, error) {
					return "", msgSentinelErr
				},
			}

			rec, recs := msgServe(t, tc, fake, tc.validBody, msgAuthed)

			if !tc.generatesID {
				if rec.Code != http.StatusOK {
					t.Fatalf("handler que nao gera ID falhou por causa de NewMessageID: status %d", rec.Code)
				}
				if n := len(fake.NewMessageIDCalls); n != 0 {
					t.Fatalf("handler passou a gerar message ID (%d chamada(s)) — a tabela ficou desatualizada", n)
				}
				return
			}

			if rec.Code < 400 {
				t.Fatalf("falha na geracao de message ID produziu status de sucesso %d", rec.Code)
			}
			if n := len(fake.NewMessageIDCalls); n != 1 {
				t.Fatalf("NewMessageID chamado %d vez(es), quero 1", n)
			}
			logassert.OutcomeLogged(t, recs, msgSentinelToken)
		})
	}
}

// TestMessageHandlers_ClientSuppliedIDSkipsGeneration: com Id no payload, nem
// send/text nem send/template tocam NewMessageID, e o ID do cliente e' o que
// volta. Sem este caso, o ramo `if msgID == ""` dos dois use cases so' seria
// exercitado num sentido.
func TestMessageHandlers_ClientSuppliedIDSkipsGeneration(t *testing.T) {
	bodies := map[string]string{
		"SendMessage":  `{"Phone":"5511999999999","Body":"ola","Id":"id-do-cliente"}`,
		"SendTemplate": `{"Phone":"5511999999999","Content":"corpo","Footer":"rodape","Id":"id-do-cliente"}`,
	}

	for _, tc := range msgHandlerCases() {
		body, ok := bodies[tc.name]
		if !ok {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			fake := &contractsfake.MessageComposer{
				NewMessageIDFunc: func(context.Context, string) (string, error) {
					t.Error("NewMessageID chamado com Id fornecido pelo cliente")
					return "", nil
				},
			}

			rec, _ := msgServe(t, tc, fake, body, msgAuthed)

			if rec.Code != http.StatusOK {
				t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
			}
			var data struct {
				MessageID string `json:"message_id"`
			}
			if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &data); err != nil {
				t.Fatalf("envelope.data invalido: %v", err)
			}
			if data.MessageID != "id-do-cliente" {
				t.Errorf("message_id: got %q, want %q", data.MessageID, "id-do-cliente")
			}
		})
	}
}

// TestMessageHandlers_NoSecretInLog e' a clausula (d) do co-gate D exercitada
// contra os quatro handlers com os segredos da F9.4 REALMENTE presentes no
// caminho — plantados no corpo da requisicao e no cabecalho Authorization. Sem
// o segredo no caminho, a ausencia dele no log nao significaria nada.
//
// A falha da porta usa o erro-sentinela comum, e nao um erro cujo TEXTO seja
// um segredo: logar a causa que a porta devolveu e' o comportamento correto e
// exigido pelo co-gate D. O que esta' sob teste aqui e' o handler nao despejar
// no log o que veio do CLIENTE — corpo e cabecalhos.
func TestMessageHandlers_NoSecretInLog(t *testing.T) {
	for _, tc := range msgHandlerCases() {
		t.Run(tc.name, func(t *testing.T) {
			fake := &contractsfake.MessageComposer{SessionGuard: contractsfake.FailSession(msgSentinelErr)}
			body := `{"Phone":"5511999999999","Body":"` + logassertGlobalEncryptionKey +
				`","Content":"x","Footer":"y","Id":"` + logassertGlobalHMACKey + `"}`

			h, capture := logassert.Wrap(tc.build(fake))
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(body))
			req.Header.Set("Authorization", logassertAdminToken)
			h.ServeHTTP(rec, msgAuthed(req))

			if rec.Code < 400 {
				t.Fatalf("porta em falha produziu status de sucesso %d", rec.Code)
			}
			logassert.NoSecrets(t, capture.Records(t))
		})
	}
}

// msgAssertPortUntouched: uma rejeicao de fronteira nao pode ter falado com o
// WhatsApp antes de descobrir que ia rejeitar.
func msgAssertPortUntouched(t *testing.T, fake *contractsfake.MessageComposer) {
	t.Helper()
	if n := len(fake.EnsureSessionCalls); n != 0 {
		t.Fatalf("requisicao rejeitada alcancou EnsureSession %d vez(es)", n)
	}
	if n := len(fake.NewMessageIDCalls); n != 0 {
		t.Fatalf("requisicao rejeitada alcancou NewMessageID %d vez(es)", n)
	}
}
