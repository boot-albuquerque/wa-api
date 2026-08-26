package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/notification"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
	"wa-api/pkg/infra/wa-noise/errmap"
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
		{"/newsletter/demote", func(h *NewsletterHandlers) http.Handler { return h.Demote },
			`{"jid":"` + canalDeTeste + `","userJID":"5516900000000@s.whatsapp.net"}`,
			"DemoteNewsletterAdmin", canalDeTeste, "5516900000000@s.whatsapp.net"},
		{"/newsletter/change-owner", func(h *NewsletterHandlers) http.Handler { return h.ChangeOwner },
			`{"jid":"` + canalDeTeste + `","userJID":"5516900000000@s.whatsapp.net"}`,
			"ChangeNewsletterOwner", canalDeTeste, "5516900000000@s.whatsapp.net"},
		{"/newsletter/delete", func(h *NewsletterHandlers) http.Handler { return h.Delete },
			`{"jid":"` + canalDeTeste + `","confirmJID":"` + canalDeTeste + `"}`,
			"DeleteNewsletter", canalDeTeste, ""},
		{"/newsletter/admin-invite", func(h *NewsletterHandlers) http.Handler { return h.AdminInvite },
			`{"jid":"` + canalDeTeste + `","userJID":"5516900000000@s.whatsapp.net"}`,
			"CreateNewsletterAdminInvite", canalDeTeste, "5516900000000@s.whatsapp.net"},
		{"/newsletter/admin-invite/accept", func(h *NewsletterHandlers) http.Handler { return h.AdminInviteAccept },
			`{"jid":"` + canalDeTeste + `"}`,
			"AcceptNewsletterAdminInvite", canalDeTeste, ""},
		{"/newsletter/admin-invite/revoke", func(h *NewsletterHandlers) http.Handler { return h.AdminInviteRevoke },
			`{"jid":"` + canalDeTeste + `","userJID":"5516900000000@s.whatsapp.net"}`,
			"RevokeNewsletterAdminInvite", canalDeTeste, "5516900000000@s.whatsapp.net"},
	}

	if len(casos) != 17 {
		t.Fatalf("a familia tem 17 operacoes, a tabela tem %d", len(casos))
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

// ---------------------------------------------------------------------------
// F258 — newsletter create must NOT produce data.data in the response.
// Before the fix, the handler passed rsp (a NewsletterResult with a json
// "data" field) as the data argument to RespondJSON, which already wraps
// in {"code":..., "data":..., "success":...}. Result: data.data.
// ---------------------------------------------------------------------------

func TestNewsletter_Create_NoDoubleWrap(t *testing.T) {
	nr := &contractsfake.NewsletterReader{}

	rec, _ := ipmServe(t, newsletterOps(nr).Create, http.MethodPost, "/newsletter/create",
		`{"name":"Canal X","description":"d"}`,
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}

	dataBytes, ok := raw["data"]
	if !ok {
		return
	}

	var dataObj map[string]json.RawMessage
	if err := json.Unmarshal(dataBytes, &dataObj); err != nil {
		return
	}
	if _, hasNestedData := dataObj["data"]; hasNestedData {
		t.Fatalf("F258 regression: response has data.data (double-wrapped): %s", rec.Body.String())
	}
	if _, hasDuration := dataObj["duration_seconds"]; hasDuration {
		t.Fatalf("F258 regression: response has data.duration_seconds (NewsletterResult leaked): %s", rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// F233 — GraphQL 405 from the WhatsApp server on unfollow must be 403
// with code "newsletter_admin_cannot_unfollow", not 500.
// ---------------------------------------------------------------------------

// newsletterAdminRefusalError builds the *apperr.AppError that
// errmap.ClassifyNewsletter produces for the measured 405 case.
//
// Source: errmap.ClassifyNewsletter, which wraps the GraphQLError chain.
func newsletterAdminRefusalError() *apperr.AppError {
	return apperr.New(
		errmap.CodeNewsletterAdminCannotUnfollow,
		apperr.CategoryForbidden,
		"channel admins cannot unfollow their own channel; dismiss yourself as admin first",
		false,
		nil,
	)
}

// Requirement 1: GraphQL 405 on unfollow → 403 with the named code.
func TestNewsletter_GraphQL405_Returns403WithCode(t *testing.T) {
	nr := &contractsfake.NewsletterReader{
		UnfollowFunc: func(_ context.Context, _ string, _ domain.JID) error {
			return newsletterAdminRefusalError()
		},
	}

	rec, _ := ipmServe(t, newsletterOps(nr).Unfollow, http.MethodPost, "/newsletter/unfollow",
		`{"jid":"`+canalDeTeste+`"}`,
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	assertErrorEnvelope(t, rec, http.StatusForbidden)

	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal = %v", err)
	}
	if env.Error.Code != errmap.CodeNewsletterAdminCannotUnfollow {
		t.Errorf("error.code = %q, want %q", env.Error.Code, errmap.CodeNewsletterAdminCannotUnfollow)
	}
	if env.Error.Message == "" {
		t.Error("error.message is empty: the client needs to know what to do")
	}
}

// Requirement 2: non-405 GraphQL error → still 500.
func TestNewsletter_GraphQLNon405_Returns500(t *testing.T) {
	gqlErr := fmt.Errorf("graphql error: %w", types.GraphQLErrors{{
		Extensions: types.GraphQLErrorExtensions{
			ErrorCode: 500,
			Severity:  "CRITICAL",
		},
		Message: "Internal Server Error",
	}})

	nr := &contractsfake.NewsletterReader{
		UnfollowFunc: func(_ context.Context, _ string, _ domain.JID) error {
			return gqlErr
		},
	}

	rec, _ := ipmServe(t, newsletterOps(nr).Unfollow, http.MethodPost, "/newsletter/unfollow",
		`{"jid":"`+canalDeTeste+`"}`,
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	assertErrorEnvelope(t, rec, http.StatusInternalServerError)
}

// Requirement 3: non-GraphQL error (network, missing session) → still 500.
func TestNewsletter_NonGraphQLError_Returns500(t *testing.T) {
	nr := &contractsfake.NewsletterReader{
		UnfollowFunc: func(_ context.Context, _ string, _ domain.JID) error {
			return errors.New("dial tcp: connection refused")
		},
	}

	rec, _ := ipmServe(t, newsletterOps(nr).Unfollow, http.MethodPost, "/newsletter/unfollow",
		`{"jid":"`+canalDeTeste+`"}`,
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	assertErrorEnvelope(t, rec, http.StatusInternalServerError)
}

// ---------------------------------------------------------------------------
// F233b — demote, change_owner, delete validation and success paths
// ---------------------------------------------------------------------------

func TestNewsletter_DemoteSemUserJID_E400(t *testing.T) {
	nr := &contractsfake.NewsletterReader{}
	rec, _ := ipmServe(t, newsletterOps(nr).Demote, http.MethodPost, "/newsletter/demote",
		`{"jid":"`+canalDeTeste+`"}`,
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	if len(nr.NewsletterCalls) != 0 {
		t.Fatalf("demote without userJID reached the port: %+v", nr.NewsletterCalls)
	}
}

func TestNewsletter_ChangeOwnerSemUserJID_E400(t *testing.T) {
	nr := &contractsfake.NewsletterReader{}
	rec, _ := ipmServe(t, newsletterOps(nr).ChangeOwner, http.MethodPost, "/newsletter/change-owner",
		`{"jid":"`+canalDeTeste+`"}`,
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	if len(nr.NewsletterCalls) != 0 {
		t.Fatalf("change_owner without userJID reached the port: %+v", nr.NewsletterCalls)
	}
}

func TestNewsletter_DeleteSemConfirmJID_E400(t *testing.T) {
	nr := &contractsfake.NewsletterReader{}
	rec, _ := ipmServe(t, newsletterOps(nr).Delete, http.MethodPost, "/newsletter/delete",
		`{"jid":"`+canalDeTeste+`"}`,
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	if len(nr.NewsletterCalls) != 0 {
		t.Fatalf("delete without confirmJID reached the port: %+v", nr.NewsletterCalls)
	}
}

func TestNewsletter_DeleteConfirmJIDMismatch_E400(t *testing.T) {
	nr := &contractsfake.NewsletterReader{}
	rec, _ := ipmServe(t, newsletterOps(nr).Delete, http.MethodPost, "/newsletter/delete",
		`{"jid":"`+canalDeTeste+`","confirmJID":"999999999@newsletter"}`,
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	if len(nr.NewsletterCalls) != 0 {
		t.Fatalf("delete with mismatched confirmJID reached the port: %+v", nr.NewsletterCalls)
	}
}

func TestNewsletter_DemoteFalhaDaPortaE500(t *testing.T) {
	nr := &contractsfake.NewsletterReader{
		DemoteFunc: func(_ context.Context, _ string, _, _ domain.JID) error {
			return errors.New("server down")
		},
	}
	rec, _ := ipmServe(t, newsletterOps(nr).Demote, http.MethodPost, "/newsletter/demote",
		`{"jid":"`+canalDeTeste+`","userJID":"5516900000000@s.whatsapp.net"}`,
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	assertErrorEnvelope(t, rec, http.StatusInternalServerError)
}

func TestNewsletter_ChangeOwnerFalhaDaPortaE500(t *testing.T) {
	nr := &contractsfake.NewsletterReader{
		ChangeOwnerFunc: func(_ context.Context, _ string, _, _ domain.JID) error {
			return errors.New("server down")
		},
	}
	rec, _ := ipmServe(t, newsletterOps(nr).ChangeOwner, http.MethodPost, "/newsletter/change-owner",
		`{"jid":"`+canalDeTeste+`","userJID":"5516900000000@s.whatsapp.net"}`,
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	assertErrorEnvelope(t, rec, http.StatusInternalServerError)
}

func TestNewsletter_DeleteFalhaDaPortaE500(t *testing.T) {
	nr := &contractsfake.NewsletterReader{
		DeleteFunc: func(_ context.Context, _ string, _ domain.JID) error {
			return errors.New("server down")
		},
	}
	rec, _ := ipmServe(t, newsletterOps(nr).Delete, http.MethodPost, "/newsletter/delete",
		`{"jid":"`+canalDeTeste+`","confirmJID":"`+canalDeTeste+`"}`,
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	assertErrorEnvelope(t, rec, http.StatusInternalServerError)
}

// ---------------------------------------------------------------------------
// F233(b) — admin invite validation and error paths
// ---------------------------------------------------------------------------

func TestNewsletter_AdminInviteSemUserJID_E400(t *testing.T) {
	nr := &contractsfake.NewsletterReader{}
	rec, _ := ipmServe(t, newsletterOps(nr).AdminInvite, http.MethodPost, "/newsletter/admin-invite",
		`{"jid":"`+canalDeTeste+`"}`,
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	if len(nr.NewsletterCalls) != 0 {
		t.Fatalf("admin_invite without userJID reached the port: %+v", nr.NewsletterCalls)
	}
}

func TestNewsletter_AdminInviteRevokeSemUserJID_E400(t *testing.T) {
	nr := &contractsfake.NewsletterReader{}
	rec, _ := ipmServe(t, newsletterOps(nr).AdminInviteRevoke, http.MethodPost, "/newsletter/admin-invite/revoke",
		`{"jid":"`+canalDeTeste+`"}`,
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	if len(nr.NewsletterCalls) != 0 {
		t.Fatalf("admin_invite_revoke without userJID reached the port: %+v", nr.NewsletterCalls)
	}
}

func TestNewsletter_AdminInviteAcceptSemJID_E400(t *testing.T) {
	nr := &contractsfake.NewsletterReader{}
	rec, _ := ipmServe(t, newsletterOps(nr).AdminInviteAccept, http.MethodPost, "/newsletter/admin-invite/accept",
		`{}`,
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	if len(nr.NewsletterCalls) != 0 {
		t.Fatalf("admin_invite_accept without jid reached the port: %+v", nr.NewsletterCalls)
	}
}

func TestNewsletter_AdminInviteFalhaDaPortaE500(t *testing.T) {
	nr := &contractsfake.NewsletterReader{
		CreateAdminInviteFunc: func(_ context.Context, _ string, _, _ domain.JID) error {
			return errors.New("server down")
		},
	}
	rec, _ := ipmServe(t, newsletterOps(nr).AdminInvite, http.MethodPost, "/newsletter/admin-invite",
		`{"jid":"`+canalDeTeste+`","userJID":"5516900000000@s.whatsapp.net"}`,
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	assertErrorEnvelope(t, rec, http.StatusInternalServerError)
}

func TestNewsletter_AdminInviteAcceptFalhaDaPortaE500(t *testing.T) {
	nr := &contractsfake.NewsletterReader{
		AcceptAdminInviteFunc: func(_ context.Context, _ string, _ domain.JID) error {
			return errors.New("server down")
		},
	}
	rec, _ := ipmServe(t, newsletterOps(nr).AdminInviteAccept, http.MethodPost, "/newsletter/admin-invite/accept",
		`{"jid":"`+canalDeTeste+`"}`,
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	assertErrorEnvelope(t, rec, http.StatusInternalServerError)
}

func TestNewsletter_AdminInviteRevokeFalhaDaPortaE500(t *testing.T) {
	nr := &contractsfake.NewsletterReader{
		RevokeAdminInviteFunc: func(_ context.Context, _ string, _, _ domain.JID) error {
			return errors.New("server down")
		},
	}
	rec, _ := ipmServe(t, newsletterOps(nr).AdminInviteRevoke, http.MethodPost, "/newsletter/admin-invite/revoke",
		`{"jid":"`+canalDeTeste+`","userJID":"5516900000000@s.whatsapp.net"}`,
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	assertErrorEnvelope(t, rec, http.StatusInternalServerError)
}
