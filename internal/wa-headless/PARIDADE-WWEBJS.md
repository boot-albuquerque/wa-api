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
| 4 | `onMessageMeta` | metadata-only, invariante 13; realtime, sem envio |
| 5 | `fetchMessages` | metadata-only; mesma superfície de leitura da anterior |
| 6 | `backupNow` | durabilidade do perfil; depende do lifecycle da CAP-05 estar fechado |

`sendText` **permanece no mapa** (CAP-07 do handoff) apesar de a linha 6 dizer
"já servido": o `wa-api` envia hoje pelo `wa-noise`, e um envio pelo
`wa-headless` só passa a ser necessário quando uma conta rodar no motor de
browser. Ele deixa de ser a primeira capacidade de produto e passa a ser
consequência do CAP-09 — mas o fluxo Resolve → Validate → Act → Verify
continua obrigatório quando chegar, porque a alternativa é sucesso silencioso.

---

## 4. O que NÃO implementar

Por evidência desta matriz:

- **nada de grupos, broadcast, status/stories**: a interface filtra os três em
  todo método que os menciona (`adapter.ts:97`, `:117`, `:128`) — o produto é
  1:1.
- **nada de mídia no envio**: `sendText(waJid, body)` é a única superfície de
  envio declarada. Não existe `sendMedia`, `sendImage`, `sendDocument`.
- **nada de corpo de mensagem**: `WaMessageMeta` é **metadata-only por
  invariante** (`adapter.ts:57`, `:116`, `:136`; invariante 13 do handoff). O texto live é
  outra via (core-NATS `live.>`).
- **nada de `primeContactRoster` no motor de browser**: o próprio
  `adapter.ts:188` registra que o `wwebjs` não tem equivalente. Implementá-lo
  seria superar o `wwebjs`, e o handoff §1 exclui isso do escopo.

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
