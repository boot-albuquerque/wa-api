package bootstrap

import (
	"strings"
	"testing"

	waE2E "wa-api/internal/noise/protocol/proto/waE2E"
	"wa-api/internal/noise/protocol/types"
	"wa-api/internal/noise/protocol/types/events"
)

// F102. O defeito era de OBSERVABILIDADE, não de entrega: uma mensagem
// classificada como mídia que não casasse com nenhum tipo tratado não produzia
// nada — nem download, nem aviso, nem erro. O operador via
// `Message Received ... type: media` e mais nada.
//
// Esse silêncio é indistinguível de um download que falhou calado, e as duas
// situações pedem ações opostas de quem investiga.

// capturarLog routes the global logger into a buffer for this test. See
// logcapture_test.go: the global itself is never reassigned (F132).
func capturarLog(t *testing.T) *logCapture {
	t.Helper()
	return captureLogInto(t)
}

func eventoDeMidia(msg *waE2E.Message) *events.Message {
	return &events.Message{
		Info: types.MessageInfo{
			ID:        "MSG-1",
			Type:      "media",
			MediaType: "desconhecido",
		},
		Message: msg,
	}
}

// TestMidia_TipoNaoTratadoDeixaRastro é o teste da correção.
func TestMidia_TipoNaoTratadoDeixaRastro(t *testing.T) {
	evh := &UserEventHandler{UserID: "u1"}
	buf := capturarLog(t)

	// Mensagem sem NENHUM dos tipos que processMessageMedia trata.
	evh.processMessageMedia(eventoDeMidia(&waE2E.Message{}), messageS3Config{}, &eventState{postmap: map[string]any{}})

	saida := buf.String()
	if saida == "" {
		t.Fatal("nada registrado: o operador ve 'Message Received type: media' e mais nada, indistinguivel de um download que falhou calado (F102)")
	}
	// Os campos que fazem o aviso servir para investigar. Sem o id nao da' para
	// cruzar com o `Message Received`; sem o tipo nao da' para saber o que
	// chegou que a gente nao trata.
	for _, campo := range []string{"MSG-1", "media_type", "desconhecido"} {
		if !strings.Contains(saida, campo) {
			t.Errorf("o aviso saiu sem %q, entao nao serve para investigar: %s", campo, saida)
		}
	}
}

// TestMidia_CabecalhoDeAlbumNaoVira Ruido é o controle que impede a correção de
// virar barulho de rotina.
//
// Álbum é o caso MEDIDO: 4 fotos chegam como cabeçalho + 4 imagens, e nenhuma
// mídia se perde. Se o cabeçalho disparasse o aviso, todo álbum enviado geraria
// uma linha de "não tratado" — e um diagnóstico que aparece no caminho normal
// deixa de ser lido.
func TestMidia_CabecalhoDeAlbumNaoViraRuido(t *testing.T) {
	evh := &UserEventHandler{UserID: "u1"}
	buf := capturarLog(t)

	esperadas := uint32(4)
	msg := &waE2E.Message{AlbumMessage: &waE2E.AlbumMessage{ExpectedImageCount: &esperadas}}
	evh.processMessageMedia(eventoDeMidia(msg), messageS3Config{}, &eventState{postmap: map[string]any{}})

	if saida := buf.String(); saida != "" {
		t.Errorf("cabecalho de album gerou aviso de 'nao tratado'; todo album enviado poluiria o log: %s", saida)
	}
}

// --- F180: o aviso de mídia não tratada não pode disparar em texto ----------
//
// processMessageMedia corre para TODA mensagem recebida. Antes da guarda, uma
// mensagem de texto produzia "mensagem de midia de tipo nao tratado" — medido
// em campo, 25 de 30 ocorrências eram `type=text`.
//
// O teste tem DOIS lados de propósito. Só o primeiro (texto não avisa) deixaria
// passar a correção preguiçosa de apagar o aviso; só o segundo (mídia avisa)
// deixaria passar o defeito original. É o par que define o comportamento.

func TestProcessMedia_TextoNaoDisparaAvisoDeMidia(t *testing.T) {
	evh := &UserEventHandler{UserID: "u-texto"}
	buf := capturarLog(t)

	evt := eventoNaoClassificavel("MSG-TEXTO", "text")
	evt.Message = &waE2E.Message{Conversation: proto("ola")}

	evh.processMessageMedia(evt, messageS3Config{}, &eventState{postmap: map[string]any{}})

	if strings.Contains(buf.String(), "midia de tipo nao tratado") {
		t.Fatalf("mensagem de TEXTO disparou o aviso de mídia não tratada (F180): %s", buf.String())
	}
}

func TestProcessMedia_MidiaAnunciadaENaoTratadaAindaAvisa(t *testing.T) {
	evh := &UserEventHandler{UserID: "u-midia"}
	buf := capturarLog(t)

	// O servidor DISSE que era mídia e a mensagem não traz nenhum dos tipos
	// que sabemos tratar. É exatamente o caso da F102, e tem de continuar a
	// avisar — silenciá-lo seria trocar um defeito por outro.
	evt := eventoNaoClassificavel("MSG-MIDIA", messageWireTypeMedia)

	evh.processMessageMedia(evt, messageS3Config{}, &eventState{postmap: map[string]any{}})

	if !strings.Contains(buf.String(), "midia de tipo nao tratado") {
		t.Fatalf("mídia anunciada e não tratada deixou de avisar: %s", buf.String())
	}
}
