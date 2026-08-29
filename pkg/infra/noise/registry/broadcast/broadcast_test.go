package broadcast

import (
	"sync"
	"testing"

	"github.com/coder/websocket"

	clientpkg "wa-api/pkg/infra/noise/client"
)

// TestWriteTimeout_MenorQueRequest: o fan-out WS percorre as conexões em
// série, então seu teto por conexão tem de ser bem menor que o de uma
// requisição ao servidor do WhatsApp.
func TestWriteTimeout_MenorQueRequest(t *testing.T) {
	if writeTimeout <= 0 {
		t.Fatalf("writeTimeout = %v, want > 0: um timeout zero expira na hora", writeTimeout)
	}
	if writeTimeout >= clientpkg.RequestTimeout {
		t.Errorf("writeTimeout (%v) >= clientpkg.RequestTimeout (%v)", writeTimeout, clientpkg.RequestTimeout)
	}
}

func TestAddRemoveConta(t *testing.T) {
	r := New()
	c1, c2 := &websocket.Conn{}, &websocket.Conn{}

	r.Add("u", c1)
	r.Add("u", c2)
	if got := len(r.conns["u"]); got != 2 {
		t.Fatalf("apos dois Add = %d conexoes, esperado 2", got)
	}

	r.Remove("u", c1)
	if got := len(r.conns["u"]); got != 1 {
		t.Fatalf("apos Remove = %d conexoes, esperado 1", got)
	}

	// A ultima remocao tem de apagar a CHAVE, nao deixar um mapa vazio: um
	// mapa vazio por usuario que ja' se desconectou e' vazamento lento.
	r.Remove("u", c2)
	if _, ok := r.conns["u"]; ok {
		t.Error("a chave do usuario sobreviveu a' remocao da ultima conexao")
	}
}

func TestRemoveDeUsuarioDesconhecidoNaoEntraEmPanico(t *testing.T) {
	New().Remove("nunca-existiu", &websocket.Conn{})
}

// TestRegistryConcorrente: Add, Remove e Broadcast simultâneos sobre chaves
// sobrepostas.
//
// Broadcast roda contra um usuário SEM conexão viva: com uma conexão
// registrada ele tentaria wsjson.Write num *websocket.Conn zerado, que não é
// socket. O que se exercita é a leitura concorrente do mapa, que é o ponto
// de contenção.
func TestRegistryConcorrente(t *testing.T) {
	r := New()
	const workers, iters = 8, 100
	users := []string{"u0", "u1", "u2"}

	conns := make([]*websocket.Conn, workers)
	for i := range conns {
		conns[i] = &websocket.Conn{}
	}

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(3)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				r.Add(users[(w+i)%len(users)], conns[w])
			}
		}(w)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				r.Remove(users[(w+i)%len(users)], conns[w])
			}
		}(w)
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				r.Broadcast("usuario-sem-conexao", map[string]string{"k": "v"})
			}
		}()
	}
	wg.Wait()

	r.Add("depois", &websocket.Conn{})
	if len(r.conns["depois"]) != 1 {
		t.Error("o registry nao aceitou escrita depois do acesso concorrente")
	}
}
