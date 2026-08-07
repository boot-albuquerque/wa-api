package waclient_test

import (
	"testing"
	waclient "wa-api/pkg/infra/wa-noise/client"

	wanoise "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/persistence/store"
)

// TestRealWAClient_Store devolve Client.Store.
func TestRealWAClient_Store(t *testing.T) {
	wac := &wanoise.Client{Store: &store.Device{}}
	rc := waclient.RealClient{Client: wac}
	if got := rc.Store(); got != wac.Store {
		t.Errorf("waclient.RealClient.Store = %v, want %v", got, wac.Store)
	}
}
