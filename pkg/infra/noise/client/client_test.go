package client_test

import (
	"testing"

	"wa-api/pkg/infra/noise/client"
	"wa-api/pkg/infra/noise/client/testkit"
)

// TestFake_SatisfiesInterface é o guarda-compilação: o fake tem a forma exata
// de client.Client e client.Getter; se um dia a interface ganhar um método
// novo, este teste quebra antes de qualquer outro.
func TestFake_SatisfiesInterface(t *testing.T) {
	var _ client.Client = (*testkit.Fake)(nil)
	var _ client.Getter = testkit.GetterWith(nil)
}
