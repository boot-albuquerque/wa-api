# Auditoria de fachada — linhas MISSING do LEDGER-WWEBJS

**Data**: 2026-08-22
**Contexto**: H109 revelou que a família `Chat` era quase toda fachada.
Esta auditoria mede se o padrão se repete nas outras famílias de
`src/structures/` que têm linhas `MISSING` no ledger.

**Upstream fixado**: `whatsapp-web.js` v1.34.7
**SHA**: `f935b500117e264c2b3abc25b63a280bd98182a7`

## Vocabulário

| tipo | significa |
|---|---|
| **(a)** | delegação PURA para método homônimo do Client (uma linha, só repassa id) |
| **(b)** | delegação COM lógica extra (guarda, transformação, iteração antes de delegar) |
| **(c)** | implementação própria (avalia JS na página ou lógica local sem Client) |

Regra para "herda?":
- `sim` — somente quando for **(a)** E o par no Client tiver estado (PROVEN ou PARTIAL).
- `não — lógica extra` — tipo **(b)**, com a lógica descrita.
- `não — implementação própria` — tipo **(c)**.
- `não — par também MISSING` — tipo **(a)** mas o Client também não tem o método provado.

---

## Contact

| linha MISSING | tipo | arquivo:linha upstream | par no Client | estado do par | herda? |
|---|---|---|---|---|---|
| `getProfilePicUrl` | (a) | Contact.js:119–121 | `getProfilePicUrl` | `PROVEN` | sim |
| `getChat` | (b) | Contact.js:144–148 | `getChatById` | `PARTIAL` | não — lógica extra: `if (this.isMe) return null` antes de delegar |
| `getCommonGroups` | (a) | Contact.js:223–225 | `getCommonGroups` | `PROVEN` | sim |

**Evidência**: `getProfilePicUrl` é `return await this.client.getProfilePicUrl(this.id._serialized)` — uma linha. `getCommonGroups` é `return await this.client.getCommonGroups(this.id._serialized)` — uma linha. `getChat` tem a guarda `if (this.isMe) return null` (linha 145) antes de delegar para `getChatById`.

---

## GroupChat

| linha MISSING | tipo | arquivo:linha upstream | par no Client | estado do par | herda? |
|---|---|---|---|---|---|
| `setDescription` | (c) | GroupChat.js:407–440 | — | — | não — implementação própria |
| `deletePicture` | (c) | GroupChat.js:548–554 | — | — | não — implementação própria |
| `setPicture` | (c) | GroupChat.js:561–571 | — | — | não — implementação própria |

**Evidência**: `setDescription` avalia JS na página chamando `WAWebGroupModifyInfoJob.setGroupDescription` (linha 421), com `descId` e `newId` locais — 34 linhas de lógica de página. `deletePicture` chama `window.WWebJS.deletePicture(chatid)` na página. `setPicture` chama `window.WWebJS.setPicture(chatid, media)`. Nenhum deles delega para o Client.

---

## Message

| linha MISSING | tipo | arquivo:linha upstream | par no Client | estado do par | herda? |
|---|---|---|---|---|---|
| `getMentions` | (b) | Message.js:403–412 | `getContactById` (por menção) | `PARTIAL` | não — lógica extra: `Promise.all` sobre `this.mentionedIds.map(m => client.getContactById(m))` |
| `getGroupMentions` | (b) | Message.js:418–425 | `getChatById` (por menção) | `PARTIAL` | não — lógica extra: `Promise.all` sobre `this.groupMentions.map(m => client.getChatById(m.groupJid._serialized))` |
| `acceptGroupV4Invite` | (a) | Message.js:487–489 | `acceptGroupV4Invite` | `MISSING` | não — par também MISSING |
| `getOrder` | (c) | Message.js:792–806 | — | — | não — implementação própria |
| `getPayment` | (c) | Message.js:812–828 | — | — | não — implementação própria |
| `getReactions` | (c) | Message.js:843–867 | — | — | não — implementação própria |
| `editScheduledEvent` | (c) | Message.js:951–992 | — | — | não — implementação própria |
| `pin` | (c) | Message.js:718–730 | — | — | não — implementação própria |
| `unpin` | (c) | Message.js:736–739 | — | — | não — implementação própria |

**Evidência**: `getMentions` e `getGroupMentions` iteram sobre listas internas e resolvem cada id via Client — o padrão é `Promise.all(this.X.map(m => this.client.getY(m)))`, que não é uma delegação de uma linha. `acceptGroupV4Invite` é puro (`return await this.client.acceptGroupV4Invite(this.inviteV4)`, linha 488) mas o Client também é MISSING. `getOrder` avalia `window.WWebJS.getOrderDetail(orderId, token, chatId)` na página (linha 796). `getPayment` busca a mensagem e serializa (linhas 814–825). `getReactions` busca `WAWebCollections.Reactions.find(msgId)` na página (linha 849). `editScheduledEvent` avalia `WAWebSendEventEditMsgAction.sendEventEditMessage` na página (linha 980). `pin` chama `window.WWebJS.pinUnpinMsgAction(msgId, 1, duration)` (linha 721). `unpin` chama `window.WWebJS.pinUnpinMsgAction(msgId, 2, 0)` (linha 738).

---

## GroupNotification

| linha MISSING | tipo | arquivo:linha upstream | par no Client | estado do par | herda? |
|---|---|---|---|---|---|
| `getChat` | (a) | GroupNotification.js:77–79 | `getChatById` | `PARTIAL` | sim |
| `getContact` | (a) | GroupNotification.js:85–87 | `getContactById` | `PARTIAL` | sim |
| `getRecipients` | (b) | GroupNotification.js:93–99 | `getContactById` (por recipient) | `PARTIAL` | não — lógica extra: `Promise.all` sobre `this.recipientIds.map(m => client.getContactById(m))` |
| `reply` | (a) | GroupNotification.js:108–110 | `sendMessage` | `PARTIAL` | sim |

**Evidência**: `getChat` é `return this.client.getChatById(this.chatId)` — uma linha. `getContact` é `return this.client.getContactById(this.author)` — uma linha. `reply` é `return this.client.sendMessage(this.chatId, content, options)` — uma linha. `getRecipients` itera sobre `recipientIds` resolvendo cada um via `getContactById`.

---

## Label

| linha MISSING | tipo | arquivo:linha upstream | par no Client | estado do par | herda? |
|---|---|---|---|---|---|
| `getChats` | (a) | Label.js:45–47 | `getChatsByLabelId` | `MISSING` | não — par também MISSING |

**Evidência**: `getChats` é `return this.client.getChatsByLabelId(this.id)` — uma linha, mas `getChatsByLabelId` no Client também está MISSING (linha 95 do ledger: H114 diz que os 3 rótulos da conta têm zero itens).

---

## ClientInfo

| linha MISSING | tipo | arquivo:linha upstream | par no Client | estado do par | herda? |
|---|---|---|---|---|---|
| `getBatteryStatus` | (c) | ClientInfo.js:63–67 | — | — | não — implementação própria |

**Evidência**: avalia `window.require('WAWebConnModel').Conn` na página para ler `battery` e `plugged` (linhas 64–66). Método marcado como `@deprecated` no upstream. Não delega para o Client.

---

## Broadcast

| linha MISSING | tipo | arquivo:linha upstream | par no Client | estado do par | herda? |
|---|---|---|---|---|---|
| `getChat` | (a) | Broadcast.js:55–57 | `getChatById` | `PARTIAL` | sim |
| `getContact` | (a) | Broadcast.js:63–65 | `getContactById` | `PARTIAL` | sim |

**Evidência**: `getChat` é `return this.client.getChatById(this.id._serialized)` — uma linha. `getContact` é `return this.client.getContactById(this.id._serialized)` — uma linha.

---

## Channel

| linha MISSING | tipo | arquivo:linha upstream | par no Client | estado do par | herda? |
|---|---|---|---|---|---|
| `getSubscribers` | (c) | Channel.js:105–132 | — | — | não — implementação própria |
| `setDescription` | (c) | Channel.js:153–160 | — | — | não — implementação própria |
| `setProfilePicture` | (c) | Channel.js:167–172 | — | — | não — implementação própria |
| `setReactionSetting` | (c) | Channel.js:184–197 | — | — | não — implementação própria |
| `mute` | (c) | Channel.js:203–210 | — | — | não — implementação própria |
| `unmute` | (c) | Channel.js:216–223 | — | — | não — implementação própria |
| `sendMessage` | (a) | Channel.js:239–241 | `sendMessage` | `PARTIAL` | sim |
| `sendSeen` | (a) | Channel.js:247–249 | `sendSeen` | `PARTIAL` | sim |
| `sendChannelAdminInvite` | (a) | Channel.js:262–268 | `sendChannelAdminInvite` | `MISSING` | não — par também MISSING |
| `acceptChannelAdminInvite` | (a) | Channel.js:274–276 | `acceptChannelAdminInvite` | `MISSING` | não — par também MISSING |
| `revokeChannelAdminInvite` | (a) | Channel.js:283–288 | `revokeChannelAdminInvite` | `MISSING` | não — par também MISSING |
| `demoteChannelAdmin` | (a) | Channel.js:295–297 | `demoteChannelAdmin` | `MISSING` | não — par também MISSING |
| `transferChannelOwnership` | (a) | Channel.js:312–318 | `transferChannelOwnership` | `MISSING` | não — par também MISSING |
| `fetchMessages` | (c) | Channel.js:327–374 | — | — | não — implementação própria |
| `deleteChannel` | (a) | Channel.js:380–382 | `deleteChannel` | `PROVEN` | sim |

**Evidência**: `sendMessage` é `return this.client.sendMessage(this.id._serialized, content, options)` — uma linha. `sendSeen` é `return this.client.sendSeen(this.id._serialized)` — uma linha. `deleteChannel` é `return this.client.deleteChannel(this.id._serialized)` — uma linha. Os cinco de admin channel (`sendChannelAdminInvite`, `acceptChannelAdminInvite`, `revokeChannelAdminInvite`, `demoteChannelAdmin`, `transferChannelOwnership`) são todos delegação pura de uma linha, mas TODOS delegam para Client methods que também estão MISSING.

`getSubscribers` avalia JS extenso na página (linhas 106–131): chama `WAWebMexFetchNewsletterSubscribersJob`, `WAWebNewsletterGatingUtils`, `WAWebNewsletterSubscriberListAction`. `setDescription` e `setProfilePicture` delegam para o método INTERNO `_setChannelMetadata` (linhas 390–424), que avalia `WAWebEditNewsletterMetadataAction.editNewsletterMetadataAction` na página. `setReactionSetting` faz o mesmo com mapeamento de código de reação. `mute`/`unmute` delegam para `_muteUnmuteChannel` (linhas 431–461), que avalia `WAWebNewsletterUpdateUserSettingJob.updateNewsletterUserSetting` na página. `fetchMessages` carrega mensagens diretamente da página (linhas 328–373) usando `WAWebChatLoadMessages.loadEarlierMsgs`.

---

## MessageMedia

| linha MISSING | tipo | arquivo:linha upstream | par no Client | estado do par | herda? |
|---|---|---|---|---|---|
| `fromFilePath` | (c) | MessageMedia.js:48–53 | — | — | não — implementação própria |
| `fromUrl` | (c) | MessageMedia.js:67–121 | — | — | não — implementação própria |

**Evidência**: ambos são métodos `static` (construtores de conveniência). `fromFilePath` lê do filesystem com `fs.readFileSync` (linha 49). `fromUrl` faz fetch HTTP (linhas 76–103). Nenhum deles é delegação para o Client — são utilitários de construção.

---

## Chat (linhas MISSING residuais — não auditadas na H109)

A H109 já auditou as linhas herdadas. As três MISSING restantes:

| linha MISSING | tipo | arquivo:linha upstream | par no Client | estado do par | herda? |
|---|---|---|---|---|---|
| `markUnread` | (a) | Chat.js:192–194 | `markChatUnread` | `MISSING` | não — par também MISSING |
| `addOrEditCustomerNote` | (b) | Chat.js:326–329 | `addOrEditCustomerNote` | `MISSING` | não — lógica extra: `if (this.isGroup \|\| this.isChannel) return` (e par também MISSING) |
| `getCustomerNote` | (b) | Chat.js:344–348 | `getCustomerNote` | `MISSING` | não — lógica extra: `if (this.isGroup \|\| this.isChannel) return null` (e par também MISSING) |

**Evidência**: `markUnread` é `return this.client.markChatUnread(this.id._serialized)` — uma linha, puro, mas o par no Client está MISSING. `addOrEditCustomerNote` tem guarda para grupo/canal (linha 327) antes de delegar. `getCustomerNote` idem (linha 345).

---

## Famílias mencionadas na tarefa sem arquivo correspondente em `src/structures/`

| família | existe em `src/structures/`? | por que não aparece no ledger |
|---|---|---|
| `PrivateChat` | sim (PrivateChat.js) | classe vazia — `class PrivateChat extends Chat {}`. Sem métodos adicionais. |
| `Buttons` | sim (Buttons.js) | construtor de formatação. Sem métodos de instância que deleguem para Client. |
| `List` | sim (List.js) | construtor de formatação. Sem métodos de instância. |
| `Reaction` | sim (Reaction.js) | classe de dados. Só `_patch`, sem métodos públicos. |
| `Order` | sim (Order.js) | classe de dados. Só `_patch`, sem métodos públicos. |

Nenhuma destas contribui métodos de capacidade — são tipos de dados ou construtores de mensagem. Corretamente ausentes do ledger.

---

## Resumo quantitativo

### Por tipo de implementação

| tipo | contagem | descrição |
|---|---|---|
| **(a)** delegação pura | **18** | uma linha, repassa id para método do Client |
| **(b)** delegação com lógica extra | **6** | guarda, iteração ou transformação antes de delegar |
| **(c)** implementação própria | **19** | avalia JS na página ou lógica local, sem Client |
| **total** | **43** | |

### Por resultado de herança

| resultado | contagem | linhas |
|---|---|---|
| **herda (sim)** | **10** | Contact.getProfilePicUrl, Contact.getCommonGroups, GroupNotification.getChat, GroupNotification.getContact, GroupNotification.reply, Broadcast.getChat, Broadcast.getContact, Channel.sendMessage, Channel.sendSeen, Channel.deleteChannel |
| **não herda — lógica extra** | **6** | Contact.getChat, Message.getMentions, Message.getGroupMentions, GroupNotification.getRecipients, Chat.addOrEditCustomerNote, Chat.getCustomerNote |
| **não herda — par também MISSING** | **8** | Message.acceptGroupV4Invite, Label.getChats, Channel.sendChannelAdminInvite, Channel.acceptChannelAdminInvite, Channel.revokeChannelAdminInvite, Channel.demoteChannelAdmin, Channel.transferChannelOwnership, Chat.markUnread |
| **não herda — implementação própria** | **19** | (ver tabelas acima) |
| **total** | **43** | |

### Por família auditada

| família | MISSING examinadas | herda | não herda |
|---|---|---|---|
| Contact | 3 | 2 | 1 |
| GroupChat | 3 | 0 | 3 |
| Message | 9 | 0 | 9 |
| GroupNotification | 4 | 3 | 1 |
| Label | 1 | 0 | 1 |
| ClientInfo | 1 | 0 | 1 |
| Broadcast | 2 | 2 | 0 |
| Channel | 15 | 3 | 12 |
| MessageMedia | 2 | 0 | 2 |
| Chat (residual) | 3 | 0 | 3 |
| **total** | **43** | **10** | **33** |

### Conclusão

De 43 linhas MISSING examinadas em 10 famílias, **10 são fachada pura com par
já provado** e candidatas a herdar estado. As outras 33 são trabalho real:
implementação própria (19), delegação com lógica extra (6), ou delegação pura
cujo par no Client também está MISSING (8).

A hipótese da H109 — que Chat não era caso único — **confirma-se parcialmente**:
GroupNotification (3 de 4 herdam), Broadcast (2 de 2 herdam) e a fatia de
delegação do Channel (3 de 15 herdam) seguem o padrão de fachada. Mas a maioria
das linhas MISSING nas outras famílias é implementação própria, especialmente
Message (9 de 9) e GroupChat (3 de 3).
