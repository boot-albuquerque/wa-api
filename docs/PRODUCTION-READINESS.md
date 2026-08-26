# Production Readiness Scorecard — wa-api

**Data**: 2026-08-26, revisto depois da padronização de caminhos e da **medição
das quatro lacunas de produção** (`MEDICAO-PRODUCAO.md`). **Medido**, não
estimado: cada número desta página vem de um comando cuja saída está registada
nos commits desta série.

A revisão de 26/08 mexeu na secção 6 porque **três das quatro linhas ❌ diziam
coisas que a medição contradisse**. Uma linha de scorecard que descreve o
mecanismo errado é pior que uma linha em branco: manda corrigir no sítio errado.

## Como ler este documento

O eixo é **implementação → contrato explícito → testes → evidência → auditoria
→ produção**. É deliberadamente agnóstico de tecnologia: as perguntas valem
para qualquer API HTTP, e as respostas é que são deste projeto.

A regra de preenchimento é uma só:

> Uma linha só fica ✅ se houver um **comando que a prove** e que falhe quando
> a propriedade se perder. Sem gate, é 🟡 — "está feito hoje" não é o mesmo que
> "continuará feito".

Um scorecard que se auto-atribui verdes é um instrumento de conforto. Este tem
🟡 e ❌ de propósito, e a coluna que interessa é a última.

---

## Resumo

| estágio | ✅ | 🟡 | ❌ |
|---|---:|---:|---:|
| 1. Implementação | 3 | 2 | 1 |
| 2. Contrato explícito | 10 | 3 | 2 |
| 3. Testes | 5 | 1 | 1 |
| 4. Evidência | 4 | 1 | 0 |
| 5. Auditoria | 5 | 0 | 0 |
| 6. Produção | 2 | 5 | 3 |
| **Total** | **29** | **12** | **7** |

**A leitura honesta**: o contrato e a evidência estão fortes; a **prontidão
operacional não está**. Uma API pode ser perfeitamente documentada e continuar
imprópria para escala — é o caso aqui, e os ❌ do estágio 6 dizem porquê.

---

## 1. Implementação

| propriedade | estado | prova / lacuna |
|---|---|---|
| Todas as rotas registadas são servidas | ✅ | `TestOpenAPICobreTodasAsRotasRegistadas` — enumera de `Routes(Deps{})`, a mesma função que serve o processo |
| Erro interno nunca vaza no corpo | ✅ | `TestRespondJSONNaoVazaDetalheDeErroInterno`, com par de controlo. Sem stack trace, caminho, SQL ou segredo — **em ambiente nenhum** |
| Campo desconhecido é observável | ✅ | log `unknown field "<nome>"` em 48 sítios de descodificação; `WA_API_STRICT_UNKNOWN_FIELDS` recusa |
| Ordem de validação previsível | 🟡 | medida rota a rota e documentada; **não há regra** que a garanta ao acrescentar um campo |
| Valor zero com significado usa ponteiro | 🟡 | regra escrita (contrato §4.5) e aplicada em `mute_chat.go`; **não há gate** que a imponha |
| Erro por campo (`error.details[]`) | ❌ | não existe. A API devolve **um** código, o do primeiro campo que falha. Um pedido com três campos errados revela-os um de cada vez |

## 2. Contrato explícito

| propriedade | estado | prova / lacuna |
|---|---|---|
| Cobertura OpenAPI | ✅ | **141 de 141** operações documentadas. O router serve 234 rotas: 141 canónicas mais 91 formas antigas que continuam a responder e **saíram do contrato** — uma operação, um nome |
| Toda operação tem tag, título e descrição | ✅ | `TestOpenAPIOperacoesEstaoCompletas` |
| **Toda propriedade tem semântica de presença** | ✅ | **694 de 694** com `description` **e** `nullable`. `TestTodaPropriedadeDeclaraNulabilidade` |
| Enums dizem o que acontece fora da lista | ✅ | `TestTodoEnumDizOQueAconteceComValorDesconhecido` |
| Exemplos coerentes com o tipo e com o esquema | ✅ | `TestContratoExemplosSaoValidosContraOTipo` + `TestContratoExemploDeErroBateComOEsquema` (125 exemplos) |
| Códigos de erro documentados existem no código | ✅ | `TestContratoCodigosDeErroExistemNoCodigo` (112 códigos) |
| Nenhuma credencial real na especificação | ✅ | `TestOpenAPINaoTrazSegredoReal` |
| Segredos marcados `writeOnly` | ✅ | 13 propriedades; 69 `readOnly` |
| Envelope de erro único | 🟡 | **duas** formas em produção: `error` objecto e `error` string (F266, 14 pontos). Ambas documentadas, o que impede o cliente de partir — mas é um contrato com duas caras |
| Nomes consistentes | 🟡 | glossário escrito e vinculante para o novo; o existente mistura `snake_case`, `camelCase` e `PascalCase`, e `group_jid` tem **quatro** grafias (F269) |
| Prosa da documentação verificada | 🟡 | os gates leem estrutura, não texto. Oito menções de um código inexistente sobreviveram em descrições até serem apanhadas à mão (ARMADILHAS #29) |
| Singular/plural coerente | ✅ | **91 formas canónicas** registadas a 26/08, com gate que recusa família de colecção sem canónica. As antigas continuam a **responder**, e **saíram do contrato**: medido a 26/08 contra o binário em execução, **0 de 141** operações servidas estão marcadas `deprecated` — não há o que marcar, porque nenhuma forma antiga está no documento |
| Relação no caminho, não no nome | ✅ | 9 rotas reestruturadas com o identificador no caminho e o método a dizer a operação; `TestCaminhosCanonicosCumpremARegra` recusa a relação colada |
| Códigos de estado semânticos | ❌ | `201`, `202`, `204`, `410`, `412`, `415`, `502`, `503`, `504` **nunca** são usados. `POST /group/create` cria recurso e devolve `200` sem `Location`. **`429` saiu desta lista**: é alcançável em 64 sítios (`errmap.ClassifyIQ` → `CategoryRateLimited` → `429`) como relais do estrangulamento do WhatsApp, e **0 de 141** operações o documentam (F293) |
| `additionalProperties: false` | ❌ | não declarado, e **corretamente**: seria falso enquanto o padrão for aceitar. Ligar o modo estrito é decisão de versão |

## 3. Testes

| propriedade | estado | prova / lacuna |
|---|---|---|
| Especificação está em dia com as fontes | ✅ | `TestOpenAPIGeradoEstaAtualizado` |
| Respostas reais batem com o documentado | ✅ | `TestContratoRespostaDeRecusaBateComOEsquema` — **141 operações exercitadas** contra o router |
| Os testes de contrato não podem ficar cegos | ✅ | piso mínimo em cada um; falham se exercitarem menos do que o esperado |
| Controlo negativo em cada gate novo | ✅ | executado e colado nos commits. Duas vezes o primeiro não valeu por partir o build — refeito com mutação que compila (ARMADILHAS #4) |
| Suite passa | ✅ | 116 pacotes verdes |
| Caminho de SUCESSO coberto por contract test | 🟡 | os contract tests exercitam a **recusa** — é o único caminho percorrível sem conta ligada. Está dito no comentário do ficheiro em vez de implícito |
| `make check` totalmente verde | ❌ | uma falha, `internal/wa-headless`, **herdada** e anterior a esta série. Fora do âmbito por instrução |

## 4. Evidência

| propriedade | estado | prova / lacuna |
|---|---|---|
| Classificação por rota, visível no título | ✅ | ✅🟡❌⬜ no `summary` de cada operação, aplicada de `evidencias.tsv` pelo gerador |
| Nenhuma rota sem classificação | ✅ | `TestOpenAPISummariesTrazemMarcaDeEvidencia`; **141 de 141** |
| Tabela não pode desactualizar-se | ✅ | rota nova sem linha **falha o gerador**; linha órfã **também** — a falha por excesso é a silenciosa |
| Os números batem entre as quatro fontes | ✅ | `TestEvidenceTableMatchesSpec`, `TestEvidenceLegendMatchesTable`, `TestEvidenceReportMatchesSpec` e `TestEvidenceReportSummaryMatchesTable` reconciliam tabela, spec embutida, legenda de `/docs` e relatório — **rota a rota**, não só no total (F281) |
| Observador concreto registado por rota | 🟡 | **43 de 141**: as 24 promovidas, as 4 ❌, as 8 🟡 e as 7 ⬜ (24+4+8+7 = 43; o número anterior dizia 25 e não fechava). As restantes 98 ✅ trazem a frase-modelo e são inauditáveis a partir do registo (F282) |
| Distinção ✅ / 🟡 respeitada | ✅ | 🟡 é a recusa deliberada de chamar confirmado o que só devolveu `2xx`. **122 ✅, 8 🟡, 4 ❌, 7 ⬜** |
| Cobertura de efeito confirmado | 🟡 | **122 de 141**, contra 98 antes desta ronda. As 24 promoções são **medição nova** — chamada real contra o servidor local, com observador independente por rota — e **não** espelhamento de nome canónico sobre nome antigo: o eixo de canonicalização não moveu marca nenhuma. As 7 ⬜ que restam exigem um bucket S3 descartável, uma chamada a entrar, ou uma conta emparelhada cujo perfil seria alterado — cada uma com o motivo escrito. O motivo "mexeria na sessão em uso" caiu com as sessões descartáveis |

## 5. Auditoria

| propriedade | estado | prova |
|---|---|---|
| Achados registados com medição | ✅ | **F256** a **F271** no `HOUSEKEEP.md`, com comando e saída |
| Defeitos não mascarados na documentação | ✅ | as 3 rotas que falham estão documentadas **como falhando**, com o erro medido |
| Correcções não alteram contrato sem aval | ✅ | as 3 de comportamento só mudam respostas que **já falhavam**; a padronização de caminhos é puramente ADITIVA — nenhuma rota antiga mudou |
| Erros do auditor também registados | ✅ | 5 meus nesta série: contador mais grosseiro que o gate, acusação injusta a um executor, F267 a apanhar-me, e duas mutações que partiram o build |
| Armadilhas catalogadas | ✅ | 53 entradas em `ARMADILHAS.md`, cada uma com a evidência que a produziu |
| Padronização não parte cliente | ✅ | 143 rotas antigas continuam a responder; medido `GET /chat/list` e `GET /chats/list` a devolverem ambos `200` |

## 6. Produção

**É aqui que a API não está pronta**, e nenhuma quantidade de documentação o
resolve.

| propriedade | estado | prova / lacuna |
|---|---|---|
| Rastreio por pedido | ✅ | cabeçalho `Request-Id` em toda resposta, incluindo erro; `req_id` em todos os logs |
| Logs estruturados | ✅ | `req_id`, `method`, `route` normalizada, `status`, `duration_ms`, `outcome`, `size`. Segredos nunca registados |
| `request_id` no corpo do erro | 🟡 | só no cabeçalho. Correlacionável, mas o cliente tem de o ler de outro sítio |
| Prazos em dependência externa | 🟡 | 30 s por operação contra o WhatsApp, uniforme. **Não é configurável por rota**, e é o que produz o `500` de `/newsletter/updates` |
| Sondas separadas | 🟡 | `/livez` e `/health/ready` distinguem vivacidade de prontidão. Mas `/health` — a funcional — está **atrás de token**, o que a torna inútil para um balanceador |
| **Paginação** | 🟡 | **duas colecções paginam, uma delas sem tecto, e o resto não pagina**. `GET /chats/list` tem `limit` (padrão 50, **máx. 500**), `offset` e `total` — medido com 2 000 conversas, e está assim desde `9d9dd7ec`, 2026-08-08. `GET /chats/history` tem `limit` **sem tecto**: `limit=999999` e `limit=-1` devolveram as 5 001 mensagens (1,5 MB), e 100 pedidos concorrentes levaram o processo de 16 MB a 264 MB de RSS (F291). `GET /users/contacts` continua sem limite nenhum — **61 459 bytes** com 1266 contactos. A afirmação anterior, "nenhuma colecção é paginada", **nunca foi verdade** (F294) |
| **Versionamento** | 🟡 | continua sem `/v1`. Mas a padronização de 26/08 mostrou que **alias lado a lado** resolve tudo o que é ADITIVO — um caminho novo coexiste com o antigo, sem versão. `/v1` continua pré-requisito para o que **não pode coexistir**: mudar o status code de uma rota, ou unificar o envelope de erro |
| **Idempotência** | ❌ | não há `Idempotency-Key` em rota nenhuma. **A lacuna é só no envio**: as rotas de configuração são naturalmente idempotentes (medido: `POST /webhook`, `PUT /webhook`, `POST /session/proxy` repetidos deixam o mesmo estado), e `POST /admin/users` deduplica pelo índice único de `token_hash` — a repetição devolve `409`, não um segundo inquilino. No envio existe já `Id`, escolhido pelo cliente, que vira o stanza ID do WhatsApp, e a nossa escrita local deduplica por ele (`UNIQUE(user_id, message_id)` com `ON CONFLICT`). **Não medido**: se o WhatsApp deduplica na recepção, e se um `500` chegou a enviar — as duas exigem conta emparelhada (`HUMAN-LAST.md`) |
| **Limitação de ritmo** | ❌ | **nenhuma rota recusa por ritmo**, e nenhum `429` vem de protecção nossa. O `x/time/rate` que existe protege o **SERVIDOR, por IP**, e não a conta no envio: 10 req/s, rajada 20, ligado em `router.go:257` em modo **observe-only** — mede e regista, nunca recusa. Medido com 4 rajadas de 60 pedidos concorrentes: **60× `200`, 0× `429`, ~40 avisos** "would have been rejected" por rajada. Um `429` é alcançável, mas só como **relais** do estrangulamento do WhatsApp (F293) |
| **Concorrência** | ❌ | sem `ETag`, `If-Match` ou versão, e há perda silenciosa de escrita — **mas não pelo mecanismo que esta linha dizia**. Não há read-modify-write da linha: `UpdateUser` escreve só os campos informados e `SaveProxyConfig` escreve as duas colunas num único `UPDATE`. A janela é o par leitura→escrita de `resolveWebhookUseProxy`: um `POST /session/proxy` que OMITE `webhook_use_proxy` lê a coluna e reescreve-a, apagando o valor que um pedido concorrente acabou de declarar e foi respondido `200` a confirmar. Travado em `session_config_concurrency_test.go` sob `-race`, com dois controlos negativos executados (F292). **0 perdas em 200 rodadas** sem controlo de escalonamento — o defeito é real e raro |

---

## O que falta, por ordem de dependência

A ordem não é de importância — é de **dependência**. Fazer fora de ordem
duplica trabalho:

1. ~~**`/v1`** como pré-requisito do naming.~~ **Feito de outra forma**: a
   padronização de 26/08 registou as formas canónicas **ao lado** das antigas,
   sem versão. Descobriu-se que o alias resolve tudo o que é ADITIVO — um
   caminho novo pode coexistir com o antigo. O que `/v1` ainda é
   pré-requisito é para o que **não pode coexistir**: mudar o status code de
   uma rota, ou unificar o envelope de erro.
2. **Tecto em `/chats/history`** e **paginação em `/users/contacts`** e
   `/groups/list`. `/chats/list` já pagina, com tecto de 500, desde 2026-08-08 —
   o que fazia deste ponto um ❌ maior do que era. O tecto vem primeiro: sem ele
   qualquer cliente transforma a rota que mais cresce em "traga tudo", e foi o
   que levou o processo a 264 MB com 100 pedidos concorrentes.
3. **`429` e limitação de ritmo** — o número certo depende do perfil de uso e é
   decisão de produto, não de engenharia; o observador por IP já instalado
   existe para o produzir. Ao ligá-lo, o balde vira **recurso limitado**, e a
   invariante do projecto passa a aplicar-se: *nada que espere por relógio ou
   por par morto pode ocupar slot limitado*. Os detentores longos que passariam
   a disputá-lo estão enumerados em `MEDICAO-PRODUCAO.md` §4 — rotas com timeout
   de 30 s contra o WhatsApp, pareamento, e sobretudo o WebSocket, que detém
   pela ligação inteira. As sondas têm de ficar de fora, e o `429` tem de trazer
   `Retry-After`.
4. **Idempotência nas rotas de envio**, com `Idempotency-Key`. As de
   configuração já são idempotentes e não precisam. O `Id` do cliente é o
   candidato natural a chave: já é o stanza ID do WhatsApp e a escrita local já
   deduplica por ele — falta deduplicar ANTES do wire e devolver a resposta
   original em vez de repetir a operação.
5. **`/v2`** com naming canónico, códigos de estado semânticos, envelope de
   erro único, `error.details[]` e `additionalProperties: false`. Tudo o que
   parte contrato, de uma vez, num sítio onde partir é legítimo.

Os pontos 1 a 4 são compatíveis com todos os clientes actuais. O ponto 5 não é,
e é por isso que vem depois do 1.

## Cobertura de mensagens face ao alvo

O alvo — 23 folhas, de `send_text` a `send_order_status` — está em
`docs/REFERENCIA-META-OFICIAL.md` com o estado medido de cada uma:

```
12 ✅ existe e foi exercitada       4 🟡 existe em forma parcial
 1 📥 sabemos receber, não enviar    6 ❌ não existe
```

As seis em falta são **catálogo, produtos, encomendas e Flows** — e não são
"uma rota a mais": dependem de um catálogo associado à conta e, no caso dos
Flows, de um recurso definido no painel da Meta. A capability `catalog` do
`wa-noise` existe e **não está ligada** a rota nenhuma.

**Não conta como lacuna deste scorecard** porque não é prontidão operacional —
é alcance de produto. Está aqui para que os dois não se confundam.

## Como isto se compara com a Cloud API da Meta

`docs/REFERENCIA-META-OFICIAL.md` tem o levantamento oficial, com URLs.

Vários ❌ do estágio 6 são coisas que a Cloud API **tem por construção**, por
ser um serviço da Meta: rate limiting com `429`, versionamento do Graph API,
paginação com cursor. Não é que este projecto tenha escolhido pior — é que uma
API de plataforma nasce com essa camada e uma API sobre protocolo não.

**A comparação capacidade a capacidade está por fazer**, e está dita como tal
no ficheiro de referência. O que já se sabe: cerca de **60 das 141 rotas** —
grupos, comunidades, canais e status — não têm equivalente na Cloud API, logo
uma integração coexistiria com elas em vez de as substituir.

## Duas coisas que este scorecard NÃO afirma

- **Que a API funciona.** Afirma que 122 operações tiveram efeito confirmado,
  8 responderam sem observador, 4 falham e 7 não foram exercitadas. A
  diferença entre isto e "funciona" é o assunto inteiro da coluna de evidência.

  As 24 promoções desta ronda vieram de **sessões descartáveis** — criadas por
  `POST /admin/users`, nunca emparelhadas, medidas num servidor isolado e
  apagadas no fim —, cada uma confirmada por observador independente (SQLite,
  rota irmã de leitura, ou quadro de WebSocket). O registo rota a rota está em
  `CAMPANHA-DESCARTAVEL.md`. A quarta ❌ é `POST /session/logout` (F275).

  **Três das quatro ❌ têm hoje causa determinada**, e é `PROTOCOL_CHANGED` nas
  três: `INVESTIGATION-block-unblock.md` e
  `INVESTIGATION-newsletter-updates.md`. Continuam ❌ — a marca só se move
  quando a rota responder.

  Estes números subiram para 178/13/6/35 durante algumas horas, quando as
  formas antigas e canónicas estavam ambas documentadas. Voltaram ao que eram
  quando as antigas saíram do contrato — e o episódio vale como aviso: **um
  total que cresce sem mais verificação não é progresso**. Era a mesma prova,
  contada duas vezes.
- **Que os gates cobrem tudo.** Eles leem estrutura, não prosa
  (ARMADILHAS #29), e verificam o repositório, não o binário em execução
  (ARMADILHAS #27). As duas limitações estão escritas nos próprios gates.

  A segunda foi **verificada à mão** a 2026-08-26, e é a primeira vez nesta
  campanha: com o binário deste worktree a correr na porta 8093,
  `curl -s localhost:8093/docs/openapi.yaml | cmp - pkg/presentation/http/apidocs/openapi.yaml`
  saiu com `exit 0` — **idênticos byte a byte**, 963 862 bytes. O binário serve
  **122 caminhos**, **141 operações**, **0 `deprecated`**.
