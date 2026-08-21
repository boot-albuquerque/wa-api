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
| `getWWebVersion` | — | `MISSING` | — | — | — | trivial, nunca feito |
| `setDeviceName` | — | `MISSING` | — | — | — | — |
| `sendSeen` | chats.MarkRead | `PARTIAL` | sim | duvidosa | sim | **rebaixada em 2026-08-21 (H82)**: a pós-condição afirma que `chat.unreadCount` moveu NA MESMA SESSÃO, e a H78 mediu esse contador como CROSS_SESSION. A H52 provou contra um chat em que ele moveu; se generaliza é pergunta em aberto |
| `sendMessage` | send.Text / send.SendMedia / send.PollTo | `PARTIAL` | sim | sim | sim | texto, mídia, documento, figurinha e enquete OK; localização e vCard MISSING (H75) |
| `sendReaction` | capabilities/react | `PARTIAL` | sim | sim | sim | H53: adicionar provado; remover devolve Verified:false |
| `sendChannelAdminInvite` | — | `MISSING` | — | — | — | não atacado |
| `searchMessages` | — | `MISSING` | — | — | — | — |
| `getChats` | chats.List | `PROVEN` | sim | sim | sim | — |
| `getChannels` | — | `MISSING` | — | — | — | não atacado |
| `getChatById` | resolução interna às capacidades | `PARTIAL` | sim | sim | sim | existe como passo interno, não como capacidade exposta |
| `getChannelByInviteCode` | — | `MISSING` | — | — | — | não atacado |
| `getContacts` | contacts.List | `PROVEN` | sim | sim | sim | 944 -> 544 após dedup |
| `getContactById` | resolução interna | `PARTIAL` | sim | sim | sim | idem getChatById |
| `getMessageById` | varredura da MsgCollection nas capacidades | `PARTIAL` | sim | sim | sim | idem |
| `getPinnedMessages` | pin.PinnedIn | `PARTIAL` | sim | vazia | sim | o leitor funciona e a conta não tem NADA fixado; provar não-vazio exigiria fixar, que está bloqueado (H81) |
| `getInviteInfo` | — | `MISSING` | — | — | — | ler convite de terceiro; distinto de getInviteCode |
| `acceptInvite` | — | `MISSING` | — | — | — | entrar em grupo por link |
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
| `resetState` | — | `MISSING` | — | — | — | — |
| `isRegisteredUser` | spa.ResolveIdentityExpr | `PARTIAL` | sim | sim | sim | é passo interno de todo envio; não exposto |
| `getNumberId` | spa.ResolveIdentityExpr | `PARTIAL` | sim | sim | sim | idem |
| `getFormattedNumber` | — | `MISSING` | — | — | — | — |
| `getCountryCode` | — | `MISSING` | — | — | — | — |
| `createGroup` | group.Ensure | `PROVEN` | sim | sim | sim | idempotência é do fixture, não da capacidade |
| `createChannel` | — | `MISSING` | — | — | — | não atacado |
| `subscribeToChannel` | — | `MISSING` | — | — | — | não atacado |
| `unsubscribeFromChannel` | — | `MISSING` | — | — | — | não atacado |
| `transferChannelOwnership` | — | `MISSING` | — | — | — | não atacado |
| `searchChannels` | — | `MISSING` | — | — | — | não atacado |
| `deleteChannel` | — | `MISSING` | — | — | — | não atacado |
| `getLabels` | contacts.ListLabels | `PROVEN` | sim | sim | sim | H72; só mensurável por a conta ser Business |
| `getBroadcasts` | — | `MISSING` | — | — | — | família de listas de transmissão |
| `getBroadcastById` | — | `MISSING` | — | — | — | idem |
| `revokeStatusMessage` | — | `MISSING` | — | — | — | — |
| `getLabelById` | contacts.ListLabels + filtro | `PARTIAL` | sim | sim | sim | — |
| `getChatLabels` | contacts.LabelsOfChat | `PROVEN` | sim | sim | sim | — |
| `getChatsByLabelId` | — | `MISSING` | — | — | — | — |
| `getBlockedContacts` | capabilities/block | `PARTIAL` | sim | sim | sim | bloquear/desbloquear provados; LISTAR os bloqueados não é exposto |
| `setProfilePicture` | — | `MISSING` | — | — | — | — |
| `deleteProfilePicture` | — | `MISSING` | — | — | — | — |
| `addOrRemoveLabels` | contacts.AddLabel / RemoveLabel | `PROVEN` | sim | sim | sim | H72; forma medida pelo instrumento da H73 |
| `getGroupMembershipRequests` | — | `MISSING` | — | — | — | família inteira de pedidos de entrada |
| `approveGroupMembershipRequests` | — | `MISSING` | — | — | — | idem |
| `rejectGroupMembershipRequests` | — | `MISSING` | — | — | — | idem |
| `setAutoDownloadAudio` | — | `MISSING` | — | — | — | família de configurações |
| `setAutoDownloadDocuments` | — | `MISSING` | — | — | — | idem |
| `setAutoDownloadPhotos` | — | `MISSING` | — | — | — | idem |
| `setAutoDownloadVideos` | — | `MISSING` | — | — | — | idem |
| `setBackgroundSync` | — | `MISSING` | — | — | — | idem |
| `getContactDeviceCount` | — | `MISSING` | — | — | — | — |
| `syncHistory` | capabilities/fetchmessages | `PARTIAL` | sim | sim | sim | buscamos histórico de uma conversa; sincronizar não |
| `createCallLink` | — | `MISSING` | — | — | — | — |
| `sendResponseToScheduledEvent` | — | `MISSING` | — | — | — | — |
| `saveOrEditAddressbookContact` | — | `MISSING` | — | — | — | — |
| `deleteAddressbookContact` | — | `MISSING` | — | — | — | — |
| `getContactLidAndPhone` | spa.ResolveIdentityExpr | `PARTIAL` | sim | sim | sim | build LID-first: 397 de 399 mensagens sob @lid |
| `addOrEditCustomerNote` | — | `MISSING` | — | — | — | — |
| `getCustomerNote` | — | `MISSING` | — | — | — | — |
| `getPollVotes` | — | `MISSING` | — | — | — | enquete é enviável (H69), votos não são lidos |

## Broadcast

**Família inteira `MISSING`** — listas de transmissão — família inteira não atacada.

| upstream | estado |
|---|---|
| `getChat` | `MISSING` |
| `getContact` | `MISSING` |

## Call

**Família inteira `MISSING`** — chamadas — família inteira não atacada.

| upstream | estado |
|---|---|
| `reject` | `MISSING` |

## Channel

**Família inteira `MISSING`** — canais/newsletters — família inteira não atacada.

| upstream | estado |
|---|---|
| `getSubscribers` | `MISSING` |
| `setSubject` | `MISSING` |
| `setDescription` | `MISSING` |
| `setProfilePicture` | `MISSING` |
| `setReactionSetting` | `MISSING` |
| `mute` | `MISSING` |
| `unmute` | `MISSING` |
| `sendMessage` | `MISSING` |
| `sendSeen` | `MISSING` |
| `sendChannelAdminInvite` | `MISSING` |
| `acceptChannelAdminInvite` | `MISSING` |
| `revokeChannelAdminInvite` | `MISSING` |
| `demoteChannelAdmin` | `MISSING` |
| `transferChannelOwnership` | `MISSING` |
| `fetchMessages` | `MISSING` |
| `deleteChannel` | `MISSING` |

## Chat

| upstream | wa-headless | estado | unitário | SPA real | ctrl. neg. | nota |
|---|---|---|---|---|---|---|
| `sendMessage` | — | `MISSING` | — | — | — | não atacado |
| `sendSeen` | — | `MISSING` | — | — | — | não atacado |
| `clearMessages` | chats.Clear | `PARTIAL` | sim | NÃO (por desenho) | sim | H66: destruiria o fixture de todos os outros testes |
| `delete` | chats.Delete | `PARTIAL` | sim | NÃO (por desenho) | sim | idem |
| `archive` | — | `MISSING` | — | — | — | não atacado |
| `unarchive` | — | `MISSING` | — | — | — | não atacado |
| `pin` | — | `MISSING` | — | — | — | não atacado |
| `unpin` | — | `MISSING` | — | — | — | não atacado |
| `mute` | — | `MISSING` | — | — | — | não atacado |
| `unmute` | — | `MISSING` | — | — | — | não atacado |
| `markUnread` | chats.MarkUnread | `MISSING` | sim | falha (H78) | sim | duas primitivas medidas e nenhuma marca: `sendConversationSeen` com delta negativo e `Cmd.markChatUnread`. O `Cmd` é barramento de EVENTOS, e o ouvinte deste verbo vive num pedaço de UI que sessão headless não carrega |
| `fetchMessages` | capabilities/fetchmessages | `PROVEN` | sim | sim | sim | — |
| `sendStateTyping` | capabilities/chatstate | `PROVEN` | sim | sim | sim | — |
| `sendStateRecording` | presence.StateRecording | `PARTIAL` | sim | bloqueada (H50) | sim | **linha corrigida**: eu a marquei MISSING de memória e ela JÁ EXISTIA, mapeada para `markRecording`. A prova ao vivo esbarra no mesmo bloqueio da observação de presença |
| `clearState` | capabilities/chatstate | `PROVEN` | sim | sim | sim | — |
| `getContact` | — | `MISSING` | — | — | — | não atacado |
| `getLabels` | — | `MISSING` | — | — | — | não atacado |
| `changeLabels` | — | `MISSING` | — | — | — | não atacado |
| `getPinnedMessages` | pin.PinnedIn | `PARTIAL` | sim | vazia | sim | o leitor funciona e a conta não tem NADA fixado; provar não-vazio exigiria fixar, que está bloqueado (H81) |
| `syncHistory` | — | `MISSING` | — | — | — | não atacado |
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
| `getProfilePicUrl` | — | `MISSING` | — | — | — | não atacado |
| `getFormattedNumber` | — | `MISSING` | — | — | — | não atacado |
| `getCountryCode` | — | `MISSING` | — | — | — | não atacado |
| `getChat` | — | `MISSING` | — | — | — | não atacado |
| `block` | capabilities/block | `PROVEN` | sim | sim | sim | H59: blocklist 0->1->0 |
| `unblock` | capabilities/block | `PROVEN` | sim | sim | sim | — |
| `getAbout` | contacts.AboutOf | `PARTIAL` | sim | parcial | sim | H70: o par tem recado VAZIO e em cache; a busca no servidor não foi exercitada |
| `getCommonGroups` | — | `MISSING` | — | — | — | não atacado |
| `getBroadcast` | — | `MISSING` | — | — | — | — |

## GroupChat

| upstream | wa-headless | estado | unitário | SPA real | ctrl. neg. | nota |
|---|---|---|---|---|---|---|
| `owner` | — | `MISSING` | — | — | — | não atacado |
| `createdAt` | — | `MISSING` | — | — | — | não atacado |
| `description` | — | `MISSING` | — | — | — | não atacado |
| `participants` | — | `MISSING` | — | — | — | não atacado |
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
| `getGroupMembershipRequests` | — | `MISSING` | — | — | — | não atacado |
| `approveGroupMembershipRequests` | — | `MISSING` | — | — | — | não atacado |
| `rejectGroupMembershipRequests` | — | `MISSING` | — | — | — | não atacado |
| `leave` | group.Leave | `PARTIAL` | sim | NÃO (por desenho) | sim | H65: conta que sai de grupo que criou não volta sem convite |

## GroupNotification

**Família inteira `MISSING`** — notificações de grupo como objeto — não atacada.

| upstream | estado |
|---|---|
| `getChat` | `MISSING` |
| `getContact` | `MISSING` |
| `getRecipients` | `MISSING` |
| `reply` | `MISSING` |

## Label

**Família inteira `MISSING`** — Label.getChats — não atacado.

| upstream | estado |
|---|---|
| `getChats` | `MISSING` |

## Message

| upstream | wa-headless | estado | unitário | SPA real | ctrl. neg. | nota |
|---|---|---|---|---|---|---|
| `reload` | — | `MISSING` | — | — | — | — |
| `rawData` | — | `MISSING` | — | — | — | não atacado |
| `getChat` | — | `MISSING` | — | — | — | não atacado |
| `getContact` | — | `MISSING` | — | — | — | não atacado |
| `getMentions` | — | `MISSING` | — | — | — | — |
| `getGroupMentions` | — | `MISSING` | — | — | — | — |
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
| `getOrder` | — | `MISSING` | — | — | — | família de comércio |
| `getPayment` | — | `MISSING` | — | — | — | idem |
| `getReactions` | — | `MISSING` | — | — | — | — |
| `edit` | capabilities/edit | `PROVEN` | sim | sim | sim | H60: janela de 1200s |
| `editScheduledEvent` | — | `MISSING` | — | — | — | — |
| `getPollVotes` | — | `MISSING` | — | — | — | — |
| `vote` | — | `MISSING` | — | — | — | — |

## MessageMedia

**Família inteira `MISSING`** — construtores de mídia — nós passamos bytes direto.

| upstream | estado |
|---|---|
| `fromFilePath` | `MISSING` |
| `fromUrl` | `MISSING` |

## Product

**Família inteira `MISSING`** — catálogo — família de comércio não atacada.

| upstream | estado |
|---|---|
| `getData` | `MISSING` |

## Events

Trinta e um eventos no upstream. Nós **não temos barramento de eventos**: as
capacidades que observam mudanças o fazem por assinatura pontual. Isso é
divergência estrutural, e está aqui como tal em vez de espalhada por linhas.

| upstream | wa-headless | estado | nota |
|---|---|---|---|
| `AUTHENTICATED` | — | `MISSING` | sem equivalente |
| `AUTHENTICATION_FAILURE` | — | `MISSING` | sem equivalente |
| `READY` | core.StartSession | `PROVEN` | — |
| `CHAT_REMOVED` | — | `MISSING` | sem equivalente |
| `CHAT_ARCHIVED` | events.ChatChanged | `PARTIAL` | o nosso evento é grosso: diz que a conversa mudou, não QUAL campo (H87) |
| `MESSAGE_RECEIVED` | events.MessageAdded | `PROVEN` | disparado ao vivo por um envio (H87) |
| `MESSAGE_CIPHERTEXT` | — | `MISSING` | sem equivalente |
| `MESSAGE_CIPHERTEXT_FAILED` | — | `MISSING` | sem equivalente |
| `MESSAGE_CREATE` | events.MessageAdded | `PARTIAL` | o mesmo evento cobre os dois; o upstream distingue criada de recebida e nós não (H87) |
| `MESSAGE_REVOKED_EVERYONE` | events.MessageRevoked | `PROVEN` | disparado ao vivo; reconhecido pelo predicado de TRÊS sinais que a capacidade de apagar mede (H87) |
| `MESSAGE_REVOKED_ME` | — | `MISSING` | sem equivalente |
| `MESSAGE_ACK` | events.MessageAck | `PROVEN` | disparado ao vivo (H87) |
| `MESSAGE_EDIT` | events.MessageEdited | `PROVEN` | disparado ao vivo por uma edição (H87) |
| `UNREAD_COUNT` | events.ChatChanged | `PARTIAL` | idem |
| `MESSAGE_REACTION` | — | `MISSING` | sem equivalente |
| `MEDIA_UPLOADED` | — | `MISSING` | sem equivalente |
| `CONTACT_CHANGED` | events.ContactChanged | `PARTIAL` | instalado; NÃO provado neste barramento — nada aqui faz outra conta mudar o perfil (H87) |
| `GROUP_JOIN` | — | `MISSING` | sem equivalente |
| `GROUP_LEAVE` | — | `MISSING` | sem equivalente |
| `GROUP_ADMIN_CHANGED` | — | `MISSING` | sem equivalente |
| `GROUP_MEMBERSHIP_REQUEST` | — | `MISSING` | sem equivalente |
| `GROUP_UPDATE` | — | `MISSING` | sem equivalente |
| `QR_RECEIVED` | core (pareamento) | `PROVEN` | QR nunca é logado nem versionado |
| `CODE_RECEIVED` | — | `MISSING` | sem equivalente |
| `LOADING_SCREEN` | — | `MISSING` | sem equivalente |
| `DISCONNECTED` | capabilities/liveness | `PARTIAL` | detectamos sessão morta; não emitimos evento |
| `STATE_CHANGED` | capabilities/liveness | `PARTIAL` | idem |
| `BATTERY_CHANGED` | — | `MISSING` | sem equivalente |
| `INCOMING_CALL` | — | `MISSING` | sem equivalente |
| `REMOTE_SESSION_SAVED` | — | `MISSING` | sem equivalente |
| `VOTE_UPDATE` | — | `MISSING` | sem equivalente |

## Placar

| estado | itens | fração |
|---|---|---|
| `PROVEN` | 41 | 18% |
| `PARTIAL` | 35 | 15% |
| `BLOCKED` | 2 | 0% |
| `INTENTIONAL_DIFFERENCE` | 2 | 0% |
| `MISSING` | 140 | 63% |
| **total** | **220** | |
