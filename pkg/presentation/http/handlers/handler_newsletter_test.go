package handlers

import (
	"context"
	"net/http"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/notification"
	"wa-api/pkg/domain"
)

// As onze rotas de newsletter. O que estes testes travam NAO e' "o handler
// responde 200": e' que CADA rota chega ao metodo certo da porta com o
// identificador certo. O modo de falha real destas rotas e' o handler ligar a
// rota ao NewsletterOp errado — onze rotas construidas pelo mesmo tipo com um
// campo a diferenciá-las e' exatamente onde um copiar-colar troca dois — e
// isso responde 200 com a operacao errada feita no servidor do WhatsApp.

func newsletterOps(nr *contractsfake.NewsletterReader) *NewsletterHandlers {
	return NewNewsletterHandlers(
		notification.NewNewsletterOpsUseCase(nr, &contractsfake.Logger{}))
}

// canalDeTeste e' o JID de canal usado em todas as rotas que exigem um.
const canalDeTeste = "120363000000000000@newsletter"

// TestNewsletter_CadaRotaChamaOMetodoCerto e' o teste da CAUSA: a ligacao
// rota -> operacao. Cada linha diz que corpo entra e que metodo da porta tem
// de sair, com o identificador que foi enviado.
func TestNewsletter_CadaRotaChamaOMetodoCerto(t *testing.T) {
	casos := []struct {
		rota       string
		handler    func(*NewsletterHandlers) http.Handler
		corpo      string
		wantMetodo string
		wantJID    domain.JID
		wantExtra  string
	}{
		{"/newsletter/create", func(h *NewsletterHandlers) http.Handler { return h.Create },
			`{"name":"Canal X","description":"d"}`, "CreateNewsletter", "", "Canal X"},
		{"/newsletter/info", func(h *NewsletterHandlers) http.Handler { return h.Info },
			`{"jid":"` + canalDeTeste + `"}`, "NewsletterInfo", canalDeTeste, ""},
		{"/newsletter/info-invite", func(h *NewsletterHandlers) http.Handler { return h.InfoInvite },
			`{"invite":"AbCd1234"}`, "NewsletterInfoWithInvite", "", "AbCd1234"},
		{"/newsletter/follow", func(h *NewsletterHandlers) http.Handler { return h.Follow },
			`{"jid":"` + canalDeTeste + `"}`, "FollowNewsletter", canalDeTeste, ""},
		{"/newsletter/unfollow", func(h *NewsletterHandlers) http.Handler { return h.Unfollow },
			`{"jid":"` + canalDeTeste + `"}`, "UnfollowNewsletter", canalDeTeste, ""},
		{"/newsletter/mute", func(h *NewsletterHandlers) http.Handler { return h.Mute },
			`{"jid":"` + canalDeTeste + `","mute":true}`, "ToggleNewsletterMute", canalDeTeste, "true"},
		{"/newsletter/messages", func(h *NewsletterHandlers) http.Handler { return h.Messages },
			`{"jid":"` + canalDeTeste + `","count":5,"before":"99"}`, "NewsletterMessages", canalDeTeste, "99"},
		{"/newsletter/updates", func(h *NewsletterHandlers) http.Handler { return h.Updates },
			`{"jid":"` + canalDeTeste + `","count":5,"after":"7"}`, "NewsletterMessageUpdates", canalDeTeste, "7"},
		{"/newsletter/mark-viewed", func(h *NewsletterHandlers) http.Handler { return h.MarkViewed },
			`{"jid":"` + canalDeTeste + `","serverIDs":[1,2]}`, "MarkNewsletterViewed", canalDeTeste, "[1 2]"},
		{"/newsletter/react", func(h *NewsletterHandlers) http.Handler { return h.React },
			`{"jid":"` + canalDeTeste + `","serverID":3,"reaction":"👍","messageID":"m1"}`,
			"SendNewsletterReaction", canalDeTeste, "👍"},
		{"/newsletter/subscribe", func(h *NewsletterHandlers) http.Handler { return h.Subscribe },
			`{"jid":"` + canalDeTeste + `"}`, "SubscribeNewsletterLiveUpdates", canalDeTeste, ""},
	}

	if len(casos) != 11 {
		t.Fatalf("a familia tem 11 operacoes, a tabela tem %d", len(casos))
	}

	for _, c := range casos {
		t.Run(c.rota, func(t *testing.T) {
			nr := &contractsfake.NewsletterReader{}
			rec, _ := ipmServe(t, c.handler(newsletterOps(nr)), http.MethodPost, c.rota, c.corpo,
				func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

			if rec.Code != http.StatusOK {
				t.Fatalf("status %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
			}
			if len(nr.NewsletterCalls) != 1 {
				t.Fatalf("porta chamada %d vez(es), quero 1: %+v", len(nr.NewsletterCalls), nr.NewsletterCalls)
			}
			got := nr.NewsletterCalls[0]
			if got.Method != c.wantMetodo {
				t.Errorf("metodo %q, quero %q", got.Method, c.wantMetodo)
			}
			if got.JID != c.wantJID {
				t.Errorf("JID %q, quero %q", got.JID, c.wantJID)
			}
			if got.Extra != c.wantExtra {
				t.Errorf("extra %q, quero %q", got.Extra, c.wantExtra)
			}
		})
	}
}

// TestNewsletter_IdentificadorEmFaltaE400 trava o mapeamento de estado: uma
// recusa de validacao NAO pode sair como 500. Um 500 diz ao cliente para tentar
// outra vez algo que nunca vai passar.
func TestNewsletter_IdentificadorEmFaltaE400(t *testing.T) {
	nr := &contractsfake.NewsletterReader{}

	rec, _ := ipmServe(t, newsletterOps(nr).Follow, http.MethodPost, "/newsletter/follow", `{}`,
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	if len(nr.NewsletterCalls) != 0 {
		t.Fatalf("pedido sem JID alcancou a porta: %+v", nr.NewsletterCalls)
	}
}

// TestNewsletter_SemAutenticacaoNaoAlcancaAPorta e' o caminho de recusa antes
// do use case.
func TestNewsletter_SemAutenticacaoNaoAlcancaAPorta(t *testing.T) {
	nr := &contractsfake.NewsletterReader{}

	rec, _ := ipmServe(t, newsletterOps(nr).Info, http.MethodPost, "/newsletter/info",
		`{"jid":"`+canalDeTeste+`"}`, nil)

	assertErrorEnvelope(t, rec, http.StatusUnauthorized)
	if len(nr.EnsureSessionCalls) != 0 {
		t.Fatal("requisicao nao autenticada alcancou a porta")
	}
}

// TestNewsletter_SinceMalFormadoE400 cobre a unica conversao do handler que
// pode falhar. Sem este teste, um `since` invalido sairia como zero-value e a
// rota devolveria a janela errada em silencio.
func TestNewsletter_SinceMalFormadoE400(t *testing.T) {
	nr := &contractsfake.NewsletterReader{}

	rec, _ := ipmServe(t, newsletterOps(nr).Updates, http.MethodPost, "/newsletter/updates",
		`{"jid":"`+canalDeTeste+`","since":"ontem"}`,
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	if len(nr.NewsletterCalls) != 0 {
		t.Fatalf("since invalido alcancou a porta: %+v", nr.NewsletterCalls)
	}
}

// TestNewsletter_FalhaDaPortaE500 confirma que o outro lado do mapeamento
// continua a ser 500 — se tudo virasse 400, o teste acima passaria por acidente.
func TestNewsletter_FalhaDaPortaE500(t *testing.T) {
	nr := &contractsfake.NewsletterReader{
		FollowFunc: func(_ context.Context, _ string, _ domain.JID) error { return ipmErrBoom },
	}

	rec, _ := ipmServe(t, newsletterOps(nr).Follow, http.MethodPost, "/newsletter/follow",
		`{"jid":"`+canalDeTeste+`"}`,
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	assertErrorEnvelope(t, rec, http.StatusInternalServerError)
}
