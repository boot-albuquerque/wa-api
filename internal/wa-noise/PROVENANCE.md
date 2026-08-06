# Proveniência de internal/wa-noise/

Vendorizado a partir de `go.mau.fi/whatsmeow v0.0.0-20260516102357-8d3700152a69` (MPL-2.0, © Tulir Asokan e
contribuidores) em 2026-08-06 via `scripts/waclient-vendor.sh`.

Decisão registrada em:
- [ADR-0002](../../docs/adr/0002-vendorizar-whatsmeow-em-vez-de-reimplementar.md)
- [ADR-0003](../../docs/adr/0003-vendorizar-modulo-inteiro-sem-selecao.md)

## Escopo

Módulo inteiro copiado (raiz + `appstate` + `argo` + `binary` + `proto`
+ `socket` + `store` + `types` + `util`), preservando a API pública do
`Client`. `go.mau.fi/libsignal` **não** é vendorizado — módulo separado,
sem import reverso, permanece dependência externa normal do `go.mod`.

## Licença (MPL-2.0)

O texto integral da licença original está em `LICENSE-whatsmeow`. Todo
arquivo `.go` sob este diretório (exceto `proto/`, gerado e marcado
`linguist-generated` via `.gitattributes`) mantém o header MPL-2.0
original — verificado automaticamente via `make waclient-license-check`.

A troca do import path conta como "Modification" pela definição da própria
MPL-2.0 (§1.10) — não há distinção entre "arquivo só com import trocado" e
"arquivo customizado" para fins de header de licença; todos mantêm o aviso.

**§3.2 (disponibilidade de Source Form ao distribuir Executable Form)**:
decisão registrada em 2026-08-06 — o `wa-api` roda em infraestrutura
interna, a imagem Docker não é distribuída a terceiros. A obrigação do
§3.2 não é acionada neste cenário. Se isso mudar (imagem distribuída a
clientes/terceiros, ou o repo virar produto distribuído), esta decisão
precisa ser revisitada.

## Customização

Qualquer patch que precise alterar lógica inline dos arquivos vendorizados
(em vez de viver num arquivo novo `wa-api_*.go` por pacote) deve ser
registrado em `PATCHES.md`. `make waclient-drift` detecta edição não
registrada.
