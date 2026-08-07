# Arquitetura de `internal/wa-noise/`

`internal/wa-noise/` é o fork ativo do `go.mau.fi/whatsmeow` (ADR-0002/0003/0004).
Este documento explica **por que** a árvore tem a forma que tem, depois da
reorganização da Fase H (7 etapas, sobre as 10 da Fase F/G).

Documentos irmãos: `CONTRIBUTING.md` (as regras de quem escreve),
`DEPENDENCIES.md` (as arestas reais), `LOCKS.md` (o mapa de concorrência),
`FASE_H_INVENTORY.md` (o levantamento factual que fundamentou tudo isto).

---

## 1. Por que **não** DDD / Clean Architecture

A pergunta apareceu e foi respondida com "não". Não há `domain/`,
`application/`, `infra/`, `presentation/` aqui, e não devem aparecer.

O motivo é que **isto não é uma aplicação de domínio de negócio — é um SDK de
protocolo**. As camadas do DDD servem para isolar regras de negócio de detalhes
de entrega e persistência, porque as regras de negócio mudam por decisão nossa e
os detalhes mudam por decisão de terceiros. Aqui é o inverso: não existe regra de
negócio nossa. Existe o protocolo do WhatsApp, que é integralmente decisão de
terceiros, e o nosso trabalho é falá-lo corretamente.

Consequências concretas de aplicar DDD aqui:

- **`domain/` ficaria vazio ou absurdo.** O que seria a "entidade" de domínio?
  `types.JID`? É um identificador de wire, não um agregado. As invariantes que
  importam neste código são "o nó binário tem o atributo `type`" e "o contador
  Noise incrementa a cada frame" — invariantes de **protocolo**, não de negócio.
- **`infra/` viraria um depósito.** Socket, criptografia, SQL, protobuf e proxy
  cairiam todos lá, apagando exatamente a distinção que mais importa quando se
  depura este código: a diferença entre "o handshake falhou" e "o SQL falhou".
- **A fronteira útil não é vertical, é por capacidade.** Quando algo quebra, a
  pergunta é "grupo ou mídia?", não "aplicação ou infraestrutura?".

Então a árvore é organizada por **papel técnico** (o que este código *é*:
protocolo, criptografia, persistência, runtime, observabilidade) e por
**capacidade** (que operação do WhatsApp este código implementa). É a
organização que responde às perguntas que este repositório realmente faz.

---

## 2. As categorias, e a pergunta de decisão de cada uma

Cada diretório de topo tem **uma** pergunta. Se a resposta é sim, é ali. Se duas
perguntas dão sim, você provavelmente tem dois arquivos, não um.

```
internal/wa-noise/
├── main.go            fachada (package whatsmeow) — a única porta de entrada
├── core/              Client, ciclo de conexão, socketLock, composition root
├── capabilities/      12 capacidades do protocolo
├── protocol/          binary, proto, types, argo, socket, msgpad, msgattrs, appstate
├── security/          cbc, gcm, hkdf, keys, paircrypto, handshake
├── persistence/       store, store/sqlstore
├── runtime/           keepalive, proxy
├── observability/     log
└── docs/              este diretório
```

### `core/` — o núcleo

**O que é:** a identidade do `Client`. A struct `Client`, o ciclo de vida da
conexão, o `socketLock`, o roteamento de requests/respostas, o despacho de
eventos — e, por composição, os adaptadores que implementam **todas** as portas
`Transport` das capacidades.

`core/` é o **composition root**: é o lugar onde as peças se conhecem. Cada
capacidade declara a fatia do cliente de que precisa; `core/` é quem entrega
essa fatia.

**O que `core/` explicitamente NÃO é:**

- **Não é uma camada de negócio.** Não existe "lógica de aplicação" aqui.
- **Não é um depósito de helpers genéricos.** Se uma função não toca o `Client`
  nem o seu estado, ela não pertence a `core/`. Funções puras foram para
  `protocol/msgpad/`, `protocol/msgattrs/`, `security/paircrypto/` exatamente por
  isso.

**Por que `core/` é um pacote só, e grande:** por causa do `socketLock`. São 6
`Lock()` e 8 `RLock()` espalhados por `client_connection.go`, `connectionevents.go`,
`client_events.go` e `request.go` (`LOCKS.md` §1). Quebrar isso em dois pacotes
Go exigiria exportar o mutex ou o socket através de uma fronteira de pacote — a
análise do F/G lote 10 (`PATCHES.md:8334+`) concluiu que é inviável, e a etapa 5
respeitou a conclusão: moveu os 114 arquivos de `.` para `./core` em um único
`git mv`, sem dividir nada.

### `capabilities/` — as operações do protocolo

**Pergunta de decisão: "isto representa uma capacidade concreta do protocolo
WhatsApp?"**

Doze pacotes: `appstatesync`, `group`, `media`, `message`, `newsletter`,
`notification`, `pairing`, `prekeys`, `retry`, `send`, `tctoken`, `user`.

O teste operacional é mais estreito do que parece — uma capacidade tipicamente
**fala com o servidor** e por isso precisa de uma `Transport`. Ela responde a
"o usuário/servidor pode *fazer* isso?": entrar num grupo, baixar uma mídia,
enviar uma mensagem, parear um dispositivo.

O contraexemplo que define a fronteira: `appstate` **não** é capacidade (é o
codec dos patches — não fala com ninguém) mas `appstatesync` **é** (é quem
busca, despacha e pede chaves). Ver `DEPENDENCIES.md` §4.

Um segundo contraexemplo: `msgpad` faz padding de payload de mensagem. Parece
"mensagem", mas padding é regra de **wire format**, não uma operação que alguém
executa. Foi para `protocol/msgpad/`.

### `protocol/` — a representação e o trânsito

**Pergunta de decisão: "isto define como os dados/a conexão são representados ou
trafegam no protocolo?"**

`binary` (codec do XML binário), `proto` (protobuf gerado), `types`
(+`types/events`), `argo` (codec das queries MEX), `socket` (FrameSocket +
NoiseSocket), `msgpad`, `msgattrs`, `appstate` (+`appstate/lthash`).

Note que `socket/` está aqui e não em `runtime/`: o websocket **é** o transporte
do protocolo, não um serviço de apoio ao processo.

### `security/` — a criptografia

**Pergunta de decisão: "isto é primitiva criptográfica ou o estabelecimento de
confiança?"**

`cbc`, `gcm`, `hkdf`, `keys` (KeyPair Curve25519), `paircrypto` (assinatura ADV
do pareamento), `handshake` (Noise_XX_25519_AESGCM_SHA256 + verificação da
cadeia de certificados do servidor).

### `persistence/` — o que sobrevive ao processo

**Pergunta de decisão: "isto guarda estado entre execuções?"**

`store` (interfaces + `Device` + cache de sessões Signal), `store/sqlstore` (a
implementação SQL), `store/sqlstore/upgrades` (migrações versionadas).

### `runtime/` — o que mantém o processo vivo

**Pergunta de decisão: "isto mantém o processo/conexão funcionando, sem fazer
parte do protocolo em si?"**

`keepalive` (o ping periódico do websocket) e `proxy` (montagem dos
`http.Transport` a partir de configuração de proxy).

É a categoria mais fácil de confundir com `protocol/`. O corte: o keepalive
**usa** o protocolo (manda um `<iq type="get" xmlns="w:p">`) mas não o
**define** — trocar o intervalo de ping não muda o wire format. Nenhum dos dois
guarda estado concorrente próprio (`LOCKS.md` §2), o que é sintoma da categoria:
código de runtime coordena, não possui.

### `observability/` — como o sistema conta o que está fazendo

**Pergunta de decisão: "isto serve para observar o sistema, e não para
executá-lo?"**

`log` (o `waLog.Logger`). É o único pacote importado por praticamente toda
categoria, e é aceitável precisamente porque não tem comportamento próprio.

---

## 3. O padrão `Transport`: o consumidor é dono da interface

Este é o padrão central do fork, herdado das 10 etapas da Fase F/G e preservado
integralmente pela Fase H.

**A regra:** cada capacidade declara, **no seu próprio pacote**, uma interface
estreita com exatamente os métodos do cliente de que ela precisa. `core.Client`
implementa todas, por adaptadores, com asserção em tempo de compilação.

As interfaces concretas hoje
(`grep -rn "type Transport interface" internal/wa-noise/capabilities/*/`):

| Interface | Arquivo | Métodos |
|---|---|---|
| `notification.Transport` | `capabilities/notification/transport.go:27` | 2 |
| `media.Transport` | `capabilities/media/transport.go:44` | 3 |
| `prekeys.Transport` | `capabilities/prekeys/transport.go:47` | 4 |
| `tctoken.Transport` | `capabilities/tctoken/transport.go:34` | 5 |
| `keepalive.Transport` | `runtime/keepalive/transport.go:40` | 8 |
| `group.Transport` | `capabilities/group/transport.go:69` | 9 |
| `newsletter.Transport` | `capabilities/newsletter/transport.go:45` | 10 |
| `user.Transport` | `capabilities/user/transport.go:71` | 10 |
| `appstatesync.Transport` | `capabilities/appstatesync/transport.go:47` | 13 |
| `pairing.Transport` | `capabilities/pairing/transport.go:42` | 14 |
| `retry.Transport` | `capabilities/retry/transport.go:37` | 21 |
| `send.Transport` | `capabilities/send/transport.go:97` | 33 |
| `message.Transport` | `capabilities/message/transport.go:111` | 36 |

(+ `media.HTTPTransport`, `capabilities/media/transport.go:24`, que separa o
lado HTTP do lado protocolo da mídia.)

Repare na distribuição: de 2 a 36 métodos. **Não existe uma `Transport`
compartilhada**, e a diferença de tamanho é a prova de que a regra está sendo
seguida — `notification` precisa de `Log()` e `DispatchEvent()`, e é só isso que
ela vê do cliente.

O lado do `core/` são 15 asserções, uma por adaptador:

```go
core/notification_transport.go:25   var _ notification.Transport = notifTransport{}
core/media_transport.go:29          var _ media.Transport        = mediaTransport{}
core/send_adapter.go:36             var _ send.Transport         = sendTransport{}
core/keepalive.go:44                var _ keepalive.Transport    = keepAliveTransport{}
…
```

**Por que este padrão e não uma interface compartilhada:** o doc comment de
`capabilities/notification/transport.go` diz a razão real, e ela é sobre testes:

> Deliberadamente não expõe nada do `*whatsmeow.Client` além disso: é o que
> permite que este pacote não importe o pacote raiz e que os testes usem um
> dublê em vez de um cliente com socket e sessão Noise.

Uma `Transport` gigante compartilhada quebraria as duas propriedades de uma vez:
todo dublê de teste teria de implementar 100 métodos, e qualquer método novo
recompilaria todas as capacidades.

**Como o controle flui:** `core/` importa `capabilities/`, mas a *definição* do
contrato mora na capacidade. A dependência de compilação e a dependência de
design apontam em sentidos opostos — é inversão de dependência sem container,
sem registro e sem `Service`.

---

## 4. Direção de dependência — como ela realmente é

O diagrama abaixo é **verificado**, não aspiracional. As arestas foram
conferidas por `grep` no HEAD; os comandos estão em `DEPENDENCIES.md` §5.

```
   consumidores externos (pkg/bootstrap, pkg/infra/wa-noise/*, pkg/infra/{history,media})
                              │  importam SÓ isto
                              ▼
                    internal/wa-noise  —  main.go, package whatsmeow (fachada)
                              │  type Client = core.Client  (alias de tipo)
                              ▼
                            core/
                    Client · socketLock · composition root
                              │
                importa       │       implementa as 13 Transport
          ┌───────────────────┼───────────────────┐
          ▼                                       ▼
    capabilities/                          runtime/keepalive
    appstatesync group media message       runtime/proxy
    newsletter notification pairing
    prekeys retry send tctoken user
          │  (9 arestas internas — ver DEPENDENCIES.md §2)
          ▼
    ┌─────────────────────────────────────────────────────┐
    │  protocol/    security/    persistence/             │
    │  observability/                                     │
    │                                                     │
    │   protocol/socket ──────────► security/gcm          │
    │   protocol/appstate ────────► security/{cbc,hkdf}   │
    │   security/handshake ───────► protocol/socket       │
    │   security/paircrypto ──────► protocol/proto/waAdv  │
    │   protocol/appstate ────────► persistence/store     │
    └─────────────────────────────────────────────────────┘
```

**Sobre as setas dentro da caixa de baixo — a parte honesta.** `protocol/` e
`security/` têm arestas **nos dois sentidos**. Isto foi encontrado pela etapa 1
(`FASE_H_INVENTORY.md` §6.3) e confirmado no HEAD:
`protocol/socket → security/gcm` e `security/handshake → protocol/socket`.

Isso **compila** porque Go proíbe ciclo entre pacotes concretos, e não há
nenhum: `socket`, `gcm` e `handshake` são três pacotes distintos, e o grafo de
pacotes é acíclico. O ciclo é entre os *diretórios-categoria*, que não são
pacotes Go — são agrupadores de sistema de arquivos.

Mas isso **invalida** a leitura "`security/` é uma camada abaixo (ou acima) de
`protocol/`". Não é. AES-GCM é primitiva criptográfica; o websocket é formato de
wire; o handshake produz o socket cifrado. As três classificações estão certas —
é a metáfora do bolo de camadas que está errada para este código.

Por isso a arquitetura afirma só **duas** invariantes, escolhidas por serem as
que se sustentam e são checáveis por comando:

1. **Nada importa `core/`** exceto a fachada `main.go`. Travado por
   `scripts/waclient-facade-check.sh`.
2. **`protocol/`, `security/`, `persistence/`, `observability/` nunca importam
   `capabilities/` nem `core/`.** Zero exceções no HEAD. `runtime/` também
   satisfaz — verificado na etapa 7.

Detalhes e os comandos de reverificação em `DEPENDENCIES.md`.

---

## 5. A fachada raiz

`main.go` (`package whatsmeow`) é a **única** porta de entrada do fork. Ela é
fina de propósito e **não** reexporta os ~732 símbolos de topo um a um:

```go
type Client = core.Client        // alias de TIPO, não tipo novo
var NewClient = core.NewClient
```

`type Client = core.Client` é um **alias de tipo**, então o method set inteiro de
`*Client` — inclusive os 178 wrappers de `DangerousInternals` — vem junto em uma
linha. O que sobra na fachada são os poucos tipos, constantes, erros e funções
que os consumidores nomeiam explicitamente.

A regra de escrita está no topo do arquivo: **`main.go` só contém aliases e
delegação.** Nada que precise de corpo mora ali.

E a regra é travada, não sugerida. O racional de
`scripts/waclient-facade-check.sh` é a lição da etapa 5:

> A etapa 5 é a prova: ao mover os 114 arquivos da raiz para `core/`, os 44
> consumidores foram mecanicamente repontados para `core` e ninguém percebeu que
> a árvore tinha perdido a fachada. Uma regra de arquitetura que não falha o
> build não é uma regra, é um comentário.

---

## 6. Código gerado: `proto/` e `internals.go` — não reestruture

Quatro alvos são **gerados** e estão fora do trabalho de arquitetura:

| Alvo | Gerador | Evidência |
|---|---|---|
| `protocol/proto/` (+69 subpacotes) | `protoc-gen-go` | `// Code generated by protoc-gen-go. DO NOT EDIT.` |
| `protocol/binary/proto/` | `generatelegacy.sh` | `DO NOT MODIFY: Generated by generatelegacy.sh` |
| `core/internals.go` (757 linhas) | `core/internals_generate.go` | 178 wrappers de `DangerousInternals` |
| `core/internals_generate.go` | (o próprio gerador) | lista de arquivos **hardcoded** |

A única edição permitida nesses arquivos é reescrita de import path em
movimentação de diretório. Dividi-los não tem valor: o gerador os recria. É o
mesmo racional que o ADR-0004 já aplicava a `proto/`, e é por isso que os dois
`proto/` também ficam fora do gate de tamanho de arquivo.

### O bug F29 — conhecido, e deliberadamente não corrigido

Não vale fingir que `internals.go` está saudável. `HOUSEKEEP.md:908` registra:

> **F29 — `internals_generate.go` tem lista de arquivos hardcoded: `go generate`
> hoje derruba 96 dos 178 wrappers de `DangerousInternals`**

O `main()` do gerador não escaneia o diretório; ele carrega uma lista literal de
32 nomes de arquivo (`internals_generate.go:101-110`) que são os nomes do
**upstream**, de antes da Fase A. A Fase A dividiu a raiz em 94 arquivos com
nomes novos que nunca entraram na lista. Resultado, nas palavras do registro:

> Rodar `go generate` **agora** regenera um `internals.go` menor e derruba 96
> wrappers em silêncio — sem erro de compilação, porque `DangerousInternals` não
> tem consumidor no repo.

E há um adendo (`HOUSEKEEP.md:971`): a Fase D **editou `internals.go` à mão** para
referenciar `msgattrs.MessageAttrs`, e o gerador monta o bloco de import copiando
apenas os imports do *primeiro* arquivo da lista (`internals_generate.go:123`).
Ou seja, trocar a lista literal por varredura de diretório **não basta mais**.

**Consequências para quem mexe na árvore:**

- Não rode `go generate` sobre `internals.go` esperando resultado correto.
- A Fase H **não piorou** o F29. `internals.go` e `internals_generate.go` foram
  movidos para `core/` sem uma única edição, e os nomes da lista continuam
  resolvendo relativo ao diretório do gerador — a lista não ficou mais quebrada
  do que já estava. Está registrado na etapa 5 (`PATCHES.md`, "Impacto no bug
  F29") e replicado como adendo no bloco F29 do `HOUSEKEEP.md`.
- Não corrija o F29 "de graça" no meio de outro trabalho. Ele é escopo próprio,
  toca arquivo gerado e tem duas dependências acopladas. Pergunte antes.

Isto é aplicação direta da regra do projeto: achado fora de escopo vai para
`HOUSEKEEP.md`, não para o diff.

---

## 7. Onde continuar

- Vai **escrever código**? `CONTRIBUTING.md` — as 11 regras.
- Vai **mexer em lock ou goroutine**? `LOCKS.md` — inclusive o placar de quais
  locks tiveram revisão independente e quais não tiveram.
- Vai **adicionar um import entre pacotes**? `DEPENDENCIES.md` — as arestas que
  existem e a ordem topológica que elas impõem.
- Quer **o histórico**? `PATCHES.md` (Fase A→H) e `FASE_H_INVENTORY.md` (o
  levantamento que decidiu a árvore).
