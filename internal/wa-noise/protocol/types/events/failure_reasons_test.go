package events

import (
	"strconv"
	"strings"
	"testing"
)

// IsLoggedOut e' a decisao mais consequente do pacote: um true APAGA os dados
// de sessao do device. Marcar de menos deixa o cliente em loop de reconexao
// com credencial morta; marcar de mais destroi uma sessao boa e obriga o
// usuario a reler o QR.
func TestIsLoggedOutMarksOnlyTheThreeTerminalReasons(t *testing.T) {
	loggedOut := map[ConnectFailureReason]bool{
		ConnectFailureLoggedOut:      true,
		ConnectFailureMainDeviceGone: true,
		ConnectFailureUnknownLogout:  true,
	}
	all := []ConnectFailureReason{
		ConnectFailureGeneric, ConnectFailureLoggedOut, ConnectFailureTempBanned,
		ConnectFailureMainDeviceGone, ConnectFailureUnknownLogout,
		ConnectFailureClientOutdated, ConnectFailureBadUserAgent,
		ConnectFailureCATExpired, ConnectFailureCATInvalid, ConnectFailureNotFound,
		ConnectFailureClientUnknown, ConnectFailureInternalServerError,
		ConnectFailureExperimental, ConnectFailureServiceUnavailable,
	}
	for _, reason := range all {
		if got := reason.IsLoggedOut(); got != loggedOut[reason] {
			t.Errorf("%d: IsLoggedOut = %v, esperado %v", int(reason), got, loggedOut[reason])
		}
	}
	// Um codigo desconhecido nao pode apagar a sessao.
	if ConnectFailureReason(499).IsLoggedOut() {
		t.Error("codigo desconhecido marcou logout")
	}
}

// Um ban temporario NAO e' logout: a sessao continua valida e volta sozinha
// quando o ban expira. Apagar os dados aqui seria o pior falso positivo.
func TestTempBanIsNotLoggedOut(t *testing.T) {
	if ConnectFailureTempBanned.IsLoggedOut() {
		t.Error("ban temporario foi tratado como logout")
	}
}

func TestNumberStringIsTheDecimalCode(t *testing.T) {
	for _, reason := range []ConnectFailureReason{ConnectFailureLoggedOut, ConnectFailureTempBanned, 499} {
		if got := reason.NumberString(); got != strconv.Itoa(int(reason)) {
			t.Errorf("%d: NumberString = %q", int(reason), got)
		}
	}
}

func TestConnectFailureStringCarriesCodeAndDescription(t *testing.T) {
	got := ConnectFailureLoggedOut.String()
	if !strings.HasPrefix(got, "401: ") {
		t.Errorf("= %q, esperado comecar com o codigo", got)
	}
	if !strings.Contains(got, "logged out from another device") {
		t.Errorf("= %q", got)
	}
}

// Um codigo sem descricao ainda tem que sair com o NUMERO. E' o unico dado
// util quando o servidor manda algo que a lib nao conhece.
func TestConnectFailureStringOfUnknownCodeKeepsTheNumber(t *testing.T) {
	got := ConnectFailureReason(499).String()
	if !strings.HasPrefix(got, "499: ") {
		t.Errorf("= %q", got)
	}
	if !strings.Contains(got, "unknown error") {
		t.Errorf("= %q", got)
	}
}

func TestTempBanReasonStringCarriesCodeAndDescription(t *testing.T) {
	got := TempBanSentToTooManyPeople.String()
	if !strings.HasPrefix(got, "101: ") {
		t.Errorf("= %q", got)
	}
	if !strings.Contains(got, "address books") {
		t.Errorf("= %q", got)
	}
}

func TestTempBanReasonStringOfUnknownCodeIsStillUseful(t *testing.T) {
	got := TempBanReason(199).String()
	if !strings.HasPrefix(got, "199: ") {
		t.Errorf("= %q", got)
	}
	if !strings.Contains(got, "terms of service") {
		t.Errorf("= %q", got)
	}
}

// Cada codigo de ban tem que ter descricao propria: sao os textos que chegam
// ao usuario final explicando por que ele foi banido.
func TestEveryTempBanReasonHasItsOwnDescription(t *testing.T) {
	seen := map[string]TempBanReason{}
	for _, reason := range []TempBanReason{
		TempBanSentToTooManyPeople, TempBanBlockedByUsers, TempBanCreatedTooManyGroups,
		TempBanSentTooManySameMessage, TempBanBroadcastList,
	} {
		msg, ok := tempBanReasonMessage[reason]
		if !ok {
			t.Errorf("%d nao tem descricao", int(reason))
			continue
		}
		if other, dup := seen[msg]; dup {
			t.Errorf("%d e %d tem a mesma descricao", int(reason), int(other))
		}
		seen[msg] = reason
	}
}

// Os codigos vem do protocolo e sao lidos direto do frame; dois iguais
// significariam dois motivos indistinguiveis.
func TestFailureCodesAreDistinct(t *testing.T) {
	t.Run("ConnectFailure", func(t *testing.T) {
		seen := map[ConnectFailureReason]bool{}
		for _, reason := range []ConnectFailureReason{
			ConnectFailureGeneric, ConnectFailureLoggedOut, ConnectFailureTempBanned,
			ConnectFailureMainDeviceGone, ConnectFailureUnknownLogout,
			ConnectFailureClientOutdated, ConnectFailureBadUserAgent,
			ConnectFailureCATExpired, ConnectFailureCATInvalid, ConnectFailureNotFound,
			ConnectFailureClientUnknown, ConnectFailureInternalServerError,
			ConnectFailureExperimental, ConnectFailureServiceUnavailable,
		} {
			if seen[reason] {
				t.Errorf("codigo %d duplicado", int(reason))
			}
			seen[reason] = true
		}
	})
	t.Run("TempBan", func(t *testing.T) {
		seen := map[TempBanReason]bool{}
		for _, reason := range []TempBanReason{
			TempBanSentToTooManyPeople, TempBanBlockedByUsers, TempBanCreatedTooManyGroups,
			TempBanSentTooManySameMessage, TempBanBroadcastList,
		} {
			if seen[reason] {
				t.Errorf("codigo %d duplicado", int(reason))
			}
			seen[reason] = true
		}
	})
}
