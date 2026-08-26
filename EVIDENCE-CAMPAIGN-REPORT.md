# Campanha de redução de evidências — relatório final

**Data:** 2026-08-26
**Base:** `feature/wa-noise` @ `3a0b48b4` (141 operações, nomes canónicos)
**Ramo integrado:** `campaign/evidence-integration`

O alvo desta campanha **não era maximizar ✅**. Era **minimizar incerteza evitável**.
Um ⬜ que vira ❌ com causa conhecida é progresso; um ❌ opaco que vira
`PROTOCOL_CHANGED` com data e PR é progresso; e um 🟡 que continua 🟡 mas
passa a dizer *qual* observador existe e o que o bloqueia também é progresso.

---

## 1. Placar antes → depois

| marca | antes | depois | Δ |
|---|---:|---:|---:|
| ✅ chamada real com efeito confirmado | 98 | **122** | +24 |
| 🟡 `200` sem observador independente | 8 | **8** | 0 |
| ❌ falhou, com o erro medido | 3 | **4** | +1 |
| ⬜ não executada, com o motivo dito | 32 | **7** | **−25** |
| **total** | **141** | **141** | — |

Métricas da campanha:

| métrica | valor |
|---|---:|
| incógnitas (⬜) eliminadas | **25** |
| 🟡 promovidos a ✅ | 0 (nenhum era honesto — ver §4) |
| ❌ com causa determinada | **3 de 3** |
| bugs achados | **22** (F273–F294) |
| bugs corrigidos | 1 (legenda `base.yaml`, com gate) |
| gates novos de regressão | **9** (5 de reconciliação + 4 de concorrência sob `-race`) |
| itens que ainda exigem humano | **16** (`HUMAN-LAST.md`) |
| irreversíveis ainda pendentes | 3 (`IRREVERSIBLE-LAST.md`) |
| afirmações do scorecard derrubadas por medição | **3 de 4** (§12) |

**Nota sobre espelhamento de rota.** Este placar é sobre as **141 operações
canónicas**. Durante a campanha o tronco passou por um estado intermédio com 232
operações (141 canónicas + 91 formas antigas), em que ✅ saltava de 98 para 178 —
mas isso era **duplicação de caminho**, não medição nova: as canónicas herdam a
prova por serem o mesmo handler noutro caminho. `3a0b48b4` retirou as formas
antigas do contrato e o número voltou a 141. **Nada aqui conta espelhamento como
progresso**: as 24 promoções são medição nova contra servidor local com fixtures
descartáveis.

---

## 2. O achado que rendeu os 25 ⬜: a desculpa não era técnica

Dos 32 ⬜, quase todos tinham como motivo alguma variação de
**"mexeria na sessão real em uso"** — *"escreveria a chave HMAC da sessao em
uso"*, *"criaria uma sessao real no servidor em uso"*, *"apagaria uma sessao
real em uso"*.

Isso **não é "não dá para testar"**. É **falta de fixture descartável**. A API
sobe localmente com SQLite, sem dependência externa, e cria utilizadores por
`POST /admin/users`. Uma sessão criada para o efeito, **nunca emparelhada**, é
descartável por construção.

Montado um servidor isolado na porta 8091, com base própria, quatro sessões
descartáveis foram criadas, medidas e apagadas — com a limpeza **também
confirmada por evidência** (`GET /admin/users` a devolver `null` *e*
`select count(*)` a devolver `0`).

### ⬜ → ✅ (24), cada uma com o seu observador independente

| rota | observador que confirmou o efeito |
|---|---|
| `POST /admin/users` | `GET /admin/users`: `null` → array com o id |
| `PUT /admin/users/{id}` | `GET /admin/users/{id}`: `name` e `events` novos |
| `DELETE /admin/users/{id}` | SQLite `count(*)` → `0` |
| `DELETE /admin/users/{id}/full` | `GET /admin/users` + SQLite `count(*)` → `0` |
| `POST /webhook` | `GET /webhook`: `""` → URL + `subscribe` |
| `PUT /webhook` | `GET /webhook`: URL novo, `ReadReceipt` anterior sumiu |
| `DELETE /webhook` | `GET /webhook`: `webhook:""`, `subscribe:[""]` |
| `POST /webhook/history` | `GET /webhook/history`: `0` → `77` |
| `POST /session/history` | `GET /webhook/history`: `77` → `33` |
| `POST /s3/config` | `GET /s3/config`, campo a campo |
| `DELETE /s3/config` | `GET /s3/config`: `enabled:false`, `bucket:""` |
| `POST /s3/configure` | `GET /s3/config` com corpo distinto |
| `POST /session/s3/config` | `GET /session/s3/config` com terceiro corpo |
| `DELETE /session/s3/config` | `GET /session/s3/config` limpo |
| `POST /hmac/config` | `GET /hmac/config`: `""` → `"***"` |
| `DELETE /hmac/config` | `GET /hmac/config`: `"***"` → `""` |
| `POST /hmac/configure` | idem, chave distinta, sobre estado limpo |
| `POST /session/hmac/config` | `GET /session/hmac/config`: `""` → `"***"` |
| `DELETE /session/hmac/config` | idem, inverso |
| `POST /proxy/set` | `GET /session/status`: `proxy_url` novo |
| `POST /session/proxy` | dois: `/session/status` e `GET /admin/users/{id}` |
| `GET /session/connect` | `/session/status`: `connected false→true` + QR real de 1842 chars |
| `GET /session/disconnect` | `/session/status`: `connected true→false` |
| `GET /session/ws` | `101` + `Sec-Websocket-Accept` válido + **três quadros `type: QR`** reais |

O `GET /session/ws` merece nota: o motivo do ⬜ era *"fora do alcance de curl e
do Try it out"*. Isso era verdade sobre o **instrumento**, não sobre a rota —
um cliente RFC 6455 próprio mediu o handshake e os quadros.

### ⬜ → ❌ (1)

`POST /session/logout` — `500 {"error":"internal server error"}` numa sessão
ligada e nunca emparelhada; no log, `the store doesn't contain a device JID`.
Sem transporte vivo devolve `409 session_not_connected`, que **bate com a
documentação**. Achado **F275**.

### Os 7 ⬜ que sobraram, agora com motivo preciso

| rota | motivo (novo, verificado) |
|---|---|
| `POST /s3/test`, `POST /session/s3/test` | docker existe, mas `POST /s3/config` recusa `http://127.0.0.1:9000` com `400 invalid_s3_endpoint` — o validador rejeita loopback, logo o MinIO local é inútil como fixture (F277) |
| `POST /call/reject` | exige chamada a entrar; sem terceiro que ligue |
| `POST /session/pairphone` | exige número real |
| `POST /users/avatar`, `POST /users/privacy`, `POST /users/status` | alterariam a conta real; sem conta descartável emparelhada |

A diferença face ao estado anterior é que **nenhum destes sete diz mais
"mexeria na sessão em uso"**. Cada um nomeia o recurso que falta.

---

## 3. As três ❌: de opacas a `PROTOCOL_CHANGED` com data

A regra do projecto proíbe encerrar com *"não funciona e não sabemos porquê"*.
As três continuam ❌ — **e é correcto que continuem**, porque a marca só se move
quando a rota responder — mas as três têm agora causa.

| rota | classificação | causa |
|---|---|---|
| `POST /users/block` | **`PROTOCOL_CHANGED`** | a escrita da blocklist migrou para endereçamento por **LID**: `<item jid='…@lid' action='block' pn_jid='…@s.whatsapp.net'/>`. Enviamos a forma anterior. |
| `POST /users/unblock` | **`PROTOCOL_CHANGED`** | idem, com `jid`=LID e **sem** `pn_jid`. |
| `POST /newsletters/updates` | **`PROTOCOL_CHANGED`** | o servidor deixou de atender `<message_updates>` endereçado ao JID do canal — **ignora-o**, daí silêncio até ao timeout em vez de `400`. |

### Qual referência respondeu o quê

O `CLAUDE.md` manda registar isto nome por nome, **incluindo as respostas
negativas**. Aqui elas foram decisivas:

- **whatsmeow** deu a *forma*: PR #1137, **2026-08-13** — *"switch UpdateBlocklist to use LIDs"*. Treze dias antes da nossa medição.
- **Baileys** deu a *regra* e a *data*: PR #2265, **2026-04-24** — *"unblock uses LID and block uses LID+PN"*, *"require pn_jid only for block"*. Quatro meses antes do whatsmeow.
- **whatsapp-web.js** confirmou pela via da SPA: `Contact.js:154` converte PN→LID antes de `blockContact`.
- **Baileys #2555** (issue **aberta**, 2026-05-13) trouxe o repro exacto do nosso timeout de canal; **PR #2620** trouxe o stanza do WA Web e a frase *"instead of `<message_updates>` addressed to the channel jid, which timed out"* — fechado por stale-bot, nunca refutado.
- **whatsmeow**, no caso do canal, respondeu *"eu também não resolvo"*: código idêntico ao nosso, intocado desde 2023-10-18.
- **whatsapp-web.js**, no mesmo caso, respondeu *"isto não é comigo"*: dirige a SPA, não emite stanza — a inversão exacta do caso do `sendText`.
- **Evolution API** **não é opinião independente**: consome o Baileys sem lógica de identidade própria.
- **Meta/WhatsApp:** **não há documentação oficial pública** de nenhuma das duas capacidades. Registado como informação válida — **nenhuma afirmação aqui atinge Nível A**.

**Correcção posterior, sobre a graduação dessa ausência.** As três primeiras
versões destes documentos graduavam *"a Cloud API não expõe esta capacidade"*
como **Nível B — ausência verificada**. Estava errado.
`docs/REFERENCIA-META-OFICIAL.md` estabelece que a Cloud API e este projecto são
**superfícies diferentes** — Graph API com conta registada, templates e custo por
conversa, contra o protocolo do WhatsApp Web falado pelo fork em
`internal/wa-noise` — e que cerca de **60 das 141 rotas** daqui (grupos,
comunidades, canais, status) não têm equivalente lá **por desenho**. Os canais,
em particular, são 18 rotas aqui e *"não"* na Cloud API.

Verificar que algo não está num sítio onde nunca estaria **não mede a
capacidade**. As três linhas passaram de **B** para **sem nível**. Isto **não
altera classificação nenhuma**: os três `PROTOCOL_CHANGED` assentam em medição
de campo (B) e em implementações de referência (C/D), e **nenhum desses elos
passa pela Cloud API**. A correcção existe para que uma leitura futura não
empreste à ausência um apoio a `UNSUPPORTED_CONFIRMED` que ela não dá — seria
exactamente a inferência que o `CLAUDE.md` proíbe.

### Duas linhas do registo anterior estavam erradas

1. A F264 dizia *"testado em PN e em LID"*. **Não testou**: `resolveBlocklistPNJID` (`pkg/infra/wa-noise/adapters/user/blocklist.go:103`) converte LID→PN, e as duas entradas produzem o **mesmo** stanza — no sentido **inverso** ao que o upstream passou a exigir. Achado **F278**.
2. A hipótese da F265 (query ID MEX desactualizado) é **falsa**: `GetMessageUpdates` é IQ binário simples e não tem query ID nenhum. Corrigida no lugar.

Nada foi corrigido no código: as três são `PROTOCOL_CHANGED`, não `BUG_LOCAL`.
Fica **proposto** um sub-caso barato — para `unblock`, deixar de degradar o LID
em `pkg/` produziria o stanza correcto sem tocar na biblioteca vendorizada, em
três linhas. Não aplicado: o `CLAUDE.md` manda perguntar primeiro.

---

## 4. Os 🟡: zero promoções, e o resultado inesperado

Nenhum 🟡 subiu. Isso é **conformidade, não falha** — promover sem observador
independente é proibido, e não há conta emparelhada neste ambiente.

O resultado que não se esperava é outro: **nenhuma das oito está sem
observador**. Esperava-se catalogar 🟡 irredutíveis; não há nenhum. O que as
bloqueia é **pré-condição**, não ausência de observador. E **quatro dos oito
motivos registados eram factualmente falsos**:

- `request-unavailable-message` dizia *"não há como observá-lo daqui"* — **há três formas**: o `request_id` do `200` reaparece em `UnavailableRequestID` da mensagem reenviada (`capabilities/message/history_sync.go:250`), legível por `GET /chats/history`, pelo webhook e por `/session/ws`.
- `newsletters/react` dizia *"o canal de teste não tinha mensagem"* — **obsoleto no próprio dia**: a mesma bateria mediu um canal com 60 publicações e `ReactionCounts` reais (`{'👍':3,'😂':58}`).
- `newsletters/mark-viewed` dizia *"não há leitura que confirme"* — **há, por desenho** (`core/newsletter.go:52` e `:206`); está bloqueada pela F265, não pela ausência.
- `status/set/{audio,video}` diziam *"idem — herdada"*. **Herdar evidência não é evidência**; passam a exigir medição própria.

Duas descobertas que poupam trabalho a quem medir:

- **A fila de pedidos de grupo cria-se só por API** — a sequência create → joinapprovalmode → invitelink → join(B) → GET(≠[]) → approve → GET([]) é 100% HTTP e só precisa de duas sessões. Era a pré-condição em falta do `updaterequestparticipants`.
- **A pré-condição da F256 é verificável ANTES de publicar**, por `GET /users/contacts` e `FullName` (`core/broadcast.go:70`) — evita a publicação inútil que não distingue *"não publicou"* de *"publicou e o observador não era destinatário"*.

E um observador foi **desqualificado por medição**: `ViewsCount` de
`/newsletters/messages` veio `0` em 60 de 60. Observador que mostra zero em
todos os casos não distingue caso nenhum.

---

## 5. Auditoria independente: o maior achado é sobre os ✅, não sobre os ⬜

Uma auditoria hostil, que não confiou em nenhuma classificação, reconciliou as
cinco fontes. Quatro batiam; **a que divergia era a legenda `info.description`
— exactamente a que `/docs` serve ao consumidor** — e **toda a suíte passava com
ela errada**.

O veredicto sobre a qualidade da marca mais forte:

| marca | evidência específica |
|---|---|
| ❌ | 3/3 — **100%** |
| 🟡 | 6/8 — 75% |
| ⬜ | 21/32 — 66% |
| **✅** | **0/98 — 0%** |

As 98 ✅ herdadas partilham **a mesma frase-modelo**: *"chamada real com resposta
e efeito confirmado por segunda leitura ou pelo cliente."* Não dizem **qual**
leitura nem **o quê** foi visto. Não se afirma que estejam erradas — afirma-se
que são **inauditáveis a partir do registo**, e que a marca com a afirmação mais
forte é a única sem prova. Achado **F282**.

**Dez ✅ não têm leitura de volta em lado nenhum da API** (archive, pin, mute,
markread, star, presence ×2, subscribe, ephemeral ×2): não existe campo
`archived`/`pinned`/`muted`/`starred`/`unread_count` na spec. E `POST
/chats/markread` é ✅ enquanto a gémea `POST /newsletters/mark-viewed` é 🟡 *"não
há leitura que confirme a marcação"*. São candidatas a 🟡.

**Transições na história: zero.** A tabela nasceu em `b0b8323` já com as 98 ✅ e
nunca mudou até esta campanha. Não há violações da regra de transição porque
**não houve transições** — as 25 desta campanha são as primeiras.

Uma armadilha do próprio instrumento, registada: `grep` ao YAML contava **zero
🟡**. Falso — 🟡 é U+1F7E1, fora do BMP, e o serializador escapa-o para
`"\U0001F7E1"` enquanto ✅/❌/⬜ ficam literais. O gerador estava certo; o
instrumento é que era mais pobre que o formato. **O gate parseia YAML e nunca
faz `grep`.**

---

## 6. Gate novo de reconciliação

`pkg/bootstrap/openapi_reconciliation_test.go` — 5 testes. Compara **rota a
rota**, não só totais, e confere o Total contra a soma das parcelas.

**Controlo negativo EXECUTADO, três variantes:**

1. Marca trocada (`POST /webhook/history` ✅→🟡) → `EXIT=1`, **nomeando a rota**.
2. **Troca compensada** (`/webhook/history` ✅→🟡 *e* `/newsletters/react` 🟡→✅): os quatro totais ficam idênticos e todos os testes de total passam; só a comparação rota a rota acusa — **e acusa as duas pelo nome**. É precisamente a razão de o gate não se contentar com totais.
3. Legenda desalinhada (122→121) → `EXIT=1`, `legenda diz 121 ✅, mas evidencias.tsv tem 122`.

O quarto controlo **não precisou de ser encenado**: na primeira execução o gate
falhou contra o repositório tal como estava, apanhando o defeito real da legenda.

---

## 7. Bugs achados (17) — nenhum corrigido sem autorização

Todos registados com data, contexto, ficheiro:linha, evidência medida,
correcção sugerida e estado. **F273–F289** em `HOUSEKEEP.md`; **LIB-02 a LIB-04**
em `internal/wa-noise/HOUSEKEEP.md`.

Os que merecem decisão primeiro:

- **F273** — o token de uma sessão **apagada continua a autenticar**: `GET /webhook` respondeu `200` depois do `DELETE`, enquanto um token inexistente dá `401`. Vale para `DELETE /admin/users/{id}` **e** para o `/full`. **É fronteira de autenticação.**
- **F274** — `connect → disconnect → connect` devolve `200` e **não religa** (`start already in flight`). É a classe da F108, que a documentação declara fechada, e contradiz a descrição de `/session/disconnect`.
- **F279** — WebP de entrada **não é convertido**: `http.DetectContentType` devolve `image/webp` e cai no `default` de `ConvertToWebPSticker` (`media/sticker/exif.go:62`). A documentação afirmava *"sempre convertida"*. **Explica a bolha vazia do `send/sticker`.**
- **F283** — a regeneração do relatório tinha **apagado a coluna Evidência**, deixando duas promessas (`evidencias.tsv:7`, `base.yaml:122`) a apontar ao vazio. Reposta com observador concreto em 43 das 141 linhas.

Nenhuma correcção de código foi aplicada sem teste de regressão e controlo
negativo executado — só a legenda do `base.yaml`, que nasceu com o gate que a
tranca.

---

## 8. Lotes finais: humano e irreversível

**`HUMAN-LAST.md` — 13 entradas**, agrupadas **por tipo de intervenção** (A sem
conta · B uma sessão · C duas sessões · D terceiro a interagir · E código antes
de medir · F não vale a pena · G fora de escopo). O agrupamento é deliberado: o
objectivo é **lotear as idas ao telemóvel**, para o humano fazer tudo de uma vez
em vez de ser interrompido repetidamente. Cada entrada diz o que já foi tentado,
por que a automação não resolve, a acção necessária, o risco, e a evidência a
produzir.

**`IRREVERSIBLE-LAST.md` — 3 entradas**, com tabela Endpoint / Efeito / Fixture
descartável disponível? / Recuperação / Autorizado?. Nenhuma foi executada:
**um ⬜ com motivo é preferível a um ✅ obtido destruindo recurso real.**

---

## 9. Estado dos gates

| gate | exit |
|---|---:|
| `go build ./...` | 0 |
| `go vet ./...` | 0 |
| `gofmt -l pkg cmd` | 0 |
| `TestGolden`, `TestOpenAPICobreTodasAsRotasRegistadas`, `TestOpenAPINaoInventaRotas`, `TestOpenAPISummariesTrazemMarcaDeEvidencia`, `TestOpenAPIGeradoEstaAtualizado` | 0 |
| os 5 de reconciliação (novos) | 0 |
| `go test ./cmd/logcov/` | 0 |
| `make check` | **2** — uma falha, **provada** pré-existente |

A falha de `make check` é `TestHousekeepEntriesAreMachineReadable` em
`internal/wa-headless` (H144/H5/H14/H90), introduzida há **466 commits** por
`1117852d`. Provada, não assumida: o mesmo teste no worktree do tronco produz
saída **byte a byte idêntica**, e o diff da campanha não toca um único ficheiro
em `internal/wa-headless`. Registada como **F288**.

O `orphan-browser-check` deu um falso positivo numa das corridas — acusou como
órfão um browser cujo dono estava **vivo**, um `go test` de outro worktree.
Registado como **F289**. **`make orphan-browser-clean` não foi corrido**: mataria
a suíte de outra sessão, e quem seguir a instrução do gate sabota trabalho alheio
sem ficar a saber.

---

## 10. Onde está o trabalho

**Ramo integrado: `campaign/evidence-integration`** (12 commits sobre `3a0b48b4`).

Ramos de origem preservados para revisão — os worktrees foram removidos, as
branches ficam:

| ramo | entrega |
|---|---|
| `campaign/evidence-disposable-session` | as 25 promoções + `CAMPANHA-DESCARTAVEL.md` |
| `campaign/evidence-failures` | os dois `INVESTIGATION-*.md` + `WHATSAPP-CAPABILITIES.md` |
| `campaign/evidence-amber-observers` | `OBSERVADORES-AMBAR.md` |
| `campaign/evidence-audit` | `AUDITORIA-EVIDENCIAS.md` + o gate de reconciliação |
| `campaign/evidence-production-gaps` | `MEDICAO-PRODUCAO.md` + os testes de concorrência sob `-race` |

Documentos produzidos, todos no ramo integrado: `CAMPANHA-DESCARTAVEL.md`,
`AUDITORIA-EVIDENCIAS.md`, `OBSERVADORES-AMBAR.md`,
`INVESTIGATION-block-unblock.md`, `INVESTIGATION-newsletter-updates.md`,
`WHATSAPP-CAPABILITIES.md` (sete capacidades — **só as investigadas**, sem
matriz inventada), `HUMAN-LAST.md`, `IRREVERSIBLE-LAST.md`.

---

## 11. O segundo eixo: as quatro ❌ de produção, medidas

O eixo por rota é só metade. O scorecard de prontidão tinha quatro linhas ❌
**arquitecturais** — paginação, idempotência, limitação de ritmo, concorrência.
A regra do projecto manda **medir antes de projectar**, e o teste dessa regra é
que *uma medição útil produz pelo menos um "eu não teria adivinhado"*.

Produziu seis. **Três das quatro afirmações estavam erradas.**

| linha | veredicto | o que a medição mostrou |
|---|---|---|
| **Paginação** | **CAI como generalização** | `/chats/list` **pagina** — `limit` (padrão 50, tecto 500), `offset` e `total`, medido com 2 000 conversas — e paginava desde `9d9dd7ec`, **18 dias antes** de a frase *"nenhuma colecção é paginada"* ter sido escrita. `/chats/history` tem `limit` **sem tecto**; `/users/contacts` é que continua sem nada. |
| **Limitação de ritmo** | **as DUAS metades falsas** | o `x/time/rate` **não** protege a conta no envio: protege o **servidor, por IP** (10 r/s, rajada 20), e está em **observe-only** (`router.go:257`) — 4 rajadas de 60 concorrentes deram **60×`200`, 0×`429`**, só avisos. E a API **responde `429`**, em 64 sítios via `errmap.ClassifyIQ`, como relais do WhatsApp — documentado em **0 de 141** operações. |
| **Idempotência** | confirma-se na letra, **reescreve-se no conteúdo** | zero `Idempotency-Key`, verdade. Mas configuração **é** idempotente (medido), `POST /admin/users` deduplica por `token_hash` → `409` (medido), e o envio já tem `Id` de cliente que vira stanza ID, com dedup local por `UNIQUE(user_id, message_id)` + `ON CONFLICT`. A lacuna real é menor e mais precisa do que a linha dizia. |
| **Concorrência** | **confirma-se no efeito, corrige-se no mecanismo** | não é falta de `ETag` no geral: `UpdateUser` e `SaveProxyConfig` escrevem em `UPDATE` único. A janela real é o par leitura→escrita de `resolveWebhookUseProxy` (`set_proxy.go`) — um `POST /session/proxy` que **omite** `webhook_use_proxy` lê a coluna e reescreve-a, **apagando o valor que um pedido concorrente acabou de declarar e recebeu `200` a confirmar**. |

A concorrência é a única das quatro que ficou **travada em teste**:
`pkg/application/usecase/storage/session_config_concurrency_test.go`, 4 testes
com repositório real sobre SQLite real, sob `-race`. A hipótese "é a cache" foi
testada e **descartada** — `RepublishUser` invalida e relê, converge.

### Os "eu não teria adivinhado"

1. A evidência que *"provava"* que `/chats/list` não pagina — 7 144 bytes — era a **primeira página de 50 de 2 000**, lida como se fosse o total. A resposta trazia `total` e `limit` no próprio corpo.
2. O `x/time/rate` faz o **oposto** do que o scorecard dizia sobre ele.
3. A API responde `429` e o contrato documenta-o em **zero** operações.
4. **0 perdas em 200 rodadas** concorrentes sem controlo de escalonamento — um teste ingénuo teria declarado o sistema seguro. A perda só aparece com encontro marcado.
5. `limit=-1` devolve tudo **só em SQLite** (`LIMIT -1` = sem limite); em Postgres seria `500`. *Não medido contra Postgres* — afirmado da semântica documentada, e dito como tal.
6. `/webhook/history` **não é colecção** apesar do nome: devolve o escalar `users.history`.

O controlo negativo foi executado e **uma das variantes não valeu**: fazer B
declarar o flag faz o teste falhar, mas **noutra asserção** (a de ordem), o que
não prova que a asserção da perda morde. Está dito no documento e no HOUSEKEEP
em vez de contado como controlo válido.

### A verificação servido-vs-repo (ARMADILHAS #27)

Feita como o `CLAUDE.md` prescreve, com o binário a correr:
`curl -s localhost:8093/docs/openapi.yaml | cmp - pkg/…/openapi.yaml` → **exit 0**,
idênticos byte a byte, 963 862 bytes. O binário serve **122 caminhos, 141
operações, 0 `deprecated`** — o que **contradiz** a linha 67 do scorecard
(*"as antigas continuam a funcionar, marcadas `deprecated`"*), corrigida.

Medições em `MEDICAO-PRODUCAO.md`; achados **F290–F294**.

---

## 12. Limite honesto desta campanha

**Não existe conta WhatsApp emparelhada neste ambiente.** Nenhuma medição contra
o servidor real do WhatsApp foi feita nesta campanha, e **nenhuma foi
inventada**. Tudo o que foi medido em campo — os `200`, os `422`, o timeout de
30 s — vem da bateria anterior, de 2026-08-26, e está citado como tal.

O que esta campanha produziu de próprio foi: medição real contra um servidor
local com fixtures descartáveis (as 25), trace de código, pesquisa nas
implementações de referência, e reconciliação. É por isso que os 🟡 não subiram e
que as ❌ continuam ❌ — e ambas as coisas são o resultado correcto, não uma
falha de esforço.
