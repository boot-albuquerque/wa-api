package jid

import (
	"context"
	"testing"
)

// TestParseJIDRejectsEmptyInsteadOfPanicking trava a CAUSA da F101.
//
// A entrada medida em campo foi a string vazia, e o defeito não era um erro
// errado: era `arg[0]` numa string de tamanho zero, ou seja, um panic onde a
// assinatura promete (JID, bool). O teste do sintoma — a rota devolvendo 400 —
// vive em handler_group_mgmt_test.go e continuaria passando se a guarda fosse
// removida daqui, porque a validação da fronteira o esconde. Por isso os dois
// existem.
func TestParseJIDRejectsEmptyInsteadOfPanicking(t *testing.T) {
	if _, ok := ParseJID(""); ok {
		t.Fatal("ParseJID(\"\") aceitou a string vazia, quero recusa")
	}
}

// TestResolveJIDOnEmptyReturnsErrorNotPanic exercita a porta que os use cases
// realmente consomem — appport.JIDResolver — e não o helper por baixo dela.
// Um panic aqui falha o teste sem asserção nenhuma, que é exatamente o
// comportamento que a F101 mediu antes da correção.
func TestResolveJIDOnEmptyReturnsErrorNotPanic(t *testing.T) {
	var r JIDResolverAdapter
	got, err := r.ResolveJID(context.Background(), "")
	if err == nil {
		t.Fatalf("ResolveJID(\"\") devolveu %q sem erro, quero erro explícito", string(got))
	}
	if got != "" {
		t.Fatalf("ResolveJID(\"\") devolveu %q junto do erro, quero vazio", string(got))
	}
}

// TestResolveJIDStillAcceptsBareNumber é o controle de que a guarda não
// atropelou o caminho de sucesso: o número nu continua ganhando o servidor
// padrão, que é a promessa medida em produção e registrada em
// get_user_profile.go.
func TestResolveJIDStillAcceptsBareNumber(t *testing.T) {
	var r JIDResolverAdapter
	got, err := r.ResolveJID(context.Background(), "5511999999999")
	if err != nil {
		t.Fatalf("número nu recusado: %v", err)
	}
	if want := "5511999999999@s.whatsapp.net"; string(got) != want {
		t.Fatalf("got %q, want %q", string(got), want)
	}
}
