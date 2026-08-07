package events

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// PermanentDisconnect e' a interface que o cliente usa para decidir se PARA de
// reconectar. Um evento que deveria implementa-la e nao implementa vira loop
// infinito de reconexao; um que implementa por engano deixa o cliente offline
// depois de uma falha transitoria.
func TestPermanentDisconnectIsImplementedByExactlyTheTerminalEvents(t *testing.T) {
	permanent := []any{
		&LoggedOut{}, &StreamReplaced{}, &ClientOutdated{},
		&CATRefreshError{}, &TemporaryBan{}, &ConnectFailure{},
	}
	for _, evt := range permanent {
		if _, ok := evt.(PermanentDisconnect); !ok {
			t.Errorf("%T deveria implementar PermanentDisconnect", evt)
		}
	}

	// Estes nao sao desconexoes permanentes: o cliente tem que reconectar.
	transient := []any{
		&Disconnected{}, &KeepAliveTimeout{}, &KeepAliveRestored{},
		&Connected{}, &StreamError{}, &QRScannedWithoutMultidevice{},
		&ManualLoginReconnect{},
	}
	for _, evt := range transient {
		if _, ok := evt.(PermanentDisconnect); ok {
			t.Errorf("%T nao deveria implementar PermanentDisconnect", evt)
		}
	}
}

// Cada descricao tem que ser propria e nao vazia: e' o texto que chega ao log
// e ao operador explicando por que a sessao parou.
func TestEveryPermanentDisconnectHasItsOwnDescription(t *testing.T) {
	events := []PermanentDisconnect{
		&LoggedOut{Reason: ConnectFailureLoggedOut},
		&StreamReplaced{},
		&ClientOutdated{},
		&CATRefreshError{Error: errors.New("falhou")},
		&TemporaryBan{Code: TempBanBlockedByUsers},
		&ConnectFailure{Reason: ConnectFailureBadUserAgent},
	}
	seen := map[string]PermanentDisconnect{}
	for _, evt := range events {
		desc := evt.PermanentDisconnectDescription()
		if desc == "" {
			t.Errorf("%T devolveu descricao vazia", evt)
			continue
		}
		if other, dup := seen[desc]; dup {
			t.Errorf("%T e %T tem a mesma descricao %q", evt, other, desc)
		}
		seen[desc] = evt
	}
}

// A descricao de LoggedOut delega para o motivo, entao o codigo do servidor
// atravessa ate' o log em vez de virar um texto generico.
func TestLoggedOutDescriptionCarriesTheReason(t *testing.T) {
	evt := &LoggedOut{Reason: ConnectFailureMainDeviceGone}
	got := evt.PermanentDisconnectDescription()
	if !strings.Contains(got, "403") {
		t.Errorf("= %q, esperado conter o codigo", got)
	}
	if got != ConnectFailureMainDeviceGone.String() {
		t.Errorf("= %q, esperado delegar para o motivo", got)
	}
}

func TestConnectFailureDescriptionCarriesTheReason(t *testing.T) {
	evt := &ConnectFailure{Reason: ConnectFailureServiceUnavailable}
	got := evt.PermanentDisconnectDescription()
	if !strings.Contains(got, "503") || !strings.Contains(got, "connect failure") {
		t.Errorf("= %q", got)
	}
}

// TemporaryBan tem duas formas: com e sem prazo. Sem o prazo, o operador nao
// sabe se espera minutos ou semanas, entao o ramo com Expire preenchido tem
// que de fato mostra-lo.
func TestTemporaryBanStringShowsTheExpiryWhenKnown(t *testing.T) {
	withoutExpiry := (&TemporaryBan{Code: TempBanBroadcastList}).String()
	if strings.Contains(withoutExpiry, "expires in") {
		t.Errorf("sem prazo = %q, nao deveria falar de expiracao", withoutExpiry)
	}
	if !strings.Contains(withoutExpiry, "106") {
		t.Errorf("sem prazo = %q, esperado conter o codigo", withoutExpiry)
	}

	withExpiry := (&TemporaryBan{Code: TempBanBroadcastList, Expire: 24 * time.Hour}).String()
	if !strings.Contains(withExpiry, "expires in") || !strings.Contains(withExpiry, "24h") {
		t.Errorf("com prazo = %q", withExpiry)
	}
	if !strings.Contains(withExpiry, "106") {
		t.Errorf("com prazo = %q, esperado conter o codigo", withExpiry)
	}
}

func TestTemporaryBanDescriptionWrapsItsString(t *testing.T) {
	ban := &TemporaryBan{Code: TempBanCreatedTooManyGroups, Expire: time.Hour}
	got := ban.PermanentDisconnectDescription()
	if !strings.Contains(got, "temporarily banned") {
		t.Errorf("= %q", got)
	}
	if !strings.Contains(got, ban.String()) {
		t.Errorf("= %q, esperado embutir %q", got, ban.String())
	}
}

// LoggedOut distingue a origem pelo campo OnConnect: mensagem de falha de
// conexao ou stream:error. So' o primeiro traz codigo de motivo.
func TestLoggedOutDistinguishesItsOrigin(t *testing.T) {
	onConnect := LoggedOut{OnConnect: true, Reason: ConnectFailureLoggedOut}
	if !onConnect.OnConnect || onConnect.Reason != ConnectFailureLoggedOut {
		t.Errorf("= %+v", onConnect)
	}
	fromStream := LoggedOut{}
	if fromStream.OnConnect {
		t.Error("o valor zero deveria indicar origem em stream:error")
	}
}
