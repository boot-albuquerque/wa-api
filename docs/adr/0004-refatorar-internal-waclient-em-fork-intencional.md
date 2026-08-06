# ADR-0004: `internal/waclient/` deixa de ser espelho drift-zero e vira fork intencional (exceto código gerado)

- **Status**: accepted (amenda o ADR-0003, aprovado 2026-08-06)
- **Data**: 2026-08-06
- **Amenda**: [ADR-0003](0003-vendorizar-modulo-inteiro-sem-selecao.md), especificamente
  a garantia de "diff vazio contra upstream" que `make waclient-drift` impunha
  como trava, e a premissa implícita do ADR-0002 de que o vendored ficaria
  "sem modificação, exceto se necessário" tratado como exceção rara.

## Contexto

O ADR-0002/0003 vendorizaram `go.mau.fi/whatsmeow` inteiro em
`internal/waclient/` como cópia fiel do upstream (~124.7k linhas, 155
arquivos), com `scripts/waclient-diff.sh` comparando byte-a-byte (após
normalização de import path) contra uma versão upstream baixada, e
`make waclient-drift` travando o `check` se o diff não for vazio. A decisão
original tratava qualquer modificação como exceção pontual, documentada em
`PATCHES.md` (hoje vazio).

Durante a revisão de arquitetura desta mesma sessão (achados F1/F2/F4 em
`HOUSEKEEP.md`, 2026-08-06), ficou claro que os padrões de qualidade que o
`wa-api` já aplica ao resto do repositório — teto de 300 linhas por arquivo
de produção, logging estruturado via zerolog, tradução de erro via
`apperr`, zero magic number/string hardcoded, cobertura de teste unitária E
de integração — nunca foram avaliados para dentro de `internal/waclient/`
em si, porque o ADR-0003 tratava esse diretório como fronteira intocável
por design.

O usuário decidiu explicitamente que **esses padrões devem se aplicar a
todo `internal/waclient/`**, com uma única exceção: arquivos e pares
gerados automaticamente a partir de definições `.proto`
(`internal/waclient/proto/*.pb.go`, 55 pares, ~99.2k linhas) — dividir ou
reformatar um arquivo gerado não tem valor (o gerador simplesmente o
recria do jeito que estava) e reintroduz o risco de corrupção de
descriptor binário já mitigado no vendoring original (ver histórico desta
sessão: `waclient-vendor.sh` teve 3 iterações até isolar a substituição de
import path sem tocar em bytes de descriptor).

## Decisão

1. **`internal/waclient/proto/` permanece intocado e fora de qualquer
   refactor** — mesmo tratamento que código gerado já recebe no resto do
   repo (`.gitattributes` já marca esses arquivos como gerados). O gate
   `waclient-drift` continua válido e travando **especificamente** para
   este subdiretório.
2. **Todo o restante de `internal/waclient/`** — raiz do pacote
   (`client.go` e afins), `appstate/`, `argo/`, `binary/`, `socket/`,
   `store/` (+ `sqlstore/`), `types/` (+ `events/`), `util/` (+
   subpacotes) — passa a seguir os mesmos pilares do resto do `wa-api`:
   SOLID, arquivos de produção ≤ 300 linhas, zero magic number/string
   hardcoded fora de constante nomeada, logging estruturado via o bridge
   `waLog.Logger`→zerolog (ver trabalho em andamento em
   `pkg/infra/whatsmeow/walog/`), e cobertura de teste unitária + de
   integração para o que for tocado.
3. **`make waclient-drift` deixa de ser um gate binário sobre todo o
   diretório** e passa a validar apenas `internal/waclient/proto/` — a
   partir desta ADR, divergência do restante do código contra o upstream é
   **esperada e correta**, não uma falha a corrigir.
4. **Toda modificação continua registrada em `PATCHES.md`** — não como
   exceção pontual (premissa antiga), mas como o mecanismo normal de
   rastrear "o que mudamos e por quê" em um fork agora ativamente mantido,
   arquivo por arquivo/PR, permitindo reconciliar contra futuras versões
   upstream por leitura humana em vez de diff automático.
5. **Execução faseada por prioridade** (não é um único PR/sessão de 124.7k
   linhas):
   - **Fase A (prioridade alta)**: raiz do pacote (`client.go`, handshake,
     dispatch de evento) e `socket/` — pontos de entrada de toda mensagem
     e onde o bridge de log e futuras features customizadas mais
     provavelmente se conectam.
   - **Fase B**: `appstate/` e `store/` (+ `sqlstore/`) — já em uso ativo
     documentado no ADR-0003 (`appstate` importado em 6 arquivos do
     `wa-api` hoje), maior probabilidade de precisar de patch.
   - **Fase C**: `binary/` (XML/binário do protocolo WhatsApp),
     `types/`/`types/events/`, `util/` — menor prioridade imediata.
   - **Fora do escopo até segunda ordem**: `argo/` (dependência isolada de
     baixo acoplamento, sem consumidor próprio dentro do `wa-api` hoje).

## Racional

- **Consistência arquitetural vale mais do que diff-zero para código que
  vamos efetivamente customizar.** O ADR-0002 original já previa
  modificação "se necessário" — esta ADR apenas reconhece que "necessário"
  deixou de ser exceção rara: o `wa-api` pretende usar `internal/waclient/`
  como base ativa de extensão (motivador original do ADR-0001), não como
  vendoring passivo.
- **Código gerado é uma categoria diferente de "vendored".** O racional de
  não tocar em `.pb.go` nunca foi "é do upstream", foi "é derivado
  mecanicamente de outra fonte e reformatá-lo não muda nada que
  importe" — o mesmo racional que já protege `binary/proto/doc.go` e
  outros artefatos gerados no resto do repo.
- **`waclient-drift` restrito a `proto/` preserva o único benefício real do
  diff automático**: garantir que a parte mais frágil e mais fácil de
  corromper silenciosamente (descriptors binários de protobuf) nunca
  diverge por acidente, sem transformar o resto do módulo em um museu que
  não pode evoluir com o resto do projeto.
- **Faseamento por prioridade evita o erro de tratar 124.7k linhas como um
  único trabalho atômico** — cada fase é revisável e testável
  independentemente, e a ordem reflete onde a customização é mais provável
  de ser necessária primeiro (raiz/socket, depois appstate/store, depois o
  resto).

## Alternativas descartadas

- **Manter `internal/waclient/` intocado indefinidamente (status quo do
  ADR-0003)**: rejeitada pelo usuário nesta sessão — os padrões de
  qualidade do projeto (300 linhas, logging, apperr, testes) precisam
  valer para todo código que o `wa-api` mantém ativamente, e
  `internal/waclient/` já deixou de ser "só vendored" no momento em que
  aceitamos customizá-lo.
- **Refatorar tudo de uma vez, um PR gigante**: descartada — 124.7k linhas
  em um único ciclo de revisão é ingovernável e de alto risco (o próprio
  vendoring inicial já teve um bug de corrupção de descriptor por pressa
  em um regex genérico; um refactor massivo sem faseamento reproduziria
  esse tipo de risco em escala maior).
- **Excluir também `binary/` do refactor** (por lidar com parsing
  binário sensível, similar a `proto/`): descartada — `binary/` é código
  Go escrito à mão (parser/encoder do formato WhatsApp), não gerado
  mecanicamente; a sensibilidade do domínio justifica cautela e testes
  extras na Fase C, não exclusão do escopo.

## Consequências

- `make waclient-drift` precisa ser reescrito para escopar apenas
  `internal/waclient/proto/` (hoje compara o diretório inteiro) —
  trabalho de infraestrutura antes de qualquer PR de refactor de conteúdo.
- `PATCHES.md` deixa de ser um arquivo vazio de template e passa a ganhar
  entradas reais a cada PR que toca `internal/waclient/` fora de `proto/`.
- Atualizações futuras do whatsmeow upstream deixam de poder ser
  re-vendorizadas automaticamente via `scripts/waclient-vendor.sh` para
  tudo fora de `proto/` — merges futuros exigirão reconciliação manual
  guiada por `PATCHES.md`. Este é um custo aceito conscientemente, não um
  efeito colateral.
- O trabalho já em andamento em `pkg/infra/whatsmeow/` (splits, bridge
  `walog`, `apperr`) continua válido e serve de base/padrão de referência
  para a Fase A dentro de `internal/waclient/` propriamente dito.
- Cada fase (A/B/C) deve ser planejada via `ralplan` própria antes de
  execução, dado o volume — esta ADR autoriza a direção, não substitui o
  planejamento tático de cada fase.

## Follow-ups

- Reescrever `scripts/waclient-diff.sh`/`make waclient-drift` para escopo
  restrito a `proto/` antes de iniciar a Fase A.
- Abrir `ralplan` para a Fase A (raiz + `socket/`) como próximo passo
  tático desta sessão.
- Avaliar se `PATCHES.md` precisa de um formato estruturado (tabela
  arquivo→motivo→PR) em vez de prosa livre, dado o volume esperado de
  entradas.
