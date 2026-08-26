# Investigação — `POST /users/block` e `POST /users/unblock` (F264)

**Data**: 2026-08-26. **Estado**: causa DETERMINADA.
**Classificação final**: `PROTOCOL_CHANGED`.

**Nota de nomes**: a medição de campo correu contra as formas históricas
`POST /user/block` e `POST /user/unblock`. O contrato canonicalizou os caminhos
depois disso; os nomes actuais são `POST /users/block` e `POST /users/unblock`,
que este documento usa. É a mesma implementação, servida noutro caminho.

---

## Sintoma

`422 upstream_rejected` na fronteira HTTP. Por baixo, no log:

```
failed to unblock user: … info query returned status 400: bad-request
```

Medido nas quatro combinações (bateria de campo de 2026-08-26, registada na F264):

| conta | destinatário | resultado |
|---|---|---|
| `filarapida` (Business) | `5511999999999@s.whatsapp.net` (inexistente) | `422 upstream_rejected` |
| `filarapida` | `554192421234@s.whatsapp.net` (real) | `422 upstream_rejected` |
| `filarapida` | `90937376170214@lid` (o LID do mesmo) | `422 upstream_rejected` |
| `lucas` (pessoal) | `5511987654321@s.whatsapp.net` | `422 upstream_rejected` |

`GET /users/blocklist` responde `200`. Só a ESCRITA é recusada.

## Ambiente

Transporte **wa-noise** (socket / protocolo binário). O `info query returned
status 400` é a assinatura do `sendIQ` desta biblioteca; o transporte
**wa-headless** (SPA) não produz esta mensagem.

## Tipo de conta

Testado nas duas — Business (`filarapida`) e pessoal (`lucas`) — com resultado
IDÊNTICO. O tipo de conta **não** é o discriminador.
Etiqueta: **BOTH_SUPPORTED** (a capacidade existe nas duas; a recusa é anterior
a qualquer regra de conta).

## Pré-condições

Sessão ligada e autenticada — provado pelo `GET /users/blocklist` a devolver
`200 {"Blocklist":[],"DHash":"…"}` no mesmo período.

---

## Decomposição do caminho

| etapa | ficheiro:linha | veredicto |
|---|---|---|
| validação local | `pkg/presentation/http/handlers/` (rota `POST /users/block`) | passa — chega ao adaptador |
| transformação (1) | `pkg/infra/wa-noise/adapters/user/blocklist.go:40` `wajid.ToJID` | passa |
| transformação (2) | `pkg/infra/wa-noise/adapters/user/blocklist.go:43` `normalizeBlocklistJID` | passa |
| **transformação (3)** | `pkg/infra/wa-noise/adapters/user/blocklist.go:50` `resolveBlocklistPNJID` | **converte LID → PN** |
| biblioteca | `internal/wa-noise/core/user_queries.go:43` `Client.UpdateBlocklist` | delega |
| **stanza** | `internal/wa-noise/capabilities/user/blocklist.go:50-65` | **monta a forma antiga** |
| pedido upstream | IQ `set`, xmlns `blocklist`, `to=s.whatsapp.net` | enviado |
| resposta upstream | `400 bad-request` | **recusa explícita** |
| tradução HTTP | `pkg/infra/wa-noise/errmap.ClassifyIQ` → `422 upstream_rejected` | correcta (F204) |

### O pedido que montamos hoje

`internal/wa-noise/capabilities/user/blocklist.go:54-65`:

```go
resp, err := t.SendIQ(ctx, IQ{
    Namespace: blocklistIQNamespace,   // "blocklist" (constants.go:7)
    Type:      IQSet,
    To:        types.ServerJID,        // s.whatsapp.net
    Content: []waBinary.Node{{
        Tag: "item",
        Attrs: waBinary.Attrs{
            "jid":    jid,              // <- o que o adaptador entregou
            "action": string(action),   // "block" | "unblock"
        },
    }},
})
```

Na prática, para as quatro linhas medidas:

```xml
<iq to='s.whatsapp.net' type='set' xmlns='blocklist'>
  <item jid='554192421234@s.whatsapp.net' action='unblock'/>
</iq>
```

### A transformação que torna as quatro linhas na MESMA linha

`pkg/infra/wa-noise/adapters/user/blocklist.go:103-116`:

```go
func resolveBlocklistPNJID(ctx context.Context, client waclient.Client, jid types.JID) (types.JID, error) {
	switch jid.Server {
	case types.DefaultUserServer:  return jid, nil          // PN passa
	case types.HiddenUserServer:   … GetPNForLID …          // LID VIRA PN
	}
}
```

**Isto explica por que "testado em PN e em LID" deu o mesmo resultado**: as duas
entradas produzem o MESMO stanza no fio, com `jid` em `@s.whatsapp.net`. A
medição de campo não distinguiu as duas formas — a nossa própria conversão
apagou a diferença antes de chegar ao socket. Este é exactamente o caso da
ARMADILHA #1: o teste (aqui, a medição) não estava a medir a variável que
julgava medir.

### A resposta que esperamos

Um `<list dhash='…'>` com filhos `<item jid='…'/>`
(`internal/wa-noise/capabilities/user/blocklist.go:69-73`). Nunca chega: o
servidor responde `400 bad-request` ANTES.

---

## Árvore de hipóteses

| # | hipótese | veredicto |
|---|---|---|
| H1 | Pedido malformado (XML/binário inválido) | **ELIMINADA** — o mesmo `SendIQ`, o mesmo codificador e o mesmo namespace servem o `GET /users/blocklist`, que devolve `200`. Um erro de codificação não seria selectivo por tipo de IQ. |
| H2 | Namespace errado | **ELIMINADA** — `blocklist` é o namespace que o `GET` usa com sucesso, e é o que Baileys (`src/Socket/chats.ts`) e whatsmeow (`user.go`) usam nas versões actuais. |
| H3 | Atributos em falta no `<item>` | **CONFIRMADA (parcial)** — falta `pn_jid` no caso `block`. Ver H4: a causa raiz é o `jid`, e `pn_jid` é o atributo que a nova forma acrescenta. |
| H4 | **Formato de JID errado (PN em vez de LID)** | **CONFIRMADA** — as três referências maduras migraram o atributo `jid` deste stanza de PN para LID. Nós enviamos PN em 4 de 4 casos (H4 é a causa; ver "Referências"). |
| H5 | Resposta esperada errada (chega como evento assíncrono) | **ELIMINADA** — não há timeout. O servidor RESPONDE, e responde `400 bad-request`. Uma resposta esperada no sítio errado produz silêncio, não recusa. |
| H6 | Estado de conta incompatível | **ELIMINADA** — duas contas distintas, uma Business e uma pessoal, ambas com sessão viva (o `GET` funciona), mesmo erro. |
| H7 | Bug da biblioteca (defeito nosso na vendorização) | **ELIMINADA como causa primária** — o código vendorizado é uma tradução fiel do whatsmeow de antes de 2026-08-13. Não divergimos do upstream: ficámos PARADOS nele. A divergência ACRESCENTADA por nós (`resolveBlocklistPNJID`) agrava mas não origina. |
| H8 | **Mudança de protocolo recente** | **CONFIRMADA** — datada: Baileys em 2026-04-24, whatsmeow em 2026-08-13. A bateria de campo é de 2026-08-26, 13 dias depois do commit do whatsmeow. |
| H9 | Regra de negócio (Business vs Personal) | **ELIMINADA** — ver H6 e a secção "tipo de conta". |

**Hipótese adicional levantada pelo trace, e o seu veredicto:**

| # | hipótese | veredicto |
|---|---|---|
| H10 | Pré-condição da app: "bloquear contacto PN exige conversa existente" | **NÃO ELIMINADA, mas não é a causa** — a mensagem `"[blocklist] trying to block a pn contact (id: …) without a chat"` foi lida do bundle vivo da SPA (`internal/wa-headless/capabilities/block/block.go:15-20`). Ela confirma que o subsistema de blocklist discrimina PN, o que é evidência CORROBORANTE de H4, mas é uma guarda do cliente, não do servidor, e não explica o `400` no `unblock` (que não tem essa pré-condição). |

---

## Referências consultadas — o que cada uma respondeu

### whatsmeow — **RESPONDEU A CAUSA**

Commit `8d023aa973`, **2026-08-13**, PR [#1137] *"user: switch UpdateBlocklist
to use LIDs"* (autor externo: `zennn08`; mergeado pelo mantenedor). O diff, na
íntegra do que interessa:

```go
+	if jid.Server == types.DefaultUserServer {
+		pnJID = jid
+		lid, err := cli.Store.LIDs.GetLIDForPN(ctx, jid)
+		…
+		if lid.IsEmpty() {                      // fallback: pergunta ao servidor
+			info, err := cli.GetUserInfo(ctx, []types.JID{jid})
+			lid = info[jid].LID
+		}
+		lidJID = lid
+	} else if jid.Server == types.HiddenUserServer {
+		lidJID = jid
+		if action == events.BlocklistChangeActionBlock {
+			pn, _ := cli.Store.LIDs.GetPNForLID(ctx, jid)
+			pnJID = pn
+		}
+	}
+	itemAttrs := waBinary.Attrs{"jid": lidJID, "action": string(action)}
+	if action == events.BlocklistChangeActionBlock && !pnJID.IsEmpty() {
+		itemAttrs["pn_jid"] = pnJID
+	}
 	resp, err := cli.sendIQ(ctx, infoQuery{
 		Namespace: "blocklist", Type: iqSet, To: types.ServerJID,
-		Content: … Attrs: waBinary.Attrs{"jid": jid, "action": string(action)},
+		Content: … Attrs: itemAttrs,
 	})
```

Repare no **sentido**: o upstream resolve **PN → LID**. Nós resolvemos
**LID → PN**. Estamos a fazer a conversão exactamente ao contrário.

O corpo do PR está vazio (só o checklist de contribuição) e não há comentários
— não há, portanto, relato do sintoma do lado do whatsmeow. O que a referência
dá é a FORMA, não o diagnóstico.

### Baileys — **RESPONDEU A CAUSA, E MAIS CEDO**

Commit `8ca9316a10`, **2026-04-24**, `src/Socket/chats.ts`, PR #2265
*"fix(chats): add validation for jid and pn_jid in updateBlockStatus"*. As
mensagens dos commits internos do PR contam a história por si:

- *"resolve PN/LID mapping for updateBlockStatus"*
- *"fix(blocklist): require pn_jid only for block, skip mapping on unblock"*
- *"fix(blocklist): ensure unblock uses LID and block uses LID+PN"*

O código resultante:

```ts
if (isLidUser(normalizedJid) || isHostedLidUser(normalizedJid)) {
    lid = normalizedJid
    if (action === 'block') { pn_jid = jidNormalizedUser(await …getPNForLID(normalizedJid)) }
} else if (isPnUser(normalizedJid) || isHostedPnUser(normalizedJid)) {
    lid = await signalRepository.lidMapping.getLIDForPN(normalizedJid)   // obrigatório
    if (action === 'block') { pn_jid = jidNormalizedUser(normalizedJid) }
}
const itemAttrs = { action, jid: lid }
if (action === 'block') { itemAttrs.pn_jid = pn_jid }   // e lança 400 se faltar
```

**A regra exacta, das duas referências, é a mesma**:

| acção | `jid` | `pn_jid` |
|---|---|---|
| `block` | LID | PN (obrigatório) |
| `unblock` | LID | ausente |

Baileys chegou lá **quatro meses antes** do whatsmeow. É o dado que data a
mudança do lado do servidor como pelo menos Abril/2026.

### whatsapp-web.js — **RESPONDEU: LID, pela via da SPA**

`src/structures/Contact.js:154-207`. Não monta stanza (dirige a página), mas
converte a identidade antes de chamar a app, e converte para **LID**:

```js
const lid = contact.id.isLid()
    ? contact.id
    : window.require('WAWebApiContact').getAlternateUserWid(contact.id);
const ContactToBlock = { id: lid, isContactBlocked: false, phoneNumber: null };
await window.require('WAWebBlockContactAction')
           .blockContact({ contact: ContactToBlock, blockEntryPoint: 'ChatListBlock' });
```

E no `unblock` faz o mesmo: se não é LID, resolve com `getAlternateUserWid` e
vai **buscar o contacto pelo LID** antes de chamar `unblockContact`.

É a **terceira** referência independente a dizer LID, e a mais próxima do nosso
terreno — é a mesma SPA. Note o `phoneNumber: null` no payload: é o campo que a
página preenche e que sai no fio como `pn_jid`.

Issues abertas do wwebjs sobre block/unblock: a pesquisa pela API do GitHub
devolveu contagem nula para o repositório `pedroslopez/whatsapp-web.js` (a
pesquisa de issues foi recusada sem autenticação). **Registo isto como não
consultado, não como "não existe"** — o código foi lido directamente do `main`.

### Evolution API — **NÃO RESPONDEU (é consumidor)**

A Evolution API expõe block/unblock por cima do Baileys; herda o `updateBlockStatus`
tal como ele estiver na versão fixada. Não tem lógica de identidade própria neste
caminho, portanto **não é uma quarta opinião** — é a mesma do Baileys, com atraso
de versão. A pesquisa de código no repositório exigiu autenticação e não foi feita.

### open-wa / wa-automate — **NÃO CONSULTADA**

Não foi consultada nesta sessão. Registo a lacuna em vez de a inventar. Dado que
três referências independentes concordam, a quarta não muda o veredicto; muda
apenas a margem.

### O nosso próprio transporte wa-headless — **RESPONDEU: a operação é possível**

`pkg/infra/wa-headless/blocklist/manager.go:1-11` declara block e unblock
PROVADOS no LEDGER (H59, blocklist 0→1→0). O transporte SPA entrega o JID como
recebeu (`pkg/infra/wa-headless/jid.go:31-36`) e é a **página** que faz a
resolução de identidade — a mesma `getAlternateUserWid` que o wwebjs chama.

Isto NÃO prova a forma do fio (não vemos o stanza), mas prova duas coisas úteis:
1. a operação de produto é permitida neste build e nestas contas — elimina
   qualquer leitura de "o WhatsApp deixou de deixar bloquear";
2. quem resolve identidade no caminho que funciona é o código que resolve para
   **LID**.

---

## Documentação oficial Meta/WhatsApp

**Não existe documentação pública oficial** do protocolo binário do WhatsApp
Web/multi-device. Nem o namespace `blocklist`, nem o `<item action=…>`, nem o
atributo `pn_jid` aparecem em qualquer documento publicado pela Meta. A
Cloud API (a API oficial documentada) **não expõe bloquear/desbloquear
contactos** de todo.

Isto é informação válida e não uma lacuna da pesquisa: significa que **não há
evidência de Nível A possível para esta rota**, e que o teto de evidência aqui é
Nível C (implementações de referência maduras concordantes) + Nível B (medição).
Nenhuma afirmação deste documento deve ser lida como "a Meta diz".

## Graduação da evidência

| afirmação | nível | fonte |
|---|---|---|
| O servidor responde `400 bad-request` ao nosso IQ de escrita | **B** | bateria de campo 2026-08-26, 4/4 |
| `GET /users/blocklist` funciona na mesma sessão | **B** | mesma bateria |
| O stanza correcto usa `jid`=LID e `pn_jid`=PN no block | **C** | whatsmeow `8d023aa973`, Baileys `8ca9316a10`, wwebjs `Contact.js` |
| A mudança é de 2026 (Abr–Ago) | **C/D** | datas dos commits das duas bibliotecas |
| Nós enviamos PN em 4/4 dos casos medidos | **B** (leitura de código determinística) | `blocklist.go:50` + `blocklist.go:103-116` |
| Não há doc oficial | **B** (ausência verificada) | Cloud API não expõe a capacidade |
| Bloquear contacto PN sem conversa é recusado pela app | **C** | string lida do bundle vivo, `block.go:15` |

---

## Conclusão

**Classificação: `PROTOCOL_CHANGED`.**

O WhatsApp migrou a escrita da blocklist para endereçamento por **LID**. O
`<item>` passou a exigir `jid` em `@lid` e, no `block`, um `pn_jid` com o número
de telefone. A nossa biblioteca vendorizada envia a forma anterior a essa
migração, e o nosso adaptador **converte activamente LID → PN**, garantindo que
nenhuma das entradas possíveis produz a forma nova. Daí o `400 bad-request`
uniforme em 4 de 4 combinações, e daí a leitura (`GET`) continuar a funcionar:
ela não carrega `<item>` nenhum.

Escolho `PROTOCOL_CHANGED` e não `BUG_DEPENDENCY` porque o código local estava
CORRECTO quando foi escrito e nada nele se partiu; o que mudou foi o servidor.
A consequência local — biblioteca parada num commit anterior a `8d023aa973`,
mais uma conversão nossa no sentido inverso — é o efeito, não a causa.

## Correcção (NÃO aplicada nesta sessão)

Não foi aplicada por decisão explícita: a classificação não é `BUG_LOCAL`, e o
CLAUDE.md proíbe corrigir de graça fora do escopo sem perguntar. Registada em
`internal/wa-noise/HOUSEKEEP.md` (biblioteca) com referência cruzada em
`HOUSEKEEP.md` (adaptador).

O caminho, em duas partes:

1. **`internal/wa-noise/capabilities/user/blocklist.go`** — portar a lógica de
   `8d023aa973`: `UpdateBlocklist` passa a receber (ou a resolver) o LID e a
   emitir `pn_jid` no `block`. Exige acesso ao `LIDs` store e ao `GetUserInfo`
   como fallback, tal como o upstream.
2. **`pkg/infra/wa-noise/adapters/user/blocklist.go:103-116`** — inverter
   `resolveBlocklistPNJID`: passa a resolver **PN → LID**, e a guardar o PN para
   o `pn_jid`. A função deixa de ter o nome certo.

**O sub-caso mais barato, e é só nosso**: para `unblock`, a forma nova é
`<item jid='…@lid' action='unblock'/>` — sem `pn_jid`. Hoje, quando o cliente
já nos dá um LID, nós degradamo-lo para PN antes de enviar. Deixar de o fazer
produziria exactamente o stanza correcto do `unblock`, **sem tocar na
biblioteca**. É uma alteração de três linhas em `pkg/`. Fica proposta, não
aplicada.

## Teste de regressão

Não escrito, porque nada foi corrigido. Quando for, o mínimo exigido pelo
CLAUDE.md, nesta ordem:

1. **Teste do defeito, ao nível do stanza**: com o `Transport` falso já
   existente em `internal/wa-noise/capabilities/user/blocklist_test.go`,
   afirmar que `UpdateBlocklist` com entrada PN emite `item.jid` terminado em
   `@lid` e, no `block`, um `pn_jid` em `@s.whatsapp.net`. Os valores da tabela
   medida (`554192421234` / `90937376170214`) são os que devem ser usados —
   não uma aproximação.
2. **Teste da ORDEM/direcção**: afirmar que `resolveBlocklist…` recebendo `@lid`
   devolve `@lid`. É a asserção que morre se alguém reintroduzir a conversão
   invertida — e é a causa, não o sintoma.
3. **Controlo negativo EXECUTADO**: repor `"jid": jid` sem a resolução e colar a
   saída da falha na entrada do HOUSEKEEP.
4. **Dublê não mais simples que a produção**: o `Transport` falso tem de
   atravessar `types.JID.String()` como o codificador real atravessa, senão o
   teste abençoa o formato errado (ARMADILHA #1, variante "mais SIMPLES").

## Evidência final

- Três implementações de referência maduras e independentes concordam em LID.
- Duas delas datam a mudança (2026-04-24 e 2026-08-13), ambas ANTES da nossa
  medição de 2026-08-26.
- O nosso próprio transporte alternativo executa a operação com sucesso, pela
  via que resolve para LID.
- A leitura da blocklist, que não carrega `<item>`, continua a funcionar.

Nenhuma medição nova foi feita nesta sessão: este ambiente não tem conta
emparelhada. Ver `HUMAN-LAST.md`.

## Próxima pista

Não é preciso: a causa está determinada. O que falta é **confirmação em campo**
da forma nova, e o experimento exacto está em `HUMAN-LAST.md` (E.1 e E.2).
