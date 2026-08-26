# Medição das quatro lacunas de produção

**Data**: 2026-08-26. **Objecto**: as quatro linhas ❌ da secção 6 do
`docs/PRODUCTION-READINESS.md` — paginação, idempotência, limitação de ritmo e
concorrência. Elas foram tratadas como **hipóteses**, não como factos.

O eixo por rota já estava fechado (141 operações: 122 ✅ / 8 🟡 / 4 ❌ / 7 ⬜).
Este documento é o outro eixo: lacunas arquitecturais, que nenhuma medição rota
a rota apanha.

**Nada foi corrigido.** As quatro são decisões de produto. O que saiu daqui é
medição, um teste de regressão e cinco achados no `HOUSEKEEP.md` (F290–F294).

---

## O ambiente

Servidor isolado, **não** o `run.sh` (que mata o processo da 8080 e apaga base
alheia):

```
go build -o /tmp/wa-api-prod ./cmd/core

WA_API_ADMIN_TOKEN=prodadmin123 /tmp/wa-api-prod \
  -port 8093 \
  -datadir <scratchpad>/proddata \
  -admintoken prodadmin123 \
  -globalencryptionkey 'MinhaChaveDe32Caracteres12345678' \
  -globalhmackey 'MinhaHMACKeyDe32Caracteres123456' \
  --logtype=json --color=false > <scratchpad>/wa-api-prod.log 2>&1 &
```

Porta **8093**, base SQLite própria, uma sessão descartável (`prod-1`) criada
por `POST /admin/users` e **nunca emparelhada**. Nenhuma sessão real foi tocada.
Processo morto no fim, com `lsof -ti:8093` confirmado vazio.

Duas colecções foram **semeadas** directamente no SQLite para produzir curva em
vez de ponto: 2 000 conversas em `message_history` (uma mensagem cada) e 5 001
mensagens numa única conversa. Semear é legítimo aqui porque a coisa medida é o
comportamento da rota perante volume, não a origem do volume.

---

## 0. A verificação do `CLAUDE.md`: o que `/docs` serve vs. o que está no repo

Pedida explicitamente, e nunca feita nesta campanha. `ARMADILHAS #27` avisa que
regenerar o ficheiro não muda o que o binário serve.

```
$ curl -s localhost:8093/docs/openapi.yaml -o /tmp/served-openapi.yaml -w "http=%{http_code} bytes=%{size_download}\n"
http=200 bytes=963862

$ cmp /tmp/served-openapi.yaml pkg/presentation/http/apidocs/openapi.yaml
CMP_EXIT=0
```

**Idênticos, byte a byte.** O binário compilado deste worktree serve exactamente
o ficheiro que está no repositório.

O que o binário serve, contado a partir do documento servido:

| medida | valor |
|---|---:|
| caminhos | **122** |
| operações | **141** |
| operações `deprecated` | **0** |
| operações que documentam `429` | **0** |

Códigos de estado declarados, com a contagem de operações em que aparecem:

```
101:1  200:140  400:131  401:138  403:4  404:11  409:8  422:31  500:136  501:1
```

**Achado, e é uma contradição com o scorecard**: a linha "Singular/plural
coerente" (linha 67) diz que as formas antigas "continuam a funcionar, marcadas
`deprecated`". Nenhuma operação servida está marcada `deprecated` — e
correctamente, porque as formas antigas **saíram do contrato** (linha 56): não
há nada no documento para marcar. A linha 67 foi corrigida.

---

## 1. Concorrência — a afirmação confirma-se; o mecanismo que ela dá está errado

### A afirmação do scorecard

> sem `ETag`, `If-Match` ou versão. Duas escritas simultâneas ao mesmo recurso
> perdem uma, em silêncio

### O que foi medido, e como

Leitura de código de todas as rotas de escrita local, seguida de teste Go sob
`-race` com o **repositório real** sobre SQLite real
(`db.InitializeSchema`, os mesmos pragmas da produção).

Ficheiro:
`pkg/application/usecase/storage/session_config_concurrency_test.go`.

Rotas examinadas: `PUT /admin/users/{id}`, `POST /s3/config`,
`POST /session/proxy`, `POST /webhook/history`, `PUT /webhook`.

### Números

**1.1 — Não há read-modify-write da linha em `PUT /admin/users/{id}`.**
`UserRepository.UpdateUser` (`pkg/infra/db/user_repository.go:114-191`) monta um
`UPDATE users SET <só os campos informados> WHERE id = ?`. Duas escritas
concorrentes de campos **diferentes** não se perdem. A parte da afirmação que
sugere perda genérica de escrita **cai**.

**1.2 — A cache não é o problema, e eu esperava que fosse.**
`userInfoRepublisher.RepublishUser` (`pkg/bootstrap/user_republish_adapter.go`)
**invalida** e relê do banco em vez de aplicar o payload. Duas invalidações
concorrentes convergem para o estado do banco. Hipótese testada e **descartada**.

**1.3 — A janela real é o par leitura→escrita de `resolveWebhookUseProxy`.**
Um `POST /session/proxy` que **omite** `webhook_use_proxy` LÊ a coluna
(`SELECT COALESCE(webhook_use_proxy, true) …`) e reescreve-a com o `proxy_url`
novo (`UPDATE users SET proxy_url = ?, webhook_use_proxy = ? WHERE id = ?`).

Medido, com encontro marcado que força a leitura de B a preceder a escrita de A:

```
$ go test -race -run TestSetProxy_ConcorrenciaPerdeOFlagDeclarado -v ./pkg/application/usecase/storage/
=== RUN   TestSetProxy_ConcorrenciaPerdeOFlagDeclarado
    session_config_concurrency_test.go:248: perda silenciosa confirmada:
        A respondeu webhook_use_proxy=false, o banco tem true
        (proxy_url="http://203.0.113.11:3128")
--- PASS: TestSetProxy_ConcorrenciaPerdeOFlagDeclarado (0.06s)
```

A é respondido `200` a dizer `webhook_use_proxy: false`. O banco tem `true`.
Nenhuma das duas respostas menciona a sobreposição.

**1.4 — E o número que reorienta a prioridade.** Sem encontro marcado, 200
rodadas de duas escritas verdadeiramente concorrentes:

```
    session_config_concurrency_test.go:377: perdas naturais em 200 rodadas: 0 (0.0%)
        — sem encontro marcado a janela fecha por sorte do escalonador
```

**Zero em 200.** O SQLite serializa os escritores e a leitura é rápida demais
para a janela abrir sozinha nesta máquina.

### Veredicto

A afirmação **confirma-se no efeito** — há perda silenciosa de escrita — e
**corrige-se no mecanismo**. Não é read-modify-write da linha, e um `ETag` só a
fecharia se a comparação de versão acontecesse DENTRO do mesmo `UPDATE`. O
caminho mais barato é fazer "não informado" chegar ao SQL como tal
(`webhook_use_proxy = COALESCE(?, webhook_use_proxy)`), eliminando a leitura
separada.

Registado em **F292**.

### Controlos negativos, EXECUTADOS

```
# 1) serializar A e B — tira o entrelaçamento, mantém tudo o resto:
--- FAIL: TestSetProxy_ConcorrenciaPerdeOFlagDeclarado (0.06s)
    session_config_concurrency_test.go:230: MUDANÇA DE COMPORTAMENTO:
        webhook_use_proxy=false sobreviveu à escrita concorrente de B.
        Este teste travava a PERDA medida em 2026-08-26 (F292). …

# 2) fazer B DECLARAR o flag — tira a leitura, logo tira a janela:
--- FAIL: TestSetProxy_ConcorrenciaPerdeOFlagDeclarado (0.06s)
    session_config_concurrency_test.go:240: o UPDATE de B devia ter ficado
        como o último; proxy_url="http://203.0.113.10:3128"
```

**Honestidade sobre o segundo controlo**: ele falha noutra asserção — a de ordem
—, não na da perda. Vale como prova de que o teste é sensível ao corpo de B;
**não** vale como prova de que a asserção da perda morde. Essa é a do primeiro.

### O que ficou por medir, e porquê

- **PostgreSQL.** A medição correu só sobre SQLite. Com `MVCC` e um nível de
  isolamento diferente a janela pode ser **maior**, não menor: o `SELECT` e o
  `UPDATE` estão em transacções implícitas separadas nos dois motores, mas o
  Postgres não serializa escritores como o SQLite. As 0/200 rodadas naturais são
  um número **deste** motor.
- **Um segundo mecanismo, visto e não medido**: `ConfigureS3UseCase` grava no
  banco e depois chama `storage.GetS3Manager().InitializeS3Client` — um registo
  global do processo. Duas escritas concorrentes podem deixar o banco com a
  configuração de B e o cliente vivo com a de A. Não foi medido porque exigiria
  um endpoint S3 e sai do âmbito; a forma é a mesma da F292 e a ordem documentada
  ("a ORDEM é o contrato") protege o caso sequencial e não o concorrente.

---

## 2. Paginação — a afirmação **cai**, e o erro estava na leitura da evidência

### A afirmação do scorecard

> **nenhuma colecção é paginada**. `GET /user/contacts` devolveu **61 459 bytes**
> com 1266 contactos, sem limite nem cursor

### O que foi medido, e como

Três coisas: leitura da especificação servida à procura de parâmetros de
paginação; leitura do código de todas as colecções; e curva medida contra o
servidor com 2 000 conversas e 5 001 mensagens semeadas. Sem repetição — os
valores são determinísticos (mesma base, mesma resposta), e foram reconfirmados
ao mudar o volume.

### Números

**2.1 — A especificação servida declara `limit` e `offset`.**

```
ops com param de paginacao:
  ('/chats/history','get',['chat_jid','limit'])
  ('/chats/list','get',['limit','offset'])
```

A pergunta seguinte era a que a tarefa pedia: são aceites e **silenciosamente
ignorados**? Não. São aceites e aplicados.

**2.2 — `/chats/list` tem paginação COMPLETA.** 2 000 conversas semeadas:

```
query                      bytes  chats  total  limit_eco offset_eco
(sem parametro)             5332     50   2000         50          0
?limit=10                   1132     10   2000         10          0
?limit=100                 10583    100   2000        100          0
?limit=500                 52583    500   2000        500          0
?limit=501                 52583    500   2000        500          0
?limit=2000                52583    500   2000        500          0
?limit=999999              52583    500   2000        500          0
?limit=0                    5332     50   2000         50          0
?limit=-1                   5332     50   2000         50          0
?limit=abc                  5332     50   2000         50          0
?limit=50&offset=1990       1135     10   2000         50       1990
?offset=99999                 87      0   2000         50      99999
?offset=-5                  5332     50   2000         50          0
```

Padrão de 50, **tecto de 500**, `offset`, `total` devolvido, e entrada fora de
faixa corrigida em vez de recusada. É paginação por deslocamento, não por
cursor — mas é paginação, e com tecto. E está no código **desde o commit que
criou a rota**, `9d9dd7ec`, 2026-08-08.

**2.3 — `/chats/history` tem limite e NÃO tem tecto.** 5 001 mensagens numa
conversa:

```
query                              bytes    msgs
(sem limit)                        15760      50
&limit=10                           3160      10
&limit=100                         31510     100
&limit=1000                       315010    1000
&limit=5000                      1575010    5000
&limit=999999                    1575325    5001
&limit=-1                        1575325    5001
&limit=0                              38       0
```

`limit=999999` devolve tudo. E `limit=-1` chega ao SQL como `LIMIT -1`, que em
**SQLite significa "sem limite"** — a mesma chamada em PostgreSQL seria erro de
faixa, logo `500`. As duas rotas irmãs divergem: a que pagina tem tecto, a que
guarda o objecto que mais cresce não tem.

**2.4 — O custo do que não tem tecto, medido em memória.** `RespondJSON`
materializa o corpo inteiro com `json.Marshal` antes de escrever
(`pkg/presentation/http/response.go:83`) — não há encoder em streaming:

```
conc=20  limit=50       wall=0.07s  RSS  14 720KB ->  23 168KB
conc=20  limit=999999   wall=0.18s  RSS  23 168KB -> 139 232KB
conc=50  limit=999999   wall=0.34s  RSS  16 832KB -> 183 504KB
conc=100 limit=999999   wall=0.53s  RSS 183 504KB -> 264 496KB
conc=100 limit=50       wall=0.29s  RSS sem crescimento
```

Três rodadas de 20 concorrentes; a primeira é a que conta, porque a partir da
segunda o heap já cresceu e o delta some. ~2,6 MB de RSS por pedido concorrente
de um corpo de 1,5 MB — um factor de amplificação de **~1,7×** sobre o corpo, e
o processo não devolve a memória de imediato.

**2.5 — Inventário das colecções.** Não só as três citadas:

| rota | pagina? | cresce com dados do utilizador? |
|---|---|---|
| `GET /chats/list` | **sim** — `limit` (50/máx. 500), `offset`, `total` | sim, mas com tecto por página |
| `GET /chats/history` | `limit` (padrão 50), **sem tecto**, sem cursor | sim, e é o que mais cresce |
| `GET /users/contacts` | **não** | sim — a agenda inteira, `GetAllContacts` devolve o mapa completo |
| `GET /users/contacts/last-activity` | **não** | sim — agenda ∩ histórico |
| `GET /users/blocklist` | **não** | tecto natural (lista de bloqueio do WhatsApp) |
| `GET /labels` | **não** | tecto natural (etiquetas são poucas) |
| `GET /labels/{id}/chats` | **não** | sim — conversas por etiqueta |
| `POST /groups/list` | **não** | sim, com tecto natural (limite de grupos por conta) |
| `GET /newsletters/list` | **não** | sim, pequeno |
| `GET /admin/users` | **não** | cresce com **inquilinos**, não com dados do utilizador |
| `GET /communities/{jid}/subgroups` | **não** | tecto natural |
| `GET /groups/{jid}/join-requests` | **não** | tecto natural |
| `GET /webhook/history` | **não é colecção** | — |

**2.6 — `/webhook/history` não é uma colecção.** O nome engana: devolve o
**limite de gravação de mensagens** (`users.history`), um escalar. Já estava
registado como F252 (`wiring_routes.go:139-145`), e vale repeti-lo aqui porque a
tarefa pedia para o enumerar como colecção.

### Veredicto

A afirmação **cai como generalização** e **mantém-se para `/users/contacts`**.
"Nenhuma colecção é paginada" nunca foi verdade: `/chats/list` pagina há 18 dias
antes de a frase ter sido escrita.

**A causa do erro é o mais útil daqui.** A evidência gravada no
`CONTRATO-ARQUITETURAL.md` §19 era "`GET /chat/list` | 7 144 bytes" — número
compatível com a **primeira página de 50**, lido como se fosse a colecção
inteira. A resposta trazia `total` e `limit` no corpo; ninguém olhou.

Regra que sai disto: **uma colecção mede-se com dados que excedam qualquer
página plausível**. 1 266 contactos provaram que `/user/contacts` não pagina
porque 1 266 é maior que qualquer padrão; 7 144 bytes de conversas não provaram
nada.

Registado em **F291** (o tecto que falta) e **F294** (a afirmação falsa).

### O que ficou por medir, e porquê

- **A curva de `/users/contacts`.** `GetAllContacts` exige sessão com wa-noise
  viva (`EnsureSession`), e a sessão descartável responde `500` por não estar
  autenticada. O ponto de 61 459 bytes / 1 266 contactos continua a ser o único,
  e a linearidade (~48,5 bytes por contacto) é **inferida do código**, não
  medida. Procedimento para a medir está no `HUMAN-LAST.md`.
- **PostgreSQL para `limit=-1`.** Afirmado a partir da semântica documentada do
  `LIMIT` nos dois motores, **não** medido.

---

## 3. Idempotência — a afirmação separa-se em duas, e uma delas é mais fraca do que parece

### A afirmação do scorecard

> não há `Idempotency-Key`. `POST /chats/send/text` chamado duas vezes envia
> **duas** mensagens; um `500` pode ter enviado

### O que foi medido, e como

A primeira metade é verificável por leitura (`grep` por `Idempotency-Key`: zero
ocorrências) e confirma-se. A segunda metade exige conta emparelhada, que não
existe aqui. Foram medidas as rotas que **dão** para medir, e traçado o código
das que não dão.

### Números

**3.1 — As rotas de configuração são naturalmente idempotentes.** Cada corpo
enviado duas vezes, contra o servidor:

| rota | 1.ª chamada | 2.ª chamada idêntica | estado final |
|---|---|---|---|
| `POST /webhook` | `200 {"webhook":"…/hook"}` | `200 {"webhook":"…/hook"}` | igual |
| `PUT /webhook` | `200 {"active":true,…}` | `200 {"active":true,…}` | igual |
| `POST /session/proxy` | `200 {"Set":true,…}` | `200 {"Set":true,…}` | igual |

Repetir uma configuração é seguro. A lacuna **não** é aqui.

**3.2 — `POST /admin/users` tem uma chave de deduplicação de facto.**

```
1.ª: {"code":200,"data":{"id":"61b038b0…","name":"dup","token":"duptok",…}}
2.ª: {"code":409,"error":"conflict","success":false}
```

O índice `UNIQUE` sobre `token_hash` faz de chave: uma repetição depois de um
timeout **não** cria um segundo inquilino, devolve `409`. Não está documentado
como garantia de repetição; é.

(De passagem: o corpo do `409` usa a forma `error` **string**, não a forma
objecto — é a F266, o envelope com duas caras.)

**3.3 — As rotas de envio já aceitam um identificador escolhido pelo cliente.**
`SendMessageRequest.ID` (`pkg/domain/message.go:38`, `json:"Id,omitempty"`)
atravessa o use case (`send_message.go:58`) e chega ao adaptador como o
**stanza ID** do WhatsApp:

```go
var extra []wanoise.SendRequestExtra
if id != "" {
    extra = append(extra, wanoise.SendRequestExtra{ID: types.MessageID(id)})
}
```

**3.4 — E a nossa própria escrita local já é idempotente por esse identificador.**
`message_history` tem `UNIQUE(user_id, message_id)` e o insert é
`ON CONFLICT (user_id, message_id) DO UPDATE SET sender_push_name = … `
(`pkg/infra/db/message_history.go:41`). Repetir um envio com o mesmo `Id`
produz **uma** linha de histórico, não duas.

**3.5 — A especificação já é honesta sobre isto, e ao mesmo tempo contradiz-se.**
O esquema `IdCliente` diz:

> **NÃO é chave de idempotência.** Não foi medido que o servidor ou o WhatsApp
> deduplique por este campo; enviar o mesmo `Id` duas vezes envia duas mensagens.

As duas frases não podem ser ambas verdade: se não foi medido, a segunda é
afirmação sem medição. O mesmo texto está no `CONTRATO-ARQUITETURAL.md` §18.

### Veredicto

A afirmação **confirma-se na letra** (não há `Idempotency-Key`) e
**precisa de ser reescrita no conteúdo**. O que existe hoje:

- configuração: **idempotente**, sem precisar de chave;
- criação de inquilino: **deduplicada** pelo índice único, com `409`;
- envio: existe um identificador de cliente que já é o identificador de wire, e
  a escrita local já deduplica por ele. Falta (a) medir se o WhatsApp deduplica
  na recepção, (b) deduplicar do **nosso** lado para o segundo pedido nem chegar
  ao wire, e (c) devolver a resposta original em vez de repetir a operação.

**A resposta negativa das referências também é informação** (regra do
`CLAUDE.md`): o Baileys depende exactamente desta propriedade — reenvia com o
MESMO id no caminho de retry-receipt —, o que torna a dedução por id plausível
mas **não medida por nós**. Registá-lo como "não medido" é mais honesto do que
copiar a conclusão deles.

### O que ficou por medir, e porquê

- **Se o WhatsApp deduplica um `Id` repetido.** Exige conta emparelhada e um
  destinatário real. Procedimento exacto em `HUMAN-LAST.md`, entrada H14.
- **Se um `500` de `/chats/send/text` chegou a enviar.** Mesma razão. É a
  pergunta que mais importa das três, porque é a que decide se repetir é seguro.

---

## 4. Limitação de ritmo — metade da afirmação confirma-se, e a outra metade é falsa

### A afirmação do scorecard

> nenhuma rota devolve `429`. O `x/time/rate` que existe protege a CONTA no
> envio, não o servidor

### O que foi medido, e como

`grep` por `time/rate` e por `429`/`StatusTooManyRequests`, leitura das duas
ocorrências, e rajada de 60 pedidos concorrentes contra o servidor, repetida
quatro vezes, contando códigos de resposta e linhas de log.

### Números

**4.1 — Há exactamente UM `x/time/rate` no repositório, e ele protege o
SERVIDOR, por IP.** `pkg/bootstrap/limits.go:50-93`, ligado em
`pkg/bootstrap/router.go:257`. É um **observador**: mede e regista, nunca
recusa. Taxa 10 req/s, rajada 20, por IP de origem — e deliberadamente **sem**
confiar no `X-Forwarded-For`, para que o cliente não escape do balde inventando
um IP.

**A segunda metade da afirmação é falsa.** Não protege a conta no envio; não
protege nada, porque não bloqueia. E está instalada há tempo suficiente para o
comentário do ficheiro explicar o desenho: uma release de observação antes de
ligar o limite, para que o número saia do tráfego real.

**4.2 — A medição bate com os parâmetros, ao pedido.** Quatro rajadas de 60
pedidos concorrentes a `GET /labels`:

```
rodada 0: 429=0  avisos observe-only=40  códigos distintos: 200
rodada 1: 429=0  avisos observe-only=39  códigos distintos: 200
rodada 2: 429=0  avisos observe-only=47  códigos distintos: 200
rodada 3: 429=0  avisos observe-only=44  códigos distintos: 200
```

60 pedidos, rajada de 20 permitida, ~40 acima do balde. Todos respondidos `200`.
A linha de log é `rate limit observe-only: this request would have been
rejected`.

**4.3 — Mas a API DEVOLVE `429`, por outra razão inteiramente.**
`Category.HTTPStatus()` mapeia `CategoryRateLimited` para
`http.StatusTooManyRequests` (`pkg/domain/apperr/codes.go:140`), e
`errmap.classifyIQCode` produz essa categoria quando o servidor do WhatsApp
responde `429` ou `419` (`pkg/infra/wa-noise/errmap/iqerror.go:81`). São **64**
sítios a chamar `errmap.ClassifyIQ`. `RespondJSON` usa o status da categoria e
**ignora** o que o call site passou (`response.go:52`).

Ou seja: um cliente PODE receber `429` desta API — como relais do
estrangulamento do WhatsApp. E **zero** das 141 operações o documentam.

### Veredicto

- "nenhuma rota devolve `429`" — **falsa como afirmação sobre o código**,
  verdadeira como afirmação sobre a protecção do servidor. Corrigida.
- "o `x/time/rate` protege a CONTA no envio" — **falsa**. Protege o servidor,
  por IP, em modo observação. Corrigida.

Registado em **F293** (o `429` alcançável e não documentado) e **F290** (o mapa
por IP sem despejo).

### Inventário de detentores, pedido explicitamente (Regra 1 do `CLAUDE.md`)

No dia em que o observador virar limitador, ele passa a ser recurso **limitado**,
e a invariante do projecto aplica-se:

> Nada que espere por relógio ou por par morto pode ocupar slot limitado.

Quem passaria a disputar o balde por IP, e o pior caso de cada um:

| detentor | pior caso de ocupação | é bloqueio? |
|---|---|---|
| pedido HTTP normal (leitura local) | milissegundos | não |
| rota que fala com o WhatsApp | **30 s**, o timeout uniforme por operação (linha 117 do scorecard) | **sim** — 10 req/s de rajada 20 significam que 20 pedidos parados 30 s esgotam o balde do IP |
| `POST /session/connect` e o pareamento | dezenas de segundos até o QR expirar | **sim** |
| `GET /session/ws` (WebSocket) | **a ligação inteira**, minutos a horas | **sim, e é o pior** — se o handshake do WS consumir um token, um cliente com N abas consome N tokens e nunca os devolve |
| `/livez`, `/health/ready` | milissegundos | não, **mas** têm de ficar FORA do limitador: um balde esgotado que devolva `429` à sonda tira o processo do balanceador exactamente quando ele está sobrecarregado |
| entrega de webhook (saída) | não passa pelo router | n/a |

**A conclusão de desenho**, e ela não é especulação, é aplicação da regra: um
limitador por IP colocado no `router.Use` — onde o observador está hoje — cobre
os cinco primeiros com a MESMA política, e três deles são detentores longos. A
opção segura é limitar por **classe de rota**, com as sondas isentas e o
WebSocket contado à ligação e não ao pedido; e o `429` tem de trazer
`Retry-After`, porque sem ele o cliente honesto repete imediatamente e a
protecção vira amplificador.

### O que ficou por medir, e porquê

- **O número certo do limite.** Depende do perfil de uso real, que não existe
  neste servidor. O observador foi desenhado exactamente para o produzir; falta
  correr uma release com tráfego a sério e ler os avisos.
- **O crescimento do mapa por IP em campo.** A medição correu de um único IP
  (`127.0.0.1`), logo o mapa teve uma entrada. O defeito (F290) é de leitura de
  código.

---

## Os "eu não teria adivinhado"

A regra do `CLAUDE.md` diz que uma medição que só confirma o que já se pensava
provavelmente não mediu nada. Estes são os cinco:

1. **`/chats/list` pagina desde sempre, e a evidência que provava o contrário
   era uma página.** 7 144 bytes lidos como "a colecção inteira" eram os
   primeiros 50 de 2 000. A resposta trazia `total` e `limit` no corpo.
2. **O `x/time/rate` faz o oposto do que o scorecard diz.** Não protege a conta
   no envio: protege o servidor, por IP, em modo observação, já ligado no
   router, com os números exactos a baterem com a medição (40 avisos em 60
   pedidos, rajada 20).
3. **A API responde `429` e ninguém sabia.** 64 sítios podem produzi-lo como
   relais do WhatsApp; 0 das 141 operações o documentam; e as duas linhas do
   scorecard afirmam que ele nunca acontece.
4. **A perda de escrita concorrente é real e praticamente nunca acontece
   sozinha: 0 em 200 rodadas.** Um teste de concorrência sem controlo de
   escalonamento teria declarado o sistema seguro — e essa é a razão de o
   encontro marcado existir.
5. **`limit=-1` devolve a conversa inteira, e só em SQLite.** A mesma chamada em
   PostgreSQL seria `500`. O mesmo pedido, dois comportamentos, decididos pelo
   motor.

E um sexto, mais pequeno mas do mesmo tipo: **`/webhook/history` não é uma
colecção** — devolve um escalar de configuração. O nome sugeriu uma lista a toda
a gente que o leu, inclusive ao enunciado desta tarefa.

---

## Resumo dos vereditos

| linha | afirmação do scorecard | veredicto |
|---|---|---|
| **Paginação** | "nenhuma colecção é paginada" | **cai** como generalização; **mantém-se** para `/users/contacts`. `/chats/list` pagina com tecto de 500; `/chats/history` limita sem tecto |
| **Idempotência** | "não há `Idempotency-Key`" | **confirma-se na letra**; reescrita no conteúdo — configuração é idempotente, criação de inquilino é deduplicada por `409`, e o envio já tem identificador de cliente com dedup local. O que falta medir precisa de conta emparelhada |
| **Limitação de ritmo** | "nenhuma rota devolve `429`; o `x/time/rate` protege a CONTA no envio" | **primeira metade falsa** (o `429` é alcançável em 64 sítios, e não documentado); **segunda metade falsa** (protege o servidor, por IP, em observação) |
| **Concorrência** | "duas escritas simultâneas perdem uma, em silêncio" | **confirma-se no efeito**, **corrige-se no mecanismo**: não é read-modify-write da linha, é o par leitura→escrita de `resolveWebhookUseProxy`. 0/200 naturais, 1/1 com escalonamento controlado |
