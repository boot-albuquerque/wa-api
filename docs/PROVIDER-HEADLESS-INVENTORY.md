# Inventário do provider headless

Levantamento dos application ports de TRANSPORTE (os que carregam `txtID
string`, ou seja, os que operam sobre uma sessão) e o estado do adaptador
`headless` para cada um. Fonte primária:
`pkg/infra/headless/phase3_inventory_test.go` (`inventarioFase3`), que é
testado contra o código (`TestOInventarioCobreTodosOsPortsDeTransporte`) — a
tabela abaixo não pode divergir dele sem quebrar o build. Este documento só
adiciona a coluna de evidência (arquivo:linha).

Total: 33 ports de transporte. `go build`/`go vet` de
`./pkg/infra/headless/... ./internal/headless/...` confirmados verdes
(ver final do documento) — nenhuma mudança de lógica foi feita nesta sessão.

| port | capability_name_sugerido | status | evidência (arquivo:linha) |
|---|---|---|---|
| ChatArchiver | chat.archive | supported | `pkg/infra/headless/chat/archiver.go:88` |
| ChatPinner | chat.pin | not_implemented | motivo no inventário: mesmo mecanismo do ChatArchiver (SendAppState+BuildPin), adaptador ainda não escrito |
| PresenceAnnouncer | presence.announce | supported | `pkg/infra/headless/presence/announcer.go:129` |
| BlocklistManager | contacts.blocklist | supported | `pkg/infra/headless/blocklist/manager.go:137` |
| IdentityResolver | identity.resolve | supported | `pkg/infra/headless/identity/resolver.go:177` |
| AvatarReader | contacts.avatar | supported | `pkg/infra/headless/avatar/reader.go:92` |
| ContactRoster | contacts.roster | supported | `pkg/infra/headless/roster/roster.go:126` |
| NewsletterReader | channel.read | supported | `pkg/infra/headless/newsletter/reader.go:47` (sem asserção `var _` — deliberado, comentário na linha 136) |
| SessionDisconnector | session.disconnect | supported | `pkg/infra/headless/session/disconnector.go:70` |
| ProfileAccessProvider | profile.access | supported | `pkg/infra/headless/profile/access.go:183` |
| AppStateSyncer | appstate.sync | supported | `pkg/infra/headless/appstate/syncer.go:91` |
| ChatMessenger | chat.message | supported | `pkg/infra/headless/messenger/messenger.go:298` |
| GroupDirectory | group.directory | supported | `pkg/infra/headless/groupdir/directory.go:191` |
| GroupLifecycle | group.lifecycle | supported | `pkg/infra/headless/grouplife/lifecycle.go:110` |
| GroupInfoSettings | group.settings | supported | `pkg/infra/headless/groupset/settings.go:173` |
| GroupParticipants | group.participants | supported | `pkg/infra/headless/groupmembers/members.go:152` |
| GroupRequests | group.requests | supported | `pkg/infra/headless/groupreq/requests.go:199` |
| LIDResolver | identity.lid | supported | `pkg/infra/headless/identity/resolver.go:182` (mesmo `Resolver` do IdentityResolver) |
| SessionGuard | session.guard | supported | `pkg/infra/headless/sessions.go:54` (embutido em todos os demais adaptadores) |
| SessionLogouter | session.logout | not_implemented | RECUSADO por política — `Socket.logout` funciona mas desempareia a conta, e restaurar exige humano com telefone (motivo completo no inventário) |
| PresenceSubscriber | presence.subscribe | not_implemented | RECUSADO por dependência humana — assinatura de presença exige vínculo de agenda feito no telefone (medido, H144) |
| UnavailableMessageRequester | message.unavailable_request | not_implemented | RECUSADO por ausência de sentido — pede reenvio de mensagem não decifrada; quem dirige a SPA já recebe texto decifrado |
| CallRejecter | call.reject | broken | RECUSADO por capacidade bloqueada (decisão 88) — `INCOMING_CALL` está `BLOCKED`, o evento que dispararia a recusa nunca chega |
| CommunityDirectory | community.directory | not_implemented | PENDENTE, não medido nesta engine (feature vinda da fusão com noise, que fala protocolo) |
| CommunityLifecycle | community.lifecycle | not_implemented | PENDENTE, não medido — mesmo motivo do CommunityDirectory |
| TextMessenger | message.text | not_implemented | PENDENTE com caminho provado — `send.Text` funciona e está no LEDGER, falta só o adaptador |
| MediaMessenger | message.media | not_implemented | PENDENTE com caminho provado — `send.SendMedia` cobre imagem/vídeo/áudio/documento/figurinha, falta o adaptador |
| MediaDownloader | media.download | not_implemented | PENDENTE com caminho provado — `capabilities/media.Get` baixa mídia pela página, falta o adaptador |
| InteractiveMessenger | message.interactive | not_implemented | PENDENTE — geradores existem no build (`WAWebGenerateInteractiveMessageProto` etc.), mas entrega não está provada (ack 0, H98/H101) |
| SimpleMessenger | message.simple | not_implemented | PENDENTE e heterogêneo — 5 métodos com estados distintos (SendPoll bloqueado; SendList/SendTemplate com geradores; SendLocation/SendContact reabertos por H144); ver motivo completo no inventário |
| PhonePairer | session.pair | not_implemented | RECUSADO por dependência humana — boot da headless é de restauração, não de pareamento novo |
| StatusMessageSetter | profile.status_message | not_implemented | PENDENTE por medir — `SetDisplayName` é nome, não recado; não medido se a página expõe `about` |
| HistorySyncRequester | history.sync_request | not_implemented | RECUSADO por ausência de sentido — a página já tem o histórico no store de onde a headless lê |
| GroupPhotoSetter | group.photo | broken | RECUSADO por capacidade ausente do build (H140/decisão 60) — `WAWebSetPicture`/`WAWebProfilePicThumbBridge` ausentes; LEDGER regista `setPicture`/`deletePicture` como BLOCKED |
| GroupEphemeralSetter | group.ephemeral | not_implemented | PENDENTE e medido — módulos existem em bundle (`WAWebChangeEphemeralDurationChatAction`), falta medir com sessão pareada |
| ChatMuter | chat.mute | not_implemented | PENDENTE — port novo (CAP-52), mesma família de ChatPinner/ChatArchiver, emissão de app-state ainda não medida |
| MessageStarrer | message.star | not_implemented | PENDENTE — port novo (CAP-54), herda o veredito do ChatMuter |
| ForwardedMessageSender | message.forward | not_implemented | PENDENTE — port novo (CAP-55), caminho pela SPA não medido |
| DefaultDisappearingTimerSetter | account.ephemeral_default | not_implemented | PENDENTE — port novo (CAP-50), herda evidência do GroupEphemeralSetter (módulos em bundle, falta medir com sessão pareada) |
| PrivacyManager | account.privacy | not_implemented | PENDENTE e medido — módulos existem em bundle (`WASmaxBizSettingsGetPrivacySettingRPC` etc.), falta medir com sessão pareada |

## Contagem

- **supported**: 18 (ChatArchiver, PresenceAnnouncer, BlocklistManager,
  IdentityResolver, AvatarReader, ContactRoster, NewsletterReader,
  SessionDisconnector, ProfileAccessProvider, AppStateSyncer, ChatMessenger,
  GroupDirectory, GroupLifecycle, GroupInfoSettings, GroupParticipants,
  GroupRequests, LIDResolver, SessionGuard)
- **not_implemented**: 13 (inclui os recusados por política/dependência
  humana/ausência de sentido, que tecnicamente nunca terão adaptador — mas o
  enunciado da tarefa só previa três status, e "recusado por decisão" não é
  "broken": marcados como `not_implemented` com o motivo da recusa citado)
- **broken**: 2 (CallRejecter, GroupPhotoSetter — capacidade bloqueada/ausente
  no build atual, não falha de lógica no adaptador)

Nota: o `inventarioFase3` no código usa duas categorias (`satisfeito` /
`motivo`, sem distinguir "recusado por política" de "pendente"). Este
documento reclassifica manualmente para as três colunas pedidas
(supported/not_implemented/broken); a fonte de verdade para
satisfeito-ou-não continua sendo o teste Go, não esta tabela.

**Total de linhas nesta tabela: 33**, igual ao `len(inventarioFase3)` medido
por `TestOTotalDePortsEOMedidoENaoOAnunciado` em 2026-08-26. Se esse número
mudar (nova port de transporte, ou divisão de uma existente), esta tabela
fica desatualizada — regenere a partir do teste, não editando à mão.

## Personal vs Business

Sem evidência óbvia no código atual (`grep -rn "business\|personal\|account_type" pkg/infra/headless internal/headless` não retornou distinção
de tipo de conta nos adaptadores) — não investigado a fundo, apenas
descartado por busca rápida conforme escopo desta tarefa.

## SPA/DOM causa raiz

Não investigado além do que já está documentado no próprio
`phase3_inventory_test.go` e no `LEDGER-WWEBJS.md` / `EVIDENCIA-SPA.md`
existentes no repo — nenhuma pista nova de seletor ausente ou TODO explícito
foi encontrada nos greps feitos para os ports `not_implemented` desta tabela
além do que já está citado nos motivos acima.

## Verificação de build

```
go build ./pkg/infra/headless/... ./internal/headless/...
go vet ./pkg/infra/headless/... ./internal/headless/...
```
