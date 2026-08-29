# Dependências de `internal/wa-noise/`

Documento de arestas **reais**, verificadas por `grep` no HEAD. Nada aqui é
aspiracional: onde a árvore não é uma camada limpa, este documento diz isso em
vez de desenhar um diagrama bonito que não bate com o código.

---

## 1. As duas invariantes — as únicas que valem, e valem porque são checáveis

O inventário da etapa 1 (`FASE_H_INVENTORY.md` §6.3) recomendou declarar as
camadas **por pacote-folha**, e afirmar só o que é verificável. São duas:

### Invariante A — nada importa `core/`, exceto a fachada raiz

```
$ grep -rn '"wa-api/internal/wa-noise/core"' --include="*.go" . | grep -v '^./internal/wa-noise/core/'
internal/wa-noise/main.go:39:   core "wa-api/internal/wa-noise/core"
internal/wa-noise/core/client_test.go:16:       "wa-api/internal/wa-noise/core"
```

Dois resultados, ambos legítimos: a fachada, e o teste externo
(`package core_test`) que vive dentro do próprio `core/`.

**Isto é travado por gate**, não por convenção: `scripts/waclient-facade-check.sh`
falha o build se qualquer `.go` **fora** de `internal/wa-noise/` importar `core`.
O racional do próprio script vale citar, porque é a lição da etapa 5:

> A etapa 5 é a prova: ao mover os 114 arquivos da raiz para `core/`, os 44
> consumidores foram mecanicamente repontados para `core` e ninguém percebeu que
> a árvore tinha perdido a fachada. Uma regra de arquitetura que não falha o
> build não é uma regra, é um comentário.

Os ~44 consumidores externos (`pkg/bootstrap/`, `pkg/infra/wa-noise/*`,
`pkg/infra/{history,media}/`) importam `wa-api/internal/wa-noise` — a fachada
`main.go`, `package wa-noise`. `Client` chega lá por **alias de tipo**
(`type Client = core.Client`), que preserva o method set inteiro, inclusive os
178 wrappers de `DangerousInternals`, em uma linha.

### Invariante B — as camadas de baixo nunca importam capacidade nem núcleo

```
$ grep -rn 'wa-api/internal/wa-noise/\(capabilities\|core\)' --include="*.go" \
    internal/wa-noise/protocol internal/wa-noise/security \
    internal/wa-noise/persistence internal/wa-noise/observability
(nenhum resultado)
```

`protocol/`, `security/`, `persistence/` e `observability/` **nunca** importam
`capabilities/` nem `core/`. Zero exceções no HEAD.

`runtime/` satisfaz a mesma propriedade e foi verificado na etapa 7:
`runtime/keepalive/` importa apenas `protocol/binary`, `protocol/types/events` e
`observability/log`; `runtime/proxy/` não importa **nada** de `wa-noise`. A
direção `core -> runtime` (e nunca o inverso) se mantém.

### Direção de dependência, como ela realmente é

```
                    consumidores externos (pkg/…)
                              │
                              ▼
                    internal/wa-noise  (main.go — fachada, package wa-noise)
                              │  alias de tipo
                              ▼
                          core/  ──────────────┐
                     (Client, socketLock,      │ implementa as 13 Transport
                      composition root)        │
                        │           │          │
              ┌─────────┘           └──────┐   │
              ▼                            ▼   │
       capabilities/*  ◄────────────── runtime/{keepalive,proxy}
              │                            │
              ▼                            ▼
    ┌──────────────────────────────────────────────┐
    │  protocol/   security/   persistence/        │
    │  observability/                              │
    └──────────────────────────────────────────────┘
              ▲ arestas entre folhas nos dois sentidos — ver §3
```

As setas para `capabilities/` a partir de `core/` são **de importação**; o
controle vai no sentido inverso pelas interfaces `Transport` (o consumidor é
quem declara a interface). Ver `ARCHITECTURE.md`.

---

## 2. Dependências entre capacidades — todas as arestas, verificadas

`capabilities/` não é uma camada plana. Estas são **todas** as arestas
capacidade→capacidade em código de produção no HEAD:

```
$ grep -rn 'wa-noise/capabilities/' --include="*.go" internal/wa-noise/capabilities | grep -v _test.go
```

| De | Para | Arquivo |
|---|---|---|
| `message` | `send` | `message/decrypt_loop.go`, `decrypt_session.go`, `poll.go`, `protocol.go` |
| `message` | `media` | `message/history_sync.go` |
| `message` | `user` | `message/parse.go` |
| `message` | `appstatesync` | `message/transport.go` |
| `retry` | `prekeys` | `retry/handle_parts.go`, `send.go`, `transport.go` |
| `send` | `group` | `send/fb_outbound.go`, `prepare.go`, `transport.go` |
| `send` | `retry` | `send/fb_outbound.go` |
| `send` | `tctoken` | `send/outbound.go` |
| `user` | `group` | `user/avatar.go` |

Isso dá a ordem topológica que a Fase F/G teve de respeitar para extrair uma
capacidade por vez sem quebrar o build entre lotes:

```
prekeys, tctoken, group, appstatesync, media, newsletter, notification, pairing
  → retry → user → send → message
```

`message` e `send` são os sumidouros: são as capacidades mais acopladas, e não
por acidente — enviar uma mensagem exige saber o grupo, o dispositivo, o token e
a política de retry.

### Como os call sites reversos foram resolvidos — fachada, não import direto

A Fase D mediu os pontos em que a raiz chamava de volta a lógica extraída e
previu um custo alto. A Fase F/G mediu de novo e o custo foi quase zero, por um
motivo específico: **fachada sobre função livre**. Os call sites continuam
chamando o mesmo método (exportado ou não) de `*Client`; o método virou uma
fachada fina que delega ao pacote.

**Grupo — F/G lote 6 (`PATCHES.md:7129-7146`).** Seis call sites reversos;
**quatro não mudaram uma letra**:

| Call site | Resolução |
|---|---|
| `send_prepare.go:176` `cli.getCachedGroupData(...)` | Fachada delega a `group.GetOrFetch`. Zero mudança. |
| `sendfb_transport.go:41,37` | Fachada + apelido `groupMetaCache = group.Meta`. Zero mudança. |
| `notification.go:143` `cli.parseGroupNotification(node)` | Fachada. Zero mudança (só deslocou de linha). |
| `disappearing_timer.go:70,72` `cli.sendGroupIQ(...)` | Fachada → `group.SendIQ`. Zero mudança. |
| `send_ack.go:96-98` — `cli.groupCacheLock.Lock(); delete(cli.groupCache, to); Unlock()` | **Único que precisou mudar de fato**: três linhas viraram `cli.groupCache.Delete(to)`. Mutex e mapa passaram a ser privados de `group.Cache`. |
| `user_avatar.go:65` `namespace = groupIQNamespace` | Virou `group.IQNamespace`. Exportar a constante foi preferido a duplicar o literal de wire `"w:g2"` — duas definições divergem em silêncio. |

**Usuário — F/G lote 7 (`PATCHES.md:7455-7469`).** Sete call sites; **os cinco
que a Fase D listou não mudaram nem de linha**:

| Call site | Resolução |
|---|---|
| `send_node_build.go:142`, `sendfb_transport.go:178` `cli.GetUserDevices(...)` | Fachada → `user.GetDevices`. Zero mudança. |
| `send_prepare.go:207` `cli.GetUserInfo(...)` | Fachada → `user.GetInfo`. Zero mudança. |
| `message_decrypt.go:39,42` `go cli.updateBusinessName/updatePushName(...)` | Fachadas não exportadas, mesma assinatura — inclusive o `go` na frente. |
| `message_history_sync.go:143` `cli.handleHistoricalPushNames(...)` | Fachada não exportada. Zero mudança. |
| `send_ack.go:100-102` — acesso direto ao mapa e ao lock | Virou `cli.userDevicesCache.Delete(to)`. |
| `notification_device.go` inteiro (~20 acessos diretos) | **O call site reverso mais pesado de toda a Fase F/G.** Reescrito para `cache.Lock()`/`GetLocked`/`SetLocked`/`DeleteLocked`. |

**A lição, repetida nos dois lotes:** a Fase D estava certa sobre *onde* estavam
os call sites e errada sobre o *custo*. Só os que tocavam **estado** ou
**constante** precisaram de edição. Chamada de método sobrevive à extração;
acesso direto a campo, não. É de onde vem a regra 6 do `CONTRIBUTING.md`.

**Envio e recepção — F/G lotes 8 e 9.** `send` e `message` importam outras
capacidades **diretamente** (tabela acima), não por fachada. A justificativa
está nos registros: nenhum dos dois introduziu concorrência nova
(`PATCHES.md:7951` e `:8274`), e o acoplamento é semântico e irredutível — não
existe "enviar mensagem" que não conheça grupo, dispositivo e retry. É o caso
concreto que a regra 7 do `CONTRIBUTING.md` cobre.

---

## 3. As duas arestas bidirecionais entre `protocol/` e `security/`

**Isto é uma característica aceita e entendida, não um bug a corrigir.**
Levantado na etapa 1 (`FASE_H_INVENTORY.md` §6.3) e confirmado no HEAD:

```
$ grep -rn 'wa-noise/security/' --include="*.go" internal/wa-noise/protocol | grep -v _test.go
protocol/socket/noisehandshake.go:20:   "wa-api/internal/wa-noise/security/gcm"
protocol/appstate/encode.go:14:         "wa-api/internal/wa-noise/security/cbc"
protocol/appstate/decode_mutation.go:23:"wa-api/internal/wa-noise/security/cbc"
protocol/appstate/keys.go:16:           "wa-api/internal/wa-noise/security/hkdf"
protocol/appstate/lthash/lthash.go:16:  "wa-api/internal/wa-noise/security/hkdf"

$ grep -rn 'wa-noise/protocol/' --include="*.go" internal/wa-noise/security | grep -v _test.go
security/handshake/handshake.go:16:     "wa-api/internal/wa-noise/protocol/proto/waWa6"
security/handshake/handshake.go:17:     "wa-api/internal/wa-noise/protocol/socket"
security/handshake/cert.go:17:          "wa-api/internal/wa-noise/protocol/proto/waCert"
security/paircrypto/signature.go:12:    "wa-api/internal/wa-noise/protocol/proto/waAdv"
```

As duas arestas nomeadas pelo inventário:

1. `protocol/socket → security/gcm` — o NoiseSocket cifra com AES-GCM.
2. `security/handshake → protocol/socket` — o handshake Noise produz o
   NoiseSocket.

**Por que isso compila:** Go proíbe ciclo entre **pacotes concretos**, e não há
nenhum. `socket` e `gcm` são pacotes distintos de `handshake`; o grafo de
pacotes é acíclico. O que existe é um ciclo entre os *diretórios-categoria*,
que não são pacotes Go — são agrupadores de sistema de arquivos.

**Por que não corrigimos:** a "correção" seria mover `gcm` para dentro de
`protocol/` ou `socket` para dentro de `security/`, e as duas alternativas são
piores. AES-GCM é primitiva criptográfica, não formato de wire; o websocket é
formato de wire, não criptografia. A classificação está certa; é a metáfora de
"camadas empilhadas" que está errada.

**Consequência prática:** não leia `security/` como "uma camada abaixo (ou
acima) de `protocol/`". Leia as duas invariantes da §1, que são por pacote-folha
e são as que realmente se sustentam.

### A terceira aresta não declarada: `protocol/appstate → persistence/store`

```
protocol/appstate/keys.go:15, decode_mutation.go:22, recovery.go:23
    "wa-api/internal/wa-noise/persistence/store"
```

A especificação original da Fase H não declarava `protocol → persistence`. A
etapa 1 (§6.1) encontrou a aresta e escolheu a saída (a): manter `appstate/` em
`protocol/` e **declarar a aresta explicitamente aqui**. O racional:
`appstate` depende de `store` só por **tipos de dado**
(`store.AppStateMutationMAC`, `store.Device`, `store.AppStateSyncKey`), não por
comportamento. A alternativa (b) — quebrar `appstate.WAPatchName` para
`protocol/types/` — era mudança de API pública, não de diretório, e ficou fora
de escopo.

---

## 4. `appstate` × `appstatesync` — dois nomes parecidos, categorias diferentes

Esta é a confusão mais provável de quem chega na árvore, e é o motivo de os dois
terem acabado em lugares diferentes:

| | `protocol/appstate/` | `capabilities/appstatesync/` |
|---|---|---|
| **O que é** | O **codec**: encode/decode de patches de app state, LTHash, MACs, mutações, recovery | O **sincronizador**: fetch, dispatch, key requests contra o servidor |
| **Fala com o servidor?** | Não. Biblioteca pura. | Sim. É a definição de capacidade. |
| **Tem `Transport`?** | Não | Sim — `appstatesync/transport.go:47`, 13 métodos |
| **Tem lock?** | `keyCacheLock` (`appstate/keys.go:22`), cache do `Processor` | `syncLock`, `keyRequestsLock` (`appstatesync/state.go:31,34`) |
| **Subpacote** | `protocol/appstate/lthash/` (hash homomórfico) | — |

A classificação não foi arbitrária: `protocol/types/events/appstate.go:12`
importa `appstate` para o tipo `appstate.WAPatchName`. Se `appstate` fosse
classificado como capacidade, teríamos `protocol/` importando `capabilities/` —
violação direta da invariante B. O teste "isto fala o protocolo ou define como
os dados são representados?" resolve o caso: `appstate` **define a
representação**, `appstatesync` **fala**.

---

## 5. Como reverificar tudo isto

```bash
# Invariante A (também travada por gate)
bash scripts/waclient-facade-check.sh

# Invariante B
grep -rn 'wa-api/internal/wa-noise/\(capabilities\|core\)' --include="*.go" \
  internal/wa-noise/{protocol,security,persistence,observability,runtime}

# Arestas entre capacidades
grep -rn 'wa-noise/capabilities/' --include="*.go" internal/wa-noise/capabilities | grep -v _test.go

# Arestas protocol <-> security
grep -rn 'wa-noise/security/'  --include="*.go" internal/wa-noise/protocol | grep -v _test.go
grep -rn 'wa-noise/protocol/'  --include="*.go" internal/wa-noise/security | grep -v _test.go

# Ciclo de verdade (o único que importa para o compilador)
go build ./...
```
