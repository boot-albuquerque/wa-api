#!/usr/bin/env bash
# Compara internal/waclient/ contra uma versão do go.mau.fi/whatsmeow
# upstream, normalizando o import path antes do diff (sem essa
# normalização, TODO arquivo apareceria como modificado só pela troca de
# path, tornando o diff inútil).
#
# Uso: scripts/waclient-diff.sh <versão>
#   ex: scripts/waclient-diff.sh v0.0.0-20260516102357-8d3700152a69
#
# Saída vazia (exit 0) = internal/waclient/ é cópia fiel da versão indicada,
# sem patches locais não registrados.
# Saída não-vazia = há diferença; revise se é patch conhecido (registrado em
# internal/waclient/PATCHES.md) ou deriva não intencional.
set -euo pipefail

VERSION="${1:?uso: scripts/waclient-diff.sh <versão>}"
MODULE="go.mau.fi/whatsmeow"
DEST="internal/waclient"
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
if [ -f "$SRC/.gitattributes" ]; then
  cp "$SRC/.gitattributes" "$NORM/.gitattributes"
fi
DOCGO="$NORM/binary/proto/doc.go"
if [ -f "$DOCGO" ]; then
  perl -pi -e "s{\Q${MODULE}\E/proto/wa\* packages}{wa-api/${DEST}/proto/wa* packages}" "$DOCGO"
fi
gofmt -w "$NORM" 2>/dev/null || true

# UPSTREAM/PROVENANCE.md/PATCHES.md são artefatos nossos, não existem na
# fonte normalizada — remove do lado vendorizado antes do diff pra não
# aparecerem como "removidos".
diff -ru \
  -x UPSTREAM \
  -x PROVENANCE.md \
  -x PATCHES.md \
  "$NORM" "$DEST"
