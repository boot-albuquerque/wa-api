package bootstrap

import (
	"testing"

	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// F215: a device never holds a Signal session with itself, so self-echoes
// always fail to decrypt. The filter must silence those (IsFromMe == true)
// without suppressing third-party failures — which are legitimate signal.

// TestUndecryptable_ThirdPartyReachesWebhook verifies that an
// UndecryptableMessage from a THIRD PARTY triggers the webhook.
//
// Based on the real counter-example from field surveillance 2026-08-22:
// sender key failure from 96465066184854@lid in status@broadcast.
func TestUndecryptable_ThirdPartyReachesWebhook(t *testing.T) {
	evh := &UserEventHandler{}
	st := &eventState{postmap: make(map[string]interface{})}

	evt := &events.UndecryptableMessage{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     types.NewJID("status", types.BroadcastServer),
				Sender:   types.NewJID("96465066184854", types.HiddenUserServer),
				IsFromMe: false,
				IsGroup:  true,
			},
		},
		IsUnavailable: false,
	}

	evh.handleUndecryptableMessage(evt, st)

	if st.dowebhook != 1 {
		t.Fatalf("dowebhook = %d, want 1 — third-party UndecryptableMessage "+
			"must reach the webhook (F215)", st.dowebhook)
	}
}

// TestUndecryptable_SelfEchoSuppressed verifies that an UndecryptableMessage
// with IsFromMe == true does NOT trigger the webhook.
func TestUndecryptable_SelfEchoSuppressed(t *testing.T) {
	evh := &UserEventHandler{}
	st := &eventState{postmap: make(map[string]interface{})}

	evt := &events.UndecryptableMessage{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     types.NewJID("29343770251463", types.HiddenUserServer),
				Sender:   types.NewJID("29343770251463", types.HiddenUserServer),
				IsFromMe: true,
			},
		},
		IsUnavailable: false,
	}

	evh.handleUndecryptableMessage(evt, st)

	if st.dowebhook != 0 {
		t.Fatalf("dowebhook = %d, want 0 — self-echo UndecryptableMessage "+
			"must NOT reach the webhook (F215)", st.dowebhook)
	}
}
