# ADR-0001: Reimplementação nativa e gradual do protocolo binário do WhatsApp

- **Status**: accepted (roadmap de longo prazo, execução incremental)
- **Data**: 2026-08-06

## Contexto

O `wa-api` é construído sobre o [`whatsmeow`](https://go.mau.fi/whatsmeow), uma
biblioteca Go que implementa o protocolo do WhatsApp Web/Multi-Device: o
transporte binário `waBinary` (nós `<iq>`/`<message>` codificados, não HTTP
nem protobuf de topo), a criptografia de sessão (Signal/Noise) e a
serialização protobuf específica de cada tipo de payload (`waE2E`,
`waHistorySync`, `waSyncAction` etc.).

Depender de uma lib de terceiros pra isso é a escolha certa por padrão — não
é razoável reimplementar Signal/Noise do zero por capricho. Mas na prática já
sentimos o custo dessa dependência de duas formas:

1. **O `whatsmeow` não aceita correções nossas com facilidade.** Quando
   encontramos uma lacuna ou comportamento que não serve o `wa-api`, o
   caminho normal (abrir PR upstream, esperar merge, publicar release, subir
   `go.mod`) é lento ou trava. Hoje `go.mod` já fixa um pseudo-commit
   (`v0.0.0-20260516102357-8d3700152a69`), não uma tag — sintoma de que já
   dependemos de estado do `whatsmeow` que não necessariamente vira release
   estável no ritmo que precisamos.
2. **A lib expõe só o que o autor decidiu expor, na forma que ele decidiu
   expor.** O caso concreto que motivou este ADR: `GetProfilePictureInfo`
   (`whatsmeow/user.go:517`) suporta busca condicional — você passa
   `ExistingID` (o `pic.ID` da última foto que já se tem) e, se a foto não
   mudou, o servidor responde rápido com "não mudou" (código 304) em vez de
   reenviar tudo. **Ninguém na nossa pipeline usa isso:**
   - `domain.GetAvatarRequest` (wa-api) só tem `Phone`/`Preview`, sem campo
     pra um ID anterior;
   - `user_adapters.go` sempre chama `GetProfilePictureInfo` com
     `ExistingID: ""`;
   - `app-core` (repo `disparazaap`, `upsert-avatar.ts`) só persiste
     `avatar_url`/`avatar_synced_at` — **descarta o `pic.ID`** que o
     `whatsmeow` devolve a cada fetch bem-sucedido.

   Resultado: **todo fetch de avatar é sempre um fetch completo**, mesmo pra
   contatos cuja foto não mudou desde o último ciclo. Isso consome o cap
   `avatar_fetch` (rate-limited, autoridade do `app-core`) à toa a cada
   rodada de batch — é a explicação mais provável de o batch de avatar
   render pouco por rodada (ver `wa.avatar.batch.done` com `fetched: 0`
   observado em dev, 2026-08-06).

   Esse caso específico **não exige reimplementar nada** — é threadar um
   campo que o `whatsmeow` já expõe (`ExistingID`) através de
   `GetAvatarRequest` → `user_adapters.go` → contrato HTTP → `wa-worker` →
   schema do `app-core` (`avatar_id` além de `avatar_url`). Fica registrado
   aqui como **evidência do padrão maior**: a lib expõe uma capacidade, mas
   como ninguém na nossa cadeia pediu por ela no design original, ela nunca
   chegou a ser usada — e корrigir isso depende só de mudar código nosso, não
   de esperar o `whatsmeow`.

O padrão que nos preocupa é o oposto: quando a lacuna está **dentro** do
`whatsmeow` (um bug, uma falha de mapeamento de erro, um caso do protocolo
que ele não cobre), estamos reféns do ritmo de aceitação de PR upstream.

## Decisão

Adotar como **objetivo de longo prazo, perseguido incrementalmente** a
reimplementação nativa do protocolo binário do WhatsApp — `waBinary` (nós
IQ), a camada de criptografia de sessão, e a serialização dos payloads que
mais nos interessam — dentro do próprio `wa-api`, reduzindo a dependência do
`whatsmeow` peça por peça, começando pelas superfícies onde:

1. já identificamos lacuna concreta e recorrente (como o caso do avatar
   acima), e
2. o custo de reimplementar é baixo comparado ao custo de esperar upstream.

**Não é um rewrite.** O `whatsmeow` continua sendo a base até que cada peça
nativa esteja madura e testada em produção. A migração é **superfície por
superfície**, nunca big-bang:

- Cada peça nativa vive atrás do mesmo *port* (`appport.ContactDirectory` e
  afins) que os adapters `whatsmeow/*` já implementam hoje — trocar a
  implementação por baixo não deve exigir tocar em use case ou handler.
- Critério de partida pra "vale a pena nativizar esta superfície": (a) já
  sentimos dor real nela (bug, lacuna, ou latência de correção upstream), e
  (b) o escopo é pequeno o suficiente pra testar isoladamente contra tráfego
  real antes de substituir o caminho `whatsmeow`.
- Nenhuma superfície native migra sem a mesma cobertura de testes (unit +
  contract) que o `whatsmeow` tem hoje via os testes existentes do adapter.

### Primeira candidata concreta (não iniciada)

**Perfil/avatar** (`w:profile:picture` IQ) é a superfície mais simples pra
começar: request/response pequenos, sem stream de mensagens em tempo real,
já mapeados em `user_adapters.go`. Antes de qualquer reimplementação nativa
aqui, o passo imediato e de baixo risco é o já descrito no Contexto: threadar
`ExistingID`/`avatar_id` pelas camadas existentes usando a API que o
`whatsmeow` já oferece. Isso resolve o sintoma (fetch sempre completo) sem
exigir nenhum trabalho de protocolo nativo — fica registrado como o próximo
passo antes de decidir se vale nativizar essa IQ específica.

## Racional

- **Correções em código nosso não esperam merge alheio.** O ritmo de
  release do `whatsmeow` upstream não é algo que controlamos; peças nativas
  sob nosso controle eliminam essa dependência crítica de tempo.
- **Superfície por superfície é reversível.** Cada port abstrai a troca —
  se uma peça nativa se provar pior que o `whatsmeow` em produção, a
  reversão é trocar o adapter de volta, não desfazer um rewrite.
- **O caso do avatar mostra o padrão sem forçar prematuramente a
  reimplementação binária** — a maioria das lacunas que vamos encontrar
  provavelmente é "a lib expõe, ninguém threadou" (fix rápido, nosso lado),
  não "a lib não expõe" (motivo real pra nativizar). Vale medir caso a caso
  antes de gastar esforço em protocolo binário.

## Alternativas descartadas

- **Fork completo do `whatsmeow` agora**: alto custo de manutenção
  imediato (toda a superfície de protocolo, criptografia incluída) sem
  evidência suficiente ainda de que a maioria das lacunas exige isso — o
  caso do avatar já mostra que parte da dor é uso incompleto da lib, não
  limitação dela.
- **Esperar passivamente por PRs upstream**: já demonstrado que trava o
  ritmo (pseudo-versão pinada em `go.mod` em vez de release estável).
- **Reimplementação big-bang do protocolo**: risco alto demais — criptografia
  de sessão e o transporte binário são superfícies onde um bug é
  silencioso e caro (sessões que não conectam, mensagens perdidas). Migração
  incremental por superfície reduz o raio de explosão de qualquer erro.

## Consequências

- Cada nova lacuna encontrada num adapter `whatsmeow/*` deve, antes de virar
  "esperar PR upstream", ser avaliada contra as duas perguntas do critério de
  partida (dor real + escopo pequeno o bastante pra isolar).
- Este ADR não cria trabalho imediato — a primeira ação concreta é o fix de
  `ExistingID`/`avatar_id` (fora do escopo deste documento, ver Contexto),
  não uma reimplementação de protocolo.
- Revisar este ADR quando a primeira superfície nativa (se houver) estiver
  em produção, pra validar se o critério de partida se provou útil na
  prática.
