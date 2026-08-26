# Observadores para as oito rotas 🟡

**Data**: 2026-08-26. **Worktree**: `campaign/evidence-amber-observers`, sobre
`fea7e6e`. **Sessão sem conta WhatsApp emparelhada** — nada aqui é medição de
campo nova; tudo é rastreio de código, do protocolo vendorizado e da
documentação já medida, com `ficheiro:linha`.

## O que esta investigação foi, e o que não foi

A regra do projecto é que 🟡 só sobe a ✅ com **observador independente**.
A pergunta desta sessão não foi "como promover", foi **"existe observador
programático, e onde está ele no código?"**.

O resultado é assimétrico e vale dizê-lo já: **nenhuma das oito não tem
observador**. Sete têm observador identificado no código, e a que sobra tem o
observador identificado e a razão exacta pela qual ele mostra zero. O que
bloqueia as oito não é ausência de observador — é **pré-condição**: todas
exigem uma conta emparelhada, e cinco exigem **duas**.

Isto contradiz o motivo escrito hoje em `docs/OPENAPI-EVIDENCIAS.md` para
quatro delas, e as correcções estão marcadas abaixo com **⚠️ MOTIVO ANTIGO
ERRADO**.

## Legenda dos veredictos

| veredicto | significa |
|---|---|
| `OBSERVADOR_EXISTE` | há leitura programática que muda de valor por causa da operação; falta a pré-condição, e ela está escrita |
| `SEM_OBSERVADOR_PROGRAMATICO` | esgotado o inventário, nenhuma leitura desta API distingue o efeito |
| `SÓ_FALTA_FIXTURE` | o observador já observou; o que era mau era a entrada |

## Graduação das fontes externas consultadas

| nível | o que é | usado aqui |
|---|---|---|
| A | doc oficial / doc do próprio código vendorizado | `internal/wa-noise/core/newsletter.go:52` e `:206` |
| B | comportamento medido neste repositório | `docs/OPENAPI-EVIDENCIAS.md`, `api/openapi/schemas/canal.yaml` |
| C | implementação madura de terceiro | whatsmeow (upstream do fork), whatsapp-web.js |
| D | issue/PR com reprodução | — |
| E | opinião sem fonte | não usada |

---

# 1. `POST /chats/request-unavailable-message`

**Motivo antigo**: *"200; o reenvio depende do par e não há como observá-lo
daqui."* ⚠️ **MOTIVO ANTIGO ERRADO** — há três formas de o observar daqui.

## Inventário de observadores

| candidato | existe? | prova |
|---|---|---|
| segundo `GET` da mesma rota | **não** | é `POST` e não devolve estado |
| endpoint relacionado | **não** | não há rota "listar pedidos de reenvio" |
| base de dados (SQLite local) | **SIM** | a mensagem reenviada é gravada em `message_history` por `saveMessageHistory` (`pkg/bootstrap/eventhandler_message.go:278-332`), com `datajson = json.Marshal(evt)` (linha 311) |
| evento | **SIM** | o reenvio chega como `*events.Message` normal com `UnavailableRequestID` preenchido: `msgEvt.UnavailableRequestID = reqID` em `internal/wa-noise/capabilities/message/history_sync.go:250` |
| webhook | **SIM** | `postmap["event"] = rawEvt` (`pkg/bootstrap/eventhandler.go:29`) — o evento INTEIRO vai no corpo, e `UnavailableRequestID` é campo exportado sem etiqueta JSON (`internal/wa-noise/protocol/types/events/message.go:80`) |
| WebSocket `/session/ws` | **SIM** | mesmo `postmap`, mesmo portão de subscrição, despachado em `pkg/bootstrap/lifecycle_webhook.go:165` |
| sessão receptora | não aplicável | quem reenvia é o PAR, não uma segunda sessão nossa |
| metadados / contador / timestamp | **não** | a resposta devolve `request_id` e `timestamp` do PRÓPRIO pedido, não do reenvio (`pkg/domain/unavailable_message.go:13-21`) |
| `message_id` | **SIM, e é a chave da junção** | o `request_id` que o `200` devolve é o `resp.ID` do peer message (`pkg/infra/wa-noise/adapters/misc/adapter.go:141`), e é exactamente o valor que reaparece em `UnavailableRequestID` |
| ack / recibo de leitura | **não** | o `SendMessage(..., Peer: true)` devolve ack de ENTREGA do pedido, não do reenvio |
| histórico | **SIM** | `GET /chats/history` devolve a coluna `datajson` como `data_json` (`pkg/infra/db/chat_history_repository.go:22` e `:52`) |
| listagem / estado | **não** | — |
| sistema de ficheiros / armazenamento | parcial | S3, quando ligado, guarda mídia do reenvio; é derivado do evento, não fonte independente |
| `GET /webhook/history` | **não** | apesar do nome, lê `users.history` — o LIMITE de gravação. Já registado como F252, e documentado em `api/openapi/paths/infra.yaml:783-786` |

## O observador, escrito como asserção

Depois de `POST /chats/request-unavailable-message` devolver
`{"request_id": R}`:

```
GET /chats/history?chat_jid=<chat>
  -> existe elemento cujo data_json contém "UnavailableRequestID":"R"
```

e, em paralelo, o webhook ou `/session/ws` recebe um evento `type: "Message"`
cujo `event.UnavailableRequestID == R`.

## Pré-condição, e o que a torna difícil

Tem de existir uma mensagem **genuinamente indecifrável** — um
`*events.UndecryptableMessage` real. Não se fabrica: exige que um par envie
com sessão Signal dessincronizada. Não é criável com segurança daqui.

Duas armadilhas medidas no código, que quem for medir tem de saber:

1. `historyLimitForUser(evh.UserID) <= 0` faz `saveMessageHistory` sair no
   topo (`eventhandler_message.go:279-282`). Com gravação desligada, o
   observador de banco **não existe** — resta o webhook/WS.
2. `mensagemJaProcessada` (`pkg/bootstrap/message_dedup.go:97`) descarta pelo
   `Info.ID`. Se o mesmo ID já tiver passado pelo caminho de tempo real, o
   reenvio é suprimido antes de chegar ao webhook. Não medi este caso; é
   hipótese, não facto.

**Veredicto: `OBSERVADOR_EXISTE`** — três, independentes entre si
(banco/`GET /chats/history`, webhook, WebSocket). Falta a pré-condição, que não
é criável sem par real. Procedimento em `HUMAN-LAST.md` D.2.

---

# 2. `POST /chats/send/sticker`

**Motivo antigo**: *"200 e a mensagem chegou, mas o WebP é sintético e o
cliente desenha bolha vazia."* Correcto no sintoma, **incompleto na causa** —
e a causa muda o que é preciso fazer.

## A causa, rastreada

A documentação afirma, em `api/openapi/paths/envio.yaml:458-460`:

> A imagem recebida é **sempre convertida para WebP** pelo servidor; o que
> sobe ao WhatsApp é o resultado da conversão, nunca os bytes de entrada.

**Isto é falso.** `ConvertToWebPSticker`
(`pkg/infra/media/sticker/exif.go:40-65`) converte em dois ramos apenas:

```go
case strings.HasPrefix(mimeType, "video/"), mimeType == "image/gif":   // ffmpeg
case mimeType == "image/jpeg", mimeType == "image/png", mimeType == "image/jpg": // ffmpeg
default:
    return data, mimeType, nil   // ← passa incólume
```

O tipo é decidido por `http.DetectContentType(data)` (linha 41). **Medido
nesta sessão**, com o próprio exemplo que a documentação publica
(`envio.yaml:489`):

```
$ go run /tmp/wsniff.go
webp sniff: image/webp
```

`image/webp` cai no `default`. Ou seja: um WebP de entrada **nunca é
convertido** — recebe só o EXIF (`exif.go:33-35`) e sobe tal e qual. O
autocolante sintético de 26 bytes foi enviado como estava, sem passar pelo
`scale=512:512` que o ramo de imagem aplica
(`pkg/infra/media/sticker/sticker.go:92-96`). A bolha vazia é isso.

Registado como **F279** no `HOUSEKEEP.md`.

**Nível C — o que a referência diz.** O whatsapp-web.js tem a MESMA política
("conversion to webp is done through ffmpeg, and you'll need it if you want to
send stickers that are not already in webp format"). Logo a divergência não
está na política de conversão — está só na nossa documentação, que promete
conversão incondicional. É um caso de resposta útil da referência a confirmar
que **não** há bug de desenho a corrigir aqui, só documentação e fixture.

## Inventário de observadores

| candidato | existe? | prova |
|---|---|---|
| segundo `GET` | **não** | rota de envio |
| histórico | **SIM** | `recordOutgoing(..., "sticker", ...)` em `pkg/infra/wa-noise/adapters/chat/messenger.go:570`; o `datajson` traz a `StickerMessage` completa (`pkg/infra/history/outgoing.go:91-129`) |
| endpoint relacionado | **SIM, e fecha o ciclo** | `POST /chats/downloadsticker` (✅) consome o descritor que sai do `data_json` de `GET /chats/history` — é o mesmo par documentado em `api/openapi/MATRIZ.md:44` para a imagem |
| sistema de ficheiros / bytes | **SIM** | o corpo devolvido pela descarga é verificável: `RIFF`/`WEBP`, presença de `VP8 `/`VP8L`/`VP8X`, e 512×512 |
| evento / webhook / WS | **SIM**, no receptor | numa segunda sessão, o autocolante chega como `*events.Message` com `stickerMessage` |
| sessão receptora | **SIM** | `GET /chats/history` da sessão B |
| ack | parcial | o `200` já traz `message_id` e `status: sent`; prova envio, não prova WebP válido |
| metadados / contador / timestamp / estado / listagem | não aplicáveis | — |

## O observador, escrito como asserção

```
POST /chats/send/sticker   -> 200 {message_id: M}
GET  /chats/history?chat_jid=<destino>
     -> elemento com message_id == M, message_type == "sticker",
        data_json.Message.stickerMessage presente
POST /chats/downloadsticker  (descritor tirado desse data_json)
     -> bytes começando por "RIFF"…"WEBP", com chunk VP8/VP8L/VP8X e 512×512
```

Isto é verificação de **bytes**, não de olho humano. O que a bateria de campo
fez foi olhar para a bolha; o observador programático estava disponível e não
foi usado.

## Pré-condição

Fixture válido. E o fixture **não precisa de ser um autocolante real**: basta
um **PNG 512×512** em URI de dados, porque `image/png` cai no ramo que corre
`ffmpeg -vf scale=512:512 -c:v libwebp -lossless 1`
(`sticker.go:92-96`). Gerável em três linhas com `image/png` da biblioteca
padrão. `ffmpeg` está presente nesta máquina (`/opt/homebrew/bin/ffmpeg`); em
produção é dependência de sistema e a sua ausência dá
`500 sticker_conversion_failed`.

**Veredicto: `SÓ_FALTA_FIXTURE`** — o fixture é um PNG 512×512, e o observador
(descarga + verificação dos bytes) já existe e é ✅. Falta só a conta
emparelhada para correr o ciclo. Procedimento em `HUMAN-LAST.md` B.1.

---

# 3. `POST /groups/{group_jid}/join-requests`

**Motivo antigo**: *"200 sem fila de pedidos pendente para observar o efeito."*
Correcto — e o essencial é que a fila **é criável inteiramente pela nossa
própria API**, sem tocar em nenhuma interface.

## Inventário de observadores

| candidato | existe? | prova |
|---|---|---|
| endpoint relacionado | **SIM, é o natural** | `GET /groups/{group_jid}/join-requests` (✅). Aprovar/rejeitar retira o solicitante da lista — transição de estado, não repetição do pedido |
| segundo `GET`/`POST` de leitura | **SIM** | `POST /groups/info` (✅) passa a listar o aprovado em `Participants` |
| corpo da própria resposta | **existe no protocolo, e é DEITADO FORA** | `client.UpdateGroupRequestParticipants` devolve `[]types.GroupParticipant` com `Error` por solicitante, e o adaptador descarta: `_, err = client.UpdateGroupRequestParticipants(...)` em `pkg/infra/wa-noise/adapters/group/participants.go:81`. A rota irmã `POST /groups/{group_jid}/participants` **devolve** essa lista (`api/openapi/paths/grupo.yaml:1324-1337`). Registado como **F280** |
| evento | **SIM** | `*events.GroupInfo` é tratado (`pkg/bootstrap/eventhandler.go:132`) e a aprovação muda a composição do grupo |
| webhook / WS | **SIM** | mesmo evento, mesmo `postmap` |
| sessão receptora | **SIM** | a sessão B, aprovada, passa a ver o grupo em `POST /groups/list` |
| base de dados | **não** | pedidos pendentes não são persistidos localmente |
| metadados / contador / timestamp / ack / recibo / ficheiros | não aplicáveis | — |

## Pré-condição — e é toda criável por API

`GET /groups/{group_jid}/join-requests` "só tem conteúdo em grupos com
`IsJoinApprovalRequired` verdadeiro" (`api/openapi/paths/grupo.yaml:1088-1090`),
e as três chamadas da bateria devolveram `[]` porque nenhum grupo o exigia
(`grupo.yaml:1150-1153`). **Um `200` contra fila vazia não prova nada** —
concordo com a classificação.

O que provei ao ler o protocolo é que a fila se enche sem interface nenhuma.
`JoinWithLink` (`internal/wa-noise/capabilities/group/invite.go:102-125`)
verifica primeiro o nó `membership_approval_request`:

```go
membershipApprovalModeNode, ok := resp.GetOptionalChildByTag("membership_approval_request")
if ok {
    return membershipApprovalModeNode.AttrGetter().JID("jid"), nil
}
```

Quer dizer: entrar por link num grupo com aprovação exigida **produz um pedido
pendente**, e devolve `200`. Logo a sequência abaixo é 100% HTTP:

```
A: POST /groups/create              -> G
A: PUT /groups/{group_jid}/settings/join-approval    {groupjid: G, mode: true}
A: POST /groups/invitelink          -> L
B: POST /groups/join                {code: L}       ← cria o pendente
A: GET  /groups/{group_jid}/join-requests ?group_jid=G     ← ANTES: contém B
A: POST /groups/{group_jid}/join-requests {groupJID:G, Phone:[B-JID-completo], Action:"approve"}
A: GET  /groups/{group_jid}/join-requests ?group_jid=G     ← DEPOIS: já não contém B
A: POST /groups/info                {groupJID: G}    ← B aparece em Participants
```

Duas armadilhas já documentadas e que fazem esta sequência falhar se ignoradas:
`Phone` exige **JID completo** aqui (número nu dá `400 invalid_phone`,
`grupo.yaml:1396-1401`), e `Action` é `approve`/`reject`, não `add`/`remove`.

**Veredicto: `OBSERVADOR_EXISTE`** — `GET /groups/{group_jid}/join-requests`, com
transição medida antes/depois, mais `POST /groups/info` como segunda
confirmação. Exige duas sessões emparelhadas; nada mais.
Procedimento em `HUMAN-LAST.md` C.1.

---

# 4. `POST /newsletters/mark-viewed`

**Motivo antigo**: *"200 com `data:null`; não há leitura que confirme a
marcação."* ⚠️ **MOTIVO ANTIGO ERRADO na forma, certo no efeito prático** — há
leitura que confirma, ela existe por desenho, e está **inoperante por outro
achado nosso**. É diferente de não existir.

## O que o protocolo diz (Nível A, do próprio código vendorizado)

`internal/wa-noise/core/newsletter.go:52-53`:

> `NewsletterMarkViewed` marks a channel message as viewed, **incrementing the
> view counter**.

`internal/wa-noise/core/newsletter.go:206-208`:

> `GetNewsletterMessageUpdates` gets updates in a WhatsApp channel. These are
> the same kind of updates that `NewsletterSubscribeLiveUpdates` triggers
> (**reaction and view counts**).

Ou seja: o observador desenhado para esta rota é o par
`updates` / `subscribe`. Não é dedução minha — está escrito ao lado da função.

## Inventário de observadores

| candidato | existe? | prova |
|---|---|---|
| `POST /newsletters/updates` — o observador DESENHADO | **existe e está INOPERANTE** | é a rota ❌ da bateria: `500` ao fim de 30,0 s, `context deadline exceeded`, achado **F265**. A hipótese aberta lá é query ID desactualizado, e o `HOUSEKEEP` da biblioteca já tem precedente disso na F32 (duas query IDs de newsletter erradas no upstream) |
| `POST /newsletters/subscribe` + evento — o MESMO dado por outra via | **SIM, e não passa pela rota partida** | `NewsletterSubscribeLiveUpdates` entrega `*events.NewsletterLiveUpdate`, que carrega `Messages []*types.NewsletterMessage` (`internal/wa-noise/protocol/types/events/newsletter.go:23-27`) — logo `ViewsCount` e `ReactionCounts`. Nós tratamos o evento (`pkg/bootstrap/eventhandler_group.go:44-48`) e ele vai para webhook e `/session/ws` com `postmap["event"]` inteiro |
| `POST /newsletters/messages` → `ViewsCount` | **existe e NÃO DISTINGUE** | o campo existe (`internal/wa-noise/protocol/types/newsletter.go:149`) e chega ao JSON. Mas foi medido **`0` nas sessenta publicações**, incluindo as que tinham dezenas de reações (`api/openapi/schemas/canal.yaml:1035-1043`). Um observador que mostra zero em todos os casos não distingue caso nenhum — é literalmente a ressalva que a F256 escreveu sobre o WhatsApp Web da conta Business |
| ack do próprio protocolo | **existe e é DEITADO FORA** | `MarkViewed` espera a resposta do servidor e descarta-a: `// TODO handle response?` seguido de `<-resp` em `internal/wa-noise/capabilities/newsletter/actions.go:71-72`. Registado como **LIB-04** em `internal/wa-noise/HOUSEKEEP.md` |
| base de dados | **não** | nada de newsletter é persistido em `message_history` por esta via |
| sessão receptora | **não** | vista é acto do LEITOR; não há segunda ponta a notificar |
| segundo `GET` / listagem / estado | **não** | `POST /newsletters/info` devolve metadados do canal, não estado de leitura por mensagem |
| recibo de leitura | **não** | o `MarkRead` normal é outro mecanismo, e a própria doc do vendor separa os dois (`newsletter.go:54`) |
| metadados / contador / timestamp / ficheiros | não aplicáveis | — |

## Pré-condição

Duas, e a segunda é a que morde:

1. Um canal com pelo menos uma publicação e um `server_id` real — satisfeita:
   há canal medido com sessenta (`canal.yaml:973-976`).
2. **A contagem de vistas tem de ser visível para a conta que observa.** Nos
   sessenta medidos — canal público que a conta apenas SEGUE — veio sempre
   `0`. A leitura honesta é que o contador não é servido a um seguidor comum.
   Não medi um canal PRÓPRIO, e é aí que a promoção se decide.

**Veredicto: `OBSERVADOR_EXISTE`, hoje inalcançável.** O observador desenhado
(`/newsletters/updates`) está partido pela F265; o alternativo
(`/newsletters/subscribe` + evento) está inteiro no código e nunca foi
exercitado. A rota **fica 🟡**, mas por motivo preciso: *"o observador é
`ViewsCount`, servido por `/newsletters/updates` (inoperante, F265) ou por
`/newsletters/subscribe` + evento; `POST /newsletters/messages` traz o campo mas
mediu `0` em 60/60 e não distingue"*. Procedimento em `HUMAN-LAST.md` B.2.

---

# 5. `POST /newsletters/react`

**Motivo antigo**: *"200 com `data:null`; o canal de teste não tinha mensagem
para observar a reacção."* ⚠️ **MOTIVO ANTIGO ERRADO, e obsoleto no mesmo
dia** — no dia da bateria mediu-se um canal com **sessenta** publicações, e o
observador não só existe como já devolveu contagens não-nulas.

## Inventário de observadores

| candidato | existe? | prova |
|---|---|---|
| `POST /newsletters/messages` → `ReactionCounts` | **SIM, e já observou dado real** | `types.NewsletterMessage.ReactionCounts map[string]int` (`internal/wa-noise/protocol/types/newsletter.go:150`), servido directamente em `data`. Medido, com valores: o exemplo publicado em `api/openapi/paths/canal.yaml:543` é `ReactionCounts: {'👍': 3, '😂': 58}`, e o schema diz "sempre presente e nunca `null` nas sessenta medidas" (`api/openapi/schemas/canal.yaml:1051-1053`) |
| `POST /newsletters/updates` | existe, **inoperante** | F265, o mesmo `500` de sempre |
| `POST /newsletters/subscribe` + evento | **SIM** | `*events.NewsletterLiveUpdate.Messages[].ReactionCounts`, via webhook/`/session/ws` |
| corpo da própria resposta | **não** | `data` é `null`; `SendNewsletterReaction` devolve só `error` (`pkg/infra/wa-noise/adapters/misc/adapter.go:375-387`) |
| ack | **não** | `SendReaction` faz `t.SendNode` e devolve; não espera resposta (`internal/wa-noise/capabilities/newsletter/actions.go:99-112`) — ao contrário de `MarkViewed`, que ao menos espera |
| base de dados / histórico | **não** | reacções de canal não entram em `message_history` |
| sessão receptora | **SIM, opcional** | a segunda sessão, seguindo o mesmo canal, lê o mesmo `ReactionCounts` — mas não é preciso: a contagem é global e a primeira sessão já a lê |
| listagem / estado / metadados / ficheiros | não aplicáveis | — |

## O observador, escrito como asserção

```
POST /newsletters/messages {jid: C, count: N}   -> ReactionCounts[E] == k   (ANTES)
POST /newsletters/react    {jid: C, serverID: S, reaction: E}  -> 200
POST /newsletters/messages {jid: C, count: N}   -> ReactionCounts[E] == k+1 (DEPOIS)
POST /newsletters/react    {jid: C, serverID: S, reaction: ""} -> 200   (remove)
POST /newsletters/messages {jid: C, count: N}   -> ReactionCounts[E] == k   (VOLTOU)
```

O terceiro passo é o que faz disto observação de causa e não de sintoma:
**pôr e tirar**, e ver o contador voltar. Um teste que só visse o incremento
passaria também com uma reacção de outra pessoa a chegar no mesmo segundo.

Duas armadilhas medidas e obrigatórias na asserção:

- `count` é **obrigatório de facto**: ausente, `0`, `null` ou negativo dá
  `500 newsletter_failed` ao fim do tempo limite (`canal.yaml:483-503`).
  A leitura ANTES/DEPOIS tem de o levar sempre.
- **As chaves de emoji não são normalizadas**: `"❤"` e `"❤️"` são entradas
  SEPARADAS (`api/openapi/schemas/canal.yaml:1055-1059`). Compare a chave
  exacta que enviou, byte a byte, ou compara a coisa errada.

**Nível C — Baileys.** Expõe `newsletterReactMessage(jid, serverId, '❤️')` com
remoção por valor vazio/`null`, e `newsletterFetchMessages` como leitura — a
MESMA topologia: escrita por uma chamada, observação pela listagem. Confirma
que ler as contagens da listagem é o caminho normal do protocolo, e não um
truque nosso.

**Veredicto: `OBSERVADOR_EXISTE`, e é o mais barato das oito.** Uma única
sessão emparelhada e um canal com uma publicação bastam. O que falta é
autorização humana: reagir a uma publicação de um canal público é acto
visível numa conta real. Procedimento em `HUMAN-LAST.md` B.3.

---

# 6, 7 e 8. `POST /status/set/image`, `/status/set/video`, `/status/set/audio`

**Motivo antigo**: *"200 com `message_id`; a lista de destinatários resolve
para 2 numa conta Business (F256)"* e *"idem — herdada"*. Correcto, e
**incompleto num ponto que decide a promoção**: a F256 é uma pré-condição, não
uma ausência de observador. O observador existe.

As três partilham tudo: a mesma pré-condição, o mesmo observador, o mesmo
bloqueio. `/status/set/image` chama `media.SendImage` para
`domain.StatusBroadcastJID` (`pkg/application/usecase/status/publish_image.go:77`),
e as outras duas o análogo — mudam os bytes, não o caminho.

## Inventário de observadores

| candidato | existe? | prova |
|---|---|---|
| **sessão receptora** — o observador real | **SIM** | um status entregue chega à sessão B como `*events.Message` com `Info.Chat == status@broadcast`. Que ISTO acontece está medido: a F256 regista o processo a anotar a chegada de vários "… in `status@broadcast`" |
| histórico da sessão B | **SIM** | `saveMessageHistory` grava por `evt.Info.Chat.String()` (`pkg/bootstrap/eventhandler_message.go:320`), logo `GET /chats/history?chat_jid=status@broadcast` na sessão B lista o status publicado por A |
| webhook / WS da sessão B | **SIM** | evento `type: "Message"` com o `event` inteiro |
| histórico da sessão A (a que publica) | **existe e NÃO SERVE** | `recordOutgoing(..., "image", ...)` grava o envio (`pkg/infra/wa-noise/adapters/chat/messenger.go:357`), mas é a NOSSA escrita a confirmar a nossa escrita. Prova que a API enviou, não que o WhatsApp publicou — que é exactamente o que o `200` já dizia, e exactamente o que a F256 mostrou ser falso |
| `GET /users/contacts` — observador da PRÉ-CONDIÇÃO | **SIM, e é o achado prático** | expõe `FullName` por contacto, e é o campo que decide a lista de destinatários. Permite verificar ANTES de publicar se a observação é sequer possível |
| segundo `GET` / endpoint de leitura de status | **NÃO EXISTE** | não há `GET /status`. As três rotas do grupo Status são só de escrita — confirmado na tabela de `docs/OPENAPI-EVIDENCIAS.md`, grupo "Status": 3 rotas, 0 ✅ |
| `message_id` devolvido | **não é resolúvel** | não há rota que aceite um `message_id` e devolva a mensagem. `POST /chats/downloadimage` e irmãs consomem o DESCRITOR do `data_json`, não o id (`api/openapi/paths/conversa.yaml:1239`) |
| ack / recibo de leitura | **não** | o `200` traz `status: sent`, que é ack de envio |
| base de dados (a de A) | como acima | mesma objecção |
| contador / metadados / timestamp / ficheiros | **não** | — |
| interface do WhatsApp Web | **desqualificado, e por medição** | a F256 já o tentou e registou a ressalva: o WhatsApp Web daquela conta não mostra status de NINGUÉM. "Um observador que mostra zero em todos os casos não distingue caso nenhum" |

## A pré-condição, que é a F256 inteira

`getStatusBroadcastRecipients` (`internal/wa-noise/core/broadcast.go:70`) só
inclui um contacto se ele tiver `FullName`:

```go
// TODO should there be a better way to separate contacts and found push names in the db?
if len(contact.FullName) > 0 {
    contactsArray = append(contactsArray, jid)
}
```

Medido e contado na F256:

| sessão | contactos | com `full_name` | destinatários |
|---|---|---|---|
| `filarapida` (Business) | 461 | **1** | 2 (esse + self) |
| `lucas` (pessoal) | 1266 | 164 | 165 |

**A consequência para esta campanha**: a sessão B só observa o status de A se
estiver na lista de A — isto é, se B tiver `FullName` não vazio no roster de
A. E isso é verificável por API **antes** de publicar seja o que for:
`GET /users/contacts` na sessão A, procurar o JID de B, confirmar `FullName`
não vazio. A própria documentação já traz a contagem 1 vs 164
(`api/openapi/schemas/contacto.yaml:375`) e já liga a escassez de `FullName` à
publicação de status (`:386`).

Pela conta `filarapida`, a pré-condição **está provadamente por satisfazer**:
o único destinatário externo é `5541992421234@s.whatsapp.net`, forma com nono
dígito que a F256 suspeita não corresponder a conta nenhuma.

**Veredicto (as três): `OBSERVADOR_EXISTE`** — a sessão receptora, lida por
`GET /chats/history?chat_jid=status@broadcast` ou pelo webhook/WS dela. Não é
`SEM_OBSERVADOR_PROGRAMATICO`: o que falta é a pré-condição da F256, e ela é
verificável por API antes de gastar uma publicação.
Procedimento em `HUMAN-LAST.md` C.2.

---

# Quadro final

| rota | veredicto | observador | o que falta |
|---|---|---|---|
| `POST /chats/request-unavailable-message` | `OBSERVADOR_EXISTE` | `data_json.UnavailableRequestID` em `GET /chats/history`; webhook; `/session/ws` | mensagem genuinamente indecifrável |
| `POST /chats/send/sticker` | `SÓ_FALTA_FIXTURE` | `GET /chats/history` → `POST /chats/downloadsticker` → bytes WebP 512×512 | PNG 512×512 (gerável) + conta emparelhada |
| `POST /groups/{group_jid}/join-requests` | `OBSERVADOR_EXISTE` | `GET /groups/{group_jid}/join-requests` antes/depois; `POST /groups/info` | duas sessões; a fila é criável só por API |
| `POST /newsletters/mark-viewed` | `OBSERVADOR_EXISTE`, inalcançável | `ViewsCount` via `/newsletters/updates` (❌ F265) ou `/newsletters/subscribe` + evento | corrigir a F265, ou exercitar a via de evento; e um canal onde o contador seja servido |
| `POST /newsletters/react` | `OBSERVADOR_EXISTE` | `ReactionCounts` em `POST /newsletters/messages`, antes/depois/remoção | uma sessão + autorização para reagir em canal real |
| `POST /status/set/image` | `OBSERVADOR_EXISTE` | `GET /chats/history?chat_jid=status@broadcast` na sessão B | pré-condição da F256, verificável por `GET /users/contacts` |
| `POST /status/set/video` | idem | idem | idem |
| `POST /status/set/audio` | idem | idem | idem |

**Promoções a ✅ nesta sessão: zero.** Todas as oito exigem conta emparelhada,
que esta sessão não tem, e forçar a promoção sem observador é proibido.
**Motivos precisados: oito de oito**, quatro deles a corrigir afirmação hoje
escrita como facto.

**Achados abertos**: F279 (WebP não é convertido, e a documentação diz que
sim), F280 (resultado por participante deitado fora), LIB-04 do
`internal/wa-noise` (ack de `MarkViewed` descartado).
