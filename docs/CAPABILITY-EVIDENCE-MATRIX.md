# Matriz de evidência por quadrante (Engine × AccountType)

Este documento cobre o item 71 do prompt arquitetural: uma tabela
Capability | NP | NB | HP | HB | Causa para as capabilities que já têm
caracterização real no `pkg/capabilityregistry/matrix.go` — isto é, onde o
status difere entre pelo menos duas das quatro combinações
(wa_noise×personal, wa_noise×business, wa_headless×personal,
wa_headless×business), ou onde a combinação constante já é, em si, um fato
que vale travar.

**Método**: a tabela abaixo NÃO foi copiada à mão de `matrix.go`. Foi gerada
chamando `CapabilityRegistry.Decide` programaticamente para as quatro
combinações, com um `main.go` descartável (não commitado — a fonte de
verdade permanece o código; o comando de regeneração está no rodapé). Os
mesmos oito pares (capability × quadrante) estão travados por teste de
regressão em `pkg/capabilityregistry/quadrant_test.go`
(`TestQuadrant_*`).

## Legenda

- **Status**: `supported` / `unknown` / `engine_unsupported` (ver
  `domain.CapabilityStatus`). `unknown` é diferente de `engine_unsupported`:
  o primeiro significa "nenhum adapter encontrado, mas não sabemos se o
  transporte poderia servir isto"; o segundo significa "confirmado que o
  motor não pode, com citação".
- **NP/NB/HP/HB**: wa_noise×personal, wa_noise×business,
  wa_headless×personal, wa_headless×business.
- Toda célula de personal/business tem `evidence=not_tested` neste
  documento — não porque a capability em si não tenha evidência, mas porque
  `matrix.expand()` **desconta** a evidência ao expandir de
  `account_type=unknown` para personal/business especificamente: a dimensão
  engine foi verificada, a dimensão account_type nunca foi. Isso é o
  ACHADO do item 4 abaixo, não um artefato do script.

## Tabela

| Capability | NP | NB | HP | HB | Causa |
|---|---|---|---|---|---|
| `send_carousel` | supported | supported | unknown | unknown | Engine gap **confirmado** no lado wa_noise (HOUSEKEEP F216, foto em dispositivo); wa_headless é ausência de adapter, não confirmação de impossibilidade. |
| `set_group_photo` | supported | supported | engine_unsupported | engine_unsupported | Engine gap **confirmado** dos dois lados: wa_headless não tem módulos de foto na página (H140, citado em `pkg/application/contracts/group_ports.go`). É a única linha da matriz com `engine_unsupported` + `EvidenceConfirmed` na dimensão engine. |
| `send_text` | supported | supported | unknown | unknown | Representa toda a família "mensagem nova": `pkg/infra/wa-headless/messenger` só cobre ações sobre mensagens **existentes** (mark read, reação, edição, revogação, voto de enquete) — criar mensagem não tem adapter ali. Ambíguo entre `not_implemented` e `engine_unsupported`; fica `unknown`. |
| `star_message` | supported | supported | unknown | unknown | wa_noise suportado via patch de app-state (`appstate.BuildStar`) — corrige uma afirmação desatualizada em `docs/CAPACIDADES.md` (auto-sinalizado obsoleto desde 2026-08-24). wa_headless sem adapter. |
| `mark_read` | supported | supported | supported | supported | **Mesmo status nos dois motores** — caso raro na matriz. wa_headless tem a nota "ack local apenas, recibo-ao-remetente NÃO confirmado" (H160, citado em `messenger.go`); `Supported=true` ainda vale porque a operação É servida, mesmo que nem todo efeito observável esteja confirmado. |
| `detect_account_type` | supported | supported | supported | supported | Mesmo status, **evidência diferente na dimensão engine** antes do desconto por account_type: wa_noise é `EvidenceConfirmed` (teste unitário com certificado usync real); wa_headless é `EvidenceProbable` (reaproveita getter medido ao vivo, não reconfirmado por esta worktree). Ver item "Interação" abaixo — esta é a capability cujo PROPÓSITO é justamente determinar o account_type, o que a torna o caso mais relevante para os itens 76-79 e, ainda assim, o que menos se aplica a eles. |
| `check_pairing_status` | supported | supported | unknown | unknown | wa_noise suportado; wa_headless tem nota explícita dizendo que `engine_unsupported` é **plausível** (autenticação headless é QR/cookie, não código de pareamento) mas **não confirmado** por leitura do fluxo de auth. Deliberadamente distinto de `set_group_photo`: mesma forma (N=sim, H=não), confiança diferente. |
| `update_group_participants` | supported | supported | supported | supported | Mesmo status nos dois motores, mas a nota do wa_headless registra uma assimetria (H58/H65: a escrita pode ter sucesso enquanto a sessão que agiu não consegue ler de volta) que a matriz **não** codifica como diferença de status — só como texto. |

## Interação Engine × AccountType (itens 76-79)

Os itens 76-79 pedem para classificar o padrão de quatro status em uma das
categorias (regra de Business, gap de implementação wa_noise, limitação
SPA/DOM, etc.) quando a matriz já tiver essa informação.

**Resultado**: nenhuma das oito capabilities acima permite essa
classificação hoje, porque **nenhuma delas tem NP≠NB ou HP≠HB** — toda
diferença observada é puramente Engine (N vs H), nunca AccountType
(P vs B). Isso não é uma omissão desta tabela: é o estado real do código.
`matrix.expand()` (pkg/capabilityregistry/matrix.go:100-129) sempre copia o
mesmo `status` de `account_type=unknown` para personal e para business —
por construção, `NP == NB` e `HP == HB` para as 88 capabilities inteiras, não
só para as oito aqui. Isso está travado como fato executável em
`TestQuadrant_AccountTypeNeverChangesStatusToday`
(`pkg/capabilityregistry/quadrant_test.go`).

Então, para os itens 76-79: **"não é possível classificar ainda — account_type
não é medido por evidência real"** é a resposta honesta para as 88
capabilities, não só para as oito caracterizadas aqui. A única
capability onde a dimensão account_type tem qualquer nuance é
`detect_account_type` — e mesmo ali, a nuance é de **evidência** (confirmed
vs probable), não de **status** (ambos supported). Nenhum padrão do tipo
"NP✅ NB❌ HP✅ HB❌" (regra de Business) ou "NP✅ NB✅ HP❌ HB❌" (limitação
SPA/DOM) existe hoje na matriz — implementar detecção de account_type real
(fora do escopo desta worktree) é pré-requisito para esses padrões
aparecerem.

## Como regenerar a tabela

```go
// main.go descartável, não commitado
package main

import (
    "fmt"
    "wa-api/pkg/capabilityregistry"
    "wa-api/pkg/domain"
)

func main() {
    r := capabilityregistry.NewCapabilityRegistry()
    // ... iterar capabilities × {NP,NB,HP,HB} chamando r.Decide(...)
}
```

Rodar com `go run <arquivo>.go` a partir da raiz do módulo.
