package registry

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/coder/websocket"
	"github.com/go-resty/resty/v2"

	"wa-api/internal/noise"
	port "wa-api/pkg/application/contracts"
)

// Este arquivo fixa o comportamento concorrente do ClientManager ANTES da
// quebra em sub-registries, para que a quebra seja comparavel contra um
// baseline e nao contra a memoria de quem a fez.
//
// O que ele NAO afirma: que os pares (sessions, noiseClients) e
// (userClients, pollOptions) sejam observaveis atomicamente. Nao sao — nenhum
// metodo publico le dois mapas, entao qualquer leitor ja' precisa de duas
// chamadas com dois RLocks e ja' pode intercalar hoje. Register e
// DeleteUserClient apenas ESCREVEM dois mapas sob o mesmo lock.
//
// O que ele afirma e' o que de fato importa preservar: nenhum acesso
// concorrente as' 21 operacoes produz data race, e o estado resultante
// continua coerente. E' esse contrato que a quebra em sub-registries tem
// que manter, e e' por isso que este teste roda sob -race (o gate `make
// test` ja' passa -race em toda a arvore).

// hammer roda fn em n goroutines, cada uma iterando iters vezes, e espera
// todas. E' o formato usado por todos os testes de corrida deste pacote.
func hammer(n, iters int, fn func(worker, iter int)) {
	var wg sync.WaitGroup
	wg.Add(n)
	for w := 0; w < n; w++ {
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				fn(worker, i)
			}
		}(w)
	}
	wg.Wait()
}

const (
	concurrencyWorkers = 8
	concurrencyIters   = 50
)

// raceSession e' um port.Session INERTE: sem estado, sem contadores, sem
// nada que possa ser escrito de duas goroutines. Deliberadamente nao e'
// contractsfake.Session — aquela grava cada chamada num Recorder, e sob
// -race a corrida detectada seria a da propria fake, escondendo a do
// ClientManager, que e' o objeto sob teste.
//
// Implementa NoiseClient() devolvendo um ponteiro nao-nil para que
// Register exercite de fato a escrita nos DOIS mapas (sessions e
// noiseClients); devolvendo nil, o `if client != nil` de Register
// pularia a segunda escrita e o teste nao cobriria o caminho que a quebra
// em sub-registries precisa preservar.
type raceSession struct{ client *noise.Client }

func (s *raceSession) NoiseClient() *noise.Client    { return s.client }
func (s *raceSession) HasCredentials() bool          { return false }
func (s *raceSession) JID() (string, bool)           { return "", false }
func (s *raceSession) Connect(context.Context) error { return nil }
func (s *raceSession) Disconnect()                   {}
func (s *raceSession) IsConnected() bool             { return false }
func (s *raceSession) Logout(context.Context) error  { return nil }
func (s *raceSession) IsLoggedIn() bool              { return false }
func (s *raceSession) SetProxy(port.ProxyConfig) error {
	return nil
}
func (s *raceSession) Pair(context.Context) (<-chan port.PairingEvent, error) {
	return nil, nil
}
func (s *raceSession) Subscribe(func(port.SessionEvent)) (func(), error) {
	return func() {}, nil
}

var inertSession = &raceSession{client: &noise.Client{}}

var _ port.Session = (*raceSession)(nil)

// TestClientManagerAcessoConcorrenteNaoTemCorrida exercita todas as
// operacoes do ClientManager simultaneamente, de goroutines distintas,
// sobre chaves sobrepostas — chaves distintas por worker esconderiam
// exatamente as corridas que interessam, porque cada worker mexeria em um
// bucket proprio.
func TestClientManagerAcessoConcorrenteNaoTemCorrida(t *testing.T) {
	cm := NewClientManager()

	// Chaves propositalmente poucas: os workers colidem entre si.
	userIDs := []string{"u0", "u1", "u2"}
	uid := func(w, i int) string { return userIDs[(w+i)%len(userIDs)] }

	var wg sync.WaitGroup
	run := func(fn func(w, i int)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hammer(concurrencyWorkers, concurrencyIters, fn)
		}()
	}

	// noiseClients — escrita, leitura, remocao, snapshot, contagem e
	// iteracao, todos concorrentes entre si.
	run(func(w, i int) { cm.SetNoiseClient(uid(w, i), nil) })
	run(func(w, i int) { _ = cm.GetNoiseClient(uid(w, i)) })
	run(func(w, i int) { cm.DeleteNoiseClient(uid(w, i)) })
	run(func(w, i int) { _ = cm.GetAllClients() })
	run(func(w, i int) { _ = cm.GetNoiseClientsCount() })
	run(func(w, i int) { cm.IterateNoiseClients(func(*noise.Client) bool { return true }) })

	// userClients + pollOptions — DeleteUserClient apaga os dois.
	run(func(w, i int) { cm.SetUserClient(uid(w, i), &fakeUserClient{}) })
	run(func(w, i int) { _ = cm.GetUserClient(uid(w, i)) })
	run(func(w, i int) { cm.DeleteUserClient(uid(w, i)) })
	run(func(w, i int) {
		cm.SetPollOptions(uid(w, i), fmt.Sprintf("msg-%d", i%4), []string{"a", "b"})
	})
	run(func(w, i int) { _ = cm.GetPollOptions(uid(w, i), fmt.Sprintf("msg-%d", i%4)) })

	// httpClients — sem acesso cruzado a nenhum outro mapa.
	run(func(w, i int) { cm.SetHTTPClient(uid(w, i), resty.New()) })
	run(func(w, i int) { _ = cm.GetHTTPClient(uid(w, i)) })
	run(func(w, i int) { cm.DeleteHTTPClient(uid(w, i)) })

	// wsConns — idem. BroadcastToUser roda contra usuarios SEM conexao
	// viva: com uma conexao registrada ele tentaria wsjson.Write num
	// *websocket.Conn zerado, que nao e' um socket. O que se exercita aqui
	// e' a leitura concorrente do mapa, que e' o ponto de contencao.
	conns := make([]*websocket.Conn, concurrencyWorkers)
	for i := range conns {
		conns[i] = &websocket.Conn{}
	}
	run(func(w, i int) { cm.AddWSConn(uid(w, i), conns[w]) })
	run(func(w, i int) { cm.RemoveWSConn(uid(w, i), conns[w]) })
	run(func(w, i int) { cm.BroadcastToUser("usuario-sem-conexao", map[string]string{"k": "v"}) })

	// sessions — Register escreve sessions E noiseClients.
	run(func(w, i int) { cm.Register(uid(w, i), inertSession) })
	run(func(w, i int) { _, _ = cm.Get(uid(w, i)) })
	run(func(w, i int) { cm.Unregister(uid(w, i)) })

	wg.Wait()

	// Depois da tempestade o manager continua utilizavel — o que descarta
	// um mapa deixado nil por alguma remocao concorrente.
	cm.SetNoiseClient("depois", nil)
	if _, ok := cm.GetAllClients()["depois"]; !ok {
		t.Error("o manager nao aceitou escrita depois do acesso concorrente")
	}
	cm.SetPollOptions("depois", "m", []string{"x"})
	if got := cm.GetPollOptions("depois", "m"); len(got) != 1 || got[0] != "x" {
		t.Errorf("pollOptions apos concorrencia = %v, esperado [x]", got)
	}
}

// TestSetPollOptionsCopiaOSliceDoChamador trava a copia defensiva de
// SetPollOptions. Sem ela, o chamador continuaria dono do array por baixo do
// slice guardado e uma escrita dele seria uma corrida com qualquer leitor —
// invisivel para o -race do teste acima, porque acontece FORA do lock.
func TestSetPollOptionsCopiaOSliceDoChamador(t *testing.T) {
	cm := NewClientManager()
	original := []string{"sim", "nao"}
	cm.SetPollOptions("u", "m", original)

	original[0] = "mutado pelo chamador"

	got := cm.GetPollOptions("u", "m")
	if got[0] != "sim" {
		t.Errorf("opcao guardada = %q, esperado \"sim\": SetPollOptions nao copiou o slice", got[0])
	}
}
