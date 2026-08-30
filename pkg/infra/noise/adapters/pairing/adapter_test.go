package pairing_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	wapairing "wa-api/internal/noise/capabilities/pairing"
	"wa-api/pkg/domain/apperr"
	pairingadapter "wa-api/pkg/infra/noise/adapters/pairing"
	"wa-api/pkg/infra/noise/client"
	"wa-api/pkg/infra/noise/client/testkit"
)

const txtID = "user-1"

// internationalPhone e' o numero da sonda de campo da F152, com os digitos
// finais trocados. Passa na validacao REAL do fork.
const internationalPhone = "5541992400000"

// wireCode e' o formato de codigo que
// internal/noise/capabilities/pairing/paircode.go:100 monta: 8 caracteres
// base32 em dois grupos de 4.
const wireCode = "WXYZ-2468"

func adapterFor(f *testkit.Fake) *pairingadapter.PhonePairerAdapter {
	return pairingadapter.NewPhonePairerAdapter(testkit.GetterWith(map[string]client.Client{txtID: f}))
}

// TestRequestPairingCode_ReturnsCode: o caminho de SUCESSO — o codigo do
// cliente atravessa o adapter sem ser descartado. E' o defeito da F152 medido
// uma camada abaixo do handler.
func TestRequestPairingCode_ReturnsCode(t *testing.T) {
	var gotDisplay, gotPhone string
	var gotPush bool
	var gotType wapairing.ClientType
	f := &testkit.Fake{
		PairPhoneFn: func(_ context.Context, phone string, push bool, ct wapairing.ClientType, display string) (string, error) {
			gotPhone, gotPush, gotType, gotDisplay = phone, push, ct, display
			return wireCode, nil
		},
	}

	code, err := adapterFor(f).RequestPairingCode(context.Background(), txtID, internationalPhone)

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if code == "" {
		t.Fatal("codigo VAZIO — este e' o defeito da F152")
	}
	if code != wireCode {
		t.Errorf("codigo: got %q, want %q", code, wireCode)
	}
	if gotPhone != internationalPhone {
		t.Errorf("phone: got %q, want %q", gotPhone, internationalPhone)
	}
	// Os tres parametros fixos sao contrato com o servidor do WhatsApp, que
	// responde 400 para display name fora do conjunto comum
	// (41bc8e2^:handlers.go:733).
	if !gotPush {
		t.Error("showPushNotification: got false, want true (contrato historico)")
	}
	if gotType != wapairing.ClientChrome {
		t.Errorf("clientType: got %v, want ClientChrome", gotType)
	}
	if gotDisplay != "Chrome (Linux)" {
		t.Errorf("clientDisplayName: got %q, want %q", gotDisplay, "Chrome (Linux)")
	}
}

// TestRequestPairingCode_RealPhoneRule: o dublê aplica a regra REAL de
// validacao de telefone (ARMADILHA 1 — dublê mais permissivo que a producao
// esconde o defeito). Os valores vem de
// internal/noise/capabilities/pairing/paircode_test.go:89-91, o teste do
// proprio fork.
func TestRequestPairingCode_RealPhoneRule(t *testing.T) {
	cases := map[string]struct {
		phone string
		want  error
	}{
		"curto demais":  {"12345", wapairing.ErrPhoneNumberTooShort},
		"so' simbolos":  {"+-() ", wapairing.ErrPhoneNumberTooShort},
		"nacional (0…)": {"0119999999", wapairing.ErrPhoneNumberIsNotInternational},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			called := false
			f := &testkit.Fake{
				PairPhoneFn: func(context.Context, string, bool, wapairing.ClientType, string) (string, error) {
					called = true
					return wireCode, nil
				},
			}

			code, err := adapterFor(f).RequestPairingCode(context.Background(), txtID, tc.phone)

			if err == nil {
				t.Fatalf("telefone %q foi aceito; a producao o recusa", tc.phone)
			}
			if code != "" {
				t.Errorf("codigo devolvido junto do erro: %q", code)
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("causa: got %v, want %v", err, tc.want)
			}
			if called {
				t.Error("PairPhoneFn rodou apesar de o numero ser invalido — o dublê ficou mais permissivo que a producao")
			}
			// O erro chega tipado a' fronteira, com a mensagem do fork, e
			// mapeia para 400 (contrato historico 41bc8e2^:handlers.go:737).
			var appErr *apperr.AppError
			if !errors.As(err, &appErr) {
				t.Fatalf("erro nao e' *apperr.AppError: %T", err)
			}
			if appErr.Category.HTTPStatus() != 400 {
				t.Errorf("status derivado: got %d, want 400", appErr.Category.HTTPStatus())
			}
			if !strings.Contains(appErr.Message, tc.want.Error()) {
				t.Errorf("mensagem: got %q, quero conter %q", appErr.Message, tc.want.Error())
			}
		})
	}
}

// TestIsPaired_ReflectsLoggedIn: a guarda le' IsLoggedIn do cliente, que e' o
// que 41bc8e2^:handlers.go:729 lia.
func TestIsPaired_ReflectsLoggedIn(t *testing.T) {
	for _, want := range []bool{true, false} {
		f := &testkit.Fake{IsLoggedInFn: func() bool { return want }}

		got, err := adapterFor(f).IsPaired(context.Background(), txtID)

		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if got != want {
			t.Errorf("IsPaired = %v, want %v", got, want)
		}
	}
}

// TestPhonePairer_SemSessao: sem cliente, as duas operacoes recusam com o erro
// tipado de sessao ausente — e RequestPairingCode nao inventa codigo.
func TestPhonePairer_SemSessao(t *testing.T) {
	a := pairingadapter.NewPhonePairerAdapter(func(string) client.Client { return nil })

	if _, err := a.IsPaired(context.Background(), txtID); err == nil {
		t.Error("IsPaired aceitou sessao ausente")
	}
	code, err := a.RequestPairingCode(context.Background(), txtID, internationalPhone)
	if err == nil {
		t.Error("RequestPairingCode aceitou sessao ausente")
	}
	if code != "" {
		t.Errorf("codigo devolvido sem sessao: %q", code)
	}
}
