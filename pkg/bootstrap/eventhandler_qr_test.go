package bootstrap

import (
	"strings"
	"testing"

	"wa-api/internal/wa-noise/protocol/types/events"
)

// codigoDePareamentoFalso imita a forma real de um codigo de QR do WhatsApp
// (ver o dump da F76). O que importa e' que seja uma string inconfundivel:
// se ela aparecer no log, o vazamento aconteceu.
const codigoDePareamentoFalso = "https://wa.me/settings/linked_devices#2@SEGREDO-QUE-NAO-PODE-VAZAR,abc="

// TestHandleEvent_QR_NaoVazaOsCodigosNoLog trava a F76.
//
// *events.QR carrega em Codes os codigos de pareamento da sessao. Um deles,
// lido por quem tenha acesso ao arquivo de log, vincula um aparelho a conta —
// e' credencial, nao diagnostico. Antes da correcao, o evento caia no ramo
// `default` do switch de handleEvent, que dumpa a struct inteira com
// fmt.Sprintf("%+v"): foram 6 codigos e 1678 bytes num unico log.Warn.
//
// captureLog usa zerolog.New sem filtro de nivel, entao ele ve ate Debug —
// que e' o ajuste mais severo possivel para esta assercao. Se o codigo nao
// vaza aqui, nao vaza em nivel nenhum.
func TestHandleEvent_QR_NaoVazaOsCodigosNoLog(t *testing.T) {
	evh := &UserEventHandler{UserID: "user-42"}
	evt := &events.QR{Codes: []string{codigoDePareamentoFalso, "segundo-codigo"}}

	out := captureLog(t, func() { evh.handleEvent(evt) })

	if strings.Contains(out, "SEGREDO-QUE-NAO-PODE-VAZAR") {
		t.Fatalf("codigo de pareamento vazou no log: %q", out)
	}
	if strings.Contains(out, "segundo-codigo") {
		t.Fatalf("codigo de pareamento vazou no log: %q", out)
	}
	// A contagem pode e deve sair: e' o que torna o evento diagnosticavel sem
	// entregar a credencial.
	if !strings.Contains(out, `"codes":2`) {
		t.Errorf("esperava a contagem de codigos no log, obtive: %q", out)
	}
}

// TestHandleEvent_QR_NaoCaiNoRamoDefault e' a outra metade da F76: o teste
// acima passaria tambem se alguem silenciasse o `default` inteiro, o que
// esconderia todo evento imprevisto. Este fixa a causa — QR tem `case`
// proprio — em vez do sintoma.
func TestHandleEvent_QR_NaoCaiNoRamoDefault(t *testing.T) {
	evh := &UserEventHandler{UserID: "user-42"}

	out := captureLog(t, func() { evh.handleEvent(&events.QR{Codes: []string{"x"}}) })

	if strings.Contains(out, "Unhandled event") {
		t.Fatalf("QR voltou a cair no ramo default: %q", out)
	}
}

// TestHandleEvent_DefaultContinuaAvisando e' o controle negativo do teste
// anterior. Sem ele, remover o proprio `default` faria os dois passarem — e o
// aviso de "apareceu algo que nao previmos" desapareceria sem que nada
// acusasse.
//
// eventoDesconhecido nao e' um tipo de events.*, entao esta' garantido que
// nenhum `case` do switch o alcanca.
func TestHandleEvent_DefaultContinuaAvisando(t *testing.T) {
	type eventoDesconhecido struct{ Campo string }

	evh := &UserEventHandler{UserID: "user-42"}

	out := captureLog(t, func() { evh.handleEvent(&eventoDesconhecido{Campo: "valor"}) })

	if !strings.Contains(out, "Unhandled event") {
		t.Fatalf("o ramo default parou de avisar sobre evento imprevisto: %q", out)
	}
}
