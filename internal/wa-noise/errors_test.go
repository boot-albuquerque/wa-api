// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
)

// errors.go e' quase todo declaracao, mas os quatro tipos com Is/Unwrap
// proprios definem como o resto do fork — e os consumidores em pkg/infra —
// classificam falhas. Um Is errado faz um errors.Is casar demais (tratar
// "grupo nao existe" como "sem permissao") ou de menos (perder o retry).

// --- IQError ---

func iqNode(code, text string) *waBinary.Node {
	return &waBinary.Node{
		Tag: "iq",
		Content: []waBinary.Node{{
			Tag:   "error",
			Attrs: waBinary.Attrs{"code": code, "text": text},
		}},
	}
}

// Os sentinelas exportados sao comparados por (code, text). Trocar um por outro
// muda o tratamento de erro de quem chama.
func TestIQErrorIsMatchesByCodeAndText(t *testing.T) {
	cases := []struct {
		name     string
		code     string
		text     string
		sentinel error
	}{
		{name: "400", code: "400", text: "bad-request", sentinel: ErrIQBadRequest},
		{name: "401", code: "401", text: "not-authorized", sentinel: ErrIQNotAuthorized},
		{name: "403", code: "403", text: "forbidden", sentinel: ErrIQForbidden},
		{name: "404", code: "404", text: "item-not-found", sentinel: ErrIQNotFound},
		{name: "405", code: "405", text: "not-allowed", sentinel: ErrIQNotAllowed},
		{name: "406", code: "406", text: "not-acceptable", sentinel: ErrIQNotAcceptable},
		{name: "410", code: "410", text: "gone", sentinel: ErrIQGone},
		{name: "419", code: "419", text: "resource-limit", sentinel: ErrIQResourceLimit},
		{name: "423", code: "423", text: "locked", sentinel: ErrIQLocked},
		{name: "429", code: "429", text: "rate-overlimit", sentinel: ErrIQRateOverLimit},
		{name: "500", code: "500", text: "internal-server-error", sentinel: ErrIQInternalServerError},
		{name: "503", code: "503", text: "service-unavailable", sentinel: ErrIQServiceUnavailable},
		{name: "530", code: "530", text: "partial-server-error", sentinel: ErrIQPartialServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := parseIQError(iqNode(tc.code, tc.text))
			if !errors.Is(err, tc.sentinel) {
				t.Errorf("erro %s/%s nao casou com o sentinela", tc.code, tc.text)
			}
			// E nao pode casar com nenhum dos outros.
			for _, other := range cases {
				if other.code == tc.code {
					continue
				}
				if errors.Is(err, other.sentinel) {
					t.Errorf("erro %s casou indevidamente com o sentinela %s", tc.code, other.code)
				}
			}
		})
	}
}

// Mesmo code com text diferente nao e' o mesmo erro: o par e' que identifica.
func TestIQErrorIsRequiresMatchingText(t *testing.T) {
	err := parseIQError(iqNode("404", "outra-coisa"))
	if errors.Is(err, ErrIQNotFound) {
		t.Error("code igual com text diferente nao pode casar")
	}
}

// Um <iq> sem <error> vira IQError de code 0. Nao pode casar com sentinela
// nenhum, e a mensagem tem que mostrar o no' bruto para dar diagnostico.
func TestIQErrorWithoutErrorChild(t *testing.T) {
	node := &waBinary.Node{Tag: "iq", Attrs: waBinary.Attrs{"id": "1"}}
	err := parseIQError(node)

	var iqe *IQError
	if !errors.As(err, &iqe) {
		t.Fatalf("err = %T, queria *IQError", err)
	}
	if iqe.Code != 0 {
		t.Errorf("Code = %d, queria 0", iqe.Code)
	}
	if iqe.ErrorNode != nil {
		t.Error("ErrorNode deveria ser nil sem filho <error>")
	}
	if iqe.RawNode != node {
		t.Error("RawNode tem que guardar o no' original")
	}
	if errors.Is(err, ErrIQNotFound) || errors.Is(err, ErrIQBadRequest) {
		t.Error("erro sem code nao pode casar com sentinela")
	}
	if !strings.Contains(err.Error(), "unexpected response") {
		t.Errorf("Error() = %q, queria falar de resposta inesperada", err.Error())
	}
}

// As tres formas da mensagem de IQError, escolhidas por quais campos existem.
func TestIQErrorMessageForms(t *testing.T) {
	withCode := &IQError{Code: 404, Text: "item-not-found"}
	if !strings.Contains(withCode.Error(), "404") {
		t.Errorf("Error() = %q, queria conter o code", withCode.Error())
	}

	withNode := &IQError{ErrorNode: &waBinary.Node{Tag: "error"}}
	if !strings.Contains(withNode.Error(), "unknown error") {
		t.Errorf("Error() = %q, queria \"unknown error\"", withNode.Error())
	}

	empty := &IQError{}
	if empty.Error() != "unknown info query error" {
		t.Errorf("Error() = %q, queria \"unknown info query error\"", empty.Error())
	}
}

// IQError.Is so' aceita outro *IQError — comparar com um erro qualquer nao pode
// casar por acidente.
func TestIQErrorIsRejectsOtherTypes(t *testing.T) {
	err := &IQError{Code: 404, Text: "item-not-found"}
	if errors.Is(err, errors.New("item-not-found")) {
		t.Error("IQError nao pode casar com erro de outro tipo")
	}
	if errors.Is(err, ErrNotConnected) {
		t.Error("IQError nao pode casar com sentinela de outro tipo")
	}
}

// --- wrappedIQError ---

// wrapIQError guarda os dois lados: a mensagem legivel vai para o usuario e o
// IQError original continua alcancavel por errors.Is/As. Perder qualquer um dos
// dois lados quebra o tratamento de erro rio acima.
func TestWrappedIQErrorKeepsBothSides(t *testing.T) {
	iqErr := parseIQError(iqNode("403", "forbidden"))
	wrapped := wrapIQError(ErrGroupInviteLinkUnauthorized, iqErr)

	if wrapped.Error() != ErrGroupInviteLinkUnauthorized.Error() {
		t.Errorf("Error() = %q, queria a mensagem humana", wrapped.Error())
	}
	if !errors.Is(wrapped, ErrGroupInviteLinkUnauthorized) {
		t.Error("perdeu o erro humano")
	}
	if !errors.Is(wrapped, ErrIQForbidden) {
		t.Error("perdeu o IQError original, alcancavel por Unwrap")
	}
	if errors.Is(wrapped, ErrIQNotFound) {
		t.Error("casou com um IQError que nao e' o dele")
	}
}

// --- DownloadHTTPError ---

// A comparacao e' por status code: e' assim que o codigo de download distingue
// "midia expirou" (410) de "sem permissao" (403).
func TestDownloadHTTPErrorIsByStatusCode(t *testing.T) {
	cases := []struct {
		code     int
		sentinel error
	}{
		{code: 403, sentinel: ErrMediaDownloadFailedWith403},
		{code: 404, sentinel: ErrMediaDownloadFailedWith404},
		{code: 410, sentinel: ErrMediaDownloadFailedWith410},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.code), func(t *testing.T) {
			err := DownloadHTTPError{Response: &http.Response{StatusCode: tc.code}}
			if !errors.Is(err, tc.sentinel) {
				t.Errorf("status %d nao casou com o proprio sentinela", tc.code)
			}
			for _, other := range cases {
				if other.code != tc.code && errors.Is(err, other.sentinel) {
					t.Errorf("status %d casou indevidamente com %d", tc.code, other.code)
				}
			}
			if !strings.Contains(err.Error(), fmt.Sprint(tc.code)) {
				t.Errorf("Error() = %q, queria conter o status", err.Error())
			}
		})
	}
}

// Tem que continuar funcionando embrulhado, que e' como chega de dentro do
// caminho de download.
func TestDownloadHTTPErrorIsThroughWrapping(t *testing.T) {
	err := fmt.Errorf("baixando anexo: %w", DownloadHTTPError{Response: &http.Response{StatusCode: 404}})
	if !errors.Is(err, ErrMediaDownloadFailedWith404) {
		t.Error("perdeu a classificacao ao ser embrulhado")
	}
}

// --- DisconnectedError ---

// A comparacao e' por Action: qualquer request interrompido por desconexao vira
// o mesmo tipo, e o Action e' o que diz qual era.
func TestDisconnectedErrorIsByAction(t *testing.T) {
	if !errors.Is(&DisconnectedError{Action: "info query"}, ErrIQDisconnected) {
		t.Error("mesma Action deveria casar")
	}
	if errors.Is(&DisconnectedError{Action: "message send"}, ErrIQDisconnected) {
		t.Error("Action diferente nao pode casar")
	}
	if errors.Is(ErrIQDisconnected, errors.New("info query")) {
		t.Error("nao pode casar com erro de outro tipo")
	}
	if !strings.Contains(ErrIQDisconnected.Error(), "info query") {
		t.Errorf("Error() = %q, queria conter a Action", ErrIQDisconnected.Error())
	}
}

// O no' recebido junto e' so' contexto: nao pode entrar na comparacao.
func TestDisconnectedErrorIgnoresNodeInComparison(t *testing.T) {
	withNode := &DisconnectedError{Action: "info query", Node: &waBinary.Node{Tag: "iq"}}
	if !errors.Is(withNode, ErrIQDisconnected) {
		t.Error("o no' anexado nao pode atrapalhar a comparacao")
	}
}

// --- erros de pareamento ---

// Os dois erros de pareamento embrulham a causa original, que e' o que permite
// diagnosticar uma falha de pareamento sem log adicional.
func TestPairErrorsUnwrap(t *testing.T) {
	cause := errors.New("campo obrigatorio ausente")

	protoErr := &PairProtoError{Message: "falha ao decodificar", ProtoErr: cause}
	if !errors.Is(protoErr, cause) {
		t.Error("PairProtoError perdeu a causa")
	}
	if !strings.Contains(protoErr.Error(), "falha ao decodificar") ||
		!strings.Contains(protoErr.Error(), cause.Error()) {
		t.Errorf("Error() = %q, queria as duas partes", protoErr.Error())
	}

	dbErr := &PairDatabaseError{Message: "falha ao salvar", DBErr: cause}
	if !errors.Is(dbErr, cause) {
		t.Error("PairDatabaseError perdeu a causa")
	}
	if !strings.Contains(dbErr.Error(), "falha ao salvar") {
		t.Errorf("Error() = %q, queria a mensagem", dbErr.Error())
	}
}

// --- ElementMissingError ---

func TestElementMissingErrorMessage(t *testing.T) {
	err := &ElementMissingError{Tag: "media", In: "iq response"}
	got := err.Error()
	if !strings.Contains(got, "<media>") || !strings.Contains(got, "iq response") {
		t.Errorf("Error() = %q, queria a tag e o contexto", got)
	}
}

// --- sentinelas distintos ---

// Os sentinelas de errors.New sao identidades. Se dois compartilhassem valor,
// errors.Is confundiria os dois casos — este teste trava a distincao dos que
// mais aparecem juntos no mesmo tratamento de erro.
func TestSentinelErrorsAreDistinct(t *testing.T) {
	sentinels := map[string]error{
		"ErrClientIsNil":      ErrClientIsNil,
		"ErrNoSession":        ErrNoSession,
		"ErrIQTimedOut":       ErrIQTimedOut,
		"ErrNotConnected":     ErrNotConnected,
		"ErrNotLoggedIn":      ErrNotLoggedIn,
		"ErrMessageTimedOut":  ErrMessageTimedOut,
		"ErrAlreadyConnected": ErrAlreadyConnected,
	}
	for nameA, a := range sentinels {
		for nameB, b := range sentinels {
			if nameA == nameB {
				continue
			}
			if errors.Is(a, b) {
				t.Errorf("%s e %s sao o mesmo erro", nameA, nameB)
			}
		}
	}
}
