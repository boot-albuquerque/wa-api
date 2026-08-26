# O que sobrou para um humano — as três campanhas de evidência

**Data**: 2026-08-26. **Origem**: fusão dos três `HUMAN-LAST.md` das campanhas
`disposable-session`, `failures` e `amber-observers`.

**Treze entradas**, agrupadas pelo **tipo de intervenção** e não pela campanha
que as produziu. O objectivo do agrupamento é **lotear**: quem for ao telemóvel
faz tudo o que precisa daquele emparelhamento de uma vez, em vez de ser
interrompido treze vezes.

Cada entrada diz, sem excepção: **o que já foi tentado**, **por que a automação
não resolve**, **a acção humana necessária**, o **risco**, e a **evidência que o
humano deve produzir**.

**Regra que vale para todas**: registe o **ANTES** antes de mexer. Sem linha de
base, o depois não significa nada.

## Índice por lote

| lote | o que é preciso ter | entradas |
|---|---|---|
| **A** | nada além de infraestrutura — nenhuma conta de WhatsApp | A.1 |
| **B** | **uma** sessão emparelhada, descartável | B.1 · B.2 · B.3 · B.4 · B.5 · B.6 · B.7 · B.8 |
| **C** | **duas** sessões emparelhadas | C.1 · C.2 |
| **D** | uma sessão emparelhada **e um terceiro a interagir** | D.1 · D.2 |
| **E** | alteração de código ANTES de medir | E.1 · E.2 |
| **F** | nada — registado para que ninguém gaste sessão de campo | F.1 · F.2 |

---

## Lote A — sem conta de WhatsApp

Este lote não precisa de telemóvel nenhum. Precisa de credenciais.

### A.1 — `POST /s3/test` e `POST /session/s3/test`: falta um bucket descartável

**O que já foi tentado**

1. **MinIO local em Docker.** O `docker` está disponível nesta máquina
   (27.3.1), portanto levantar um MinIO era trivial. Não chegou a ser feito
   porque o passo anterior é impossível: **a configuração recusa apontar para
   ele.** `POST /s3/config` valida o `endpoint` com
   `egress.ValidateOutboundURL`, que rejeita loopback e faixas reservadas.
   Medido:

   ```
   $ curl -s -X POST -H 'token: …' -H 'Content-Type: application/json' \
       -d '{"enabled":true,"endpoint":"http://127.0.0.1:9000", …}' localhost:8091/s3/config
   {"code":400,"error":{"code":"invalid_s3_endpoint","message":"invalid S3 endpoint"},"success":false}
   ```

   O mesmo valeria para qualquer IP de LAN — a validação resolve o nome e
   percorre todos os endereços. Achado **F277**.

2. **Endpoint público com credenciais falsas.** Funcionou até ao fim do
   percurso: a rota chegou à AWS e recebeu
   `403 InvalidAccessKeyId: The AWS Access Key Id you provided does not exist
   in our records.` O servidor devolveu isso como `500` com `error` em string
   (achado **F276**). Isto prova que o percurso está montado, mas **não é
   evidência da rota**: a falha veio do meu input.

**Por que a automação não resolve**: não há credenciais de S3 descartáveis
neste ambiente, e a única alternativa barata — um bucket local — está fechada
por uma regra de segurança que existe por bom motivo (SSRF, sec/F24) e que não
se contorna com uma variável de ambiente.

**Que acção humana falta**: uma das duas, à escolha:

- **(a)** fornecer credenciais de um bucket S3 **descartável** e público
  (qualquer provedor compatível com a API S3 serve: AWS, Cloudflare R2,
  Backblaze B2), com permissão de `ListObjectsV2` no bucket; ou
- **(b)** decidir a correcção da F277 — dar ao validador uma forma explícita de
  permitir loopback em ambiente de desenvolvimento, com o interruptor
  desligado por omissão. É uma mudança na fronteira de saída e precisa de
  decisão, não de iniciativa.

**Risco**: com (a), o risco é o do próprio bucket — a rota faz um
`ListObjectsV2`, que é leitura, e não escreve nada. Com (b), o risco é maior e
é de desenho: qualquer relaxamento do validador de saída é uma superfície de
SSRF, e por isso é decisão e não conserto.

**Evidência que o humano deve produzir**: `POST /s3/test` a devolver `200`, e
como observador independente o log da linha `S3 client initialized` seguido da
ausência de `S3 connection test failed` — ou, melhor, um `ListObjectsV2` do
mesmo bucket feito por fora (`aws s3 ls`) a mostrar o mesmo conteúdo.

---


---

## Lote B — uma sessão emparelhada, descartável

Emparelhe **uma** sessão nova (nunca a de trabalho) e faça as cinco de seguida.
B.1 é a que consome o emparelhamento, portanto deixe-a para o fim.

### B.1 — `POST /chats/send/sticker`: o fixture é um PNG 512×512, não um WebP

**Já tentado**: a bateria enviou um WebP sintético de 26 bytes, recebeu `200`,
a mensagem chegou e o cliente desenhou bolha vazia. Esta sessão apurou a causa:
**um WebP de entrada não é convertido** — `http.DetectContentType` devolve
`image/webp` (medido) e esse tipo cai no `default:` de `ConvertToWebPSticker`
(`pkg/infra/media/sticker/exif.go:62-63`), que devolve os bytes tal e qual.
Achado **F279**.

**Por que a automação não resolve**: o fixture resolve-se sozinho — o que não
se resolve é a conta. Enviar exige sessão emparelhada e um destino real.

**Acção humana**:

1. Gere um PNG **512×512** e passe-o como URI de dados. PNG, e não WebP, é o
   ponto: `image/png` cai no ramo que corre
   `ffmpeg -vf scale=512:512 -c:v libwebp -lossless 1`
   (`pkg/infra/media/sticker/sticker.go:92-96`). Confirme que `ffmpeg` existe
   na máquina que serve a API.
2. `POST /chats/send/sticker {Phone: <destino>, Sticker: "data:image/png;base64,…"}`
   → anote `message_id` **M**.
3. `GET /chats/history?chat_jid=<destino>` → localize **M**, confirme
   `message_type == "sticker"` e extraia o descritor de
   `data_json.Message.stickerMessage`.
4. `POST /chats/downloadsticker` com esse descritor → guarde os bytes.
5. Verifique os bytes: começam por `RIFF`, têm `WEBP` no offset 8, contêm um
   chunk `VP8 `/`VP8L`/`VP8X`, e as dimensões são 512×512.

**Risco**: baixo. Envia um autocolante a um destino de teste; nada público.

**Evidência a produzir**: o `200`, a linha do histórico com o descritor, e o
`xxd | head -2` dos bytes descarregados mostrando `RIFF…WEBP`, mais as
dimensões. E, para fechar a F279 com honestidade, repita o passo 5 com o
**WebP sintético antigo** — os dois resultados lado a lado é que provam que a
diferença estava no fixture e não no caminho.

---


---

### B.2 — `POST /newsletters/mark-viewed`: o contador de vistas

**Já tentado**: `200` com `data: null`. Esta sessão localizou o observador
desenhado — o próprio código vendorizado diz que `NewsletterMarkViewed`
"increments the view counter" (`internal/wa-noise/core/newsletter.go:52`) e que
`GetNewsletterMessageUpdates` entrega "reaction and view counts" (`:206-208`).
Também descartou uma via com medição: `POST /newsletters/messages` traz
`ViewsCount`, mas veio **`0` em 60 de 60** (`api/openapi/schemas/canal.yaml:1039-1042`)
e portanto não distingue nada.

**Por que a automação não resolve**: a via documentada
(`POST /newsletters/updates`) está **inoperante** — `500` ao fim de 30 s,
achado **F265**. A alternativa (`POST /newsletters/subscribe` + evento
`NewsletterLiveUpdate`) exige sessão emparelhada, canal com publicação, e um
receptor de eventos a correr.

**Acção humana** — duas frentes, e a ordem importa:

*Frente 1, barata, decide se vale a pena a frente 2*: com uma sessão ligada a
um canal **PRÓPRIO** (criado por `POST /newsletters/create`), com pelo menos uma
publicação:

```
POST /newsletters/messages {jid: C, count: 5}   ← ANTES: anote ViewsCount de S
POST /newsletters/mark-viewed {jid: C, serverIDs: [S]}   -> 200
POST /newsletters/messages {jid: C, count: 5}   ← DEPOIS: ViewsCount de S mudou?
```

Se mudar, a rota promove-se por aqui e a F265 deixa de a bloquear. Se vier `0`
nas duas, está provado que o contador não é servido por esta leitura **também
num canal próprio** — o que fecha a hipótese e é resultado publicável.

*Frente 2, se a 1 falhar*: aponte um receptor de webhook descartável (ou ligue
um cliente a `/session/ws`), chame `POST /newsletters/subscribe {jid: C}`,
depois `POST /newsletters/mark-viewed`, e capture o evento
`type: "NewsletterLiveUpdate"`. O corpo traz `event.Messages[]` com
`ViewsCount` e `ReactionCounts`.

**Risco**: baixo num canal próprio. **Não** faça num canal de terceiros: marcar
como visto num canal público é acto de conta real, mesmo que discreto.

**Evidência a produzir**: os dois corpos de `POST /newsletters/messages`
(ANTES/DEPOIS) com o `ViewsCount` do mesmo `MessageServerID`; ou o payload do
evento `NewsletterLiveUpdate`. Se ambas as frentes derem zero, cole as duas e a
rota fica 🟡 com **prova de ausência**, que é entrega válida.

**Nota de dependência**: corrigir a F265 desbloqueia esta rota e a B.3 de uma
só vez.

**Correcção da integração**: a versão original desta nota apontava a **F32** —
duas query IDs de newsletter erradas no upstream — como hipótese mais provável
para a F265. **Essa hipótese caiu.** `GetMessageUpdates` não é uma query MEX e
não tem query ID nenhum: é um IQ binário com namespace `newsletter`
(`internal/wa-noise/capabilities/newsletter/messages.go:89`). A causa
determinada é `PROTOCOL_CHANGED`, registada em `LIB-03` do
`internal/wa-noise/HOUSEKEEP.md` e em `INVESTIGATION-newsletter-updates.md`. O
que falta medir está em B.4, não num extractor de query IDs.

---


---

### B.3 — `POST /newsletters/react`: o ciclo k → k+1 → k

**Já tentado**: `200` com `data: null`, e o motivo registado foi "o canal de
teste não tinha mensagem". **Esse motivo ficou obsoleto no mesmo dia**: a
bateria mediu um canal com sessenta publicações, e o observador devolveu
contagens reais — `ReactionCounts: {'👍': 3, '😂': 58}`
(`api/openapi/paths/canal.yaml:543`).

**Por que a automação não resolve**: nada técnico. Falta **uma sessão
emparelhada** e a **autorização** para reagir a uma publicação real.

**Acção humana** — o ciclo completo, com remoção:

```
POST /newsletters/messages {jid: C, count: 5}          ← ANTES: ReactionCounts[E] == k
POST /newsletters/react    {jid: C, serverID: S, reaction: E}   -> 200
POST /newsletters/messages {jid: C, count: 5}          ← DEPOIS: k+1
POST /newsletters/react    {jid: C, serverID: S, reaction: ""}  -> 200   (remove)
POST /newsletters/messages {jid: C, count: 5}          ← VOLTOU: k
```

Três regras que, ignoradas, produzem um resultado que parece falha e não é:

- **`count` é obrigatório de facto.** Ausente, `0`, `null` ou negativo dá
  `500 newsletter_failed` ao fim do tempo limite. Leve `count` nas três
  leituras.
- **Não normalize o emoji.** `"❤"` e `"❤️"` são chaves SEPARADAS no mapa
  (`api/openapi/schemas/canal.yaml:1055-1059`). Compare a chave exacta que
  enviou.
- **A remoção é `reaction` vazio**, e é o passo que prova causalidade. Sem ele,
  um incremento pode ser de outra pessoa.

**Risco**: **este é o mais alto das seis.** Uma reacção num canal público é
visível para o canal a partir de uma conta real. Prefira um canal **próprio**;
se tiver de ser um seguido, peça autorização explícita e faça o passo de
remoção imediatamente a seguir.

**Evidência a produzir**: os três corpos de `POST /newsletters/messages` com o
mesmo `MessageServerID`, mostrando `k → k+1 → k`. É o mais barato dos seis
procedimentos e o que promove uma rota com uma única sessão.

---


---

### B.4 — `POST /newsletters/updates`: confirmar a forma nova de leitura de canal (EXP-3)

**Rota**: `POST /newsletters/updates`. Achado F265.

**O que já foi tentado**
`POST /newsletters/updates {"jid":"1203…@newsletter","count":5}` → `500` ao fim
de 30,0 s, `context deadline exceeded`. O irmão
`POST /newsletters/messages` no MESMO canal e segundo → `200 []`.

E o que a investigação acrescentou: `POST /newsletters/subscribe`, que envia um
IQ `set` para o MESMO `@newsletter` JID, responde `200`. Logo o servidor roteia
IQs para canais; o que ele ignora é o `<message_updates>` em particular.

**Por que a automação não resolve**
Duas razões, e a segunda é a que trava mesmo:
1. o silêncio do servidor não é reproduzível sem servidor;
2. **o canal de teste estava VAZIO** (`200 []`). Mesmo com a correcção
   aplicada, um canal sem mensagens não distingue "a forma nova funciona e não
   há nada" de "a forma nova funciona e não traz contadores". A pergunta que
   desenha a correcção — a resposta traz `views`/`reactions`? — exige um canal
   COM mensagens, e criá-lo exige conta.

**Acção humana necessária**
1. Usar (ou criar e publicar em) um canal com **pelo menos 3 mensagens**, uma
   delas com reacção e com visualizações.
2. `POST /newsletters/messages {"jid":"<canal>","count":5}` — capturar o corpo
   COMPLETO, não só o status.
3. Inspeccionar se cada mensagem traz `views`, `forwards`, `responses`,
   `reactions[]`. É isto que o port rsalcara/InfiniteAPI #503 afirma e que
   nunca vimos com os nossos olhos.
4. Só depois, com a correcção do stanza aplicada, repetir por
   `POST /newsletters/updates` e comparar os dois corpos.

**Risco**: baixo se o canal for do operador. Publicar mensagens num canal é
visível para os subscritores — usar canal de teste, não um canal com público.

**Evidência a produzir**
- corpo completo de `/newsletters/messages` num canal com conteúdo (é a linha de
  base, e hoje ela NÃO EXISTE);
- a mesma coisa depois da correcção, por `/newsletters/updates`;
- se os contadores aparecerem: a decisão de contrato (fundir as duas rotas ou
  manter projecções distintas) passa a ser tomável com dado em vez de com
  suposição.

---


---

### B.5 — `POST /session/logout`: o caminho de `200` precisa de um telemóvel

**O que já foi tentado**: os três estados possíveis de uma sessão descartável.

| estado da sessão | resultado medido |
|---|---|
| sem transporte vivo | `409 session_not_connected` — **bate com a documentação**, incluindo o `Detach` forçado (F93) visível no log como `Received kill signal` |
| com transporte vivo, nunca emparelhada | `500 {"error":"internal server error"}`; no log, `the store doesn't contain a device JID`. Achado **F275** |
| emparelhada | **não medido** |

**Por que a automação não resolve**: emparelhar exige ler um QR code com um
telemóvel, ou digitar um código de oito caracteres em *Aparelhos conectados*.
É a definição de acção que precisa de corpo humano.

**Que acção humana falta**: emparelhar **uma sessão descartável** — não a
sessão de trabalho — lendo o QR de `GET /session/qr`, e depois chamar
`POST /session/logout` nela.

**Risco**: baixo, e delimitado. O aparelho que se desemparelha é o da sessão
descartável, criada minutos antes; a conta de WhatsApp não é afectada para
além de perder essa entrada em *Aparelhos conectados*. **Não use uma sessão em
produção** — o logout é irreversível sem o telemóvel outra vez.

**Evidência que o humano deve produzir**: `200 {"details":""}`, e como
observador independente `GET /session/status` a passar de `loggedIn: true` para
`loggedIn: false`, **mais** a entrada a desaparecer de *Aparelhos conectados*
no telemóvel. As duas: a primeira prova o estado local, a segunda prova que o
`remove-companion-device` chegou mesmo ao WhatsApp.

---

### B.6 — `POST /chats/send/text` com o MESMO `Id` duas vezes: o WhatsApp deduplica?

**Origem**: medição das quatro lacunas de produção, 2026-08-26
(`MEDICAO-PRODUCAO.md` §3). É a pergunta que decide se a lacuna de idempotência
é grande ou pequena.

**O que já foi medido sem conta**, por leitura de código e contra o servidor
local:

| facto | onde |
|---|---|
| o `Id` do cliente vira o **stanza ID** do WhatsApp | `pkg/infra/wa-noise/adapters/chat/messenger.go:298`, `SendRequestExtra{ID: types.MessageID(id)}` |
| a nossa escrita local **já deduplica** por ele | `pkg/infra/db/message_history.go:41`, `ON CONFLICT (user_id, message_id)` sobre `UNIQUE(user_id, message_id)` |
| as rotas de **configuração** são idempotentes | medido: `POST /webhook`, `PUT /webhook`, `POST /session/proxy` repetidos deixam o mesmo estado |
| `POST /admin/users` deduplica por `token_hash` | medido: 2.ª chamada devolve `409`, não um segundo inquilino |

**O que a especificação afirma hoje**, e que ninguém mediu: o esquema
`IdCliente` diz "**NÃO é chave de idempotência**. Não foi medido que o servidor
ou o WhatsApp deduplique por este campo; enviar o mesmo `Id` duas vezes envia
duas mensagens". As duas frases não podem ser ambas verdade — a segunda é uma
afirmação sem medição, e é essa que esta entrada existe para resolver.

**A resposta negativa das referências também é informação**: o Baileys reenvia
com o MESMO id no caminho de *retry receipt*, o que torna a deduplicação na
recepção plausível. Plausível não é medido, e copiar a conclusão deles sem
medir é exactamente o que o `CLAUDE.md` proíbe.

**Por que a automação não resolve**: enviar exige sessão emparelhada e um
destinatário real que possa ser inspeccionado.

**Que acção humana falta**:

1. emparelhar uma sessão **descartável**;
2. `POST /chats/send/text` com `{"Phone":"<destino>","Body":"idem-1","Id":"3EB0IDEMPOTENCIA01"}`;
3. repetir **o mesmo corpo, byte a byte**;
4. olhar para o telemóvel de destino.

**Evidência que o humano deve produzir**, e são três observadores distintos:

- **o destinatário**: **uma** bolha ou **duas** no ecrã. É a resposta à pergunta.
- **`GET /chats/history?chat_jid=<destino>`** na sessão de origem: quantas
  linhas com `message_id = 3EB0IDEMPOTENCIA01`. Esperado **uma**, pelo
  `ON CONFLICT` — se forem duas, o achado é maior do que se pensava.
- **os dois corpos de resposta**: se o segundo devolver `message_id` diferente
  do `Id` enviado, o campo não chegou ao wire e a experiência não mediu nada.

**Risco**: baixo. Duas mensagens de texto para um destino escolhido pelo
operador. Nada é apagado, nada é irreversível.

---

### B.7 — um `500` de `POST /chats/send/text` chegou a enviar?

**Origem**: a mesma medição. É a metade da linha "Idempotência" do scorecard
que continua sem prova: *"um `500` pode ter enviado"*.

**O que já se sabe sem conta**: o prazo contra o WhatsApp é de **30 s por
operação, uniforme e não configurável por rota** (linha 117 do scorecard). Um
`500` por expiração desse prazo é exactamente o caso ambíguo: o `SendMessage`
foi para o wire e a resposta é que não voltou a tempo.

**Por que a automação não resolve**: provocar o timeout de forma controlada
exige uma sessão emparelhada **e** degradar a rede entre o processo e o
WhatsApp — não é reproduzível contra um servidor sem conta.

**Que acção humana falta**: com uma sessão descartável emparelhada, cortar a
saída de rede do processo (ou apontá-lo a um proxy que engula a resposta)
**depois** de o `POST /chats/send/text` partir, deixar o prazo de 30 s expirar,
e ver o telemóvel de destino.

**Evidência que o humano deve produzir**: o corpo `500` da API **e** o estado do
destinatário. Se a mensagem chegou, está provado que repetir cegamente duplica,
e o `Idempotency-Key` deixa de ser opcional para quem tem retries automáticos.
Se não chegou, a repetição é segura para este caso concreto — o que é uma
informação diferente e menos alarmante do que a linha do scorecard sugere.

**Risco**: médio. Cortar a rede do processo afecta todas as sessões que ele
serve. **Faça-o num processo isolado**, com a sessão descartável e mais nada —
como o servidor da porta 8093 desta medição.

---

### B.8 — a curva de `GET /users/contacts`: o ponto de 1 266 contactos precisa de um segundo

**Origem**: medição da linha "Paginação" (`MEDICAO-PRODUCAO.md` §2).

**O que já foi medido sem conta**: nada, e é essa a questão.
`GetContactsUseCase` chama `EnsureSession` antes de qualquer coisa
(`pkg/application/usecase/user/get_contacts.go:23`), e uma sessão descartável
**não emparelhada** responde `500` — o cliente wa-noise existe mas não tem
device JID. O adaptador devolve o mapa inteiro sem limite nenhum
(`pkg/infra/wa-noise/adapters/user/adapter.go:80-90`).

O único ponto que existe é o histórico: **61 459 bytes com 1 266 contactos**,
≈48,5 bytes por contacto. **Um ponto não é uma curva**, e a linearidade está
inferida do código, não medida.

**Que acção humana falta**: com uma sessão emparelhada cuja agenda tenha
tamanho conhecido, medir `curl -s -o /dev/null -w '%{size_download}'` em
`GET /users/contacts` e comparar com o número de contactos devolvido. Duas
contas de tamanhos diferentes dariam dois pontos; a mesma conta antes e depois
de um `HistorySync` daria dois também.

**Evidência que o humano deve produzir**: os pares (bytes, nº de contactos), e
o **RSS do processo** antes e depois da chamada — porque `RespondJSON`
materializa o corpo inteiro com `json.Marshal` antes de escrever
(`pkg/presentation/http/response.go:83`), e foi essa amplificação que levou o
processo a 264 MB no teste equivalente com `/chats/history` (F291).

**Risco**: nulo. É uma leitura.

---


---

## Lote C — duas sessões emparelhadas

As duas exigem **duas contas distintas**. Emparelhe as duas de uma vez.

### C.1 — `POST /groups/{group_jid}/join-requests`: a fila tem de estar CHEIA antes

**Já tentado**: a bateria chamou a rota contra grupos sem fila e recebeu `200`.
Um `200` contra fila vazia não prova nada. Esta sessão confirmou, lendo o
protocolo, que a fila **se cria só por HTTP**: `JoinWithLink` devolve o nó
`membership_approval_request` quando o grupo exige aprovação
(`internal/wa-noise/capabilities/group/invite.go:117-120`).

**Por que a automação não resolve**: são precisas **duas sessões emparelhadas**
distintas. Nenhum outro passo precisa de mãos.

**Acção humana** — sequência inteira por API, A é a dona do grupo, B a
candidata:

```
A: POST /groups/create              {name: "obs-req-<data>"}      -> G
A: PUT /groups/{group_jid}/settings/join-approval    {groupjid: G, mode: true}     -> 200
A: POST /groups/info                {groupJID: G}   ← confirme IsJoinApprovalRequired == true
A: POST /groups/invitelink          {groupJID: G}                 -> L
B: POST /groups/join                {code: L}                     -> 200
A: GET  /groups/{group_jid}/join-requests ?group_jid=G   ← ANTES: tem de conter B. Se vier [], PARE
A: POST /groups/{group_jid}/join-requests
       {groupJID: G, Phone: ["<JID COMPLETO de B>"], Action: "approve"}   -> 200
A: GET  /groups/{group_jid}/join-requests ?group_jid=G   ← DEPOIS: já não contém B
A: POST /groups/info                {groupJID: G}  ← B aparece em Participants
```

Duas armadilhas que fazem isto falhar por motivo errado: `Phone` exige **JID
completo** (número nu dá `400 invalid_phone`), e `Action` é
`approve`/`reject` — `add`/`remove` dá `400 invalid_action`.

Repita com `Action: "reject"` num segundo candidato: a rejeição também retira
da fila e **não** acrescenta a `Participants`. É o controlo que separa "a fila
esvaziou" de "aprovou mesmo".

**Risco**: baixo. Grupo criado para o efeito, apagável no fim (`/groups/leave`).
Não use um grupo com pessoas dentro.

**Evidência a produzir**: os três corpos — `GET` ANTES não vazio, o `200` da
decisão, o `GET` DEPOIS vazio — mais o `POST /groups/info` com B em
`Participants`. **Sem o "ANTES não vazio" a promoção não vale**, e é
precisamente o que faltou à bateria original.

---


---

### C.2 — `POST /status/set/image`, `/status/set/video`, `/status/set/audio`

**Já tentado**: três `200` com `message_id`, e **zero status na interface**
depois de recarga completa — a F256. A causa principal foi medida:
`getStatusBroadcastRecipients` só inclui contactos com `FullName`
(`internal/wa-noise/core/broadcast.go:70`), e a conta Business tem **1** em
461. A F256 também **desqualificou** o observador em que se confiou: o
WhatsApp Web daquela conta não mostra status de ninguém, logo não distingue
caso nenhum.

**Por que a automação não resolve**: o observador é a **segunda sessão**, e são
precisas duas contas reais — mais a pré-condição de que B esteja na lista de
destinatários de A.

**Acção humana** — e o passo 1 é o que poupa a publicação inútil:

1. **Verifique a pré-condição ANTES de publicar.** `GET /users/contacts` na
   sessão A; localize o JID da sessão B; confirme `FullName` **não vazio**. Se
   estiver vazio, PARE: o status não chegará a B, e o resultado não distinguirá
   "não publicou" de "publicou e B não é destinatário" — que é exactamente o
   nó da F256. Corrija primeiro (guardar B na agenda de A e ressincronizar com
   `POST /users/contacts/sync`), e reconfirme por `GET /users/contacts`.
2. Anote o ANTES em B: `GET /chats/history?chat_jid=status@broadcast`.
3. Em A: `POST /status/set/image` com uma imagem reconhecível → anote
   `message_id` **M**.
4. Em B: `GET /chats/history?chat_jid=status@broadcast` → procure **M**.
   Alternativa em tempo real: webhook/`/session/ws` de B, evento
   `type: "Message"` com `event.Info.Chat == "status@broadcast"`.
5. Repita para `/status/set/video` e `/status/set/audio`. **Cada uma precisa
   da sua própria medição** — as duas de hoje estão marcadas "idem, herdada",
   e herdar evidência é o mesmo defeito que esta campanha existe para corrigir.

**Risco**: **alto em visibilidade.** Um status vai para toda a lista de
destinatários — 165 pessoas na conta `lucas`. Use conteúdo neutro e apague a
seguir. O utilizador já restringiu testes de visibilidade pública na conta
`lucas`: **peça autorização explícita antes**, e prefira a conta Business, cuja
lista tem 2 destinatários — desde que o passo 1 confirme que B é um deles.

**Evidência a produzir**: o `GET /users/contacts` mostrando `FullName` de B
preenchido; o `200` de A com `message_id`; e o `GET /chats/history` de B, ANTES
e DEPOIS, com **M** a aparecer. As três juntas promovem a rota; qualquer uma
sozinha não.

**Se a pré-condição não for satisfazível** — por exemplo, se nenhuma das contas
disponíveis puder ter a outra com `FullName` —, então a entrada correcta é
manter 🟡 com a prova de que a lista de destinatários exclui todo observador
possível, e escalar a **correcção da F256** (deixar de exigir `FullName`, e
recusar com `4xx` quando a lista resolve só para o próprio), que é decisão
pendente do utilizador por tocar em código vendorizado.


---

## Lote D — uma sessão emparelhada e um terceiro a interagir

As duas dependem de alguém do outro lado a fazer alguma coisa. Combine o
horário antes de emparelhar.

### D.1 — `POST /call/reject`: precisa de alguém a ligar

**O que já foi tentado**: nada, e é honesto dizê-lo — a rota exige um evento
`CallOffer` a entrar, e uma sessão nunca emparelhada não recebe chamadas.

**Por que a automação não resolve**: não há forma de provocar uma chamada de
WhatsApp a partir daqui. O `call_id` que a rota recebe vem de um evento que só
o servidor do WhatsApp emite.

**Que acção humana falta**: com uma sessão emparelhada e `CallOffer` (ou `All`)
subscrito em `POST /webhook`, pedir a alguém que ligue para o número, apanhar o
`call_id` do evento, e chamar a rota dentro da janela da chamada.

**Risco**: nenhum para os dados — rejeitar uma chamada é o que o utilizador
faria à mão.

**Evidência que o humano deve produzir**: `200`, e do lado de quem ligou, a
chamada a cair imediatamente. Esse "do lado de quem ligou" **é** o observador
independente; sem ele fica 🟡, não ✅.

---


---

### D.2 — `POST /chats/request-unavailable-message`: precisa de uma mensagem genuinamente indecifrável

**Já tentado**: rastreado o caminho inteiro em código. O reenvio chega como
`*events.Message` com `UnavailableRequestID` igual ao `request_id` que o `200`
devolveu (`internal/wa-noise/capabilities/message/history_sync.go:250`), é
gravado em `message_history` com `datajson = json.Marshal(evt)`
(`pkg/bootstrap/eventhandler_message.go:311`) e é servido como `data_json` por
`GET /chats/history` (`pkg/infra/db/chat_history_repository.go:22`). Vai também
por webhook e `/session/ws` (`pkg/bootstrap/eventhandler.go:29`).

**Por que a automação não resolve**: a pré-condição é uma mensagem
**genuinamente indecifrável**, isto é, uma sessão Signal dessincronizada entre
duas pontas reais. Não se fabrica por HTTP e não se simula sem falsificar o
próprio efeito que se quer medir.

**Acção humana**:

1. Ligue a sessão A e confirme `historyLimitForUser > 0` —
   `GET /webhook/history` devolve `{History: N}`; se `N == 0`, a gravação está
   desligada e o observador de banco não existe (só resta o webhook).
2. Provoque um `UndecryptableMessage`: a via prática é ter a conta ligada
   noutro dispositivo, enviar mensagens enquanto A está desligada, e forçar
   reinstalação/re-pareamento parcial. Anote o `chat` e o `id` da mensagem que
   ficou por decifrar (o log de A regista o `UndecryptableMessage`).
3. `POST /chats/request-unavailable-message {chat, sender, id}` → anote o
   `request_id` **R** do corpo `200`.
4. Espere. Depois: `GET /chats/history?chat_jid=<chat>` e procure um elemento
   cujo `data_json` contenha `"UnavailableRequestID":"R"`.

**Risco**: baixo em efeito visível (nada é publicado a terceiros); médio em
custo, porque dessincronizar a sessão pode obrigar a re-parear. Não faça na
sessão que está a servir outras medições.

**Evidência a produzir**: o `200` com `request_id`, e o elemento de
`GET /chats/history` com o mesmo valor em `UnavailableRequestID`, colados lado a
lado. Se o reenvio não chegar em, digamos, cinco minutos, isso **também** é
evidência — e transforma o 🟡 em ❌ ou mantém 🟡 com prazo medido, o que é
melhor do que hoje.

---


---

## Lote E — exigem alteração de código ANTES de medir

Ao contrário de todos os outros lotes, estes dois **não são só uma medição**:
é preciso construir uma build com a alteração, e só depois medir. Leia
`INVESTIGATION-block-unblock.md` primeiro.

### E.1 — confirmar que `unblock` com `jid`=LID é aceite (EXP-1)

**Rota**: `POST /users/unblock`. Achado F264.

**O que já foi tentado**
Quatro combinações medidas em 2026-08-26 (duas contas, número real e
inexistente, PN e LID), todas `422 upstream_rejected` com
`info query returned status 400: bad-request` por baixo. E — o dado que o trace
acrescentou — **as quatro produziram o MESMO stanza no fio**, porque
`resolveBlocklistPNJID` (`pkg/infra/wa-noise/adapters/user/blocklist.go:103`)
converte LID → PN antes do envio. A linha "testado em LID" da tabela da F264
**não testou LID**.

**Por que a automação não resolve**
O stanza é aceite ou recusado pelo servidor do WhatsApp. Não há dublê que
responda por ele: qualquer fake que escrevêssemos aceitaria a forma que nós
achamos correcta, que é a definição de dublê que abençoa o próprio erro
(ARMADILHA #1). Precisa de socket autenticado contra `s.whatsapp.net`.

**Acção humana necessária**
1. Numa build com a alteração mínima proposta em `INVESTIGATION-block-unblock.md`
   (deixar de degradar LID → PN no caminho de `unblock`), chamar:
   `POST /users/unblock {"jid":"90937376170214@lid"}` — o LID já conhecido de
   `554192421234`.
2. Capturar o log da resposta do IQ.

**Risco**: baixo. Desbloquear alguém que não está bloqueado é inócuo, e a
blocklist medida estava VAZIA (`{"Blocklist":[],"DHash":"…"}`), logo não há
estado a perder. Reversível por `POST /users/block`.

**Evidência a produzir**
- resposta HTTP completa (status + corpo);
- a linha de log do IQ: `200` com `<list>`, ou o `status` do erro;
- a blocklist ANTES e DEPOIS (`GET /users/blocklist`) — sem a linha de base, o
  "depois" não significa nada.

**O que cada resultado significa**
- `200` → H4 confirmada em campo; a causa fica de Nível C+B e a correcção da
  biblioteca é um port mecânico.
- `400 bad-request` outra vez → H4 cai, e a próxima pista é capturar o stanza
  real da SPA (E.2).

---


---

### E.2 — confirmar que `block` exige `pn_jid` (EXP-2)

**Rota**: `POST /users/block`. Achado F264.

**O que já foi tentado**: o mesmo que E.1. Nunca foi enviado um `<item>` com
`pn_jid`, porque a biblioteca vendorizada não sabe montá-lo.

**Por que a automação não resolve**: idem E.1 — e, aqui, com um agravante:
o whatsmeow e o Baileys divergem no que fazem quando o PN não resolve
(whatsmeow pergunta ao servidor com `GetUserInfo`; Baileys lança `400` local).
Qual dos dois o servidor exige só se sabe enviando.

**Acção humana necessária**
1. Portar `8d023aa973` para `internal/wa-noise/capabilities/user/blocklist.go`
   (ver o caminho detalhado na investigação).
2. `POST /users/block {"jid":"554192421234@s.whatsapp.net"}` — número REAL, com
   conversa existente (a SPA recusa bloquear contacto PN sem conversa; ver
   `internal/wa-headless/capabilities/block/block.go:15`).
3. Repetir com `{"jid":"90937376170214@lid"}`.
4. **Variante de controlo, e é a que interessa**: enviar `block` com `jid`=LID e
   **sem** `pn_jid`. Se der `400`, fica provado que `pn_jid` é obrigatório — que
   é a afirmação que hoje só temos por leitura de código alheio.

**Risco**: **médio, e não trivial.** Bloquear um contacto real é visível para o
outro lado (deixa de receber mensagens e de ver estado) e altera estado da
conta. Usar um número controlado pelo próprio operador, nunca um terceiro. Após
cada medição, desbloquear e confirmar por `GET /users/blocklist`.

**Evidência a produzir**
- `GET /users/blocklist` antes (linha de base);
- resposta de cada uma das quatro chamadas (3 + controlo);
- `GET /users/blocklist` depois de cada uma — é a segunda leitura que prova o
  efeito, não o `200`;
- estado final: blocklist de volta ao valor da linha de base.

---


---

## Lote F — registado para NÃO se gastar sessão de campo

### F.1 — datar a retirada do `<message_updates>` (EXP-4): não vale a pena

**O que já foi tentado**: a pesquisa encontrou dois pontos no tempo — a issue
#761 do whatsmeow (2025-02) em que ainda chegava RESPOSTA com campos nulos, e a
issue #2555 do Baileys (2026-05) em que já não chega nada.

**Por que a automação não resolve**: não resolve mesmo. Não há como consultar o
histórico do servidor do WhatsApp.

**Acção humana necessária**: nenhuma que valha a pena. Fica registado porque a
sequência "resposta degradada → silêncio" é a assinatura de uma retirada
progressiva, e essa leitura pode ser útil quando a PRÓXIMA rota de newsletter
começar a devolver campos nulos. Não é motivo para gastar uma medição.

**Risco**: nenhum. **Evidência**: nenhuma exigida.

---


---

### F.2 — o que NÃO precisa de humano

Registado para que ninguém gaste uma sessão de campo com isto:

- **Correr o extractor de `scripts/mex-query-ids/` para a F265.** Inútil.
  `GetMessageUpdates` não é uma query MEX — é um IQ binário simples e não tem
  query ID nenhum (`internal/wa-noise/capabilities/newsletter/messages.go:89`).
  A hipótese registada na F265 está falsificada por leitura de código, e a
  entrada foi corrigida.
- **Repetir a bateria de block/unblock com mais números ou mais contas.** Já
  foram 4 combinações em 2 contas. O trace mostra que todas produzem o mesmo
  stanza: mais linhas da mesma tabela não acrescentam informação, só custo.


---


## Lote G — sem procedimento escrito, e por opção

`POST /session/pairphone`, `POST /users/avatar`, `POST /users/privacy` e
`POST /users/status` exigem humano e continuam ⬜. As quatro foram
**explicitamente excluídas do escopo** das três campanhas: não foram tentadas
nem medidas, e ficam com o motivo que já tinham em
`docs/OPENAPI-EVIDENCIAS.md`.

O que as separa dos lotes A–F, e é a distinção que vale a pena enunciar:

> É descartável o que pertence à **sessão**. Não é descartável o que pertence à
> **conta de WhatsApp** por trás dela.

Uma sessão nova custa uma chamada a `POST /admin/users`. Um avatar, um recado
ou uma definição de privacidade são de uma pessoa, e apagá-los não se desfaz
criando outra sessão. Escrever procedimento para elas sem essa decisão tomada
seria convidar a executá-lo. O inventário completo está em
`IRREVERSIBLE-LAST.md`.

---

## Nota de fusão

Este ficheiro funde os três `HUMAN-LAST.md` das campanhas paralelas, sem perder
nenhuma entrada. A contagem, antes e depois:

| origem | entradas | onde ficaram |
|---|---:|---|
| `disposable-session` | 3 | A.1, B.5, D.1 |
| `failures` (EXP-1…EXP-4) | 4 | E.1, E.2, B.4, F.1 |
| `amber-observers` (§1…§6) | 6 | D.2, B.1, C.1, B.2, B.3, C.2 |
| **subtotal da fusão** | **13** | **13** — nenhuma perdida, nenhuma duplicada |
| `evidence-production-gaps` (26/08) | 3 | B.6, B.7, B.8 |
| **total** | **16** | — |

As três de 26/08 vieram da medição das quatro lacunas de produção
(`MEDICAO-PRODUCAO.md`) e são todas do **mesmo tipo**: perguntas cuja resposta
está do outro lado do wire do WhatsApp, e que nenhum servidor local responde.

F.2 (*o que NÃO precisa de humano*) veio do `failures` e não é uma entrada
accionável: é a lista do que ficaria por engano. O Lote G não vinha de nenhum
dos três como entrada — vinha como nota de exclusão, e foi promovido a lote
para que a razão da exclusão fique junto do resto.

**Os caminhos foram actualizados para as formas canónicas** pela tabela
`api/openapi/caminhos.tsv`, e as referências a achados foram renumeradas na
integração: o `F273` do `failures` é hoje **F278**, os `F273`/`F274` do
`amber-observers` são **F279**/**F280**, e o `F70` do `internal/wa-noise` é
**LIB-04**.
