package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"github.com/rs/zerolog/log"

	"wa-api/internal/noise"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
	wachat "wa-api/pkg/infra/noise/adapters/chat"
	clientpkg "wa-api/pkg/infra/noise/client"
	wajid "wa-api/pkg/infra/noise/mapping/jid"
	"wa-api/pkg/infra/noise/observability/applog"
)

// FIX-14 — trava do wiring `.WithPollOptions` (wiring_handlers.go:115).
//
// O CAP-14 fez SendPoll RECUSAR-SE a enviar quando o registrador de opcoes nao
// esta ligado (messenger.go:510). O fail-closed protege o dado; ele nao protege
// a FIACAO. Sem estes testes, remover `.WithPollOptions(clientManager)` do
// wiring compila, passa em todo o resto da suite, e so' aparece quando um
// usuario tenta criar enquete em producao — descoberto por usuario, nao por CI,
// que e' exatamente o custo que a F129 mediu.
//
// Precedente: chat_history_route_test.go (CAP-09A). Teste de FIACAO, nao de
// payload — o defeito e' de montagem e sobreviveria a qualquer assercao sobre
// corpo de resposta. Como la', o teste passa pela ROTA REGISTRADA, e os
// handlers sao os que `initCustomHandlers` monta, e nao um conjunto montado
// pelo teste: um conjunto montado a mao nao exercitaria wiring_handlers.go.
//
// Por que log e nao corpo de resposta: o handler responde 500 com o texto
// generico "internal server error" (handler_interactive.go:235 ->
// RespondJSON), entao a CAUSA so' e' observavel no log que o use case emite
// (send_poll.go:76). Medir comportamento pela unica saida que carrega a causa
// e' mais forte que inspecionar o texto do fonte com grep: o que se prova aqui
// e' que a execucao REAL atravessou o guarda fail-closed.

// causaSemRegistrador e' a mensagem de errNoPollOptionRecorder
// (pkg/infra/wa-noise/adapters/chat/messenger.go:494) — a recusa de enviar sem
// onde guardar as opcoes em claro. E' a assinatura do wiring quebrado.
const causaSemRegistrador = "poll option recorder not configured"

// causaNaoLogado e' core.ErrNotLoggedIn
// (internal/wa-noise/core/errors.go:22), o erro que o SDK devolve quando o
// envio chega ate' ele com um device sem JID. E' a assinatura do wiring
// INTEIRO percorrido: so' se chega ali DEPOIS do guarda de registrador. A
// fachada (internal/wa-noise/main.go) nao reexporta o sentinel, e alargar a
// fachada seria mudanca de producao — por isso o literal, com a origem citada.
const causaNaoLogado = "the store doesn't contain a device JID"

// pollWiringUser e' o txtID usado pelos dois testes.
const pollWiringUser = "FIX14"

// pollWiringBody e' um payload VALIDO: os tres campos que send_poll.go valida
// antes de tocar a porta (Group, Header, >= 2 opcoes). Um payload invalido
// pararia na validacao e nunca alcancaria o adapter — o teste passaria sem
// medir a fiacao.
const pollWiringBody = `{"Group":"5511999999999","Header":"almoco?","Options":["pizza","sushi"]}`

// newPollWiringRouter monta o roteador de producao sobre os handlers que
// initCustomHandlers construiu.
//
// customHandlerSet e' global e initCustomHandlers o reescreve; o teste o
// restaura no Cleanup para nao deixar estado para os testes seguintes.
func newPollWiringRouter(t *testing.T) *mux.Router {
	t.Helper()

	anterior := customHandlerSet
	t.Cleanup(func() { customHandlerSet = anterior })

	initCustomHandlers(&server{DB: newChatHistoryDB(t), ExPath: t.TempDir()})

	inject := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(
				r.Context(), appport.UserInfoKey, *userValues(pollWiringUser, 0))))
		})
	}

	router := mux.NewRouter()
	registerCustomRoutes(router, alice.New(inject), customHandlerSet)
	return router
}

// seedPollWiringSession registra um cliente wa-noise para pollWiringUser.
//
// Nao e' um duble: e' um *noise.Client de verdade, sem device. E' o minimo
// que faz EnsureSession (guard.go:57) passar — sem ele o use case para ANTES
// de chamar SendPoll (send_poll.go:62) e o teste nunca alcancaria o guarda de
// registrador. O cliente nao pode ser um fake porque a producao converte o
// tipo CONCRETO (clientpkg.ClientForGetter, client.go:160).
func seedPollWiringSession(t *testing.T) {
	t.Helper()
	clientManager.SetNoiseClient(pollWiringUser, noise.NewClient(nil, nil))
	t.Cleanup(func() { clientManager.DeleteNoiseClient(pollWiringUser) })
}

// TestPollOptionsRecorderIsWiredIntoChatMessenger e' a trava.
//
// O que ela impede: que `.WithPollOptions(clientManager)` desapareca de
// wiring_handlers.go:115. Com a chamada no lugar, um POST /chats/send/poll pela
// rota real atravessa handler -> use case -> ChatMessengerAdapter.SendPoll ->
// SDK, e falha no SDK (sem device). Sem a chamada, ele para no guarda
// fail-closed do adapter e a causa vira causaSemRegistrador — voto ilegivel,
// que e' precisamente o defeito que o CAP-14 se recusa a produzir.
func TestPollOptionsRecorderIsWiredIntoChatMessenger(t *testing.T) {
	captura := captureLogInto(t)
	router := newPollWiringRouter(t)
	seedPollWiringSession(t)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(
		http.MethodPost, "/chats/send/poll", strings.NewReader(pollWiringBody)))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, quero 500 (corpo: %s)", rec.Code, rec.Body.String())
	}

	logado := captura.String()
	if strings.Contains(logado, causaSemRegistrador) {
		t.Fatalf("o envio de enquete parou no guarda fail-closed: o wiring deixou de ligar "+
			"o registrador de opcoes ao ChatMessengerAdapter "+
			"(wiring_handlers.go: .WithPollOptions(clientManager)). "+
			"Enquanto ele faltar, /chats/send/poll nao envia NENHUMA enquete; se a "+
			"guarda tambem cair, a enquete e' criada e todo voto chega ilegivel — "+
			"o voto vem como SHA-256 do texto da opcao, e sem as opcoes memorizadas "+
			"eventhandler_message.go:131 nao tem com o que casar o hash. log: %s", logado)
	}
	if !strings.Contains(logado, causaNaoLogado) {
		t.Fatalf("o envio nao chegou ao SDK: quero a causa %q no log, que so' aparece "+
			"DEPOIS do guarda de registrador. Sem ela o teste nao provou nada sobre a "+
			"fiacao — pode ter parado antes, na validacao ou na sessao. log: %s",
			causaNaoLogado, logado)
	}
}

// TestPollOptionsRecorderAbsenceIsWhatTheWiringTestDetects e' o controle
// POSITIVO da assercao acima: prova que causaSemRegistrador e' de fato o que um
// wiring sem `.WithPollOptions` produz, e nao uma string inventada pelo teste.
//
// Sem ele, TestPollOptionsRecorderIsWiredIntoChatMessenger poderia passar para
// sempre por procurar uma causa que nada jamais emite — a ARMADILHA 3 deste
// repo na sua forma silenciosa.
//
// O adapter e o use case sao os da PRODUCAO, com o mesmo lookup de cliente
// (clientpkg.ClientForGetter sobre clientManager.GetNoiseClient, como
// wiring_handlers.go:106). A UNICA diferenca em relacao ao wiring real e' a
// chamada `.WithPollOptions` ausente.
func TestPollOptionsRecorderAbsenceIsWhatTheWiringTestDetects(t *testing.T) {
	seedPollWiringSession(t)

	lookup := clientpkg.ClientForGetter(clientManager.GetNoiseClient)
	semRegistrador := wachat.NewChatMessengerAdapter(lookup)
	uc := message.NewSendPollUseCase(
		semRegistrador, wajid.NewJIDResolverAdapter(), applog.NewZerologAdapter(log.Logger))

	_, err := uc.Execute(context.Background(), pollWiringUser, domain.SendPollRequest{
		Group:   "5511999999999",
		Header:  "almoco?",
		Options: []string{"pizza", "sushi"},
	})
	if err == nil {
		t.Fatal("envio sem registrador retornou sucesso; o guarda fail-closed do CAP-14 sumiu")
	}
	if !strings.Contains(err.Error(), causaSemRegistrador) {
		t.Fatalf("erro = %q, quero conter %q — se a mensagem mudou, "+
			"TestPollOptionsRecorderIsWiredIntoChatMessenger deixou de morder e "+
			"a constante causaSemRegistrador precisa acompanhar messenger.go",
			err.Error(), causaSemRegistrador)
	}
	if strings.Contains(err.Error(), causaNaoLogado) {
		t.Fatalf("erro = %q: o envio passou do guarda e chegou ao SDK sem registrador", err.Error())
	}
}
