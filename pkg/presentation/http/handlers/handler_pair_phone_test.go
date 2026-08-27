package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/session"
	"wa-api/pkg/domain/apperr"
	dtosession "wa-api/pkg/presentation/http/dto/session"
)

// Este arquivo cobre POST /session/pairphone desde o CAP-26 — o handler que
// efetivamente devolve o LinkingCode, e nao mais uma struct VAZIA com 200
// (HOUSEKEEP F152, medido em campo:
//
//	POST /session/pairphone {"Phone":"55419924XXXXX"}
//	  -> 200 {"code":200,"data":{"LinkingCode":""},"success":true}
//
// A mesma sonda, contra uma sessao JA' PAREADA, provou as DUAS perdas: o
// codigo vazio e a guarda `already paired` que o contrato historico
// (41bc8e2^:handlers.go:729-733) devolvia como 400.
//
// Tudo aqui roda pela ROTA REGISTRADA (gorilla/mux), como wiring_routes.go:53
// faz — ARMADILHA 2 deste repo: defeito de rota so' aparece testando pela rota
// registrada.

const (
	// pairPhoneRoute e' o caminho REGISTRADO em wiring_routes.go:53.
	pairPhoneRoute = "/session/pairphone"
	// pairPhoneNumber e' o numero internacional da sonda de campo, com os
	// digitos finais trocados.
	pairPhoneNumber = "5541992400000"
	// pairPhoneBody e' o menor corpo VALIDO da rota.
	// A chave e' `phone`, minuscula: era `Phone` ate' a migracao para DTO.
	// Note que trocar de volta NAO faria este teste falhar — o
	// encoding/json casa a etiqueta sem distinguir caixa —, e por isso o
	// que trava a grafia e' o teste de contrato da rota, nao este corpo.
	pairPhoneBody = `{"phone":"` + pairPhoneNumber + `"}`
	// pairPhoneWireCode e' o codigo que a porta devolve nos casos felizes.
	// Formato de 8 caracteres em dois grupos, como
	// internal/wa-noise/capabilities/pairing/paircode.go:100 monta.
	pairPhoneWireCode = "WXYZ-2468"
	// pairPhoneLinkingCodeKey e' a UNICA chave do corpo de sucesso, e a que
	// o integrador le' para saber o que digitar no aparelho.
	//
	// Era "LinkingCode" — o nome do campo Go, PascalCase no fio — ate' a
	// migracao para DTO (docs/HTTP-DTO-CONVENTIONS.md). O teste antigo
	// afirmava a grafia ANTIGA e ainda listava "linking_code" entre as chaves
	// estrangeiras; um teste que protege um contrato mau nao e' requisito de
	// compatibilidade, e foi atualizado em vez de mantido.
	pairPhoneLinkingCodeKey = "linking_code"
)

// pairPhoneRouter registra o handler pela rota real (gorilla/mux).
func pairPhoneRouter(pp *contractsfake.PhonePairer) http.Handler {
	h := NewPairPhoneHandler(session.NewPairPhoneUseCase(pp, silentLogger{}))

	r := mux.NewRouter()
	r.Handle(pairPhoneRoute, h).Methods(http.MethodPost)
	return r
}

func pairPhoneServe(t *testing.T, pp *contractsfake.PhonePairer, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, pairPhoneRoute, strings.NewReader(body))
	pairPhoneRouter(pp).ServeHTTP(rec, msgAuthed(req))
	return rec
}

// pairPhoneServeCapturingLog e' o pairPhoneServe com a cadeia hlog que
// router.go instala. Existe porque o envelope de erro nao distingue duas
// recusas de mesmo status: so' o registro carrega a CAUSA (co-gate D).
func pairPhoneServeCapturingLog(t *testing.T, pp *contractsfake.PhonePairer, body string) (*httptest.ResponseRecorder, []logLine) {
	t.Helper()
	wrapped, capture := logassert.Wrap(pairPhoneRouter(pp))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, pairPhoneRoute, strings.NewReader(body))
	wrapped.ServeHTTP(rec, msgAuthed(req))
	return rec, capture.Records(t)
}

// pairPhoneIssuing e' o pairer do caminho feliz: nao pareado, devolve codigo.
func pairPhoneIssuing() *contractsfake.PhonePairer {
	return &contractsfake.PhonePairer{
		RequestPairingCodeFunc: func(context.Context, string, string) (string, error) {
			return pairPhoneWireCode, nil
		},
	}
}

// --- 1. sucesso ---------------------------------------------------------

// TestPairPhone_Success_ViaRegisteredRoute e' o teste do DEFEITO da F152: o
// LinkingCode chega PREENCHIDO no corpo da resposta. Ele e' o caminho de
// SUCESSO — ARMADILHA 2 deste repo diz que suite que so' exercita a guarda
// deixa passar exatamente esta classe de defeito, e foi o que aconteceu aqui.
func TestPairPhone_Success_ViaRegisteredRoute(t *testing.T) {
	pp := pairPhoneIssuing()

	rec := pairPhoneServe(t, pp, pairPhoneBody)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if !env.Success {
		t.Fatalf("envelope.success=false num 200: %s", rec.Body.String())
	}

	var data dtosession.PairPhoneResponse
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.LinkingCode == "" {
		t.Fatal("LinkingCode VAZIO no 200 — este e' o defeito da F152, medido em campo")
	}
	if data.LinkingCode != pairPhoneWireCode {
		t.Errorf("LinkingCode: got %q, want %q (o que a porta devolveu)", data.LinkingCode, pairPhoneWireCode)
	}

	// O telefone do payload chega a' porta sem reescrita.
	if len(pp.RequestPairingCodeCalls) != 1 {
		t.Fatalf("RequestPairingCode chamada %d vezes, quero 1", len(pp.RequestPairingCodeCalls))
	}
	if got := pp.RequestPairingCodeCalls[0].Phone; got != pairPhoneNumber {
		t.Errorf("Phone repassado: got %q, want %q", got, pairPhoneNumber)
	}
}

// --- 2. Phone ausente ---------------------------------------------------

// TestPairPhone_MissingPhone_400_LogsCause: sem Phone e' 400 E o registro que
// diz por que. A validacao de payload precede a guarda de sessao, entao
// EnsureSession nao pode nem ter sido chamada.
func TestPairPhone_MissingPhone_400_LogsCause(t *testing.T) {
	pp := pairPhoneIssuing()

	rec, recs := pairPhoneServeCapturingLog(t, pp, `{}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400 (corpo: %s)", rec.Code, rec.Body.String())
	}
	got := logassert.OutcomeLogged(t, recs, "missing Phone")
	if got.str("level") != "warn" {
		t.Errorf("recusa de payload no nivel %q, quero warn", got.str("level"))
	}
	if len(pp.EnsureSessionCalls) != 0 {
		t.Error("EnsureSession foi chamada apesar do payload invalido")
	}
	if len(pp.RequestPairingCodeCalls) != 0 {
		t.Error("RequestPairingCode foi chamada apesar do payload invalido")
	}
}

// --- 3. sessao ja' pareada: a ORDEM ------------------------------------

// TestPairPhone_AlreadyPaired_400_AndDoesNotRequestCode e' o teste da ORDEM,
// nao do sintoma. A guarda `already paired` roda ANTES de pedir o codigo
// (41bc8e2^:handlers.go:729-741); inverter as duas ainda devolveria um codigo
// e passaria em qualquer teste que so' olhasse o resultado feliz.
//
// A prova da ordem e' a asserção sobre RequestPairingCodeCalls estar VAZIO: o
// status 400 sozinho nao distingue "a guarda mordeu antes" de "a guarda
// mordeu depois de o codigo ja' ter sido pedido ao servidor do WhatsApp".
func TestPairPhone_AlreadyPaired_400_AndDoesNotRequestCode(t *testing.T) {
	pp := pairPhoneIssuing()
	pp.IsPairedFunc = func(context.Context, string) (bool, error) { return true, nil }

	rec, recs := pairPhoneServeCapturingLog(t, pp, pairPhoneBody)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400 (corpo: %s)", rec.Code, rec.Body.String())
	}
	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	env := decodeEnvelope(t, rec)
	if !strings.Contains(string(env.Error), "already paired") {
		t.Errorf("erro: got %s, want a mensagem historica \"already paired\"", env.Error)
	}
	if got := logassert.OutcomeLogged(t, recs, "already paired"); got.str("level") != "warn" {
		t.Errorf("recusa de sessao pareada no nivel %q, quero warn", got.str("level"))
	}

	if len(pp.IsPairedCalls) != 1 {
		t.Fatalf("IsPaired chamada %d vezes, quero 1", len(pp.IsPairedCalls))
	}
	if len(pp.RequestPairingCodeCalls) != 0 {
		t.Fatalf("ORDEM INVERTIDA: RequestPairingCode foi chamada %d vez(es) numa sessao ja' pareada; "+
			"a guarda tem de rodar ANTES do pedido de codigo", len(pp.RequestPairingCodeCalls))
	}
}

// --- 4. erro do cliente -------------------------------------------------

// TestPairPhone_PortFailure_400_NoSilentFallback: quando a porta falha, a
// resposta e' 400 e NENHUM 200 — nada de devolver codigo vazio com sucesso,
// que era exatamente o comportamento da F152.
func TestPairPhone_PortFailure_400_NoSilentFallback(t *testing.T) {
	// A causa e' o erro REAL do fork para numero nacional
	// (internal/wa-noise/capabilities/pairing/errors.go:18), embrulhado pelo
	// adapter como o adapter faz em producao
	// (pkg/infra/wa-noise/adapters/pairing/adapter.go).
	cause := errors.New("international phone number required (must not start with 0)")
	pp := &contractsfake.PhonePairer{
		RequestPairingCodeFunc: func(context.Context, string, string) (string, error) {
			return "", apperr.New("pair_phone_failed", apperr.CategoryValidation, cause.Error(), false, cause)
		},
	}

	rec, recs := pairPhoneServeCapturingLog(t, pp, pairPhoneBody)

	if rec.Code == http.StatusOK {
		t.Fatalf("200 apos falha da porta — fallback silencioso (corpo: %s)", rec.Body.String())
	}
	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	env := decodeEnvelope(t, rec)
	if strings.Contains(string(env.Error), pairPhoneLinkingCodeKey) {
		t.Errorf("resposta de erro carrega LinkingCode: %s", rec.Body.String())
	}
	if got := logassert.OutcomeLogged(t, recs, "international phone number"); got.str("level") != "warn" {
		t.Errorf("falha da porta no nivel %q, quero warn", got.str("level"))
	}
}

// --- 5. contrato de wire ------------------------------------------------

// pairPhoneWireKeys sao as chaves do corpo de sucesso, escritas A MAO — nao
// derivadas de domain.PairPhoneResult por reflexao, porque derivar da struct
// e' reintroduzir o defeito que send_wire_contract_test.go documenta: a lista
// esperada mudaria junto com a tag.
var pairPhoneWireKeys = []string{pairPhoneLinkingCodeKey}

// pairPhoneForeignWireKeys e' o vocabulario que NAO e' desta resposta.
// `linkingCode` e `LinkingCode` sao as duas renomeacoes mais provaveis de
// quem "padronizar" o corpo; `Details` e `Id` sao a forma historica dos
// envios, a troca mais provavel de quem restaurar fidelidade ao antigo.
var pairPhoneForeignWireKeys = []string{"linkingCode", "LinkingCode", "code", "Details", "Id"}

// TestPairPhone_WireContract_ViaRegisteredRoute trava o envelope de sempre e
// a chave LinkingCode no JSON REAL, decodificado em map[string]any pela rota
// registrada.
func TestPairPhone_WireContract_ViaRegisteredRoute(t *testing.T) {
	rec := pairPhoneServe(t, pairPhoneIssuing(), pairPhoneBody)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}

	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("resposta nao e' JSON: %v (corpo %s)", err, rec.Body.String())
	}
	// Envelope do ADR-002: code, data, success — e nenhum campo `error`.
	if raw["code"] != float64(http.StatusOK) {
		t.Errorf("envelope.code: got %v, want 200", raw["code"])
	}
	if raw["success"] != true {
		t.Errorf("envelope.success: got %v, want true", raw["success"])
	}
	if _, ok := raw["error"]; ok {
		t.Errorf("envelope de sucesso carrega `error`: %s", rec.Body.String())
	}

	data, ok := raw["data"].(map[string]any)
	if !ok {
		t.Fatalf("envelope.data nao e' objeto: %s", rec.Body.String())
	}
	for _, k := range pairPhoneWireKeys {
		v, ok := data[k]
		if !ok {
			t.Errorf("chave %q ausente no corpo: %s", k, rec.Body.String())
			continue
		}
		if v != pairPhoneWireCode {
			t.Errorf("data[%q]: got %v, want %q", k, v, pairPhoneWireCode)
		}
	}
	for _, k := range pairPhoneForeignWireKeys {
		if _, ok := data[k]; ok {
			t.Errorf("chave estrangeira %q no corpo: %s", k, rec.Body.String())
		}
	}
	if len(data) != len(pairPhoneWireKeys) {
		t.Errorf("data tem %d chaves, quero exatamente %d (%v): %s",
			len(data), len(pairPhoneWireKeys), pairPhoneWireKeys, rec.Body.String())
	}
}
