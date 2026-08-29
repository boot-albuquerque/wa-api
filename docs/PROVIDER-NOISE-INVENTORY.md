# Inventário de capacidades — noise

Levantamento real (código, não teste contra WhatsApp) do que o motor
`noise` implementa hoje, porta a porta, para alimentar a matriz da worktree
`feature/capability-registry`. Feito em 2026-08-26, a partir do checkpoint
`checkpoint/engine-foundation`.

**Método**: para cada `interface` em `pkg/application/contracts/*.go`,
verificado se algum adapter em `pkg/infra/noise/adapters/*` implementa
TODOS os métodos com corpo real (não stub, não `errors.New("not implemented")`
hardcoded). Onde havia asserção de compilação (`var _ appport.X = (*Y)(nil)`),
usada como evidência direta — é o compilador a confirmar, não leitura visual.
Rodado `go build ./pkg/infra/noise/... ./pkg/application/...` (limpo),
`go vet ./pkg/infra/noise/...` (limpo) e `go test ./pkg/infra/noise/...`
(uma falha pré-existente, ver HOUSEKEEP H187 abaixo).

**Fora do escopo deste documento**: portas que não são capacidade do MOTOR de
protocolo — persistência (`UserRepository`, `StoragePort`, `S3ConfigStore`,
`HmacKeyStore`, `SessionConfigPort`/`ProxyConfigStore`, `ChatHistoryReader`,
`StoredMessageReader`, `ChatActivityReader`), mensageria (`MessagingPort`,
RabbitMQ), e utilitários genéricos partilhados entre motores
(`MediaFetcher`/`LinkPreviewFetcher` em `pkg/infra/media/opengraph`,
`StickerProcessor` em `pkg/infra/media/sticker` — não são específicos de
noise, servem qualquer motor). Essas ficam marcadas `N/A` na tabela.

## Tabela: porta → capacidade → status → evidência

| Port (contracts) | capability_name sugerido | Status | Arquivo:linha (evidência) |
|---|---|---|---|
| `JIDResolver` | `resolve_jid` | supported | `pkg/infra/noise/mapping/jid/resolver.go` |
| `PresenceAnnouncer` | `presence_announce` | supported | `pkg/infra/noise/adapters/presence/controller.go:27` (`SendPresence`), `:47` (`SendChatPresence`) |
| `PresenceSubscriber` | `presence_subscribe` | supported | `pkg/infra/noise/adapters/presence/controller.go:60` (`SubscribePresence`) |
| `PresenceController` | `presence_full` | supported | `pkg/infra/noise/adapters/presence/controller.go:73` — `var _ appport.PresenceController = (*PresenceControllerAdapter)(nil)` |
| `ChatMessenger` | `chat_message_ops` (mark-read, react, revoke, edit) | supported | `pkg/infra/noise/adapters/chat/messenger.go:1037` — asserção de compilação |
| `TextMessenger` | `send_text` | supported | `pkg/infra/noise/adapters/chat/messenger.go:242` + `:1038` |
| `MediaMessenger` | `send_media` (image/document/audio/video/sticker) | supported | `pkg/infra/noise/adapters/chat/messenger.go:315,366,417,472,529` + `:1039` |
| `SimpleMessenger` | `send_location` | supported | `pkg/infra/noise/adapters/chat/messenger.go:581` + `:1040` |
| `InteractiveMessenger` | `send_buttons_list_carousel_template_poll` | supported | `chat/messenger_buttons.go:170`, `messenger_list.go:92`, `messenger_carousel.go:46`, `chat/messenger.go:680` (poll), `:735` (poll vote), `:926` (template) — sem asserção de compilação explícita, mas os cinco métodos da interface existem com corpo real |
| `ForwardedMessageSender` | `forward_message` | supported | `pkg/infra/noise/adapters/chat/forward.go:175` — asserção de compilação |
| `StoredMessageReader` | — | N/A | DB (`pkg/infra/db/stored_message_repository.go`), não noise |
| `MediaDownloader` | `download_media` | supported | `pkg/infra/noise/adapters/chat/downloader.go:128` — asserção de compilação |
| `GroupDirectory` | `group_read` (info/link/names/list) | supported | `pkg/infra/noise/adapters/group/adapter.go:36` — asserção de compilação |
| `GroupLifecycle` | `group_create_join_leave` | supported | `pkg/infra/noise/adapters/group/adapter.go:37` |
| `GroupInfoSettings` | `group_settings` (nome/tópico/announce/locked) | supported | `pkg/infra/noise/adapters/group/write.go:60,73,100,113` — parte de `GroupSettings`, asserção em `adapter.go:38` |
| `GroupParticipants` | `group_participants` | supported | `pkg/infra/noise/adapters/group/participants.go:14` |
| `GroupPhotoSetter` | `group_photo` | supported | `pkg/infra/noise/adapters/group/write.go:86` |
| `GroupEphemeralSetter` | `group_disappearing_timer` | supported | `pkg/infra/noise/adapters/group/write.go:126` |
| `GroupSettings` (composição) | `group_settings_full` | supported | `pkg/infra/noise/adapters/group/adapter.go:38` |
| `CommunityDirectory` | `community_read` (`GetSubGroups`, `GetLinkedGroupsParticipants`) | **broken** | código existe em `pkg/infra/noise/adapters/group/community.go:14,30`, asserção de compilação em `adapter.go:40` — MAS `GetSubGroups` e `GetLinkedGroupsParticipants` estão na lista dos 7 métodos sem wrapper de erro (HOUSEKEEP H187): recusa do servidor chega como `500` cru |
| `CommunityLifecycle` | `community_write` (`LinkGroup`, `UnlinkGroup`) | **broken** | código existe em `community.go:46,65`, asserção em `adapter.go:41` — mesmos dois métodos na lista H187 |
| `GroupRequests` | `group_join_requests` | supported | `pkg/infra/noise/adapters/group/participants.go:44,57` + `write.go:139` |
| `ChatArchiver` | `chat_archive` | supported | `pkg/infra/noise/adapters/misc/adapter.go:46` |
| `ChatMuter` | `chat_mute` | supported | `misc/adapter.go:95` |
| `ChatPinner` | `chat_pin` | supported | `misc/adapter.go:82` |
| `CallRejecter` | `call_reject` | supported | `misc/adapter.go:108` |
| `UnavailableMessageRequester` | `request_unavailable_message` + `chat_disappearing_timer` | supported | `misc/adapter.go:121,204` |
| `DefaultDisappearingTimerSetter` | `account_disappearing_timer_default` | supported | `misc/adapter.go:213` |
| `ChatOperations` (composição) | `chat_ops_full` | supported | `misc/adapter.go:224` — asserção de compilação |
| `ProfileAccessProvider` | `profile_access` | supported | `misc/adapter.go:148,226` |
| `NewsletterReader` — `ListSubscribed`, `CreateNewsletter`, `NewsletterInfo(WithInvite)`, `Follow/UnfollowNewsletter`, `ToggleNewsletterMute`, `NewsletterMessages(Updates)`, `MarkNewsletterViewed`, `SendNewsletterReaction`, `SubscribeNewsletterLiveUpdates`, `DemoteNewsletterAdmin`, `ChangeNewsletterOwner`, `DeleteNewsletter` | `newsletter_*` (12 subcapacidades) | supported | `misc/adapter.go:157,245,258,274,285,299,313,333,346,359,377,391,402,418,434` + asserção `:227` |
| `NewsletterReader` — `CreateNewsletterAdminInvite`, `AcceptNewsletterAdminInvite`, `RevokeNewsletterAdminInvite` | `newsletter_admin_invite` | **broken** | código existe em `misc/adapter.go:445,461,472` — MAS os três estão na lista H187 (sem wrapper de erro); `NewsletterCreateAdminInvite`/`NewsletterAcceptAdminInvite`/`NewsletterRevokeAdminInvite` (nomes do SDK) |
| `AppStateSyncer` | `sync_contact_roster` | supported | `misc/adapter.go:179,228` — asserção de compilação |
| `StatusMessageSetter` | `set_status_message` | supported | `misc/adapter.go:559` |
| `HistorySyncRequester` | `request_history_sync` | supported | `misc/adapter.go:579` |
| `MessageStarrer` | `star_message` | supported | `misc/adapter.go:63,230` — asserção de compilação |
| `SessionCounter` | `count_sessions` | supported | `pkg/infra/noise/adapters/sessioncount/adapter.go:33,52` — asserção de compilação |
| `PhonePairer` | `pair_by_phone` (`IsPaired`, `RequestPairingCode`) | supported | `pkg/infra/noise/adapters/pairing/adapter.go:49,65,88` — asserção de compilação |
| `IdentityResolver` | `identity_lid_pn` (`IsOnWhatsApp`, `GetLIDForPN`, `GetPNForLID`, `GetManyLIDsForPNs`) | supported | `pkg/infra/noise/adapters/user/adapter.go:26,94,119,153` |
| `LIDResolver` | `pn_for_lid` (subset de `IdentityResolver`) | supported | `user/adapter.go:119` — trivialmente satisfeito por qualquer `IdentityResolver` |
| `AvatarReader` | `get_avatar` | supported | `pkg/infra/noise/adapters/user/avatar.go:27` |
| `ContactRoster` | `contact_roster` (`GetAllContacts`, `ContactNames`, `GetUserInfo`) | supported | `user/adapter.go:55,80,189` |
| `ContactDirectory` (composição) | `contact_directory_full` | supported | `pkg/infra/noise/adapters/user/adapter.go:178` — asserção de compilação |
| `BlocklistManager` | `blocklist` | supported | `pkg/infra/noise/adapters/user/blocklist.go:17,35` + `adapter.go:179` |
| `PrivacyManager` | `privacy_settings` | supported | `pkg/infra/noise/adapters/user/privacy.go:11,24` + `adapter.go:180` |
| `SessionProvider` / `Session` | `session_lifecycle_provider` | supported | `pkg/infra/noise/runtime/session/provider.go` |
| `SessionGuard` / `SessionDisconnector` / `SessionLogouter` / `SessionController` | `session_control` | supported | `pkg/infra/noise/runtime/session/guard.go` |
| `SessionAttachHook` / `SessionDetacher` | `session_attach_hooks` | supported | `pkg/infra/noise/runtime/session/events.go` |
| `SessionRegistry` | `session_registry` | supported | `pkg/infra/noise/registry/manager.go`, `pkg/infra/noise/registry/clients/clients.go` |
| `ProfileDataAccess` | `profile_data_access` (pushname/ownJID/avatar/contact/device info) | supported | `pkg/infra/noise/adapters/profile/data_access.go:37,45,64,80,104` |
| `DeleteUserProvider` | — | N/A | não é capacidade de protocolo; verificar dono real fora desta worktree (não localizado sob `pkg/infra/noise`) |
| `UserInfoRepublisher` | — | N/A | webhook/republish, não protocolo; implementação fora de `pkg/infra/noise` |
| `MessagingPort` | — | N/A | RabbitMQ, não noise |
| `HmacKeyStore` / `HmacKeyEncryptor` / `UserInfoHmacCache` | — | N/A | config/cripto, não protocolo |
| `S3ConfigStore` / `S3SecretCipher` / `S3ClientManager` / `UserInfoS3Cache` | — | N/A | storage, não protocolo |
| `HistoryConfigStore` / `ProxyConfigStore` / `UserInfoHistoryCache` / `UserInfoProxyCache` | — | N/A | configuração persistida, não protocolo |
| `UserRepository` / `SessionStatusReader` | — | N/A | DB |
| `StoragePort` | — | N/A | storage genérico |
| `ChatHistoryReader` | — | N/A | DB (`pkg/infra/db/chat_history_repository.go`) |
| `ChatActivityReader` | — | N/A | DB (leitura local, não exige sessão noise) |
| `MediaFetcher` | — | N/A | `pkg/infra/media/opengraph/url_fetcher.go` — genérico, não específico de noise |
| `LinkPreviewFetcher` | — | N/A | `pkg/infra/media/opengraph/adapter.go` — idem |
| `StickerProcessor` | — | N/A | `pkg/infra/media/sticker/adapter.go` — idem |
| `Logger` | — | N/A | infra transversal, não protocolo |

## Contagem

- **76 interfaces** em `pkg/application/contracts/*.go`.
- **~50 métodos/capacidades noise-relevantes** mapeados (agrupados em ~40
  linhas de porta acima, algumas portas cobrindo várias sub-capacidades como
  `NewsletterReader`).
- **supported**: todas as linhas acima exceto as sete marcadas.
- **broken**: 2 linhas de tabela (`CommunityDirectory`, `CommunityLifecycle`)
  + 1 linha (`newsletter_admin_invite`, 3 métodos) = **7 métodos concretos**
  sem wrapper de tradução de erro (`GetSubGroups`, `GetLinkedGroupsParticipants`,
  `LinkGroup`, `UnlinkGroup`, `NewsletterCreateAdminInvite`,
  `NewsletterAcceptAdminInvite`, `NewsletterRevokeAdminInvite`) — ver
  HOUSEKEEP H187. O código existe e compila; o defeito é na tradução de erro
  do servidor, não na ausência de implementação.
- **not_implemented**: nenhuma porta application-level relevante ao motor
  noise ficou sem adapter. A única lacuna de CAPACIDADE real da biblioteca
  (não de porta) encontrada foi `GetBusinessProfile` — ver seção seguinte.
- **N/A** (fora do escopo do motor): 18 interfaces de persistência/config/
  mensageria/utilitário genérico, listadas na tabela.

## Diferença Personal vs Business no noise

Evidência encontrada no código (biblioteca vendorizada, não nos adapters
`pkg/`):

1. **`internal/noise/capabilities/user/business.go`** implementa
   `GetBusinessProfile` (consulta `w:biz` por perfil de conta business —
   endereço, email, horário de funcionamento, categorias) e
   `ParseVerifiedName`/`ParseVerifiedNameContent` (certificado de nome
   verificado). O comentário em `ParseVerifiedName` é explícito sobre a
   diferença: *"Ausência não é erro: um usuário comum, sem conta business,
   cai aqui"* (`business.go:143-149`) — ou seja, o parser já trata "conta
   pessoal" como caminho normal (nó ausente), não como erro.
2. **`UpdateBusinessName`** (`business.go:95`) só dispara quando o nome
   verificado de uma conta business muda; é chamado do fluxo de parsing de
   mensagem e não tem efeito para contas pessoais (não há nome business a
   mudar).
3. **Gap identificado**: `GetBusinessProfile` é uma capacidade REAL da
   biblioteca (`internal/noise/capabilities/user/business.go:72`), mas
   **não há porta em `pkg/application/contracts/` nem adapter em
   `pkg/infra/noise/adapters/` que a exponha** — não localizado nenhuma
   referência a `GetBusinessProfile`/`BusinessProfile` fora do próprio pacote
   da biblioteca. Não é um bug (está fora do escopo desta tarefa decidir se
   deve ser exposta), mas é informação relevante para a matriz de capacidade:
   "perfil business" é uma capacidade que o motor SABE fazer e que o wa-api
   hoje não pede.

**Sem evidência no código atual** de qualquer outro tratamento diferenciado
por tipo de conta (Personal vs Business) nos adapters de `pkg/infra/noise`
ou nos 13 subpacotes de `internal/noise/capabilities/` além do que está
descrito acima — não há condicional `if isBusiness`/`AccountType` nos
adapters, nem teste que distinga os dois tipos de conta.
