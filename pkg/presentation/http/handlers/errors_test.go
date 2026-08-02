package handlers

import (
	"errors"
	"testing"
)

// errors.go e user_context.go sao os dois arquivos-costura do pacote: o
// vocabulario de erro de fronteira que ~30 handlers respondem, e a forma
// minima que eles exigem do userinfo do middleware. Nenhum dos dois tem
// caminho de saida proprio — sao testados diretamente.

// TestSentinelErrors_CarryCause: cada sentinela tem de dizer o que faltou. Um
// sentinela com Error() vazio faz o log de caminho de saida perder a causa
// justamente nos ramos 401/400 mais frequentes do pacote.
func TestSentinelErrors_CarryCause(t *testing.T) {
	sentinels := map[string]error{
		"errUnauthorized":     errUnauthorized,
		"errMissingSessionID": errMissingSessionID,
		"errMissingID":        errMissingID,
		"errDecodePayload":    errDecodePayload,
	}

	seen := make(map[string]string, len(sentinels))
	for name, err := range sentinels {
		if err == nil {
			t.Fatalf("%s e' nil", name)
		}
		msg := err.Error()
		if msg == "" {
			t.Fatalf("%s.Error() e' vazio", name)
		}
		if other, dup := seen[msg]; dup {
			t.Fatalf("%s e %s tem a mesma causa (%q) — sao indistinguiveis no log", name, other, msg)
		}
		seen[msg] = name
	}
}

// TestSentinelErrors_AreDistinctIdentities: os sentinelas sao comparados por
// identidade (errors.Is) nos handlers e nos testes de contrato do envelope.
// Dois ponteiros distintos nunca podem casar entre si.
func TestSentinelErrors_AreDistinctIdentities(t *testing.T) {
	if !errors.Is(errUnauthorized, errUnauthorized) {
		t.Fatal("errors.Is nao reconhece o proprio sentinela")
	}
	if errors.Is(errUnauthorized, errMissingSessionID) {
		t.Fatal("sentinelas distintos casam em errors.Is")
	}
	if errors.Is(errDecodePayload, errMissingID) {
		t.Fatal("sentinelas distintos casam em errors.Is")
	}
}

// TestSimpleErr_ValueReceiver: Error() tem receptor por VALOR, e os sentinelas
// sao ponteiros. Trocar para receptor por ponteiro quebraria silenciosamente
// qualquer copia por valor; este teste fixa a forma.
func TestSimpleErr_ValueReceiver(t *testing.T) {
	byValue := simpleErr{msg: "falha de fronteira"}
	byPointer := &byValue

	if byValue.Error() != "falha de fronteira" {
		t.Fatalf("copia por valor perdeu a causa: %q", byValue.Error())
	}
	if byPointer.Error() != byValue.Error() {
		t.Fatalf("ponteiro (%q) e valor (%q) divergem", byPointer.Error(), byValue.Error())
	}
	var asError error = byPointer
	if asError.Error() != byValue.Error() {
		t.Fatal("*simpleErr nao satisfaz error com a mesma causa")
	}
}

// stubUserInfo e' a menor implementacao possivel de userInfo — a prova de que
// a interface local do pacote nao exige nada de pkg/bootstrap.
type stubUserInfo map[string]string

func (s stubUserInfo) Get(key string) string { return s[key] }

// TestUserInfo_MinimalContract: a interface pede exatamente um metodo, e a
// chave ausente devolve vazio (que e' o que os handlers tratam como "sem
// sessao"), nao panico.
func TestUserInfo_MinimalContract(t *testing.T) {
	var info userInfo = stubUserInfo{"Id": "42", "Token": "tok-42"}

	if got := info.Get("Id"); got != "42" {
		t.Fatalf("Get(\"Id\") = %q", got)
	}
	if got := info.Get("Token"); got != "tok-42" {
		t.Fatalf("Get(\"Token\") = %q", got)
	}
	if got := info.Get("Inexistente"); got != "" {
		t.Fatalf("chave ausente devolveu %q, esperado vazio", got)
	}
}
