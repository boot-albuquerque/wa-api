package bootstrap

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"wa-api/pkg/infra/db"
)

// ADR-0005 D2. These tests pin the FENCING rule — what to do when a renewal
// fails — which is where this kind of mechanism usually goes wrong.
//
// The happy path (renewed, still the owner) is the least interesting case: a
// manager that never releases anything passes it comfortably.

// fakeLeaseStore imitates the REAL rule from pkg/infra/db/session_lease.go:
//
//	Claim -> (true, nil)   when the lease is free or already ours
//	Claim -> (false, nil)  when ANOTHER owner holds a valid lease (not an error)
//	Claim -> (_, err)      only when the DATABASE fails
//
// A double that returned an error for "I lost ownership" would collapse the two
// cases into one, and the test would agree with the defect — pitfall 1.
type fakeLeaseStore struct {
	mu             sync.Mutex
	owners         map[string]string
	err            error
	calls          int
	ultimoEndereco string
}

func newFakeLeaseStore() *fakeLeaseStore {
	return &fakeLeaseStore{owners: map[string]string{}}
}

func (s *fakeLeaseStore) Claim(_ context.Context, userID, ownerID, ownerAddr string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	// Guardado para que um teste possa provar que o endereco CHEGA ao store.
	// Sem isto, o campo poderia ser lido do gerenciador e nunca gravado, e
	// nenhum teste notaria (ARMADILHAS 25).
	s.ultimoEndereco = ownerAddr
	if s.err != nil {
		return false, s.err
	}
	if current, exists := s.owners[userID]; exists && current != ownerID {
		return false, nil
	}
	s.owners[userID] = ownerID
	return true, nil
}

func (s *fakeLeaseStore) Release(_ context.Context, userID, ownerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	if s.owners[userID] == ownerID {
		delete(s.owners, userID)
	}
	return nil
}

func (s *fakeLeaseStore) failWith(err error) {
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()
}

func (s *fakeLeaseStore) giveTo(userID, ownerID string) {
	s.mu.Lock()
	s.owners[userID] = ownerID
	s.mu.Unlock()
}

func (s *fakeLeaseStore) remaining() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.owners)
}

// TestLease_LostOwnershipReleasesImmediately: losing to another replica admits
// no tolerance. Continuing to serve produces the two simultaneous owners this
// mechanism exists to prevent — and F89 measured that WhatsApp settles that by
// killing one of them for good.
func TestLease_LostOwnershipReleasesImmediately(t *testing.T) {
	store := newFakeLeaseStore()
	manager := newLeaseManager(store, "pod-A", "pod-A:8080", 15*time.Second, 5*time.Second, nil)

	if ok, err := manager.Claim(context.Background(), "u1"); err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}

	store.giveTo("u1", "pod-B")

	if manager.renewOne(context.Background(), "u1") {
		t.Error("stayed owner after another replica took the lease")
	}
}

// TestLease_DatabaseDownWithinTTLKeeps: a network blip must NOT kill a healthy
// session. If it did, the cure would be worse than the disease — a few seconds
// of trouble would cost a session reconnect on every replica.
func TestLease_DatabaseDownWithinTTLKeeps(t *testing.T) {
	store := newFakeLeaseStore()
	clock := time.Now()
	manager := newLeaseManager(store, "pod-A", "pod-A:8080", 15*time.Second, 5*time.Second, nil)
	manager.now = func() time.Time { return clock }

	if ok, err := manager.Claim(context.Background(), "u1"); err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}

	store.failWith(fmt.Errorf("%w: connection refused", db.ErrLeaseUnavailable))
	clock = clock.Add(9 * time.Second) // within the 15s TTL

	if !manager.renewOne(context.Background(), "u1") {
		t.Error("dropped the session on a database failure INSIDE the TTL; killing a healthy session over a network blip is worse than the original problem")
	}
}

// TestLease_DatabaseDownBeyondTTLReleases is the other half, and it is what
// separates this design from a naive one.
//
// Once the TTL has passed without a CONFIRMED renewal, the lease has expired
// from every other replica's point of view — any of them may legitimately take
// over. Continuing to serve here, merely because we cannot reach the database,
// is exactly how two owners are created. The decision is not "did the database
// answer?", it is "how long since the last confirmation?".
func TestLease_DatabaseDownBeyondTTLReleases(t *testing.T) {
	store := newFakeLeaseStore()
	clock := time.Now()
	manager := newLeaseManager(store, "pod-A", "pod-A:8080", 15*time.Second, 5*time.Second, nil)
	manager.now = func() time.Time { return clock }

	if ok, err := manager.Claim(context.Background(), "u1"); err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}

	store.failWith(fmt.Errorf("%w: connection refused", db.ErrLeaseUnavailable))
	clock = clock.Add(16 * time.Second) // BEYOND the 15s TTL

	if manager.renewOne(context.Background(), "u1") {
		t.Error("kept ownership past the TTL without a confirmed renewal: another replica may already have taken over, making two owners")
	}
}

// TestLease_LossTriggersDisconnect: coordinating who CONNECTS is not enough.
// F89 measured that the loser of a StreamReplaced never reconnects, so whoever
// loses ownership must drop the session itself, before WhatsApp settles it.
func TestLease_LossTriggersDisconnect(t *testing.T) {
	store := newFakeLeaseStore()
	var dropped []string
	var mu sync.Mutex
	manager := newLeaseManager(store, "pod-A", "pod-A:8080", 100*time.Millisecond, 10*time.Millisecond, func(userID string) {
		mu.Lock()
		dropped = append(dropped, userID)
		mu.Unlock()
	})

	if ok, err := manager.Claim(context.Background(), "u1"); err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); manager.RunHeartbeat(ctx) }()
	// F130: cancelling is not enough — nobody waited for the goroutine, so it
	// outlived the test and kept reading the global log.Logger that the NEXT
	// test writes. Defers run LIFO, so the order below is load-bearing:
	// cancel() (declared last) runs FIRST, then wg.Wait(). Swapping the two
	// lines waits for a heartbeat nobody told to stop, and the test hangs.
	defer wg.Wait()
	defer cancel()

	store.giveTo("u1", "pod-B")

	deadline := time.After(3 * time.Second)
	for {
		mu.Lock()
		count := len(dropped)
		mu.Unlock()
		if count > 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("the heartbeat detected the loss but never called the disconnect hook; the session would go zombie")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

// TestLease_ReleaseAllClears: without this, failover waits out the full TTL on
// a routine deploy — and F89 measured reconnect at 1.2s to 2.3s, so the TTL
// would dominate.
func TestLease_ReleaseAllClears(t *testing.T) {
	store := newFakeLeaseStore()
	manager := newLeaseManager(store, "pod-A", "pod-A:8080", 15*time.Second, 5*time.Second, nil)

	for _, userID := range []string{"u1", "u2", "u3"} {
		if ok, err := manager.Claim(context.Background(), userID); err != nil || !ok {
			t.Fatalf("claim %s: ok=%v err=%v", userID, ok, err)
		}
	}

	manager.ReleaseAll(context.Background())

	if held := len(manager.ownedSessions()); held != 0 {
		t.Errorf("still holds %d sessions after ReleaseAll", held)
	}
	if left := store.remaining(); left != 0 {
		t.Errorf("%d leases were left in the store; failover would wait out the TTL", left)
	}
}

// TestLease_DeniedClaimIsNotRecorded: if it were recorded, the heartbeat would
// start renewing a session that is not ours and the process would believe it
// owns it.
func TestLease_DeniedClaimIsNotRecorded(t *testing.T) {
	store := newFakeLeaseStore()
	store.giveTo("u1", "pod-B")
	manager := newLeaseManager(store, "pod-A", "pod-A:8080", 15*time.Second, 5*time.Second, nil)

	ok, err := manager.Claim(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("claimed a session owned by someone else")
	}
	if held := len(manager.ownedSessions()); held != 0 {
		t.Errorf("recorded %d sessions even though the claim was denied", held)
	}
}

// TestLeaseSettings_HeartbeatMustFitInTTL: with an interval greater than or
// equal to the TTL, the very first renewal already arrives late and the session
// is dropped during NORMAL operation. Refusing at startup beats discovering it
// in production.
func TestLeaseSettings_HeartbeatMustFitInTTL(t *testing.T) {
	cases := []struct {
		ttl, heartbeat string
		wantErr        bool
	}{
		{"15", "5", false},
		{"10", "5", false},
		{"10", "6", true}, // heartbeat over half the TTL
		{"5", "5", true},  // equal
		{"5", "10", true}, // heartbeat longer than the TTL
		{"0", "5", true},
		{"15", "0", true},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("ttl=%s_heartbeat=%s", c.ttl, c.heartbeat), func(t *testing.T) {
			t.Setenv(envLeaseTTL, c.ttl)
			t.Setenv(envLeaseHeartbeat, c.heartbeat)
			_, _, err := leaseSettings()
			if c.wantErr && err == nil {
				t.Errorf("ttl=%s heartbeat=%s was accepted", c.ttl, c.heartbeat)
			}
			if !c.wantErr && err != nil {
				t.Errorf("ttl=%s heartbeat=%s rejected: %v", c.ttl, c.heartbeat, err)
			}
		})
	}
}

func TestLeaseSettings_DefaultsAreValid(t *testing.T) {
	t.Setenv(envLeaseTTL, "")
	t.Setenv(envLeaseHeartbeat, "")
	ttl, heartbeat, err := leaseSettings()
	if err != nil {
		t.Fatalf("defaults rejected: %v", err)
	}
	if ttl != 15*time.Second || heartbeat != 5*time.Second {
		t.Errorf("ttl=%s heartbeat=%s, want 15s and 5s", ttl, heartbeat)
	}
}

// TestBuildOwnerID_StableAndNonEmpty: on k8s the hostname is the pod name,
// which is what an operator looks for when asking "who holds this session?".
func TestBuildOwnerID_StableAndNonEmpty(t *testing.T) {
	id := buildOwnerID()
	if id == "" {
		t.Fatal("owner id is empty")
	}
	if id != buildOwnerID() {
		t.Error("owner id is not stable within the same process")
	}
}

// TestLease_ReleaseHandsBackASingleLease covers the F96 fix at the manager
// level: a session claimed but never started must give its lease back, or the
// heartbeat renews ownership of something that does not exist and no other
// replica can ever take that user.
func TestLease_ReleaseHandsBackASingleLease(t *testing.T) {
	store := newFakeLeaseStore()
	manager := newLeaseManager(store, "pod-A", "pod-A:8080", 15*time.Second, 5*time.Second, nil)

	for _, userID := range []string{"u1", "u2"} {
		if ok, err := manager.Claim(context.Background(), userID); err != nil || !ok {
			t.Fatalf("claim %s: ok=%v err=%v", userID, ok, err)
		}
	}

	manager.Release(context.Background(), "u1")

	if held := len(manager.ownedSessions()); held != 1 {
		t.Errorf("still holds %d sessions, want 1: Release must drop exactly one", held)
	}
	if left := store.remaining(); left != 1 {
		t.Errorf("%d leases left in the store, want 1", left)
	}
}

// TestLease_ReleaseForgetsEvenWhenTheStoreFails: if the manager kept renewing a
// lease it failed to delete, the session would stay unreachable to every
// replica until the process died. Forgetting locally lets the TTL clear it.
func TestLease_ReleaseForgetsEvenWhenTheStoreFails(t *testing.T) {
	store := newFakeLeaseStore()
	manager := newLeaseManager(store, "pod-A", "pod-A:8080", 15*time.Second, 5*time.Second, nil)

	if ok, err := manager.Claim(context.Background(), "u1"); err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	store.failWith(fmt.Errorf("%w: connection refused", db.ErrLeaseUnavailable))

	manager.Release(context.Background(), "u1")

	if held := len(manager.ownedSessions()); held != 0 {
		t.Errorf("still holds %d sessions after a failed Release: the heartbeat would keep renewing a lease we tried to drop", held)
	}
}

// --- F98: leases held for sessions that no longer exist ---------------------
//
// The safety net for the class the orchestrator fix covers one case of. Any
// asynchronous death of a session — the QR expiring, the transport dropping,
// anything a future path forgets to hand the lease back on — leaves the
// heartbeat renewing ownership of nothing, and pins the user to this replica
// for as long as the process lives.

// TestLease_AbandonedSessionIsReleased: no live session and past the grace
// period, the lease must go back. Measured before the fix, in `multi` mode with
// a real Postgres: connected=0, GET /session/status answering "no session", and
// expires_at still moving forward three minutes after the pairing died.
func TestLease_AbandonedSessionIsReleased(t *testing.T) {
	store := newFakeLeaseStore()
	manager := newLeaseManager(store, "pod-A", "pod-A:8080", 30*time.Millisecond, 10*time.Millisecond, nil)
	manager.hasLiveSession = func(string) bool { return false }

	if ok, err := manager.Claim(context.Background(), "u1"); err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); manager.RunHeartbeat(ctx) }()
	// F130: cancelling is not enough — nobody waited for the goroutine, so it
	// outlived the test and kept reading the global log.Logger that the NEXT
	// test writes. Defers run LIFO, so the order below is load-bearing:
	// cancel() (declared last) runs FIRST, then wg.Wait(). Swapping the two
	// lines waits for a heartbeat nobody told to stop, and the test hangs.
	defer wg.Wait()
	defer cancel()

	// Bounded wait, never WaitGroup.Wait(): a test that hangs reports nothing
	// (ARMADILHAS.md 16).
	deadline := time.After(3 * time.Second)
	for {
		if store.remaining() == 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("the lease of a session that no longer exists was renewed instead of released; under N pods this user is pinned to this replica forever")
		default:
			time.Sleep(2 * time.Millisecond)
		}
	}
}

// TestLease_StartingSessionKeepsItsLease is the control that stops the fix from
// becoming a worse defect than the leak.
//
// Ownership is claimed BEFORE the session is materialized — deliberately, so a
// denial leaves nothing dirty behind. So "no session yet" is the NORMAL state
// of a healthy startup for a moment. Releasing on first sight of it would drop
// the lease of every session that is still coming up, which is precisely the
// race the lease exists to prevent.
func TestLease_StartingSessionKeepsItsLease(t *testing.T) {
	store := newFakeLeaseStore()
	// A TTL far longer than the test: the session is inside the grace period
	// for the whole run.
	manager := newLeaseManager(store, "pod-A", "pod-A:8080", time.Hour, 5*time.Millisecond, nil)
	manager.hasLiveSession = func(string) bool { return false }

	if ok, err := manager.Claim(context.Background(), "u1"); err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); manager.RunHeartbeat(ctx) }()
	// F130: cancelling is not enough — nobody waited for the goroutine, so it
	// outlived the test and kept reading the global log.Logger that the NEXT
	// test writes. Defers run LIFO, so the order below is load-bearing:
	// cancel() (declared last) runs FIRST, then wg.Wait(). Swapping the two
	// lines waits for a heartbeat nobody told to stop, and the test hangs.
	defer wg.Wait()
	defer cancel()

	time.Sleep(60 * time.Millisecond) // a dozen heartbeats

	if store.remaining() != 1 {
		t.Fatal("the lease of a session still coming up was released; every startup would lose ownership in its first milliseconds")
	}
}

// TestLease_LiveSessionKeepsItsLease is the other control, in the opposite
// direction: a session that EXISTS must never be touched by this check, no
// matter how long it has been running.
func TestLease_LiveSessionKeepsItsLease(t *testing.T) {
	store := newFakeLeaseStore()
	manager := newLeaseManager(store, "pod-A", "pod-A:8080", 10*time.Millisecond, 5*time.Millisecond, nil)
	manager.hasLiveSession = func(string) bool { return true }

	if ok, err := manager.Claim(context.Background(), "u1"); err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); manager.RunHeartbeat(ctx) }()
	// F130: cancelling is not enough — nobody waited for the goroutine, so it
	// outlived the test and kept reading the global log.Logger that the NEXT
	// test writes. Defers run LIFO, so the order below is load-bearing:
	// cancel() (declared last) runs FIRST, then wg.Wait(). Swapping the two
	// lines waits for a heartbeat nobody told to stop, and the test hangs.
	defer wg.Wait()
	defer cancel()

	time.Sleep(80 * time.Millisecond) // many TTLs' worth of heartbeats

	if store.remaining() != 1 {
		t.Fatal("the lease of a LIVE session was released; the running session would be handed to another replica")
	}
}

// TestLease_WithoutLiveSessionCheckKeepsRenewing pins the nil case: every
// existing caller that does not wire the check must keep the old behaviour,
// rather than silently start releasing leases.
func TestLease_WithoutLiveSessionCheckKeepsRenewing(t *testing.T) {
	store := newFakeLeaseStore()
	manager := newLeaseManager(store, "pod-A", "pod-A:8080", 10*time.Millisecond, 5*time.Millisecond, nil)
	// hasLiveSession deliberately left nil.

	if ok, err := manager.Claim(context.Background(), "u1"); err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); manager.RunHeartbeat(ctx) }()
	// F130: cancelling is not enough — nobody waited for the goroutine, so it
	// outlived the test and kept reading the global log.Logger that the NEXT
	// test writes. Defers run LIFO, so the order below is load-bearing:
	// cancel() (declared last) runs FIRST, then wg.Wait(). Swapping the two
	// lines waits for a heartbeat nobody told to stop, and the test hangs.
	defer wg.Wait()
	defer cancel()

	time.Sleep(80 * time.Millisecond)

	if store.remaining() != 1 {
		t.Fatal("a manager with no live-session check released a lease; single mode and every existing test would change behaviour")
	}
}

// TestLease_EnderecoChegaAoStore fecha o seam da decisão 1 do ADR-0007.
//
// O endereço podia perfeitamente existir como campo do gerenciador, aparecer no
// construtor, e nunca ser gravado — nenhum outro teste notaria, porque nenhum
// outro olha o que a chamada leva. Este olha.
func TestLease_EnderecoChegaAoStore(t *testing.T) {
	store := newFakeLeaseStore()
	manager := newLeaseManager(store, "pod-A", "10.1.2.3:8080", 15*time.Second, 5*time.Second, nil)

	if _, err := manager.Claim(context.Background(), "user-1"); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.ultimoEndereco != "10.1.2.3:8080" {
		t.Errorf("endereco gravado = %q, want 10.1.2.3:8080 — quem for rotear leria a posse sem saber onde o dono esta'",
			store.ultimoEndereco)
	}
}

// TestEnderecoAnunciado_VariavelTemPrecedencia: só o operador sabe o que é
// alcançável de fora. Adivinhar de dentro do processo é heurística que funciona
// na máquina de quem escreveu.
func TestEnderecoAnunciado_VariavelTemPrecedencia(t *testing.T) {
	t.Setenv(envAdvertiseAddr, "wa-api-0.wa-api-hl:9000")

	if addr := buildOwnerAddr(); addr != "wa-api-0.wa-api-hl:9000" {
		t.Errorf("buildOwnerAddr() = %q, want o valor declarado na variavel", addr)
	}
}

// TestEnderecoAnunciado_SemVariavelUsaHostnameEPorta fixa o palpite honesto: o
// mesmo hostname que o `owner_id` já usa, para que dono e endereço contem a
// mesma história.
func TestEnderecoAnunciado_SemVariavelUsaHostnameEPorta(t *testing.T) {
	t.Setenv(envAdvertiseAddr, "")

	addr := buildOwnerAddr()
	if addr == "" {
		t.Skip("hostname indisponivel nesta maquina; o caminho de fallback nao e' exercitavel aqui")
	}
	if !strings.HasSuffix(addr, ":"+*port) {
		t.Errorf("buildOwnerAddr() = %q; deveria terminar na porta em uso (%s)", addr, *port)
	}
	if strings.Contains(addr, ownerIDUnknownHost) {
		t.Errorf("buildOwnerAddr() = %q; caiu no host desconhecido com hostname disponivel", addr)
	}
}

// F109. O par de testes abaixo trava a DISTINÇÃO, não a mensagem: renovar um
// lease que ainda era meu e retomar um que já tinha expirado são eventos
// diferentes, e antes disto os dois saíam idênticos — em silêncio.
//
// O que torna o par honesto é o segundo: sem ele, um aviso emitido em TODA
// renovação passaria no primeiro e ninguém notaria.

func TestLease_RetomadaAposExpirarDeixaRastro(t *testing.T) {
	store := newFakeLeaseStore()
	manager := newLeaseManager(store, "pod-A", "pod-A:8080", 15*time.Second, 5*time.Second, nil)

	relogio := time.Now()
	manager.now = func() time.Time { return relogio }

	if _, err := manager.Claim(context.Background(), "user-1"); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	var buf bytes.Buffer
	orig := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = orig })

	// Congelamento além do TTL: o lease expirou e foi RETOMADO, não renovado.
	relogio = relogio.Add(20 * time.Second)
	if !manager.renewOne(context.Background(), "user-1") {
		t.Fatal("renewOne devolveu false; o dublê concede a posse, entao o caminho medido nem foi alcancado")
	}

	saida := buf.String()
	if saida == "" {
		t.Fatal("nada registrado: o processo serviu numa janela em que nao era dono legitimo e nao ficou rastro (F109)")
	}
	if !strings.Contains(saida, "RETOMADO") {
		t.Errorf("o aviso nao distingue retomada de renovacao: %s", saida)
	}
	// A lacuna precisa estar no registro: sem ela, quem investiga sabe QUE
	// houve janela e nao sabe de quanto.
	if !strings.Contains(saida, `"gap"`) {
		t.Errorf("o aviso saiu sem a duracao da lacuna: %s", saida)
	}
}

// TestLease_RenovacaoNormalNaoAvisa é o controle negativo do teste acima, e o
// que impede a correção de virar ruído: com heartbeat de 5s, um aviso por
// renovação seriam 12 linhas por minuto por sessão.
func TestLease_RenovacaoNormalNaoAvisa(t *testing.T) {
	store := newFakeLeaseStore()
	manager := newLeaseManager(store, "pod-A", "pod-A:8080", 15*time.Second, 5*time.Second, nil)

	relogio := time.Now()
	manager.now = func() time.Time { return relogio }

	if _, err := manager.Claim(context.Background(), "user-1"); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	var buf bytes.Buffer
	orig := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = orig })

	// Três renovações dentro do prazo, como na operação normal.
	for i := 0; i < 3; i++ {
		relogio = relogio.Add(5 * time.Second)
		if !manager.renewOne(context.Background(), "user-1") {
			t.Fatalf("renovacao %d devolveu false", i)
		}
	}

	if saida := buf.String(); saida != "" {
		t.Errorf("renovacao normal gerou aviso; com heartbeat de 5s isso seria ruido perpetuo: %s", saida)
	}
}

// countHeartbeatGoroutines reports how many live goroutines are inside
// RunHeartbeat right now, by name in the runtime's own stack dump.
//
// The dump is the only honest instrument here: the defect of F130 is a
// goroutine that OUTLIVES its test, and no assertion on the manager's own
// state can see one, precisely because the manager is done with it.
func countHeartbeatGoroutines() int {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			return strings.Count(string(buf[:n]), "leaseManager).RunHeartbeat")
		}
		buf = make([]byte, 2*len(buf))
	}
}

// TestLease_HeartbeatNaoSobreviveAoTeste is the F130 guard.
//
// The measured defect was not "the heartbeat logs the wrong thing": it was
// that `go manager.RunHeartbeat(ctx)` with only `defer cancel()` leaves a
// goroutine running AFTER the test returns, still reading the global
// log.Logger that the next test overwrites. The race detector caught it once
// in a full `make check` and refused to reproduce in 570 targeted runs — so
// the guard cannot be "run -race and hope". It has to observe the leak
// DIRECTLY, which is what this test does.
//
// Two assertions, and the second is the one that would have caught F130:
//
//  1. while the heartbeat is running, the stack dump sees it — otherwise the
//     instrument is blind and assertion 2 would pass for the wrong reason;
//  2. once the launcher block has returned, ZERO heartbeats remain.
func TestLease_HeartbeatNaoSobreviveAoTeste(t *testing.T) {
	if leftover := countHeartbeatGoroutines(); leftover != 0 {
		t.Fatalf("a heartbeat from an EARLIER test is still running (%d): the leak this test guards against already happened upstream", leftover)
	}

	observedWhileRunning := 0

	// The launcher block reproduces, verbatim, the corrected pattern used by
	// the five launch sites in this file. If someone reverts one of them to
	// `defer cancel(); go manager.RunHeartbeat(ctx)`, the same revert here
	// makes this test fail instead of making the suite intermittently red.
	func() {
		store := newFakeLeaseStore()
		manager := newLeaseManager(store, "pod-A", "pod-A:8080", time.Hour, time.Millisecond, nil)

		if ok, err := manager.Claim(context.Background(), "u1"); err != nil || !ok {
			t.Fatalf("claim: ok=%v err=%v", ok, err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		wg.Add(1)
		go func() { defer wg.Done(); manager.RunHeartbeat(ctx) }()
		// Same LIFO order as the five launch sites: cancel() runs FIRST,
		// wg.Wait() second. Inverted, this blocks forever.
		defer wg.Wait()
		defer cancel()

		// Wait until the instrument actually sees the goroutine, so a zero at
		// the end means "it exited", not "it never started".
		deadline := time.After(3 * time.Second)
		for observedWhileRunning == 0 {
			observedWhileRunning = countHeartbeatGoroutines()
			select {
			case <-deadline:
				t.Fatal("the heartbeat never appeared in the stack dump; the leak detector below would pass vacuously")
			default:
				time.Sleep(time.Millisecond)
			}
		}
	}()

	if observedWhileRunning == 0 {
		t.Fatal("instrument blind: never observed a running heartbeat")
	}
	if leftover := countHeartbeatGoroutines(); leftover != 0 {
		t.Fatalf("%d heartbeat goroutine(s) survived the launcher: this is F130 exactly — the survivor keeps reading the global log.Logger that the next test writes", leftover)
	}
}
