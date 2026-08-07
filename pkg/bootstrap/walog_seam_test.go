package bootstrap

import (
	"bytes"
	"errors"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	wanoise "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types/events"
	"wa-api/pkg/infra/wa-noise/walog"
)

// TestWalogSeam_ErroDoSDKSaiSemWadebug exercita os dois lados do seam de log
// numa execução só, sem mock de logger nenhum:
//
//  1. O caminho que o wa-noise percorre: bridge.Sub(...).Errorf(...), que é
//     literalmente o que internal/wa-noise/client.go faz com o sublogger que
//     recebe em NewClient. Sem --wadebug — ou seja, no default de produção,
//     que antes desta mudança era waLog.Noop e descartava o registro.
//
//  2. O caminho da aplicação: o myEventHandler real, com um events.Connected
//     real, sobre um *wa-noise.Client real construído com o bridge como
//     logger. É o handler de produção, não um stub.
//
// Ambos escrevem no mesmo buffer, que é o ponto: SDK e aplicação saem no
// mesmo sink JSON.
func TestWalogSeam_ErroDoSDKSaiSemWadebug(t *testing.T) {
	var buf bytes.Buffer
	orig := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = orig })

	// ParseLevel("") = sem --wadebug. Warn e Error têm de sair mesmo assim.
	bridge := walog.New(log.Logger, walog.ModuleClient, walog.ParseLevel(""))

	// --- Lado 1: o SDK reportando um erro por um sublogger ---
	errSentinel := errors.New("bad mac")
	bridge.Sub("Recv").Errorf("decrypt failed: %v", errSentinel)

	recs := decodeRecords(t, &buf)
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
	// e retorna antes de qualquer entrega — sem banco, sem HTTP.
	appCtx.UserInfoCache.Set(walogSeamToken, Values{M: map[string]string{
		"Id": walogSeamUser, "Webhook": "", "Events": "", "Jid": "", "Name": "",
	}}, 0)
	t.Cleanup(func() { appCtx.UserInfoCache.Delete(walogSeamToken) })

	mycli := &MyClient{
		UserID:   walogSeamUser,
		Token:    walogSeamToken,
		WAClient: wanoise.NewClient(&store.Device{Log: bridge.Sub("Device")}, bridge),
	}
	mycli.myEventHandler(&events.Connected{})

	recs = decodeRecords(t, &buf)
	if len(recs) == 0 {
		t.Fatal("myEventHandler nao emitiu registro nenhum para events.Connected")
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
