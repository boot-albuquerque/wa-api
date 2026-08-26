# Campanha da sessão descartável — 2026-08-26

## A hipótese, e o que ela custou

Dos 32 ⬜ do relatório de evidências, 25 tinham como motivo alguma variação de
**"mexeria na sessão real em uso"**:

> `escreveria a chave HMAC da sessao em uso` · `criaria uma sessao real no
> servidor em uso` · `apagaria uma sessao real em uso` · `escreveria a
> configuracao de webhook da sessao em uso` · `derrubaria a sessao real em uso
> nesta sessao de trabalho` · `mudaria a rota de saida da sessao em uso`

Esse motivo **não é "não dá para testar"**. É **falta de fixture descartável**.
Uma sessão criada por `POST /admin/users`, nunca emparelhada com telemóvel
nenhum, é descartável por construção: escrevê-la, reconfigurá-la, derrubá-la e
apagá-la não custa nada a ninguém.

Resultado: **25 rotas saíram de ⬜** — 24 para ✅, 1 para ❌. Restam 7 ⬜, e
nenhuma delas com o motivo antigo.

| | antes | depois |
|---|---:|---:|
| ✅ | 98 | **122** |
| 🟡 | 8 | 8 |
| ❌ | 3 | **4** |
| ⬜ | 32 | **7** |
| total | 141 | 141 |

## O ambiente da medição

Servidor **isolado**, deliberadamente **não** o `run.sh` — ele mata o processo
da porta 8080 e apaga a base, e podia colidir com outro trabalho a decorrer.

```
go build -o /tmp/wa-api-ev ./cmd/core          # a partir deste worktree

WA_API_ADMIN_TOKEN=evadmin123 /tmp/wa-api-ev \
  -port 8091 \
  -datadir <scratchpad>/evdata \
  -admintoken evadmin123 \
  -globalencryptionkey 'MinhaChaveDe32Caracteres12345678' \
  -globalhmackey 'MinhaHMACKeyDe32Caracteres123456' \
  --logtype=json --color=false > <scratchpad>/wa-api-ev.log 2>&1 &
```

Porta 8091, base SQLite própria em `<scratchpad>/evdata/dbdata/`, log próprio.
Quatro sessões descartáveis (`descartavel-1` a `descartavel-4`), todas criadas
e apagadas dentro da campanha. **Nenhuma sessão real foi tocada.**

Observadores usados, por ordem de força:

1. **SQLite directo** — `sqlite3 evdata/dbdata/users.db "select ... from users"`;
2. **segunda rota** — a leitura irmã da escrita (`GET /webhook` para
   `POST /webhook`, `GET /session/status` para `/session/connect`);
3. **quadro de WebSocket** — para `/session/ws`;
4. **log do servidor** — usado para diagnosticar, **nunca** sozinho para promover.

## Rota a rota

### Administração

| rota | antes → depois | pedido | resposta | observador | resultado |
|---|---|---|---|---|---|
| `POST /admin/users` | ⬜ → ✅ | `POST /admin/users {"name":"descartavel-1","token":"tok-desc-1"}` | `200 {"id":"75b9d2a9…","name":"descartavel-1","token":"tok-desc-1", …}` | `GET /admin/users` antes: `data: null`; depois: array com `75b9d2a9…` | promovida |
| `PUT /admin/users/{id}` | ⬜ → ✅ | `PUT /admin/users/75b9d2a9… {"name":"descartavel-1-renomeado","events":"Message,ReadReceipt"}` | `200 {"status":"ok"}` | `GET /admin/users/{id}` devolve `name: descartavel-1-renomeado` e `events: Message,ReadReceipt` | promovida |
| `DELETE /admin/users/{id}` | ⬜ → ✅ | `DELETE /admin/users/80eeb48b…` | `200 {"status":"deleted"}` | **SQLite**: `select count(*) … where id='80eeb48b…'` → `0` | promovida |
| `DELETE /admin/users/{id}/full` | ⬜ → ✅ | `DELETE /admin/users/e9c0201e…/full` | `200 {"id":"e9c0201e…","name":"descartavel-4","jid":""}` | `GET /admin/users` deixa de listar o id, **e SQLite** `count(*)` → `0` | promovida |

Nota medida ao validar o observador do `DELETE`: **o token da sessão apagada
continua a autenticar** — `GET /webhook` com `tok-desc-2` respondeu `200`
depois da remoção, enquanto um token inexistente responde `401`. Achado
**F273**.

### Webhook e histórico

| rota | antes → depois | pedido | resposta | observador | resultado |
|---|---|---|---|---|---|
| `POST /webhook` | ⬜ → ✅ | `{"webhookurl":"http://127.0.0.1:8092/hook","events":["Message","ReadReceipt"]}` | `200 {"webhook":"http://127.0.0.1:8092/hook"}` | `GET /webhook` → `webhook` gravado, `subscribe: ["Message","ReadReceipt"]` | promovida |
| `PUT /webhook` | ⬜ → ✅ | `{"webhook":"http://127.0.0.1:8092/hook2","events":["Message"],"active":true}` | `200 {"active":true,"events":["Message"],"webhook":"…/hook2"}` | `GET /webhook` → `webhook: …/hook2`, `subscribe: ["Message"]` (o `ReadReceipt` anterior desapareceu — a substituição é observável) | promovida |
| `DELETE /webhook` | ⬜ → ✅ | `DELETE /webhook` | `200 {"Details":"Webhook and events deleted successfully"}` | `GET /webhook` → `webhook: ""`, `subscribe: [""]` (a lista com uma string vazia que a documentação avisa) | promovida |
| `POST /webhook/history` | ⬜ → ✅ | `{"history":77}` | `200 {"Details":"History configured successfully","History":77}` | `GET /webhook/history` → `{"History":77}`, contra baseline `{"History":0}` | promovida |
| `POST /session/history` | ⬜ → ✅ | `{"history":33}` | `200 {"Details":"History configured successfully","History":33}` | `GET /webhook/history` → `{"History":33}` — confirma também que é o MESMO manipulador, como a documentação diz | promovida |

**Pré-requisito descoberto na medição**: as duas rotas de histórico exigem
cliente vivo (`EnsureSession`). Sem `GET /session/connect` antes, respondem
`400 no_session`. Foi por isso que a ordem da bateria passou a ser
*criar → ligar → configurar*.

### S3

| rota | antes → depois | pedido | resposta | observador | resultado |
|---|---|---|---|---|---|
| `POST /s3/config` | ⬜ → ✅ | corpo completo com `endpoint: https://s3.us-east-1.amazonaws.com`, `bucket: descartavel`, `retention_days: 7`, `media_delivery: both` | `200 {"Details":"S3 configuration saved successfully","Enabled":true}` | `GET /s3/config` devolve campo a campo o que foi enviado, com `access_key: "***"` | promovida |
| `DELETE /s3/config` | ⬜ → ✅ | `DELETE /s3/config` | `200 {"Details":"S3 configuration deleted successfully"}` | `GET /s3/config` → `enabled: false`, `endpoint: ""`, `bucket: ""` | promovida |
| `POST /s3/configure` | ⬜ → ✅ | corpo distinto (`eu-west-1`, `descartavel-configure`, `retention_days: 11`, `media_delivery: s3`) | `200`, idem | `GET /s3/config` devolve o corpo NOVO — prova que é o mesmo manipulador e que escreve mesmo | promovida |
| `POST /session/s3/config` | ⬜ → ✅ | corpo distinto (`ap-south-1`, `descartavel-sessao`, `retention_days: 5`) | `200`, idem | `GET /session/s3/config` devolve esse corpo | promovida |
| `DELETE /session/s3/config` | ⬜ → ✅ | `DELETE /session/s3/config` | `200 {"Details":"S3 configuration deleted successfully"}` | `GET /session/s3/config` → `enabled: false`, `bucket: ""` | promovida |
| `POST /s3/test` | ⬜ → **⬜** | ver abaixo | `500` | — | **não promovida** |
| `POST /session/s3/test` | ⬜ → **⬜** | idem | `500` | — | **não promovida** |

Corpos diferentes em cada rota irmã de propósito: se todas escrevessem o mesmo,
um `GET` a devolver o valor certo não distinguiria "esta rota escreveu" de
"a anterior já lá tinha deixado".

**Por que `/s3/test` continua ⬜, e o motivo novo.** O motivo antigo — *"faria
round-trip real contra um bucket externo"* — foi substituído por um motivo
medido:

- o `docker` **está** disponível nesta máquina (27.3.1), logo um MinIO local
  seria trivial de levantar;
- **mas a configuração não aceita apontar para ele**: `POST /s3/config` valida
  o `endpoint` com `egress.ValidateOutboundURL`, que recusa loopback e faixas
  reservadas. Medido:
  `{"code":400,"error":{"code":"invalid_s3_endpoint","message":"invalid S3 endpoint"}}`
  para `http://127.0.0.1:9000`. Achado **F277**;
- não há credenciais AWS descartáveis neste ambiente.

O percurso **foi** exercitado até ao fim, com endpoint público e credenciais
deliberadamente falsas: a rota chegou à AWS e recebeu
`403 InvalidAccessKeyId`, que o servidor devolveu como `500` com `error` em
string (achado **F276**). Isso **não** vale ❌: a falha foi provocada pelo meu
input, não pelo percurso. Marcar ❌ diria ao leitor que a rota está partida,
e não está — está sem alvo.

### HMAC

| rota | antes → depois | pedido | resposta | observador | resultado |
|---|---|---|---|---|---|
| `POST /hmac/config` | ⬜ → ✅ | `{"hmac_key":"chave-descartavel-de-32-caracter"}` | `200 {"Details":"HMAC configuration saved successfully","Enabled":true}` | `GET /hmac/config` passa de `{"hmac_key":""}` a `{"hmac_key":"***"}` | promovida |
| `DELETE /hmac/config` | ⬜ → ✅ | `DELETE /hmac/config` | `200 {"Details":"HMAC configuration deleted successfully"}` | `GET /hmac/config` volta a `{"hmac_key":""}` | promovida |
| `POST /hmac/configure` | ⬜ → ✅ | `{"hmac_key":"outra-chave-descartavel-32-carac"}` | `200`, idem | `GET /hmac/config` → `"***"` a partir de `""` (o `DELETE` anterior tinha limpado — a transição é observável, não herdada) | promovida |
| `POST /session/hmac/config` | ⬜ → ✅ | `{"hmac_key":"terceira-chave-descartavel-32-ca"}` | `200`, idem | `GET /session/hmac/config` → `"***"` a partir de `""` | promovida |
| `DELETE /session/hmac/config` | ⬜ → ✅ | `DELETE /session/hmac/config` | `200 {"Details":"HMAC configuration deleted successfully"}` | `GET /session/hmac/config` → `{"hmac_key":""}` | promovida |

A chave nunca é devolvida — o observador é a **presença** (`"***"` contra
`""`), que é o que a rota de leitura promete e tudo o que ela dá.

### Proxy

| rota | antes → depois | pedido | resposta | observador | resultado |
|---|---|---|---|---|---|
| `POST /session/proxy` | ⬜ → ✅ | `{"enable":true,"proxy_url":"socks5://proxy.exemplo.invalid:1080","webhook_use_proxy":false}` | `200 {"Details":"Proxy configured successfully","Set":true,"ProxyURL":"socks5://…","webhook_use_proxy":false}` | **dois**: `GET /session/status` → `proxy_url: socks5://proxy.exemplo.invalid:1080`; e `GET /admin/users/{id}` → `proxy_config: {enabled: true, proxyUrl: …, webhookUseProxy: false}` | promovida |
| `POST /proxy/set` | ⬜ → ✅ | `{"enable":true,"proxy_url":"http://outro.exemplo.invalid:3128","webhook_use_proxy":false}` | `200`, idem com o URL novo | `GET /session/status` → `proxy_url: http://outro.exemplo.invalid:3128` (substituiu o `socks5` anterior) | promovida |

**Pré-requisito**: as duas exigem a sessão **desligada** — com transporte vivo
respondem `400 proxy_while_connected`. Daí a ordem
*ligar → medir connect → desligar → medir disconnect → medir proxy*.

Limpeza: `{"enable":false}` → `200 {"Details":"Proxy disabled successfully"}`,
confirmada por `GET /session/status` a devolver `proxy_url: ""` e
`proxy_config.enabled: false`.

### Ciclo de vida da sessão

| rota | antes → depois | pedido | resposta | observador | resultado |
|---|---|---|---|---|---|
| `GET /session/connect` | ⬜ → ✅ | `GET /session/connect` com `tok-desc-1` | `200 {"status":"connecting"}` | `GET /session/status` passa de `connected: false, qrcode: ""` a `connected: true` com `qrcode` de 1842 caracteres — **a sessão ligou-se mesmo ao WhatsApp e recebeu um QR real** | promovida |
| `GET /session/disconnect` | ⬜ → ✅ | `GET /session/disconnect` | `200 {"details":""}` | `GET /session/status` volta a `connected: false` (com `loggedIn: false` inalterado, como a documentação promete) | promovida |
| `GET /session/ws` | ⬜ → ✅ | *upgrade* WebSocket com `token: tok-desc-4` (cliente RFC 6455 escrito para o efeito, em Python de biblioteca padrão) | `101 Switching Protocols`, com `Sec-Websocket-Accept: jpjeKWkdIfhfzzkNbFelmoEr8R0=` | **três quadros de texto reais**, `type: QR`, com `code`, `qrCodeBase64` e `expiresAt` na raiz e **sem envelope** `{code,data,success}` — exactamente a forma documentada; e `GET /session/qr` a servir QR da mesma sessão em paralelo | promovida |
| `POST /session/logout` | ⬜ → **❌** | `POST /session/logout` numa sessão LIGADA e nunca emparelhada | `500 {"code":500,"error":"internal server error","success":false}` — no log, `the store doesn't contain a device JID` | `GET /session/status` inalterado (`connected: true`) | **falhou** |

**O motivo antigo de `/session/ws` era falso, e vale dizê-lo**: *"fora do
alcance de curl e do Try it out"* é verdade sobre o `curl` e sobre o Swagger
UI, e irrelevante sobre a rota. WebSocket testa-se com um cliente WebSocket.
Bastaram 60 linhas de `socket` + `base64` + `struct`.

**O que `/session/logout` faz numa sessão nunca emparelhada, com precisão** —
a tarefa pediu que um comportamento degenerado fosse dito e não escondido:

- sessão **sem** transporte vivo → `409 session_not_connected`, com
  `message: session has no live connection; call /session/connect before
  logging out`. **Bate exactamente com a documentação**, incluindo o `Detach`
  forçado do estado local (F93), visível no log como `Received kill signal`;
- sessão **com** transporte vivo mas sem aparelho emparelhado → `500` com
  envelope de texto simples. É o achado **F275**, e é a razão do ❌;
- o caminho de `200` exige uma conta emparelhada — está fora deste lote.

## Achados registados no HOUSEKEEP

| # | o quê | onde apareceu |
|---|---|---|
| **F273** | token de sessão apagada continua a autenticar (`200` em `GET /webhook` depois do `DELETE`; `401` para token inexistente) | ao validar o observador do `DELETE /admin/users/{id}` |
| **F274** | `connect → disconnect → connect` devolve `200` e não religa (`start already in flight`) — classe da F108 | ao medir `/session/connect` e `/session/disconnect` |
| **F275** | `POST /session/logout` sem aparelho emparelhado → `500` com `error` em string | ao medir `/session/logout` |
| **F276** | `POST /s3/test` transforma `403 InvalidAccessKeyId` em `500` com `error` em string | ao tentar promover `/s3/test` |
| **F277** | documentação do `endpoint` do S3 diz "URL analisável"; a regra real recusa loopback e faixas reservadas | ao tentar levantar MinIO local |

Nenhum foi corrigido: são todos fora do escopo da tarefa, e três deles
(F273, F274, F275) tocam em fronteiras — autenticação, ciclo de vida da sessão,
taxonomia de erro — que a política anti-regressão do `CLAUDE.md` exige que se
conserte com teste do defeito **e** controlo negativo executado.

## Ciclo de vida do fixture, com a confirmação de limpeza

Quatro sessões criadas, quatro apagadas:

| sessão | id | destino |
|---|---|---|
| `descartavel-1` (→ `descartavel-1-renomeado`) | `75b9d2a96cb3483721330dddf0a0a0f1` | `DELETE /admin/users/{id}/full` |
| `descartavel-2` | `80eeb48bdff35433626c7268551288d1` | `DELETE /admin/users/{id}` (era o próprio teste) |
| `descartavel-3` | `0970ba6c8bee9efe377178dd2c58a0ee` | `DELETE /admin/users/{id}/full` |
| `descartavel-4` | `e9c0201e926c3fd2074b351cfdd27f1b` | `DELETE /admin/users/{id}/full` (era o próprio teste) |

Confirmação, com os **dois** observadores:

```
$ curl -s -H 'Authorization: evadmin123' localhost:8091/admin/users
{"code":200,"data":null,"success":true}

$ sqlite3 <scratchpad>/evdata/dbdata/users.db "select count(*) from users;"
0
```

Processo terminado no fim e morte confirmada com `lsof -ti:8091` sem saída.

## O que ficou por fazer, e porquê

Sete ⬜ restantes. **Nenhuma** com o motivo "mexeria na sessão em uso":

| rota | motivo |
|---|---|
| `POST /s3/test` | nenhum bucket descartável disponível: o validador de saída recusa endpoint em loopback (F277), e não há credenciais AWS descartáveis |
| `POST /session/s3/test` | idem — mesmo manipulador |
| `POST /call/reject` | exige uma chamada a entrar; não há como provocar uma sem par |
| `POST /session/pairphone` | fora deste lote por enunciado — exige um número real |
| `POST /users/avatar` | fora deste lote — exige conta emparelhada |
| `POST /users/privacy` | fora deste lote — exige conta emparelhada |
| `POST /users/status` | fora deste lote — exige conta emparelhada |

As quatro últimas foram **explicitamente excluídas do escopo** desta tarefa e
seguem para outro lote; ficam com o motivo que já tinham.
