# GLOBAL-WWEBJS-PARITY-LEDGER

**Upstream fixado**: `whatsapp-web.js` **v1.34.7**
**SHA**: `f935b500117e264c2b3abc25b63a280bd98182a7`
**Data do commit**: 2026-04-24
**Ledger gerado em**: 2026-08-21

Este arquivo nasce da **superfície pública real do upstream**, extraída do
código-fonte da tag acima — não do que nós já atacamos. Essa diferença é o
motivo de ele existir: o `PARIDADE-WWEBJS.md` lista o que foi ATACADO, e por
construção não consegue mostrar o que nunca foi procurado.

## Regra de completude

Nenhuma capacidade pública do commit fixado pode ficar ausente em silêncio.
Item que não implementarmos tem de ter linha aqui dizendo POR QUÊ e qual
evidência impede o fechamento. A Fase 1 só fecha quando não houver `MISSING`
nem `PARTIAL` sem justificativa explícita.

## Vocabulário de estado

| estado | significa |
|---|---|
| `PROVEN` | equivalente entregue, com prova unitária, prova contra a SPA real e controle negativo |
| `PARTIAL` | entregue com metade não provável, ou existente só como passo interno de outra capacidade |
| `MISSING` | não atacado |
| `BLOCKED` | atacado, medido, e impedido por algo fora do nosso alcance |
| `INTENTIONAL_DIFFERENCE` | resolvemos de outro jeito, de propósito e registrado |

## Client

| upstream | wa-headless | estado | unitário | SPA real | ctrl. neg. | nota |
|---|---|---|---|---|---|---|
| `inject` | spa.VerifyInventory | `INTENTIONAL_DIFFERENCE` | sim | sim | sim | não injetamos ExposeStore; verificamos o inventário de módulos no boot |
| `initialize` | core.StartSession + runtime.Holder | `PROVEN` | sim | sim | sim | — |
| `requestPairingCode` | — | `MISSING` | — | — | — | pareamos por QR; código por telefone nunca atacado |
| `cancelPairingCode` | — | `MISSING` | — | — | — | idem |
| `attachEventListeners` | capabilities/messagemeta + contacts.onContact | `PARTIAL` | sim | sim | sim | dois fluxos de 31 eventos |
| `initWebVersionCache` | — | `INTENTIONAL_DIFFERENCE` | — | — | — | não fixamos versão da web; o inventário de módulos é a nossa guarda |
| `destroy` | core.Session.Stop | `PROVEN` | sim | sim | sim | stopped_via medido |
| `logout` | — | `MISSING` | — | — | — | apagar credenciais da sessão nunca foi atacado |
| `getWWebVersion` | spa.WebVersion | `PROVEN` | sim | sim | sim | H111: lido ao vivo (`2.3000.1045798079`); página sem versão é `ErrNoWebVersion`, não string vazia |
| `setDeviceName` | — | `MISSING` | — | — | — | — |
| `sendSeen` | chats.MarkRead | `PARTIAL` | sim | duvidosa | sim | **rebaixada em 2026-08-21 (H82)**: a pós-condição afirma que `chat.unreadCount` moveu NA MESMA SESSÃO, e a H78 mediu esse contador como CROSS_SESSION. A H52 provou contra um chat em que ele moveu; se generaliza é pergunta em aberto |
| `sendMessage` | send.Text / send.SendMedia / send.PollTo | `PARTIAL` | sim | sim | sim | texto, mídia, documento e figurinha OK. **ENQUETE NÃO SAI**: criada localmente como `poll_creation` com as opções intactas, `ack` fica em **0** e o par nunca recebe — medido no primeiro round trip que olhou o OUTRO lado (H98). A H69 provou o envio pela aparição LOCAL. O suspeito nomeado (`pollType` omitido) foi **perseguido e descartado**: o enum foi achado em `WAWebPollCreationUtils` (singular — uma letra é por que três buscas o perderam), `PollType.POLL` e `PollContentType.TEXT` foram aplicados de dentro da própria página, e o ack continua 0 (H101). Localização e vCard MISSING (H75) |
| `sendReaction` | capabilities/react | `PARTIAL` | sim | sim | sim | H53: adicionar provado; remover devolve Verified:false |
| `sendChannelAdminInvite` | — | `MISSING` | — | — | — | não atacado |
| `searchMessages` | search.Messages | `PROVEN` | sim | sim | sim | H114: o resultado é ENDEREÇO, nunca corpo — a projeção acontece na página. Sem escopo por conversa: passar o jid como quarto argumento (o que a referência faz com `options.chatId`) mediu 0 resultados com `eof`, contra 20 sem escopo |
| `getChats` | chats.List | `PROVEN` | sim | sim | sim | — |
| `getChannels` | — | `MISSING` | — | — | — | lista os canais SEGUIDOS, e esta conta não segue nenhum (`modelCount` 0). Não é falta de código: falta uma inscrição (H104) |
| `getChatById` | resolução interna às capacidades | `PARTIAL` | sim | sim | sim | existe como passo interno, não como capacidade exposta |
| `getChannelByInviteCode` | channel.ByInviteCode | `PROVEN` | sim | sim | sim | provado contra canal público real: jid `@newsletter`, nome, **45.460 assinantes**, `state=active`, `verification=verified`, e `following=false` — **nada foi seguido**. Aceita o link inteiro, não só o código (H104) |
| `getContacts` | contacts.List | `PROVEN` | sim | sim | sim | 944 -> 544 após dedup |
| `getContactById` | resolução interna | `PARTIAL` | sim | sim | sim | idem getChatById |
| `getMessageById` | varredura da MsgCollection nas capacidades | `PARTIAL` | sim | sim | sim | idem |
| `getPinnedMessages` | pin.PinnedIn | `PARTIAL` | sim | vazia | sim | o leitor funciona e a conta não tem NADA fixado; provar não-vazio exigiria fixar, que está bloqueado (H81) |
| `getInviteInfo` | group.InviteInfo | `PROVEN` | sim | sim | sim | lê o grupo atrás de um link SEM entrar; provado ao vivo reportando `approval=true` no grupo armado (H89) |
| `acceptInvite` | group.JoinByInvite | `PARTIAL` | sim | sim | sim | provado ao vivo o caminho de APROVAÇÃO: o page REJEITA com `UnexpectedJoinGroupViaInviteResponse` carregando `gid` e `membershipApprovalMode`, e isso É a criação do pedido. O caminho de entrada direta (grupo sem aprovação) não foi exercitado (H89) |
| `acceptChannelAdminInvite` | — | `MISSING` | — | — | — | não atacado |
| `revokeChannelAdminInvite` | — | `MISSING` | — | — | — | não atacado |
| `demoteChannelAdmin` | — | `MISSING` | — | — | — | não atacado |
| `acceptGroupV4Invite` | — | `MISSING` | — | — | — | — |
| `setStatus` | capabilities/profile (parcial) | `BLOCKED` | — | — | — | H66: setMyTextStatus tem 5 primitivos e ZERO chamadores no bundle |
| `setDisplayName` | profile.SetDisplayName | `BLOCKED` | sim | impossível | sim | H66: canSetMyPushname()=false — a conta é Business |
| `getState` | capabilities/liveness | `PROVEN` | sim | sim | sim | pior latência 1ms em 5 amostras |
| `sendPresenceAvailable` | capabilities/presence | `PARTIAL` | sim | parcial | sim | H50: observação não provada; exige as duas contas na agenda uma da outra |
| `sendPresenceUnavailable` | capabilities/presence | `PARTIAL` | sim | parcial | sim | idem |
| `archiveChat` | chats (arquivar) | `PROVEN` | sim | sim | sim | H55: causa era pedido REDUNDANTE |
| `unarchiveChat` | chats | `PROVEN` | sim | sim | sim | — |
| `pinChat` | chats | `PROVEN` | sim | sim | sim | — |
| `unpinChat` | chats | `PROVEN` | sim | sim | sim | — |
| `muteChat` | capabilities/mute | `PROVEN` | sim | sim | sim | H62: sendDevice:true é o que faz o efeito sair do dispositivo |
| `unmuteChat` | capabilities/mute | `PROVEN` | sim | sim | sim | — |
| `markChatUnread` | — | `MISSING` | — | — | — | temos marcar-como-LIDA, não o inverso |
| `getProfilePicUrl` | capabilities/avatar | `PROVEN` | sim | sim | sim | H41 |
| `getCommonGroups` | contacts.CommonGroupsWith | `PROVEN` | sim | sim | sim | H68: null significa "sou eu", não "nenhum" |
| `resetState` | liveness.Reset | `PARTIAL` | sim | sim | sim | H116: a chamada é feita e o socket é provado SAUDÁVEL depois; provar que ela FEZ algo não passa pelo Go — a transição dura ~450ms e cada leitura é um ida-e-volta do chromedp (0 de 3 ao vivo, contra 9 de 100 amostrando DENTRO da página). A referência não devolve nada, nem erro |
| `isRegisteredUser` | spa.ResolveIdentityExpr | `PARTIAL` | sim | sim | sim | é passo interno de todo envio; não exposto |
| `getNumberId` | spa.ResolveIdentityExpr | `PARTIAL` | sim | sim | sim | idem |
| `getFormattedNumber` | phone.Lookup (.Formatted) | `PROVEN` | sim | sim | sim | H111: a página NÃO recusa lixo — `findCC("notaphone")` devolve `"not"`, medido. Guardamos dos dois lados: a entrada tem de ser dígitos e a RESPOSTA também, e as duas guardas foram provadas independentes por controle negativo |
| `getCountryCode` | phone.Lookup (.CountryCode) | `PROVEN` | sim | sim | sim | H111: a página NÃO recusa lixo — `findCC("notaphone")` devolve `"not"`, medido. Guardamos dos dois lados: a entrada tem de ser dígitos e a RESPOSTA também, e as duas guardas foram provadas independentes por controle negativo |
| `createGroup` | group.Ensure | `PROVEN` | sim | sim | sim | idempotência é do fixture, não da capacidade |
| `createChannel` | channel.Create | `PROVEN` | sim | sim | sim | H113: canal real criado e apagado na conta de laboratório, com autorização explícita. Verificado relendo pelo código de convite, não pelo eco da própria chamada; gate desabilitado é erro PRÓPRIO (a referência devolve a mensagem como STRING) |
| `subscribeToChannel` | — | `MISSING` | — | — | — | não atacado |
| `unsubscribeFromChannel` | — | `MISSING` | — | — | — | não atacado |
| `transferChannelOwnership` | — | `MISSING` | — | — | — | não atacado |
| `searchChannels` | channel.Search | `PROVEN` | sim | sim | sim | H112: o diretório RESPONDE (50 resultados) — ao contrário de `getRecommendedNewsletters`, que trava. Sem assinatura. Tipo próprio `DirectoryEntry`: um resultado de diretório é um MODELO com campos `__x_`, não o saco de mixins da consulta de metadados, e `__x_state` não existe. Sem opção `limit`: a referência a implementa remendando uma função da página que não existe neste build |
| `deleteChannel` | channel.Delete | `PROVEN` | sim | sim | sim | H113: pós-condição é o canal deixar de ser legível; apagar sem código de convite devolve erro dizendo que NÃO deu para verificar, em vez de sucesso |
| `getLabels` | contacts.ListLabels | `PROVEN` | sim | sim | sim | H72; só mensurável por a conta ser Business |
| `getBroadcasts` | status.List | `PARTIAL` | sim | sim | sim | **NÃO é lista de transmissão** — o upstream chama de Broadcast o STATUS (stories): `getBroadcasts` é `Status.getModelsArray`. Nossa nota descrevia a coisa errada, e três linhas iam ser feitas contra a ideia errada (H100). Caminho provado ao vivo, com **zero** feeds; provar um não-vazio exige POSTAR status, visível aos 944 contatos da conta |
| `getBroadcastById` | status.ByContact | `PARTIAL` | sim | sim | sim | tenta as duas formas de identidade; feed ausente é erro próprio e não um feed de zeros, que um chamador leria como "essa pessoa não postou nada" (H100) |
| `revokeStatusMessage` | — | `MISSING` | — | — | — | — |
| `getLabelById` | contacts.ListLabels + filtro | `PARTIAL` | sim | sim | sim | — |
| `getChatLabels` | contacts.LabelsOfChat | `PROVEN` | sim | sim | sim | — |
| `getChatsByLabelId` | — | `MISSING` | — | medido | — | H114: os 3 rótulos desta conta têm ZERO itens (`chatLabelItems: 0`), então um leitor nunca seria visto devolvendo nada — armadilha H93 |
| `getBlockedContacts` | capabilities/block | `PARTIAL` | sim | sim | sim | bloquear/desbloquear provados; LISTAR os bloqueados não é exposto |
| `setProfilePicture` | — | `MISSING` | — | — | — | — |
| `deleteProfilePicture` | — | `MISSING` | — | — | — | — |
| `addOrRemoveLabels` | contacts.AddLabel / RemoveLabel | `PROVEN` | sim | sim | sim | H72; forma medida pelo instrumento da H73 |
| `getGroupMembershipRequests` | groupreq.List | `PROVEN` | sim | sim | sim | refresca a metadata antes de ler; campos do registro medidos ao vivo: `id t addedBy requestMethod parentGroupId` (H89) |
| `approveGroupMembershipRequests` | groupreq.Approve | `PROVEN` | sim | sim | sim | uma chamada RPC por solicitante, resultado por solicitante; provado ao vivo do pedido ao desaparecimento (H89) |
| `rejectGroupMembershipRequests` | groupreq.Reject | `PARTIAL` | sim | não | sim | mesma RPC do approve, diferindo só na chave enviada — travado por teste unitário que casa `rejectArgs:` com os dois pontos. NÃO exercitado ao vivo: rejeitar conta-B a expulsaria do grupo de laboratório (H89) |
| `setAutoDownloadAudio` | settings.SetAutoDownload(KindAudio) | `PROVEN` | sim | sim | sim | H110: a referência devolve o flag pedido sem olhar; nós relemos e falhamos com `ErrNotTaken` se a página não moveu. Escrita redundante é evitada (H55) e DITA em `Changed` |
| `setAutoDownloadDocuments` | settings.SetAutoDownload(KindDocuments) | `PROVEN` | sim | sim | sim | H110; as quatro categorias viradas ao vivo e o baseline restaurado e verificado |
| `setAutoDownloadPhotos` | settings.SetAutoDownload(KindPhotos) | `PROVEN` | sim | sim | sim | H110 |
| `setAutoDownloadVideos` | settings.SetAutoDownload(KindVideos) | `PROVEN` | sim | sim | sim | H110 |
| `setBackgroundSync` | settings.SetBackgroundSync | `PROVEN` | sim | sim | sim | H110: o valor guardado é lido de volta na hora; a referência avisa que o EFEITO só vale após reiniciar, e nada aqui afirma que a sessão viva mudou |
| `getContactDeviceCount` | addressbook.DeviceCount | `PARTIAL` | sim | sim | sim | o caminho funciona e o par NÃO tem registro de dispositivo nesta conta; "sem registro" e "zero dispositivos" são respostas diferentes e não foram fundidas no número 0 (H90) |
| `syncHistory` | capabilities/fetchmessages | `PARTIAL` | sim | sim | sim | buscamos histórico de uma conversa; sincronizar não |
| `createCallLink` | call.CreateLink | `PROVEN` | sim | sim | sim | provado ao vivo para `voice` e `video`; o link é credencial e nunca é renderizado. Usa `WAWebGenerateEventCallLink` como a referência — o `WAWebVoipCreateCallLink` deste build **TRAVA** na primeira chamada, medido em 40s (H92) |
| `sendResponseToScheduledEvent` | — | `MISSING` | — | — | — | — |
| `saveOrEditAddressbookContact` | addressbook.Save | `PROVEN` | sim | sim | sim | verifica lendo de volta, com o relógio no Go; `syncToAddressbook` é parâmetro sem padrão porque `true` escreve na agenda do TELEFONE pareado (H90) |
| `deleteAddressbookContact` | addressbook.Delete | `PROVEN` | sim | sim | sim | idempotente, medido; exige **wid**, enquanto o save exige dígitos crus — assimetria que a referência esconde passando o mesmo valor aos dois (H90) |
| `getContactLidAndPhone` | spa.ResolveIdentityExpr | `PARTIAL` | sim | sim | sim | build LID-first: 397 de 399 mensagens sob @lid |
| `addOrEditCustomerNote` | — | `MISSING` | — | — | — | — |
| `getCustomerNote` | — | `MISSING` | — | — | — | — |
| `getPollVotes` | poll.Votes | `PROVEN` | sim | sim | sim | lido contra uma enquete REAL: as duas opções presentes com zero. Usa a chave que a mensagem já carrega — `MsgKey.fromString(_serialized)` da referência lança neste build (H98) |

## Broadcast

**O nome do upstream engana, e o nosso ledger repetiu o engano.** "Broadcast"
aqui é **STATUS** — as stories de 24 horas —, não lista de transmissão:
`Client.getBroadcasts` é `getAllStatuses`, que é `Status.getModelsArray`, e a
estrutura carrega `msgs`, `totalCount` e `unreadCount` por CONTATO (H100).

`capabilities/status` só LÊ. Este build exporta `sendStatusTextMsgAction` e
`sendStatusMediaMsgAction` — sabe postar, coisa que o upstream nem expõe — e
isso **não está ligado**: um status é visível a toda a agenda, medida em 944
contatos nesta conta.

| upstream | estado |
|---|---|
| `getChat` | `PARTIAL` |
| `getContact` | `PARTIAL` |

## Call

`capabilities/call` entrega o link (provado) e a recusa (implementada, não
disparada). O que falta é **um gatilho que toca telefone**, não código.

Achado que a referência não tem: este build expõe uma superfície VOIP inteira —
`WAWebVoipStartCall` origina chamada, `WAWebVoipCancelOutgoingCall` cancela,
`WAWebVoipCreateCallLink` cria link próprio (e trava). O `whatsapp-web.js` não
tem equivalente para nenhum dos três.

| upstream | estado |
|---|---|
| `reject` | `PARTIAL` |

`reject` está implementado com a estranha exata que o protocolo pede — este
build **não exporta ação de recusa nenhuma** (`WAWebRejectCallAction`,
`WAWebEndCallAction`, `WAWebCallActions`, `WAWebOfferCallAction` todos ausentes,
medido), então a estranha à mão da referência é o único caminho, e não
improviso dela. Travado por teste que casa `call-id`, `call-creator`, `to` e
`count`. Não provado ao vivo: exige chamada entrante.

## Channel

**Superfície MEDIDA, família ainda não implementada** (H102). Vinte e quatro
módulos presentes com aridade batendo com a referência; só
`WAWebMexFetchNewsletterSubscribersJob` resolve `falsy`. Criação **habilitada**
(`isNewsletterCreationEnabled` true, teto de 5000 assinantes) e a superfície de
envio inteira existe (`sendNewsletterTextMsg`, `MediaMsg`, `PollCreationMsg`,
`AlbumMsg`, `EditMsg`).

**E a família destravou sem tocar na conta** (H104). O plano era seguir um canal
e devolver; acabou desnecessário: `queryNewsletterMetadataByInviteCode` responde
para um canal que esta conta **não** segue e carrega a forma inteira. O mixin de
membership volta `null`, que é precisamente o que diz "não sou membro" — a
leitura prova a si mesma.

A forma é de **MIXINS**, e a referência não a descreve. Não existe campo `name`
no topo; existe `newsletterNameMetadataMixin.nameElementValue`. Ler o campo óbvio
devolve `undefined` para sempre — quarta ocorrência dessa classe aqui.

O discovery do próprio app (`getRecommendedNewsletters`) continua **sem
responder nem ao próprio timeout de 8s**, medido em quatro execuções. Ele não é
mais necessário para ler, mas continua sendo o que falta para DESCOBRIR um canal
sem link.

**Atualizado em 2026-08-22 (H113)**, com um canal real criado e apagado na conta
de laboratório sob autorização explícita:

- `setSubject` passa a `PROVEN`: renomear pega e é relido do servidor.
- `setDescription` continua `MISSING` **por medição, não por omissão**: a página
  aceita a chamada e o servidor nunca reporta a descrição nova — nem depois de
  20s, enquanto o rename é legível na hora. Não é latência. A descrição passada
  na CRIAÇÃO também não fica.
- `getSubscribers` continua `MISSING` porque
  `WAWebMexFetchNewsletterSubscribersJob`, o módulo que a referência usa, **não
  existe neste build**.

| upstream | estado |
|---|---|
| `getSubscribers` | `MISSING` |
| `setSubject` | `PROVEN` |
| `setDescription` | `MISSING` |
| `setProfilePicture` | `MISSING` |
| `setReactionSetting` | `MISSING` |
| `mute` | `MISSING` |
| `unmute` | `MISSING` |
| `sendMessage` | `PARTIAL` |
| `sendSeen` | `PARTIAL` |
| `sendChannelAdminInvite` | `MISSING` |
| `acceptChannelAdminInvite` | `MISSING` |
| `revokeChannelAdminInvite` | `MISSING` |
| `demoteChannelAdmin` | `MISSING` |
| `transferChannelOwnership` | `MISSING` |
| `fetchMessages` | `MISSING` |
| `deleteChannel` | `PROVEN` |

## Chat

**A diferença desta família, dita uma vez.** Em `Chat.js` do upstream fixado,
quase todo método é uma delegação de UMA LINHA para o método homônimo do
`Client`, passando `this.id._serialized`. Aqui não há objeto `Chat` vivo: quem
chama passa o jid direto. Então o equivalente de `Chat.archive` **é** a
capacidade já provada em `chats`, e não uma segunda coisa a construir.

As linhas abaixo herdam o estado do seu par no `Client`, com a linha do upstream
nomeada para que a herança seja auditável. Herdar estado é escrituração, não
entrega: nenhuma delas ganhou código nesta passagem (H109).

Uma diferença real: `Chat.mute`/`unmute` atualizam `isMuted`/`muteExpiration` no
objeto em memória depois da chamada. Não temos esse cache, então toda leitura
nossa é fresca — não há campo velho para consertar.

| upstream | wa-headless | estado | unitário | SPA real | ctrl. neg. | nota |
|---|---|---|---|---|---|---|
| `sendMessage` | send.Text / send.SendMedia / send.PollTo | `PARTIAL` | sim | sim | sim | delegação literal para `Client.sendMessage` (Chat.js:102); herda a linha dele, ENQUETE INCLUSA no que não sai |
| `sendSeen` | chats.MarkRead | `PARTIAL` | sim | sim | sim | delegação literal para `Client.sendSeen` (Chat.js:110); herda o rebaixamento da H82 |
| `clearMessages` | chats.Clear | `PARTIAL` | sim | NÃO (por desenho) | sim | H66: destruiria o fixture de todos os outros testes |
| `delete` | chats.Delete | `PARTIAL` | sim | NÃO (por desenho) | sim | idem |
| `archive` | chats (arquivar) | `PROVEN` | sim | sim | sim | delegação literal para `Client.archiveChat` (Chat.js:137); H55 |
| `unarchive` | chats | `PROVEN` | sim | sim | sim | delegação literal para `Client.unarchiveChat` (Chat.js:144) |
| `pin` | chats | `PROVEN` | sim | sim | sim | delegação literal para `Client.pinChat` (Chat.js:152). NÃO confundir com a H81, que é fixar MENSAGEM e continua falhando |
| `unpin` | chats | `PROVEN` | sim | sim | sim | delegação literal para `Client.unpinChat` (Chat.js:160) |
| `mute` | capabilities/mute | `PROVEN` | sim | sim | sim | delegação para `Client.muteChat` (Chat.js:168); H62 |
| `unmute` | capabilities/mute | `PROVEN` | sim | sim | sim | delegação para `Client.unmuteChat` (Chat.js:182) |
| `markUnread` | chats.MarkUnread | `MISSING` | sim | falha (H78) | sim | duas primitivas medidas e nenhuma marca: `sendConversationSeen` com delta negativo e `Cmd.markChatUnread`. O `Cmd` é barramento de EVENTOS, e o ouvinte deste verbo vive num pedaço de UI que sessão headless não carrega |
| `fetchMessages` | capabilities/fetchmessages | `PROVEN` | sim | sim | sim | — |
| `sendStateTyping` | capabilities/chatstate | `PROVEN` | sim | sim | sim | — |
| `sendStateRecording` | presence.StateRecording | `PARTIAL` | sim | bloqueada (H50) | sim | **linha corrigida**: eu a marquei MISSING de memória e ela JÁ EXISTIA, mapeada para `markRecording`. A prova ao vivo esbarra no mesmo bloqueio da observação de presença |
| `clearState` | capabilities/chatstate | `PROVEN` | sim | sim | sim | — |
| `getContact` | resolução interna | `PARTIAL` | sim | sim | sim | delegação literal para `Client.getContactById` (Chat.js:284); herda a linha dele |
| `getLabels` | contacts.LabelsOfChat | `PROVEN` | sim | sim | sim | delegação literal para `Client.getChatLabels` (Chat.js:292); H72 |
| `changeLabels` | contacts.AddLabel / RemoveLabel | `PROVEN` | sim | sim | sim | delegação literal para `Client.addOrRemoveLabels` (Chat.js:301); H72 |
| `getPinnedMessages` | pin.PinnedIn | `PARTIAL` | sim | vazia | sim | o leitor funciona e a conta não tem NADA fixado; provar não-vazio exigiria fixar, que está bloqueado (H81) |
| `syncHistory` | capabilities/fetchmessages | `PARTIAL` | sim | sim | sim | delegação literal para `Client.syncHistory` (Chat.js:317); herda a linha dele — buscamos histórico de uma conversa, sincronizar não |
| `addOrEditCustomerNote` | — | `MISSING` | — | — | — | não atacado |
| `getCustomerNote` | — | `MISSING` | — | — | — | não atacado |

## ClientInfo

**Família inteira `MISSING`** — informação de bateria/plataforma — não atacada.

| upstream | estado |
|---|---|
| `getBatteryStatus` | `MISSING` |

## Contact

| upstream | wa-headless | estado | unitário | SPA real | ctrl. neg. | nota |
|---|---|---|---|---|---|---|
| `getProfilePicUrl` | capabilities/avatar | `PROVEN` | sim | sim | sim | H118: delegação literal para `Client.getProfilePicUrl` (Contact.js:119) |
| `getFormattedNumber` | phone.Lookup (.Formatted) | `PROVEN` | sim | sim | sim | delegação literal para o `Client` (Contact.js:128 e :136); H111: a página NÃO recusa lixo — `findCC("notaphone")` devolve `"not"`, medido. Guardamos dos dois lados: a entrada tem de ser dígitos e a RESPOSTA também, e as duas guardas foram provadas independentes por controle negativo |
| `getCountryCode` | phone.Lookup (.CountryCode) | `PROVEN` | sim | sim | sim | delegação literal para o `Client` (Contact.js:128 e :136); H111: a página NÃO recusa lixo — `findCC("notaphone")` devolve `"not"`, medido. Guardamos dos dois lados: a entrada tem de ser dígitos e a RESPOSTA também, e as duas guardas foram provadas independentes por controle negativo |
| `getChat` | — | `MISSING` | — | — | — | não atacado |
| `block` | capabilities/block | `PROVEN` | sim | sim | sim | H59: blocklist 0->1->0 |
| `unblock` | capabilities/block | `PROVEN` | sim | sim | sim | — |
| `getAbout` | contacts.AboutOf | `PARTIAL` | sim | parcial | sim | H70: o par tem recado VAZIO e em cache; a busca no servidor não foi exercitada |
| `getCommonGroups` | contacts.CommonGroups | `PROVEN` | sim | sim | sim | H118: delegação literal para `Client.getCommonGroups` (Contact.js:223) |
| `getBroadcast` | status.ByContact | `PARTIAL` | sim | sim | sim | idem `getBroadcastById` (H100) |

## GroupChat

| upstream | wa-headless | estado | unitário | SPA real | ctrl. neg. | nota |
|---|---|---|---|---|---|---|
| `owner` | group.Metadata | `PROVEN` | sim | sim | sim | provado no grupo de laboratório; o participante que é super admin também é admin — as duas flags são distintas, não a mesma lida duas vezes (H105) |
| `createdAt` | group.Metadata | `PROVEN` | sim | sim | sim | `md.creation`, em segundos; zero fica zero em vez de virar a época (H105) |
| `description` | group.Metadata | `PARTIAL` | sim | sim | sim | leitor entregue e **nunca observado não-vazio**: no grupo de laboratório `desc` E `displayedDesc` leem `undefined`, compatível com "não tem descrição" E com "campo errado". Por isso `DescriptionSource` viaja junto — `none` diz que nada veio, não que nada existe. Quinta ocorrência da classe (H105) |
| `participants` | group.Metadata | `PROVEN` | sim | sim | sim | lista com `admin`, `superAdmin` e `joinedAt`; identidades chegam como **LID**. Exatamente um super admin, travado por teste (H105) |
| `addParticipants` | group.AddParticipant | `PARTIAL` | sim | entre sessões | sim | H58: este build não confirma na MESMA sessão |
| `removeParticipants` | group.RemoveParticipant | `PARTIAL` | sim | entre sessões | sim | idem |
| `promoteParticipants` | group.Promote | `PARTIAL` | sim | entre sessões | sim | H65 |
| `demoteParticipants` | group.Demote | `PARTIAL` | sim | entre sessões | sim | H65 |
| `setSubject` | group.SetSubject | `PROVEN` | sim | sim | sim | H64: o assunto vive em chat.formattedTitle |
| `setDescription` | — | `MISSING` | — | — | — | — |
| `setAddMembersAdminsOnly` | group.SetPolicy(PolicyJoinNeedsApproval) | `PROVEN` | sim | sim | sim | H79 mediu o nome pelo oráculo do app; **H85** corrigiu a classificação: é visível NA MESMA sessão em ~1s, e a pós-condição é real |
| `setMessagesAdminsOnly` | group.SetPolicy(PolicyMessagesAdminsOnly) | `PROVEN` | sim | sim | sim | H79 mediu o nome pelo oráculo do app; **H85** corrigiu a classificação: é visível NA MESMA sessão em ~1s, e a pós-condição é real |
| `setInfoAdminsOnly` | group.SetPolicy(PolicyInfoAdminsOnly) | `PROVEN` | sim | sim | sim | H79 mediu o nome pelo oráculo do app; **H85** corrigiu a classificação: é visível NA MESMA sessão em ~1s, e a pós-condição é real |
| `deletePicture` | — | `MISSING` | — | — | — | — |
| `setPicture` | — | `MISSING` | — | — | — | — |
| `getInviteCode` | group.InviteCode | `PROVEN` | sim | sim | sim | H57: a chamada popula o MODELO; o retorno é undefined |
| `revokeInvite` | group.RevokeInvite | `PROVEN` | sim | sim | sim | H57 |
| `getGroupMembershipRequests` | groupreq.List | `PROVEN` | sim | sim | sim | refresca a metadata antes de ler; campos do registro medidos ao vivo: `id t addedBy requestMethod parentGroupId` (H89) |
| `approveGroupMembershipRequests` | groupreq.Approve | `PROVEN` | sim | sim | sim | uma chamada RPC por solicitante, resultado por solicitante; provado ao vivo do pedido ao desaparecimento (H89) |
| `rejectGroupMembershipRequests` | groupreq.Reject | `PARTIAL` | sim | não | sim | mesma RPC do approve, diferindo só na chave enviada — travado por teste unitário que casa `rejectArgs:` com os dois pontos. NÃO exercitado ao vivo: rejeitar conta-B a expulsaria do grupo de laboratório (H89) |
| `leave` | group.Leave | `PARTIAL` | sim | NÃO (por desenho) | sim | H65: conta que sai de grupo que criou não volta sem convite |

## GroupNotification

**Família inteira `MISSING`** — notificações de grupo como objeto — não atacada.

| upstream | estado |
|---|---|
| `getChat` | `PARTIAL` |
| `getContact` | `PARTIAL` |
| `getRecipients` | `MISSING` |
| `reply` | `PARTIAL` |

## Label

**Família inteira `MISSING`** — Label.getChats — não atacado.

| upstream | estado |
|---|---|
| `getChats` | `MISSING` |

## Message

| upstream | wa-headless | estado | unitário | SPA real | ctrl. neg. | nota |
|---|---|---|---|---|---|---|
| `reload` | message.CurrentOf | `PROVEN` | sim | sim | sim | H108: 32 mensagens re-lidas ao vivo deram 11 estados distintos; `hasAck` separa "ack 0" de "sem ack", medido em 5 de 395 |
| `rawData` | message.ShapeOf | `INTENTIONAL_DIFFERENCE` | sim | sim | sim | H107: devolvemos os NOMES dos campos, nunca os valores — a medição achou 598 nomes no modelo cru, entre eles `body` e `caption`; devolver rawData como é derrubaria a invariante 12 em vez de entregar funcionalidade |
| `getChat` | message.OriginOf (.ChatJID) | `PROVEN` | sim | sim | sim | H106 |
| `getContact` | message.OriginOf (.SenderJID) | `PROVEN` | sim | sim | sim | H106: 2 de 2 mensagens de grupo com remetente ≠ chat; grupo é PERGUNTADO à página (getIsGroup), não inferido do sufixo |
| `getMentions` | — | `MISSING` | — | medido | — | H106: 395 mensagens carregadas, ZERO com menção sob nenhum de cinco nomes de campo candidatos; leitor não embarcado (armadilha H93) |
| `getGroupMentions` | — | `MISSING` | — | medido | — | idem |
| `getQuotedMessage` | metadados em messagemeta | `PARTIAL` | sim | sim | sim | — |
| `reply` | send.Reply | `PROVEN` | sim | sim | sim | H54 |
| `react` | capabilities/react | `PARTIAL` | sim | parcial | sim | H53 |
| `acceptGroupV4Invite` | — | `MISSING` | — | — | — | não atacado |
| `forward` | capabilities/forward | `PROVEN` | sim | sim | sim | H63: cópia identificada por conjunto de ids, não por instante |
| `downloadMedia` | capabilities/media | `PROVEN` | sim | sim | sim | H67: pós-condição CRIPTOGRÁFICA — SHA-256 do texto claro |
| `delete` | capabilities/revoke | `PROVEN` | sim | sim | sim | direito consultado na página |
| `star` | capabilities/star | `PROVEN` | sim | sim | sim | H61: o await não é a conclusão — 696ms |
| `unstar` | capabilities/star | `PROVEN` | sim | sim | sim | — |
| `pin` | pin.Message | `MISSING` | sim | falha (H81) | sim | chamada aceita e nada é fixado; vocabulário, duração e forma do modelo medidos |
| `unpin` | pin.Unpin | `MISSING` | sim | falha (H81) | sim | idem |
| `getInfo` | capabilities/ack | `PARTIAL` | sim | sim | sim | H71: MsgInfoCollection VAZIA (0 de 368); temos ack, não "quem leu" |
| `getOrder` | — | `MISSING` | — | — | — | `WAWebBizOrderBridge.queryOrder` existe (2 chaves) e **não foi exercitado**: a conta tem zero pedidos entre as mensagens varridas, então falta o dado, não o código. A aridade declarada é 1 contra os 5 argumentos que a referência passa (H102) |
| `getPayment` | — | `MISSING` | — | — | — | idem `getOrder`: zero pagamentos na conta, caminho sem exercício (H102) |
| `getReactions` | — | `MISSING` | — | — | — | — |
| `edit` | capabilities/edit | `PROVEN` | sim | sim | sim | H60: janela de 1200s |
| `editScheduledEvent` | — | `MISSING` | — | — | — | — |
| `getPollVotes` | poll.Votes | `PROVEN` | sim | sim | sim | idem |
| `vote` | poll.Vote | `PARTIAL` | sim | não | sim | implementado e travado por teste: nomes viram ids locais NA PÁGINA, e um nome que não casa é recusa e não omissão — o envio da página recebe um SET, e um nome perdido produziria um voto por menos coisas do que se pediu, reportado como sucesso. Não provado ao vivo porque a enquete não chega ao par (ver `sendMessage`, H98) |

## MessageMedia

**Família inteira `MISSING`** — construtores de mídia — nós passamos bytes direto.

| upstream | estado |
|---|---|
| `fromFilePath` | `MISSING` |
| `fromUrl` | `MISSING` |

## Product

`capabilities/catalog` lê a vitrine de um vendedor, **provado ao vivo sem criar
nada** (H103). O caminho óbvio — acrescentar um produto ao perfil comercial desta
conta — deixaria um item real visível a quem abrisse aquele perfil;
`queryCatalog` existe para um CLIENTE abrir a loja de um vendedor, então a prova
honesta lê uma vitrine que já existe.

Medido: a conta conhece **58** perfis de negócio; dos 3 primeiros, dois
responderam `ServerStatusCodeError` (sem vitrine) e o terceiro devolveu **1
produto** com 19 campos. Sem esse terceiro, a recusa seria indistinguível de uma
capacidade quebrada — a armadilha do leitor só visto devolvendo zero (H93).

A forma da resposta **não** é a que a referência sugere: é
`{data, catalog_id, catalog_name, catalog_type, paging}`, com os itens em `data`
e não em `products`. E a chamada é POSICIONAL: o objeto de opções devolve
`CatalogUnknownError` enquanto a forma posicional chega ao servidor.

`getData` fica `PARTIAL` porque o DADO do produto é obtido — id, nome, preço,
moeda, disponibilidade, contagem de imagens — mas por vitrine inteira, não por
produto individual. No vendedor observado, `catalog_id`, `catalog_name` e o preço
vieram **vazios**, e isso está reportado em vez de assumido: uma observação não
separa "sempre vazio" de "esta loja".

| upstream | estado |
|---|---|
| `getData` | `PARTIAL` |

## Events

Trinta e um eventos no upstream. Nós temos barramento (`events/`) desde a H87,
com **duas** origens: a página (uma instalação, um buffer, uma sequência) e este
processo (fatos de ciclo de vida, que a página não tem como conhecer). As duas
não compartilham relógio nem sequência, e o `Origin` do evento diz qual é qual.

**Frescor tem TRÊS estados** desde a H96: `REPLAY`, `LIVE` e `UNKNOWN`. O
terceiro não é dúvida — é a resposta exata para um tipo sem discriminador
causal, e nunca é promovido a `LIVE` por tempo ou por taxa. Só `message.added`
alcança `LIVE`, pelo carimbo da própria mensagem. Medido: um boot entrega ~3578
eventos de hidratação, **zero** deles vivos.

O ciclo de vida chega ao barramento por uma **porta**, não por dependência:
`core` declara um callback e não conhece `events`; `events` expõe uma segunda
porta e não conhece `core`; `runtime` é o único lugar que conhece os dois lados
(H88).

| upstream | wa-headless | estado | nota |
|---|---|---|---|
| `AUTHENTICATED` | — | `MISSING` | **sem observável neste build**: o boot ou chega a APP_READY verificado ou falha; não há um degrau "credenciais aceitas, app ainda montando" que este módulo consiga distinguir (H88) |
| `AUTHENTICATION_FAILURE` | events.SessionBootFailed | `PARTIAL` | o evento carrega o ESTÁGIO do boot; um boot que morre em `not_ready` contra uma tela de QR é o caso do upstream, mas o MOTIVO (a classe da página) não viaja no evento — `BootFailure` o guarda na mensagem de erro, e mensagem de erro não entra no barramento (H88) |
| `READY` | events.SessionReady | `PROVEN` | no barramento desde a H88, e distingue um ready ORDINÁRIO de um que recuperou um perfil suspeito — a quitação da invariante 2 fica contada em vez de inferida |
| `CHAT_REMOVED` | — | `MISSING` | o método correspondente não existe (não apagamos conversa), e provar o evento exigiria destruir a fixture — a regra do barramento é que um tipo entra quando um teste o dispara SOB DEMANDA (H88) |
| `CHAT_ARCHIVED` | events.ChatChanged | `PARTIAL` | o nosso evento é grosso: diz que a conversa mudou, não QUAL campo (H87) |
| `MESSAGE_RECEIVED` | events.MessageAdded | `PROVEN` | disparado ao vivo por um envio (H87) |
| `MESSAGE_CIPHERTEXT` | — | `MISSING` | **vive abaixo do modelo**: é a mensagem antes de decifrar, e este barramento escuta COLEÇÕES, não o fio. É a linha concreta que autorizaria descer ao decodificador de stanzas, quando for atacada (H88) |
| `MESSAGE_CIPHERTEXT_FAILED` | — | `MISSING` | idem `MESSAGE_CIPHERTEXT` (H88) |
| `MESSAGE_CREATE` | events.MessageAdded | `PARTIAL` | o mesmo evento cobre os dois; o upstream distingue criada de recebida e nós não (H87) |
| `MESSAGE_REVOKED_EVERYONE` | events.MessageRevoked | `PROVEN` | disparado ao vivo; reconhecido pelo predicado de TRÊS sinais que a capacidade de apagar mede (H87) |
| `MESSAGE_REVOKED_ME` | — | `MISSING` | não temos "apagar para mim"; `revoke.ForEveryone` é o único caminho implementado. É falta de MÉTODO antes de ser falta de evento (H88) |
| `MESSAGE_ACK` | events.MessageAck | `PROVEN` | disparado ao vivo (H87) |
| `MESSAGE_EDIT` | events.MessageEdited | `PROVEN` | disparado ao vivo por uma edição (H87) |
| `UNREAD_COUNT` | events.ChatChanged | `PARTIAL` | idem |
| `MESSAGE_REACTION` | events.MessageReaction | `PARTIAL` | disparado ao vivo (H87); diz que as reações se moveram e NÃO quais são — o agregado não tem fonte neste build (H83) |
| `MEDIA_UPLOADED` | — | `MISSING` | `send.SendMedia` o dispararia, e `message.added` com `Kind` PODE já cobrir a semântica — mas isso não foi medido, e "provavelmente coberto" não é um estado deste vocabulário (H88) |
| `CONTACT_CHANGED` | events.ContactChanged | `PROVEN` | disparado sob demanda por `addressbook.Save`, sem segunda conta: 8 eventos ao nomear o par. Era o único tipo instalado e nunca provado (H90) |
| `GROUP_JOIN` | events.GroupJoined | `PARTIAL` | classificador embarcado e provado em unidade (H119). A H86 continua valendo e fica MAIS PRECISA: o portador `gp2` chega ao barramento (provado via `subject`), então o zero da H86 é dos subtipos de PARTICIPANTE na sessão que agiu — uma sessão OBSERVADORA nunca foi testada |
| `GROUP_LEAVE` | events.GroupLeft | `PARTIAL` | idem GROUP_JOIN (H86 + H119) |
| `GROUP_ADMIN_CHANGED` | events.GroupAdminChanged | `PARTIAL` | idem GROUP_JOIN (H86 + H119) |
| `GROUP_MEMBERSHIP_REQUEST` | events.ChatChanged | `PARTIAL` | a chegada MOVE o modelo nesta sessão e o barramento a vê — medida isolada: a saída de conta-B sozinha deu 5 `chat.changed`, o pedido sozinho deu **9** mais 1 `message.added` (H89). Não há tipo dedicado, e `chat.changed` é grosso demais para ser um: quem quer o pedido tem de chamar `groupreq.List`. Contraste com a H86, onde a sessão que MUDA participantes vê zero — quem recebe enxerga, quem age não |
| `GROUP_UPDATE` | events.GroupUpdated | `PROVEN` | H119: o portador `gp2` É reclassificado neste barramento — assunto do grupo trocado de propósito, `group.updated:1` com subtipo `subject`, e o assunto restaurado |
| `QR_RECEIVED` | core (pareamento) | `PROVEN` | QR nunca é logado nem versionado |
| `CODE_RECEIVED` | — | `MISSING` | **depende de uma família que não existe**: o pareamento por código não é uma fatia deste módulo (H88) |
| `LOADING_SCREEN` | — | `MISSING` | **sem observável neste build**: o loop de settle mede CLASSES de página, não progresso de carga (H88) |
| `DISCONNECTED` | events.SessionStateChanged | `PARTIAL` | emitimos desde a H88 — antes só detectávamos. Continua parcial porque o nosso é uma TRANSIÇÃO de liveness com a classe da página anexada, não o motivo de desligamento que o upstream entrega |
| `STATE_CHANGED` | events.SessionStateChanged | `PARTIAL` | emitimos desde a H88, e só na TRANSIÇÃO: repetir "ainda vivo" a cada tique é heartbeat vestido de evento. Parcial porque o vocabulário é o nosso (`ALIVE`, `PROCESS_GONE`, `APP_ABSENT`, …) e não o estado do socket do upstream — os dois não foram medidos um contra o outro |
| `BATTERY_CHANGED` | — | `BLOCKED` | H119: `WAWebBatteryStore` NÃO existe neste build (medido), e a própria referência marca o evento como depreciado e não enviado em multi-device — que é o que este build é |
| `INCOMING_CALL` | events.CallIncoming | `BLOCKED` | ouvinte instalado por `CallCollection.on('add')` — a referência não achou ouvinte e patcheia um `Map` interno; aqui a porta limpa existe. **NUNCA visto disparar**, e agora com causa isolada: `startWAWebVoipCall` resolve `undefined` e **nada se move em lugar nenhum** — cada contêiner da coleção observado por nome, `pendingOutgoingCall` fica `null`. Classe NOTHING (H82), terceira ocorrência. SEIS hipóteses eliminadas (ambiente, pilha VOIP, ordem, aba de chamadas, leitor, e o próprio veredito do app: `showCallBlockedModalIfNeeded()` devolve **false**) em H93 e H95. Reabre com EVIDÊNCIA nova, não hipótese: qualquer coisa que faça `pendingOutgoingCall` deixar de ser `null` |
| `REMOTE_SESSION_SAVED` | — | `MISSING` | **depende de uma família que não existe**: não há store remoto de sessão neste módulo, e nada a salvar em lugar nenhum (H88) |
| `VOTE_UPDATE` | — | `MISSING` | sem equivalente |

## Placar

| estado | itens | fração |
|---|---|---|
| `PROVEN` | 86 | 39% |
| `PARTIAL` | 63 | 29% |
| `BLOCKED` | 4 | 2% |
| `INTENTIONAL_DIFFERENCE` | 3 | 1% |
| `MISSING` | 64 | 29% |
| **total** | **220** | |
