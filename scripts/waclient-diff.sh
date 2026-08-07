#!/usr/bin/env bash
# Compara internal/wa-noise/proto/ contra uma versão do go.mau.fi/whatsmeow
# upstream, normalizando o import path antes do diff (sem essa
# normalização, TODO arquivo apareceria como modificado só pela troca de
# path, tornando o diff inútil).
#
# ADR-0004 (2026-08-06): a partir desta ADR, internal/wa-noise/ deixou de
# ser um espelho drift-zero do upstream inteiro — só internal/wa-noise/proto/
# (código GERADO a partir de .proto, nunca editado à mão) continua sob essa
# trava. O restante do módulo (raiz, appstate/, argo/, binary/, socket/,
# store/, types/, util/) é agora um fork ativamente mantido, com
# modificações esperadas e registradas em internal/wa-noise/PATCHES.md — não
# faz sentido compará-lo contra upstream byte-a-byte.
#
# Uso: scripts/waclient-diff.sh <versão>
#   ex: scripts/waclient-diff.sh v0.0.0-20260516102357-8d3700152a69
#
# Saída vazia (exit 0) = internal/wa-noise/proto/ é cópia fiel do upstream
# gerado pela versão indicada.
# Saída não-vazia = proto/ divergiu do gerador upstream — isso é sempre bug,
# nunca patch intencional (código gerado não se edita à mão).
set -euo pipefail

VERSION="${1:?uso: scripts/waclient-diff.sh <versão>}"
MODULE="go.mau.fi/whatsmeow"
DEST="internal/wa-noise"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

if [ ! -d "$DEST" ]; then
  echo "ERRO: $DEST não existe — rode scripts/waclient-vendor.sh primeiro." >&2
  exit 1
fi

TMPDIR="$(mktemp -d)"
trap 'chmod -R u+w "$TMPDIR" 2>/dev/null; rm -rf "$TMPDIR"' EXIT

echo "==> Baixando ${MODULE}@${VERSION} para comparação" >&2
GOFLAGS=-mod=mod go mod download "${MODULE}@${VERSION}" >&2
SRC="$(go env GOMODCACHE)/${MODULE}@${VERSION}"
if [ ! -d "$SRC" ]; then
  echo "ERRO: não encontrei o módulo baixado em $SRC" >&2
  exit 1
fi

NORM="$TMPDIR/normalized"
mkdir -p "$NORM"
rsync -a \
  --exclude='.git' \
  --exclude='.github' \
  --exclude='go.mod' \
  --exclude='go.sum' \
  --exclude='.pre-commit-config.yaml' \
  --exclude='.editorconfig' \
  --exclude='README.md' \
  "$SRC/" "$NORM/"
chmod -R u+w "$NORM"
if [ -f "$NORM/LICENSE" ]; then
  mv "$NORM/LICENSE" "$NORM/LICENSE-whatsmeow"
fi

# Mesma regra de substituição do waclient-vendor.sh: só linhas que SÃO
# inteiramente um import Go entre aspas (com alias opcional) — nunca texto
# embutido em outro literal (ex: bytes de descriptor protobuf, onde um
# replace ingênuo corrompe o parse em runtime).
grep -rl "$MODULE" "$NORM" --include="*.go" | while read -r f; do
  perl -pi -e "s{^(\s*(?:[A-Za-z_][A-Za-z0-9_]*\s+)?)\"\Q${MODULE}\E((?:/[A-Za-z0-9_.-]+)*)\"(\s*)\$}{\$1\"wa-api/${DEST}\$2\"\$3}" "$f"
done
grep -rl "^//go:generate.*${MODULE}" "$NORM" --include="*.go" | while read -r f; do
  perl -pi -e "s{(^//go:generate.*)\Q${MODULE}\E}{\$1wa-api/${DEST}}g" "$f"
done

# Fase H (etapa 3): proto/ passou a viver em ${DEST}/protocol/proto/. O
# upstream normalizado acima ficou com "wa-api/${DEST}/proto/...", entao
# precisa de um segundo passo para casar com o layout atual do fork.
# MESMA regra ancorada de linha-inteira dos passos acima — nunca um match
# parcial no meio de linha, porque os .pb.go embutem o go_package original
# como string serializada no rawDesc do file descriptor e um replace de
# texto ali corromperia o varint de comprimento que precede a string.
grep -rl "wa-api/${DEST}/proto" "$NORM" --include="*.go" | while read -r f; do
  perl -pi -e "s{^(\s*(?:[A-Za-z_][A-Za-z0-9_]*\s+)?)\"\Qwa-api/${DEST}/proto\E((?:/[A-Za-z0-9_.-]+)*)\"(\s*)\$}{\$1\"wa-api/${DEST}/protocol/proto\$2\"\$3}" "$f"
done
if [ -f "$SRC/.gitattributes" ]; then
  cp "$SRC/.gitattributes" "$NORM/.gitattributes"
fi
gofmt -w "$NORM/proto" 2>/dev/null || true

# ADR-0004: escopo restrito a proto/ — só o código gerado continua sob a
# trava de diff-zero. O restante do módulo é fork ativo, comparação
# byte-a-byte deixou de fazer sentido para ele.
if [ ! -d "$NORM/proto" ] || [ ! -d "$DEST/protocol/proto" ]; then
  echo "ERRO: proto/ ausente em um dos lados da comparação." >&2
  exit 1
fi
diff -ru "$NORM/proto" "$DEST/protocol/proto"
