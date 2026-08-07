package webhook

import (
	"sync"
	"testing"

	"github.com/go-resty/resty/v2"
)

// TestClientTimeout_Positivo: um timeout zero em resty expira na hora —
// seria quebra silenciosa de toda entrega de webhook.
func TestClientTimeout_Positivo(t *testing.T) {
	if clientTimeout <= 0 {
		t.Errorf("clientTimeout = %v, want > 0", clientTimeout)
	}
	if maxRedirects <= 0 {
		t.Errorf("maxRedirects = %d, want > 0", maxRedirects)
	}
}

// TestEnvTLSSkipVerify_Nome trava o nome da variável de ambiente: ele é
// contrato com o operador e com pkg/bootstrap/lifecycle.go, que lê a MESMA
// variável a partir da sua própria cópia da lógica. Divergir os dois nomes
// é uma falha silenciosa — o operador desliga a verificação num caminho e
// não no outro.
func TestEnvTLSSkipVerify_Nome(t *testing.T) {
	if EnvTLSSkipVerify != "WA_API_WEBHOOK_TLS_SKIP_VERIFY" {
		t.Errorf("EnvTLSSkipVerify = %q", EnvTLSSkipVerify)
	}
}

func TestProvisionRegistraOCliente(t *testing.T) {
	r := New()
	if err := r.Provision("u", ""); err != nil {
		t.Fatalf("Provision: %v", err)
	}
	c := r.Get("u")
	if c == nil {
		t.Fatal("Provision nao registrou o cliente")
	}
	if c.GetClient().Timeout != clientTimeout {
		t.Errorf("timeout do cliente = %v, esperado %v", c.GetClient().Timeout, clientTimeout)
	}

	r.Delete("u")
	if r.Get("u") != nil {
		t.Error("Delete nao removeu o cliente")
	}
}

func TestGetDeUsuarioDesconhecidoDevolveNil(t *testing.T) {
	if got := New().Get("nunca-existiu"); got != nil {
		t.Errorf("Get de usuario desconhecido = %v, esperado nil", got)
	}
}

// TestRegistryConcorrente: Provision, Set, Get e Delete simultâneos sobre
// chaves sobrepostas.
func TestRegistryConcorrente(t *testing.T) {
	r := New()
	const workers, iters = 8, 50
	users := []string{"u0", "u1", "u2"}
	uid := func(w, i int) string { return users[(w+i)%len(users)] }

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(4)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				_ = r.Provision(uid(w, i), "")
			}
		}(w)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				r.Set(uid(w, i), resty.New())
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
	}
	wg.Wait()

	r.Set("depois", resty.New())
	if r.Get("depois") == nil {
		t.Error("o registry nao aceitou escrita depois do acesso concorrente")
	}
}
