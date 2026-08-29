package bootstrap

import (
	"errors"
	"strings"
	"testing"

	"wa-api/pkg/domain"
)

// --- a regra que a decisão 94 sublinhou, e que sobrevive à remoção dela ---
//
// A seleção estática de engine (WA_API_ENGINE / WA_API_ENGINE_HEADLESS_SESSIONS)
// foi removida: o engine agora é escolhido por sessão, no corpo de
// POST /admin/users, e testado em
// pkg/application/usecase/user/add_user_test.go. O que este arquivo ainda
// testa é o roteamento por PORT (rotaDeEngine/ErrEngineSemPort), que nunca
// dependeu da seleção estática e continua valendo.

func TestSemFallbackSILENCIOSO(t *testing.T) {
	casos := []struct {
		nome        string
		engine      string
		temHeadless bool
		querUsar    bool
		querRecusar bool
	}{
		{"socket serve tudo", string(domain.EngineWaNoise), false, false, false},
		{"socket serve tudo mesmo havendo headless", string(domain.EngineWaNoise), true, false, false},
		{"headless com implementação", string(domain.EngineWaHeadless), true, true, false},
		{"headless SEM implementação RECUSA", string(domain.EngineWaHeadless), false, false, true},
	}
	for _, c := range casos {
		usar, recusar := rotaDeEngine(c.engine, c.temHeadless)
		if usar != c.querUsar || recusar != c.querRecusar {
			t.Errorf("%s: usar=%v recusar=%v; queria usar=%v recusar=%v",
				c.nome, usar, recusar, c.querUsar, c.querRecusar)
		}
	}
}

// A recusa tem de DIZER o que aconteceu: qual port, qual engine, e o motivo
// medido quando há. Um erro genérico mandaria quem opera adivinhar.
func TestARecusaDIZOQueAconteceu(t *testing.T) {
	err := error(ErrEngineSemPort{
		Port: "GroupPhotoSetter", Engine: string(domain.EngineWaHeadless), TxtID: "s1",
		Motivo: "modulos de foto ausentes do build (H140)",
	})
	msg := err.Error()
	for _, pedaco := range []string{"GroupPhotoSetter", "headless", "H140"} {
		if !strings.Contains(msg, pedaco) {
			t.Errorf("a recusa não diz %q: %s", pedaco, msg)
		}
	}
	var alvo ErrEngineSemPort
	if !errors.As(err, &alvo) || alvo.TxtID != "s1" {
		t.Fatal("a recusa não é reconhecível por errors.As com a sessão dentro")
	}
}

func TestSemMotivoARecusaAindaDizQueNaoHaFallback(t *testing.T) {
	msg := ErrEngineSemPort{Port: "X", Engine: string(domain.EngineWaHeadless)}.Error()
	if !strings.Contains(msg, "sem fallback") {
		t.Fatalf("a recusa sem motivo não explica por que não caiu no outro transporte: %s", msg)
	}
}
