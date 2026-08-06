#!/usr/bin/env bash
# Vendoriza o módulo go.mau.fi/whatsmeow inteiro para internal/wa-noise/,
# preservando a API pública, trocando só o import path.
#
# Uso: scripts/waclient-vendor.sh <versão>
#   ex: scripts/waclient-vendor.sh v0.0.0-20260516102357-8d3700152a69
#
# Ver docs/adr/0002-*.md, docs/adr/0003-*.md e
# .omc/plans/vendor-whatsmeow-native-fork.md para o racional completo.
set -euo pipefail

VERSION="${1:?uso: scripts/waclient-vendor.sh <versão>}"
MODULE="go.mau.fi/whatsmeow"
DEST="internal/wa-noise"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$REPO_ROOT"

echo "==> Baixando ${MODULE}@${VERSION}"
GOFLAGS=-mod=mod go mod download "${MODULE}@${VERSION}"
SRC="$(go env GOMODCACHE)/${MODULE//\//\!}@${VERSION}"
# go module cache paths lower-case uppercase letters as !<lower>; whatsmeow
# não tem maiúsculas no path, então isso é um no-op aqui, mas mantemos a
# forma genérica caso a versão mude.
if [ ! -d "$SRC" ]; then
  SRC="$(go env GOMODCACHE)/${MODULE}@${VERSION}"
fi
if [ ! -d "$SRC" ]; then
  echo "ERRO: não encontrei o módulo baixado em $SRC" >&2
  exit 1
fi
echo "==> Fonte: $SRC"

echo "==> Limpando destino anterior (se houver): $DEST"
if [ -d "$DEST" ]; then
  chmod -R u+w "$DEST"
  rm -rf "$DEST"
fi
mkdir -p "$DEST"

echo "==> Copiando módulo inteiro, exceto metadados/README/LICENSE/editorconfig"
# go.mod/go.sum não fazem sentido dentro de um pacote interno (o wa-api já
# tem os seus); .github/.pre-commit-config.yaml/.editorconfig são infra do
# repo upstream, não do código; README.md upstream traria instruções de
# instalação do módulo externo, confuso dentro do fork.
rsync -a \
  --exclude='.git' \
  --exclude='.github' \
  --exclude='go.mod' \
  --exclude='go.sum' \
  --exclude='.pre-commit-config.yaml' \
  --exclude='.editorconfig' \
  --exclude='README.md' \
  "$SRC/" "$DEST/"

echo "==> Liberando permissão de escrita (módulo em cache Go é read-only)"
chmod -R u+w "$DEST"

echo "==> Renomeando LICENSE -> LICENSE-whatsmeow (preserva o texto MPL-2.0 original)"
if [ -f "$DEST/LICENSE" ]; then
  mv "$DEST/LICENSE" "$DEST/LICENSE-whatsmeow"
fi

echo "==> Reescrevendo import path ${MODULE} -> wa-api/${DEST} em todos os .go"
# CUIDADO: os .pb.go gerados (proto/) embutem o go_package original como
# STRING SERIALIZADA dentro do file descriptor (rawDesc), com um prefixo de
# tamanho (varint) que precede a string. Fazer replace de texto ali muda o
# comprimento da string sem ajustar o prefixo, corrompendo o parse do
# descriptor em runtime (panic "slice bounds out of range" no init do
# protobuf). O byte de tag de algumas entradas do descriptor (0x22) também
# aparece escapado como `\"` no texto Go, o que engana até um regex que só
# procura aspas ao redor do path — por isso a checagem abaixo exige que a
# LINHA INTEIRA (ignorando espaço em branco) seja só o import (com alias
# opcional), nunca um match parcial no meio de uma linha com mais conteúdo.
# Isso é seguro: nenhuma linha de rawDesc é só o path entre aspas — sempre
# tem escapes/outros campos misturados na mesma linha.
grep -rl "$MODULE" "$DEST" --include="*.go" | while read -r f; do
  perl -pi -e "s{^(\s*(?:[A-Za-z_][A-Za-z0-9_]*\s+)?)\"\Q${MODULE}\E((?:/[A-Za-z0-9_.-]+)*)\"(\s*)\$}{\$1\"wa-api/${DEST}\$2\"\$3}" "$f"
done

echo "==> Reescrevendo diretivas //go:generate (internals.go / internals_generate.go)"
# Essas linhas não são strings Go entre aspas (são argumento de linha de
# comando dentro de um comentário //go:generate), então o passo acima não
# as cobre — tratamento separado, seguro porque são comentários de texto
# puro, não dado binário.
grep -rl "^//go:generate.*${MODULE}" "$DEST" --include="*.go" | while read -r f; do
  perl -pi -e "s{(^//go:generate.*)\Q${MODULE}\E}{\$1wa-api/${DEST}}g" "$f"
done

echo "==> Corrigindo referência textual em comentário (binary/proto/doc.go)"
# Não é import nem dado de descriptor — é um comentário de prosa apontando
# o path antigo. Ajuste cosmético, mas mantém consistência com o resto.
DOCGO="$DEST/binary/proto/doc.go"
if [ -f "$DOCGO" ]; then
  perl -pi -e "s{\Q${MODULE}\E/proto/wa\* packages}{wa-api/${DEST}/proto/wa* packages}" "$DOCGO"
fi

echo "==> Marcando arquivos gerados (.gitattributes) para reduzir ruído de review"
if [ -f "$SRC/.gitattributes" ]; then
  cp "$SRC/.gitattributes" "$DEST/.gitattributes"
fi

echo "==> Gravando proveniência"
mkdir -p "$DEST"
cat > "$DEST/UPSTREAM" <<EOF
${MODULE} ${VERSION}
EOF

cat > "$DEST/PROVENANCE.md" <<EOF
# Proveniência de internal/wa-noise/

Vendorizado a partir de \`${MODULE} ${VERSION}\` (MPL-2.0, © Tulir Asokan e
contribuidores) em $(date -u +%Y-%m-%d) via \`scripts/waclient-vendor.sh\`.

Decisão registrada em:
- [ADR-0002](../../docs/adr/0002-vendorizar-whatsmeow-em-vez-de-reimplementar.md)
- [ADR-0003](../../docs/adr/0003-vendorizar-modulo-inteiro-sem-selecao.md)

## Escopo

Módulo inteiro copiado (raiz + \`appstate\` + \`argo\` + \`binary\` + \`proto\`
+ \`socket\` + \`store\` + \`types\` + \`util\`), preservando a API pública do
\`Client\`. \`go.mau.fi/libsignal\` **não** é vendorizado — módulo separado,
sem import reverso, permanece dependência externa normal do \`go.mod\`.

## Licença (MPL-2.0)

O texto integral da licença original está em \`LICENSE-whatsmeow\`. Todo
arquivo \`.go\` sob este diretório (exceto \`proto/\`, gerado e marcado
\`linguist-generated\` via \`.gitattributes\`) mantém o header MPL-2.0
original — verificado automaticamente via \`make waclient-license-check\`.

A troca do import path conta como "Modification" pela definição da própria
MPL-2.0 (§1.10) — não há distinção entre "arquivo só com import trocado" e
"arquivo customizado" para fins de header de licença; todos mantêm o aviso.

**§3.2 (disponibilidade de Source Form ao distribuir Executable Form)**:
decisão registrada em 2026-08-06 — o \`wa-api\` roda em infraestrutura
interna, a imagem Docker não é distribuída a terceiros. A obrigação do
§3.2 não é acionada neste cenário. Se isso mudar (imagem distribuída a
clientes/terceiros, ou o repo virar produto distribuído), esta decisão
precisa ser revisitada.

## Customização

Qualquer patch que precise alterar lógica inline dos arquivos vendorizados
(em vez de viver num arquivo novo \`wa-api_*.go\` por pacote) deve ser
registrado em \`PATCHES.md\`. \`make waclient-drift\` detecta edição não
registrada.
EOF

touch "$DEST/PATCHES.md"
if [ ! -s "$DEST/PATCHES.md" ]; then
  cat > "$DEST/PATCHES.md" <<'EOF'
# Patches locais em internal/wa-noise/

Lista de edições inline nos arquivos vendorizados que não puderam ser
expressas como arquivo novo (`wa-api_*.go`) por pacote. Cada item:
arquivo:linha tocado, motivo, data.

`make waclient-drift` falha se houver diff contra o upstream declarado em
`UPSTREAM` além do que está registrado aqui.

(nenhum patch registrado ainda — vendorização inicial é cópia fiel)
EOF
fi

echo "==> gofmt"
gofmt -w "$DEST"

echo "==> Concluído. Revise o diff antes de commitar."
