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

## Critério de encerramento da Fase 1

**Decidido pela orquestração em 2026-08-22 (decisão 52), e CONFIRMADO por ela na
decisão 62.**

O texto original chegou truncado em *"Fase 1 fecha com zero MISSING e zero
PARTIAL acion…"* — a quarta truncagem seguida daquele canal —, e eu completei
uma única palavra por inferência, registrando aqui que era inferência minha.
Perguntei de volta em vez de tratar como definitivo, e a resposta foi
literal: *"Confirmo, o termo era PARTIAL acionável."*

A ressalva sai. O critério é:

- **`MISSING` tem de chegar a zero.** Nenhuma linha pode continuar dizendo "não
  atacado" — ou vira `PROVEN`, ou ganha veredito medido (`BLOCKED`,
  `INTENTIONAL_DIFFERENCE`, `PARTIAL`).
- **`PARTIAL` acionável tem de chegar a zero.** Um `PARTIAL` só sobrevive se a
  metade que falta for impossível por MEDIÇÃO, com a evidência na nota. `PARTIAL`
  por trabalho não feito não fecha a fase.

Isto substitui a leitura literal anterior ("0 MISSING e 0 PARTIAL"), que era
inatingível: linhas como `MEDIA_UPLOADED` (não existe momento de upload nesta
página) e `resetState` (a transição de ~450ms não é observável pelo Go) são
`PARTIAL` por impossibilidade, não por preguiça.

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
| `requestPairingCode` | — | `BLOCKED` | — | medido | — | H122: NÃO é falta de máquina — `WAWebAltDeviceLinkingApi` existe com `setPairingType`, `initializeAltDeviceLinking` e `startAltLinkingFlow`, e é alcançável SEM o `AuthStore` que a referência injeta (medido `hasWwebjsAuthStore:false`). O bloqueio é de ESTADO: o fluxo só roda com o socket em `UNPAIRED`/`UNPAIRED_IDLE`, e o nosso lê `CONNECTED`. Desemparelhar exige humano |
| `cancelPairingCode` | — | `BLOCKED` | — | medido | — | H122: `WAWebLaunchSocketUtils.refreshQR` existe; mesmo bloqueio de estado do `requestPairingCode` — não há código de pareamento ativo para cancelar numa sessão pareada |
| `attachEventListeners` | capabilities/messagemeta + contacts.onContact | `PARTIAL` | sim | sim | sim | dois fluxos de 31 eventos |
| `initWebVersionCache` | — | `INTENTIONAL_DIFFERENCE` | — | — | — | não fixamos versão da web; o inventário de módulos é a nossa guarda |
| `destroy` | core.Session.Stop | `PROVEN` | sim | sim | sim | stopped_via medido |
| `logout` | — | `BLOCKED` | — | medido | — | H122: `Socket.logout` EXISTE neste build. O bloqueio é de política, não técnico: desemparelha a conta e exige um humano com o telefone para restaurar. Não é exercitável por agente |
| `getWWebVersion` | spa.WebVersion | `PROVEN` | sim | sim | sim | H111: lido ao vivo (`2.3000.1045798079`); página sem versão é `ErrNoWebVersion`, não string vazia |
| `setDeviceName` | — | `INTENTIONAL_DIFFERENCE` | — | medido | — | H122: a referência REMENDA `WAWebMiscBrowserUtils.info`. Medido: o módulo existe e `info` NÃO é função — o remendo falharia. E remendar global de página é o que a H112 recusou. Além disso o nome só aparece no PAREAMENTO, que uma sessão já pareada não exercita. Não implementado de propósito |
| `sendSeen` | chats.MarkRead | `PARTIAL` | sim | duvidosa | sim | **rebaixada em 2026-08-21 (H82)**: a pós-condição afirma que `chat.unreadCount` moveu NA MESMA SESSÃO, e a H78 mediu esse contador como CROSS_SESSION. A H52 provou contra um chat em que ele moveu; se generaliza é pergunta em aberto |
| `sendMessage` | send.Text / send.SendMedia / send.PollTo | `PARTIAL` | sim | sim | sim | texto, mídia, documento e figurinha OK. **ENQUETE NÃO SAI**: criada localmente como `poll_creation` com as opções intactas, `ack` fica em **0** e o par nunca recebe — medido no primeiro round trip que olhou o OUTRO lado (H98). A H69 provou o envio pela aparição LOCAL. O suspeito nomeado (`pollType` omitido) foi **perseguido e descartado**: o enum foi achado em `WAWebPollCreationUtils` (singular — uma letra é por que três buscas o perderam), `PollType.POLL` e `PollContentType.TEXT` foram aplicados de dentro da própria página, e o ack continua 0 (H101). Localização e vCard MISSING (H75) |
| `sendReaction` | capabilities/react | `PARTIAL` | sim | sim | sim | H53: adicionar provado; remover devolve Verified:false |
| `sendChannelAdminInvite` | channel (admin) | `PROVEN` | sim | sim | sim | H136: **provado com SESSÃO DUPLA** (H135). conta-A cria canal e convida conta-B; a página responde `messageSendResult: OK` e o convite é ACEITÁVEL — provado pelo passo seguinte funcionar. O convidado é resolvido por `lookup.NumberID` antes: procurar pelo jid de telefone não acha o chat, porque este build arquiva sob LID (lição da H34) |
| `searchMessages` | search.Messages | `PROVEN` | sim | sim | sim | H114: o resultado é ENDEREÇO, nunca corpo — a projeção acontece na página. Sem escopo por conversa: passar o jid como quarto argumento (o que a referência faz com `options.chatId`) mediu 0 resultados com `eof`, contra 20 sem escopo |
| `getChats` | chats.List | `PROVEN` | sim | sim | sim | — |
| `getChannels` | channel.Followed | `PROVEN` | sim | sim | sim | H123: leitor provado NÃO-VAZIO — `Followed` devolveu 1 entrada com `membership=owner` logo após criar um canal, e 0 depois de apagá-lo. Assinar canal alheio para provar é impossível neste build (ver `subscribeToChannel`), então a prova veio de um canal PRÓPRIO, que vive na mesma coleção. **Ressalva medida na H139**: `Followed` reflete o CACHE do cliente, não o servidor. Quando OUTRA conta apaga um canal do qual esta é admin ou inscrita, o modelo local FICA — foram 6 sobras encontradas com `serverAlive:false`. A prova desta linha continua válida (mesmo dono criou e apagou, contagem 1 e depois 0), mas quem chama precisa saber o que está lendo |
| `getChatById` | chats.ByJID | `PROVEN` | sim | sim | sim | H128: deixou de ser passo interno. Reusa a projeção provada de `chats.List` em vez de escrever uma segunda consulta — duas projeções divergiriam justo nos campos que ninguém reconfere (arquivado, mudo, somente-leitura). Ao vivo: a lista trunca em 100 de 384 e o `ByJID` acha a que ordena por ÚLTIMO, e reporta as 384 varridas |
| `getChannelByInviteCode` | channel.ByInviteCode | `PROVEN` | sim | sim | sim | provado contra canal público real: jid `@newsletter`, nome, **45.460 assinantes**, `state=active`, `verification=verified`, e `following=false` — **nada foi seguido**. Aceita o link inteiro, não só o código (H104) |
| `getContacts` | contacts.List | `PROVEN` | sim | sim | sim | 944 -> 544 após dedup |
| `getContactById` | contacts.ByJID | `PROVEN` | sim | sim | sim | H129: fecha o padrão da H127. Casa em QUALQUER das duas identidades, porque o roster mescla linha de telefone e de lid numa só — casar por um campo só perderia a pessoa sob o outro nome, que é a classe de defeito da H34. Ao vivo: 521 contatos de 945 linhas, 421 mesclados, e um mesclado alcançável pelas duas identidades; 255 sem nome, todos encontráveis |
| `getMessageById` | capabilities/message | `PROVEN` | sim | sim | sim | H127: deixou de ser varredura interna — `message.OriginOf`, `CurrentOf` e `ShapeOf` recebem o id cru e foram provados ao vivo (H106, H107, H108) |
| `getPinnedMessages` | pin.PinnedIn | `PARTIAL` | sim | vazia | sim | o leitor funciona e a conta não tem NADA fixado; provar não-vazio exigiria fixar, que está bloqueado (H81) |
| `getInviteInfo` | group.InviteInfo | `PROVEN` | sim | sim | sim | lê o grupo atrás de um link SEM entrar; provado ao vivo reportando `approval=true` no grupo armado (H89) |
| `acceptInvite` | group.JoinByInvite | `PARTIAL` | sim | sim | sim | provado ao vivo o caminho de APROVAÇÃO: o page REJEITA com `UnexpectedJoinGroupViaInviteResponse` carregando `gid` e `membershipApprovalMode`, e isso É a criação do pedido. O caminho de entrada direta (grupo sem aprovação) não foi exercitado (H89) |
| `acceptChannelAdminInvite` | channel (admin) | `PROVEN` | sim | sim | sim | H136: provado com sessão dupla. conta-B aceita e o canal vai de **0 para 1 assinante** — pós-condição independente da chamada não ter lançado |
| `revokeChannelAdminInvite` | channel (admin) | `PROVEN` | sim | sim | sim | H136: provado pela ORDEM CERTA. Testado depois do aceite, o servidor responde `Not Allowed`, que é a resposta correta para convite já consumido e medição errada da capacidade. Revogando ANTES: o aceite seguinte falha com `Not Found` e os assinantes ficam em **0** — a revogação é provada pelo aceite FALHAR, não pela chamada não lançar |
| `demoteChannelAdmin` | — | `BLOCKED` | — | medido | — | H137: **a forma foi resolvida** — `demoteNewsletterAdminAction(modeloDoCanal, modeloDoContato)`, aridade 2, e a chamada responde ok. Três tentativas antes falharam por nome ou forma inferidos; só ENUMERAR o módulo resolveu. Fica `BLOCKED` e não `PROVEN` porque a pós-condição NÃO EXISTE: verificar exigiria ler a lista de admins, e `WAWebMexFetchNewsletterSubscribersJob` não existe neste build (H113). A chamada funcionar não é a coisa acontecer |
| `acceptGroupV4Invite` | — | `BLOCKED` | — | medido | — | H125: bloqueio DUPLO, medido. `WAWebGroupInviteV4Job` existe mas **nenhuma** das duas funções que a referência chama existe nele — décimo desencontro com a lista do wwebjs. E a conta tem zero convites v4 |
| `setStatus` | capabilities/profile (parcial) | `BLOCKED` | — | — | — | H66: setMyTextStatus tem 5 primitivos e ZERO chamadores no bundle |
| `setDisplayName` | profile.SetDisplayName | `BLOCKED` | sim | impossível | sim | H66: canSetMyPushname()=false — a conta é Business |
| `getState` | capabilities/liveness | `PROVEN` | sim | sim | sim | pior latência 1ms em 5 amostras |
| `sendPresenceAvailable` | capabilities/presence | `PARTIAL` | sim | parcial | sim | H50 + **H144**: observação não provada, e a causa agora é MEDIDA em vez de suposta. Com as duas contas acordadas ao mesmo tempo (sessão dupla), `Observe` nunca chega a `subscribed` em 45 s. O par está na coleção com PN e LID fundidos, e os sinalizadores da agenda leem `isMyContact:false isAddressBookContact:false isWAContact:false` — a assinatura de presença exige o vínculo de agenda, que se cria no TELEFONE. Passa a ser dependência humana explícita, não pendência deste módulo |
| `sendPresenceUnavailable` | capabilities/presence | `PARTIAL` | sim | parcial | sim | H50 + **H144**: idem `sendPresenceAvailable` — a sessão dupla mediu `isMyContact:false isAddressBookContact:false` e a assinatura nunca chega. Dependência humana (salvar o contato no telefone), não pendência do módulo. *(era `idem`, expandido na H130 — referência por posição de linha já produziu um `idem` pendurado)* |
| `archiveChat` | chats (arquivar) | `PROVEN` | sim | sim | sim | H55: causa era pedido REDUNDANTE |
| `unarchiveChat` | chats | `PROVEN` | sim | sim | sim | — |
| `pinChat` | chats | `PROVEN` | sim | sim | sim | — |
| `unpinChat` | chats | `PROVEN` | sim | sim | sim | — |
| `muteChat` | capabilities/mute | `PROVEN` | sim | sim | sim | H62: sendDevice:true é o que faz o efeito sair do dispositivo |
| `unmuteChat` | capabilities/mute | `PROVEN` | sim | sim | sim | — |
| `markChatUnread` | — | `BLOCKED` | sim | falha (H78) | sim | H140: reclassificado de `MISSING` para `BLOCKED` (decisão 60). A H78 mediu DUAS primitivas e nenhuma marca; o ouvinte do verbo vive num pedaço de UI que sessão headless não carrega. Está fora do alcance deste módulo, não por fazer |
| `getProfilePicUrl` | capabilities/avatar | `PROVEN` | sim | sim | sim | H41 |
| `getCommonGroups` | contacts.CommonGroupsWith | `PROVEN` | sim | sim | sim | H68: null significa "sou eu", não "nenhum" |
| `resetState` | liveness.Reset | `PARTIAL` | sim | sim | sim | H116: a chamada é feita e o socket é provado SAUDÁVEL depois; provar que ela FEZ algo não passa pelo Go — a transição dura ~450ms e cada leitura é um ida-e-volta do chromedp (0 de 3 ao vivo, contra 9 de 100 amostrando DENTRO da página). A referência não devolve nada, nem erro |
| `isRegisteredUser` | lookup.NumberID | `PROVEN` | sim | sim | sim | H147: a nota estava METADE obsoleta e METADE certa por outro motivo. Obsoleta porque a H127 expôs a resolução como capacidade; certa porque toda prova ao vivo até hoje perguntara sobre um número que EXISTE — e uma capacidade só vista dizendo "sim" passa em qualquer teste que só pergunte por números reais. Exercitada a resposta NEGATIVA, com controle positivo na MESMA sessão: o par resolve, o número implausível volta em `ErrNotOnWhatsApp` e não num falso positivo. A referência define `isRegisteredUser` como `Boolean(await getNumberId(id))`, então é literalmente esta linha |
| `getNumberId` | lookup.NumberID | `PROVEN` | sim | sim | sim | H127: deixou de ser passo interno e virou capacidade. A expressão de resolução é EMBUTIDA, não copiada — uma cópia divergiria da que o `send` usa. Provado com três casos: número real resolve para outra identidade, número impossível dá `ErrNotOnWhatsApp` definitivo, e grupo curto-circuita em vez de ser reportado ausente |
| `getFormattedNumber` | phone.Lookup (.Formatted) | `PROVEN` | sim | sim | sim | H111: a página NÃO recusa lixo — `findCC("notaphone")` devolve `"not"`, medido. Guardamos dos dois lados: a entrada tem de ser dígitos e a RESPOSTA também, e as duas guardas foram provadas independentes por controle negativo |
| `getCountryCode` | phone.Lookup (.CountryCode) | `PROVEN` | sim | sim | sim | H111: a página NÃO recusa lixo — `findCC("notaphone")` devolve `"not"`, medido. Guardamos dos dois lados: a entrada tem de ser dígitos e a RESPOSTA também, e as duas guardas foram provadas independentes por controle negativo |
| `createGroup` | group.Ensure | `PROVEN` | sim | sim | sim | idempotência é do fixture, não da capacidade |
| `createChannel` | channel.Create | `PROVEN` | sim | sim | sim | H113: canal real criado e apagado na conta de laboratório, com autorização explícita. Verificado relendo pelo código de convite, não pelo eco da própria chamada; gate desabilitado é erro PRÓPRIO (a referência devolve a mensagem como STRING) |
| `subscribeToChannel` | — | `BLOCKED` | — | medido | — | H138: **a causa foi isolada** e é mais geral que a linha. A coleção de newsletters deste build está sem a maquinaria de BUSCA: `findImpl` ausente (H123) e `markFetchStart` ausente (medido agora). `subscribeToNewsletterAction` (aridade 3) quer um modelo memoizado que a busca quebrada não produz; `subscribeToNewsletterWidAction` (aridade 2, que a referência NÃO chama e só apareceu ao enumerar o módulo) aceita o Wid e morre dentro, no mesmo buraco. Não é a assinatura que falta: é a coleção que não busca |
| `unsubscribeFromChannel` | — | `BLOCKED` | — | medido | — | H138: **a causa foi isolada** e é mais geral que a linha. A coleção de newsletters deste build está sem a maquinaria de BUSCA: `findImpl` ausente (H123) e `markFetchStart` ausente (medido agora). `subscribeToNewsletterAction` (aridade 3) quer um modelo memoizado que a busca quebrada não produz; `subscribeToNewsletterWidAction` (aridade 2, que a referência NÃO chama e só apareceu ao enumerar o módulo) aceita o Wid e morre dentro, no mesmo buraco. Não é a assinatura que falta: é a coleção que não busca |
| `transferChannelOwnership` | channel (admin) | `PROVEN` | sim | sim | sim | H139: **provado com autorização explícita e a limpeza mudando de mãos**. conta-A cria o canal, convida conta-B como admin, B aceita, e A transfere: `changeNewsletterOwnerAction(modeloDoCanal, modeloDoContato)` — as formas que a H137 estabeleceu. Pós-condição de verdade: a membership de B lida do lado DELE volta `owner`. E o canal é apagado **por B**, porque quem deixou de ser dono não pode apagar — a recusa da H136 não era do verbo, era do teardown |
| `searchChannels` | channel.Search | `PROVEN` | sim | sim | sim | H112: o diretório RESPONDE (50 resultados) — ao contrário de `getRecommendedNewsletters`, que trava. Sem assinatura. Tipo próprio `DirectoryEntry`: um resultado de diretório é um MODELO com campos `__x_`, não o saco de mixins da consulta de metadados, e `__x_state` não existe. Sem opção `limit`: a referência a implementa remendando uma função da página que não existe neste build |
| `deleteChannel` | channel.Delete | `PROVEN` | sim | sim | sim | H113: pós-condição é o canal deixar de ser legível; apagar sem código de convite devolve erro dizendo que NÃO deu para verificar, em vez de sucesso |
| `getLabels` | contacts.ListLabels | `PROVEN` | sim | sim | sim | H72; só mensurável por a conta ser Business |
| `getBroadcasts` | status.List | `PARTIAL` | sim | sim | sim | **NÃO é lista de transmissão** — o upstream chama de Broadcast o STATUS (stories): `getBroadcasts` é `Status.getModelsArray`. Nossa nota descrevia a coisa errada, e três linhas iam ser feitas contra a ideia errada (H100). Caminho provado ao vivo, com **zero** feeds; provar um não-vazio exige POSTAR status, visível aos 944 contatos da conta |
| `getBroadcastById` | status.ByContact | `PARTIAL` | sim | sim | sim | tenta as duas formas de identidade; feed ausente é erro próprio e não um feed de zeros, que um chamador leria como "essa pessoa não postou nada" (H100) |
| `revokeStatusMessage` | — | `BLOCKED` | — | medido | — | H134: `WAWebRevokeStatusAction` **existe**. O bloqueio é de DADO e de decisão humana: não há status postado para revogar, e postar um é visível a 944 contatos — decisão que a H100 deixou para o humano |
| `getLabelById` | contacts.LabelByID | `PROVEN` | sim | sim | sim | H128: filtra a lista provada (H72) em vez de consultar a página de novo. Rótulo com contagem ZERO continua sendo rótulo — os 3 desta conta têm zero itens (H114), e tratar zero como ausente encontraria nenhum |
| `getChatLabels` | contacts.LabelsOfChat | `PROVEN` | sim | sim | sim | — |
| `getChatsByLabelId` | — | `BLOCKED` | — | medido | — | H114: os 3 rótulos desta conta têm ZERO itens (`chatLabelItems: 0`), então um leitor nunca seria visto devolvendo nada — armadilha H93 |
| `getBlockedContacts` | block.List | `PROVEN` | sim | sim | sim | H146: "não é exposto" era afirmação sobre a NOSSA superfície e nunca fora conferida contra a página. `WAWebCollections.Blocklist` responde `getModelsArray` — lia 0 porque ninguém está bloqueado. O caminho de trás é que NÃO existe: de 945 contatos, ZERO carregam `isBlocked`, então filtrar o roster devolveria vazio para sempre. Provado com restauração: 0 → 1 nomeando o par → 0 |
| `setProfilePicture` | — | `BLOCKED` | — | medido | — | H134: o módulo que a referência usa **não existe neste build** (medido) — `WAWebSetPicture` e `WAWebProfilePicThumbBridge` ausentes |
| `deleteProfilePicture` | — | `BLOCKED` | — | medido | — | H134: o módulo que a referência usa **não existe neste build** (medido) — idem `setProfilePicture` |
| `addOrRemoveLabels` | contacts.AddLabel / RemoveLabel | `PROVEN` | sim | sim | sim | H72; forma medida pelo instrumento da H73 |
| `getGroupMembershipRequests` | groupreq.List | `PROVEN` | sim | sim | sim | refresca a metadata antes de ler; campos do registro medidos ao vivo: `id t addedBy requestMethod parentGroupId` (H89) |
| `approveGroupMembershipRequests` | groupreq.Approve | `PROVEN` | sim | sim | sim | uma chamada RPC por solicitante, resultado por solicitante; provado ao vivo do pedido ao desaparecimento (H89) |
| `rejectGroupMembershipRequests` | groupreq.Reject | `PARTIAL` | sim | não | sim | mesma RPC do approve, diferindo só na chave enviada — travado por teste unitário que casa `rejectArgs:` com os dois pontos. NÃO exercitado ao vivo: rejeitar conta-B a expulsaria do grupo de laboratório (H89) |
| `setAutoDownloadAudio` | settings.SetAutoDownload(KindAudio) | `PROVEN` | sim | sim | sim | H110: a referência devolve o flag pedido sem olhar; nós relemos e falhamos com `ErrNotTaken` se a página não moveu. Escrita redundante é evitada (H55) e DITA em `Changed` |
| `setAutoDownloadDocuments` | settings.SetAutoDownload(KindDocuments) | `PROVEN` | sim | sim | sim | H110; as quatro categorias viradas ao vivo e o baseline restaurado e verificado |
| `setAutoDownloadPhotos` | settings.SetAutoDownload(KindPhotos) | `PROVEN` | sim | sim | sim | H110 |
| `setAutoDownloadVideos` | settings.SetAutoDownload(KindVideos) | `PROVEN` | sim | sim | sim | H110 |
| `setBackgroundSync` | settings.SetBackgroundSync | `PROVEN` | sim | sim | sim | H110: o valor guardado é lido de volta na hora; a referência avisa que o EFEITO só vale após reiniciar, e nada aqui afirma que a sessão viva mudou |
| `getContactDeviceCount` | addressbook.DeviceCount | `PROVEN` | sim | sim | sim | H148: o registro SEMPRE esteve lá — sob a identidade RESOLVIDA. Medidos os dois jids lado a lado na mesma sessão: pelo LID, **5 dispositivos**; pelo jid de telefone, "sem registro". A H90 mediu contra o telefone num build LID-first, antes de a H136 nomear essa armadilha. A decisão de manter "sem registro" fora do número 0 continua certa e agora tem os dois lados observados |
| `syncHistory` | capabilities/fetchmessages | `PARTIAL` | sim | sim | sim | buscamos histórico de uma conversa; sincronizar não |
| `createCallLink` | call.CreateLink | `PROVEN` | sim | sim | sim | provado ao vivo para `voice` e `video`; o link é credencial e nunca é renderizado. Usa `WAWebGenerateEventCallLink` como a referência — o `WAWebVoipCreateCallLink` deste build **TRAVA** na primeira chamada, medido em 40s (H92) |
| `sendResponseToScheduledEvent` | — | `BLOCKED` | — | medido | — | H134: o módulo que a referência usa **não existe neste build** (medido) — `WAWebScheduledEventResponseAction` ausente, coerente com a H125 |
| `saveOrEditAddressbookContact` | addressbook.Save | `PROVEN` | sim | sim | sim | verifica lendo de volta, com o relógio no Go; `syncToAddressbook` é parâmetro sem padrão porque `true` escreve na agenda do TELEFONE pareado (H90) |
| `deleteAddressbookContact` | addressbook.Delete | `PROVEN` | sim | sim | sim | idempotente, medido; exige **wid**, enquanto o save exige dígitos crus — assimetria que a referência esconde passando o mesmo valor aos dois (H90) |
| `getContactLidAndPhone` | spa.ResolveIdentityExpr | `PARTIAL` | sim | sim | sim | build LID-first: 397 de 399 mensagens sob @lid |
| `addOrEditCustomerNote` | — | `BLOCKED` | — | medido | — | H134: **as ações EXISTEM** — `noteAddAction` e `retrieveOnlyNoteForChatJid`, exatamente as que a referência chama. O que falta é `WAWebBizGatingUtils`, o módulo do PORTÃO (`smbNotesV1Enabled`), ausente neste build: dá para chamar a ação e não dá para saber se o recurso deveria estar ligado |
| `getCustomerNote` | — | `BLOCKED` | — | medido | — | H134: **as ações EXISTEM** — `noteAddAction` e `retrieveOnlyNoteForChatJid`, exatamente as que a referência chama. O que falta é `WAWebBizGatingUtils`, o módulo do PORTÃO (`smbNotesV1Enabled`), ausente neste build: dá para chamar a ação e não dá para saber se o recurso deveria estar ligado |
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

| upstream | estado | nota |
|---|---|---|
| `getChat` | `PARTIAL` | H118: delegação literal para `Client.getChatById` (Broadcast.js:56); herda a linha do par |
| `getContact` | `PARTIAL` | H118: delegação literal para `Client.getContactById` (Broadcast.js:64); herda a linha do par |

## Call

`capabilities/call` entrega o link (provado) e a recusa (implementada, não
disparada). O que falta é **um gatilho que toca telefone**, não código.

Achado que a referência não tem: este build expõe uma superfície VOIP inteira —
`WAWebVoipStartCall` origina chamada, `WAWebVoipCancelOutgoingCall` cancela,
`WAWebVoipCreateCallLink` cria link próprio (e trava). O `whatsapp-web.js` não
tem equivalente para nenhum dos três.

| upstream | estado | nota |
|---|---|---|
| `reject` | `PARTIAL` | H130: este build **não exporta ação de rejeitar** — `WAWebRejectCallAction`, `WAWebEndCallAction`, `WAWebCallActions` e `WAWebOfferCallAction` estão todos ausentes (medido). A stanza montada à mão é a única porta, e ela não é improviso: é o que sobrou. Fica `PARTIAL` porque não há chamada entrando para rejeitar — `INCOMING_CALL` está `BLOCKED` |

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

| upstream | estado | nota |
|---|---|---|
| `getSubscribers` | `BLOCKED` | H140: reclassificado (decisão 60) — `WAWebMexFetchNewsletterSubscribersJob` confirmado ausente na re-auditoria da H140 |
| `setSubject` | `PROVEN` | H113: renomear pega e é relido do servidor |
| `setDescription` | `BLOCKED` | H140: reclassificado (decisão 60). A página ACEITA e o servidor nunca armazena — medido em canal (H113) e, independentemente, em grupo (H126). Duas superfícies, mesma falha: é comportamento da página, não trabalho pendente |
| `setProfilePicture` | `BLOCKED` | H134: o módulo que a referência usa **não existe neste build** (medido) — `WAWebSetPicture` e `WAWebProfilePicThumbBridge` ausentes |
| `setReactionSetting` | `BLOCKED` | H133: `channel.SetReactionPolicy` escrito e provado em unidade; ao vivo é **INVERIFICÁVEL**, não falho. A metadata de um canal recém-criado NÃO carrega o mixin de reação — ele existe num canal estabelecido e não num novo —, então a pós-condição não tem o que ler. A escrita pode ter chegado; ninguém pode dizer, e chamar isso de falha seria afirmar conhecimento que não existe |
| `mute` | `BLOCKED` | H134: **corrigi uma medição minha**. Testei `WAWebMuteChatAction` (ausente) e quase concluí bloqueio; a referência usa `WAWebNewsletterUpdateUserSettingJob.updateNewsletterUserSetting`, que **EXISTE**. O bloqueio real é de assinatura: silenciar canal exige segui-lo, e seguir é impossível (H123) |
| `unmute` | `BLOCKED` | H134: idem `mute` — módulo presente, bloqueio de assinatura (H123) |
| `sendMessage` | `PARTIAL` | H118: delegação literal para `Client.sendMessage` (Channel.js:240) |
| `sendSeen` | `PARTIAL` | H118: delegação literal para `Client.sendSeen` (Channel.js:248) |
| `sendChannelAdminInvite` | `PROVEN` | H136: **provado com SESSÃO DUPLA** (H135). conta-A cria canal e convida conta-B; a página responde `messageSendResult: OK` e o convite é ACEITÁVEL — provado pelo passo seguinte funcionar. O convidado é resolvido por `lookup.NumberID` antes: procurar pelo jid de telefone não acha o chat, porque este build arquiva sob LID (lição da H34) |
| `acceptChannelAdminInvite` | `PROVEN` | H136: provado com sessão dupla. conta-B aceita e o canal vai de **0 para 1 assinante** — pós-condição independente da chamada não ter lançado |
| `revokeChannelAdminInvite` | `PROVEN` | H136: provado pela ORDEM CERTA. Testado depois do aceite, o servidor responde `Not Allowed`, que é a resposta correta para convite já consumido e medição errada da capacidade. Revogando ANTES: o aceite seguinte falha com `Not Found` e os assinantes ficam em **0** — a revogação é provada pelo aceite FALHAR, não pela chamada não lançar |
| `demoteChannelAdmin` | `BLOCKED` | H137: **a forma foi resolvida** — `demoteNewsletterAdminAction(modeloDoCanal, modeloDoContato)`, aridade 2, e a chamada responde ok. Três tentativas antes falharam por nome ou forma inferidos; só ENUMERAR o módulo resolveu. Fica `BLOCKED` e não `PROVEN` porque a pós-condição NÃO EXISTE: verificar exigiria ler a lista de admins, e `WAWebMexFetchNewsletterSubscribersJob` não existe neste build (H113). A chamada funcionar não é a coisa acontecer |
| `transferChannelOwnership` | `PROVEN` | H139: **provado com autorização explícita e a limpeza mudando de mãos**. conta-A cria o canal, convida conta-B como admin, B aceita, e A transfere: `changeNewsletterOwnerAction(modeloDoCanal, modeloDoContato)` — as formas que a H137 estabeleceu. Pós-condição de verdade: a membership de B lida do lado DELE volta `owner`. E o canal é apagado **por B**, porque quem deixou de ser dono não pode apagar — a recusa da H136 não era do verbo, era do teardown |
| `fetchMessages` | `BLOCKED` | H134: exige um canal COM mensagens. Um canal recém-criado não tem nenhuma, e um alheio exigiria assinatura, medida impossível (H123) |
| `deleteChannel` | `PROVEN` | H113: pós-condição é o canal deixar de ser legível |

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
| `delete` | chats.Delete | `PARTIAL` | sim | NÃO (por desenho) | sim | H66: provar destruiria o fixture de todos os outros testes. É recusa DELIBERADA, não pendência. *(era `idem`, expandido na H130)* |
| `archive` | chats (arquivar) | `PROVEN` | sim | sim | sim | delegação literal para `Client.archiveChat` (Chat.js:137); H55 |
| `unarchive` | chats | `PROVEN` | sim | sim | sim | delegação literal para `Client.unarchiveChat` (Chat.js:144) |
| `pin` | chats | `PROVEN` | sim | sim | sim | delegação literal para `Client.pinChat` (Chat.js:152). NÃO confundir com a H81, que é fixar MENSAGEM e continua falhando |
| `unpin` | chats | `PROVEN` | sim | sim | sim | delegação literal para `Client.unpinChat` (Chat.js:160) |
| `mute` | capabilities/mute | `PROVEN` | sim | sim | sim | delegação para `Client.muteChat` (Chat.js:168); H62 |
| `unmute` | capabilities/mute | `PROVEN` | sim | sim | sim | delegação para `Client.unmuteChat` (Chat.js:182) |
| `markUnread` | chats.MarkUnread | `BLOCKED` | sim | falha (H78) | sim | H140: idem `Client.markChatUnread` — duas primitivas medidas, nenhuma marca, ouvinte em UI que não carregamos |
| `fetchMessages` | capabilities/fetchmessages | `PROVEN` | sim | sim | sim | — |
| `sendStateTyping` | capabilities/chatstate | `PROVEN` | sim | sim | sim | — |
| `sendStateRecording` | presence.StateRecording | `PARTIAL` | sim | bloqueada (H50) | sim | **linha corrigida**: eu a marquei MISSING de memória e ela JÁ EXISTIA, mapeada para `markRecording`. A prova ao vivo esbarra no mesmo bloqueio da observação de presença |
| `clearState` | capabilities/chatstate | `PROVEN` | sim | sim | sim | — |
| `getContact` | resolução interna | `PARTIAL` | sim | sim | sim | delegação literal para `Client.getContactById` (Chat.js:284); herda a linha dele |
| `getLabels` | contacts.LabelsOfChat | `PROVEN` | sim | sim | sim | delegação literal para `Client.getChatLabels` (Chat.js:292); H72 |
| `changeLabels` | contacts.AddLabel / RemoveLabel | `PROVEN` | sim | sim | sim | delegação literal para `Client.addOrRemoveLabels` (Chat.js:301); H72 |
| `getPinnedMessages` | pin.PinnedIn | `PARTIAL` | sim | vazia | sim | o leitor funciona e a conta não tem NADA fixado; provar não-vazio exigiria fixar, que está bloqueado (H81) |
| `syncHistory` | capabilities/fetchmessages | `PARTIAL` | sim | sim | sim | delegação literal para `Client.syncHistory` (Chat.js:317); herda a linha dele — buscamos histórico de uma conversa, sincronizar não |
| `addOrEditCustomerNote` | — | `BLOCKED` | — | medido | — | H134: **as ações EXISTEM** — `noteAddAction` e `retrieveOnlyNoteForChatJid`, exatamente as que a referência chama. O que falta é `WAWebBizGatingUtils`, o módulo do PORTÃO (`smbNotesV1Enabled`), ausente neste build: dá para chamar a ação e não dá para saber se o recurso deveria estar ligado |
| `getCustomerNote` | — | `BLOCKED` | — | medido | — | H134: **as ações EXISTEM** — `noteAddAction` e `retrieveOnlyNoteForChatJid`, exatamente as que a referência chama. O que falta é `WAWebBizGatingUtils`, o módulo do PORTÃO (`smbNotesV1Enabled`), ausente neste build: dá para chamar a ação e não dá para saber se o recurso deveria estar ligado |

## ClientInfo

**Família inteira `MISSING`** — informação de bateria/plataforma — não atacada.

| upstream | estado | nota |
|---|---|---|
| `getBatteryStatus` | `BLOCKED` | H126: `WAWebBatteryStore` não existe neste build (medido na H119) |

## Contact

| upstream | wa-headless | estado | unitário | SPA real | ctrl. neg. | nota |
|---|---|---|---|---|---|---|
| `getProfilePicUrl` | capabilities/avatar | `PROVEN` | sim | sim | sim | H118: delegação literal para `Client.getProfilePicUrl` (Contact.js:119) |
| `getFormattedNumber` | phone.Lookup (.Formatted) | `PROVEN` | sim | sim | sim | delegação literal para o `Client` (Contact.js:128 e :136); H111: a página NÃO recusa lixo — `findCC("notaphone")` devolve `"not"`, medido. Guardamos dos dois lados: a entrada tem de ser dígitos e a RESPOSTA também, e as duas guardas foram provadas independentes por controle negativo |
| `getCountryCode` | phone.Lookup (.CountryCode) | `PROVEN` | sim | sim | sim | delegação literal para o `Client` (Contact.js:128 e :136); H111: a página NÃO recusa lixo — `findCC("notaphone")` devolve `"not"`, medido. Guardamos dos dois lados: a entrada tem de ser dígitos e a RESPOSTA também, e as duas guardas foram provadas independentes por controle negativo |
| `getChat` | chats.OfContact | `PROVEN` | sim | sim | sim | H132: é `ByJID` MAIS uma guarda, e a guarda é a diferença — `Contact.getChat` devolve null quando o contato É esta conta (Contact.js:144). Delegar sem ela entregaria a conversa que a página guarda para o self, que existe e não significa nada. Ao vivo: a guarda dispara para as DUAS identidades da conta |
| `block` | capabilities/block | `PROVEN` | sim | sim | sim | H59: blocklist 0->1->0 |
| `unblock` | capabilities/block | `PROVEN` | sim | sim | sim | — |
| `getAbout` | contacts.AboutOf | `PARTIAL` | sim | parcial | sim | H70: o par tem recado VAZIO e em cache; a busca no servidor não foi exercitada. **H149: a hipótese de identidade foi TESTADA E DESCARTADA** — depois da H148, era natural suspeitar que a leitura tivesse sido feita contra o jid errado. Não foi: os dois jids, resolvido e de telefone, leem `len=0 fetched=false` na mesma sessão. O `fetched=false` nos dois é o dado que importa — o caminho de servidor não é exercitado por nenhuma das identidades, então a nota da H70 está intacta e a causa não é a da H148 |
| `getCommonGroups` | contacts.CommonGroups | `PROVEN` | sim | sim | sim | H118: delegação literal para `Client.getCommonGroups` (Contact.js:223) |
| `getBroadcast` | status.ByContact | `PARTIAL` | sim | sim | sim | idem `getBroadcastById` (H100) |

## GroupChat

| upstream | wa-headless | estado | unitário | SPA real | ctrl. neg. | nota |
|---|---|---|---|---|---|---|
| `owner` | group.Metadata | `PROVEN` | sim | sim | sim | provado no grupo de laboratório; o participante que é super admin também é admin — as duas flags são distintas, não a mesma lida duas vezes (H105) |
| `createdAt` | group.Metadata | `PROVEN` | sim | sim | sim | `md.creation`, em segundos; zero fica zero em vez de virar a época (H105) |
| `description` | group.Metadata | `PARTIAL` | sim | sim | sim | leitor entregue e **nunca observado não-vazio**: no grupo de laboratório `desc` E `displayedDesc` leem `undefined`, compatível com "não tem descrição" E com "campo errado". Por isso `DescriptionSource` viaja junto — `none` diz que nada veio, não que nada existe. Quinta ocorrência da classe (H105) | **H145: a metade aberta é comprovadamente NÃO ACIONÁVEL por este módulo.** Tentado produzir a descrição com `group.SetDescription` para observar o leitor não-vazio; a escrita reproduziu pela TERCEIRA vez o bloqueio de servidor (0 bytes de 35 pedidos), depois de canal (H113) e grupo (H126). Não há produtor dentro deste módulo, logo a linha não é trabalho parado |
| `participants` | group.Metadata | `PROVEN` | sim | sim | sim | lista com `admin`, `superAdmin` e `joinedAt`; identidades chegam como **LID**. Exatamente um super admin, travado por teste (H105) |
| `addParticipants` | group.AddParticipant | `PARTIAL` | sim | entre sessões | sim | H58: este build não confirma na MESMA sessão |
| `removeParticipants` | group.RemoveParticipant | `PARTIAL` | sim | entre sessões | sim | H58: este build não confirma na MESMA sessão — a mudança chega ao servidor e a sessão que agiu não a vê. *(era `idem`, expandido na H130)* |
| `promoteParticipants` | group.Promote | `PARTIAL` | sim | entre sessões | sim | H65: mesmo limite do `addParticipants` (H58) — a mudança chega ao servidor e a sessão que AGIU não a confirma. *(era só a referência `H65`, expandido na H134)* |
| `demoteParticipants` | group.Demote | `PARTIAL` | sim | entre sessões | sim | H65: idem `promoteParticipants` — confirmação só entre sessões. *(expandido na H134)* |
| `setSubject` | group.SetSubject | `PROVEN` | sim | sim | sim | H64: o assunto vive em chat.formattedTitle |
| `setDescription` | group.SetDescription | `BLOCKED` | sim | medido | sim | H126: implementado e medido. A chamada é ACEITA e o servidor nunca armazena — 0 bytes depois de 20s de espera. É o MESMO comportamento que a descrição de CANAL mostrou na H113: duas superfícies independentes, escrita de descrição que não persiste neste build. O código fica no lugar porque a diferença é da PÁGINA |
| `setAddMembersAdminsOnly` | group.SetPolicy(PolicyJoinNeedsApproval) | `PROVEN` | sim | sim | sim | H79 mediu o nome pelo oráculo do app; **H85** corrigiu a classificação: é visível NA MESMA sessão em ~1s, e a pós-condição é real |
| `setMessagesAdminsOnly` | group.SetPolicy(PolicyMessagesAdminsOnly) | `PROVEN` | sim | sim | sim | H79 mediu o nome pelo oráculo do app; **H85** corrigiu a classificação: é visível NA MESMA sessão em ~1s, e a pós-condição é real |
| `setInfoAdminsOnly` | group.SetPolicy(PolicyInfoAdminsOnly) | `PROVEN` | sim | sim | sim | H79 mediu o nome pelo oráculo do app; **H85** corrigiu a classificação: é visível NA MESMA sessão em ~1s, e a pós-condição é real |
| `deletePicture` | — | `BLOCKED` | — | medido | — | H140: idem `setPicture` — módulos de foto confirmados ausentes |
| `setPicture` | — | `BLOCKED` | — | medido | — | H140: reclassificado (decisão 60) — `WAWebSetPicture` e `WAWebProfilePicThumbBridge` confirmados ausentes na re-auditoria |
| `getInviteCode` | group.InviteCode | `PROVEN` | sim | sim | sim | H57: a chamada popula o MODELO; o retorno é undefined |
| `revokeInvite` | group.RevokeInvite | `PROVEN` | sim | sim | sim | H57 |
| `getGroupMembershipRequests` | groupreq.List | `PROVEN` | sim | sim | sim | refresca a metadata antes de ler; campos do registro medidos ao vivo: `id t addedBy requestMethod parentGroupId` (H89) |
| `approveGroupMembershipRequests` | groupreq.Approve | `PROVEN` | sim | sim | sim | uma chamada RPC por solicitante, resultado por solicitante; provado ao vivo do pedido ao desaparecimento (H89) |
| `rejectGroupMembershipRequests` | groupreq.Reject | `PARTIAL` | sim | não | sim | mesma RPC do approve, diferindo só na chave enviada — travado por teste unitário que casa `rejectArgs:` com os dois pontos. NÃO exercitado ao vivo: rejeitar conta-B a expulsaria do grupo de laboratório (H89) |
| `leave` | group.Leave | `PARTIAL` | sim | NÃO (por desenho) | sim | H65: conta que sai de grupo que criou não volta sem convite |

## GroupNotification

**Família inteira `MISSING`** — notificações de grupo como objeto — não atacada.

| upstream | estado | nota |
|---|---|---|
| `getChat` | `PARTIAL` | H118: delegação literal para `Client.getChatById` (GroupNotification.js:78) |
| `getContact` | `PARTIAL` | H118: delegação literal para `Client.getContactById` (GroupNotification.js:86) — usa `this.author`, não o chat |
| `getRecipients` | `PROVEN` | H132: mapeia os jids por `contacts.Recipients`, que resolve por QUALQUER das duas identidades e devolve os desconhecidos SEPARADAMENTE. O `Promise.all` da referência transforma uma falta em entrada indefinida; encurtar a lista em silêncio seria pior — quem contasse destinatários teria número menor que o evento nomeou. Ao vivo: pediu 4, achou 3, faltou 1, e a soma bate |
| `reply` | `PARTIAL` | H118: delegação literal para `Client.sendMessage` (GroupNotification.js:109); herda a linha do par, ENQUETE inclusa no que não sai |

## Label

**Família inteira `BLOCKED`** (H126) — `Label.getChats` tem a mesma medição do
`Client.getChatsByLabelId`: os 3 rótulos desta conta têm ZERO itens, então um
leitor nunca seria visto devolvendo nada (armadilha H93). Não é falta de
código — é falta de dado.

| upstream | estado | nota |
|---|---|---|
| `getChats` | `BLOCKED` | H126: mesma medição do `getChatsByLabelId` (H114) — os 3 rótulos desta conta têm ZERO itens |

## Message

| upstream | wa-headless | estado | unitário | SPA real | ctrl. neg. | nota |
|---|---|---|---|---|---|---|
| `reload` | message.CurrentOf | `PROVEN` | sim | sim | sim | H108: 32 mensagens re-lidas ao vivo deram 11 estados distintos; `hasAck` separa "ack 0" de "sem ack", medido em 5 de 395 |
| `rawData` | message.ShapeOf | `INTENTIONAL_DIFFERENCE` | sim | sim | sim | H107: devolvemos os NOMES dos campos, nunca os valores — a medição achou 598 nomes no modelo cru, entre eles `body` e `caption`; devolver rawData como é derrubaria a invariante 12 em vez de entregar funcionalidade |
| `getChat` | message.OriginOf (.ChatJID) | `PROVEN` | sim | sim | sim | H106 |
| `getContact` | message.OriginOf (.SenderJID) | `PROVEN` | sim | sim | sim | H106: 2 de 2 mensagens de grupo com remetente ≠ chat; grupo é PERGUNTADO à página (getIsGroup), não inferido do sufixo |
| `getMentions` | message.MentionsOf | `PROVEN` | sim | sim | sim | H142: a H106 estava certa em recusar e errada sobre a causa — o zero era da DADO, não da página. Ninguém nesta conta jamais mencionara ninguém. Produzida uma menção no grupo de laboratório, `mentionedJidList` apareceu: lista de Wid `{_serialized, server, user}`, idêntica pelo getter e pelo `__x_`. Lida de volta pelo capability, a identidade bate com a resolvida por `lookup.NumberID` |
| `getGroupMentions` | message.MentionsOf | `PROVEN` | sim | sim | sim | H142: campo SEPARADO, com forma própria — `groupMentions` é lista de `{groupJid, groupSubject}`, não jids com sufixo de grupo. O assunto vem congelado na mensagem, não do grupo de hoje. Provado na mesma mensagem que provou o `getMentions`: 1 pessoa e 1 grupo, ambos lidos de volta |
| `getQuotedMessage` | message.QuotedOf | `PROVEN` | sim | sim | sim | H131: PRODUZI a citação para poder prová-la — 336 mensagens carregadas e ZERO com id citado, então esperar era a armadilha H93. Mensagem comum diz `quotes=false`, resposta diz `quotes=true` apontando para a mensagem certa, e o id devolvido é usável por `OriginOf`. Lê `quotedStanzaID`, o mesmo campo que o `send` usa para provar a citação — os campos `__x_*QuotedMsg*` existem em TODA mensagem e guardam sentinela preguiçosa, não dado |
| `reply` | send.Reply | `PROVEN` | sim | sim | sim | H54 |
| `react` | capabilities/react | `PARTIAL` | sim | parcial | sim | H53: as duas metades NÃO são iguais — `Add` verifica a pós-condição e `Remove` não, e o `Result` diz qual foi qual em vez de fingir simetria. *(era só a referência `H53`, expandido na H134)* |
| `acceptGroupV4Invite` | — | `BLOCKED` | — | medido | — | H125: bloqueio DUPLO, medido. `WAWebGroupInviteV4Job` existe mas **nenhuma** das duas funções que a referência chama existe nele — décimo desencontro com a lista do wwebjs. E a conta tem zero convites v4 |
| `forward` | capabilities/forward | `PROVEN` | sim | sim | sim | H63: cópia identificada por conjunto de ids, não por instante |
| `downloadMedia` | capabilities/media | `PROVEN` | sim | sim | sim | H67: pós-condição CRIPTOGRÁFICA — SHA-256 do texto claro |
| `delete` | capabilities/revoke | `PROVEN` | sim | sim | sim | direito consultado na página |
| `star` | capabilities/star | `PROVEN` | sim | sim | sim | H61: o await não é a conclusão — 696ms |
| `unstar` | capabilities/star | `PROVEN` | sim | sim | sim | — |
| `pin` | pin.Message | `BLOCKED` | sim | falha (H81) | sim | H140: reclassificado (decisão 60) — a H81 mediu chamada aceita e nada fixado, com vocabulário, duração e forma do modelo medidos. É comportamento da página |
| `unpin` | pin.Unpin | `BLOCKED` | sim | falha (H81) | sim | H140: idem `pin` — chamada aceita, nada desfixado (H81) |
| `getInfo` | capabilities/ack | `PARTIAL` | sim | sim | sim | H71: MsgInfoCollection VAZIA (0 de 368); temos ack, não "quem leu" |
| `getOrder` | — | `BLOCKED` | — | medido | — | H125: `WAWebBizOrderBridge.queryOrder` EXISTE. O bloqueio é de DADO: a conta tem **zero** mensagens de pedido. Exercitar exigiria atividade comercial real, que não é produzível por agente |
| `getPayment` | — | `BLOCKED` | — | medido | — | H125: idem `getOrder` — módulo presente, **zero** pagamentos na conta |
| `getReactions` | — | `BLOCKED` | — | medido | — | H124: a H83 tinha concluído que não há fonte para QUAL reação. Remedido com instrumento melhor: `WAWebCollections.Reactions` EXISTE com `on`/`getModelsArray`, e fica em **0 mesmo depois de uma reação que a capacidade verificou**. Não é falta de coleção — a coleção não enche. A `RecentReactions` (1 item) é a lista do seletor de emoji, não reações em mensagens |
| `edit` | capabilities/edit | `PROVEN` | sim | sim | sim | H60: janela de 1200s |
| `editScheduledEvent` | — | `BLOCKED` | — | medido | — | H125: `WAWebScheduledEventEditAction` e `WAWebScheduledEventCreateAction` **não existem** neste build. Não há o que chamar |
| `getPollVotes` | poll.Votes | `PROVEN` | sim | sim | sim | idem |
| `vote` | poll.Vote | `PARTIAL` | sim | não | sim | implementado e travado por teste: nomes viram ids locais NA PÁGINA, e um nome que não casa é recusa e não omissão — o envio da página recebe um SET, e um nome perdido produziria um voto por menos coisas do que se pediu, reportado como sucesso. Não provado ao vivo porque a enquete não chega ao par (ver `sendMessage`, H98) |

## MessageMedia

**Família inteira `MISSING`** — construtores de mídia — nós passamos bytes direto.

| upstream | estado | nota |
|---|---|---|
| `fromFilePath` | `INTENTIONAL_DIFFERENCE` | H140: reclassificado (decisão 60) — construtor de mídia da referência. Nós passamos BYTES direto para `send.SendMedia`, que é a mesma capacidade sem o intermediário. Diferença deliberada |
| `fromUrl` | `INTENTIONAL_DIFFERENCE` | H140: idem `fromFilePath` — passamos bytes direto, sem construtor |

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

| upstream | estado | nota |
|---|---|---|
| `getData` | `PARTIAL` | H130: `capabilities/catalog` lê a vitrine de um vendedor e foi provado ao vivo SEM criar nada (H103). Fica `PARTIAL` porque provar o caminho do produto PRÓPRIO exigiria acrescentar um item real ao perfil comercial desta conta, visível a quem abrisse o perfil — recusa deliberada, não pendência |

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
| `AUTHENTICATED` | — | `BLOCKED` | H140: reclassificado (decisão 60) — sem observável neste build: o boot chega a APP_READY verificado ou falha, sem degrau intermediário (H88) |
| `AUTHENTICATION_FAILURE` | events.SessionBootFailed | `PARTIAL` | o evento carrega o ESTÁGIO do boot; um boot que morre em `not_ready` contra uma tela de QR é o caso do upstream, mas o MOTIVO (a classe da página) não viaja no evento — `BootFailure` o guarda na mensagem de erro, e mensagem de erro não entra no barramento (H88) |
| `READY` | events.SessionReady | `PROVEN` | no barramento desde a H88, e distingue um ready ORDINÁRIO de um que recuperou um perfil suspeito — a quitação da invariante 2 fica contada em vez de inferida |
| `CHAT_REMOVED` | — | `BLOCKED` | H140: reclassificado (decisão 60) — provar exigiria APAGAR uma conversa, destruindo a fixture de todos os outros testes (H66) |
| `CHAT_ARCHIVED` | events.ChatChanged | `PARTIAL` | o nosso evento é grosso: diz que a conversa mudou, não QUAL campo (H87). **H150: o refinamento foi TENTADO e a página não o suporta.** A coleção não é Backbone — `model.changed` é nulo em 40 de 40 eventos —, mas o build tem contabilidade própria em `__fired`, que parecia a saída. Não é: medindo por ATO, arquivar e desarquivar disparam AMBOS `showUnreadInTitle` e nada mais, e marcar não-lida dispara `pendingAction`, enquanto `markedUnread` dispara durante um ENVIO. Os nomes são campos derivados de UI, não o campo semântico — não dá para classificar `archived` a partir deles |
| `MESSAGE_RECEIVED` | events.MessageAdded | `PROVEN` | disparado ao vivo por um envio (H87) |
| `MESSAGE_CIPHERTEXT` | — | `BLOCKED` | H140: reclassificado (decisão 60) — vive ABAIXO do modelo: é a mensagem antes de decifrar, e este barramento escuta COLEÇÕES, não o fio (H88) |
| `MESSAGE_CIPHERTEXT_FAILED` | — | `BLOCKED` | H140: idem `MESSAGE_CIPHERTEXT` — abaixo do modelo (H88) |
| `MESSAGE_CREATE` | events.MessageAdded | `PROVEN` | H152: a nota estava errada sobre COMO o upstream distingue. Ele emite `MESSAGE_CREATE` para toda mensagem e então `if (msg.id.fromMe) return;` antes do `MESSAGE_RECEIVED` (`client.js:648-664`) — o único discriminador é `fromMe`, que é exatamente o campo que o nosso `message.added` carrega. Nosso evento **é** o `MESSAGE_CREATE`, um para um, e o `MESSAGE_RECEIVED` é ele filtrado. O que faltava era prova de que o campo VARIA: nenhum teste unitário jamais vira `fromMe:false` (o helper `row` o fixava em `true`) e nenhuma medição ao vivo tinha as duas direções. Provado com sessão dupla, `own=27 incoming=14` sobre linha de base zerada |
| `MESSAGE_REVOKED_EVERYONE` | events.MessageRevoked | `PROVEN` | disparado ao vivo; reconhecido pelo predicado de TRÊS sinais que a capacidade de apagar mede (H87) |
| `MESSAGE_REVOKED_ME` | events.MessageRemoved + revoke.ForMe | `PROVEN` | H143: o diagnóstico da H88 estava certo — era falta de MÉTODO. Fechado escrevendo os dois: `revoke.ForMe` (`Cmd.sendDeleteMsgs`, nome ENUMERADO no build, aridade 6) e o ouvinte `MsgCollection.on('remove')` filtrado por `isNewMsg`. Nomeado `message.removed` e não `revoked`: revogar é fato da CONVERSA, apagar local é fato deste APARELHO. Provado com linha de base (0 antes, 1 depois, nomeando a mensagem apagada) |
| `MESSAGE_ACK` | events.MessageAck | `PROVEN` | disparado ao vivo (H87) |
| `MESSAGE_EDIT` | events.MessageEdited | `PROVEN` | disparado ao vivo por uma edição (H87) |
| `UNREAD_COUNT` | events.ChatChanged | `PARTIAL` | H130: o `idem` daqui estava PENDURADO — apontava para a nota do `MESSAGE_EDIT`, que é sobre outra coisa. O veredito de verdade: `chat.changed` DISPARA ao vivo (H87, e as sondas de hoje o viram às dezenas), mas é um evento genérico de "campos da conversa se moveram", não o evento dedicado que o upstream entrega COM a contagem. E o contador em si é `CROSS_SESSION` (H78): chega ao servidor e esta sessão não o vê mudar **H150: mesma medição do `CHAT_ARCHIVED`** — o campo semântico não viaja em `__fired`; marcar não-lida dispara `pendingAction`, e `markedUnread` dispara durante ENVIO. O refinamento por campo foi medido e a página não o sustenta |
| `MESSAGE_REACTION` | events.MessageReaction | `PARTIAL` | disparado ao vivo (H87); diz que as reações se moveram e NÃO quais são — o agregado não tem fonte neste build (H83) |
| `MEDIA_UPLOADED` | events.MessageAdded (`Kind`) | `PARTIAL` | **agora MEDIDO** (H120), fechando o que a H88 deixou aberto: um envio de mídia real produziu 12 eventos — `chat.changed:9`, `message.ack:2`, `message.added:1` — com `kind=image` em 3 deles. A mensagem de mídia CHEGA ao barramento e progride pelos acks. O que NÃO existe é um momento distinto de "upload concluído": não dá para separar "subiu" de "mensagem criada", e por isso é PARTIAL e não PROVEN |
| `CONTACT_CHANGED` | events.ContactChanged | `PROVEN` | disparado sob demanda por `addressbook.Save`, sem segunda conta: 8 eventos ao nomear o par. Era o único tipo instalado e nunca provado (H90) |
| `GROUP_JOIN` | events.GroupJoined | `PROVEN` | H135: **provado com SESSÃO DUPLA**. A H86 mediu zero na sessão que AGE; com conta-A observando e conta-B saindo/voltando, o observador recebeu `group.joined:1` (subtipo `invite`). O zero da H86 era do ATOR, não do barramento |
| `GROUP_LEAVE` | events.GroupLeft | `PROVEN` | H135: idem `GROUP_JOIN` — o observador recebeu `group.left:1` (subtipo `leave`) enquanto conta-B saía |
| `GROUP_ADMIN_CHANGED` | events.GroupAdminChanged | `PARTIAL` | H135: **NÃO chega ao observador**, e agora isso é medido e não presumido. Com o barramento em conta-B e conta-A promovendo/rebaixando, zero `group.admin_changed` — enquanto sair e entrar chegaram na mesma montagem. A diferença é do SUBTIPO, não do barramento |
| `GROUP_MEMBERSHIP_REQUEST` | events.ChatChanged | `PARTIAL` | a chegada MOVE o modelo nesta sessão e o barramento a vê — medida isolada: a saída de conta-B sozinha deu 5 `chat.changed`, o pedido sozinho deu **9** mais 1 `message.added` (H89). Não há tipo dedicado, e `chat.changed` é grosso demais para ser um: quem quer o pedido tem de chamar `groupreq.List`. Contraste com a H86, onde a sessão que MUDA participantes vê zero — quem recebe enxerga, quem age não |
| `GROUP_UPDATE` | events.GroupUpdated | `PROVEN` | H119: o portador `gp2` É reclassificado neste barramento — assunto do grupo trocado de propósito, `group.updated:1` com subtipo `subject`, e o assunto restaurado |
| `QR_RECEIVED` | core (pareamento) | `PROVEN` | QR nunca é logado nem versionado |
| `CODE_RECEIVED` | — | `BLOCKED` | H140: reclassificado (decisão 60) — depende do pareamento por código, cujo fluxo exige socket `UNPAIRED` e portanto um humano com o telefone (H122) |
| `LOADING_SCREEN` | — | `BLOCKED` | H140: reclassificado (decisão 60) — sem observável: o laço de settle mede CLASSES de página, não progresso de carga (H88) |
| `DISCONNECTED` | events.SessionStateChanged | `PARTIAL` | emitimos desde a H88 — antes só detectávamos. Continua parcial porque o nosso é uma TRANSIÇÃO de liveness com a classe da página anexada, não o motivo de desligamento que o upstream entrega |
| `STATE_CHANGED` | events.SessionStateChanged | `PARTIAL` | emitimos desde a H88, e só na TRANSIÇÃO: repetir "ainda vivo" a cada tique é heartbeat vestido de evento. Parcial porque o vocabulário é o nosso (`ALIVE`, `PROCESS_GONE`, `APP_ABSENT`, …) e não o estado do socket do upstream — os dois não foram medidos um contra o outro |
| `BATTERY_CHANGED` | — | `BLOCKED` | H119: `WAWebBatteryStore` NÃO existe neste build (medido), e a própria referência marca o evento como depreciado e não enviado em multi-device — que é o que este build é |
| `INCOMING_CALL` | events.CallIncoming | `BLOCKED` | ouvinte instalado por `CallCollection.on('add')` — a referência não achou ouvinte e patcheia um `Map` interno; aqui a porta limpa existe. **NUNCA visto disparar**, e agora com causa isolada: `startWAWebVoipCall` resolve `undefined` e **nada se move em lugar nenhum** — cada contêiner da coleção observado por nome, `pendingOutgoingCall` fica `null`. Classe NOTHING (H82), terceira ocorrência. SEIS hipóteses eliminadas (ambiente, pilha VOIP, ordem, aba de chamadas, leitor, e o próprio veredito do app: `showCallBlockedModalIfNeeded()` devolve **false**) em H93 e H95. Reabre com EVIDÊNCIA nova, não hipótese: qualquer coisa que faça `pendingOutgoingCall` deixar de ser `null` |
| `REMOTE_SESSION_SAVED` | — | `BLOCKED` | H140: reclassificado (decisão 60) — depende de uma família que não existe: não há store remoto de sessão neste módulo (H88) |
| `VOTE_UPDATE` | events.VoteUpdated | `PROVEN` | H121: a referência NÃO tem ouvinte — ela remenda `pollVoteTableMode.bulkUpsert`. Aqui `WAWebCollections.PollVote` expõe `on`/`off` (medido), então a porta limpa foi usada e a página não é tocada (regra da H112). Provado ao vivo VOTANDO numa enquete do próprio store: `poll.vote:1`. Segunda vez que este build tem ouvinte onde a referência patcheia — `INCOMING_CALL` foi a primeira, e lá ainda não disparou |

## Placar

| estado | itens | fração |
|---|---|---|
| `PROVEN` | 113 | 51% |
| `PARTIAL` | 52 | 24% |
| `BLOCKED` | 49 | 22% |
| `INTENTIONAL_DIFFERENCE` | 6 | 3% |
| `MISSING` | 0 | 0% |
| **total** | **220** | |
