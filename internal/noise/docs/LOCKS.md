# Mapa de locks de `internal/wa-noise/`

Inventário dos primitivos de concorrência do fork, **construído por inspeção do
código no HEAD**, não de memória. Método:

```
grep -rn "sync.Mutex\|sync.RWMutex\|sync.Map" internal/wa-noise --include="*.go" \
  | grep -v _test.go | grep -v "/proto/"
grep -rn "<nome>.\(Lock\|RLock\)()" internal/wa-noise --include="*.go" | grep -v _test.go
```

`protocol/proto/` e `protocol/binary/proto/` ficam de fora: são gerados e não
contêm concorrência nossa.

A regra que este documento existe para tornar verificável é a regra 6 do
`CONTRIBUTING.md`: **estado e o lock que o protege moram no mesmo pacote, na
mesma struct.** A tabela abaixo tem exatamente uma linha que viola isso, e ela
está marcada.

---

## 1. Tabela

| Estado/recurso | Pacote dono | Lock | Leitores | Escritores | Notas |
|---|---|---|---|---|---|
| `socket` (`*socket.NoiseSocket`), `socketWait` | `core` | `socketLock sync.RWMutex` — `core/client.go:50` | `RLock` em `client_connection.go:25,43,54,226`, `connectionevents.go:58,126`, `client_events.go:202`, `request.go:218` | `Lock` em `client_connection.go:32,85,99,152,240,253` | O lock mais quente do fork. É a razão pela qual o núcleo é **um** pacote só: `core/`. Ver `CONTRIBUTING.md` regra 10. `core/handshake.go:27` documenta que `cli.socket = ns` roda sob o `Lock()` tomado por `client_connection.go`. |
| `responseWaiters` (map de canais de resposta de IQ) | `core` | `responseWaitersLock sync.Mutex` — `core/client.go:102` | — (todo acesso é sob `Lock`) | `request.go:58,72,79,90` | Só `Mutex`: os quatro acessos escrevem no mapa. |
| `eventHandlers` (slice de handlers registrados) | `core` | `eventHandlersLock sync.RWMutex` — `core/client.go:107` | `RLock` em `client_events.go:224` (despacho) | `Lock` em `client_events.go:77,96,117` (Add/Remove) | `RWMutex` porque o despacho é o caminho quente e é leitura. |
| Ordenação de envio de mensagem | `capabilities/send` (declara e usa) | `sendLock sync.Mutex` — `capabilities/send/state.go:30`, campo privado de `send.State` | — | `send/message.go:74`, `send/fb_message.go:123`, via `t.State().SendLock()` | **Corrigido (F58, commit `ef5c599`, posterior à Fase H).** Até então o mutex era campo de `core.Client` (`messageSendLock`) e emprestado por ponteiro para `capabilities/send` — a única violação real da regra 6 no fork. `send.State` passou a possuir o mutex privadamente, exposto por `func (s *State) SendLock() *sync.Mutex { return &s.sendLock }` (`state.go:37`), no mesmo desenho que `retry`/`prekeys`/`tctoken` já usavam. `core.Client` não tem mais o campo. Ver `HOUSEKEEP.md` **F58** para a evidência completa e `FASE_H_INVENTORY.md` §4 para o histórico. |
| Canal de QR (`closed`) | `core` | `sync.Mutex` embutido — `core/qrchan.go:49`; mais `closed uint32` operado por `atomic.CompareAndSwapUint32` — `qrchan.go:63,87,101,142` e `atomic.LoadUint32` — `:73,111` | `atomic.Load` | `atomic.CAS` | O fechamento do canal é decidido por CAS, não pelo mutex; o mutex protege a emissão. |
| Flags do `Client` (`isLoggedIn`, `forceAutoReconnect`, `sendActiveReceipts`, `privacySettingsCache`, `idCounter`, `serverTimeOffset`) | `core` | `atomic.*` — `core/client.go:53-170` | — | — | Campos atômicos da própria struct; não têm mutex e não precisam. |
| `ConnCache` (conn de mídia cacheada) | `capabilities/media` | `lock sync.Mutex` — `media/conn.go:44` | — | `conn.go:58,67,80` | **Revisado independentemente** (F/G lote 1, `PATCHES.md:5168+`) — e só na segunda tentativa: o commit `160b386` alegou revisão que não aconteceu, foi retratado por `8d4ec40`, e a revisão real (`security-reviewer`, spawn limpo, focada em `ConnCache.Refresh` vs. `refreshMediaConn` original) fechou a pendência. |
| `State` de prekeys (upload em voo) | `capabilities/prekeys` | `uploadLock sync.Mutex` — `prekeys/state.go:30` | — | `prekeys/state.go:40` `LockUpload()` | **Revisado independentemente** (F/G lote 4, `PATCHES.md:6119+`, "a alegação se sustenta", tabela statement-a-statement de `uploadPreKeys` → `prekeys.Upload`). Substituiu `cli.uploadPreKeysLock` da raiz (documentado em `state.go:37`). |
| `Cache` de metadados de grupo | `capabilities/group` | `lock sync.Mutex` — `group/cache.go:49` | — | `cache.go:60` (`Lock()` público), `:89`; call sites em `group/notification.go:200`, `info.go:139,202` | **Revisado independentemente** (F/G lote 6, `PATCHES.md:7236+`). O revisor declarou o que **não** verificou: não construiu teste de estresse dirigindo `GetOrFetch` e `UpdateParticipantCache` concorrentemente contra socket vivo. Ressalva aceita e registrada em `HOUSEKEEP.md` **F53** (ponteiro vivo devolvido por `GetOrFetch`) — pré-existente e idêntica no HEAD, não introduzida pelo lote. `Lock()` é exportado de propósito: `cache.go:38` explica que dentro de `GetOrFetch` o lock **já** está segurado e um segundo `Lock()` seria deadlock (`sync.Mutex` não é reentrante). |
| `DeviceCache` (mapa de dispositivos por JID) | `capabilities/user` | `lock sync.Mutex` — `user/cache.go:53` | — | `cache.go:65` (`Lock()` público), `:103,112`; call sites em `user/devices.go:27` | **Revisado independentemente** (F/G lote 7, `PATCHES.md:7576+`), mesma limitação declarada: sem teste de estresse `GetDevices` × `handleDeviceNotification`. Ressalva registrada em `HOUSEKEEP.md` **F54** (a criação preguiçosa do mapa alargou a janela; não é classe nova de bug, e a correção limpa esbarraria em `internals.go`/F29). `user/devices.go:159` documenta o mesmo motivo do `Lock()` exportado do `group`. |
| `State` de app state sync | `capabilities/appstatesync` | `syncLock sync.Mutex` — `appstatesync/state.go:31`; `keyRequestsLock sync.RWMutex` — `:34` | `keyRequestsLock.RLock` em `state.go:89` | `state.go:44` `LockSync()`; `keyRequestsLock.Lock` em `:62` | **Revisado independentemente, com ressalvas declaradas** (F/G lote 3, `PATCHES.md:5826+`) — foi este lote que fechou a pendência que a retratação do lote 1 deixou aberta. |
| `State` de tctoken | `capabilities/tctoken` | `senderTSLock sync.Mutex` — `tctoken/state.go:35`; `dbPruneLock sync.Mutex` — `:44` | — | `state.go:54,67,89`; `dbPruneLock.TryLock()` em `:124` | **NÃO revisado independentemente** (F/G lote 4, `PATCHES.md:6472+`). O próprio registro aponta o ponto que mais mereceria revisão externa: a semântica de `TryStartDBPrune`, um `TryLock()` cujo `Unlock()` atravessa a fronteira da goroutine. Pendência aberta. |
| `State` de retry (5 locks, 7 campos) | `capabilities/retry` | `sessionRecreateLock` — `retry/state.go:65`; `incomingCounterLock` — `:69`; `messageRetriesLock` — `:73`; `recentLock sync.RWMutex` — `:82`; `pendingPhoneLock sync.RWMutex` — `:94` | `recentLock.RLock` em `:216`; `pendingPhoneLock.RLock` em `:260` | `:103` `LockSessionRecreate()`, `:138`, `:159`, `:198`, `:233,247,273` | **NÃO revisado independentemente** (F/G lote 5, `PATCHES.md:6928+`), e é **a maior mudança de concorrência de toda a Fase F/G**. O que a sustenta: `make check` verde com exit 0 observado, 98,1%/100,0% sob `-race`, `go vet` limpo, e comparação manual — feita por quem escreveu o commit. Maior pendência de revisão do fork. |
| `handlerActive` (flag do handler de history sync) | `capabilities/message` | `atomic.Bool` — `message/state.go:33` | — | — | O ponteiro precisa ser **estável**. F/G lote 9 (`PATCHES.md:8272`) não fez revisão dedicada porque não há concorrência nova; mas `:8285` registra **dívida pré-existente não corrigida**: com `EnableDecryptedEventBuffer` ligado e mensagens decifradas concorrentemente, há janela. |
| Cache de chaves do `Processor` de app state | `protocol/appstate` | `keyCacheLock sync.Mutex` — `appstate/keys.go:22` | — | `keys.go:75` | Vive em `protocol/`, não em `capabilities/` — é o codec, não o sincronizador. Ver `DEPENDENCIES.md` §4. |
| Frames pendentes do websocket | `protocol/socket` | `lock sync.Mutex` — `socket/framesocket.go:27` | — | `framesocket.go:63,91` | Coberto pela revisão independente do F/G lote 10 (`PATCHES.md:8788`, "SAFE TO COMMIT"), item (C): captura de mutex por valor nos tipos novos. |
| Escrita cifrada no socket | `protocol/socket` | `writeLock sync.Mutex` — `socket/noisesocket.go:26` | — | `noisesocket.go:92` | Idem. Mais `destroyed atomic.Bool` e o contador do `noisehandshake`. |
| Prekeys em SQL | `persistence/store/sqlstore` | `preKeyLock sync.Mutex` — `sqlstore/store.go:40` | — | `store_prekey.go:46,56` | Sem revisão dedicada: não foi tocado pela Fase F/G nem pela H (só mudou de caminho). |
| Cache de contatos em SQL | `persistence/store/sqlstore` | `contactCacheLock sync.Mutex` — `sqlstore/store.go:43` | — | `store_contact.go:58,79,100,143,174,211,221` | Idem. |
| Cache de LID↔PN | `persistence/store/sqlstore` | `lidCacheLock sync.RWMutex` — `sqlstore/lidmap.go:33` | `RLock` em `lidmap.go:93,146` | `Lock` em `lidmap.go:60,103,166,206,218` | Idem. |
| Sessões Signal | `persistence/store` | `sessionCache = exsync.Map[...]` — `store/sessioncache.go:31` | (internos do `exsync.Map`) | (idem) | Não é `sync.Mutex` nosso: é o mapa concorrente de `go.mau.fi/util/exsync`. |

---

## 2. O que a etapa 7 (`runtime/`) acrescentou a esta tabela

**Nada.** `runtime/keepalive/` e `runtime/proxy/` foram inspecionados
explicitamente durante a etapa 7 e **não contêm nenhum primitivo de
concorrência próprio**:

- `runtime/keepalive/` coordena por `context.Context`, `time.Timer` e canais; o
  estado que ele lê (`Timing`, `AutoReconnectEnabled`) chega pela interface
  `keepalive.Transport` (`runtime/keepalive/transport.go:40`, 8 métodos), e o
  `Disconnect()` que ele chama **já segura `socketLock` por dentro** — está
  documentado na própria interface. O par `ctx`/`connCtx` do keepalive foi o
  item (B) da revisão independente do F/G lote 10 (`PATCHES.md:8788`), veredito
  SAFE TO COMMIT.
- `runtime/proxy/` (pacote `proxyconf`) **não guarda estado**: recebe os três
  ponteiros de `http.Client` em `Clients` e escreve o campo `Transport` de cada
  um. É o que o próprio doc comment do pacote diz.

Portanto a etapa 7 é a única etapa da Fase H que move código sem tocar em
nenhuma linha desta tabela — que é exatamente o motivo de ela ser segura.

---

## 3. Placar de revisão independente

| Revisado de fato | Não revisado independentemente | Sem concorrência nova (revisão dispensada) |
|---|---|---|
| `media.ConnCache` (2ª tentativa) | `sqlstore` e `store` (nunca tocados) | `newsletter` (F/G lote 2) |
| `prekeys.State` | | `send` (F/G lote 8, `PATCHES.md:7951`) |
| `appstatesync.State` (com ressalvas) | | `message` (F/G lote 9, `PATCHES.md:8274`) |
| `group.Cache` (ressalva → `HOUSEKEEP.md` F53) | | |
| `user.DeviceCache` (ressalva → `HOUSEKEEP.md` F54) | | |
| `core.socketLock` + `keepalive` + `handshake` (F/G lote 10, SAFE TO COMMIT) | | |
| **`retry.State`** (5 locks — `sessionRecreateLock`, `incomingCounterLock`, `messageRetriesLock`, `recentLock`, `pendingPhoneLock`) — revisado 2026-08-12 | | |
| **`tctoken.State`** (2 locks — `senderTSLock`, `dbPruneLock`) — revisado 2026-08-12 | | |

`retry.State` (revisão 2026-08-12): todos os 5 locks têm o invariante que
protegem declarado e cada acesso ao campo protegido confirmado sob o lock
correto (nenhum acesso fora de `state.go`). Um padrão de lock-segurado-durante-
I/O existe (`sessionRecreateLock` durante `Store().ContainsSession()`), mas é
**pré-existente à extração** (idêntico ao commit anterior `080b948^`), não uma
regressão, e não viola o invariante do F86/F88 (não é pool de slot limitado
disputado por múltiplos assinantes — é um mutex único por conexão). Veredito:
**seguro**. `go test -race ./capabilities/retry/...` — PASS.

`tctoken.State` (revisão 2026-08-12): **2 locks**, não 5 (a contagem de 5 é a
de `retry.State`, PATCHES.md tinha o número certo mas atribuição ambígua).
`senderTSLock` protege `senderTS`/`lastCleanup`, todo acesso sob lock, sem I/O
segurado. `dbPruneLock` (via `TryStartDBPrune`/`FinishDBPrune`) serializa a
poda no banco; o `DELETE` roda dentro do `defer FinishDBPrune()` de uma
goroutine própria (`tctoken.go:104-117`) — o lock fica segurado durante I/O de
banco, mas, como no caso de `retry.State`, é um mutex único que só a própria
poda disputa, não um pool de slots compartilhado por outros assinantes; não
viola o invariante do F86/F88. Veredito: **seguro**. `go test -race -count=1
./capabilities/tctoken/...` — PASS.

O critério de honestidade usado em todo o `PATCHES.md`, e repetido aqui:
comparação manual feita **por quem escreveu o commit não é revisão
independente**, e é registrada como tal. As duas revisões acima foram feitas
em 2026-08-12, na reconciliação HOUSEKEEP/PATCHES, por revisor sem envolvimento
na extração original — fecha a maior dívida de verificação apontada neste
placar.
