package waclient_test

import (
	"testing"

	"wa-api/pkg/infra/wa-noise/waclient"
	"wa-api/pkg/infra/wa-noise/waclient/waclienttest"
)

// TestFake_SatisfiesInterface é o guarda-compilação: o fake tem a forma exata
// de waclient.Client e waclient.Getter; se um dia a interface ganhar um método
// novo, este teste quebra antes de qualquer outro.
func TestFake_SatisfiesInterface(t *testing.T) {
	var _ waclient.Client = (*waclienttest.Fake)(nil)
	var _ waclient.Getter = waclienttest.GetterWith(nil)
}
