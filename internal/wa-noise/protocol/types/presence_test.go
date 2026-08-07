package types

import "testing"

// GoString existe para o log: imprime o NOME da constante em vez do valor de
// fio. Os quatro tipos com nome proprio sao os que aparecem no caminho quente
// de recibo; o resto cai no fallback com o valor cru.
func TestReceiptTypeGoStringNamesTheKnownConstants(t *testing.T) {
	tests := map[ReceiptType]string{
		ReceiptTypeRead:      "types.ReceiptTypeRead",
		ReceiptTypeReadSelf:  "types.ReceiptTypeReadSelf",
		ReceiptTypeDelivered: "types.ReceiptTypeDelivered",
		ReceiptTypePlayed:    "types.ReceiptTypePlayed",
	}
	for rt, want := range tests {
		if got := rt.GoString(); got != want {
			t.Errorf("%q.GoString() = %q, esperado %q", string(rt), got, want)
		}
	}
}

func TestReceiptTypeGoStringFallsBackToTheRawValue(t *testing.T) {
	for _, rt := range []ReceiptType{ReceiptTypeSender, ReceiptTypeRetry, ReceiptTypePlayedSelf, ReceiptTypeServerError, "desconhecido"} {
		got := rt.GoString()
		if got == "" {
			t.Errorf("%q devolveu string vazia", string(rt))
		}
		if want := "types.ReceiptType(\"" + string(rt) + "\")"; got != want {
			t.Errorf("%q.GoString() = %q, esperado %q", string(rt), got, want)
		}
	}
}

// ReceiptTypeDelivered e' a string VAZIA: e' o recibo padrao, que chega sem
// atributo `type`. Confundi-lo com "tipo ausente/desconhecido" faria toda
// entrega ser reclassificada.
func TestDeliveredIsTheEmptyReceiptType(t *testing.T) {
	if ReceiptTypeDelivered != "" {
		t.Errorf("ReceiptTypeDelivered = %q, esperado vazio", string(ReceiptTypeDelivered))
	}
	if ChatPresenceMediaText != "" {
		t.Errorf("ChatPresenceMediaText = %q, esperado vazio", string(ChatPresenceMediaText))
	}
}

// Todos os tipos de recibo tem que ser distintos: sao lidos do atributo `type`
// do frame e dois iguais fundiriam dois eventos diferentes.
func TestReceiptTypesAreDistinct(t *testing.T) {
	seen := map[ReceiptType]string{}
	for name, rt := range map[string]ReceiptType{
		"Delivered":   ReceiptTypeDelivered,
		"Sender":      ReceiptTypeSender,
		"Retry":       ReceiptTypeRetry,
		"Read":        ReceiptTypeRead,
		"ReadSelf":    ReceiptTypeReadSelf,
		"Played":      ReceiptTypePlayed,
		"PlayedSelf":  ReceiptTypePlayedSelf,
		"ServerError": ReceiptTypeServerError,
		"Inactive":    ReceiptTypeInactive,
		"PeerMsg":     ReceiptTypePeerMsg,
		"HistorySync": ReceiptTypeHistorySync,
	} {
		if other, dup := seen[rt]; dup {
			t.Errorf("%s e %s tem o mesmo valor %q", name, other, string(rt))
		}
		seen[rt] = name
	}
}

func TestPresenceAndChatPresenceValues(t *testing.T) {
	if PresenceAvailable == PresenceUnavailable {
		t.Error("os dois estados de presenca colidiram")
	}
	if ChatPresenceComposing == ChatPresencePaused {
		t.Error("os dois estados de chat colidiram")
	}
	if ChatPresenceMediaText == ChatPresenceMediaAudio {
		t.Error("os dois tipos de midia colidiram")
	}
}
