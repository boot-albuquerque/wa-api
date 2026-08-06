package port

import "testing"

func TestSessionEventKinds(t *testing.T) {
	kinds := []SessionEventKind{
		SessionEventKindConnected,
		SessionEventKindDisconnected,
		SessionEventKindLoggedOut,
		SessionEventKindPairSuccess,
		SessionEventKindQR,
		SessionEventKindStreamReplaced,
	}
	if len(kinds) != 6 {
		t.Fatalf("expected exactly 6 SessionEvent kinds, got %d", len(kinds))
	}

	evt := SessionEvent{
		Kind: SessionEventKindPairSuccess,
		PairSuccess: &SessionPairSuccessEvent{
			JID:          "5511999999999.0:1@s.whatsapp.net",
			BusinessName: "Acme",
			Platform:     "android",
		},
	}
	if evt.PairSuccess == nil || evt.PairSuccess.JID == "" {
		t.Fatal("expected PairSuccess payload to be populated")
	}
	if evt.Disconnected != nil || evt.LoggedOut != nil || evt.QR != nil {
		t.Fatal("expected only the matching variant to be populated")
	}
}

func TestPairingEventKinds(t *testing.T) {
	kinds := []PairingEventKind{
		PairingEventKindQR,
		PairingEventKindTimeout,
		PairingEventKindSuccess,
	}
	if len(kinds) != 3 {
		t.Fatalf("expected exactly 3 PairingEvent kinds, got %d", len(kinds))
	}

	success := PairingEvent{Kind: PairingEventKindSuccess, JID: "5511999999999.0:1@s.whatsapp.net"}
	if success.JID == "" {
		t.Fatal("expected JID on success pairing event")
	}
}
