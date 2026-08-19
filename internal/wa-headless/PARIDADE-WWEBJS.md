# Paridade com o `whatsapp-web.js` — a definição executável de "equivalente"

O `HANDOFF-INICIATIVA.md` §1 diz que o `wa-headless` precisa ser
**funcionalmente equivalente ao `whatsapp-web.js`**. Sem esta matriz, isso é a
superfície inteira da biblioteca, que é escopo infinito. Com ela, é uma lista.

Tudo aqui vem de **leitura do código** do `disparazaap`
(`features/macbook-lucas`, árvore limpa, somente leitura), não de memória. Cada
afirmação tem caminho de arquivo.

Fonte primária: `services/wa-worker/src/infrastructure/wa/adapter.ts`.

---

## 1. O achado que muda o plano: a fenda não é o adapter TypeScript

O `wa-api` **já é um `AdapterKind` de primeira classe** e já tem implementação
própria em `services/wa-worker/src/infrastructure/wa/wa-api-adapter.ts`, ligada
em `services/wa-worker/src/index.ts:279` sob a flag `WA_WA_API_ENABLED`
(ADR-0033, fatia 0007).

Esse adapter **não fala com um browser**. Ele fala com o serviço `wa-api` por
HTTP e WebSocket — `GET /session/ws`, `GET /session/status`,
`GET /session/profile`, `GET /user/contacts`, `POST /user/contacts/sync` — e
hoje quem serve essas rotas, deste lado, é o `internal/wa-noise` (whatsmeow).
Confirmado neste repositório em `pkg/presentation/http/handlers/handler_session.go:47`,
`handler_session_ws.go:21`, `handler_session.go:281` e
`pkg/presentation/http/profile_handler.go:42`.

**Consequência direta**: o `wa-headless` não substitui o `wa-api-adapter.ts`.
Ele vira um **segundo motor atrás da mesma superfície HTTP** que o `wa-noise`
já serve. O arquivo TypeScript não muda — a incerteza nº 1 do handoff (§7)
fecha na **leitura B**, e fecha por evidência, não por preferência.

O que isso reorganiza:

- **CAP-09** deixa de ser "construir uma fachada nova" e passa a ser "servir
  rotas que já existem, com o motor novo por trás". O contrato já está escrito
  e já tem consumidor.
- A pergunta "equivalente ao `wwebjs`" vira duas perguntas separáveis:
  **(a)** o que o `wa-api` já entrega hoje pelo `wa-noise`, e **(b)** o que só o
  `wwebjs` entrega, que é onde o `wa-headless` tem de existir.

---

## 2. A matriz

14 métodos: **6 obrigatórios**, **8 opcionais**. Todos os 14 têm call site real
no `services/wa-worker/src/application/runner.ts` — nenhum é declarado e não
usado.

| # | capability | wwebjs | usado pelo produto (call site) | `wa-api` hoje | precisa do `wa-headless`? |
|---|---|:---:|---|:---:|---|
| 1 | `start` | sim `:454` | **obrigatório** · `runner.ts:1204` | sim `:98` | **não** — já servido |
| 2 | `stop` | sim `:848` | **obrigatório** · `runner.ts:1284` | sim `:131` | **não** — já servido |
| 3 | `on` (WaEvent) | sim `:396` | **obrigatório** · `runner.ts:1184` | sim `:142` | **não** — já servido |
| 4 | `onContact` | sim `:400` | **obrigatório** · `runner.ts:1197` | sim `:146` | **não** — já servido |
| 5 | `listContacts` | sim `:908` | **obrigatório** · `runner.ts:2185` | sim `:150` | **não** — já servido |
| 6 | `sendText` | sim `:899` | **obrigatório** · `runner.ts:2891` | sim `:200` | **não** — já servido |
| 7 | `fetchContactAvatar` | sim `:1122` | opcional · `runner.ts:2684`, `:2390` | sim `:208` | **não** — já servido |
| 8 | `primeContactRoster` | **NÃO** | opcional · `runner.ts:2539`, `:2559` | sim `:188` | **não** — o `wwebjs` é que não tem |
| 9 | `fetchMessages` | sim `:872` | opcional · `runner.ts:2806` | **não** | **SIM** |
| 10 | `onMessageMeta` | sim `:404` | opcional · `runner.ts:1199` | **não** | **SIM** |
| 11 | `livenessCheck` | sim `:818` | opcional · `runner.ts:2134` | **não** | **SIM** |
| 12 | `refreshOwner` | sim `:779` | opcional · `runner.ts:2087`, `:2104` | **não** | **SIM** |
| 13 | `getBrowserPid` | sim `:760` | opcional · `runner.ts:1234` | **não** | **SIM** (é browser) |
| 14 | `backupNow` | sim `:835` | opcional · `runner.ts:728` | **não** | **SIM** (perfil em disco) |

Colunas `wwebjs` e `wa-api` referem-se a
`infrastructure/wa/whatsapp-web-js-adapter.ts` e
`infrastructure/wa/wa-api-adapter.ts`, com o número da linha da definição.

O runner protege cada opcional com uma guarda (`if (!adapter.X) return`,
`runner.ts:1230`, `:2129`, `:2680`, `:2791`, `:717`), então a ausência degrada
em silêncio em vez de quebrar — o que também significa que **uma capacidade
faltando não aparece como erro**, só como funcionalidade que sumiu.

---

## 3. O escopo real do `wa-headless`

**Seis capacidades**, não catorze. São exatamente as linhas 9–14: a diferença
entre o que o `wa-noise` já entrega pelo protocolo e o que hoje só existe
dirigindo o SPA.

| ordem | capacidade | por quê primeiro |
|---|---|---|
| 1 | `livenessCheck` | é a invariante 11 do handoff, e o achado do H1 da fase 6 (renderer que não responde com tudo saudável por fora) é literalmente esta capacidade. Nada acima dela é confiável sem ela. |
| 2 | `getBrowserPid` | trivial dado o CAP-02, e o `ChromiumSupervisor` do produto depende dele para registrar o processo |
| 3 | `refreshOwner` | primeira leitura do SPA; exercita o inventário de módulos (CAP-06) num caminho de baixo risco |
| 4 | `onMessageMeta` | metadata-only, invariante **12**; realtime, sem envio |
| 5 | `fetchMessages` | metadata-only; mesma superfície de leitura da anterior |
| 6 | `backupNow` | durabilidade do perfil; depende do lifecycle da CAP-05 estar fechado |

`sendText` **permanece no mapa** (CAP-07 do handoff) apesar de a linha 6 dizer
"já servido": o `wa-api` envia hoje pelo `wa-noise`, e um envio pelo
`wa-headless` só passa a ser necessário quando uma conta rodar no motor de
browser. Ele deixa de ser a primeira capacidade de produto e passa a ser
consequência do CAP-09 — mas o fluxo Resolve → Validate → Act → Verify
continua obrigatório quando chegar, porque a alternativa é sucesso silencioso.

---

## 4. O que fica fora da fatia atual do `WaClientAdapter`

> **CORREÇÃO (2026-08-18, LOOP 04.4).** O título desta seção era "O que NÃO
> implementar", redação que lê como exclusão GLOBAL de escopo do
> `wa-headless`. Isso é falso: o que a matriz prova é **apenas** que estas
> quatro coisas estão `OUT_OF_CURRENT_WA_WORKER_ADAPTER_SLICE` — fora do
> contrato `WaClientAdapter` que o `wa-worker` consome HOJE
> (`adapter.ts`, seção 2 acima). O `internal/wa-headless` é um SDK interno
> com roadmap mais amplo do que essa fatia; paridade de integração ATUAL
> **não é** o mesmo que escopo de capacidade GLOBAL do `wa-headless`. Nenhuma
> destas quatro capacidades foi aberta para implementação por esta correção —
> é mudança de redação e escopo, não de trabalho.

Por evidência desta matriz, contra o contrato atual:

- **grupos, broadcast, status/stories — `NOT_REQUIRED_BY_CURRENT_PRODUCT_CONTRACT`**:
  a interface filtra os três em todo método que os menciona (`adapter.ts:97`,
  `:117`, `:128`) — o produto é 1:1 nesta fatia.
- **mídia no envio — `OUT_OF_CURRENT_WA_WORKER_ADAPTER_SLICE`**:
  `sendText(waJid, body)` é a única superfície de envio declarada no contrato
  atual. Não existe `sendMedia`, `sendImage`, `sendDocument`.
- **corpo de mensagem — `NOT_REQUIRED_BY_CURRENT_PRODUCT_CONTRACT`**:
  `WaMessageMeta` é **metadata-only por invariante** (`adapter.ts:57`, `:116`,
  `:136`; invariante **12** do handoff) nesta fatia. O texto live é outra via
  (core-NATS `live.>`).
- **`primeContactRoster` no motor de browser — `OUT_OF_CURRENT_WA_WORKER_ADAPTER_SLICE`**:
  o próprio `adapter.ts:188` registra que o `wwebjs` não tem equivalente.
  Implementá-lo superaria o `wwebjs`, e o handoff §1 exclui isso do escopo
  ATUAL — não é exclusão permanente do `wa-headless` como SDK.

---

## 5. O que esta matriz NÃO estabelece

- **Como o `wa-api` decide qual motor usar por conta.** Hoje o `AdapterKind` é
  resolvido no `wa-worker` (`index.ts:265`); o `wa-headless` seria uma escolha
  de motor DENTRO do `wa-api`, e essa chave não existe. É desenho do CAP-09.
- **Se as seis capacidades restantes são atingíveis pelo SPA** com o inventário
  de módulos do CAP-06. Nenhuma foi exercitada contra alvo real ainda.
- **O custo.** Nenhuma medição de memória ou tempo por capacidade.

---

_Levantado em 2026-08-11 (CAP-01), contra `disparazaap` em
`features/macbook-lucas`. Se o `adapter.ts` mudar, esta matriz vence — refaça a
busca antes de confiar nela._

---

## 6. Divergências conscientes do `whatsapp-web.js`

O `CLAUDE.md` exige que divergir seja **decisão registrada**, não acidente.
Esta seção existe para isso. Divergir é aceitável; divergir sem saber é como
este projeto reescreve os bugs deles junto.

### 6.1 — `refreshOwner` mantém PN e LID SEPARADOS

**O que o `wwebjs` faz** (`Client.js:351-364`, 1.34.7):

```js
wid: window.require('WAWebUserPrefsMeUser').getMaybeMePnUser()
  || window.require('WAWebUserPrefsMeUser').getMaybeMeLidUser()
```

Um campo só. O `||` escolhe o primeiro que existir e **descarta qual dos dois
respondeu**.

**O que nós fazemos**: `owner.Identity` carrega `PN` e `LID` em campos
distintos, e `Present()` aceita qualquer um dos dois — a mesma condição que o
`wwebjs` trata como "existe wid".

**Por quê**: LID e PN são espaços de nomes diferentes para a mesma pessoa, e
**este produto tem histórico documentado de confundi-los**. Para uma biblioteca
cujos chamadores só exibem o valor, colapsar é barato. Aqui não é: um chamador
que segura o resultado precisa saber se tem identidade de telefone ou LID,
porque os dois não são intercambiáveis ao falar com o servidor.

**O que a divergência assume, e onde isso é verificado**: que os dois chegam
juntos. A M3 mediu assim, e `TestRealSPARefreshOwnerAgainstProduction` falha com
mensagem própria se um dia só um materializar — dizendo que `Present()` continua
correto mas que **esta decisão precisa ser revisitada**, em vez de só quebrar.

### 6.2 — `DisplayName` existe na superfície e **nunca foi visto preenchido**

Medido em 2026-08-19 contra o perfil pareado real:

| getter | resultado |
|---|---|
| `WAWebUserPrefsMeUser.getMaybeMeDisplayName` | **`NULL`** (a função existe) |
| `WAWebUserPrefsMeUser.getMaybeMePnUser` | `object` |
| `WAWebUserPrefsInfoStore.getPushname` | **`GETTER_ABSENT`** |
| `WAWebUserPrefsInfoStore.getMe` | **`GETTER_ABSENT`** |

As duas últimas linhas eram **palpites meus**, e a medição as descartou — que é
o motivo de a regra "medir antes de projetar" existir.

O campo fica na struct porque é onde ele moraria, e o comentário diz que é
**UNVERIFIED**: ninguém deve tratar um `DisplayName` preenchido como garantido,
e um vazio não é evidência de nada sobre a sessão. Se é nulo porque a conta não
tem pushname ou porque este build o guarda noutro lugar continua **UNKNOWN** —
e `TestRealSPADisplayNameShape` é o instrumento que distingue os casos no dia
em que mudar.

### 6.3 — Correção de referência (2026-08-19)

As duas citações de *"invariante 13"* neste documento apontavam para a
invariante errada. No `HANDOFF-INICIATIVA.md` §6 a lista diz:

- **12** — `WaMessageMeta` é metadata-only; zero PII em log
- **13** — reclaim de `Singleton` no boot é obrigatório em contêiner

Metadata-only é a **12**. Corrigido nos dois lugares. Achado ao ir implementar
`onMessageMeta` e conferir contra qual invariante ela seria validada — conferir
é o que fez a discrepância aparecer, e ela estava no documento desde antes
desta sessão.

### 6.4 — `msg.id._serialized` é **NULL** neste build; o `wwebjs` depende dele

Medido em 2026-08-19 contra o perfil pareado real, antes de `onMessageMeta` ser
implementada. O store é `WAWebMsgCollection.MsgCollection` (`on`/`off`/
`getModelsArray`, 384 modelos); `WAWebMsgStore` — palpite meu — **não existe**.

Forma de um modelo de mensagem, só nomes e tipos:

| campo | resultado |
|---|---|
| `id` | objeto, chaves `$1 \| fromMe \| id \| remote` |
| **`id._serialized`** | **`NULL`** |
| `id.id` | `string` |
| `id.remote` | objeto WID (`_serialized`, `server`, `user`) |
| `id.fromMe` | `boolean` |
| `from` / `to` | objeto WID |
| `t` | `number` |
| `type` | `string` |
| `ack` | `number` |
| `author`, `isGroupMsg`, `isNewMsg`, `chat` | `NULL` (conversa 1:1) |

**A divergência**: o `whatsapp-web.js` usa `msg.id._serialized` como
identificador da mensagem. Neste build esse acessor responde **nulo**. Copiar o
CÓDIGO deles produziria um `waMessageId` vazio em toda mensagem — e vazio é
indistinguível de "sem id", que é a classe de defeito que este módulo passou a
semana caçando.

Copiar o ENTENDIMENTO produz outra coisa: o identificador é composto por
`fromMe`, `remote` e `id`, e é isso que o `_serialized` deles concatena.

**Decisão**: expor as PARTES (`ID`, `FromMe`, `Remote`) em vez de sintetizar uma
string. Mesmo raciocínio do §6.1: não perder informação por colapso. Sintetizar
um identificador à mão é como uma stack começa a discordar do servidor sobre
qual mensagem é qual — e se um dia o `_serialized` voltar, ele passa a ser mais
uma fonte a conferir, não a única.

**Como isto foi encontrado**: `Object.keys` no modelo NÃO lista `_serialized`,
e também não listaria um getter de protótipo que funcionasse — foi a lição já
registrada para `WAWebConnModel.Conn.wid`. Perguntar ao acessor **diretamente**,
em vez de confiar na lista de chaves, é o que separou "campo ausente da lista" de
"campo que responde nulo".

### 6.5 — `fetchMessages` lê o que está CARREGADO, não o histórico

O `whatsapp-web.js` expõe `chat.fetchMessages({limit})`, que pode **pedir mais
ao servidor**. A nossa implementação lê `WAWebMsgCollection.getModelsArray()`,
que é o que a SPA já carregou — **340 modelos** medidos contra uma lista de 917
conversas.

Um chamador que pede 100 e recebe 12 está sendo informado do que está
**carregado**, não do que **existe**.

A lacuna é declarada em vez de escondida porque a alternativa — devolver uma
lista curta como se fosse o histórico — é a incompletude silenciosa que este
módulo vem encontrando em todas as formas. Por isso o `Result` carrega
`Loaded` (o denominador) e `Matched`, e `Truncated()` diz quando o limite cortou:
12 de 340 carregados é uma conversa de pouco tráfego; 12 de 12 é uma página que
mal carregou.

Disparar o carregamento sob demanda é fatia futura, e vai precisar de medição
própria: não se sabe qual chamada a SPA usa para isso neste build.

