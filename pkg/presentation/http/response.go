// Package http contém os handlers HTTP e helpers de resposta do disparazaap-wa-api.
package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"wa-api/pkg/domain/apperr"

	"github.com/rs/zerolog/log"
)

// RespondJSON escreve uma resposta JSON com envelope compatível com s.Respond() do upstream.
// Formato: {"code": <statusCode>, "data": <data>, "success": true} em caso
// de sucesso, ou {"code": <statusCode>, "error": <erro>, "success": false}
// em caso de erro (ADR-002).
//
// Quando err é um *apperr.AppError, o statusCode passado é IGNORADO e o
// status real é derivado de err.Category.HTTPStatus() — a taxonomia da
// Fase 3 tem prioridade sobre o que o call site escreveu, porque é o call
// site que historicamente errou (ex: handler_message.go passando
// StatusInternalServerError para qualquer erro de use case). "error" vira
// um objeto {"code": <apperr.Code>, "message": <apperr.Message>} — o
// error.code estável e legível por máquina que o ADR-002 pede, vindo da
// taxonomia. err.Message é seguro para retornar ao cliente por construção
// (ver pkg/domain/apperr).
//
// Quando err é não-tipado (ou nil), o statusCode passado prevalece, e
// "error" continua sendo a string genérica http.StatusText(statusCode) em
// minúsculas ("unauthorized", "bad request", "internal server error", ...)
// — nunca err.Error(), para que nenhum detalhe interno ou PII trafegue
// para o cliente. Usecases e handlers que precisam de detalhes devem
// logar internamente via appport.Logger.
//
// O campo "success" é a janela de depreciação do ADR-002: o envelope
// legado de middleware/auth.go (retirado nesta mesma fase) emitia
// "success" em ambos os ramos, e por uma release este envelope continua
// emitindo o mesmo campo — sem isso, um consumidor das 4 rotas /webhook
// que checa `success === false` para detectar erro (o caminho de sucesso
// dessas rotas não tem golden, então essa classe de consumidor não seria
// pega por nenhum teste) passaria a ler `undefined`, que é falsy, e
// trataria erro como sucesso silenciosamente. Remover "success" é
// follow-up nomeado do ADR-002, não parte desta fase.
func RespondJSON(w http.ResponseWriter, statusCode int, data interface{}, err error) {
	envelope := make(map[string]interface{})

	if err != nil {
		var appErr *apperr.AppError
		if errors.As(err, &appErr) {
			statusCode = appErr.Category.HTTPStatus()
			envelope["code"] = statusCode
			envelope["error"] = map[string]interface{}{
				"code":    appErr.Code,
				"message": appErr.Message,
			}
		} else {
			envelope["code"] = statusCode
			envelope["error"] = genericErrorMessage(statusCode)
		}
		envelope["success"] = false
	} else {
		envelope["code"] = statusCode
		envelope["data"] = data
		envelope["success"] = true
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	respBytes, marshalErr := json.Marshal(envelope)
	if marshalErr != nil {
		// Fallback: erro de serialização — retornar erro genérico.
		// O statusCode já foi escrito acima, então este WriteHeader é um
		// no-op registrado pelo net/http; o log abaixo é a única evidência
		// de que o corpo entregue não é o corpo pretendido.
		log.Error().Err(marshalErr).
			Int("status", statusCode).
			Bool("had_error", err != nil).
			Msg("failed to marshal JSON response envelope; falling back to generic body")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":500,"error":"internal server error"}`))
		return
	}
	_, _ = w.Write(respBytes)
	_, _ = w.Write([]byte("\n"))
}

// genericErrorMessage returns the safe, generic error text for a status
// code that has no apperr taxonomy behind it — http.StatusText lowercased
// ("unauthorized", "bad request", ...), falling back to "internal server
// error" for a status code net/http doesn't recognize.
func genericErrorMessage(statusCode int) string {
	if text := http.StatusText(statusCode); text != "" {
		return strings.ToLower(text)
	}
	return "internal server error"
}

// Find searches for a value in a string slice.
// Returns true if the value is found, false otherwise.
func Find(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
