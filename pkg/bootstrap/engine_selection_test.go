package bootstrap

import (
	"errors"
	"strings"
	"testing"
)

func comAmbiente(t *testing.T, padrao, sessoes string) (EngineSelection, error) {
	t.Helper()
	t.Setenv(envEngineDefault, padrao)
	t.Setenv(envEngineSessions, sessoes)
	return engineSelectionConfigurada()
}

// O comportamento de antes da decisão 94 tem de continuar a ser o padrão: quem
// não configurou nada não pode ter mudado de transporte.
func TestSemConfiguracaoTudoContinuaNoSocket(t *testing.T) {
	sel, err := comAmbiente(t, "", "")
	if err != nil {
		t.Fatalf("configuração vazia devia ser válida: %v", err)
	}
	if got := sel.EngineFor("qualquer"); got != EngineWaNoise {
		t.Fatalf("EngineFor = %q, queria %q", got, EngineWaNoise)
	}
}

// Um typo não pode virar silenciosamente o padrão — é a mesma regra do
// clusterMode, e pela mesma razão.
func TestEngineDesconhecidoEErroENaoPadrao(t *testing.T) {
	if _, err := comAmbiente(t, "headles", ""); err == nil {
		t.Fatal("um typo em WA_API_ENGINE foi aceito e cairia no padrão em silêncio")
	}
}

func TestSessaoListadaVaiParaHeadlessEAsOutrasNao(t *testing.T) {
	sel, err := comAmbiente(t, "", "s1, s2")
	if err != nil {
		t.Fatalf("lista válida recusada: %v", err)
	}
	for _, id := range []string{"s1", "s2"} {
		if got := sel.EngineFor(id); got != EngineWaHeadless {
			t.Fatalf("EngineFor(%q) = %q, queria headless", id, got)
		}
	}
	if got := sel.EngineFor("s3"); got != EngineWaNoise {
		t.Fatalf("uma sessão NÃO listada foi para %q", got)
	}
}

func TestListaMalFormadaERecusada(t *testing.T) {
	if _, err := comAmbiente(t, "", "s1,,s2"); err == nil {
		t.Fatal("vírgula sobrando foi aceita")
	}
	if _, err := comAmbiente(t, "", "s1,s1"); err == nil {
		t.Fatal("sessão repetida foi aceita, e ninguém saberia qual venceu")
	}
}

func TestOArranquePodeDIZEROQueVaiFazer(t *testing.T) {
	sel, err := comAmbiente(t, "", "s2,s1")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(sel.SessoesEmHeadless(), ",")
	if got != "s1,s2" {
		t.Fatalf("SessoesEmHeadless = %q, queria ordenado \"s1,s2\"", got)
	}
}

// --- a regra que a decisão 94 sublinhou ---

func TestSemFallbackSILENCIOSO(t *testing.T) {
	casos := []struct {
		nome        string
		engine      string
		temHeadless bool
		querUsar    bool
		querRecusar bool
	}{
		{"socket serve tudo", EngineWaNoise, false, false, false},
		{"socket serve tudo mesmo havendo headless", EngineWaNoise, true, false, false},
		{"headless com implementação", EngineWaHeadless, true, true, false},
		{"headless SEM implementação RECUSA", EngineWaHeadless, false, false, true},
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
		Port: "GroupPhotoSetter", Engine: EngineWaHeadless, TxtID: "s1",
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
	msg := ErrEngineSemPort{Port: "X", Engine: EngineWaHeadless}.Error()
	if !strings.Contains(msg, "sem fallback") {
		t.Fatalf("a recusa sem motivo não explica por que não caiu no outro transporte: %s", msg)
	}
}
