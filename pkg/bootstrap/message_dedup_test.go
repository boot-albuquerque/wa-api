package bootstrap

import (
	"strings"
	"testing"

	"github.com/patrickmn/go-cache"

	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// F103. O que estes testes travam é a POLÍTICA, não o mecanismo: qual cópia
// fica, quem é separado de quem, e o que acontece quando não há id.

func limparDedup(t *testing.T) {
	t.Helper()
	mensagensVistas.Flush()
	t.Cleanup(func() { mensagensVistas.Flush() })
}

func eventoDedup(id, tipo, midia, pushname string) *events.Message {
	return &events.Message{Info: types.MessageInfo{
		ID:        id,
		Type:      tipo,
		MediaType: midia,
		PushName:  pushname,
	}}
}

func TestDedup_PrimeiraPassaSegundaEhSuprimida(t *testing.T) {
	limparDedup(t)

	if mensagemJaProcessada("u1", eventoDedup("M1", "media", "image", "Ana")) {
		t.Fatal("a PRIMEIRA copia foi suprimida; a mensagem nunca seria entregue")
	}
	if !mensagemJaProcessada("u1", eventoDedup("M1", "media", "image", "Ana")) {
		t.Error("a segunda copia passou; o cliente receberia a mesma mensagem duas vezes com o mesmo messageID")
	}
}

// TestDedup_UsuariosNaoSeMisturam é a proteção contra o erro mais caro
// possível aqui: suprimir a mensagem de OUTRO usuário. O `messageID` do
// WhatsApp é único por remetente, não globalmente.
func TestDedup_UsuariosNaoSeMisturam(t *testing.T) {
	limparDedup(t)

	mensagemJaProcessada("u1", eventoDedup("MESMO-ID", "media", "image", "Ana"))

	if mensagemJaProcessada("u2", eventoDedup("MESMO-ID", "media", "image", "Bruno")) {
		t.Error("a mensagem de u2 foi suprimida pela de u1; perda silenciosa, muito pior que a duplicata que isto corrige")
	}
}

// TestDedup_SemIDNaoDeduplica: sem id não há como deduplicar, e suprimir por
// chave vazia colapsaria mensagens sem relação nenhuma numa só.
func TestDedup_SemIDNaoDeduplica(t *testing.T) {
	limparDedup(t)

	if mensagemJaProcessada("u1", eventoDedup("", "media", "", "")) {
		t.Fatal("mensagem sem id foi tratada como ja vista")
	}
	if mensagemJaProcessada("u1", eventoDedup("", "media", "", "")) {
		t.Error("duas mensagens sem id colapsaram numa so'; a segunda seria descartada sem relacao com a primeira")
	}
}

func TestDedup_NilNaoEntraEmPanico(t *testing.T) {
	limparDedup(t)
	if mensagemJaProcessada("u1", nil) {
		t.Error("nil tratado como ja visto")
	}
}

// TestDedup_MetadadoPerdidoEhRegistrado é o teste que impede a decisão de
// política de virar aposta permanente.
//
// Guardamos a PRIMEIRA cópia, e a investigação da F103 mostrou que ela é a
// metadata-incompleta. Isso é uma troca deliberada — duplicar cobrança é pior
// que metadado pobre —, mas só é revisável se o custo aparecer no log.
func TestDedup_MetadadoPerdidoEhRegistrado(t *testing.T) {
	limparDedup(t)
	buf := capturarLog(t)

	// Primeira: como chega pelo reenvio do telefone — sem type, sem pushname.
	mensagemJaProcessada("u1", eventoDedup("M2", "", "", ""))
	// Segunda: a copia ao vivo, completa.
	mensagemJaProcessada("u1", eventoDedup("M2", "media", "image", "Yasmin"))

	saida := buf.String()
	for _, campo := range []string{"metadado_perdido", "type", "media_type", "pushname"} {
		if !strings.Contains(saida, campo) {
			t.Errorf("o log nao nomeia %q; a troca de politica fica invisivel e nao da' para revisar com dado: %s", campo, saida)
		}
	}
}

// TestDedup_SemPerdaNaoAlarma é o controle negativo do teste acima: quando a
// cópia descartada não trazia nada a mais, não há custo a reportar, e um Warn
// ali treinaria a ignorar o campo.
func TestDedup_SemPerdaNaoAlarma(t *testing.T) {
	limparDedup(t)
	buf := capturarLog(t)

	mensagemJaProcessada("u1", eventoDedup("M3", "media", "image", "Ana"))
	mensagemJaProcessada("u1", eventoDedup("M3", "media", "image", "Ana"))

	if strings.Contains(buf.String(), "metadado_perdido") {
		t.Errorf("reportou perda de metadado onde nao houve nenhuma: %s", buf.String())
	}
}

// TestDedup_SeamDoHandleMessage é o teste que faltava, e a ausência dele foi
// pega por controle negativo: com a chamada a `mensagemJaProcessada` REMOVIDA
// de `handleMessage`, nenhum dos testes acima falhava. Eles cobrem a função,
// não o caminho — e uma dedup que existe e não está ligada não deduplica nada
// (ARMADILHAS 24).
//
// O que se verifica é o efeito observável: `st.dowebhook` nasce 0 e só
// `handleMessage` o liga. Se a segunda cópia for suprimida, ele fica 0 e o
// despacho de `eventhandler.go:183` não acontece.
func TestDedup_SeamDoHandleMessage(t *testing.T) {
	limparDedup(t)
	capturarLog(t)

	// handleMessage escreve no LastMessageCache. Em teste o appCtx pode nao ter
	// sido montado, e o nil deref esconderia o que se quer medir.
	anterior := appCtx
	if appCtx == nil || appCtx.LastMessageCache == nil {
		appCtx = NewAppContext()
	}
	t.Cleanup(func() { appCtx = anterior })

	// Sem entrada no cache, resolveMessageS3Config cai no banco — e `evh.DB` e'
	// nil aqui. Popular o cache mantem o teste no caminho real de producao (o
	// cache e' o caminho comum) sem precisar de banco.
	appCtx.UserInfoCache.Set("u-seam", Values{M: map[string]string{
		"S3Enabled": "false", "MediaDelivery": "base64",
	}}, cache.DefaultExpiration)

	evh := &UserEventHandler{UserID: "u-seam"}
	evt := eventoDedup("M-SEAM", "media", "image", "Ana")

	primeira := &eventState{txtid: "u-seam", postmap: map[string]any{}}
	evh.handleMessage(evt, primeira)
	if primeira.dowebhook != 1 {
		t.Fatalf("a PRIMEIRA copia nao ligou o webhook (dowebhook=%d); a mensagem nunca seria entregue", primeira.dowebhook)
	}

	segunda := &eventState{txtid: "u-seam", postmap: map[string]any{}}
	evh.handleMessage(evt, segunda)
	if segunda.dowebhook != 0 {
		t.Errorf("a segunda copia ligou o webhook (dowebhook=%d); a dedup nao esta' no caminho de handleMessage e o cliente receberia duas vezes",
			segunda.dowebhook)
	}
}
