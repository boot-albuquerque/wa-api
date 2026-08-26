# Investigação — `POST /newsletters/updates` (F265)

**Data**: 2026-08-26. **Estado**: causa DETERMINADA.
**Classificação final**: `PROTOCOL_CHANGED`.

**Nota de nomes**: a medição de campo correu contra a forma histórica
`POST /newsletter/updates`. O contrato canonicalizou os caminhos depois disso;
o nome actual é `POST /newsletters/updates`, que este documento usa. É a mesma
implementação, servida noutro caminho.

---

## Sintoma

```
POST /newsletters/updates {"jid":"1203…@newsletter","count":5}
  -> 500 newsletter_failed   (duração 30001 ms)
log: ERR newsletter operation failed error="context deadline exceeded" op=updates
```

Os 30 s são exactamente `waclient.RequestTimeout`. O servidor **não responde** —
não é ele a recusar. `POST /newsletters/messages` sobre o MESMO canal e no mesmo
segundo devolve `200 []`.

É a única das dezoito rotas de newsletter que falha.

## Ambiente

Transporte **wa-noise** (socket). Sessão viva, canal válido.

## Tipo de conta

A medição foi feita numa conta; a rota falha independentemente do conteúdo do
canal. Nada na causa determinada depende do tipo de conta.
Etiqueta: **BOTH_SUPPORTED** — para a capacidade de produto ("ler mensagens de
um canal com contadores de visualização e reacção"), que funciona pela forma
correcta do stanza. A forma que usamos hoje não funciona para ninguém.

## Pré-condições

Sessão ligada, canal existente e acessível — provado por `/newsletters/messages`
e `/newsletters/info` a devolverem `200` no mesmo período.

---

## Decomposição do caminho

| etapa | ficheiro:linha | veredicto |
|---|---|---|
| validação local | handler `ch.Newsletter.Updates`, `pkg/bootstrap/wiring_routes.go:191` | passa |
| transformação | `pkg/infra/wa-noise/adapters/misc/adapter.go:346-356` `clientAndJID` + `GetNewsletterUpdatesParams` | passa |
| timeout local | `adapter.go:352` `context.WithTimeout(ctx, waclient.RequestTimeout)` = 30 s | é ele que corta |
| biblioteca | `internal/wa-noise/capabilities/newsletter/messages.go:89-105` `GetMessageUpdates` | monta o stanza |
| **pedido upstream** | IQ `get`, xmlns `newsletter`, **`to=<canal>@newsletter`**, filho `<message_updates>` | enviado |
| **resposta upstream** | **nenhuma** | o servidor ignora |
| tradução HTTP | `context deadline exceeded` → `500 newsletter_failed` | correcta para um timeout |

### O pedido que montamos hoje

`internal/wa-noise/capabilities/newsletter/messages.go:89-97`:

```go
resp, err := t.SendIQ(ctx, IQ{
    Namespace: Namespace,          // "newsletter"
    Type:      IQGet,
    To:        jid,                // <- O CANAL, não o servidor
    Content: []waBinary.Node{{
        Tag:   messageUpdatesTag,  // "message_updates"
        Attrs: messageUpdatesAttrs(params),   // count / since / after
    }},
})
```

No fio, para `{"count":5}`:

```xml
<iq to='1203…@newsletter' type='get' xmlns='newsletter'>
  <message_updates count='5'/>
</iq>
```

### O pedido irmão que FUNCIONA

`internal/wa-noise/capabilities/newsletter/messages.go:36-45` (`GetMessages`,
que serve `/newsletters/messages`, medido `200`):

```xml
<iq to='s.whatsapp.net' type='get' xmlns='newsletter'>
  <messages type='jid' jid='1203…@newsletter' count='5'/>
</iq>
```

O comentário em `messages.go:82-88` já registava a assimetria — *"herdada do
upstream e preservada"* — sem saber que ela era o defeito.

### A resposta que esperamos

`<message_updates><messages>…` (`messages.go:100`). Nunca chega nada.

---

## Árvore de hipóteses

| # | hipótese | veredicto |
|---|---|---|
| H1 | Pedido malformado | **ELIMINADA** — o stanza é byte-a-byte o que o Baileys envia em `newsletterFetchMessages` e o que o whatsmeow envia em `GetNewsletterMessageUpdates`. Se estivesse malformado, estaria malformado nas três. E um stanza malformado dá `400`, não silêncio. |
| H2 | Namespace errado | **ELIMINADA** — `newsletter` é o namespace de mais 17 rotas medidas a `200`. |
| H3 | Atributos em falta | **ELIMINADA como causa, mas com divergência real registada** — Baileys emite sempre `since` (`if (typeof since === 'number')`, e `0` é number); nós omitimos quando zero (`messages.go:70`). A divergência existe, mas é irrelevante: a referência que MEDIU descobriu que o stanza inteiro está morto, com ou sem `since`. |
| H4 | Formato de JID errado | **ELIMINADA** — o mesmo `1203…@newsletter` é aceite por `/newsletters/messages`, `/newsletters/info` e `/newsletters/subscribe` no mesmo segundo. |
| H5 | **Resposta esperada no sítio errado (evento assíncrono em vez de resposta)** | **ELIMINADA** — era a hipótese mais plausível dado o timeout, e cai por medição própria: `POST /newsletters/subscribe` faz um IQ `set` para o MESMO `@newsletter` JID (`internal/wa-noise/capabilities/newsletter/actions.go:14-27`) e recebe resposta síncrona, medida ✅. Portanto o servidor ROTEIA e RESPONDE a IQs endereçados a um canal. O que ele não responde é este `<message_updates>` em particular. |
| H6 | Estado de conta incompatível | **ELIMINADA** — as outras 17 rotas de newsletter passam na mesma conta e sessão. |
| H7 | Bug da biblioteca | **ELIMINADA como causa primária** — o nosso código é idêntico ao whatsmeow `main`, cuja `GetNewsletterMessageUpdates` não é tocada desde **2023-10-18**. Não divergimos: herdámos. |
| H8 | **Mudança de protocolo** | **CONFIRMADA** — a forma `<message_updates>` para o canal é de 2023 e o servidor deixou de a atender. A forma que o WA Web usa hoje é `<messages type='jid'>` para `s.whatsapp.net`. |
| H9 | Regra de negócio (Business vs Personal) | **ELIMINADA** — nada nas referências ou no sintoma liga a falha ao tipo de conta, e a rota irmã funciona na mesma conta. |

**Hipótese do HOUSEKEEP, e a sua queda:**

| # | hipótese | veredicto |
|---|---|---|
| H10 | *"o query ID do `updates` está desactualizado"* (F265, hipótese registada e NÃO medida) | **ELIMINADA por leitura de código** — `GetMessageUpdates` **não é uma query MEX**. É um IQ binário simples com namespace `newsletter` (`messages.go:89-97`); não passa por `queryids.go`, não tem query ID nenhum. Correr o extractor de `scripts/mex-query-ids/` não teria produzido informação sobre esta rota. A entrada da F265 tem de ser corrigida — um achado com diagnóstico errado é pior que nenhum. |

---

## Referências consultadas — o que cada uma respondeu

### Baileys — **RESPONDEU A CAUSA, E COM O STANZA MEDIDO**

Duas peças, e as duas são precisas.

**1. A issue aberta, com o nosso sintoma exacto.**
[#2555](https://github.com/WhiskeySockets/Baileys/issues/2555), **aberta em
2026-05-13 e AINDA ABERTA**: *"[BUG] - newsletterFetchMessages returns no
message, and timeouts"*. O relator diz, textualmente:

> I've tried various invite codes and jids that are valid and from active
> channels (that the session is subscribed to or unsubscribed). […] probeChunk
> is always empty here

com o log:

```
{"level":40,…,"msg":"timed out waiting for message"}
```

Tentou com e sem `since`/`after`, com janelas válidas, invertidas, e a seguir a
um `newsletterFollow`. Nenhuma variação de atributo salvou. **É por isto que H3
cai como causa.**

**2. O PR que capturou a forma certa do WA Web.**
[#2620](https://github.com/WhiskeySockets/Baileys/pull/2620), *"fix(newsletter):
parse metadata and fetch-messages responses"*, aberto 2026-06-05. O corpo diz,
textualmente:

> `newsletterFetchMessages`: send the WA-Web-shaped query
> `<iq to='s.whatsapp.net' xmlns='newsletter'><messages type='jid' jid count/></iq>`
> instead of `<message_updates>` addressed to the channel jid, **which timed
> out** […] Fixes #2555.
>
> All changes were validated end-to-end against live WhatsApp. […] fetch returns
> real messages with decoded `proto.Message` content, `reactions`, and
> `forwards` counts (both previously timed out or returned `null`).

**Estado do PR: fechado em 2026-07-19 pelo bot de stale**, depois de marcado
stale em 2026-06-20 por 14 dias sem actividade. Não foi rejeitado, nem
refutado, nem revisto. Isto importa para graduar a evidência: é um PR com
medição declarada em produção que nunca teve revisão.

### rsalcara/InfiniteAPI (fork/derivado do Baileys) — **RESPONDEU: portou e mergeou**

[PR #503](https://github.com/rsalcara/InfiniteAPI/pull/503), *"fix(newsletter):
parse metadata and fetch-messages responses (#2620 port)"*, **MERGEADO em
2026-06-06** e lançado no mesmo dia (PR #508, release develop→master). O corpo
do port descreve a causa em português e com mais detalhe que o original:

> Stanza antiga: `<iq to='<channel-jid>' xmlns='newsletter'><message_updates count since after/></iq>`
> **O servidor ignora — o request fica pendurado até o caller dar timeout.**
>
> Stanza correcta (capturada do WA Web):
> `<iq to='s.whatsapp.net' xmlns='newsletter'><messages type='jid' jid='<channel-jid>' count='N' [before/after]/></iq>`
>
> **O atributo de cursor mudou de `since` para `before`**

E acrescenta o que volta na resposta: `<messages>` com filhos `<message>`, cada
um com `views`/`forwards`/`responses`, `editTimestamp`, `originalTimestamp`,
`mediaRcat`, `reactions[]`, `pollVotes[]` e o `proto.Message` do `<plaintext>`.

**É a peça que fecha o caso**: os contadores de visualização e reacção — a
razão de existir da rota `/newsletters/updates` — vêm DENTRO da resposta de
`<messages>`. Não há duas queries; há uma, e nós temo-la a funcionar.

### whatsmeow — **RESPONDEU "eu também não resolvo"**

`GetNewsletterMessageUpdates` no `main` é **idêntico** ao nosso, incluindo o
`To: jid`. O `newsletter.go` não é tocado nesta função desde `13721155ca`
(2023-10-18); os commits posteriores são infra (`coder/websocket`, argo,
staticcheck, strip query params).

Issues: **uma só**, [#761](https://github.com/tulir/whatsmeow/issues/761)
(2025-02-28), *"GetNewsletterMessageUpdates return nil Message"* — o relator
obteve `Timestamp` e `Message` nulos. **Fechada em 2025-07-01 sem comentários e
sem correcção.** Note o sintoma diferente: em 2025 ainda chegava RESPOSTA (com
campos nulos); em 2026 não chega nada. É um dado a favor de uma retirada
progressiva do lado do servidor, e é o mais perto de uma linha temporal que
esta pesquisa produziu.

Conclusão sobre esta referência: **o whatsmeow não sabe que isto está partido.**
Copiar o caminho dele teria falhado — que é exactamente o cenário que o
CLAUDE.md descreve na secção "A resposta NEGATIVA também é informação".

### whatsapp-web.js — **RESPONDEU "isto não é comigo"**

O wwebjs dirige a SPA e não emite stanzas. Não tem equivalente a
`newsletterFetchMessages`: as mensagens de canal chegam-lhe pelos objectos da
própria página. Portanto não pode confirmar nem infirmar a forma do fio.

Registo como **resposta negativa útil**: a referência mais próxima do nosso
terreno em problemas de módulo/DOM é a menos útil num problema de protocolo, e
é a inversão exacta do caso do `sendText` (H34), onde foi ela que resolveu.

Nota lateral: a pesquisa de issues do repositório `pedroslopez/whatsapp-web.js`
pela API do GitHub foi recusada sem autenticação. **Não consultei as issues
abertas dele nesta rota** — registo a lacuna em vez de a inventar.

### Evolution API — **NÃO RESPONDEU (é consumidor)**

Expõe canais por cima do Baileys; herda `newsletterFetchMessages` na versão que
fixar. A pesquisa de código no repositório exigiu autenticação. Não é uma
opinião independente.

### open-wa / wa-automate — **NÃO CONSULTADA**

Não foi consultada. Registo a lacuna.

### devlikeapro/waha — **RESPONDEU, de lado**

[Issue #2150](https://github.com/devlikeapro/waha/issues/2150), aberta
2026-07-08: *"Channels: viewCount is never returned on any engine (messages
preview), though docs promise it"*. **Em NENHUM engine** — o que é consistente
com "a query que traz os contadores deixou de responder para toda a gente".
Evidência circunstancial, não decisiva.

---

## Documentação oficial Meta/WhatsApp

**Não existe.** O protocolo binário do WhatsApp Web não é documentado
publicamente, e os Canais (newsletters) não têm superfície na Cloud API — nem
para ler mensagens, nem para contadores de visualização.

Consequência para esta investigação: **não há evidência de Nível A possível**, e
o teto é Nível C/D. Nenhuma afirmação aqui deve ser lida como "a Meta diz".

> **Correcção posterior — a ausência na Cloud API não é evidência de nada.**
>
> `docs/REFERENCIA-META-OFICIAL.md` regista que os canais/newsletters existem
> aqui em **18 rotas** e têm **"não"** na Cloud API — e que isso vale para
> cerca de **60 das 141 rotas** deste projecto (grupos, comunidades, canais,
> status). As duas são **superfícies diferentes**: a Cloud API é HTTP sobre o
> Graph API; aqui fala-se o protocolo do WhatsApp Web.
>
> A ausência lá é, portanto, **esperada por desenho** — não é observação
> sobre a capacidade, e não pode apoiar `UNSUPPORTED_CONFIRMED`. A linha
> correspondente foi rebaixada de **B** para **sem nível**.
>
> A classificação **não muda**: o `PROTOCOL_CHANGED` assenta na medição de
> campo (Nível B) e no repro público do Baileys #2555 com o PR #2620 (Nível
> D), reforçados pelo port mergeado em `rsalcara/InfiniteAPI` #503 (Nível C).
> Nenhum desses elos passa pela Cloud API.

## Graduação da evidência

| afirmação | nível | fonte |
|---|---|---|
| A rota dá timeout de 30 s sem resposta | **B** | bateria de campo 2026-08-26 |
| `/newsletters/messages` responde `200` no mesmo canal e segundo | **B** | mesma bateria |
| `/newsletters/subscribe` (IQ `set` para o `@newsletter` JID) responde | **B** | mesma bateria — é o que mata H5 |
| Enviamos `<message_updates>` para o JID do canal | **B** (leitura determinística) | `messages.go:89-97` |
| O servidor ignora esse stanza | **D** | Baileys #2555 (repro público, aberto) + corpo do PR #2620 |
| A forma que o WA Web usa é `<messages type='jid'>` para `s.whatsapp.net` | **D** | PR #2620 + port #503, ambos alegando captura e validação em produção |
| A forma nova traz `reactions`/`views`/`forwards` | **D** | port #503 (lista os campos) |
| Um derivado mergeou e lançou a correcção | **C** | rsalcara/InfiniteAPI #503, mergeado 2026-06-06 |
| A hipótese do query ID é falsa | **B** | `GetMessageUpdates` não passa por `queryids.go` |
| Não há doc oficial do protocolo Web | **sem nível** | a ausência de canais na Cloud API é esperada por desenho (superfície diferente) e **não é evidência**; ver a correcção acima e `docs/REFERENCIA-META-OFICIAL.md` |

**Onde a evidência é mais fraca**: o Nível D do PR #2620. Ele alega captura do
WA Web e validação ponta-a-ponta, mas foi fechado por stale sem revisão. O que o
promove acima de "opinião" é (a) o port independente mergeado num derivado, (b) a
issue aberta com repro que ele diz corrigir, e (c) a nossa própria medição de que
a forma que ele propõe **já funciona aqui** — é literalmente o nosso
`/newsletters/messages`.

---

## Conclusão

**Classificação: `PROTOCOL_CHANGED`.**

O WhatsApp deixou de atender o IQ `<message_updates>` endereçado ao JID do
canal. Não o recusa — ignora-o, e por isso o sintoma é silêncio até ao nosso
timeout, e não um `400`. A capacidade não desapareceu: os contadores de
visualização, reacção e encaminhamento passaram a vir dentro da resposta de
`<messages type='jid'>` enviada a `s.whatsapp.net` — que é exactamente o stanza
que `/newsletters/messages` já envia com sucesso.

Não é `UNSUPPORTED_CONFIRMED`: a capacidade de produto continua a existir e a
ser servida, por outra forma. Não é `UPSTREAM_TEMPORARY`: a issue do Baileys
está aberta desde Maio/2026 e a nossa medição é de Agosto/2026, mais de três
meses depois, com o mesmo comportamento.

## Correcção (NÃO aplicada nesta sessão)

Não aplicada: a classificação não é `BUG_LOCAL`, o caminho está na biblioteca
vendorizada, e a correcção altera o CONTRATO de resposta da rota (o corpo passa
a ter os campos de contadores). Isso é decisão de produto, não de investigação.
Registada em `internal/wa-noise/HOUSEKEEP.md`.

O caminho, em três decisões que têm de ser tomadas juntas:

1. **`internal/wa-noise/capabilities/newsletter/messages.go:89`** —
   `GetMessageUpdates` passa a emitir `To: types.ServerJID` e
   `<messages type='jid' jid=… count=… [before]>`, com o cursor renomeado de
   `since` para `before` (dado do port #503).
2. **O parser** tem de ler os contadores dos filhos `<message>`; hoje
   `t.ParseMessages` devolve `[]*types.NewsletterMessage` e descarta o que a
   rota promete.
3. **A pergunta de produto**: se as duas rotas passam a emitir o MESMO stanza,
   `/newsletters/updates` e `/newsletters/messages` diferem apenas na projecção
   da resposta. Fundir, ou manter as duas com projecções distintas, é escolha
   de contrato — e tem de ser registada, não decidida de passagem.

## Teste de regressão

Não escrito, porque nada foi corrigido. Quando for:

1. **Teste do stanza**: com o `Transport` falso de
   `internal/wa-noise/capabilities/newsletter/`, afirmar que `GetMessageUpdates`
   emite `To == types.ServerJID` e um filho `messages` com `type="jid"` e
   `jid` igual ao canal. Usar o JID medido em campo (`1203…@newsletter`).
2. **Teste do cursor**: `Since` não-zero tem de sair como `before`, não como
   `since`. É a asserção que apanha um port pela metade.
3. **Controlo negativo EXECUTADO**: repor `To: jid` e `<message_updates>`, colar
   a saída da falha no HOUSEKEEP. Confirmar que a mutação COMPILA e falha com
   mensagem — mutação que não compila não prova nada (ARMADILHA #3).
4. **Dublê fiel**: o `Transport` falso tem de guardar o nó tal como o
   codificador o receberia; um dublê que só guarde a tag do filho passa com o
   `To` errado — que é precisamente o defeito.

## Evidência final

- O sintoma está reproduzido publicamente numa referência madura, em issue
  **aberta** (Baileys #2555, 2026-05-13).
- A forma correcta foi capturada do WA Web e publicada (Baileys PR #2620), e
  mergeada num derivado em produção (InfiniteAPI #503, 2026-06-06).
- A forma correcta **já funciona nesta instalação**: é o `/newsletters/messages`
  medido a `200` no mesmo canal e no mesmo segundo.
- A hipótese anterior (query ID desactualizado) está falsificada por leitura de
  código: esta rota não tem query ID.

Nenhuma medição nova foi feita nesta sessão — este ambiente não tem conta
emparelhada. Ver `HUMAN-LAST.md`, B.4 (EXP-3).

## Próxima pista

Não é preciso: a causa está determinada. Fica, no entanto, uma pergunta aberta
que só a medição responde e que muda o desenho da correcção:

> A resposta de `<messages type='jid'>` traz mesmo `views`/`reactions` para o
> canal desta conta, ou traz uma lista de mensagens sem contadores?

O port #503 diz que traz; o nosso `/newsletters/messages` devolveu `200 []`
(canal vazio), logo nunca vimos um filho `<message>` real. É B.4 (EXP-3) em
`HUMAN-LAST.md`, e tem de correr contra um canal COM mensagens.
