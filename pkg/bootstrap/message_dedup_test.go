package bootstrap

import (
	"strings"
	"testing"

	"github.com/patrickmn/go-cache"

	waE2E "wa-api/internal/noise/protocol/proto/waE2E"
	"wa-api/internal/noise/protocol/types"
	"wa-api/internal/noise/protocol/types/events"
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

// eventoDedupComConteudo é eventoDedup mais o campo Message — necessário
// para os testes de F358, que dependem do PAYLOAD (não só do metadado de
// Info) para decidir se a cópia tem mídia utilizável.
func eventoDedupComConteudo(id, tipo, midia, pushname string, msg *waE2E.Message) *events.Message {
	evt := eventoDedup(id, tipo, midia, pushname)
	evt.Message = msg
	return evt
}

// mensagemMidiaVazia é o que a F358 mediu chegando primeiro: type=media,
// media_type=video, mas o Message decodificado só tem o envelope de
// estabelecimento de sessão de grupo — nenhum payload de mídia.
func mensagemMidiaVazia() *waE2E.Message {
	return &waE2E.Message{
		SenderKeyDistributionMessage: &waE2E.SenderKeyDistributionMessage{},
	}
}

func mensagemComVideo() *waE2E.Message {
	return &waE2E.Message{VideoMessage: &waE2E.VideoMessage{}}
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
// TestDedup_SegundaCopiaComMidiaNaoEhSuprimidaQuandoPrimeiraEraVazia trava a
// CAUSA da F358 medida em campo (`POST /status/set/video`,
// `POST /status/set/audio`): a primeira cópia de uma mensagem de status
// chega tipada como mídia (`type=media`, `media_type=video`) mas com o
// Message decodificado carregando só o SenderKeyDistributionMessage — o
// envelope de sessão de grupo, sem o vídeo em si. A F103 original
// suprimiria a segunda cópia (mesmo Type/MediaType/PushName, então o teste
// de "perdeu" nunca dispara) — e essa segunda cópia É a que carrega o vídeo
// de verdade. Sem esta correção, o vídeo nunca chega a quem recebe.
func TestDedup_SegundaCopiaComMidiaNaoEhSuprimidaQuandoPrimeiraEraVazia(t *testing.T) {
	limparDedup(t)

	primeiraSuprimida := mensagemJaProcessada("u1",
		eventoDedupComConteudo("M-F358", "media", "video", "FilaRápida", mensagemMidiaVazia()))
	if primeiraSuprimida {
		t.Fatal("a PRIMEIRA copia foi suprimida; nunca haveria uma segunda tentativa")
	}

	segundaSuprimida := mensagemJaProcessada("u1",
		eventoDedupComConteudo("M-F358", "media", "video", "FilaRápida", mensagemComVideo()))
	if segundaSuprimida {
		t.Error("a segunda copia (com o video de verdade) foi suprimida; e' exatamente o defeito da F358 — Type/MediaType identicos escondem que so' a segunda copia tem payload")
	}
}

// TestDedup_SegundaCopiaSemMidiaContinuaSuprimidaQuandoPrimeiraJaTinha e' o
// controle: quando a PRIMEIRA copia ja' tinha midia utilizavel, uma segunda
// copia (com ou sem midia) continua suprimida como antes — a correcao da
// F358 nao reabre a supressao da F103 para o caso comum.
func TestDedup_SegundaCopiaSemMidiaContinuaSuprimidaQuandoPrimeiraJaTinha(t *testing.T) {
	limparDedup(t)

	mensagemJaProcessada("u1",
		eventoDedupComConteudo("M-F358-B", "media", "video", "FilaRápida", mensagemComVideo()))

	segundaSuprimida := mensagemJaProcessada("u1",
		eventoDedupComConteudo("M-F358-B", "media", "video", "FilaRápida", mensagemComVideo()))
	if !segundaSuprimida {
		t.Error("uma segunda copia identica, com a primeira ja' tendo midia, deveria continuar suprimida (politica F103 intacta)")
	}
}

// TestDedup_MensagemDeTextoNuncaContaComoMidiaVazia trava que a checagem de
// F358 so se aplica a mensagens tipadas como midia — uma mensagem de texto
// (Type != "media") nunca deve ser tratada como "sem conteudo utilizavel",
// mesmo com Message == nil, ou a supressao normal do F103 quebraria para
// texto tambem.
func TestDedup_MensagemDeTextoNuncaContaComoMidiaVazia(t *testing.T) {
	limparDedup(t)
	buf := capturarLog(t)

	mensagemJaProcessada("u1", eventoDedup("M-TEXTO", "text", "", "Ana"))
	segundaSuprimida := mensagemJaProcessada("u1", eventoDedup("M-TEXTO", "text", "", "Ana"))

	if !segundaSuprimida {
		t.Error("a segunda copia de uma mensagem de TEXTO deveria continuar suprimida — F358 e' so' sobre mensagens de midia")
	}
	if strings.Contains(buf.String(), "F358") {
		t.Errorf("o log de F358 disparou para uma mensagem de texto, que nao tem este modo de falha: %s", buf.String())
	}
}

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
