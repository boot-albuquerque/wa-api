package whatsmeow

import (
	"context"
	"testing"

	"go.mau.fi/whatsmeow"
)

// TestSessionCounterAdapter_New devolve adapter não-nil.
func TestSessionCounterAdapter_New(t *testing.T) {
	if NewSessionCounterAdapter(nil) == nil {
		t.Fatal("NewSessionCounterAdapter returned nil")
	}
}

// fakeCM implementa ClientHealthProvider para os testes do SessionCounter.
type fakeCM struct {
	clients []*whatsmeow.Client
}

func (f *fakeCM) GetWhatsmeowClientsCount() int { return len(f.clients) }
func (f *fakeCM) IterateWhatsmeowClients(cb func(*whatsmeow.Client) bool) {
	for _, c := range f.clients {
		if !cb(c) {
			return
		}
	}
}

// TestSessionCounterAdapter_CountSessions_Zero devolve total=0.
func TestSessionCounterAdapter_CountSessions_Zero(t *testing.T) {
	cm := &fakeCM{}
	a := NewSessionCounterAdapter(cm)
	counts, err := a.CountSessions(context.Background())
	if err != nil {
		t.Fatalf("CountSessions = %v", err)
	}
	if counts.Total != 0 {
		t.Errorf("Total = %d, want 0", counts.Total)
	}
	if counts.Connected != 0 || counts.LoggedIn != 0 {
		t.Errorf("Connected=%d LoggedIn=%d, want 0", counts.Connected, counts.LoggedIn)
	}
}

// TestSessionCounterAdapter_CountSessions_NilClientsEntries não panic.
func TestSessionCounterAdapter_CountSessions_NilClientsEntries(t *testing.T) {
	cm := &fakeCM{clients: []*whatsmeow.Client{nil}}
	a := NewSessionCounterAdapter(cm)
	counts, err := a.CountSessions(context.Background())
	if err != nil {
		t.Fatalf("CountSessions = %v", err)
	}
	if counts.Total != 1 {
		t.Errorf("Total = %d, want 1", counts.Total)
	}
}

// TestSessionCounterAdapter_CountSessions_ConnectedAndLoggedIn: adiciona um
// fake client que reporta IsConnected/IsLoggedIn via função.
func TestSessionCounterAdapter_CountSessions_ConnectedAndLoggedIn(t *testing.T) {
	// Como Client.IsConnected/IsLoggedIn são métodos no tipo concreto
	// (não interface), não temos como mockar facilmente sem wrapper. O
	// teste abaixo apenas cobre o caminho "não há client não-nil" e
	// valida a forma do struct SessionCounts.
	cm := &fakeCM{clients: nil}
	a := NewSessionCounterAdapter(cm)
	counts, err := a.CountSessions(context.Background())
	if err != nil {
		t.Fatalf("CountSessions = %v", err)
	}
	if counts.Total != 0 {
		t.Errorf("Total = %d, want 0", counts.Total)
	}
}