# Capability Registry — o que é, e o que não é

Este documento explica o núcleo do Capability Registry (`pkg/capabilityregistry`
e `pkg/domain/capability*.go`), a infraestrutura que resolve — sem executar —
se uma capacidade de negócio é servida por um engine e tipo de conta.

## Os quatro conceitos

- **Engine** (`domain.Engine`, já existia antes desta worktree): o transporte
  que serve a sessão — `noise` (socket) ou `headless` (SPA).
- **AccountType** (`domain.AccountType`, novo aqui): `personal`, `business`,
  `unknown`. **Não há lógica de detecção** — é um enum simples que um chamador
  futuro preenche quando souber. `unknown` é estado legítimo, não erro.
- **Capability** (`domain.Capability`, novo aqui): um identificador estável de
  OPERAÇÃO DE NEGÓCIO (`send_text`, `create_group`, `block_contact`-equivalente
  `update_blocklist`), diferente do nome do port/endpoint que a serve. Hoje é
  1:1 com métodos de port — nenhum caso medido de dois endpoints colapsarem
  numa capability só.
- **CapabilityDecision**: o resultado de perguntar "esta capability, neste
  engine, para este tipo de conta — dá?". Traz `Supported` (bool),
  `Status` (por quê), `Evidence` (quão confiável é a resposta), e os campos de
  precondição.

## Static vs runtime precondition

O registry só sabe o que está na matriz ESTÁTICA (engine × account_type) —
nunca inspeciona sessão real. Uma precondição RUNTIME (ex: "é admin do
grupo?") é responsabilidade de quem chama: `CapabilityRegistry.Decide` aceita
`failedPreconditions` opcionais e, se algum vier preenchido, rebaixa uma
decisão que seria `supported` para `permission_required` — mas nunca inverte o
sentido contrário (uma precondição satisfeita não torna suportado algo que o
engine/tipo de conta já recusam).

## Taxonomia de status (`domain.CapabilityStatus`)

| status | significa | quem conserta |
|---|---|---|
| `supported` | verificado, funciona | — |
| `partially_supported` | algumas variantes funcionam, outras não | quem mediu decide o que falta |
| `not_implemented` | wa-api nunca escreveu o port/rota, nada bloqueia | esta worktree/próxima |
| `engine_unsupported` | o transporte (socket ou SPA) não tem como fazer isto | quem trabalha naquele transporte |
| `upstream_unsupported` | o próprio protocolo WhatsApp Web não oferece isto | ninguém — é teto, não bug |
| `account_type_unsupported` | WhatsApp restringe a outro tipo de conta (ex: catálogo exige WABA) | ninguém, dentro destes engines |
| `account_state_unsupported` | precondição de ESTADO da conta falhou agora (não verificada, limitada) | é runtime, não estático |
| `permission_required` | falta papel/permissão na sessão | quem chama, ao re-tentar com permissão |
| `broken` | ligado ponta a ponta mas medido como incorreto | quem escreveu, com teste que trave a regressão |
| `unknown` | ninguém verificou | próxima worktree de medição de campo |

`not_implemented` ≠ `engine_unsupported` ≠ `upstream_unsupported`: o primeiro é
dívida nossa, o segundo é dívida do transporte, o terceiro é chão do
protocolo. Confundi-los faria alguém tentar "consertar" o que não se conserta,
ou desistir do que era barato.

## Evidence status — separado de status (item 72)

Duas células podem ter o mesmo `Status` e discordar em `Evidence`:

- `confirmed`: medição citável (entrada de HOUSEKEEP.md, foto de campo).
- `probable`: o adaptador foi lido e tem lógica real (não stub), mas ninguém
  verificou contra um dispositivo/conta real.
- `not_tested`: simplificação deliberada — a dimensão account_type nunca foi
  exercitada porque nada no código diferencia tipos de conta ainda.
- `unknown`: ninguém olhou.

## O que esta worktree fez e não fez

**Fez**: infraestrutura completa (`CapabilityProvider`, `CapabilityRegistry`,
taxonomia, `CapabilityDecision`, matriz) e populou toda a matriz por LEITURA
de código — grep pelo nome do método do port sob `pkg/infra/noise/...` e
`pkg/infra/headless/...`, confirmando que o hit tem lógica real (não um
stub) antes de marcar `supported`.

**Não fez**: verificação de campo da matriz inteira (isso é o trabalho de
`capability-tests-noise`/`capability-tests-headless`, worktrees futuras) e
detecção de `account_type` (depende de dado real, fora de escopo aqui).

## O gate de cobertura (item 83)

`pkg/capabilityregistry/coverage_gate_test.go` faz parsing AST de
`pkg/application/contracts`, enumera toda interface que EMBUTE
`SessionGuard` (o critério mecânico para "operação dependente de provider" —
um port de infraestrutura como `S3ConfigStore` ou `Logger` não embute
`SessionGuard` e fica, corretamente, fora do escopo deste registry) e exige
que cada método próprio dela tenha uma entrada em `PortCoverage` ou em
`EngineAgnosticPorts`. Interfaces compostas que só reexportam métodos
embutidos (`GroupSettings`, `ChatOperations`, `PresenceController`) não
contribuem método novo e ficam de fora por construção — o método já é exigido
através de quem elas compõem.

O controle negativo é EXECUTADO, não simulado: o teste injeta, num diretório
temporário, uma cópia mínima do `SessionGuard` mais uma interface fake com um
método sem mapeamento, roda o MESMO scanner AST, e confirma que ele reporta o
método como não coberto. Este mesmo experimento foi repetido manualmente
contra o `pkg/application/contracts` real durante o desenvolvimento (acrescentar
`TempFakePortForGateSmokeTest`, confirmar falha, remover, confirmar
sucesso) — ver relatório da sessão.

## Failure origin — de onde vem uma recusa

Uma `CapabilityDecision` não suportada tem origem em exatamente um destes
lugares, e `Status` diz qual:

1. Matriz estática (engine × account_type) — a maioria das entradas hoje.
2. Precondição runtime falhada, passada pelo chamador — `permission_required`.
3. Ausência total de entrada na matriz — `unknown`, nunca mascarado como
   `supported` (ver `TestDecide_NeverReturnsSupportedForAnUnknownCell`).

## O que NÃO é este registry

- Não substitui os ~40 application ports tipados — a execução real continua
  por eles.
- Não tem `Execute(capability, map[string]any)` genérico — destruiria type
  safety (ADR-001).
- Não faz fallback automático entre engines (proibido, item 53) — uma
  decisão `unsupported` num engine nunca consulta o outro por conta própria.
- Não é o lugar para regra de negócio nova — só resolução e diagnóstico.
