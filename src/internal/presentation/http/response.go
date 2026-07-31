// Package http contém os handlers HTTP e helpers de resposta do disparazaap-wuzapi.
package http

import (
	"encoding/json"
	"net/http"
)

// RespondJSON escreve uma resposta JSON com envelope compatível com s.Respond() do upstream.
// Formato: {"code": <statusCode>, "data": <data>} em caso de sucesso,
// ou {"code": <statusCode>, "error": "<mensagem genérica>"} em caso de erro.
//
// O campo "error" NUNCA contém detalhes internos ou PII — apenas mensagens genéricas.
// Usecases e handlers que precisam de detalhes devem logar internamente via port.Logger.
func RespondJSON(w http.ResponseWriter, statusCode int, data interface{}, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	envelope := make(map[string]interface{})
	if err != nil {
		envelope["code"] = statusCode
		envelope["error"] = "internal server error"
	} else {
		envelope["code"] = statusCode
		envelope["data"] = data
	}

	respBytes, marshalErr := json.Marshal(envelope)
	if marshalErr != nil {
		// Fallback: erro de serialização — retornar erro genérico
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":500,"error":"internal server error"}`))
		return
	}
	_, _ = w.Write(respBytes)
	_, _ = w.Write([]byte("\n"))
}

// RespondError é um helper de conveniência para respostas de erro.
func RespondError(w http.ResponseWriter, statusCode int, message string) {
	RespondJSON(w, statusCode, nil, &simpleError{msg: message})
}

type simpleError struct {
	msg string
}

func (e *simpleError) Error() string {
	return e.msg
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
