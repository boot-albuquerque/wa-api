package user

import (
	"testing"
	"time"

	"wa-api/internal/wa-noise/protocol/types"
)

// A invalidacao primaria do cache de dispositivos e' push do servidor. O TTL e'
// rede para o que o push nao cobre: notificacao perdida e chave orfa (F37).
func TestDeviceCacheEntradaExpiraNaLeitura(t *testing.T) {
	var c DeviceCache
	jid := types.NewJID("5511999999999", types.DefaultUserServer)

	c.Lock()
	c.SetLocked(jid, DeviceEntry{Devices: []types.JID{jid}, DHash: "h"})
	if _, ok := c.GetLocked(jid); !ok {
		t.Fatal("entrada recem-gravada deveria ser lida")
	}
	// Envelhece a entrada por dentro: o teste nao pode esperar 24h.
	e := c.entries[jid]
	e.seen = time.Now().Add(-deviceCacheTTL - time.Minute)
	c.entries[jid] = e

	if _, ok := c.GetLocked(jid); ok {
		t.Error("entrada expirada deveria ser tratada como ausente")
	}
	c.Unlock()
}

// Chave orfa e' o caso que a leitura sozinha nunca resolveria: ninguem consulta
// aquele JID, entao ninguem dispara a expiracao por leitura. Quem limpa e' a
// varredura.
func TestDeviceCacheVarreduraRemoveChaveOrfa(t *testing.T) {
	var c DeviceCache
	orfa := types.NewJID("5500000000000", types.DefaultUserServer)
	viva := types.NewJID("5511999999999", types.DefaultUserServer)

	c.Lock()
	defer c.Unlock()
	c.SetLocked(orfa, DeviceEntry{DHash: "orfa"})

	// Envelhece a orfa e força a varredura a poder rodar de novo.
	e := c.entries[orfa]
	e.seen = time.Now().Add(-deviceCacheTTL - time.Minute)
	c.entries[orfa] = e
	c.lastSweep = time.Now().Add(-deviceCacheSweepInterval - time.Minute)

	c.SetLocked(viva, DeviceEntry{DHash: "viva"})

	if _, ok := c.entries[orfa]; ok {
		t.Error("a chave orfa deveria ter saido na varredura")
	}
	if _, ok := c.entries[viva]; !ok {
		t.Error("a entrada recem-gravada nao pode ser varrida")
	}
}

// A varredura e' O(n) sob o lock do cache; o intervalo existe para ela nao
// rodar a cada gravacao.
func TestDeviceCacheVarreNoMaximoUmaVezPorIntervalo(t *testing.T) {
	var c DeviceCache
	c.Lock()
	defer c.Unlock()

	c.SetLocked(types.NewJID("1", types.DefaultUserServer), DeviceEntry{})
	primeira := c.lastSweep
	c.SetLocked(types.NewJID("2", types.DefaultUserServer), DeviceEntry{})
	if !c.lastSweep.Equal(primeira) {
		t.Error("varreu de novo antes de o intervalo passar")
	}
}
