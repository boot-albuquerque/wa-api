// Package http contém os handlers HTTP e helpers de resposta do disparazaap-wa-api.
package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"wa-api/pkg/domain/apperr"

	"github.com/rs/zerolog/log"
)

// Canonical error codes for responses that have no apperr taxonomy behind
// them. They are snake_case because every key and every enumerated value on
// this API's wire is snake_case (docs/HTTP-DTO-CONVENTIONS.md).
const (
	codeInvalidRequest      = "invalid_request"
	codeUnauthorized        = "unauthorized"
	codeForbidden           = "forbidden"
	codeNotFound            = "not_found"
	codeMethodNotAllowed    = "method_not_allowed"
	codeConflict            = "conflict"
	codeUnprocessableEntity = "unprocessable_entity"
	codeRateLimited         = "rate_limited"
	codeNotImplemented      = "not_implemented"
	codeBadGateway          = "bad_gateway"
	codeServiceUnavailable  = "service_unavailable"
	codeGatewayTimeout      = "gateway_timeout"
	codeInternalError       = "internal_error"
)

// ErrorBody is the shape of the `error` key. ALWAYS an object, for every
// status and for every kind of error — typed or not.
//
// It is a declared type and not a map so that the shape is checked at compile
// time: a call site cannot forget `code`, and cannot invent a third key.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// genericError maps a status code to the safe, canonical error body served
// when no apperr.AppError reached the boundary.
//
// The message is pt-BR because it is written for the human reading a failed
// request; the code is what a client branches on.
func genericError(statusCode int) ErrorBody {
	switch statusCode {
	case http.StatusBadRequest:
		return ErrorBody{codeInvalidRequest, "Requisição inválida."}
	case http.StatusUnauthorized:
		return ErrorBody{codeUnauthorized, "Credenciais ausentes ou inválidas."}
	case http.StatusForbidden:
		return ErrorBody{codeForbidden, "Operação não permitida."}
	case http.StatusNotFound:
		return ErrorBody{codeNotFound, "Recurso não encontrado."}
	case http.StatusMethodNotAllowed:
		// Added when the router's own 405 branch stopped answering in plain
		// text. Without this case the branch below would answer 405 with
		// {"code":"internal_error"}, which reads as "we broke" for a request
		// whose only fault is the verb.
		return ErrorBody{codeMethodNotAllowed, "Método HTTP não permitido para este recurso."}
	case http.StatusConflict:
		return ErrorBody{codeConflict, "A requisição não pode ser atendida no estado atual."}
	case http.StatusUnprocessableEntity:
		return ErrorBody{codeUnprocessableEntity, "A requisição foi recusada pelo destino."}
	case http.StatusTooManyRequests:
		return ErrorBody{codeRateLimited, "Limite de requisições excedido."}
	case http.StatusNotImplemented:
		return ErrorBody{codeNotImplemented, "Recurso não disponível nesta configuração."}
	case http.StatusBadGateway:
		return ErrorBody{codeBadGateway, "Falha ao contactar um serviço externo."}
	case http.StatusServiceUnavailable:
		return ErrorBody{codeServiceUnavailable, "Serviço temporariamente indisponível."}
	case http.StatusGatewayTimeout:
		return ErrorBody{codeGatewayTimeout, "Tempo esgotado ao contactar um serviço externo."}
	default:
		// Includes 500 and any status this function does not enumerate. The
		// fallback is deliberately the most conservative body: a status we did
		// not plan for is, by definition, a case we cannot describe safely.
		return ErrorBody{codeInternalError, "Ocorreu um erro interno."}
	}
}

// RespondJSON escreve a resposta JSON no envelope canónico da API.
//
//	sucesso: {"success": true,  "code": <status>, "data":  <data>}
//	erro:    {"success": false, "code": <status>, "error": {"code": ..., "message": ...}}
//
// O envelope é PERMANENTE, não uma janela de depreciação. `success` é o campo
// pelo qual os clientes distinguem os dois ramos sem ler o status, e `error` é
// SEMPRE um objecto — nunca texto — para que ramificar sobre `error.code` seja
// possível em todos os status, tipados ou não.
//
// Quando err é um *apperr.AppError, o statusCode passado é IGNORADO e o status
// real vem de err.Category.HTTPStatus(): é o call site que historicamente
// errou (handlers passando 500 para qualquer erro de use case), não a
// taxonomia. err.Message é seguro para devolver por construção — ver
// pkg/domain/apperr.
//
// Quando err é não-tipado, o statusCode prevalece e o corpo vem de
// genericError(statusCode). NUNCA err.Error(): este ramo apanha também o que
// `reportPanic` entrega, que é o valor cru do panic — endereços, tipos
// internos e, num panic com dado do pedido, o próprio dado. O erro completo já
// foi para o log, que é onde ele serve.
func RespondJSON(w http.ResponseWriter, statusCode int, data interface{}, err error) {
	envelope := make(map[string]interface{})

	if err != nil {
		var appErr *apperr.AppError
		if errors.As(err, &appErr) {
			statusCode = appErr.Category.HTTPStatus()
			envelope["error"] = ErrorBody{Code: appErr.Code, Message: appErr.Message}
		} else {
			// Travado por TestRespondJSONNaoVazaDetalheDeErroInterno, com par
			// de controlo — segurança que apagasse a informação legítima das
			// recusas de validação não seria segurança, seria cegueira.
			envelope["error"] = genericError(statusCode)
		}
		envelope["code"] = statusCode
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
		_, _ = w.Write([]byte(`{"code":500,"error":{"code":"internal_error","message":"Ocorreu um erro interno."},"success":false}`))
		return
	}
	_, _ = w.Write(respBytes)
	_, _ = w.Write([]byte("\n"))
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
