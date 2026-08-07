# Fase H — Inventário (etapa 1)

Levantamento factual do estado de `internal/wa-noise/` **antes** de qualquer
movimentação da Fase H. Nada foi movido para produzir este documento; todos os
números vêm de `grep`/`ls`/leitura dos arquivos no commit corrente da branch
`feature/vendor-whatsmeow`.

Este documento é o contrato de execução das etapas 3-7. Onde ele diverge da
especificação original da Fase H, a divergência está marcada como
**⚠ DECISÃO PENDENTE** e precisa de resposta do usuário antes da etapa
correspondente.

Árvore-alvo (resumo da especificação):

```
internal/wa-noise/
├── main.go            (fachada, package whatsmeow)
├── core/              client.go, client_connection.go, request.go, internals*.go
├── capabilities/      group user media message newsletter notification pairing
│                      prekeys retry send tctoken appstatesync
├── protocol/          argo binary proto socket types (+types/events)
├── security/          cbc gcm handshake hkdf keys
├── persistence/       signal sql
├── runtime/           keepalive proxy
├── observability/     log
└── docs/
```

Direção de dependência declarada:
`capabilities -> protocol | security | persistence`, `core -> capabilities + runtime`,
**nada importa `core`**.

---

## 1. Lista de pacotes atuais

### 1.1 Pacotes de primeiro nível

| Diretório | Propósito (1 linha) | Categoria-alvo |
|---|---|---|
| `appstate/` | Codificação/decodificação de patches de app state (LTHash, MACs, mutações, recovery) — biblioteca pura de protocolo | ⚠ ver §6.1 |
| `appstate/lthash/` | Implementação do LTHash (hash homomórfico) usado por `appstate/` | junto de `appstate/` |
| `appstatesync/` | Orquestração de sync de app state (fetch, dispatch, key requests) com `Transport` própria | `capabilities/appstatesync/` |
| `argo/` | Codec do protocolo binário "argo" usado pelas queries MEX de newsletter | `protocol/argo/` |
| `binary/` | Codec do XML binário do WhatsApp (`waBinary`): marshal/unmarshal de `Node` | `protocol/binary/` |
| `binary/token/` | Tabelas de tokens do dicionário do codec binário | junto de `binary/` |
| `group/` | Domínio de grupos: cache de metadados, parsing, IQs; `Transport` própria | `capabilities/group/` |
| `handshake/` | Noise_XX_25519_AESGCM_SHA256 + verificação da cadeia de certificados do servidor | `security/handshake/` |
| `keepalive/` | Ping periódico do websocket com `Transport` própria | `runtime/keepalive/` |
| `media/` | Upload/download/retry de mídia, `ConnCache`; `Transport` própria | `capabilities/media/` |
| `message/` | Recepção/decriptação/parse de mensagem; `Transport` própria | `capabilities/message/` |
| `msgattrs/` | Funções puras de extração de atributos de mensagem (sem estado, sem `Transport`) | ⚠ ver §6.2 |
| `msgpad/` | Padding/unpadding de payload de mensagem (2 funções puras, zero imports internos) | ⚠ ver §6.2 |
| `newsletter/` | Domínio de newsletter/canais (MEX queries); `Transport` própria | `capabilities/newsletter/` |
| `notification/` | Handlers de notificação (device, privacy, newsletter); `Transport` própria | `capabilities/notification/` |
| `paircrypto/` | Assinatura/verificação ADV do pareamento (funções puras de cripto) | ⚠ ver §6.2 |
| `pairing/` | Pareamento QR + pair-code; `Transport` própria | `capabilities/pairing/` |
| `prekeys/` | Upload/consulta de prekeys, `State` com `uploadLock`; `Transport` própria | `capabilities/prekeys/` |
| `proto/` (+69 subpacotes) | **Gerado** — mensagens protobuf do WhatsApp | `protocol/proto/` |
| `proxyconf/` | Configuração de proxy HTTP/SOCKS (sem estado compartilhado) | `runtime/proxy/` |
| `retry/` | Retry receipts, mensagens recentes, request-from-phone; `State` com 5 locks | `capabilities/retry/` |
| `send/` | Envio de mensagem (E2E e FB/armadillo); `Transport` própria | `capabilities/send/` |
| `socket/` | FrameSocket + NoiseSocket (websocket + cifra de transporte) | `protocol/socket/` |
| `store/` | Interfaces de persistência + `Device` + session cache (Signal store) | `persistence/signal/` |
| `store/sqlstore/` | Implementação SQL das interfaces de `store/` | `persistence/sql/` |
| `store/sqlstore/upgrades/` | Migrações SQL versionadas | `persistence/sql/upgrades/` |
| `tctoken/` | Trusted-contact tokens, `State` com 2 locks; `Transport` própria | `capabilities/tctoken/` |
| `types/` | Tipos de domínio compartilhados (JID, GroupInfo, Message, Presence…) | `protocol/types/` |
| `types/events/` | Tipos de evento emitidos pelo cliente | `protocol/types/events/` |
| `user/` | Domínio de usuário: cache de dispositivos, avatar, queries; `Transport` própria | `capabilities/user/` |
| `util/cbcutil/` | AES-CBC | `security/cbc/` |
| `util/gcmutil/` | AES-GCM | `security/gcm/` |
| `util/hkdfutil/` | HKDF | `security/hkdf/` |
| `util/keys/` | KeyPair Curve25519 / chaves pré-assinadas | `security/keys/` |
| `util/log/` | Logger (`waLog`) | `observability/log/` |

`util/` em si não contém `.go` — é só diretório-pai, some naturalmente.

### 1.2 Arquivos não-Go na raiz (não movem)

`PATCHES.md`, `PROVENANCE.md`, `UPSTREAM`, `LICENSE-whatsmeow`,
`reportingfields.json` (lido por `reportingtoken.go`).

---

## 2. Inventário dos arquivos `.go` da raiz

114 arquivos: **92 de produção + 22 de teste**. Pacote: `whatsmeow` (exceto
`client_test.go`, que é `whatsmeow_test`).

Legenda de risco:
- **F** = fachada/adaptador fino sobre um subpacote — movimentação mecânica
- **C** = núcleo do client (toca `socketLock` ou define o `Client`) — cuidado
- **G** = gerado — só atualizar import path
- **R** = lógica ainda não extraída, vive de fato na raiz — cuidado

### 2.1 Núcleo do client → `core/`

| Arquivo | Linhas | Risco | Nota |
|---|---|---|---|
| `client.go` | 270 | C | Define `Client`, `socketLock:50`, 11 primitivos de concorrência |
| `client_connection.go` | 269 | C | 20 usos de `socketLock` |
| `client_events.go` | 238 | C | `socketLock:202,204`; `eventHandlersLock` |
| `client_session.go` | 173 | C | Não toca `socketLock`, mas é estado de sessão do `Client` |
| `connectionevents.go` | 234 | C | `socketLock:58,59,126,127` |
| `request.go` | 249 | C | `socketLock:218,220`; `responseWaitersLock` |
| `internals.go` | 757 | **G** | Gerado; wrappers `DangerousInternals` |
| `internals_generate.go` | 211 | **G** | Gerador; **bug F29 conhecido — NÃO corrigir aqui** |
| `errors.go` | 284 | C | Reexporta erros de 8 subpacotes; API pública histórica |
| `qrchan.go` | 177 | C | `QRChannel` com `sync.Mutex` embutido + 7 usos de `atomic` |
| `connection_constants.go` | 100 | C | Constantes de conexão |
| `update.go` | 76 | C | Checagem de versão (usa `socket`, `store`) |
| `client_proxy.go` | 112 | F | Fachada sobre `proxyconf/` (já extraído) |
| `keepalive.go` | 104 | F | Fachada sobre `keepalive/` |
| `handshake.go` | 60 | F | Fachada sobre `handshake/`; contém `cli.socket = ns` |

Testes pareados: `client_test.go` (80, `whatsmeow_test`), `client_connection_test.go` (241),
`client_events_test.go` (327), `client_proxy_test.go` (193), `client_session_test.go` (403),
`connectionevents_test.go` (386), `errors_test.go` (293), `keepalive_test.go` (118).

### 2.2 Fachadas de capacidade (movem junto com a capacidade correspondente)

| Capacidade | Arquivos da raiz | Risco |
|---|---|---|
| `appstatesync` | `appstate.go` (81), `appstate_dispatch.go` (45), `appstate_keys.go` (31), `appstate_send.go` (86), `appstate_transport.go` (109) + `appstate_send_test.go`, `appstate_transport_test.go` | F |
| `group` | `group.go` (84), `group_create.go` (59), `group_invite.go` (67), `group_notification.go` (33), `group_parse.go` (24), `group_participants.go` (60), `group_settings.go` (86), `group_transport.go` (100) | F |
| `media` | `download.go` (102), `download-to-file.go` (86), `download_types.go` (52), `download_transport.go` (46), `upload.go` (82), `upload_newsletter.go` (66), `mediaconn.go` (36), `mediaretry.go` (151), `media_transport.go` (70) + `mediaretry_test.go`, `media_transport_test.go` | F |
| `message` | `message.go` (47), `message_adapter.go` (167), `message_builders.go` (83), `message_decrypt.go` (52), `message_decrypt_session.go` (39), `message_history_sync.go` (51), `message_id.go` (43), `message_parse.go` (34), `message_secrets_store.go` (40), `msgsecret.go` (102), `msgsecret_keys.go` (65), `msgsecret_poll.go` (81) + `message_facade_test.go` | F |
| `newsletter` | `newsletter.go` (180), `newsletter_transport.go` (86) | F |
| `notification` | `notification.go` (175), `notification_device.go` (146), `notification_newsletter.go` (41), `notification_privacy.go` (82), `notification_transport.go` (41) + `notification_test.go` | F (mas ver §4: `notification_device.go` é o maior consumidor do `user.Cache`) |
| `pairing` | `pair.go` (79), `pair-code.go` (75), `pair_constants.go` (37), `pair_transport.go` (111) + `pair_transport_test.go` | F |
| `prekeys` | `prekeys.go` (69), `prekeys_transport.go` (61) + `prekeys_transport_test.go` | F |
| `retry` | `retry.go` (55), `retry_constants.go` (32), `retry_receipt_send.go` (25), `retry_recent_messages.go` (58), `retry_request_from_phone.go` (67), `retry_transport.go` (158) + `retry_test.go` | F |
| `send` | `send.go` (77), `send_adapter.go` (173), `send_constants.go` (41), `send_facade.go` (179), `send_types.go` (40), `sendfb.go` (50), `sendfb_facade.go` (103) + `send_facade_test.go` | F, **exceto `send_adapter.go:68`** — ver §4 |
| `tctoken` | `tctoken.go` (90), `tctoken_transport.go` (63) + `tctoken_transport_test.go` | F |
| `user` | `user.go` (144), `user_queries.go` (120), `user_transport.go` (103) | F |

### 2.3 Arquivos da raiz **sem** subpacote correspondente (R — lógica ainda na raiz)

Estes **não são fachadas**. A Fase F/G nunca os extraiu; eles não têm destino
óbvio na árvore-alvo. São o maior risco da Fase H.

| Arquivo | Linhas | Importa | Observação |
|---|---|---|---|
| `armadillomessage.go` | 133 | proto/*, types, types/events | Decodificação de mensagem armadillo — logicamente pertence a `message/`, mas nunca foi extraído |
| `broadcast.go` | 143 | binary, types | Lista de transmissão / status |
| `call.go` | 125 | binary, types, types/events | Sinalização de chamadas |
| `cstoken.go` | 86 | types | Tokens CS |
| `disappearing_timer.go` | 86 | binary, types | Timer de mensagens efêmeras |
| `presence.go` | 161 | binary, types, types/events | Presença/typing |
| `privacysettings.go` | 178 | binary, types, types/events | Configurações de privacidade |
| `push.go` | 108 | binary, types | Registro de push |
| `receipt.go` | 289 + `receipt_constants.go` (27) | binary, types, types/events | Recibos de entrega/leitura |
| `reportingtoken.go` | 198 | binary, types | Tokens de reporte (lê `reportingfields.json`) |
| `token_constants.go` | 49 | — | Constantes de token |
| `misc_test.go` | 299 | binary, types, events, log | **Teste órfão**: cobre vários dos arquivos acima, não pareia com um `.go` só |

Testes pareados aqui: `receipt_test.go` (310), `reportingtoken_test.go` (220).

**⚠ DECISÃO PENDENTE (§2.3):** a especificação da Fase H não diz onde estes 12
arquivos vão. Três saídas possíveis, em ordem de menor risco:
1. **Ficam na raiz junto de `main.go`** (a raiz continua sendo um pacote real,
   não só uma fachada). Menor diff, mas a raiz deixa de ser "só fachada".
2. Vão para `core/` junto com o `Client` — coerente com o fato de que todos são
   métodos de `*Client` que usam `cli.SendIQ`/`cli.sendNode`, mas incha `core/`.
3. Viram novas capacidades (`capabilities/presence/`, `capabilities/receipt/`…) —
   é uma extração de verdade, escopo de uma Fase F/G nova, **não** de uma
   reorganização de diretórios. Não recomendado dentro da Fase H.

Recomendação: **opção 2** para a etapa de `core/`, porque todos definem métodos
em `*Client` e mover para `core/` mantém o pacote único; qualquer outra opção
exige quebrar o tipo `Client` em dois pacotes, o que a análise do lote 10
(`PATCHES.md`, "Fase F/G — lote 10") já mostrou ser inviável.

---

## 3. Arquivos gerados

Confirmado por leitura:

| Alvo | Evidência | Regra na Fase H |
|---|---|---|
| `proto/` e seus 69 subpacotes | cabeçalho `// Code generated by protoc-gen-go. DO NOT EDIT.` | apenas `mkdir`/`git mv` + reescrita de import path; zero edição manual |
| `internals.go` (757 linhas) | gerado por `internals_generate.go` | idem |
| `internals_generate.go` (211 linhas) | `//go:build ignore`-style gerador com **lista de arquivos hardcoded** | idem. **Bug F29** (`HOUSEKEEP.md:908` e adendo em `:971`) — `go generate` hoje derruba 96 dos 178 wrappers. **NÃO corrigir na Fase H.** Porém: a lista hardcoded contém **nomes de arquivo da raiz**; mover arquivos para `core/` a invalida ainda mais. Registrar o impacto no `HOUSEKEEP.md` do F29 ao executar a etapa de `core/`. |

---

## 4. Mapa de locks e estado possuído

Regra: estado e mutex viajam **juntos**. Inventário completo dos primitivos de
concorrência (excluindo `_test.go` e `proto/`):

| Primitivo | Local atual | Estado que protege | Local novo | Separa? |
|---|---|---|---|---|
| `socketLock sync.RWMutex` | `client.go:50` | `cli.socket`, `cli.socketWait` (mesma struct) | `core/client.go` | não |
| `responseWaitersLock` | `client.go:102` | `responseWaiters` | `core/` | não |
| `eventHandlersLock` | `client.go:107` | `eventHandlers` | `core/` | não |
| `messageSendLock` | `client.go:117` | ordenação de envio | `core/` | **⚠ já separado — ver abaixo** |
| `atomic.*` (isLoggedIn, forceAutoReconnect, sendActiveReceipts, privacySettingsCache, idCounter, serverTimeOffset) | `client.go:53-170` | campos do próprio `Client` | `core/` | não |
| `sync.Mutex` embutido + `closed uint32` | `qrchan.go:49,63…` | canal de QR | `core/` (junto de §2.1) | não |
| `retry.State`: `sessionRecreateLock`, `incomingCounterLock`, `messageRetriesLock`, `recentLock`, `pendingPhoneLock` | `retry/state.go:65,69,73,82,94` | mapas/buffers na mesma struct | `capabilities/retry/state.go` | não |
| `tctoken.State`: `senderTSLock`, `dbPruneLock` | `tctoken/state.go:35,44` | idem | `capabilities/tctoken/` | não |
| `appstatesync.State`: `syncLock`, `keyRequestsLock` | `appstatesync/state.go:31,34` | idem | `capabilities/appstatesync/` | não |
| `user.Cache.lock` | `user/cache.go:53` | mapa de dispositivos na mesma struct | `capabilities/user/` | não |
| `group.Cache.lock` | `group/cache.go:49` | mapa de metadados na mesma struct | `capabilities/group/` | não |
| `prekeys.State.uploadLock` | `prekeys/state.go:30` | upload em voo | `capabilities/prekeys/` | não |
| `message.State.handlerActive atomic.Bool` | `message/state.go:33` | flag de handler de history sync (ponteiro precisa ser **estável**) | `capabilities/message/` | não |
| `media` `ConnCache.lock` | `media/conn.go:44` | conn de mídia cacheada | `capabilities/media/` | não |
| `appstate` `keyCacheLock` | `appstate/keys.go:22` | cache de chaves no `Processor` | ⚠ ver §6.1 | não |
| `socket` `framesocket.lock`, `noisesocket.writeLock`, `destroyed atomic.Bool`, `noisehandshake.counter` | `socket/*.go` | estado do socket na mesma struct | `protocol/socket/` | não |
| `store/sqlstore`: `preKeyLock`, `contactCacheLock`, `lidCacheLock` | `store/sqlstore/store.go:40,43`, `lidmap.go:33` | caches SQL na mesma struct | `persistence/sql/` | não |
| `store` `sessionCache` (`exsync.Map`) | `store/sessioncache.go:31` | sessões Signal | `persistence/signal/` | não |

### ⚠ O único par estado/lock já separado: `messageSendLock`

```
client.go:117            messageSendLock sync.Mutex          (campo de *Client)
send_adapter.go:68       func (t sendTransport) SendLock() *sync.Mutex { return &t.cli.messageSendLock }
send/transport.go:141    SendLock() *sync.Mutex               (método da interface Transport)
```

O mutex é **declarado no núcleo do client e emprestado por ponteiro para
`send/`**. O comentário em `send/transport.go:135` documenta explicitamente que
o ponteiro precisa ser estável porque é campo de `*Client`.

Consequência para a Fase H: quando `client.go` for para `core/` e `send/` for
para `capabilities/send/`, a interface `send.Transport` continua exportando um
`*sync.Mutex` que pertence a `core.Client`. Isso **não** cria ciclo (a interface
é declarada em `send/` e `core` a satisfaz), mas é a única violação real de
"estado e lock viajam juntos" no fork. **Não corrigir na Fase H** (é mudança de
design, não de diretório) — mas deve ser registrado em `HOUSEKEEP.md` e citado
em `docs/LOCKS.md`.

---

## 5. Mapa de call sites cruzados

### 5.1 Fachadas da raiz → capacidade (todos os arquivos de §2.2)

São ~70 arquivos, cada um importando exatamente o seu subpacote. Movem em bloco
com a capacidade; o import path muda de
`wa-api/internal/wa-noise/<cap>` para `wa-api/internal/wa-noise/capabilities/<cap>`.
Os call sites reversos que a Fase F/G teve de corrigir de fato
(`PATCHES.md:7135-7146`, `:7455-7469`) já estão resolvidos — hoje são chamadas a
métodos, não acessos a campo. **Nada a refazer, só reescrita de import path.**

### 5.2 Capacidade → capacidade (movem atomicamente juntas)

| De → Para | Call sites (produção) |
|---|---|
| `send` → `group` | `send/fb_outbound.go:20`, `send/prepare.go:18`, `send/transport.go:28` |
| `send` → `retry` | `send/fb_outbound.go:25` |
| `send` → `tctoken` | `send/outbound.go:27` |
| `message` → `send` | `message/decrypt_loop.go:20`, `decrypt_session.go:24`, `poll.go:19`, `protocol.go:14` |
| `message` → `media` | `message/history_sync.go:25` |
| `message` → `user` | `message/parse.go:12` |
| `message` → `appstatesync` | `message/transport.go:29` |
| `retry` → `prekeys` | `retry/handle_parts.go:20`, `send.go:17`, `transport.go:16` |
| `user` → `group` | `user/avatar.go:15` |

(+ os `_test.go` correspondentes, listados no grep da §7.)

Ordem topológica segura para mover capacidades uma a uma:
`prekeys, tctoken, group, appstatesync, media, newsletter, notification, pairing`
→ `retry` → `user` → `send` → `message`.
Mover fora dessa ordem quebra o build entre etapas.

### 5.3 Capacidade → não-capacidade (só reescrita de path)

`newsletter → argo`, `pairing → paircrypto`, `media/handshake → socket`,
`retry/send → msgattrs`, `send/message → msgpad`, todos os `→ binary/types/store/util/*`.

---

## 6. Checagem de ciclos — verificada contra os imports reais

Método: `grep -rho '"…wa-noise/…"'` por pacote, agrupado. Resultado por aresta
proibida:

### 6.1 ⚠ VIOLAÇÃO REAL ENCONTRADA — `types/events` → `appstate`

```
types/events/appstate.go:12   "wa-api/internal/wa-noise/appstate"
types/events/appstate.go:187  Name  appstate.WAPatchName
types/events/appstate.go:193  Name  appstate.WAPatchName
```

`types/events` vai para `protocol/`. Se `appstate/` fosse classificado como
capacidade, teríamos **`protocol/` importando `capabilities/`** — proibido.

Além disso `appstate/` importa `store/` (`appstate/keys.go:23,27,80`,
`decode_mutation.go:30,37,43`, `recovery.go:57,69`), ou seja, o destino de
`appstate/` também cria a aresta **`protocol/ → persistence/`**, que a
especificação não declara.

Análise: `appstate/` **não é** uma capacidade — não tem interface `Transport`,
não fala com o servidor, é o codec/criptografia dos patches de app state. Quem
fala com o servidor é `appstatesync/` (essa sim, capacidade). São dois pacotes
distintos e a distinção já existe hoje.

**⚠ DECISÃO PENDENTE:** duas saídas.
- **(a)** `appstate/` → `protocol/appstate/` (+ `protocol/appstate/lthash/`), e a
  árvore passa a admitir explicitamente a aresta `protocol → persistence`
  (documentar em `docs/DEPENDENCIES.md`). É a de menor diff.
- **(b)** Quebrar `appstate.WAPatchName` (um tipo string) para `protocol/types/`
  e só então classificar `appstate/`. Elimina a aresta `types/events → appstate`,
  mas é mudança de API, não de diretório.

Recomendação: **(a)**, com a aresta `protocol → persistence` declarada.
`appstate/` depende de `store/` só por tipos de dado (`store.AppStateMutationMAC`,
`store.Device`, `store.AppStateSyncKey`), não por comportamento.

### 6.2 ⚠ Pacotes sem categoria declarada: `msgpad/`, `msgattrs/`, `paircrypto/`

Nenhum tem `Transport`; todos são funções puras. Destino proposto (usando o
julgamento que a especificação autorizou):

| Pacote | Imports internos | Consumidores | Destino proposto | Racional |
|---|---|---|---|---|
| `msgpad/` | **nenhum** | `message/`, `send/` | `protocol/msgpad/` | padding é regra de wire format |
| `msgattrs/` | `binary`, `proto`, `types`, `types/events` | `internals.go`, `retry/handle.go`, `send/*`, `sendfb_facade.go` | `protocol/msgattrs/` | só depende de `protocol/`; é leitura de atributos do `binary.Node` |
| `paircrypto/` | `proto/waAdv`, `util/keys` | só `pairing/` | `security/paircrypto/` | é assinatura criptográfica ADV; `util/keys` já vai para `security/keys/` |

Nenhum dos três cria aresta proibida no destino proposto.

### 6.3 Aresta `security → protocol` (existe, não declarada)

`handshake/handshake.go:17` importa `socket/`, e `handshake/` também importa
`proto/waCert`, `proto/waWa6`. Ou seja `security/handshake/ → protocol/socket/`
e `→ protocol/proto/`. Não é ciclo (`protocol/` não importa `security/`
— verificado: `socket/` importa apenas `binary/token`, `util/gcmutil`,
`util/log`… ⚠ **`socket/` importa `util/gcmutil`, que vai para `security/gcm/`**,
logo há também **`protocol/socket → security/gcm`**).

Resultado: `protocol/` e `security/` têm arestas **nos dois sentidos, mas entre
pacotes-folha diferentes** (`socket → gcm`, `handshake → socket`). Como Go só
proíbe ciclo entre pacotes concretos, isso compila. Mas invalida a leitura
"security é uma camada abaixo/acima de protocol".

**Recomendação:** `docs/DEPENDENCIES.md` deve declarar as camadas por
**pacote-folha**, não por diretório-categoria, e afirmar apenas as duas invariantes
que realmente importam e são verificáveis:
1. nada importa `core/`;
2. `protocol/`, `security/`, `persistence/`, `observability/` **nunca** importam
   `capabilities/`.

### 6.4 Demais arestas — verificadas, todas limpas

- `persistence/signal` (`store/`) importa `types`, `util/keys`, `util/log`,
  `proto/*` → `protocol` + `security` + `observability`. Nunca `capabilities`. ✅
- `persistence/sql` (`store/sqlstore/`) importa `store/` e `upgrades/`. ✅
- `protocol/binary` importa `binary/token`, `proto/*`, `types`. ✅
- `protocol/types` importa `appstate`(§6.1), `binary`, `proto/*`. ✅ após decisão (a)
- `protocol/argo`, `runtime/proxy` (`proxyconf/`), `observability/log`,
  `security/{cbc,gcm,hkdf,keys}`: **zero** imports de capacidade. ✅
- `runtime/keepalive` importa `binary`, `types/events`, `util/log`. ✅
- **Nada, em pacote nenhum, importa a raiz `wa-api/internal/wa-noise`** exceto
  `client_test.go` (que é `package whatsmeow_test`, na própria raiz). ✅
  Portanto a invariante "nada importa `core`" é satisfeita **desde que `core/`
  não seja importado pelos consumidores externos** — ver §8.

---

## 7. Censo de arquivos de teste

22 `_test.go` na raiz. Todos em `package whatsmeow` exceto `client_test.go`
(`package whatsmeow_test`).

| Teste | `.go` pareado | Viaja com |
|---|---|---|
| `appstate_send_test.go` | `appstate_send.go` | appstatesync |
| `appstate_transport_test.go` | `appstate_transport.go` | appstatesync |
| `client_connection_test.go` | `client_connection.go` | core |
| `client_events_test.go` | `client_events.go` | core |
| `client_proxy_test.go` | `client_proxy.go` | core |
| `client_session_test.go` | `client_session.go` | core |
| `client_test.go` | `client.go` (externo) | core |
| `connectionevents_test.go` | `connectionevents.go` | core |
| `errors_test.go` | `errors.go` | core |
| `keepalive_test.go` | `keepalive.go` | core |
| `media_transport_test.go` | `media_transport.go` | media |
| `mediaretry_test.go` | `mediaretry.go` | media |
| `message_facade_test.go` | fachadas `message_*.go` | message |
| `notification_test.go` | `notification*.go` | notification |
| `pair_transport_test.go` | `pair_transport.go` | pairing |
| `prekeys_transport_test.go` | `prekeys_transport.go` | prekeys |
| `receipt_test.go` | `receipt.go` | §2.3 |
| `reportingtoken_test.go` | `reportingtoken.go` | §2.3 |
| `retry_test.go` | `retry*.go` | retry |
| `send_facade_test.go` | `send_facade.go` | send |
| `tctoken_transport_test.go` | `tctoken_transport.go` | tctoken |
| **`misc_test.go`** | **nenhum** | ⚠ órfão |

**⚠ `misc_test.go` (299 linhas)** cobre vários arquivos de §2.3 ao mesmo tempo
(`binary`, `types`, `types/events`, `util/log`). Ele **não pode ser dividido
mecanicamente**; ou fica onde os arquivos de §2.3 ficarem, ou precisa ser
quebrado à mão. Se §2.3 for para `core/`, `misc_test.go` vai junto sem
problema.

Nenhum outro teste da raiz fica órfão. Nos subpacotes, todos os `_test.go` já
estão no mesmo diretório do código que testam (herança da Fase F/G) e movem
junto por construção.

---

## 8. Risco maior da Fase H: a superfície pública da raiz

`internal/wa-noise` (`package whatsmeow`) é importado por **30 arquivos** fora
do fork, em `pkg/infra/wa-noise/*` e `pkg/infra/history/`:

```
pkg/infra/wa-noise/{waclient,registry,group,user,chat,profile,misc}/…
pkg/infra/history/sync.go
```

A raiz expõe ~732 símbolos exportados de topo (`func`/`type`/métodos
exportados), dos quais 178 são wrappers de `DangerousInternals` em
`internals.go`.

A árvore-alvo diz que a raiz vira **`main.go`, uma fachada**. Levar `Client`
para `core/` significa que `main.go` teria de reexportar toda essa superfície
por alias (`type Client = core.Client`, e um alias por símbolo). Isso é possível
em Go, mas:

- métodos **não** podem ser reexportados por alias — só o tipo. `type Client = core.Client` resolve isso automaticamente (alias de tipo preserva o method set), então é viável;
- constantes, vars de erro e funções de topo precisam de uma linha de alias cada;
- `internals_generate.go` gera `internals.go` a partir de uma lista de arquivos
  hardcoded da raiz (bug F29) — mover os arquivos altera o que ele geraria.

**⚠ DECISÃO PENDENTE (§8):** confirmar se `main.go` deve ser um arquivo de
aliases gerado/mantido à mão, ou se a alternativa mais barata é aceitável:
manter `client.go` & cia. **na raiz** e usar `core/` apenas para o que já é
separável. A análise do lote 10 (`PATCHES.md:8334+`) argumenta que o núcleo é um
pacote só por causa de `socketLock`; mover esse pacote inteiro de `.` para
`./core` é mecanicamente possível, mas custa a camada de aliases acima.

---

## 9. Resumo das decisões pendentes

| # | Assunto | Recomendação |
|---|---|---|
| §2.3 | 12 arquivos da raiz sem subpacote (`presence`, `receipt`, `call`, `push`, …) | mover para `core/` |
| §6.1 | `appstate/` (+`lthash/`) não está na árvore-alvo e é importado por `types/events` | `protocol/appstate/`; declarar aresta `protocol → persistence` |
| §6.2 | `msgpad/`, `msgattrs/`, `paircrypto/` | `protocol/msgpad/`, `protocol/msgattrs/`, `security/paircrypto/` |
| §6.3 | `security ↔ protocol` tem arestas nos dois sentidos (`socket→gcm`, `handshake→socket`) | declarar invariantes por pacote-folha, não por categoria |
| §4 | `messageSendLock` emprestado por ponteiro de `core` para `send` | não corrigir na Fase H; registrar em `HOUSEKEEP.md` e `docs/LOCKS.md` |
| §8 | 30 importadores externos da raiz + 732 símbolos exportados | confirmar estratégia de `main.go` antes da etapa de `core/` |
| §3 | F29 piora ao mover arquivos da raiz | não corrigir; anotar impacto no `HOUSEKEEP.md` |

---

## 10. Estado após a etapa 2 (scaffold)

Diretórios criados, vazios, sem nenhum `.go`:

```
internal/wa-noise/core/
internal/wa-noise/capabilities/
internal/wa-noise/protocol/
internal/wa-noise/security/
internal/wa-noise/persistence/
internal/wa-noise/runtime/
internal/wa-noise/observability/
internal/wa-noise/docs/          (contém este arquivo)
```

Nenhum destes é um pacote Go — são agrupadores de sistema de arquivos.
`capabilities/group/` é que será o pacote, não `capabilities/`.

`go build ./...` verificado **após** o scaffold: passa, sem alteração.
