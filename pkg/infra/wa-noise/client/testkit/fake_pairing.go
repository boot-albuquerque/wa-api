package testkit

import (
	"context"
	"regexp"
	"strings"

	wapairing "wa-api/internal/wa-noise/capabilities/pairing"
)

// As tres constantes abaixo sao a regra REAL de validacao de telefone do
// fork, copiada de internal/wa-noise/capabilities/pairing/paircode.go:57-62
// com os valores de constants.go:67-69:
//
//	phone = notNumbers.ReplaceAllString(phone, "")
//	if len(phone) < codePhoneMinLength            -> ErrPhoneNumberTooShort
//	else if strings.HasPrefix(phone, "0")         -> ErrPhoneNumberIsNotInternational
//
// Ela vive aqui porque ARMADILHA 1 deste repo diz que um dublê MAIS
// PERMISSIVO que a producao esconde o defeito: se o Fake aceitasse "0119"
// e a producao recusasse, nenhum teste mediria a regra. Os valores sao
// nao-exportados no subpacote, entao a copia e' inevitavel — o comentario
// acima e' o que a torna auditavel.
const (
	pairPhoneMinLength   = 7
	pairPhoneTrunkPrefix = "0"
)

var pairPhoneNotNumbers = regexp.MustCompile("[^0-9]")

// PairPhone imita Client.PairPhone (internal/wa-noise/core/pair-code.go:50).
//
// A validacao roda SEMPRE, antes de PairPhoneFn: ela e' da producao, nao do
// caso de teste, e um teste nao pode desliga-la sem deixar o dublê mais
// permissivo que o original.
func (f *Fake) PairPhone(ctx context.Context, phone string, showPushNotification bool, clientType wapairing.ClientType, clientDisplayName string) (string, error) {
	digits := pairPhoneNotNumbers.ReplaceAllString(phone, "")
	if len(digits) < pairPhoneMinLength {
		return "", wapairing.ErrPhoneNumberTooShort
	} else if strings.HasPrefix(digits, pairPhoneTrunkPrefix) {
		return "", wapairing.ErrPhoneNumberIsNotInternational
	}
	if f.PairPhoneFn != nil {
		return f.PairPhoneFn(ctx, phone, showPushNotification, clientType, clientDisplayName)
	}
	return "", nil
}
