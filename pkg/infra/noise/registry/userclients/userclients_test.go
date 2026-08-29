package userclients

import (
	"fmt"
	"sync"
	"testing"

	"wa-api/internal/noise"
)

type fakeUserClient struct{ userID string }

func (f *fakeUserClient) GetWAClient() *noise.Client { return nil }
func (f *fakeUserClient) GetUserID() string          { return f.userID }

func TestSetGetDelete(t *testing.T) {
	r := New()
	r.Set("u", &fakeUserClient{userID: "u"})
	if got := r.Get("u"); got == nil || got.GetUserID() != "u" {
		t.Fatalf("Get = %v, esperado o cliente gravado", got)
	}
	r.Delete("u")
	if r.Get("u") != nil {
		t.Error("Delete nao removeu o cliente")
	}
}

// TestDeleteDescartaAsEnquetesDoUsuario trava o acoplamento que decidiu o
// desenho deste pacote: UserClient e o cache de enquetes vivem no mesmo
// Registry, sob o mesmo lock, porque Delete apaga os dois. Se alguém
// separar os dois mapas em pacotes distintos, este teste é o que denuncia.
func TestDeleteDescartaAsEnquetesDoUsuario(t *testing.T) {
	r := New()
	r.Set("u", &fakeUserClient{userID: "u"})
	r.SetPollOptions("u", "msg", []string{"sim", "nao"})

	r.Delete("u")

	if got := r.GetPollOptions("u", "msg"); got != nil {
		t.Errorf("enquetes apos Delete = %v, esperado nil", got)
	}
}

// TestDeleteNaoAfetaOutroUsuario: o descarte é por userID, não global.
func TestDeleteDeUmNaoApagaEnqueteDeOutro(t *testing.T) {
	r := New()
	r.SetPollOptions("a", "m", []string{"x"})
	r.SetPollOptions("b", "m", []string{"y"})

	r.Delete("a")

	if got := r.GetPollOptions("b", "m"); len(got) != 1 || got[0] != "y" {
		t.Errorf("enquete de b = %v, esperado [y]", got)
	}
}

// TestSetPollOptionsCopiaOSliceDoChamador trava a cópia defensiva. Sem ela,
// o chamador continuaria dono do array por baixo do slice guardado, e uma
// escrita dele seria corrida com qualquer leitor daqui — invisível para o
// -race dos testes de concorrência, porque acontece FORA do lock.
func TestSetPollOptionsCopiaOSliceDoChamador(t *testing.T) {
	r := New()
	original := []string{"sim", "nao"}
	r.SetPollOptions("u", "m", original)

	original[0] = "mutado pelo chamador"

	if got := r.GetPollOptions("u", "m"); got[0] != "sim" {
		t.Errorf("opcao guardada = %q, esperado \"sim\": SetPollOptions nao copiou", got[0])
	}
}

func TestGetPollOptionsDeUsuarioDesconhecidoDevolveNil(t *testing.T) {
	if got := New().GetPollOptions("nunca-existiu", "m"); got != nil {
		t.Errorf("= %v, esperado nil", got)
	}
}

// TestRegistryConcorrente: as cinco operações simultâneas sobre chaves
// sobrepostas, com Delete cruzando os dois mapas.
func TestRegistryConcorrente(t *testing.T) {
	r := New()
	const workers, iters = 8, 50
	users := []string{"u0", "u1", "u2"}
	uid := func(w, i int) string { return users[(w+i)%len(users)] }

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(5)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				r.Set(uid(w, i), &fakeUserClient{userID: uid(w, i)})
			}
		}(w)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				_ = r.Get(uid(w, i))
			}
		}(w)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				r.Delete(uid(w, i))
			}
		}(w)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				r.SetPollOptions(uid(w, i), fmt.Sprintf("m%d", i%4), []string{"a", "b"})
			}
		}(w)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				_ = r.GetPollOptions(uid(w, i), fmt.Sprintf("m%d", i%4))
			}
		}(w)
	}
	wg.Wait()

	r.SetPollOptions("depois", "m", []string{"x"})
	if got := r.GetPollOptions("depois", "m"); len(got) != 1 {
		t.Errorf("apos concorrencia = %v, esperado [x]", got)
	}
}
