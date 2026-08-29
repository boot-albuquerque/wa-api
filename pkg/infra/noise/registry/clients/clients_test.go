package clients

import (
	"context"
	"sync"
	"testing"

	"wa-api/internal/noise"
	port "wa-api/pkg/application/contracts"
)

// inertSession é um port.Session INERTE: sem estado, sem contadores, sem
// nada que possa ser escrito de duas goroutines.
//
// Deliberadamente não é contractsfake.Session — aquela grava cada chamada
// num Recorder, e sob -race a corrida detectada seria a da própria fake,
// escondendo a do Registry, que é o objeto sob teste.
type inertSession struct{ client *noise.Client }

func (s *inertSession) NoiseClient() *noise.Client    { return s.client }
func (s *inertSession) HasCredentials() bool          { return false }
func (s *inertSession) JID() (string, bool)           { return "", false }
func (s *inertSession) Connect(context.Context) error { return nil }
func (s *inertSession) Disconnect()                   {}
func (s *inertSession) IsConnected() bool             { return false }
func (s *inertSession) Logout(context.Context) error  { return nil }
func (s *inertSession) IsLoggedIn() bool              { return false }
func (s *inertSession) SetProxy(port.ProxyConfig) error {
	return nil
}
func (s *inertSession) Pair(context.Context) (<-chan port.PairingEvent, error) {
	return nil, nil
}
func (s *inertSession) Subscribe(func(port.SessionEvent)) (func(), error) {
	return func() {}, nil
}

var _ port.Session = (*inertSession)(nil)

// TestRegisterPublicaOClienteSubjacente trava o acoplamento que decidiu o
// desenho deste pacote: sessions e clientes do SDK vivem no mesmo Registry,
// sob o mesmo lock, porque Register escreve nos dois. Se alguém separar os
// dois mapas em pacotes distintos, este teste é o que denuncia.
func TestRegisterPublicaOClienteSubjacente(t *testing.T) {
	r := New()
	c := &noise.Client{}
	r.Register("u", &inertSession{client: c})

	if _, ok := r.Session("u"); !ok {
		t.Error("Register nao gravou a Session")
	}
	if got := r.GetClient("u"); got != c {
		t.Errorf("Register nao publicou o cliente do SDK: GetClient = %v", got)
	}
}

// TestRegisterComSessionSemClienteNaoGravaNil: o `if c != nil` de Register
// existe para não publicar um cliente nulo — quem depois fizer GetClient
// receberia nil sem saber se é "não registrado" ou "registrado como nil".
func TestRegisterComSessionSemClienteNaoGravaNil(t *testing.T) {
	r := New()
	r.Register("u", &inertSession{client: nil})

	if _, ok := r.Session("u"); !ok {
		t.Error("a Session deveria ter sido gravada mesmo sem cliente")
	}
	if _, existe := r.Snapshot()["u"]; existe {
		t.Error("Register gravou uma entrada de cliente para uma Session sem cliente")
	}
}

// TestUnregisterNaoRemoveOCliente: Unregister mexe só em sessions. O ciclo
// de vida do cliente do SDK é governado por DeleteClient, e confundir os
// dois derrubaria o cliente de quem só desregistrou a Session.
func TestUnregisterNaoRemoveOCliente(t *testing.T) {
	r := New()
	c := &noise.Client{}
	r.Register("u", &inertSession{client: c})

	r.Unregister("u")

	if _, ok := r.Session("u"); ok {
		t.Error("Unregister nao removeu a Session")
	}
	if r.GetClient("u") != c {
		t.Error("Unregister removeu o cliente do SDK, que nao e' dele")
	}
}

func TestSnapshotEhCopia(t *testing.T) {
	r := New()
	r.SetClient("u", nil)

	snap := r.Snapshot()
	snap["intruso"] = nil

	if _, existe := r.Snapshot()["intruso"]; existe {
		t.Error("escrever no snapshot alterou o registry: Snapshot devolveu o mapa vivo")
	}
}

func TestCountEIterate(t *testing.T) {
	r := New()
	a, b := &noise.Client{}, &noise.Client{}
	r.SetClient("a", a)
	r.SetClient("b", b)

	if got := r.Count(); got != 2 {
		t.Errorf("Count = %d, esperado 2", got)
	}

	visitados := 0
	r.Iterate(func(*noise.Client) bool { visitados++; return true })
	if visitados != 2 {
		t.Errorf("Iterate visitou %d, esperado 2", visitados)
	}

	// Devolver false tem de PARAR a iteração, não só pular o item.
	visitados = 0
	r.Iterate(func(*noise.Client) bool { visitados++; return false })
	if visitados != 1 {
		t.Errorf("Iterate com callback false visitou %d, esperado 1", visitados)
	}
}

func TestGetClientDeUsuarioDesconhecidoDevolveNil(t *testing.T) {
	if got := New().GetClient("nunca-existiu"); got != nil {
		t.Errorf("= %v, esperado nil", got)
	}
}

// TestRegistryConcorrente: as nove operações simultâneas sobre chaves
// sobrepostas, com Register cruzando os dois mapas.
func TestRegistryConcorrente(t *testing.T) {
	r := New()
	const workers, iters = 8, 50
	users := []string{"u0", "u1", "u2"}
	uid := func(w, i int) string { return users[(w+i)%len(users)] }
	sess := &inertSession{client: &noise.Client{}}

	var wg sync.WaitGroup
	run := func(fn func(w, i int)) {
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				for i := 0; i < iters; i++ {
					fn(w, i)
				}
			}(w)
		}
	}

	run(func(w, i int) { r.Register(uid(w, i), sess) })
	run(func(w, i int) { r.Unregister(uid(w, i)) })
	run(func(w, i int) { _, _ = r.Session(uid(w, i)) })
	run(func(w, i int) { r.SetClient(uid(w, i), nil) })
	run(func(w, i int) { _ = r.GetClient(uid(w, i)) })
	run(func(w, i int) { r.DeleteClient(uid(w, i)) })
	run(func(w, i int) { _ = r.Snapshot() })
	run(func(w, i int) { _ = r.Count() })
	run(func(w, i int) { r.Iterate(func(*noise.Client) bool { return true }) })

	wg.Wait()

	c := &noise.Client{}
	r.SetClient("depois", c)
	if r.GetClient("depois") != c {
		t.Error("o registry nao aceitou escrita depois do acesso concorrente")
	}
}
