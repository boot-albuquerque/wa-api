package message_test

import (
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
)

// errSession é o erro que a porta devolve quando não há sessão wa-noise. O
// que os testes cobram dos use cases é que ele chegue INTEIRO ao chamador —
// identidade preservada por errors.Is, e não um texto novo que apague a
// causa (era o que fmt.Errorf("no session") fazia).
var errSession = errors.New("porta: sessao inexistente")

const (
	callerID = "id-vindo-do-chamador"
	txtID    = "user-1"
)

// composerUC/missingField/composerUseCases (a tabela de use cases ainda
// construídos sobre port.MessageComposer) e os cinco TestComposerUseCases_*
// que a exercitavam saíram no CAP-22: SendList, o único caso que sobrava,
// migrou para port.SimpleMessenger (envio de verdade). Os eixos que a tabela
// cobria para ele — validação de campo obrigatório, propagação de falha de
// sessão, respeito ao Id do chamador, caminho feliz — foram para
// send_list_test.go, com a mesma disciplina de causa que os demais use cases
// send_* já exigem. A "geração de ID no caminho feliz" não tem destino
// porque deixou de existir: o use case não chama mais NewMessageID, e o
// MessageID publicado é o que a porta devolveu — mesma migração que
// SendContact, SendLocation, SendPoll, SendTemplate e SendButtons já
// tinham feito.
//
// Ficam aqui só os helpers de log COMPARTILHADOS por outros arquivos deste
// pacote (requireLog, assertSessionLog, errSession, callerID, txtID) — não
// apagar por engano ao tirar o último uso local.

// --- helpers de log ----------------------------------------------------

// requireLog exige um registro com nível e mensagem dados, e o devolve.
func requireLog(t *testing.T, logger *contractsfake.Logger, level, msg string) contractsfake.LogRecord {
	t.Helper()
	rec, ok := logger.FindLevel(level, msg)
	if !ok {
		t.Fatalf("faltou log %s %q; houve: %v", level, msg, logger.Messages())
	}
	if !rec.IsStructured() {
		t.Errorf("log %q nao e' estruturado (keyvals impares ou vazios): %v", msg, rec.Keyvals)
	}
	return rec
}

// assertSessionLog cobra a forma do log de ausência de sessão: nível error,
// mensagem canônica, a causa real e o identificador da sessão.
func assertSessionLog(t *testing.T, logger *contractsfake.Logger, idKey, idValue string) {
	t.Helper()
	rec := requireLog(t, logger, contractsfake.LevelWarn, "no wanoise session")
	if got, ok := rec.Keyval("error"); !ok || got != error(errSession) {
		t.Errorf("log de sessao nao carrega a causa real: %v", rec.Keyvals)
	}
	if got, ok := rec.Keyval(idKey); !ok || got != idValue {
		t.Errorf("log de sessao nao carrega %s=%q: %v", idKey, idValue, rec.Keyvals)
	}
}
