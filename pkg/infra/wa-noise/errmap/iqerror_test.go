package errmap_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	wanoise "wa-api/internal/wa-noise"
	"wa-api/pkg/domain/apperr"
	"wa-api/pkg/infra/wa-noise/errmap"
)

// O caso MEDIDO: POST /user/block com um número bem formado que não tem conta
// no WhatsApp. O servidor respondeu 400 bad-request e nós respondíamos 500
// "internal server error" — "avaria nossa, tente outra vez" — para um erro que
// nunca muda de resposta (F204).
//
// Os valores são os observados em campo a 2026-08-21, não uma aproximação:
//
//	log: ERR Failed to block user error="info query returned status 400: bad-request"
//	     jid=5511000000001@s.whatsapp.net
//	HTTP 500 {"code":500,"error":"internal server error"}
func TestClassifyIQ_ORecusadoMedidoDeixaDeSer500(t *testing.T) {
	medido := &wanoise.IQError{Code: 400, Text: "bad-request"}

	got := errmap.ClassifyIQ(fmt.Errorf("failed to block user: %w", medido))

	var app *apperr.AppError
	if !errors.As(got, &app) {
		t.Fatalf("a recusa de montante não virou AppError: %T (%v)", got, got)
	}
	if s := app.Category.HTTPStatus(); s != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, quero 422: um 400 de montante não é avaria nossa (500) "+
			"nem payload errado (400) — o número está bem formado", s)
	}
	if app.Code != errmap.CodeUpstreamRejected {
		t.Errorf("code = %q, quero %q", app.Code, errmap.CodeUpstreamRejected)
	}
	// A mensagem vai para o CLIENTE: não pode levar o JID recusado nem o texto
	// interno da biblioteca.
	for _, proibido := range []string{"5511000000001", "info query", "bad-request"} {
		if contains(app.Message, proibido) {
			t.Errorf("a mensagem ao cliente contém %q: %q", proibido, app.Message)
		}
	}
	// O erro original continua alcançável para o log.
	if !errors.Is(got, medido) {
		t.Error("o erro de montante deixou de ser alcançável por errors.Is: o log perde a causa")
	}
}

// A tabela cobre as treze sentinelas mais o caso desconhecido. O que ela trava
// não é o mapa em si — é a DIFERENÇA entre recusa (4xx, o cliente decide o que
// fazer) e falha (5xx, retry é a resposta honesta).
func TestClassifyIQ_CadaRecusaTemOSeuStatus(t *testing.T) {
	casos := []struct {
		code      int
		text      string
		querStatu int
		retentar  bool
	}{
		{400, "bad-request", http.StatusUnprocessableEntity, false},
		{401, "not-authorized", http.StatusUnauthorized, false},
		{403, "forbidden", http.StatusForbidden, false},
		{404, "item-not-found", http.StatusNotFound, false},
		{405, "not-allowed", http.StatusUnprocessableEntity, false},
		{406, "not-acceptable", http.StatusUnprocessableEntity, false},
		{410, "gone", http.StatusNotFound, false},
		{419, "resource-limit", http.StatusTooManyRequests, true},
		{423, "locked", http.StatusUnprocessableEntity, false},
		{429, "rate-overlimit", http.StatusTooManyRequests, true},
		{500, "internal-server-error", http.StatusInternalServerError, false},
		{503, "service-unavailable", http.StatusInternalServerError, false},
		{530, "partial-server-error", http.StatusInternalServerError, false},
		{418, "teapot-que-o-servidor-invente", http.StatusUnprocessableEntity, false},
	}
	for _, c := range casos {
		t.Run(fmt.Sprintf("%d_%s", c.code, c.text), func(t *testing.T) {
			got := errmap.ClassifyIQ(&wanoise.IQError{Code: c.code, Text: c.text})
			var app *apperr.AppError
			if !errors.As(got, &app) {
				t.Fatalf("não virou AppError: %T", got)
			}
			if s := app.Category.HTTPStatus(); s != c.querStatu {
				t.Errorf("status = %d, quero %d", s, c.querStatu)
			}
			if app.Retryable != c.retentar {
				t.Errorf("retryable = %v, quero %v: só o estrangulamento (419/429) "+
					"deve ser repetido sem mudar nada", app.Retryable, c.retentar)
			}
		})
	}
}

// O limite do tradutor, e é ele que impede a correção de virar outra mentira:
// o que NÃO é recusa de info query tem de passar intacto. Um tradutor que
// embrulhasse tudo transformaria falha de transporte e bug NOSSO em recusa de
// montante — a mesma classe de mentira, na direção oposta.
func TestClassifyIQ_NaoTocaNoQueNaoEhRecusa(t *testing.T) {
	if got := errmap.ClassifyIQ(nil); got != nil {
		t.Errorf("nil virou %v", got)
	}
	outro := errors.New("dial tcp: connection refused")
	if got := errmap.ClassifyIQ(outro); got != outro {
		t.Errorf("erro alheio foi trocado: %v", got)
	}
	// Timeout de info query NÃO é recusa: o servidor não disse nada. É a F209.
	semResposta := wanoise.ErrIQTimedOut
	got := errmap.ClassifyIQ(semResposta)
	var app *apperr.AppError
	if errors.As(got, &app) {
		t.Errorf("o timeout foi classificado como recusa (%s): o servidor não "+
			"recusou, não respondeu — são coisas diferentes (F209)", app.Category)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
