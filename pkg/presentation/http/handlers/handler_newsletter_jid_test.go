package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
)

// ---------------------------------------------------------------------------
// F271 — a jid that is PRESENT but impossible must come out as 400 with a code
// that says WHAT is wrong, not as 500 newsletter_failed.
//
// This is the boundary half of the test: the use case tests prove the refusal
// is classified as validation, and this one proves the classification actually
// reaches the wire as a status and an error.code the caller can branch on.
// Both are needed — the mapping from category to status lives in RespondJSON,
// not in the use case, and has been wrong before.
//
// NOTA SOBRE O DUBLE (ARMADILHAS nº1): o contractsfake.NewsletterReader aceita
// QUALQUER jid, e por isso, sem a correccao, estas rotas respondem 200 no teste
// onde a producao respondia 500. O teste continua a morder porque afirma 400 —
// tanto o 200 do duble como o 500 do adaptador real falham contra ele. O que
// nao se pode fazer aqui e' afirmar "nao e' 500": isso passaria com o duble.
// ---------------------------------------------------------------------------

// jidsMedidos são os valores exactos medidos em campo em 2026-08-26 contra
// POST /newsletter/info, cada um devolvendo 500 newsletter_failed.
var jidsMedidos = []string{
	"   ",
	"nao-e-jid",
	"554192421234@s.whatsapp.net",
}

// errorCode reads error.code from the ADR-002 envelope.
func errorCode(t *testing.T, body []byte) string {
	t.Helper()
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("resposta nao e' o envelope do ADR-002: %v (corpo: %s)", err, string(body))
	}
	var detail struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(env.Error, &detail); err != nil {
		t.Fatalf("error nao e' um objecto com code: %v (corpo: %s)", err, string(body))
	}
	return detail.Code
}

func TestNewsletter_JIDMalformadoE400(t *testing.T) {
	for _, jid := range jidsMedidos {
		t.Run(jid, func(t *testing.T) {
			nr := &contractsfake.NewsletterReader{}

			rec, _ := ipmServe(t, newsletterOps(nr).Info, http.MethodPost, "/newsletter/info",
				`{"jid":`+strconvQuote(jid)+`}`,
				func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

			assertErrorEnvelope(t, rec, http.StatusBadRequest)
			if code := errorCode(t, rec.Body.Bytes()); code != "invalid_newsletter_jid" {
				t.Fatalf("error.code = %q, quero %q (corpo: %s)", code, "invalid_newsletter_jid", rec.Body.String())
			}
			if len(nr.NewsletterCalls) != 0 {
				t.Fatalf("jid malformado alcancou a porta: %+v", nr.NewsletterCalls)
			}
		})
	}
}

// TestNewsletter_JIDAusenteContinuaMissingJID: a ausencia ja' era 400 e tem de
// continuar a dizer missing_jid. Trocar um codigo errado por outro nao seria
// correccao.
func TestNewsletter_JIDAusenteContinuaMissingJID(t *testing.T) {
	nr := &contractsfake.NewsletterReader{}

	rec, _ := ipmServe(t, newsletterOps(nr).Info, http.MethodPost, "/newsletter/info", `{}`,
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	if code := errorCode(t, rec.Body.Bytes()); code != "missing_jid" {
		t.Fatalf("error.code = %q, quero %q (corpo: %s)", code, "missing_jid", rec.Body.String())
	}
}

// TestNewsletter_JIDValidoContinuaA200 e' o controlo de sucesso desta fronteira.
func TestNewsletter_JIDValidoContinuaA200(t *testing.T) {
	nr := &contractsfake.NewsletterReader{}

	rec, _ := ipmServe(t, newsletterOps(nr).Info, http.MethodPost, "/newsletter/info",
		`{"jid":"`+canalDeTeste+`"}`,
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if len(nr.NewsletterCalls) != 1 {
		t.Fatalf("porta chamada %d vez(es), quero 1", len(nr.NewsletterCalls))
	}
}

// strconvQuote produz o literal JSON do valor, para que o jid com espacos
// atravesse o corpo exactamente como foi medido.
func strconvQuote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}
