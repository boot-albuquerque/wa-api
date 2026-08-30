#!/usr/bin/env bash
# Trava da fachada de internal/noise/ (Fase H, etapa 6).
#
# A etapa 6 estabeleceu internal/noise/main.go (package noise) como a
# UNICA porta de entrada do fork para codigo fora dele. A direcao de dependencia
# declarada no inventario da Fase H (internal/noise/docs/FASE_H_INVENTORY.md,
# §"Direcao de dependencia declarada") diz, literalmente: **nada importa `core`**.
#
# Sem esta trava isso e' so' uma frase num documento. A etapa 5 e' a prova: ao
# mover os 114 arquivos da raiz para core/, os 44 consumidores foram
# mecanicamente repontados para `core` e ninguem percebeu que a arvore tinha
# perdido a fachada. Uma regra de arquitetura que nao falha o build nao e' uma
# regra, e' um comentario.
#
# Esta checagem falha se qualquer .go FORA de internal/noise/ importar
# wa-api/internal/noise/core. Dentro do fork o import continua livre: o
# proprio main.go precisa dele, e core/*_test.go tambem.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

FACADE="internal/noise/main.go"
if [ ! -f "$FACADE" ]; then
  echo "FALHA: $FACADE ausente — a fachada da Fase H (etapa 6) e' o contrato de entrada do fork."
  exit 1
fi

offenders="$(grep -rln '"wa-api/internal/noise/core"' --include='*.go' . \
  | grep -v '^\./internal/noise/' || true)"

if [ -n "$offenders" ]; then
  echo "FALHA: import direto de wa-api/internal/noise/core fora do fork:"
  echo "$offenders" | sed 's/^/  /'
  echo
  echo "core/ e' implementacao. Consumidores importam wa-api/internal/noise"
  echo "(a fachada em $FACADE). Se o simbolo que voce precisa nao esta la',"
  echo "adicione o alias na fachada — nao contorne por baixo."
  exit 1
fi

echo "waclient-facade-check: $FACADE presente; nenhum import direto de core/ fora do fork."
