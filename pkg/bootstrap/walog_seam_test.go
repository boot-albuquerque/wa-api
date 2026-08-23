package bootstrap

import (
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog/log"

	wanoise "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types/events"
	"wa-api/pkg/infra/wa-noise/observability/walog"
)

// TestWalogSeam_ErroDoSDKSaiSemWadebug exercita os dois lados do seam de log
// numa execução só, sem mock de logger nenhum:
//
//  1. O caminho que o wa-noise percorre: bridge.Sub(...).Errorf(...), que é
//     literalmente o que internal/wa-noise/client.go faz com o sublogger que
//     recebe em NewClient. Sem --wadebug — ou seja, no default de produção,
//     que antes desta mudança era waLog.Noop e descartava o registro.
//
//  2. O caminho da aplicação: o handleEvent real, com um events.Connected
//     real, sobre um *wanoise.Client real construído com o bridge como
//     logger. É o handler de produção, não um stub.
//
// Ambos escrevem no mesmo buffer, que é o ponto: SDK e aplicação saem no
// mesmo sink JSON.
func TestWalogSeam_ErroDoSDKSaiSemWadebug(t *testing.T) {
	buf := captureLogInto(t)

	// ParseLevel("") = sem --wadebug. Warn e Error têm de sair mesmo assim.
	bridge := walog.New(log.Logger, walog.ModuleClient, walog.ParseLevel(""))

	// --- Lado 1: o SDK reportando um erro por um sublogger ---
	errSentinel := errors.New("bad mac")
	bridge.Sub("Recv").Errorf("decrypt failed: %v", errSentinel)

	recs := decodeRecords(t, buf)
	if len(recs) != 1 {
		t.Fatalf("o SDK emitiu %d registros sem --wadebug, queria 1: %v", len(recs), recs)
	}
	if recs[0]["level"] != "error" {
		t.Errorf("level = %v, queria \"error\"", recs[0]["level"])
	}
	if recs[0].str(walog.FieldModule) != "Client/Recv" {
		t.Errorf("%s = %v, queria \"Client/Recv\"", walog.FieldModule, recs[0].str(walog.FieldModule))
	}
	if recs[0]["message"] != "decrypt failed: bad mac" {
		t.Errorf("message = %v", recs[0]["message"])
	}

	// --- Lado 2: o handler de eventos real ---
	buf.Reset()

	// UserInfoCache populado com Events vazio: sendEventWithWebHook resolve
	// a assinatura pelo cache, conclui que o usuário não assina "Connected"
	// e retorna antes de qualquer entrega — sem HTTP.
	//
	// O banco, porém, é necessário desde a F82: handleConnected passou a
	// gravar users.connected=1 ANTES da guarda de pushname, e este handler
	// tem pushname vazio. Antes da F82 a guarda saía cedo e a escrita nunca
	// acontecia — era só por isso que o teste rodava sem banco.
	// Chave por userID (F100), nao por token.
	appCtx.UserInfoCache.Set(walogSeamUser, Values{M: map[string]string{
		"Id": walogSeamUser, "Webhook": "", "Events": "", "Jid": "", "Name": "",
	}}, 0)
	t.Cleanup(func() { appCtx.UserInfoCache.Delete(walogSeamUser) })

	sqlDB := schemaDB(t)
	seedUser(t, sqlDB, walogSeamUser, walogSeamToken, "")
	evh := &UserEventHandler{
		UserID:   walogSeamUser,
		Token:    walogSeamToken,
		DB:       sqlDB,
		WAClient: wanoise.NewClient(&store.Device{Log: bridge.Sub("Device")}, bridge),
	}
	evh.handleEvent(&events.Connected{})

	// handleEvent despacha o webhook global e o RabbitMQ de forma ASSINCRONA,
	// e essas goroutines escrevem no logger global — o mesmo que o Cleanup
	// deste teste vai trocar de volta. Sem esperar, o `-race` acusa (medido):
	// o worker do pool escrevendo no buffer enquanto o Cleanup restaura
	// log.Logger.
	//
	// A corrida e do TESTE, nao do codigo de producao: la o logger global nao
	// e trocado no meio da execucao. Mas ela so apareceu depois da F100 —
	// antes, o teste semeava o cache sob o token e a busca por userID errava,
	// entao o caminho assincrono nem era alcancado. Consertar a chave sem
	// esperar o despacho trocaria um defeito silencioso por um teste instavel.
	esperarDespachoDrenar(t)

	recs = decodeRecords(t, buf)
	if len(recs) == 0 {
		t.Fatal("handleEvent nao emitiu registro nenhum para events.Connected")
	}
	var sawSubscription bool
	for _, rec := range recs {
		if rec.str("userID") == walogSeamUser {
			sawSubscription = true
		}
	}
	if !sawSubscription {
		t.Errorf("nenhum registro do handler citou userID=%q: %v", walogSeamUser, recs)
	}
}

const (
	walogSeamUser  = "walog-seam-user"
	walogSeamToken = "walog-seam-token"
)

// esperarDespachoDrenar espera o pool de despacho ficar ocioso.
//
// Existe porque vários caminhos de evento despacham trabalho assíncrono que
// sobrevive ao teste que o disparou. Um teste que troca estado global no
// Cleanup — logger, cache, configuração — corre com esse trabalho, e o sintoma
// é falha intermitente sob `-race`, longe da causa.
//
// Com PRAZO, nunca espera indefinida (ARMADILHAS 16).
func esperarDespachoDrenar(t *testing.T) {
	t.Helper()

	if dispatch == nil {
		return // o pool nem chegou a ser criado
	}

	prazo := time.After(5 * time.Second)
	for {
		if emVoo, _, _, _ := dispatch.Metrics(); emVoo == 0 {
			return
		}
		select {
		case <-prazo:
			emVoo, _, _, _ := dispatch.Metrics()
			t.Fatalf("o pool de despacho nao drenou: %d trabalhos ainda em voo", emVoo)
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}
