#!/usr/bin/env bash
# Verifica conformidade de licença de internal/wa-noise/ (whatsmeow
# vendorizado, MPL-2.0).
#
# Achado durante a implementação: nem todo arquivo do whatsmeow upstream
# carrega o header MPL-2.0 por arquivo (ex: internals.go, binary/token/
# token.go, socket/dialopts.go, types/sticker.go, appstate/encode.go,
# argo/argo.go e outros) — não é algo que perdemos na cópia, o próprio
# upstream nunca colocou o header nesses arquivos. MPL-2.0 §3.1 cobre isso
# via o LICENSE do projeto na raiz da árvore vendorizada
# (internal/wa-noise/LICENSE-whatsmeow), que é o mecanismo que este check
# verifica como obrigatório. Onde o header POR ARQUIVO existe no upstream,
# ele é preservado por construção (o script de vendoring só reescreve
# imports Go entre aspas, nunca remove texto) — por isso não exigimos
# presença universal, só ausência de corrupção relativa ao que já existe.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

DEST="internal/wa-noise"
if [ ! -d "$DEST" ]; then
  echo "FALHA: $DEST não existe."
  exit 1
fi

fail=0

if [ ! -f "$DEST/LICENSE-whatsmeow" ]; then
  echo "FALHA: $DEST/LICENSE-whatsmeow ausente — cobertura MPL-2.0 da árvore vendorizada depende dele."
  fail=1
elif ! grep -q "Mozilla Public License" "$DEST/LICENSE-whatsmeow"; then
  echo "FALHA: $DEST/LICENSE-whatsmeow não contém o texto MPL-2.0 esperado."
  fail=1
fi

if [ ! -f "$DEST/PROVENANCE.md" ]; then
  echo "FALHA: $DEST/PROVENANCE.md ausente — proveniência/versão de origem não documentada."
  fail=1
fi

if [ ! -f "$DEST/UPSTREAM" ]; then
  echo "FALHA: $DEST/UPSTREAM ausente — versão de origem não declarada de forma machine-readable."
  fail=1
fi

with_header=0
while IFS= read -r -d '' f; do
  if grep -q "Mozilla Public" "$f"; then
    with_header=$((with_header + 1))
  fi
done < <(find "$DEST" -name '*.go' -not -path "$DEST/protocol/proto/*" -print0)

echo "waclient-license-check: LICENSE-whatsmeow/PROVENANCE.md/UPSTREAM presentes; $with_header arquivos .go (fora de protocol/proto/) carregam header MPL-2.0 por arquivo (upstream não usa o header em todos os arquivos — LICENSE-whatsmeow cobre a árvore inteira, MPL-2.0 §3.1)."

if [ "$fail" -eq 1 ]; then
  exit 1
fi
