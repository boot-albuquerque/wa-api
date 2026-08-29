package sqlstore

import (
	"context"
	"testing"

	"wa-api/internal/noise/persistence/store"
	"wa-api/internal/noise/protocol/types"
)

func pnJID(user string) types.JID {
	return types.JID{User: user, Server: types.DefaultUserServer}
}

func lidJID(user string) types.JID {
	return types.JID{User: user, Server: types.HiddenUserServer}
}

func newTestLIDMap(t *testing.T) *CachedLIDMap {
	t.Helper()
	return newTestContainer(t).LIDMap
}

func TestGetLIDForPNRejectsNonPNJID(t *testing.T) {
	_, err := newTestLIDMap(t).GetLIDForPN(context.Background(), lidJID("111"))
	if err == nil {
		t.Fatal("GetLIDForPN com JID que nao e' PN deveria falhar")
	}
}

func TestGetPNForLIDRejectsNonLIDJID(t *testing.T) {
	_, err := newTestLIDMap(t).GetPNForLID(context.Background(), pnJID("5511999999999"))
	if err == nil {
		t.Fatal("GetPNForLID com JID que nao e' LID deveria falhar")
	}
}

func TestPutLIDMappingRejectsWrongServers(t *testing.T) {
	ctx := context.Background()
	m := newTestLIDMap(t)
	if err := m.PutLIDMapping(ctx, pnJID("1"), pnJID("2")); err == nil {
		t.Fatal("PutLIDMapping com LID em servidor de PN deveria falhar")
	}
	if err := m.PutLIDMapping(ctx, lidJID("1"), lidJID("2")); err == nil {
		t.Fatal("PutLIDMapping com PN em servidor de LID deveria falhar")
	}
}

func TestPutLIDMappingRoundTripBothDirections(t *testing.T) {
	ctx := context.Background()
	m := newTestLIDMap(t)
	pn, lid := pnJID("5511999999999"), lidJID("111111111111111")
	if err := m.PutLIDMapping(ctx, lid, pn); err != nil {
		t.Fatalf("PutLIDMapping: %v", err)
	}

	gotLID, err := m.GetLIDForPN(ctx, pn)
	if err != nil {
		t.Fatalf("GetLIDForPN: %v", err)
	}
	if gotLID != lid {
		t.Fatalf("GetLIDForPN = %s, esperava %s", gotLID, lid)
	}

	gotPN, err := m.GetPNForLID(ctx, lid)
	if err != nil {
		t.Fatalf("GetPNForLID: %v", err)
	}
	if gotPN != pn {
		t.Fatalf("GetPNForLID = %s, esperava %s", gotPN, pn)
	}
}

// O device do JID de entrada e' preservado no de saida: a traducao e' do
// usuario, nao do dispositivo.
func TestGetLIDForPNPreservesDevice(t *testing.T) {
	ctx := context.Background()
	m := newTestLIDMap(t)
	pn, lid := pnJID("5511999999999"), lidJID("111111111111111")
	if err := m.PutLIDMapping(ctx, lid, pn); err != nil {
		t.Fatalf("PutLIDMapping: %v", err)
	}
	withDevice := pn
	withDevice.Device = 5
	got, err := m.GetLIDForPN(ctx, withDevice)
	if err != nil {
		t.Fatalf("GetLIDForPN: %v", err)
	}
	if got.Device != 5 {
		t.Fatalf("o device deveria ser preservado, veio %d", got.Device)
	}
	if got.User != lid.User || got.Server != types.HiddenUserServer {
		t.Fatalf("traducao errada: %s", got)
	}
}

func TestGetLIDForPNUnknownReturnsEmptyJID(t *testing.T) {
	got, err := newTestLIDMap(t).GetLIDForPN(context.Background(), pnJID("5511000000000"))
	if err != nil {
		t.Fatalf("GetLIDForPN: %v", err)
	}
	if !got.IsEmpty() {
		t.Fatalf("PN sem mapeamento deveria devolver JID vazio, veio %s", got)
	}
}

// Um miss consultado no banco e' cacheado como string vazia. Sem isso, cada
// mensagem de um contato sem LID reconsultaria o banco.
func TestGetLIDForPNCachesNegativeResult(t *testing.T) {
	ctx := context.Background()
	m := newTestLIDMap(t)
	pn := pnJID("5511000000000")
	if _, err := m.GetLIDForPN(ctx, pn); err != nil {
		t.Fatalf("GetLIDForPN: %v", err)
	}
	m.lidCacheLock.RLock()
	cached, ok := m.pnToLIDCache[pn.User]
	m.lidCacheLock.RUnlock()
	if !ok || cached != "" {
		t.Fatalf("o miss deveria ser cacheado como string vazia (ok=%v cached=%q)", ok, cached)
	}
}

// Depois de FillCache, cacheFilled=true faz qualquer PN ausente do cache ser
// respondido como "sem mapeamento" SEM tocar no banco.
func TestFillCacheMakesLookupsAuthoritative(t *testing.T) {
	ctx := context.Background()
	m := newTestLIDMap(t)
	pn, lid := pnJID("5511999999999"), lidJID("111111111111111")
	if err := m.PutLIDMapping(ctx, lid, pn); err != nil {
		t.Fatalf("PutLIDMapping: %v", err)
	}
	if err := m.FillCache(ctx); err != nil {
		t.Fatalf("FillCache: %v", err)
	}
	if !m.cacheFilled {
		t.Fatal("FillCache deveria marcar cacheFilled")
	}
	got, err := m.GetLIDForPN(ctx, pn)
	if err != nil {
		t.Fatalf("GetLIDForPN: %v", err)
	}
	if got != lid {
		t.Fatalf("GetLIDForPN = %s, esperava %s", got, lid)
	}
	ausente, err := m.GetLIDForPN(ctx, pnJID("5511000000000"))
	if err != nil {
		t.Fatalf("GetLIDForPN ausente: %v", err)
	}
	if !ausente.IsEmpty() {
		t.Fatalf("com o cache completo, um PN ausente deveria dar JID vazio, veio %s", ausente)
	}
}

// PutLIDMapping apaga primeiro qualquer OUTRO LID apontando para o mesmo PN
// (deleteExistingLIDMappingQuery): um numero de telefone nao pode ter dois LIDs.
func TestPutLIDMappingRemovesConflictingLIDForSamePN(t *testing.T) {
	ctx := context.Background()
	m := newTestLIDMap(t)
	pn := pnJID("5511999999999")
	antigo, novo := lidJID("111111111111111"), lidJID("222222222222222")

	if err := m.PutLIDMapping(ctx, antigo, pn); err != nil {
		t.Fatalf("PutLIDMapping antigo: %v", err)
	}
	if err := m.PutLIDMapping(ctx, novo, pn); err != nil {
		t.Fatalf("PutLIDMapping novo: %v", err)
	}

	got, err := m.GetLIDForPN(ctx, pn)
	if err != nil {
		t.Fatalf("GetLIDForPN: %v", err)
	}
	if got != novo {
		t.Fatalf("GetLIDForPN = %s, esperava %s", got, novo)
	}
	// Um mapa novo (sem cache) confirma que a linha antiga sumiu do banco.
	fresh := NewCachedLIDMap(m.db)
	stale, err := fresh.GetPNForLID(ctx, antigo)
	if err != nil {
		t.Fatalf("GetPNForLID antigo: %v", err)
	}
	if !stale.IsEmpty() {
		t.Fatalf("o LID antigo deveria ter sido apagado do banco, veio %s", stale)
	}
}

func TestPutLIDMappingIsNoOpWhenAlreadyCached(t *testing.T) {
	ctx := context.Background()
	m := newTestLIDMap(t)
	pn, lid := pnJID("5511999999999"), lidJID("111111111111111")
	if err := m.PutLIDMapping(ctx, lid, pn); err != nil {
		t.Fatalf("PutLIDMapping 1: %v", err)
	}
	if err := m.PutLIDMapping(ctx, lid, pn); err != nil {
		t.Fatalf("PutLIDMapping 2: %v", err)
	}
	got, err := m.GetLIDForPN(ctx, pn)
	if err != nil {
		t.Fatalf("GetLIDForPN: %v", err)
	}
	if got != lid {
		t.Fatalf("GetLIDForPN = %s", got)
	}
}

func TestPutManyLIDMappingsRoundTrip(t *testing.T) {
	ctx := context.Background()
	m := newTestLIDMap(t)
	mappings := []store.LIDMapping{
		{LID: lidJID("111111111111111"), PN: pnJID("5511111111111")},
		{LID: lidJID("222222222222222"), PN: pnJID("5511222222222")},
	}
	if err := m.PutManyLIDMappings(ctx, mappings); err != nil {
		t.Fatalf("PutManyLIDMappings: %v", err)
	}
	for _, mp := range mappings {
		got, err := m.GetLIDForPN(ctx, mp.PN)
		if err != nil {
			t.Fatalf("GetLIDForPN %s: %v", mp.PN, err)
		}
		if got != mp.LID {
			t.Fatalf("%s -> %s, esperava %s", mp.PN, got, mp.LID)
		}
	}
}

// Entradas com servidor errado sao descartadas com log de debug, nao fazem o
// lote inteiro falhar — o lote vem do servidor e uma entrada torta nao pode
// derrubar a sincronizacao das outras.
func TestPutManyLIDMappingsSkipsInvalidEntries(t *testing.T) {
	ctx := context.Background()
	m := newTestLIDMap(t)
	valida := store.LIDMapping{LID: lidJID("111111111111111"), PN: pnJID("5511111111111")}
	err := m.PutManyLIDMappings(ctx, []store.LIDMapping{
		{LID: pnJID("1"), PN: pnJID("2")}, // LID no servidor errado
		valida,
		{LID: lidJID("3"), PN: lidJID("4")}, // PN no servidor errado
	})
	if err != nil {
		t.Fatalf("PutManyLIDMappings: %v", err)
	}
	got, err := m.GetLIDForPN(ctx, valida.PN)
	if err != nil {
		t.Fatalf("GetLIDForPN: %v", err)
	}
	if got != valida.LID {
		t.Fatalf("a entrada valida deveria ter sido gravada, veio %s", got)
	}
}

func TestPutManyLIDMappingsEmptyIsNoOp(t *testing.T) {
	if err := newTestLIDMap(t).PutManyLIDMappings(context.Background(), nil); err != nil {
		t.Fatalf("PutManyLIDMappings vazio: %v", err)
	}
}

func TestPutManyLIDMappingsAllInvalidIsNoOp(t *testing.T) {
	err := newTestLIDMap(t).PutManyLIDMappings(context.Background(), []store.LIDMapping{
		{LID: pnJID("1"), PN: pnJID("2")},
	})
	if err != nil {
		t.Fatalf("PutManyLIDMappings so' com entrada invalida: %v", err)
	}
}

func TestGetManyLIDsForPNsEmptyInput(t *testing.T) {
	result, err := newTestLIDMap(t).GetManyLIDsForPNs(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetManyLIDsForPNs: %v", err)
	}
	if result != nil {
		t.Fatalf("entrada vazia deveria devolver nil, veio %v", result)
	}
}

func TestGetManyLIDsForPNsFromDatabase(t *testing.T) {
	ctx := context.Background()
	m := newTestLIDMap(t)
	mappings := []store.LIDMapping{
		{LID: lidJID("111111111111111"), PN: pnJID("5511111111111")},
		{LID: lidJID("222222222222222"), PN: pnJID("5511222222222")},
	}
	if err := m.PutManyLIDMappings(ctx, mappings); err != nil {
		t.Fatalf("PutManyLIDMappings: %v", err)
	}

	// Mapa novo: forca o caminho de consulta ao banco em vez do cache.
	fresh := NewCachedLIDMap(m.db)
	pns := []types.JID{mappings[0].PN, mappings[1].PN, pnJID("5511000000000")}
	result, err := fresh.GetManyLIDsForPNs(ctx, pns)
	if err != nil {
		t.Fatalf("GetManyLIDsForPNs: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("esperava 2 traducoes (o terceiro PN nao tem LID), veio %d: %v", len(result), result)
	}
	for _, mp := range mappings {
		if result[mp.PN] != mp.LID {
			t.Fatalf("%s -> %s, esperava %s", mp.PN, result[mp.PN], mp.LID)
		}
	}
}

func TestGetManyLIDsForPNsFromCache(t *testing.T) {
	ctx := context.Background()
	m := newTestLIDMap(t)
	pn, lid := pnJID("5511111111111"), lidJID("111111111111111")
	if err := m.PutLIDMapping(ctx, lid, pn); err != nil {
		t.Fatalf("PutLIDMapping: %v", err)
	}
	result, err := m.GetManyLIDsForPNs(ctx, []types.JID{pn})
	if err != nil {
		t.Fatalf("GetManyLIDsForPNs: %v", err)
	}
	if result[pn] != lid {
		t.Fatalf("%s -> %s, esperava %s", pn, result[pn], lid)
	}
}

func TestGetManyLIDsForPNsIgnoresNonPNJIDs(t *testing.T) {
	result, err := newTestLIDMap(t).GetManyLIDsForPNs(context.Background(), []types.JID{
		lidJID("111111111111111"),
	})
	if err != nil {
		t.Fatalf("GetManyLIDsForPNs: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("JIDs que nao sao PN deveriam ser ignorados, veio %v", result)
	}
}

func TestGetManyLIDsForPNsPreservesDevice(t *testing.T) {
	ctx := context.Background()
	m := newTestLIDMap(t)
	pn, lid := pnJID("5511111111111"), lidJID("111111111111111")
	if err := m.PutLIDMapping(ctx, lid, pn); err != nil {
		t.Fatalf("PutLIDMapping: %v", err)
	}
	withDevice := pn
	withDevice.Device = 2
	result, err := m.GetManyLIDsForPNs(ctx, []types.JID{withDevice})
	if err != nil {
		t.Fatalf("GetManyLIDsForPNs: %v", err)
	}
	got, ok := result[withDevice]
	if !ok {
		t.Fatalf("o JID com device deveria estar no resultado: %v", result)
	}
	if got.User != lid.User || got.Device != 2 {
		t.Fatalf("traducao = %s, esperava user=%s device=2", got, lid.User)
	}
}

func TestFillCacheOnEmptyDatabase(t *testing.T) {
	m := newTestLIDMap(t)
	if err := m.FillCache(context.Background()); err != nil {
		t.Fatalf("FillCache em banco vazio: %v", err)
	}
	if !m.cacheFilled {
		t.Fatal("FillCache deveria marcar cacheFilled mesmo com banco vazio")
	}
}
