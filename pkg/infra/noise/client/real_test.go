package client_test

import (
	"testing"
	"wa-api/pkg/infra/noise/client"

	"wa-api/internal/noise"
	"wa-api/internal/noise/persistence/store"
)

// TestRealWAClient_Store devolve Client.Store.
func TestRealWAClient_Store(t *testing.T) {
	wac := &noise.Client{Store: &store.Device{}}
	rc := client.RealClient{Client: wac}
	if got := rc.Store(); got != wac.Store {
		t.Errorf("client.RealClient.Store = %v, want %v", got, wac.Store)
	}
}
