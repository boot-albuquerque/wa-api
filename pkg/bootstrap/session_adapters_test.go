package bootstrap

import (
	"bytes"
	"context"
	"strings"
	"testing"
	wanoise "wa-api/internal/wa-noise"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// TestSessionEventDispatcher_SemUserEventHandler: o dispatcher resolve userID ->
// *UserEventHandler pelo clientManager. Quando não há handle registrado o evento é
// DESCARTADO com aviso, e não pode virar erro — o orchestrator despacha em
// caminhos de teardown (QRTimeout, ConnectFailure) onde o handle já pode ter
// sido removido, e um erro ali aborta o teardown pela metade.
func TestSessionEventDispatcher_SemUserEventHandler(t *testing.T) {
	clientManager.DeleteUserClient("user-sem-handle")

	var buf bytes.Buffer
	orig := log.Logger
	log.Logger = zerolog.New(&buf)
	defer func() { log.Logger = orig }()

	err := NewSessionEventDispatcher().Dispatch(context.Background(), "user-sem-handle", "QR", map[string]any{"event": "code"})
	if err != nil {
		t.Fatalf("Dispatch devolveu erro %v, quero nil (evento descartado, não falha)", err)
	}
	if !strings.Contains(buf.String(), "no UserEventHandler registered") {
		t.Fatalf("esperava aviso de handle ausente, log = %s", buf.String())
	}
}

// handleForaDoTipo satisfaz a interface UserEventHandler do clientManager sem ser um
// *bootstrap.UserEventHandler — o caso que a type assertion do dispatcher defende.
type handleForaDoTipo struct{}

func (handleForaDoTipo) GetWAClient() *wanoise.Client { return nil }
func (handleForaDoTipo) GetUserID() string            { return "user-tipo-errado" }

// TestSessionEventDispatcher_HandleDeOutroTipo: se o registro contiver algo
// que não é *bootstrap.UserEventHandler, o dispatcher loga e desiste em vez de
// entrar em pânico com uma asserção de tipo crua.
func TestSessionEventDispatcher_HandleDeOutroTipo(t *testing.T) {
	clientManager.SetUserClient("user-tipo-errado", handleForaDoTipo{})
	defer clientManager.DeleteUserClient("user-tipo-errado")

	var buf bytes.Buffer
	orig := log.Logger
	log.Logger = zerolog.New(&buf)
	defer func() { log.Logger = orig }()

	err := NewSessionEventDispatcher().Dispatch(context.Background(), "user-tipo-errado", "QR", nil)
	if err != nil {
		t.Fatalf("Dispatch devolveu erro %v, quero nil", err)
	}
	if !strings.Contains(buf.String(), "not *bootstrap.UserEventHandler") {
		t.Fatalf("esperava aviso de tipo inesperado, log = %s", buf.String())
	}
}

// TestSessionAttachHook_AttachSemClienteRegistrado: Attach depende do
// *wa-noise.Client que o SessionRegistry publica no clientManager. Sem ele,
// falhar alto é obrigatório — montar um UserEventHandler com WAClient nil registraria
// um handle que entra em pânico no primeiro evento recebido.
func TestSessionAttachHook_AttachSemClienteRegistrado(t *testing.T) {
	clientManager.DeleteWaNoiseClient("user-sem-cliente")

	err := NewSessionAttachHook(&server{}).Attach(context.Background(), "user-sem-cliente", "token")
	if err == nil {
		t.Fatal("Attach sem *wanoise.Client registrado deveria falhar")
	}
	if !strings.Contains(err.Error(), "user-sem-cliente") {
		t.Fatalf("erro não identifica o userID: %v", err)
	}
}

// TestSessionAttachHook_DetachIdempotente: Detach sinaliza o kill-channel, e
// o orchestrator o chama em caminhos que podem já ter sido desmontados
// (QRTimeout depois de ConnectFailure). Sinalizar um userID desconhecido tem
// de ser no-op silencioso, não pânico nem bloqueio.
func TestSessionAttachHook_DetachIdempotente(t *testing.T) {
	h := NewSessionAttachHook(&server{})

	feito := make(chan struct{})
	go func() {
		defer close(feito)
		h.Detach("user-nunca-anexado")
		h.Detach("user-nunca-anexado")
	}()

	select {
	case <-feito:
	case <-context.Background().Done():
		t.Fatal("Detach bloqueou")
	}
}
