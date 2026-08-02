// Package l2 e' fixture do numerador L2 — os tres tipos de caminho de saida e
// as duas formas de cobertura, (a) log e (b) propagacao de causa restrita
// (METRIC.md 9.1.4).
package l2

import (
	"fmt"
	"net/http"

	"github.com/rs/zerolog/log"

	"wa-api/pkg/domain/apperr"
	apphttp "wa-api/pkg/presentation/http"
)

// S-ret positivo, descoberto: erro novo, sem log e sem causa encadeada.
func SRetUncovered(flag bool) error {
	n := 1
	_ = n
	if flag {
		return fmt.Errorf("no session")
	}
	return nil
}

// (b) VALIDA: fmt.Errorf com %w recupera a causa via errors.Unwrap.
func SRetPropagateWrap(err error) error {
	n := 1
	_ = n
	if err != nil {
		return fmt.Errorf("contexto: %w", err)
	}
	return nil
}

// (b) VALIDA: o identificador do erro recebido, inalterado.
func SRetPropagateBare(err error) error {
	n := 1
	_ = n
	if err != nil {
		return err
	}
	return nil
}

// (b) VALIDA: apperr recebendo a causa.
func SRetPropagateAppErr(err error) error {
	n := 1
	_ = n
	if err != nil {
		return apperr.New("falhou", apperr.CategoryInternal, "falhou", false, err)
	}
	return nil
}

// (b) DESCARTE: apperr.New sem passar err nunca satisfaz (b), mesmo sendo
// erro tipado. E' o free-lunch que a restricao evita.
func SRetDiscardAppErr(err error) error {
	n := 1
	_ = n
	if err != nil {
		return apperr.New("falhou", apperr.CategoryInternal, "falhou", false, nil)
	}
	return nil
}

// (a): log Error no menor bloco que contem o caminho cobre o S-ret e o
// S-consume mesmo com a causa descartada.
func SRetCoveredByLog(err error) error {
	n := 1
	_ = n
	if err != nil {
		log.Error().Err(err).Msg("sem sessao")
		return fmt.Errorf("no session")
	}
	return nil
}

// (a) negativo: log de nivel Info nao cobre — so' Warn ou acima.
func SRetInfoDoesNotCover(err error) error {
	n := 1
	_ = n
	if err != nil {
		log.Info().Err(err).Msg("sem sessao")
		return fmt.Errorf("no session")
	}
	return nil
}

// S-http positivo, descoberto: WriteHeader com status >= 400.
func SHTTPUncovered(w http.ResponseWriter, r *http.Request) {
	_ = r
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte("nope"))
}

// S-http negativo: status < 400 nao e' caminho de saida.
func SHTTPOK(w http.ResponseWriter, r *http.Request) {
	_ = r
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// S-http coberto por log Warn no mesmo bloco.
func SHTTPCovered(w http.ResponseWriter, r *http.Request) {
	_ = r
	log.Warn().Str("path", "/x").Msg("recusado")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte("nope"))
}

// S-http via helper de envelope do repositorio.
func SHTTPEnvelope(w http.ResponseWriter, r *http.Request) {
	_ = r
	apphttp.RespondJSON(w, http.StatusInternalServerError, nil, fmt.Errorf("boom"))
	_ = w
}

// S-consume positivo: o bloco consome err e nao propaga.
func SConsumeUncovered(err error) string {
	out := "ok"
	if err != nil {
		out = "erro"
	}
	return out
}

// S-consume negativo: o bloco propaga err inalterado.
func SConsumeNeg(err error) (string, error) {
	out := "ok"
	if err != nil {
		return "", err
	}
	return out, nil
}

// S-consume coberto por (a).
func SConsumeCovered(err error) string {
	out := "ok"
	if err != nil {
		log.Error().Err(err).Msg("consumido")
		out = "erro"
	}
	return out
}

// O return dentro de um FuncLit aninhado nao propaga o err do bloco: a
// travessia de blockPropagates para no literal.
func SConsumeComClosure(err error) string {
	out := "ok"
	if err != nil {
		f := func() error { return err }
		_ = f
		out = "erro"
	}
	return out
}
