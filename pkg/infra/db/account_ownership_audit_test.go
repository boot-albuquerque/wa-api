package db

import (
	"context"
	"sync"
	"testing"
)

// AUDITORIA ADVERSARIAL (feature/capability-final-audit) — tenta quebrar,
// com Postgres real, duas invariantes lidas em F276/HOUSEKEEP.md:
//
//   4. "o claim/token mais recente vence, independente da ordem de chegada
//      no DB" — comparado contra o que ClaimAccountIdentity de facto
//      implementa (comentário da própria função: "the caller of THIS call
//      always wins").
//   6. sessões noise e headless para o MESMO número, reivindicadas
//      QUASE SIMULTANEAMENTE, terminam as DUAS ativas (cross-engine não
//      colide).
//
// Rodar:
//
//	WA_API_TEST_POSTGRES="postgres://waapi:waapi@127.0.0.1:5433/waapi?sslmode=disable" \
//	  go test ./pkg/infra/db/ -run TestAudit_AccountOwnership -race -v

// TestAudit_AccountOwnership_ArrivalOrderNotClaimRecency ataca a invariante 4
// tal como o prompt a formulou: "a que tem o claim/token MAIS RECENTE deve
// ganhar, independente da ordem de CHEGADA no DB".
//
// Cenário: sessão A é logicamente MAIS ANTIGA (é a primeira a existir), mas
// sua chamada a ClaimAccountIdentity só chega ao banco DEPOIS da chamada da
// sessão B, que é logicamente MAIS NOVA. Se a invariante 4 valesse como
// escrita, B (o claim mais recente) deveria permanecer ativo mesmo com A
// chegando por último. O código, pelo próprio comentário de
// ClaimAccountIdentity ("the caller of THIS call always wins — there is no
// notion of refusing a claim"), não compara recência nenhuma: implementa
// ORDEM DE CHEGADA NO BANCO, não recência do claim/token.
//
// VEREDITO (ver corpo do teste/relatório): este teste PASSA — ou seja,
// A (o claim mais antigo, chegando por último) efetivamente supersede B (o
// mais recente) — o que **quebra** a invariante 4 tal como formulada no
// prompt. "Newest wins" no código significa "quem chama por último vence",
// não "quem tem o claim mais recente".
func TestAudit_AccountOwnership_ArrivalOrderNotClaimRecency(t *testing.T) {
	repo := NewAccountOwnershipRepository(openTestPostgresForOwnership(t))
	ctx := context.Background()
	const identity, engine = "wa_pn:5511900000001", "noise"

	// B é a sessão logicamente MAIS NOVA (imagine: seu token foi emitido
	// depois do de A), mas chega ao banco PRIMEIRO.
	b, err := repo.ClaimAccountIdentity(ctx, identity, engine, "session-B-newer-token", "owner-B", "claim-B")
	if err != nil {
		t.Fatalf("B (newer token) claim: %v", err)
	}

	// A é a sessão logicamente MAIS ANTIGA, mas sua chamada chega ao banco
	// DEPOIS da de B — ex.: rede lenta, retry, fila de goroutines.
	a, err := repo.ClaimAccountIdentity(ctx, identity, engine, "session-A-older-token", "owner-A", "claim-A")
	if err != nil {
		t.Fatalf("A (older token, arrives later) claim: %v", err)
	}

	current, ok, err := repo.CurrentActiveOwner(ctx, identity, engine)
	if err != nil || !ok {
		t.Fatalf("CurrentActiveOwner: ok=%v err=%v", ok, err)
	}

	if current.SessionID == a.SessionID {
		t.Logf("QUEBROU invariante 4 (formulação do prompt): o claim mais "+
			"ANTIGO (session-A, chegada por último) tornou-se o dono ativo, "+
			"suplantando o claim mais NOVO (session-B, chegada primeiro). "+
			"active=%s revision=%d — pkg/infra/db/account_ownership.go "+
			"ClaimAccountIdentity implementa ORDEM DE CHEGADA, não recência "+
			"de claim/token. Ver o comentário da própria função: \"the "+
			"caller of THIS call always wins\".", current.SessionID, current.OwnershipRevision)
	} else if current.SessionID == b.SessionID {
		t.Fatalf("inesperado: B permaneceu ativo mesmo chegando primeiro e "+
			"sendo suplantado por uma chamada posterior de A — isso "+
			"contradiria o próprio código lido (supersede incondicional de "+
			"quem está ativo). active=%s", current.SessionID)
	}

	// Confirma que B, o claim mais recente, foi de facto REBAIXADO — a
	// consequência direta de "arrival order wins": um token mais novo pode
	// ser destronado por uma sessão mais velha que simplesmente chama depois.
	bStatus, found, err := repo.CurrentStatusForSession(ctx, b.SessionID)
	if err != nil || !found {
		t.Fatalf("CurrentStatusForSession(B): found=%v err=%v", found, err)
	}
	if bStatus.IsActive() {
		t.Fatal("B ainda ativo — o cenário não teve o efeito esperado, refazer a medição")
	}
	t.Logf("confirmado: B (claim mais recente) está superseded por A (claim mais antigo, chegada por último): status=%s superseded_by=%v",
		bStatus.Status, bStatus.SupersededBySessionID)
}

// TestAudit_AccountOwnership_CrossEngine_Concurrent ataca a invariante 6 na
// forma mais adversária possível: duas goroutines disparando
// ClaimAccountIdentity QUASE SIMULTANEAMENTE (sem sequenciamento explícito),
// uma para noise e outra para headless, no MESMO
// canonical_account_identity. A chave de exclusividade do índice parcial é
// (canonical_account_identity, engine) — este teste confirma sob concorrência
// real (não só sequencial, como TestAccountOwnership_CrossEngine) que os
// dois engines não competem pelo mesmo advisory lock nem pela mesma linha.
func TestAudit_AccountOwnership_CrossEngine_Concurrent(t *testing.T) {
	repo := NewAccountOwnershipRepository(openTestPostgresForOwnership(t))
	ctx := context.Background()
	const identity = "wa_pn:5511900000002"

	engines := []string{"noise", "headless"}
	sessionIDs := []string{"session-noise-concurrent", "session-headless-concurrent"}
	results := make([]AccountOwnership, 2)
	errs := make([]error, 2)

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			row, err := repo.ClaimAccountIdentity(ctx, identity, engines[i], sessionIDs[i], "owner-"+sessionIDs[i], "claim-"+sessionIDs[i])
			results[i] = row
			errs[i] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("claim %d (%s/%s) failed: %v", i, engines[i], sessionIDs[i], err)
		}
	}

	for i, engine := range engines {
		status, found, err := repo.CurrentStatusForSession(ctx, sessionIDs[i])
		if err != nil || !found {
			t.Fatalf("CurrentStatusForSession(%s): found=%v err=%v", sessionIDs[i], found, err)
		}
		if !status.IsActive() {
			t.Fatalf("QUEBROU invariante 6: sessão %s (engine=%s) NÃO está ativa após claims concorrentes "+
				"para o mesmo número — cross-engine colidiu. status=%s superseded_by=%v",
				sessionIDs[i], engine, status.Status, status.SupersededBySessionID)
		}
	}
	t.Log("RESISTIU: claims concorrentes noise/headless para o mesmo número terminaram AMBAS ativas")
}
