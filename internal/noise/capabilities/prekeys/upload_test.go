package prekeys

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/types"
	"wa-api/internal/noise/security/keys"
)

func TestGetServerCount(t *testing.T) {
	tr := newFakeTransport(t)
	tr.enqueueIQ(countNode("17"), nil)

	got, err := GetServerCount(t.Context(), tr)
	if err != nil {
		t.Fatalf("GetServerCount: %v", err)
	}
	if got != 17 {
		t.Errorf("= %d, esperado 17", got)
	}
	call := tr.calls()[0]
	if call.Namespace != "encrypt" || call.Type != IQGet || call.To != types.ServerJID {
		t.Errorf("IQ = %+v, esperado get/encrypt para o servidor", call)
	}
}

// O erro de transporte e' embrulhado, nao repassado cru: a mensagem diz de que
// consulta veio.
func TestGetServerCountTransportError(t *testing.T) {
	tr := newFakeTransport(t)
	sentinel := errors.New("boom")
	tr.enqueueIQ(nil, sentinel)

	got, err := GetServerCount(t.Context(), tr)
	if got != 0 {
		t.Errorf("count = %d, esperado 0", got)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("erro = %v, esperado embrulhar %v", err, sentinel)
	}
	if !strings.Contains(err.Error(), "failed to get prekey count on server") {
		t.Errorf("erro = %q, sem o contexto da consulta", err)
	}
}

// Um "value" nao numerico vira erro do AttrGetter, e nao um 0 silencioso.
func TestGetServerCountInvalidValue(t *testing.T) {
	tr := newFakeTransport(t)
	tr.enqueueIQ(countNode("nao-e-numero"), nil)

	if _, err := GetServerCount(t.Context(), tr); err == nil {
		t.Fatal("aceitou value nao numerico")
	}
}

func TestUploadSendsAndMarks(t *testing.T) {
	tr := newFakeTransport(t)
	generated := []*keys.PreKey{testPreKey(t, 1, false), testPreKey(t, 2, false)}
	tr.preKeyStore().genKeys = generated
	tr.enqueueIQ(&waBinary.Node{Tag: "iq"}, nil)

	Upload(t.Context(), tr, false)

	if got := tr.preKeyStore().genCount; got != WantedCount {
		t.Errorf("pediu %d chaves, esperado WantedCount (%d)", got, WantedCount)
	}
	calls := tr.calls()
	if len(calls) != 1 {
		t.Fatalf("%d IQs enviados, esperado 1", len(calls))
	}
	if calls[0].Type != IQSet || calls[0].Namespace != "encrypt" {
		t.Errorf("IQ = %+v, esperado set/encrypt", calls[0])
	}
	content, ok := calls[0].Content.([]waBinary.Node)
	if !ok {
		t.Fatalf("conteudo do IQ = %T, esperado []waBinary.Node", calls[0].Content)
	}
	// registration, type, identity, list, skey
	if len(content) != 5 {
		t.Fatalf("%d filhos no IQ, esperado 5", len(content))
	}
	list, ok := content[3].Content.([]waBinary.Node)
	if !ok || len(list) != len(generated) {
		t.Errorf("<list> tem %v, esperado %d nos de prekey", content[3].Content, len(generated))
	}
	if content[4].Tag != "skey" {
		t.Errorf("ultimo filho = %q, esperado a signed prekey (\"skey\")", content[4].Tag)
	}
	if tr.preKeyStore().markCalls != 1 || tr.preKeyStore().markedUpTo != 2 {
		t.Errorf("MarkPreKeysAsUploaded(%d) chamado %dx, esperado 1x com 2",
			tr.preKeyStore().markedUpTo, tr.preKeyStore().markCalls)
	}
	if tr.state.LastUpload().IsZero() {
		t.Error("lastUpload deveria ter sido registrado")
	}
}

// O upload inicial pede InitialCount, e nao WantedCount: a conta nasce com
// estoque para varias sessoes.
func TestUploadInitialUsesInitialCount(t *testing.T) {
	tr := newFakeTransport(t)
	tr.preKeyStore().genKeys = []*keys.PreKey{testPreKey(t, 1, false)}
	tr.enqueueIQ(&waBinary.Node{Tag: "iq"}, nil)

	Upload(t.Context(), tr, true)

	if got := tr.preKeyStore().genCount; got != InitialCount {
		t.Errorf("pediu %d chaves, esperado InitialCount (%d)", got, InitialCount)
	}
}

// Dentro da janela de debounce, o upload reconfere a contagem no servidor: se
// ja' ha estoque, desiste sem gerar chave nenhuma.
func TestUploadDebounceCancelsWhenServerHasEnough(t *testing.T) {
	tr := newFakeTransport(t)
	tr.state.SetLastUpload(time.Now())
	tr.enqueueIQ(countNode("50"), nil)

	Upload(t.Context(), tr, false)

	if tr.preKeyStore().genCalls != 0 {
		t.Error("gerou prekeys apesar de o servidor ja' ter estoque")
	}
	if len(tr.calls()) != 1 {
		t.Errorf("%d IQs, esperado so' o de contagem", len(tr.calls()))
	}
}

// Dentro da janela de debounce mas com o servidor abaixo do desejado, o upload
// segue normalmente.
func TestUploadDebounceProceedsWhenServerIsLow(t *testing.T) {
	tr := newFakeTransport(t)
	tr.state.SetLastUpload(time.Now())
	tr.preKeyStore().genKeys = []*keys.PreKey{testPreKey(t, 5, false)}
	tr.enqueueIQ(countNode("1"), nil)
	tr.enqueueIQ(&waBinary.Node{Tag: "iq"}, nil)

	Upload(t.Context(), tr, false)

	if tr.preKeyStore().markCalls != 1 {
		t.Error("nao concluiu o upload apesar de o servidor estar com pouco estoque")
	}
}

// Um erro na contagem durante o debounce e' ignorado (o valor volta 0, abaixo
// de WantedCount) e o upload segue — e' o comportamento historico.
func TestUploadDebounceIgnoresCountError(t *testing.T) {
	tr := newFakeTransport(t)
	tr.state.SetLastUpload(time.Now())
	tr.preKeyStore().genKeys = []*keys.PreKey{testPreKey(t, 5, false)}
	tr.enqueueIQ(nil, errors.New("sem rede"))
	tr.enqueueIQ(&waBinary.Node{Tag: "iq"}, nil)

	Upload(t.Context(), tr, false)

	if tr.preKeyStore().markCalls != 1 {
		t.Error("erro de contagem deveria deixar o upload seguir")
	}
}

// Os tres erros do caminho de upload sao logados e engolidos, cada um parando
// o upload no seu ponto.
func TestUploadErrorPaths(t *testing.T) {
	t.Run("falha ao gerar prekeys", func(t *testing.T) {
		tr := newFakeTransport(t)
		tr.preKeyStore().genErr = errors.New("db fora do ar")

		Upload(t.Context(), tr, false)

		if len(tr.calls()) != 0 {
			t.Error("enviou IQ apesar de nao ter chaves")
		}
	})

	t.Run("falha ao enviar", func(t *testing.T) {
		tr := newFakeTransport(t)
		tr.preKeyStore().genKeys = []*keys.PreKey{testPreKey(t, 1, false)}
		tr.enqueueIQ(nil, errors.New("socket fechado"))

		Upload(t.Context(), tr, false)

		if tr.preKeyStore().markCalls != 0 {
			t.Error("marcou como enviadas apesar de o envio ter falhado")
		}
		if !tr.state.LastUpload().IsZero() {
			t.Error("registrou lastUpload apesar de o envio ter falhado")
		}
	})

	t.Run("falha ao marcar como enviadas", func(t *testing.T) {
		tr := newFakeTransport(t)
		tr.preKeyStore().genKeys = []*keys.PreKey{testPreKey(t, 1, false)}
		tr.preKeyStore().markErr = errors.New("db fora do ar")
		tr.enqueueIQ(&waBinary.Node{Tag: "iq"}, nil)

		Upload(t.Context(), tr, false)

		if !tr.state.LastUpload().IsZero() {
			t.Error("registrou lastUpload apesar de a marcacao ter falhado")
		}
	})
}

// O lock de upload serializa uploads concorrentes: nenhum par de chamadas se
// sobrepoe, e as duas concluem.
func TestUploadIsSerialized(t *testing.T) {
	const goroutines = 8
	tr := newFakeTransport(t)
	tr.preKeyStore().genKeys = []*keys.PreKey{testPreKey(t, 1, false)}
	// Dois IQs por upload: depois do primeiro, lastUpload esta' dentro da
	// janela de debounce, entao cada chamada seguinte faz uma contagem antes do
	// envio. A contagem devolve 0 (abaixo de WantedCount) e o upload segue.
	for range 2 * goroutines {
		tr.enqueueIQ(&waBinary.Node{Tag: "iq"}, nil)
	}

	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			Upload(t.Context(), tr, false)
		}()
	}
	wg.Wait()

	if got := tr.preKeyStore().markCalls; got != goroutines {
		t.Errorf("%d uploads concluidos, esperado %d", got, goroutines)
	}
}

// LockUpload/UnlockUpload sao o mesmo par que abria uploadPreKeys. O teste
// trava que o segundo Lock so' passa depois do Unlock do primeiro.
func TestLockUploadIsMutuallyExclusive(t *testing.T) {
	var s State
	s.LockUpload()
	entered := make(chan struct{})
	go func() {
		s.LockUpload()
		close(entered)
		s.UnlockUpload()
	}()
	select {
	case <-entered:
		t.Fatal("segunda goroutine entrou com o lock segurado")
	case <-time.After(20 * time.Millisecond):
	}
	s.UnlockUpload()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("segunda goroutine nao entrou depois do Unlock")
	}
}

// GetOrGenPreKeys e' contrato de interface publica (store.PreKeyStore) e nada
// nele proibe devolver fatia vazia sem erro. Antes da correcao da F51, isso
// virava preKeys[-1] la' no fim de Upload — e Upload roda em goroutine, sem
// recover, entao o panic derrubava o processo. Agora sai cedo, sem mandar um
// <list> vazio para o servidor.
func TestUploadComListaVaziaNaoEntraEmPanicNemEnviaIQ(t *testing.T) {
	for _, keys := range map[string][]*keys.PreKey{"nil": nil, "vazia": {}} {
		tr := newFakeTransport(t)
		tr.preKeyStore().genKeys = keys

		Upload(t.Context(), tr, false)

		if n := len(tr.calls()); n != 0 {
			t.Errorf("esperava nenhum IQ enviado, vieram %d", n)
		}
		if n := tr.preKeyStore().markCalls; n != 0 {
			t.Errorf("esperava nenhuma marcacao de upload, vieram %d", n)
		}
	}
}
