# Handoff — iniciativa `wa-headless`

**Data:** 2026-08-10 · **Fase:** saindo de DECISÃO, entrando em EXECUÇÃO
**Escrito para:** o próximo engenheiro/agente, que continua daqui sem ter vivido
a sessão.

> ## O objetivo, numa frase
>
> **O `wa-api` precisa ter, no `wa-headless`, um motor FUNCIONALMENTE
> EQUIVALENTE ao `whatsapp-web.js`.**
>
> Tudo neste documento serve a isso. Equivalência de motor é o alvo — não
> substituir o `wa-worker`, não consolidar serviços, não reescrever coordenação.
> Ver §1 e, para o que isso descarta, §3.1.

---

## 1. Objetivo final

### O objetivo

> **Ter no `wa-api` um motor `wa-headless` funcionalmente equivalente ao
> `whatsapp-web.js`.**

Equivalência significa: tudo que o produto hoje obtém do `wwebjs`, ele passa a
poder obter do `wa-headless`, pela mesma interface, com confiabilidade não pior.

Isso e nada além disso. Explicitamente **fora** deste objetivo:

- substituir o `wa-worker` ou reescrever sua coordenação;
- consolidar serviços num único binário Go;
- mitigação de detecção/anti-ban (frente própria — ADR-0006 D3);
- superar o `wwebjs` em desempenho (o baseline dele nunca entrou no repo, então
  "superar" é objetivo, não critério).

### O que é o `wa-headless`

O **segundo motor** do `wa-api`: conduz o SPA real de `web.whatsapp.com` num
Chromium headless, dirigido por CDP, em Go.

O primeiro motor é o `wa-noise` (`internal/wa-noise/`, 178 mil linhas), que fala
o protocolo nativo e **já funciona**. Os dois convivem: protocolo é a pilha
primária e barata; browser é o motor caro que alcança o que o protocolo não
alcança.

### Que problema resolve

O `wwebjs` entrega hoje recursos que exigem o SPA real, e o produto depende
deles. O problema que o `wa-headless` resolve é **ter esse acesso sob controle
nosso**, em Go, dentro do `wa-api`, em vez de depender de uma biblioteca de
terceiro acoplada a dezenas de nomes de módulo internos da Meta (ADR-0006 D4).

A segunda justificativa registrada no ADR-0006 — camada extra de mitigação de
ban — **não é entregue** e não faz parte deste objetivo.

### Estado final

A iniciativa está concluída quando o `wa-headless` serve, pela interface que o
`wa-worker` já consome, tudo que o `wwebjs` serve e que o produto usa — com
paridade medida lado a lado — e o `wwebjs` pode ser desligado sem perda
observável.

**A medida de "equivalente" é a lista do CAP-01**, não a superfície inteira do
`wwebjs`: o alvo é o que o produto CHAMA, não o que a biblioteca OFERECE.

---

## 2. Arquitetura decidida

### 2.1 Componentes e responsabilidades

| componente | repo | papel | linguagem |
|---|---|---|---|
| `app-core`, `gateway`, `dispatcher` | `disparazaap` | produto: campanha, ritmo, quota, política | TS / Go |
| `wa-worker` | `disparazaap` | **coordenação**: lease, fencing, shard, on-demand, spool, roteamento de comando | TS |
| `wa-noise` | `wa-api` | provider de protocolo (whatsmeow) | Go |
| **`wa-headless`** | `wa-api` | **provider de browser** (Chromium+CDP) | Go |

**Fronteira de responsabilidade, decidida:** `wa-api` é **transporte**;
`disparazaap` é **política**. Anti-ban, seleção de cliente, ritmo e higiene de
lista **não** entram no `wa-api`.

### 2.2 Comunicação

```
app-core ──NATS──> wa-worker ──WaClientAdapter──> [wwebjs | wa-api]
                       │                              │
                       │                              └─HTTP/WS─> wa-api
                       └──KV (lease/fencing) ──> NATS JetStream
```

- **Comando para instância:** `commands.instance.{id}.*`, consumido só pelo dono
  atual do lease.
- **`wa-worker` → provider:** interface `WaClientAdapter` (TS).
- **`wa-worker` → `wa-api`:** HTTP + WS (contrato em §4).

### 2.3 Ciclo de vida de sessão

FSM do `wa-worker` (`application/fsm.ts`):

```
Unassigned → Claimed → Starting → QRRequired → Pairing → Connected
                          └────────────────────────────→ Connected (restore)
Connected ⇄ Heartbeating
Connected → Reconnecting → Connected | SessionExpired → QRRequired
qualquer → Released → Unassigned
```

**Uma sessão ativa por perfil** (`MEASURED` 3/3): duas abas do WhatsApp no mesmo
perfil não coexistem. Isolamento é **por perfil**, nunca por aba.

### 2.4 Persistência

- Perfil Chromium por instância em `WA_SESSION_ROOT/{instanceId}/`.
- `LocalAuth` + volume persistente (ADR-0012); `RemoteAuth` com store
  cifrado/fenced quando `AdapterDurabilityDeps.sessionStore` é injetado.
- Lease e fencing token em KV com TTL.

### 2.5 Decisões descartadas

**DECISÃO DESCARTADA:** isolar sessões por contêiner (`BrowserContext`, ou
Firefox Multi-Account Containers).
**MOTIVO:** `BrowserContext` custa ~142 MB por página contra ~10 MB no contexto
default (~14×), medido por CDP cru. E isolamento separa **armazenamento**, não
compartilha **memória** — o grosso do custo é dado por sessão. Reabre só se ficar
provado que o `browser_ui` por contexto é evitável por flag.

**DECISÃO DESCARTADA:** atacar o custo minificando/cacheando a SPA.
**MOTIVO:** o heap de JS é **64,8 MB de 651 MB de `anon`** (~10%), a 2% do
próprio limite. Memória aqui é **contagem de processo**, não tamanho de bundle —
e o código é da Meta.

**DECISÃO DESCARTADA:** compartilhar renderer entre sessões pelo mesmo origin.
**MOTIVO:** o bloqueio não é o modelo de processo — **armazenamento é
identidade**. Mesmo perfil ⇒ mesmo IndexedDB ⇒ mesmo dispositivo pareado ⇒
conflito de sessão.

**DECISÃO DESCARTADA:** `rod` como controlador.
**MOTIVO:** chromedp entregou 3,1–3,3× mais throughput em amd64 e o `rod`
colapsou com 18,8% de falhas em concorrência 32, além de derrubar browser
externo compartilhado ao fechar conexão.

**DECISÃO DESCARTADA:** reduzir renderers como lever de memória.
**MOTIVO:** refutada nesta sessão (Fase 6/H1). Os 2 renderers existem no boot,
não são os `browser_ui` (teste causal, N=3), e 5 flags candidatas não os
removem.

---

## 3. Estado atual do código

### 3.1 A descoberta que muda a leitura — LEIA ANTES DE PLANEJAR

O `wa-worker` **já tem a costura de provider pronta e exercitada**:

```
services/wa-worker/src/infrastructure/wa/
  adapter.ts               216 linhas  ← A INTERFACE (o contrato)
  adapter-selector.ts       79 linhas  ← AdapterKind e policy por instância
  whatsapp-web-js-adapter.ts           ← provider atual
  wa-api-adapter.ts        426 linhas  ← PROVIDER wa-api, JÁ IMPLEMENTADO
  wa-api-http-client.ts                ← cliente HTTP do wa-api
  fake-adapter.ts / fake-single-adapter.ts
```

```ts
export type AdapterKind = 'wwebjs' | 'wa-api' | 'fake' | 'fake-single';
```

**`wa-api` já é um provider de primeira classe**, selecionável por
`WA_ADAPTER`, com roteamento por conta previsto no envelope do comando.

Consequência: **"trocar o motor" e "substituir o `wa-worker`" são decisões
separáveis.**

| leitura | o que muda | tamanho |
|---|---|---|
| **A — ADR-0038** | `wa-headless` absorve coordenação; serviço Go único | reescrever ~14 mil linhas de TS (fencing, shard-ring, quota, circuit-breaker, on-demand, spool, rebalancer) |
| **B — pela costura** | `wa-api` ganha motor headless; `wa-worker` aponta `WA_ADAPTER` | implementar o motor; `wa-worker` intocado |

**Com o objetivo do §1, a escolha é B.** Equivalência de motor se alcança
implementando o motor atrás da costura que já existe; absorver o `wa-worker` é
outra iniciativa, com outro custo e outro risco.

**A ADR-0038 (serviço Go único absorvendo o `wa-worker`) fica FORA do escopo
desta iniciativa.** Ela foi decidida antes desta evidência e antes de o objetivo
ser fixado como equivalência de motor. Não está revogada — está deslocada para
depois, e precisa de revisão própria.

Correção a propor pelo `HOUSEKEEP.md` do `disparazaap`, **não editada daqui**:
aquele repo é da sessão C0/C1.

### 3.2 O que já existe

| item | onde | tamanho | estado |
|---|---|--:|---|
| `wa-noise` | `wa-api/internal/wa-noise` | 178.408 | funcionando |
| `wa-worker` | `disparazaap/services/wa-worker/src` | 14.292 | em produção |
| **`wa-headless`** | `wa-api/internal/wa-headless` | **132** | **só `doc.go` — esqueleto** |
| harness do estudo | `wa-api/scripts/chromium-study` | 9.228 | **validado, fora do produto** |

O módulo de produto tem **zero implementação**: `core/`, `engine/`, `runtime/`,
`spa/`, `observability/`, `capabilities/send/` são todos um `doc.go` cada,
declarando intenção.

### 3.3 O que pode ser reutilizado — e é muito

Tudo isto está **provado com controle negativo** em `scripts/chromium-study/`:

| peça | arquivo | o que garante |
|---|---|---|
| `DeadlinePolicy` + `Runner` + `OpLog` | `p4c_deadline.go` | prazo por classe de operação; nenhum caminho CDP bloqueia; registro de onde parou |
| `InteractionPolicy` | `p5_interaction.go` | Resolve→Validate→Act→Verify; 9 cenários hostis PASS |
| Suite hostil | `p5_hostile.go` | ground truth na página; critério de dois lados |
| `cleanStop` / `closeBrowserViaCDP` | `p4c_lifecycle.go` | desligamento que não corrompe sessão |
| Teste de política de shutdown | `shutdown_policy_test.go` | trava a F94 estaticamente |
| Censo de targets/processos | `p6_targets.go` | atribuição por PSS; teste causal |
| `primeTab`, `waitAppReady`, `snapshot` | `p4c_target.go` | classificação de estado de página |

### 3.4 O que pertence a quê

- **`wwebjs`:** `whatsapp-web-js-adapter.ts` e o pacote npm. **Será
  substituído** — mas só quando o novo provider tiver paridade.
- **`wa-worker`:** coordenação (`application/*`, `infrastructure/kv/*`,
  `infrastructure/nats/*`). **Continua existindo** na leitura B; seria reescrito
  na leitura A.
- **Adaptado:** `adapter-selector.ts` (nenhuma mudança estrutural — só passar a
  selecionar `wa-api`), e `wa-api-adapter.ts` (talvez nada, se o contrato HTTP
  for o mesmo).
- **Não existe:** o motor headless inteiro dentro de `internal/wa-headless`.

---

## 4. Contratos que precisam ser preservados

### OBRIGATÓRIO PARA COMPATIBILIDADE

**C1 · A interface `WaClientAdapter`** (`adapter.ts`). É o contrato central.
Obrigatórios: `start`, `stop`, `on`, `onContact`, `listContacts`, **`sendText`**
(explicitamente marcado "OBRIGATÓRIO, não opcional").
Opcionais: `fetchMessages`, `fetchContactAvatar`, `onMessageMeta`,
`getBrowserPid`, `refreshOwner`, `livenessCheck`, `backupNow`,
`primeContactRoster`.

**C2 · A máquina de estados de auth** — `WaAuthStatus`:
`starting | qr | authenticated | ready | auth_failure | disconnected`.
O `WaEvent` carrega `qr`, `qrExpiresAt`, `msisdn`, `ownerPushname`,
`ownerAvatarUrl`, `reason`.

**C3 · O contrato HTTP do `wa-api`** já consumido pelo `wa-api-http-client.ts`:
`/session/connect`, `/session/qr`, `/session/status`, `/session/logout`,
`/session/profile`, `/user/contacts`, `/user/contacts/sync`,
`/user/contacts/last-activity`, `/user/avatar`, `/admin/users`.
*(Lista obtida por grep; confirmar se envio passa por HTTP ou WS — §7.)*

**C4 · Formas de dado:** `WaSyncedContact` (`phoneE164`, `displayName`,
`waJid`, `pushname`, `isMyContact`, `lastInteractionAt`) e `WaMessageMeta`
(`waJid`, `waMessageId`, `direction`, `type`, `waTimestamp`).

**C5 · A invariante METADATA-ONLY.** `WaMessageMeta` **nunca** carrega corpo,
texto ou mídia. Está escrito na interface como invariante rev2.

**C6 · ZERO PII em log.** Nunca `waJid` cru, nunca `body`. É BLOCKER do repo.

**C7 · Comando e lease:** `commands.instance.{id}.*` consumido só pelo dono;
lease com TTL e fencing token derivado da revisão do KV.

**C8 · `sendText` lança em falha.** O runner traduz exceção em
`message_status: failed`. Retornar erro silencioso quebraria o produto.

### PODE SER ALTERADO / MELHORADO

- **Como o motor obtém os dados** — `window.require` vs protocolo é detalhe de
  implementação atrás de C1.
- **Durabilidade** — `LocalAuth` vs `RemoteAuth` é injetado, não fixo.
- **`getBrowserPid`** — específico de Puppeteer; um motor Go pode expor
  equivalente próprio ou omitir (é opcional).
- **Estrutura interna do `wa-headless`** — os pacotes `core/engine/runtime/spa`
  são intenção declarada, não contrato.
- **Instrumentação e métricas** — acrescentar é livre.

---

## 5. Fluxos principais

### F1 · Criar sessão e parear (QR)

```
Entrada:      comando claim em commands.instance.{id}.*
Processamento: lease KV (TTL+fencing) → boot Chromium com perfil da instância
               → primeTab → navegar → classificar página
Estado:       Claimed → Starting → QRRequired
Evento:       WaEvent{status:'qr', qr, qrExpiresAt}
Persistência: perfil em WA_SESSION_ROOT/{id}/; lease renovado por heartbeat
```

### F2 · Restaurar sessão existente

```
Entrada:      claim de instância já pareada
Processamento: restaurar perfil → boot → navegar → app-ready
Estado:       Claimed → Starting → Connected
Evento:       WaEvent{status:'ready', msisdn, ownerPushname, ownerAvatarUrl}
Persistência: perfil intocado; lease renovado
```
`MEASURED`: recovery gracioso p50 **10,4 s**, p95 **13,8 s**; app-ready
observado até **15,8 s** — 15 s de prazo é insuficiente.

### F3 · Enviar texto

```
Entrada:      sendText(waJid, body)
Processamento: Resolve → Validate → Act → Verify (InteractionPolicy)
Estado:       Connected (não transiciona)
Evento:       retorna {waMessageId} | LANÇA
Persistência: nenhuma no provider; o runner emite message_status
```
`MEASURED`: o caminho ingênuo (`chromedp.Click` direto) produziu
**alvo errado 5/5** contra o alvo real, a 3 CPUs, **sem starvation** —
afirmando sucesso nas cinco. Verificação de póscondição é obrigatória.

### F4 · Liveness (novo, vem do H1)

```
Entrada:      timer do runner
Processamento: Evaluate com prazo DO LADO GO
Estado:       Connected | UNRESPONSIVE
Evento:       classificação; N falhas seguidas ⇒ sessão perdida
```
`MEASURED`: existe estado em que a página **não responde por 4+ minutos**
enquanto processo, target e service worker seguem anexados e saudáveis.
Health check estrutural reporta saudável durante toda a janela.

### F5 · Desligar (standby ou shutdown)

```
Entrada:      comando de release, SIGTERM do pod, ou dormir por on-demand
Processamento: Browser.close via CDP cru → ESPERAR a saída do processo
Estado:       → Released → Unassigned
Evento:       stopped_via registrado como métrica
Persistência: perfil descarregado pelo Chromium ao sair limpo
```
`MEASURED`: `SIGTERM` deu **logout na 4ª e na 6ª iteração**; `Browser.close`
fez **40 iterações sem degradação**. `chromedp.Cancel` sobre `RemoteAllocator`
**não envia o comando** e devolve `nil` em silêncio.

### F6 · Recuperar depois de crash

```
Entrada:      lease expirado por TTL
Processamento: outro worker reivindica com fencing token novo
Estado:       Unassigned → Claimed → (F2)
Persistência: reclaim de Singleton no boot é OBRIGATÓRIO em contêiner
```

---

## 6. Requisitos e invariantes

Serão critérios de validação. Cada um tem medição por trás.

1. **Uma sessão ativa por perfil.** Nunca duas abas do WhatsApp no mesmo perfil.
2. **Desligar é `Browser.close` via CDP, e termina na saída do processo** — nem
   sinal, nem helper de biblioteca.

   **Precisão de 2026-08-12 (decisão A do usuário, H5.3).** O sinal **nunca é
   caminho de rotina**; continua sendo recurso final, só depois de o `close` ter
   sido enviado E a saída ter sido esperada. Quando ele acontece, três coisas
   são obrigatórias: `stopped_via` sujo registrado, o perfil **marcado como
   suspeito**, e a sessão **verificada** no boot seguinte em vez de presumida
   boa.

   O que essa precisão corrige: a fase 4C mediu logout na 4ª e na 6ª iteração
   com `SIGTERM` como caminho NORMAL, tomado toda vez. O caso residual mede
   outra coisa — sinal raro, depois de o caminho limpo falhar —, e transportar
   o número de 4C para cá seria usar uma medição fora da condição que a
   produziu. A alternativa ("nunca sinalizar") foi descartada com evidência: o
   `reclaimVerdict` recusa enquanto o pid do detentor viver, então ela
   protegeria a sessão de corrupção **tornando-a inalcançável**. Ver H5 no
   `HOUSEKEEP.md` do módulo.
3. **Nada externo mata o browser por sinal.** `terminationGracePeriodSeconds`
   cabe o pior caso do desligamento limpo.
4. **Toda parada registra `stopped_via`** como métrica.
5. **Nenhum caminho CDP bloqueia indefinidamente**: prazo por classe de
   operação, do lado Go.
6. **Nenhuma espera com relógio na página.** Sem `chromedp.Poll` para prazo, sem
   `requestAnimationFrame` em aba de fundo (ARMADILHAS 19 e 3).
7. **Interação crítica só com póscondição observável verificada.**
8. **Ponto de clique na interseção com o viewport**, nunca no centro geométrico.
9. **Identidade de nó, não só geometria**, entre validar e agir.
10. **Liveness por `Evaluate` com prazo**, nunca por presença de processo/target.
11. **Toda morte de sessão sai com causa classificada**, nunca erro genérico.
12. **`WaMessageMeta` é metadata-only**; zero PII em log.
13. **Reclaim de `Singleton` no boot** é obrigatório em contêiner.
14. **`sendText` lança em falha**, não devolve sucesso silencioso.

---

## 7. Incertezas restantes

**RESOLVIDA — A vs B (§3.1).** O objetivo é equivalência de motor, então o
caminho é B: implementar atrás da costura existente. ADR-0038 sai do escopo
desta iniciativa.

**INCERTEZA:** qual a lista concreta do que o produto usa do `wwebjs`?
**IMPACTO:** é **a definição de "equivalente"**. Sem ela o objetivo não é
verificável, e paridade vira a superfície inteira da biblioteca.
**PRECISA SER RESOLVIDA ANTES DE CODIFICAR? NÃO** para a primeira capacidade;
**SIM** antes de qualquer capacidade de produto. É a incerteza mais importante
que resta.

**INCERTEZA:** o envio passa pelo contrato HTTP do `wa-api` ou por WS? O grep
não achou endpoint de envio.
**IMPACTO:** define o contrato que o motor precisa servir para `sendText`.
**PRECISA SER RESOLVIDA ANTES DE CODIFICAR? NÃO** para a primeira capacidade;
**SIM** antes de F3.

**INCERTEZA:** gatilho do estado de não-resposta do renderer (H1).
**IMPACTO:** se for frequente, afeta a viabilidade; se raro, basta detectar e
reciclar.
**PRECISA SER RESOLVIDA ANTES DE CODIFICAR? NÃO** — o mitigador (invariante 10)
independe da causa.

**INCERTEZA:** custo por sessão em **amd64**. Tudo foi medido em arm64.
**IMPACTO:** viés sistemático no número que multiplica densidade e preço.
**PRECISA SER RESOLVIDA ANTES DE CODIFICAR? NÃO** — é decisão de capacidade,
não de implementação.

**INCERTEZA:** o boundary de CPU do alvo real segue **NÃO MEDIDO**, bloqueado
por um estado de layout sem causa conhecida.
**IMPACTO:** dimensionamento de CPU por sessão.
**PRECISA SER RESOLVIDA ANTES DE CODIFICAR? NÃO**

---

## 8. Riscos técnicos

**ALTO · Escopo real da paridade funcional.** `WaClientAdapter` tem 8 métodos
opcionais além dos obrigatórios, e o `wwebjs` os implementa contra dezenas de
módulos internos da Meta. Reimplementar tudo pode ser maior que o previsto.
*Detecção rápida:* fazer §10/CAP-01 (inventário) antes de qualquer capacidade.

**ALTO · Reescrever coordenação sem necessidade (leitura A).** 14 mil linhas com
fencing, shard-ring, quota, circuit-breaker e spool — cada um custou aprendizado.
*Detecção:* se a primeira estimativa de porte passar de semanas, B é o caminho.

**MÉDIO · Acoplamento ao SPA da Meta.** Renomear um módulo quebra o motor.
*Detecção:* inventário único verificado **no arranque**, falhando alto com a
lista do que sumiu (ADR-0006 D4) — em vez de exceção no meio de um disparo.

**MÉDIO · Estado de não-resposta do renderer.** Sessão viva e inútil.
*Detecção:* invariante 10 — probe de liveness com prazo, e métrica de latência
dela, não só o booleano.

**MÉDIO · Corrupção de sessão no ciclo de standby.** Sustenta 5–10× da economia.
*Detecção:* `Singleton` > 0 após parada, ou perfil encolhendo entre ciclos —
ambos observáveis a cada ciclo, sem esperar o logout.

**MÉDIO · Contas de teste.** Testar N sessões exige N números pareados.
*Detecção:* imediata — trava qualquer teste de escala.

**BAIXO · Densidade pior que o esperado em amd64.**
*Detecção:* `memory.events` deixando de ser zero.

---

## 9. Definition of Done

`wa-headless` é **equivalente ao `wwebjs`** quando **todas** forem verdadeiras:

1. Implementa **cada item da lista do CAP-01** — os obrigatórios de
   `WaClientAdapter` e os opcionais que o produto realmente chama. Equivalência
   se mede contra essa lista, não contra a superfície do `wwebjs`.
2. `WA_ADAPTER` aponta para o motor novo em **100% das instâncias** por período
   definido, sem rollback.
3. Paridade medida **lado a lado, mesma janela**: memória, tempo de restauração
   e **taxa de perda de Conta com denominador** não piores que o `wwebjs`.
4. As 14 invariantes do §6 travadas por teste, **cada uma com controle negativo
   executado** — reintroduzir o defeito e comprovar que a guarda reprova.
5. Zero PII em log, verificado por catraca.
6. `stopped_via` = caminho limpo em 100% das paradas; `Singleton` = 0.
7. Nenhum `chromedp.Poll` para prazo e nenhuma espera com relógio na página, em
   todo o módulo.
8. `wwebjs` removido do `package.json` do `wa-worker` sem quebrar suíte nem e2e.

---

## 10. Mapa de implementação

Cadeia lógica, não microtarefas. Cada capacidade deixa o sistema coerente.

**CAP-01 · Inventário do que o produto realmente usa — A DEFINIÇÃO DE
"EQUIVALENTE"**
→ lista concreta dos métodos de `WaClientAdapter` chamados em produção e dos
recursos que exigem o SPA. *Observável:* documento com a lista, derivada de grep
no `disparazaap`, não de opinião.
**Sem esta lista o objetivo do §1 não é verificável**: "equivalente ao `wwebjs`"
vira a superfície inteira da biblioteca, que é escopo infinito. Com ela,
provavelmente é meia dúzia de operações.

**CAP-02 · Fundação do motor dentro do módulo**
→ `DeadlinePolicy`, `Runner`, `OpLog`, `cleanStop`, `primeTab` portados do
estudo para `internal/wa-headless`. *Observável:* `go test ./internal/wa-headless/...`
verde, com o teste de política de shutdown e seu controle negativo.

**CAP-03 · Sessão sobe e classifica**
→ boot com perfil, navegar, classificar (`qr` / `ready` / `login_required` /
`unresponsive`). *Observável:* contra conta real, o estado correto é reportado.

**CAP-04 · Liveness e causa classificada**
→ probe por `Evaluate` com prazo; toda morte sai com causa. *Observável:* o
estado de não-resposta do H1 é detectado como `UNRESPONSIVE`, não como saudável.

**CAP-05 · Ciclo de vida completo**
→ pareamento por QR, restauração, desligamento limpo, reclaim de `Singleton`.
*Observável:* N ciclos dormir/acordar sem degradação, `Singleton` = 0, perfil
não-decrescente.

**CAP-06 · Inventário de módulos do SPA**
→ nomes de `window.require` num lugar só, resolvidos no arranque, falha alta com
a lista do que faltou. *Observável:* renomear um nome de propósito derruba o
boot com mensagem que nomeia a causa.

**CAP-07 · `sendText` com Verify**
→ a primeira capacidade de produto. *Observável:* mensagem chega, `waMessageId`
retornado, e falha lança.

**CAP-08 · Superfície de leitura**
→ `listContacts`, `onContact`, e os opcionais que o CAP-01 marcou. *Observável:*
mesmos dados que o `wwebjs` para a mesma conta.

**CAP-09 · Fachada do provider**
→ o motor servindo o contrato que o `wa-api-adapter.ts` já consome.
*Observável:* `WA_ADAPTER=wa-api` funciona ponta a ponta com o motor novo.

**CAP-10 · Paridade lado a lado**
→ mesmo comando nas duas pilhas, mesma janela, comparação registrada.
*Observável:* tabela de paridade; é aqui que o baseline do `wwebjs` finalmente
entra no repo.

**CAP-11 · Canário e expansão**
→ uma conta real, depois percentual, com rollback por flag.
*Observável:* métricas do C0 estáveis.

---

## 11. Primeira fronteira executável

**PRIMEIRA CAPACIDADE:** CAP-02 — fundação do motor dentro de
`internal/wa-headless`.

**OBJETIVO:** mover para o módulo de produto as peças do estudo que já estão
validadas: `DeadlinePolicy`, `Runner`, `OpLog`, `cleanStop`/`closeBrowserViaCDP`,
`primeTab`, e o teste estático de política de shutdown com seu controle
negativo.

**POR QUE COMEÇAR POR ELA:**
- É a **única capacidade idêntica nas leituras A e B** do §3.1 — não aposta na
  incerteza em aberto, então não vira trabalho descartável.
- Não depende de nenhuma outra sessão, de contrato novo, nem de pareamento.
- Converte 9.228 linhas provadas que hoje moram num diretório **descartável por
  desenho** em código de produto.
- Toda capacidade seguinte precisa dela: sem prazo por operação e desligamento
  limpo, CAP-03 em diante nasce com os defeitos que o estudo já pagou para
  descobrir.
- Blast radius **zero**: nada consome `internal/wa-headless` hoje.

**PRÉ-CONDIÇÕES:** nenhuma. Não precisa de conta, nó amd64, contrato ou decisão
A/B.

**ARQUIVOS/ÁREAS PROVAVELMENTE ENVOLVIDOS:**
- origem: `scripts/chromium-study/{p4c_deadline.go, p4c_lifecycle.go,
  p4c_target.go, shutdown_policy_test.go}`
- destino: `internal/wa-headless/{runtime,core,engine}/`
- o `doc.go` de cada pacote já declara a intenção — o código deve caber nela ou
  o `doc.go` muda junto, com o porquê.

**COMO VALIDAR:**
1. `go build ./...` e `go vet ./...` limpos.
2. `go test ./internal/wa-headless/...` verde.
3. O teste de política de shutdown roda no novo caminho.
4. **Controle negativo executado:** reintroduzir `gracefulStop` num caminho que
   carrega credencial e comprovar que o teste reprova, colando a saída.
5. O estudo continua compilando — o porte **não pode** quebrar
   `scripts/chromium-study`, que ainda é o laboratório.

**CRITÉRIO DE DONE:**
- `internal/wa-headless` deixa de ser só `doc.go`.
- Prazo por classe de operação e desligamento limpo disponíveis como API do
  módulo.
- Um teste que falha se alguém desligar por sinal num caminho com credencial,
  **com a saída da falha registrada**.
- Nenhum comportamento de produção alterado (nada consome o módulo ainda).
