package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
)

// Este arquivo cobre as duas capabilities de MUTAÇÃO de mensagem desde o
// CAP-10 — POST /chat/delete/message, POST /chat/delete e POST
// /chat/send/edit — que deixaram de devolver Status="validated" sem revogar
// nem editar nada e passaram a mutar de verdade pelo wa-noise.
//
// Herda os eixos que handler_message_test.go cobria para as duas antes da
// migração (não autenticado, session id vazio, corpo malformado, campo
// obrigatório ausente, falha de sessão, sucesso, ausência de segredo no
// log) e acrescenta os que só a mutação real trouxe: JID que não parseia,
// falha do envio que NÃO pode virar 200, e o alias de rota de delete.
//
// Rota REGISTRADA (gorilla/mux), nunca handler.ServeHTTP cru — ARMADILHA 2
// deste repo.

const mutationSentinelToken = "message-mutation-sentinel-cause-9c4e2a"

var errMutationSentinel = errors.New(mutationSentinelToken)

// mutationUnauthorizedCause é a causa canônica que os dois handlers põem em
// `error` ao recusar na fronteira de autenticação (errUnauthorized, em
// errors.go:7). Sem esta asserção o 401 passa mesmo se o handler parar de
// registrar a recusa — o status sozinho não prova que houve log com causa.
const mutationUnauthorizedCause = "unauthorized"

// deleteRoutePaths são as DUAS rotas que apontam para o MESMO handler de
// delete. O alias não é erro de migração: `/chat/delete` e
// `/chat/delete/message` já apontavam para o mesmo handler antes do
// refactor (`git show 41bc8e2^:custom_routes.go`, linhas 51 e 171), e
// preservar as duas é contrato público. Toda asserção sobre delete roda nas
// duas — uma tabela que só testasse uma deixaria o alias sem cobertura.
var deleteRoutePaths = []string{"/chat/delete/message", "/chat/delete"}

// mutationRouter registra os três caminhos exatamente como
// wiring_routes.go:77, :78 e :204 fazem — inclusive o alias de delete
// apontando para o MESMO handler, e não para uma segunda instância.
func mutationRouter(cm *contractsfake.ChatMessenger, jr *contractsfake.JIDResolver) http.Handler {
	del := NewDeleteMessageHandler(message.NewDeleteMessageUseCase(cm, jr, silentLogger{}))
	edit := NewSendEditMessageHandler(message.NewSendEditMessageUseCase(cm, jr, silentLogger{}))

	r := mux.NewRouter()
	for _, p := range deleteRoutePaths {
		r.Handle(p, del).Methods(http.MethodPost)
	}
	r.Handle("/chat/send/edit", edit).Methods(http.MethodPost)
	return r
}

func mutationServe(t *testing.T, cm *contractsfake.ChatMessenger, jr *contractsfake.JIDResolver, path, body string, mut func(*http.Request) *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	mutationRouter(cm, jr).ServeHTTP(rec, mut(req))
	return rec
}

// mutationServeCapturingLog é o mutationServe com a cadeia hlog de produção
// instalada, para que os caminhos de erro possam asseverar a CAUSA e não só
// o status — o envelope de erro deste repo é o genérico, então duas
// rejeições com o mesmo status são indistinguíveis pelo corpo.
func mutationServeCapturingLog(t *testing.T, cm *contractsfake.ChatMessenger, jr *contractsfake.JIDResolver, path, body string, mut func(*http.Request) *http.Request) (*httptest.ResponseRecorder, []logLine) {
	t.Helper()
	wrapped, capture := logassert.Wrap(mutationRouter(cm, jr))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	wrapped.ServeHTTP(rec, mut(req))
	return rec, capture.Records(t)
}

type mutationResultBody struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp"`
	Status    string `json:"status"`
}

// mutationCase descreve uma das duas capabilities pelos eixos que as duas
// compartilham. paths traz TODAS as rotas da capability — duas para delete,
// uma para edit.
type mutationCase struct {
	name      string
	paths     []string
	validBody string
	// wantStatus é o valor de `status` no envelope de sucesso.
	wantStatus string
	// mutations conta quantas vezes a porta de mutação DESTA capability foi
	// chamada.
	mutations func(*contractsfake.ChatMessenger) int
	// portTxtID é o txtID que a porta desta capability RECEBEU na primeira
	// chamada. Vazio quando a porta não foi alcançada.
	portTxtID func(*contractsfake.ChatMessenger) string
	// missingField mapeia campo obrigatório ausente -> payload que o omite,
	// e a causa que o log tem de registrar.
	missingField map[string]struct{ body, cause string }
}

func mutationCases() []mutationCase {
	return []mutationCase{
		{
			name:       "DeleteMessage",
			paths:      deleteRoutePaths,
			validBody:  `{"Phone":"5511999999999","Id":"3EB0ABC123"}`,
			wantStatus: domain.StatusDeleted,
			mutations:  func(cm *contractsfake.ChatMessenger) int { return len(cm.RevokeMessageCalls) },
			portTxtID: func(cm *contractsfake.ChatMessenger) string {
				if len(cm.RevokeMessageCalls) == 0 {
					return ""
				}
				return cm.RevokeMessageCalls[0].TxtID
			},
			missingField: map[string]struct{ body, cause string }{
				"Phone": {`{"Id":"3EB0ABC123"}`, "missing Phone in payload"},
				"Id":    {`{"Phone":"5511999999999"}`, "missing Id in payload"},
			},
		},
		{
			name:       "SendEditMessage",
			paths:      []string{"/chat/send/edit"},
			validBody:  `{"Phone":"5511999999999","Body":"corrigido","Id":"3EB0ABC123"}`,
			wantStatus: domain.StatusSent,
			mutations:  func(cm *contractsfake.ChatMessenger) int { return len(cm.EditMessageCalls) },
			portTxtID: func(cm *contractsfake.ChatMessenger) string {
				if len(cm.EditMessageCalls) == 0 {
					return ""
				}
				return cm.EditMessageCalls[0].TxtID
			},
			missingField: map[string]struct{ body, cause string }{
				"Phone": {`{"Body":"corrigido","Id":"3EB0ABC123"}`, "missing Phone in payload"},
				"Body":  {`{"Phone":"5511999999999","Id":"3EB0ABC123"}`, "missing Body in payload"},
				"Id":    {`{"Phone":"5511999999999","Body":"corrigido"}`, "missing Id in payload"},
			},
		},
	}
}

// TestMessageMutation_Success_ViaRegisteredRoute prova o caminho HTTP ->
// handler -> usecase -> ChatMessenger pela rota REGISTRADA, para as TRÊS
// rotas: status de mutação (não mais "validated"), message_id igual ao ID
// da mensagem ALVO e timestamp vindo do envio real.
func TestMessageMutation_Success_ViaRegisteredRoute(t *testing.T) {
	const sentAt = int64(1755500110)
	for _, tc := range mutationCases() {
		for _, path := range tc.paths {
			t.Run(tc.name+path, func(t *testing.T) {
				cm := &contractsfake.ChatMessenger{
					RevokeMessageFunc: func(_ context.Context, _ string, _ domain.JID, _ string) (domain.MessageSendResult, error) {
						return domain.MessageSendResult{ID: "id-da-revogacao", Timestamp: time.Unix(sentAt, 0)}, nil
					},
					EditMessageFunc: func(_ context.Context, _ string, _ domain.JID, _, _ string, _ *domain.EditContextInfo) (domain.MessageSendResult, error) {
						return domain.MessageSendResult{ID: "id-da-edicao", Timestamp: time.Unix(sentAt, 0)}, nil
					},
				}
				jr := &contractsfake.JIDResolver{}

				rec, recs := mutationServeCapturingLog(t, cm, jr, path, tc.validBody, msgAuthed)

				if rec.Code != http.StatusOK {
					t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
				}
				env := decodeEnvelope(t, rec)
				if !env.Success {
					t.Fatalf("envelope.success=false num 200: %s", rec.Body.String())
				}
				var data mutationResultBody
				if err := json.Unmarshal(env.Data, &data); err != nil {
					t.Fatalf("envelope.data invalido: %v", err)
				}
				if data.Status != tc.wantStatus {
					t.Errorf("status: got %q, want %q", data.Status, tc.wantStatus)
				}
				// O message_id é o da mensagem ALVO, como no histórico
				// (`"Id": msgid`), NÃO o da mensagem de revogação/edição
				// que o envio criou.
				if data.MessageID != "3EB0ABC123" {
					t.Errorf("message_id: got %q, want %q (o ID da mensagem alvo)", data.MessageID, "3EB0ABC123")
				}
				if data.Timestamp != sentAt {
					t.Errorf("timestamp: got %d, want %d (o do envio real)", data.Timestamp, sentAt)
				}
				if n := tc.mutations(cm); n != 1 {
					t.Fatalf("porta de mutacao chamada %d vez(es) pela rota registrada, quero 1", n)
				}
				// O txtID que chega a' porta e' o do CONTEXTO autenticado, e
				// nao um campo do payload nem um valor inventado pelo handler
				// (eixo da F142). As nove capabilities de ENVIO ganharam este
				// eixo no CAP-18; estas duas ficaram de fora e so' o
				// recebem agora (CAP-19).
				if got := tc.portTxtID(cm); got != "user-1" {
					t.Fatalf("a porta recebeu txtID %q, quero %q — o Id do contexto autenticado",
						got, "user-1")
				}
				// O caminho de SUCESSO nao emite warn/error: a requisicao ja'
				// e' registrada pelo middleware de fronteira, e um registro
				// aqui seria ruido redundante inflando a metrica de log. Eixo
				// herdado de TestMessageHandlers_Success, que cobria estas
				// duas capabilities antes do CAP-10.
				//
				// A assercao e' o helper compartilhado, e nao uma copia do
				// laco por nivel: era a UNICA copia restante depois do CAP-17,
				// e copia de assercao foi a causa raiz da F143 (CAP-19).
				assertNoOutcomeLog(t, recs)
			})
		}
	}
}

// TestMessageMutation_BothDeleteRoutesBehaveIdentically trava o alias como
// contrato: as duas rotas de delete têm de produzir o MESMO status, o MESMO
// corpo e a MESMA chamada de porta. Só um teste que compara as duas pega uma
// divergência introduzida em apenas uma delas.
func TestMessageMutation_BothDeleteRoutesBehaveIdentically(t *testing.T) {
	const body = `{"Phone":"5511999999999","Id":"3EB0ABC123"}`

	type observed struct {
		code      int
		body      string
		messageID string
	}
	seen := make(map[string]observed, len(deleteRoutePaths))

	for _, path := range deleteRoutePaths {
		cm := &contractsfake.ChatMessenger{
			RevokeMessageFunc: func(_ context.Context, _ string, target domain.JID, messageID string) (domain.MessageSendResult, error) {
				if target != domain.JID("5511999999999@s.whatsapp.net") {
					t.Errorf("%s: target: got %q", path, target)
				}
				if messageID != "3EB0ABC123" {
					t.Errorf("%s: messageID: got %q", path, messageID)
				}
				return domain.MessageSendResult{ID: "id-da-revogacao", Timestamp: time.Unix(1755500110, 0)}, nil
			},
		}
		jr := &contractsfake.JIDResolver{}

		rec := mutationServe(t, cm, jr, path, body, msgAuthed)

		if n := len(cm.RevokeMessageCalls); n != 1 {
			t.Fatalf("%s: RevokeMessage chamado %d vez(es), quero 1", path, n)
		}
		seen[path] = observed{code: rec.Code, body: rec.Body.String(), messageID: cm.RevokeMessageCalls[0].MessageID}
	}

	first := seen[deleteRoutePaths[0]]
	for _, path := range deleteRoutePaths[1:] {
		got := seen[path]
		if got.code != first.code {
			t.Errorf("%s devolveu %d e %s devolveu %d — o alias divergiu", path, got.code, deleteRoutePaths[0], first.code)
		}
		if got.body != first.body {
			t.Errorf("%s devolveu corpo %q e %s devolveu %q — o alias divergiu", path, got.body, deleteRoutePaths[0], first.body)
		}
		if got.messageID != first.messageID {
			t.Errorf("%s revogou %q e %s revogou %q — o alias divergiu", path, got.messageID, deleteRoutePaths[0], first.messageID)
		}
	}
}

// TestMessageMutation_RevokeTargetsOwnMessage prova o argumento que um
// engano silencioso trocaria: o usecase entrega à porta o ID da mensagem
// ALVO e o JID da conversa resolvidos do payload, e NADA MAIS — não há
// campo de payload que permita revogar mensagem de terceiro, e nenhum é
// inventado aqui. (Que o sender no wire é EmptyJID é travado um nível
// abaixo, em messenger_mutation_test.go do adapter.)
func TestMessageMutation_RevokeTargetsOwnMessage(t *testing.T) {
	for _, path := range deleteRoutePaths {
		t.Run(path, func(t *testing.T) {
			cm := &contractsfake.ChatMessenger{}
			jr := &contractsfake.JIDResolver{}

			rec := mutationServe(t, cm, jr, path, `{"Phone":"5511999999999","Id":"3EB0ABC123","Participant":"5511888888888"}`, msgAuthed)

			if rec.Code != http.StatusOK {
				t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
			}
			if n := len(cm.RevokeMessageCalls); n != 1 {
				t.Fatalf("RevokeMessage chamado %d vez(es), quero 1", n)
			}
			call := cm.RevokeMessageCalls[0]
			if call.Target != domain.JID("5511999999999@s.whatsapp.net") {
				t.Errorf("Target: got %q, want %q", call.Target, "5511999999999")
			}
			if call.MessageID != "3EB0ABC123" {
				t.Errorf("MessageID: got %q, want %q", call.MessageID, "3EB0ABC123")
			}
		})
	}
}

// TestMessageMutation_EditForwardsTargetIDAndNewBody trava os dois
// argumentos de EditMessage que um engano silencioso trocaria de lugar: o Id
// é o da mensagem a editar e o Body é o texto NOVO. Os dois são strings, e
// invertê-los compila.
func TestMessageMutation_EditForwardsTargetIDAndNewBody(t *testing.T) {
	cm := &contractsfake.ChatMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := mutationServe(t, cm, jr, "/chat/send/edit", `{"Phone":"5511999999999","Body":"texto corrigido","Id":"3EB0ABC123"}`, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if n := len(cm.EditMessageCalls); n != 1 {
		t.Fatalf("EditMessage chamado %d vez(es), quero 1", n)
	}
	call := cm.EditMessageCalls[0]
	if call.MessageID != "3EB0ABC123" {
		t.Errorf("MessageID: got %q, want %q (o Id do payload, nao o Body)", call.MessageID, "3EB0ABC123")
	}
	if call.NewText != "texto corrigido" {
		t.Errorf("NewText: got %q, want %q (o Body do payload, nao o Id)", call.NewText, "texto corrigido")
	}
}

func TestMessageMutation_RejectUnauthenticated(t *testing.T) {
	anon := func(r *http.Request) *http.Request { return r }
	for _, tc := range mutationCases() {
		for _, path := range tc.paths {
			t.Run(tc.name+path, func(t *testing.T) {
				cm := &contractsfake.ChatMessenger{}
				jr := &contractsfake.JIDResolver{}

				rec, recs := mutationServeCapturingLog(t, cm, jr, path, tc.validBody, anon)

				assertErrorEnvelope(t, rec, http.StatusUnauthorized)
				logassert.OutcomeLogged(t, recs, mutationUnauthorizedCause)
				if n := tc.mutations(cm); n != 0 {
					t.Fatalf("requisicao nao autenticada mutou %d vez(es)", n)
				}
			})
		}
	}
}

// TestMessageMutation_WrongTypeInContext: a chave do contexto é tipada mas o
// VALOR é `any`; um valor que não satisfaz userInfo tem de virar 401, não
// pânico.
func TestMessageMutation_WrongTypeInContext(t *testing.T) {
	wrongType := func(r *http.Request) *http.Request {
		return r.WithContext(context.WithValue(r.Context(), appport.UserInfoKey, 42))
	}
	for _, tc := range mutationCases() {
		for _, path := range tc.paths {
			t.Run(tc.name+path, func(t *testing.T) {
				cm := &contractsfake.ChatMessenger{}
				jr := &contractsfake.JIDResolver{}

				rec, recs := mutationServeCapturingLog(t, cm, jr, path, tc.validBody, wrongType)

				assertErrorEnvelope(t, rec, http.StatusUnauthorized)
				logassert.OutcomeLogged(t, recs, mutationUnauthorizedCause)
				if n := tc.mutations(cm); n != 0 {
					t.Fatalf("contexto com tipo errado mutou %d vez(es)", n)
				}
			})
		}
	}
}

// TestMessageMutation_MissingSessionID: requisição AUTENTICADA com `Id`
// vazio é 400 do cliente, não 401. O corpo é VÁLIDO de propósito — se a
// guarda não disparar, nada mais impede a mutação.
func TestMessageMutation_MissingSessionID(t *testing.T) {
	noSessionID := func(r *http.Request) *http.Request { return withUser(r, "") }
	for _, tc := range mutationCases() {
		for _, path := range tc.paths {
			t.Run(tc.name+path, func(t *testing.T) {
				cm := &contractsfake.ChatMessenger{}
				jr := &contractsfake.JIDResolver{}

				rec, recs := mutationServeCapturingLog(t, cm, jr, path, tc.validBody, noSessionID)

				if rec.Code != http.StatusBadRequest {
					t.Fatalf("status: got %d, want 400 (corpo: %s)", rec.Code, rec.Body.String())
				}
				logassert.OutcomeLogged(t, recs, "missing session id")
				if n := tc.mutations(cm); n != 0 {
					t.Fatalf("requisicao sem session id mutou %d vez(es)", n)
				}
			})
		}
	}
}

// TestMessageMutation_MalformedBody: JSON truncado, requisição AUTENTICADA
// de propósito — sem isso o 401 mascara o 400. A CAUSA entra na asserção
// porque um payload truncado também produziria 400 pelo use case
// (missing_phone) se o decode fosse ignorado, e o envelope de erro é o
// genérico: só o registro de saída distingue os dois.
//
// A distinção vem do campo `message`, e não do `error`: estes dois
// handlers logam o erro CRU do decodificador em `error` ("unexpected EOF"),
// que varia com o ponto do truncamento e não serve de asserção estável. O
// OutcomeLogged sem substring continua exigindo o registro bem formado
// (nível, req_id, campo error presente) e a ausência de segredos.
func TestMessageMutation_MalformedBody(t *testing.T) {
	const malformed = `{"Phone":"5511`
	for _, tc := range mutationCases() {
		for _, path := range tc.paths {
			t.Run(tc.name+path, func(t *testing.T) {
				cm := &contractsfake.ChatMessenger{}
				jr := &contractsfake.JIDResolver{}

				rec, recs := mutationServeCapturingLog(t, cm, jr, path, malformed, msgAuthed)

				assertErrorEnvelope(t, rec, http.StatusBadRequest)
				got := logassert.OutcomeLogged(t, recs)
				if msg := got.str("message"); !strings.Contains(msg, "payload could not be decoded") {
					t.Errorf("registro de saida diz %q; nao distingue decode falho de campo ausente", msg)
				}
				if n := tc.mutations(cm); n != 0 {
					t.Fatalf("corpo malformado mutou %d vez(es)", n)
				}
			})
		}
	}
}

// TestMessageMutation_MissingRequiredField cobre Phone e Id para delete (nas
// DUAS rotas) e Phone, Body e Id para edit. A causa entra na tabela: o
// status sozinho não distingue "faltou Phone" de "faltou Id".
func TestMessageMutation_MissingRequiredField(t *testing.T) {
	for _, tc := range mutationCases() {
		for _, path := range tc.paths {
			for field, want := range tc.missingField {
				t.Run(tc.name+path+"/"+field, func(t *testing.T) {
					cm := &contractsfake.ChatMessenger{}
					jr := &contractsfake.JIDResolver{}

					rec, recs := mutationServeCapturingLog(t, cm, jr, path, want.body, msgAuthed)

					if rec.Code < 400 {
						t.Fatalf("payload sem %s produziu status de sucesso %d", field, rec.Code)
					}
					logassert.OutcomeLogged(t, recs, want.cause)
					if n := tc.mutations(cm); n != 0 {
						t.Fatalf("payload invalido, mas a mutacao aconteceu %d vez(es)", n)
					}
				})
			}
		}
	}
}

func TestMessageMutation_SessionFailure(t *testing.T) {
	for _, tc := range mutationCases() {
		for _, path := range tc.paths {
			t.Run(tc.name+path, func(t *testing.T) {
				cm := &contractsfake.ChatMessenger{SessionGuard: contractsfake.FailSession(errMutationSentinel)}
				jr := &contractsfake.JIDResolver{}

				rec, recs := mutationServeCapturingLog(t, cm, jr, path, tc.validBody, msgAuthed)

				if rec.Code < 400 {
					t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
				}
				logassert.OutcomeLogged(t, recs, mutationSentinelToken)
				if n := tc.mutations(cm); n != 0 {
					t.Fatalf("sessao invalida, mas a mutacao aconteceu %d vez(es)", n)
				}
			})
		}
	}
}

func TestMessageMutation_InvalidPhoneNeverMutates(t *testing.T) {
	for _, tc := range mutationCases() {
		for _, path := range tc.paths {
			t.Run(tc.name+path, func(t *testing.T) {
				cm := &contractsfake.ChatMessenger{}
				jr := &contractsfake.JIDResolver{
					ResolveJIDFunc: func(context.Context, string) (domain.JID, error) {
						return "", errors.New("jid invalido")
					},
				}

				rec := mutationServe(t, cm, jr, path, tc.validBody, msgAuthed)

				if rec.Code == http.StatusOK {
					t.Fatalf("JID invalido produziu 200: %s", rec.Body.String())
				}
				if n := tc.mutations(cm); n != 0 {
					t.Fatalf("JID invalido, mas a mutacao aconteceu %d vez(es)", n)
				}
			})
		}
	}
}

// TestMessageMutation_DownstreamFailureNeverReturns200 é o eixo central
// desta capability: o defeito consertado no CAP-10 era justamente devolver
// 200 sem que nada tivesse acontecido. Uma falha no envio da revogação/
// edição NÃO pode virar sucesso.
func TestMessageMutation_DownstreamFailureNeverReturns200(t *testing.T) {
	for _, tc := range mutationCases() {
		for _, path := range tc.paths {
			t.Run(tc.name+path, func(t *testing.T) {
				cm := &contractsfake.ChatMessenger{
					RevokeMessageFunc: func(context.Context, string, domain.JID, string) (domain.MessageSendResult, error) {
						return domain.MessageSendResult{}, errMutationSentinel
					},
					EditMessageFunc: func(context.Context, string, domain.JID, string, string, *domain.EditContextInfo) (domain.MessageSendResult, error) {
						return domain.MessageSendResult{}, errMutationSentinel
					},
				}
				jr := &contractsfake.JIDResolver{}

				rec, recs := mutationServeCapturingLog(t, cm, jr, path, tc.validBody, msgAuthed)

				if rec.Code == http.StatusOK {
					t.Fatalf("falha do envio produziu 200: %s", rec.Body.String())
				}
				env := decodeEnvelope(t, rec)
				if env.Success {
					t.Fatalf("envelope.success=true com envio falho: %s", rec.Body.String())
				}
				logassert.OutcomeLogged(t, recs, mutationSentinelToken)
			})
		}
	}
}

// TestMessageMutation_NoSecretLeak: Phone e o header Authorization carregam
// segredos da F9.4 (a sessao usa um id NAO secreto, de proposito); a sessao
// falha e o log de saida da rota REGISTRADA nao pode carregar nenhum deles.
func TestMessageMutation_NoSecretLeak(t *testing.T) {
	bodies := map[string]string{
		"/chat/delete/message": `{"Phone":"` + logassertGlobalHMACKey + `","Id":"3EB0ABC123"}`,
		"/chat/delete":         `{"Phone":"` + logassertGlobalHMACKey + `","Id":"3EB0ABC123"}`,
		"/chat/send/edit":      `{"Phone":"` + logassertGlobalHMACKey + `","Body":"` + logassertGlobalEncryptionKey + `","Id":"3EB0ABC123"}`,
	}
	for path, body := range bodies {
		t.Run(path, func(t *testing.T) {
			cm := &contractsfake.ChatMessenger{SessionGuard: contractsfake.FailSession(errors.New("message-mutation-secret-leak-cause"))}
			jr := &contractsfake.JIDResolver{}

			wrapped, capture := logassert.Wrap(mutationRouter(cm, jr))
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
			req = withUser(req, "no-secret-leak-session")
			req.Header.Set("Authorization", logassertAdminToken)

			rec := httptest.NewRecorder()
			wrapped.ServeHTTP(rec, req)

			if rec.Code < 400 {
				t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
			}
			logassert.NoSecrets(t, capture.Records(t))
		})
	}
}

// TestSendEditMessage_PhoneParseRejectedBeforeMissingID trava a ORDEM de
// validação de send/edit contra o histórico (`git show 41bc8e2^:handlers.go`,
// linhas 2919, 2924, 2929 e 2936): Phone, Body, o parse do Phone e só então
// Id. Com Phone inválido E Id ausente as duas guardas estão armadas, e só a
// ordem decide qual causa sai — o histórico responde "could not parse Phone".
//
// Este eixo precisa de teste próprio porque inverter as duas guardas passa em
// todos os outros testes desta tabela: TestMessageMutation_MissingRequiredField
// manda Phone VÁLIDO, e TestMessageMutation_InvalidPhoneNeverMutates manda Id
// PRESENTE — nenhum dos dois tem as duas guardas armadas ao mesmo tempo.
func TestSendEditMessage_PhoneParseRejectedBeforeMissingID(t *testing.T) {
	cm := &contractsfake.ChatMessenger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) {
			return "", errors.New("jid invalido")
		},
	}

	// Phone que não parseia E Id ausente: as DUAS guardas armadas.
	const body = `{"Phone":"nao-e-um-telefone","Body":"corrigido"}`

	rec, recs := mutationServeCapturingLog(t, cm, jr, "/chat/send/edit", body, msgAuthed)

	if rec.Code < 400 {
		t.Fatalf("payload invalido produziu status de sucesso %d", rec.Code)
	}
	got := logassert.OutcomeLogged(t, recs)
	if errField := got.str("error"); !strings.Contains(errField, "could not parse Phone") {
		t.Fatalf("causa: got %q, want a do PARSE (%q) — a ordem das guardas foi invertida "+
			"em relacao ao historico (Phone, Body, parse, Id)", errField, "could not parse Phone")
	}
	if n := len(cm.EditMessageCalls); n != 0 {
		t.Fatalf("payload invalido, mas EditMessage foi chamado %d vez(es)", n)
	}
}
