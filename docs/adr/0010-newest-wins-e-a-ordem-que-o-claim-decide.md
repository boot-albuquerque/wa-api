# ADR-0010: "newest wins" em `account_ownership` significa "claim mais recentemente aceito pelo coordenador", não "autenticação mais recente"

- **Status**: accepted
- **Data**: 2026-08-26 (achado original, `feature/macbook-lucas`); portado
  para esta worktree em 2026-08-28 junto com `account_ownership.go`
  (migração 20) — ver HOUSEKEEP F368.
- **Relacionado**: ADR-0005 (posse de sessão, D1/D2/D3),
  `pkg/infra/db/account_ownership.go`. A referência original a "HOUSEKEEP
  F280 (achado da auditoria adversarial em `feature/capability-final-audit`)"
  é de outra worktree — o mecanismo que este ADR formaliza
  (`ClaimAccountIdentity`) chegou aqui já correto, sem o achado que o
  motivou lá.

## Contexto

O item 4 do prompt arquitetural que originou `account_ownership.go` pedia:
"o claim autenticado mais recente vence (newest wins)". A auditoria
adversarial (F280) atacou essa frase literalmente: criou uma sessão B
logicamente MAIS NOVA (imagine um token emitido depois) cuja chamada a
`ClaimAccountIdentity` chega ao banco PRIMEIRO, e uma sessão A logicamente
MAIS ANTIGA cuja chamada chega DEPOIS (rede lenta, retry, fila). O resultado
medido foi: A (mais antigo, chegando por último) supera B (mais novo,
chegando primeiro) — o oposto do que "mais recente vence" sugere se lido
como "mais recente autenticado".

O comentário que já existia em `ClaimAccountIdentity` era honesto sobre
isso — "the caller of THIS call always wins" — mas essa decisão nunca tinha
sido registrada como decisão consciente em ADR ou HOUSEKEEP antes do achado.

## A pergunta que não tem resposta sem relógio distribuído

"Autenticar" e "fazer o claim" (chamar `ClaimAccountIdentity`) são operações
DISTINTAS, potencialmente separadas por rede, fila ou retry. Perguntar "qual
sessão autenticou por último" entre duas réplicas exige comparar instantes
medidos por processos diferentes — e isso não é uma comparação confiável sem
um relógio distribuído (NTP com deriva limitada não é suficiente para a
granularidade que decidiria uma corrida de milissegundos, e este projeto não
tem, nem quer ter, essa dependência).

O que É observável de forma consistente entre réplicas, sem relógio nenhum,
é a ordem em que o COORDENADOR AUTORITATIVO — aqui, o Postgres — aceitou cada
claim. `ClaimAccountIdentity` já constrói essa ordem:

1. `pg_advisory_xact_lock(hashtextextended(identity || '::' || engine, 0))`
   serializa, no próprio banco, qualquer par de claims concorrentes para a
   mesma `(canonical_account_identity, engine)` — não importa em qual
   conexão, processo ou réplica a chamada se originou.
2. Dentro dessa seção crítica, `SELECT ... FOR UPDATE` lê o owner ativo
   atual, e `ownership_revision` é atribuída como `anterior + 1` — uma
   sequência monotônica cuja fonte da verdade é a transação do banco, nunca
   `time.Now()` de nenhum processo chamador.
3. A linha vencedora é sempre a última a COMMITAR essa transação para aquele
   par `(identity, engine)`.

## Decisão

**"Mais recente" (newest) significa: o claim válido cuja transação foi mais
recentemente ACEITA (commitada) pelo coordenador autoritativo — o Postgres,
via o par advisory-lock + `ownership_revision`.** Não significa "mais
recente autenticado" e não significa "carimbo de emissão de token mais
novo". A ordem de CLAIM decide, não a ordem de autenticação.

Consequência direta, testada em
`pkg/infra/db/account_ownership_newest_wins_test.go`
(`TestNewestWins_ClaimOrderDecidesNotAuthOrder`): se A autentica primeiro mas
faz o claim depois de B (que autenticou depois mas fez claim antes), A vence
— porque o claim de A foi aceito por último pelo coordenador. Isto é o MESMO
comportamento que `TestAudit_AccountOwnership_ArrivalOrderNotClaimRecency`
(a auditoria) mediu e chamou de "quebra da invariante 4" sob a leitura
literal antiga; sob esta ADR, é o comportamento CORRETO e passa a ser
verificado como tal, não como defeito.

Nenhum algoritmo mudou. `ClaimAccountIdentity` já implementava exatamente
esta semântica antes deste achado — o que faltava era o registro explícito
da decisão, e a suíte de testes cobrindo os interleavings que a tornam
verificável (ver "Cobertura" abaixo).

### Por que não a alternativa (b) — timestamp de emissão do lado do chamador

O texto do achado F280 oferecia, como alternativa, introduzir um timestamp
de emissão do token do lado do chamador e comparar isso em
`ClaimAccountIdentity`. Rejeitado: um timestamp gerado por um processo
chamador é exatamente o tipo de dado que não se pode comparar com confiança
entre réplicas sem relógio distribuído — reintroduziria, por uma porta
lateral, o problema que a arquitetura inteira (advisory lock +
`ownership_revision`) foi desenhada para evitar. Se o produto vier a
precisar de uma noção real de "reconexão fora de ordem" tolerante a
autenticação mais antiga vencendo, isso é um requisito NOVO e teria de vir
com seu próprio ADR — não é o que "newest wins" no prompt original pedia,
lido à luz desta decisão.

## Invariantes que continuam valendo (não mudaram com esta ADR)

1. No máximo um owner ativo por `(account_identity, engine)` — índice
   parcial único `idx_account_ownership_active_unique`, estrutural.
2. Um novo claim válido substitui o owner anterior.
3. O anterior vira `superseded`.
4. O novo permanece `active`.
5. Callbacks/eventos antigos são rejeitados por fencing — comparação de
   `ownership_revision` contra a linha ativa atual.
6. Falha no cleanup do runtime antigo (`domain.Fencer.Fence`) NÃO devolve
   ownership ao antigo: o claim já comitou antes de `Fence` ser chamado (ver
   `domain.FenceOutcome`), e não existe caminho de código que reverta uma
   linha `active` por causa de uma falha de fence.
7. Restart do processo não ressuscita sessão `superseded` — o estado só
   existe no Postgres, nunca em memória de processo.
8. Duas réplicas concorrentes reivindicando o mesmo `(identity, engine)`
   nunca terminam com dois owners ativos — `pg_advisory_xact_lock` serializa
   entre CONEXÕES, não apenas entre goroutines de um processo.

## Cobertura

| interleaving | teste |
|---|---|
| caso trivial (A claim → B claim) | `TestNewestWins_TrivialSequential` |
| A autentica → B autentica → B claim → A claim | `TestNewestWins_ClaimOrderDecidesNotAuthOrder` |
| claims concorrentes, mesma réplica | `TestNewestWins_ConcurrentClaimsSameReplica`, `TestAccountOwnership_Race` |
| claims concorrentes, réplicas diferentes (duas `*sqlx.DB`) | `TestNewestWins_ConcurrentClaimsDifferentReplicas` |
| callback atrasado do perdedor (fencing) | `TestAccountOwnership_Fencing` |
| restart após supersede | `TestAccountOwnership_Restart` |
| falha no cleanup da sessão antiga | `TestNewestWins_CleanupFailureDoesNotReturnOwnership` |
| proteção estrutural (bypass via SQL cru) | `TestAccountOwnership_StructuralConstraint` |
| cross-engine não colide | `TestAccountOwnership_CrossEngine` |

Todos em `pkg/infra/db/`, contra Postgres real
(`WA_API_TEST_POSTGRES`), sem duble — ARMADILHAS #1 do projeto já
estabelece que a correção deste mecanismo não é medível por dublê.

## Nota sobre o teste de auditoria original

A worktree onde este ADR nasceu tinha um `account_ownership_audit_test.go`
com `TestAudit_AccountOwnership_ArrivalOrderNotClaimRecency` — o teste da
auditoria adversarial que descreveu o resultado como "quebra da invariante
4" antes desta formalização. Esse arquivo não existe nesta worktree (não
fazia parte do que foi portado) — a trilha histórica que ele documentava
fica só no ADR original. `TestNewestWins_ClaimOrderDecidesNotAuthOrder`,
aqui, já afirma o comportamento correto diretamente, sem precisar do par
histórico ao lado.
